package server_test

import (
	"context"
	"errors"
	"fmt"
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
// must stay well below the per-package timeout in the Makefile's test target.
const testTimeout = 20 * time.Second

// newTestSession builds a server over a fresh temporary vault, connects it to a
// client over the given transport pair, and returns the client session together
// with the vault path. Both sessions and the vault are released via t.Cleanup.
func newTestSession(t *testing.T, ctx context.Context, serverTransport, clientTransport mcp.Transport) (*mcp.ClientSession, string) {
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

	return clientSession, tmpVault
}

// TestNewRegistersAllTools connects a client over an in-memory transport and
// verifies the server exposes exactly the expected tool set
func TestNewRegistersAllTools(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	clientSession, _ := newTestSession(t, ctx, serverTransport, clientTransport)

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

// TestLargeNoteFitsFrameCap sends an update_note whose body is under
// vault.MaxContentSize but whose JSON frame escapes past the SDK's default
// inbound cap. With server.MaxFrameSize the call must be answered and the
// session must survive; the negative subtest proves the same frame is rejected
// under the SDK default, so the positive case really exercises the cap.
func TestLargeNoteFitsFrameCap(t *testing.T) {
	if testing.Short() {
		t.Skip("sends an 18 MiB frame twice; skipped in -short mode")
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

	// call runs update_note with the body over a pair of OS pipes whose server side
	// applies maxLineLength, then checks the session is still usable. Deadlines on
	// every pipe end make a stalled frame fail instead of blocking forever. Most of
	// the test's time under -race is the SDK decoding the 18 MiB frame, not the pipes.
	call := func(t *testing.T, maxLineLength int) (*mcp.CallToolResult, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

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
				t.Fatalf("SetDeadline failed: %v", dlErr)
			}
		}
		serverTransport := &mcp.IOTransport{Reader: serverReader, Writer: serverWriter, MaxLineLength: maxLineLength}
		clientTransport := &mcp.IOTransport{Reader: clientReader, Writer: clientWriter}
		clientSession, tmpVault := newTestSession(t, ctx, serverTransport, clientTransport)

		res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
			Name: "update_note",
			Arguments: map[string]any{
				"path":    filepath.Join(tmpVault, "note1.md"),
				"content": content,
				"mode":    "replace",
			},
		})
		if err != nil {
			return nil, err
		}
		if _, err := clientSession.ListTools(ctx, nil); err != nil {
			return res, fmt.Errorf("session unusable after the large frame: %w", err)
		}
		return res, nil
	}

	t.Run("server.MaxFrameSize answers the call", func(t *testing.T) {
		res, err := call(t, server.MaxFrameSize)
		if err != nil {
			t.Fatalf("update_note with a %d byte body failed: %v", len(content), err)
		}
		if res.IsError {
			t.Fatalf("update_note returned a tool error: %v", res.Content)
		}
	})

	t.Run("SDK default cap ends the session", func(t *testing.T) {
		_, err := call(t, mcp.DefaultMaxLineLength)
		if err == nil {
			t.Fatal("expected the SDK default cap to reject the frame; otherwise the positive case does not exercise server.MaxFrameSize")
		}
		// A deadline would also produce an error; only a torn-down connection
		// proves the cap fired.
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			t.Fatalf("control hit a timeout rather than the cap: %v", err)
		}
	})
}
