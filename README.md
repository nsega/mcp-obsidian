# mcp-obsidian

A Model Context Protocol (MCP) server for managing Markdown notes in Obsidian vaults with full Zettelkasten workflow support.

This is a Go rewrite inspired by [mcp-obsidian](https://github.com/smithery-ai/mcp-obsidian) using the [MCP Go SDK v1.3](https://github.com/modelcontextprotocol/go-sdk).

## Features

- **Search Notes**: Find notes by filename using case-insensitive matching with regex support
- **Read Notes**: Read the content of one or more notes with error handling per file
- **Create Notes**: Create notes with `YYYY-MM-DD_slug.md` naming convention and YAML frontmatter
- **Update Notes**: Replace or append content to existing notes
- **Delete Notes**: Safely remove markdown notes from the vault
- **Search Content**: Full-text search across note bodies with line numbers and snippets
- **Get Backlinks**: Discover all `[[wikilink]]` references to a given note
- **List Tags**: Collect and count tags from YAML frontmatter and inline `#tags`
- **Security**: Built-in path validation to prevent directory traversal and access to hidden files
- **Performance**: Native Go implementation for fast execution

## Installation

### From Source

```bash
git clone https://github.com/nsega/mcp-obsidian.git
cd mcp-obsidian
go build -o mcp-obsidian .
```

### Using go install

```bash
go install github.com/nsega/mcp-obsidian@latest
```

## Usage

### Command Line

Run the server with the path to your vault:

```bash
./mcp-obsidian /path/to/your/vault
```

The server communicates over stdin/stdout using the MCP protocol.

### Configuration for Claude Desktop

Add the following to your Claude Desktop configuration file:

**MacOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
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
- **Error Handling**: Individual file read failures don't halt operations

## Building

```bash
go build -o mcp-obsidian .
```

## Testing Locally

### 1. Create a Test Vault

First, create a test directory with some sample Markdown files:

```bash
# Create a test vault directory
mkdir -p ~/test-vault

# Create some sample notes
echo "# Meeting Notes" > ~/test-vault/meeting.md
echo "# Project Ideas" > ~/test-vault/project-ideas.md
echo "# Daily Journal" > ~/test-vault/journal.md
```

### 2. Build and Run the Server

```bash
# Build the binary
go build -o mcp-obsidian .

# Run the server (it will output startup messages to stderr)
./mcp-obsidian ~/test-vault
```

The server will start and wait for MCP protocol messages on stdin. You should see:
```
MCP Obsidian server starting...
Vault path: /home/user/test-vault
```

### 3. Using the MCP Inspector

The easiest way to test your MCP server is using the official MCP Inspector tool:

```bash
# Install the MCP Inspector
npx @modelcontextprotocol/inspector mcp-obsidian ~/test-vault
```

This will:
1. Start your MCP server
2. Open a web interface (usually at http://localhost:5173)
3. Allow you to interactively test all 8 tools
4. View the JSON-RPC messages being exchanged

### 4. Manual Testing with JSON-RPC

You can also test manually by sending JSON-RPC messages via stdin. Here's an example:

```bash
# Start the server
./mcp-obsidian ~/test-vault

# Then send an initialize request (paste this JSON):
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}

# After initialization, call the search_notes tool:
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_notes","arguments":{"query":"meeting"}}}
```

### 5. Verify in Claude Desktop

After configuring the server in Claude Desktop (see Configuration section above):

1. Restart Claude Desktop
2. Open the Claude Desktop logs to verify the server started:
   - **MacOS**: `~/Library/Logs/Claude/mcp*.log`
   - **Windows**: `%APPDATA%\Claude\logs\mcp*.log`
3. In a conversation, you should see the tools become available
4. Try asking: "Search my notes for 'meeting'" or "Read my project-ideas note"

### Troubleshooting

**Server doesn't start:**
- Verify the vault path exists: `ls ~/test-vault`
- Check file permissions: `ls -la ~/test-vault`
- Ensure the binary is executable: `chmod +x mcp-obsidian`

**Tools not appearing in Claude Desktop:**
- Check the configuration file path is correct
- Verify the JSON syntax in the config file
- Restart Claude Desktop after configuration changes
- Check Claude Desktop logs for error messages

**Permission errors:**
- Ensure the vault directory is readable
- Check that you're not trying to access hidden files (those starting with `.`)
- Verify the path is absolute, not relative

**No results from search:**
- Verify your vault contains `.md` files
- Check that the search query matches your filenames
- Remember that search is case-insensitive and supports regex

## Requirements

- Go 1.25 or later
- MCP Go SDK v1.3

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
