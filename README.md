# mcp-obsidian (Go Edition)

A Model Context Protocol (MCP) server for reading and searching Markdown notes in directories like Obsidian vaults.

This is a Go rewrite of the original [mcp-obsidian](https://github.com/smithery-ai/mcp-obsidian) using the [MCP Go SDK v1.1](https://github.com/modelcontextprotocol/go-sdk).

## Features

- **Search Notes**: Find notes by filename using case-insensitive matching with regex support
- **Read Notes**: Read the content of one or more notes with error handling per file
- **Security**: Built-in path validation to prevent directory traversal and access to hidden files
- **Performance**: Native Go implementation for fast execution

## Installation

### From Source

```bash
git clone https://github.com/smithery-ai/mcp-obsidian.git
cd mcp-obsidian
go build -o mcp-obsidian .
```

### Using go install

```bash
go install github.com/smithery-ai/mcp-obsidian@latest
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

## Requirements

- Go 1.21 or later
- MCP Go SDK v1.1

## License

AGPL-3.0

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
