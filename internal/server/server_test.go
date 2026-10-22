package server_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/handler"
	"github.com/nsega/mcp-obsidian/internal/server"
	"github.com/nsega/mcp-obsidian/internal/testutil"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

// TestNewRegistersAllTools connects a client over an in-memory transport and
// verifies the server exposes exactly the expected tool set
func TestNewRegistersAllTools(t *testing.T) {
	tmpVault := testutil.SetupTestVault(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	v, err := vault.New(tmpVault)
	if err != nil {
		t.Fatalf("vault.New failed: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := handler.New(v, logger)
	srv := server.New(h, "test", logger)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect failed: %v", err)
	}
	// nolint:errcheck
	defer func() { _ = serverSession.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect failed: %v", err)
	}
	// nolint:errcheck
	defer func() { _ = clientSession.Close() }()

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
// 16 MiB inbound cap, and checks the call is answered and the session survives.
// That is what server.MaxFrameSize exists for: without it the SDK ends the
// session instead of letting the handler reply.
func TestLargeNoteFitsFrameCap(t *testing.T) {
	tmpVault := testutil.SetupTestVault(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	v, err := vault.New(tmpVault)
	if err != nil {
		t.Fatalf("vault.New failed: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := handler.New(v, logger)
	srv := server.New(h, "test", logger)

	ctx := context.Background()
	serverConn, clientConn := net.Pipe()
	serverTransport := &mcp.IOTransport{Reader: serverConn, Writer: serverConn, MaxLineLength: server.MaxFrameSize}
	clientTransport := &mcp.IOTransport{Reader: clientConn, Writer: clientConn}

	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect failed: %v", err)
	}
	// nolint:errcheck
	defer func() { _ = serverSession.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect failed: %v", err)
	}
	// nolint:errcheck
	defer func() { _ = clientSession.Close() }()

	// Every double quote escapes to two bytes, so a 9 MiB body (accepted by the
	// handler) becomes an 18 MiB frame (rejected by the SDK default cap).
	content := strings.Repeat(`"`, 9<<20)
	if len(content) > vault.MaxContentSize {
		t.Fatalf("test body %d bytes exceeds vault.MaxContentSize %d; it must be accepted by the handler", len(content), vault.MaxContentSize)
	}
	if 2*len(content) <= mcp.DefaultMaxLineLength {
		t.Fatalf("escaped test body (%d bytes) does not exceed mcp.DefaultMaxLineLength (%d); the test would not exercise the cap", 2*len(content), mcp.DefaultMaxLineLength)
	}

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "update_note",
		Arguments: map[string]any{
			"path":    filepath.Join(tmpVault, "note1.md"),
			"content": content,
			"mode":    "replace",
		},
	})
	if err != nil {
		t.Fatalf("update_note with a %d byte body failed at the transport: %v", len(content), err)
	}
	if res.IsError {
		t.Fatalf("update_note returned a tool error: %v", res.Content)
	}

	// The session must still be usable after the large frame.
	if _, err := clientSession.ListTools(ctx, nil); err != nil {
		t.Fatalf("session unusable after the large frame: %v", err)
	}
}
