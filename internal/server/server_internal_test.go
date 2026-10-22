package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/handler"
	"github.com/nsega/mcp-obsidian/internal/testutil"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

// TestLimitArguments checks the receiving middleware answers an oversized
// tools/call with a tool error before the handler runs, passes everything
// else through untouched, and leaves other methods alone.
func TestLimitArguments(t *testing.T) {
	t.Parallel()
	const limit = 16

	calls := 0
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		calls++
		return &mcp.CallToolResult{}, nil
	}
	mh := limitArguments(limit)(next)
	ctx := context.Background()
	call := func(args string) *mcp.CallToolRequest {
		return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "update_note", Arguments: json.RawMessage(args)}}
	}

	res, err := mh(ctx, "tools/call", call(`{"content":"`+strings.Repeat("x", limit)+`"}`))
	if err != nil {
		t.Fatalf("over-limit call returned an error instead of a tool error: %v", err)
	}
	ctr, ok := res.(*mcp.CallToolResult)
	if !ok || !ctr.IsError {
		t.Fatalf("over-limit call did not return a tool error: %#v", res)
	}
	if len(ctr.Content) != 1 {
		t.Fatalf("expected one content item in the tool error, got %#v", ctr.Content)
	}
	if text, ok := ctr.Content[0].(*mcp.TextContent); !ok || !strings.Contains(text.Text, "arguments too large") {
		t.Fatalf("unexpected tool error content: %#v", ctr.Content)
	}
	if calls != 0 {
		t.Fatalf("handler ran for an over-limit call (%d calls)", calls)
	}

	if _, err := mh(ctx, "tools/call", call(strings.Repeat("x", limit))); err != nil {
		t.Fatalf("at-limit call failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("at-limit call did not reach the handler (%d calls)", calls)
	}

	if _, err := mh(ctx, "tools/list", &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}}); err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("a non tools/call method did not reach the handler (%d calls)", calls)
	}
}

// TestNewWiresLimitArguments connects a real client to a server built by the
// same constructor New uses, with a tiny arguments bound, and checks an
// over-limit tools/call comes back as a tool error while the session stays up.
func TestNewWiresLimitArguments(t *testing.T) {
	t.Parallel()
	const limit = 64

	tmpVault := testutil.SetupTestVault(t)
	t.Cleanup(func() { testutil.CleanupTestVault(t, tmpVault) })
	v, err := vault.New(tmpVault)
	if err != nil {
		t.Fatalf("vault.New failed: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := newWithLimit(handler.New(v, logger), "test", logger, limit)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
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

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_notes",
		Arguments: map[string]any{"query": strings.Repeat("x", 2*limit)},
	})
	if err != nil {
		t.Fatalf("over-limit call failed at the protocol level instead of as a tool error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("over-limit call was not rejected: %#v", res)
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected one content item in the tool error, got %#v", res.Content)
	}
	if text, ok := res.Content[0].(*mcp.TextContent); !ok || !strings.Contains(text.Text, "arguments too large") {
		t.Fatalf("unexpected tool error content: %#v", res.Content)
	}
	if _, err := clientSession.ListTools(ctx, nil); err != nil {
		t.Fatalf("session unusable after the rejected call: %v", err)
	}
}
