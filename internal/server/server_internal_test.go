package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
