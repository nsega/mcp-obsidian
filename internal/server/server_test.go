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

// testTimeout bounds every session and call in this package so a stalled
// transport fails the test instead of hanging until the go test deadline. It
// is sized for a CPU-bound 18 MiB decode under -race on a slow runner and must
// stay below the per-package timeout in the Makefile's test target.
const testTimeout = 90 * time.Second

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
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
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
// a deadline so a stalled frame fails instead of blocking forever; platforms
// whose anonymous pipes cannot take deadlines (Windows) skip the test.
func pipeTransports(t *testing.T, maxLineLength int) (serverTransport, clientTransport mcp.Transport) {
	t.Helper()

	serverReader, clientWriter, err := os.Pipe() // client -> server
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	clientReader, serverWriter, err := os.Pipe() // server -> client
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	deadline := time.Now().Add(testTimeout)
	for _, f := range []*os.File{serverReader, clientWriter, clientReader, serverWriter} {
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

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	serverTransport, clientTransport := pipeTransports(t, server.MaxFrameSize)
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
// with the SDK's frame error, and the client loses the connection. A small cap
// and body show this in milliseconds; the error is asserted on the server side
// so a pipe or context deadline cannot pass for the cap firing.
func TestFrameCapEndsSession(t *testing.T) {
	t.Parallel()

	const cap = 4 << 10 // large enough for the initialize handshake
	content := strings.Repeat("x", 2*cap)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	serverTransport, clientTransport := pipeTransports(t, cap)
	clientSession, serverSession, tmpVault := newTestSession(t, ctx, serverTransport, clientTransport)

	if _, err := updateNote1(ctx, clientSession, tmpVault, content); err == nil {
		t.Fatal("expected the frame to be rejected under a cap smaller than the request")
	}

	err := serverSession.Wait()
	if err == nil || !strings.Contains(err.Error(), "maximum line length") {
		t.Fatalf("server session did not end with the SDK frame-cap error: %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		t.Fatalf("server session ended on a timeout rather than the cap: %v", err)
	}
}
