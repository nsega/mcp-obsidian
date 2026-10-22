package server

import (
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/handler"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

// MaxFrameSize bounds a single inbound JSON-RPC frame on the stdio transport.
// Note content is capped at vault.MaxContentSize before it is written, but JSON
// escaping can expand it up to six times (control characters become \u00XX), so
// the frame cap is sized to admit any content the handlers would accept, plus
// headroom for the rest of the request. Content up to vault.MaxContentSize
// therefore never trips the cap on its own, and content above it gets the
// handlers' "content too large" tool error. The other string fields (path, title,
// tags) are not bounded by the handlers and share the headroom; a frame beyond
// MaxFrameSize still ends the session, and with it the process, which is what
// keeps memory per frame bounded. Changing the cap, or disabling it with a
// negative StdioTransport.MaxLineLength, is a code change in main.go.
const MaxFrameSize = 6*vault.MaxContentSize + 1<<20

// New creates a configured MCP server with all tools registered
func New(h *handler.Handler, version string, logger *slog.Logger) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-obsidian",
		Version: version,
	}, &mcp.ServerOptions{
		Logger: logger,
	})

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
