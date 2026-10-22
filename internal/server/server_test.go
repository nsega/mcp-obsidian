package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/handler"
	"github.com/nsega/mcp-obsidian/internal/server"
	"github.com/nsega/mcp-obsidian/internal/testutil"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

// maxCallTimeout is the longest a session or call in this package may take; it
// is sized for a CPU-bound 18 MiB decode under -race on a slow runner.
const maxCallTimeout = 90 * time.Second

// callTimeout bounds every session and call so a stalled transport fails the
// test instead of hanging. It stays under the go test deadline (the Makefile's
// per-package -timeout, or the default) by a margin, so a stall surfaces as a
// context error rather than a test-binary panic.
func callTimeout(t *testing.T) time.Duration {
	t.Helper()
	timeout := maxCallTimeout
	if deadline, ok := t.Deadline(); ok {
		if left := time.Until(deadline) - 5*time.Second; left < timeout {
			timeout = left
		}
	}
	if timeout <= 0 {
		t.Fatalf("not enough time left before the test deadline (%v)", timeout)
	}
	return timeout
}

// newTestSession builds a server over a fresh temporary vault, connects it to a
// client over the given transport pair, and returns both sessions together with
// the vault path. The sessions and the vault are released via t.Cleanup.
func newTestSession(t *testing.T, ctx context.Context, serverTransport, clientTransport mcp.Transport) (*mcp.ClientSession, *mcp.ServerSession, string) {
	t.Helper()

	tmpVault := testutil.SetupTestVault(t)
	t.Cleanup(func() { testutil.CleanupTestVault(t, tmpVault) })

	v, err := vault.New(tmpVault)
	if err != nil {
		t.Fatalf("vault.New failed: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(handler.New(v, logger), "test", logger)

	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect failed: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := serverSession.Close(); closeErr != nil {
			t.Logf("server session close: %v", closeErr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect failed: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := clientSession.Close(); closeErr != nil {
			t.Logf("client session close: %v", closeErr)
		}
	})

	return clientSession, serverSession, tmpVault
}

// TestNewRegistersAllTools connects a client over an in-memory transport and
// verifies the server exposes exactly the expected tool set
func TestNewRegistersAllTools(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout(t))
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	clientSession, _, _ := newTestSession(t, ctx, serverTransport, clientTransport)

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	want := []string{
		"create_note",
		"delete_note",
		"get_backlinks",
		"list_tags",
		"read_notes",
		"search_content",
		"search_notes",
		"update_note",
	}

	got := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("registered tools = %v, want %v", got, want)
	}

	for _, tool := range res.Tools {
		if tool.Description == "" {
			t.Errorf("tool %q has an empty description", tool.Name)
		}
	}
}

// pipeTransports returns a server and client transport joined by two OS pipes,
// with the given inbound frame cap on the server side. Every pipe end carries
// the deadline so a stalled frame fails instead of blocking forever; platforms
// whose anonymous pipes cannot take deadlines (Windows) skip the test.
func pipeTransports(t *testing.T, maxLineLength int, deadline time.Time) (serverTransport, clientTransport mcp.Transport) {
	t.Helper()

	serverReader, clientWriter, err := os.Pipe() // client -> server
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	clientReader, serverWriter, err := os.Pipe() // server -> client
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	files := []*os.File{serverReader, clientWriter, clientReader, serverWriter}
	t.Cleanup(func() {
		for _, f := range files {
			// On the normal path the SDK has already closed them.
			if closeErr := f.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
				t.Logf("pipe close: %v", closeErr)
			}
		}
	})
	for _, f := range files {
		if dlErr := f.SetDeadline(deadline); dlErr != nil {
			if errors.Is(dlErr, os.ErrNoDeadline) {
				t.Skipf("anonymous pipes do not support deadlines on this platform: %v", dlErr)
			}
			t.Fatalf("SetDeadline failed: %v", dlErr)
		}
	}
	return &mcp.IOTransport{Reader: serverReader, Writer: serverWriter, MaxLineLength: maxLineLength},
		&mcp.IOTransport{Reader: clientReader, Writer: clientWriter}
}

// updateNote1 sends update_note for the test vault's note1.md with the given
// body and returns the call result, or the error the client saw.
func updateNote1(ctx context.Context, clientSession *mcp.ClientSession, tmpVault, content string) (*mcp.CallToolResult, error) {
	return clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "update_note",
		Arguments: map[string]any{
			"path":    filepath.Join(tmpVault, "note1.md"),
			"content": content,
			"mode":    "replace",
		},
	})
}

// TestLargeNoteFitsFrameCap sends an update_note whose body is under
// vault.MaxContentSize but whose JSON frame escapes past the SDK's default
// inbound cap, and checks that with server.MaxFrameSize the call is answered
// and the session survives. The static guards prove the frame exceeds the SDK
// default; TestFrameCapEndsSession proves what the SDK does on overflow.
func TestLargeNoteFitsFrameCap(t *testing.T) {
	if testing.Short() {
		t.Skip("decodes an 18 MiB frame; skipped in -short mode")
	}
	t.Parallel()

	// Every double quote escapes to two bytes, so a 9 MiB body (accepted by the
	// handler) becomes an 18 MiB frame (rejected by the SDK default cap).
	content := strings.Repeat(`"`, 9<<20)
	if len(content) > vault.MaxContentSize {
		t.Fatalf("test body %d bytes exceeds vault.MaxContentSize %d; it must be accepted by the handler", len(content), vault.MaxContentSize)
	}
	if 2*len(content) <= mcp.DefaultMaxLineLength {
		t.Fatalf("escaped test body (%d bytes) does not exceed mcp.DefaultMaxLineLength (%d); the test would not exercise the cap", 2*len(content), mcp.DefaultMaxLineLength)
	}

	deadline := time.Now().Add(callTimeout(t))
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	serverTransport, clientTransport := pipeTransports(t, server.MaxFrameSize, deadline)
	clientSession, _, tmpVault := newTestSession(t, ctx, serverTransport, clientTransport)

	// Most of this call's time under -race is the SDK decoding the frame.
	res, err := updateNote1(ctx, clientSession, tmpVault, content)
	if err != nil {
		t.Fatalf("update_note with a %d byte body failed: %v", len(content), err)
	}
	if res.IsError {
		t.Fatalf("update_note returned a tool error: %v", res.Content)
	}
	if _, err := clientSession.ListTools(ctx, nil); err != nil {
		t.Fatalf("session unusable after the large frame: %v", err)
	}
}

// TestFrameCapEndsSession proves the mechanism the cap guards against: a frame
// over the server's MaxLineLength is not answered, the server session ends
// with an error, and the client loses the connection. A small cap and body
// show this in milliseconds.
func TestFrameCapEndsSession(t *testing.T) {
	t.Parallel()

	const frameCap = 4 << 10 // large enough for the initialize handshake
	content := strings.Repeat("x", 2*frameCap)

	deadline := time.Now().Add(callTimeout(t))
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	serverTransport, clientTransport := pipeTransports(t, frameCap, deadline)
	clientSession, serverSession, tmpVault := newTestSession(t, ctx, serverTransport, clientTransport)

	// The handshake and a small call must fit under the cap, so a failure below
	// is attributable to the oversized frame and not to the cap being too small.
	if _, err := clientSession.ListTools(ctx, nil); err != nil {
		t.Fatalf("a small call did not fit under a %d byte cap: %v", frameCap, err)
	}

	if _, err := updateNote1(ctx, clientSession, tmpVault, content); err == nil {
		t.Fatal("expected the frame to be rejected under a cap smaller than the request")
	}

	// The SDK exports no sentinel for the frame error and its wording is not
	// stable, so assert the stable facts: the server session ended with an error
	// that is neither a clean EOF nor a deadline, and log the message.
	err := serverSession.Wait()
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("server session did not end with an error after the oversized frame: %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		t.Fatalf("server session ended on a timeout rather than the cap: %v", err)
	}
	t.Logf("server session ended with: %v", err)
}
