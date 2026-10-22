package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/handler"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

// MaxArgumentsSize bounds the raw, JSON-encoded arguments of a tools/call
// request. It is enforced by a receiving middleware for every tool, current and
// future, and sized so that the largest note content the handlers accept still
// fits after JSON escaping (vault.MaxContentSize can expand up to six times:
// control characters, the HTML-safe set < > &, and invalid UTF-8 bytes each
// become a six-byte \uXXXX sequence) with room for the other arguments. A call
// over the limit gets a tool error instead of reaching a handler.
const MaxArgumentsSize = 6*vault.MaxContentSize + 1<<20

// MaxFrameSize bounds a single inbound JSON-RPC frame on the stdio transport.
// The transport decodes frames up to this size and ends the session (and with
// it the process) on anything larger, regardless of the middleware, which only
// runs on requests that were decoded. It is MaxArgumentsSize plus 4 MiB of
// headroom for the rest of the request (method, id, tool name, _meta,
// whitespace), so every tools/call the middleware lets through, and the
// oversized ones it answers with a tool error, is decodable unless more than
// that headroom sits outside the arguments. Changing the cap, or disabling it
// with a negative StdioTransport.MaxLineLength, is a code change in main.go.
const MaxFrameSize = MaxArgumentsSize + 4<<20

// limitArguments returns a receiving middleware that answers a tools/call whose
// raw arguments exceed limit bytes with a tool error, before any handler runs.
func limitArguments(limit int) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// The SDK builds a CallToolRequest only for tools/call, so the type
			// assertion is the whole check.
			if call, ok := req.(*mcp.CallToolRequest); ok && len(call.Params.Arguments) > limit {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("arguments too large: %d bytes exceeds limit of %d", len(call.Params.Arguments), limit)}},
				}, nil
			}
			return next(ctx, method, req)
		}
	}
}

// New creates a configured MCP server with all tools registered
func New(h *handler.Handler, version string, logger *slog.Logger) *mcp.Server {
	return newWithLimit(h, version, logger, MaxArgumentsSize)
}

// newWithLimit is New with the tools/call arguments bound injectable, so a test
// can prove the middleware is wired without sending MaxArgumentsSize bytes.
func newWithLimit(h *handler.Handler, version string, logger *slog.Logger, argumentsLimit int) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-obsidian",
		Version: version,
	}, &mcp.ServerOptions{
		Logger: logger,
	})
	srv.AddReceivingMiddleware(limitArguments(argumentsLimit))

	// Register search_notes tool
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_notes",
		Description: "Search for notes by filename using case-insensitive matching. Supports regex patterns.",
	}, h.SearchNotes)

	// Register read_notes tool
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_notes",
		Description: "Read the content of one or more notes. Returns content with error handling per file.",
	}, h.ReadNotes)

	// Register create_note tool
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_note",
		Description: "Create a new note with YYYY-MM-DD_slug.md naming convention and YAML frontmatter. Supports tags and subfolder placement.",
	}, h.CreateNote)

	// Register update_note tool
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_note",
		Description: "Update an existing note's content. Supports 'replace' (overwrite) and 'append' modes.",
	}, h.UpdateNote)

	// Register delete_note tool
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_note",
		Description: "Delete a markdown note from the vault. Only .md files can be deleted.",
	}, h.DeleteNote)

	// Register search_content tool
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_content",
		Description: "Full-text search across note bodies. Returns matching file paths, line numbers, and snippets. Supports regex patterns.",
	}, h.SearchContent)

	// Register get_backlinks tool
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_backlinks",
		Description: "Find all notes that link to a given note via [[wikilinks]]. Supports [[note]] and [[note|alias]] syntax.",
	}, h.GetBacklinks)

	// Register list_tags tool
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_tags",
		Description: "List all tags found across the vault from YAML frontmatter and inline #tags. Optionally filter by prefix.",
	}, h.ListTags)

	return srv
}
