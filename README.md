# mcp-obsidian

[![Go Version](https://img.shields.io/github/go-mod/go-version/nsega/mcp-obsidian)](https://go.dev/)
[![Build and Test](https://github.com/nsega/mcp-obsidian/actions/workflows/build-and-test.yml/badge.svg)](https://github.com/nsega/mcp-obsidian/actions/workflows/build-and-test.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A Model Context Protocol (MCP) server for managing Markdown notes in Obsidian vaults with full Zettelkasten workflow support.

This is a Go rewrite inspired by [mcp-obsidian](https://github.com/smithery-ai/mcp-obsidian) using the [MCP Go SDK v1.6](https://github.com/modelcontextprotocol/go-sdk).

## Features

- **Search Notes**: Find notes by filename using case-insensitive matching with regex support
- **Read Notes**: Read the content of one or more notes with error handling per file
- **Create Notes**: Create notes with `YYYY-MM-DD_slug.md` naming convention and YAML frontmatter
- **Update Notes**: Replace or append content to existing notes
- **Delete Notes**: Safely remove markdown notes from the vault
- **Search Content**: Full-text search across note bodies with line numbers and snippets
- **Get Backlinks**: Discover all `[[wikilink]]` references to a given note
- **List Tags**: Collect and count tags from YAML frontmatter and inline `#tags`
- **Structured Logging**: Uses Go's standard `log/slog` with leveled, key-value output to stderr
- **Security**: Built-in path validation to prevent directory traversal and access to hidden files
- **Performance**: Native Go static binary with zero CGO dependencies

## Installation

### From Source

```bash
git clone https://github.com/nsega/mcp-obsidian.git
cd mcp-obsidian
make build
```

The binary is written to `build/mcp-obsidian`.

### Using go install

```bash
go install github.com/nsega/mcp-obsidian@latest
```

## Usage

### Command Line

Run the server with the path to your vault:

```bash
./build/mcp-obsidian /path/to/your/vault
```

The server communicates over stdin/stdout using the MCP protocol.

### Configuration for Claude Desktop

Add the following to your Claude Desktop configuration file:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "obsidian": {
      "command": "/path/to/mcp-obsidian",
      "args": ["/path/to/your/vault"]
    }
  }
}
```

### Configuration for Claude Code

Add to your `~/.claude/settings.json` or project `.claude/settings.json`:

```json
{
  "mcpServers": {
    "obsidian": {
      "command": "/path/to/mcp-obsidian",
      "args": ["/path/to/your/vault"]
    }
  }
}
```

### Configuration for VS Code

Add to your `.vscode/mcp.json`:

```json
{
  "mcpServers": {
    "obsidian": {
      "command": "/path/to/mcp-obsidian",
      "args": ["/path/to/your/vault"]
    }
  }
}
```

## Tools

### search_notes

Search for notes by filename using case-insensitive matching.

**Input**:
- `query` (string, required): Search query or regex pattern

**Example**:
```json
{
  "query": "meeting"
}
```

Returns up to 200 matching file paths.

### read_notes

Read the content of one or more notes.

**Input**:
- `paths` (array of strings, required): File paths to read

**Example**:
```json
{
  "paths": [
    "/path/to/vault/note1.md",
    "/path/to/vault/note2.md"
  ]
}
```

Returns the content of each note with error handling for individual files.

### create_note

Create a new note with the Zettelkasten naming convention and YAML frontmatter.

**Input**:
- `title` (string, required): Title for the note (used in filename slug and heading)
- `content` (string, optional): Markdown body content
- `folder` (string, optional): Subfolder within the vault (e.g. `30_Permanent`, `10_FleetingNote`)
- `tags` (array of strings, optional): Frontmatter tags

**Example**:
```json
{
  "title": "GTD Zettelkasten Flowchart",
  "content": "A note about combining GTD with Zettelkasten.",
  "folder": "30_Permanent",
  "tags": ["zettelkasten", "gtd", "productivity"]
}
```

Creates a file like `30_Permanent/2026-02-15_gtd-zettelkasten-flowchart.md` with YAML frontmatter containing tags, created, and updated dates.

### update_note

Update an existing note's content.

**Input**:
- `path` (string, required): Full path to the note
- `content` (string, required): New content to write or append
- `mode` (string, optional): `replace` (default) or `append`

**Example**:
```json
{
  "path": "/path/to/vault/30_Permanent/2026-02-15_my-note.md",
  "content": "\n## New Section\nAdditional thoughts.",
  "mode": "append"
}
```

### delete_note

Delete a markdown note from the vault.

**Input**:
- `path` (string, required): Full path to the note to delete

**Example**:
```json
{
  "path": "/path/to/vault/00_Inbox/2026-02-15_scratch.md"
}
```

Only `.md` files can be deleted. Directories cannot be deleted.

### search_content

Full-text search across note bodies. Returns matching file paths, line numbers, and snippets.

**Input**:
- `query` (string, required): Search query or regex pattern

**Example**:
```json
{
  "query": "Zettelkasten"
}
```

Returns up to 200 matches with file path, line number, and the matching line content.

### get_backlinks

Find all notes that link to a given note via `[[wikilinks]]`.

**Input**:
- `note_name` (string, required): Note name without `.md` extension or path

**Example**:
```json
{
  "note_name": "my-permanent-note"
}
```

Returns source file paths and the lines containing the wikilinks. Supports both `[[note]]` and `[[note|alias]]` syntax.

### list_tags

List all tags found across the vault from YAML frontmatter and inline `#tags`.

**Input**:
- `prefix` (string, optional): Filter tags by prefix

**Example**:
```json
{
  "prefix": "project"
}
```

Returns a sorted, deduplicated list of tags with their occurrence counts.

## Security

The server implements several security measures:

- **Path Validation**: All file operations are restricted to the specified vault directory
- **Hidden Files**: Access to files and directories starting with `.` is denied
- **Symlink Resolution**: Symlinks are resolved and validated to prevent directory escape attacks
- **Error Handling**: Individual file read failures don't halt operations (logged at `Debug` level via `slog`)

## Project Structure

```
main.go                     CLI entry point, slog setup, wiring
internal/
├── note/
│   ├── types.go            Input/Output structs for all 8 tools
│   ├── util.go             Slugify, GenerateFrontmatter, ParseFrontmatterTags
│   └── util_test.go
├── vault/
│   ├── vault.go            Vault struct, path validation, symlink resolution
│   └── vault_test.go
├── handler/
│   ├── handler.go          8 MCP tool handler methods (with injected logger)
│   └── handler_test.go
├── server/
│   └── server.go           MCP server creation, tool registration, logger passthrough
└── testutil/
    └── testutil.go         Shared test vault setup/cleanup helpers
```

## Development

### Prerequisites

- Go 1.27.1 or later
- [golangci-lint](https://golangci-lint.run/welcome/install/) (for linting)

### Build

```bash
make build          # Build binary to build/mcp-obsidian
make build-all      # Cross-compile for linux/darwin/windows (amd64/arm64)
```

### Test

```bash
make test           # Run all tests with race detector
make coverage       # Run tests with HTML coverage report
```

### Lint

```bash
make lint           # Run golangci-lint
make check          # Run fmt + vet + lint + test
```

### Run

```bash
make run VAULT=/path/to/your/vault
```

Run `make help` for the full list of targets.

### Testing with the MCP Inspector

The easiest way to interactively test the server is with the official MCP Inspector:

```bash
npx @modelcontextprotocol/inspector ./build/mcp-obsidian ~/my-vault
```

This opens a web UI at `http://localhost:5173` where you can test all 8 tools and view the JSON-RPC messages.

### Manual Testing with JSON-RPC

You can also test by piping JSON-RPC messages via stdin:

```bash
(printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}\n'; \
 sleep 0.5; \
 printf '{"jsonrpc":"2.0","method":"notifications/initialized"}\n'; \
 sleep 0.3; \
 printf '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}\n'; \
 sleep 1) | ./build/mcp-obsidian ~/my-vault 2>/dev/null
```

### Troubleshooting

**Server doesn't start:**
- Verify the vault path exists and is readable
- Ensure the binary is executable: `chmod +x build/mcp-obsidian`

**Tools not appearing in Claude Desktop / Claude Code:**
- Check the configuration file path and JSON syntax
- Restart the client after configuration changes
- Check logs for error messages:
  - Claude Desktop: `~/Library/Logs/Claude/mcp*.log` (macOS)
  - Claude Code: check terminal output

**Permission errors:**
- Ensure the vault directory is readable
- Hidden files (starting with `.`) are denied by design
- Use absolute paths, not relative

**No results from search:**
- Verify your vault contains `.md` files
- Search is case-insensitive and supports regex

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
