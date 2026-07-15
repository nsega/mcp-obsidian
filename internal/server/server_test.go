package server_test

import (
	"context"
	"io"
	"log/slog"
	"slices"
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
