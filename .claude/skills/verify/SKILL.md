---
name: verify
description: Drive the mcp-obsidian stdio MCP server end-to-end against a scratch vault to verify changes at the real tool surface.
---

# Verifying mcp-obsidian

Build: `make build` → `build/mcp-obsidian` (or `go build -o <out> .`).
Run: `build/mcp-obsidian <vault-path>`; speaks newline-delimited JSON-RPC on stdio, INFO logs on stderr.

## Drive a session

Create a scratch vault (include: plain notes, YAML frontmatter tags, inline #tags, `[[wikilinks]]` with and without alias, a `subfolder/`, a `.hidden/` dir, and a non-`.md` file). Then pipe requests:

```bash
{ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_notes","arguments":{"query":"note"}}}' \
; sleep 1; } | build/mcp-obsidian "$VAULT" 2>stderr.log | jq -c '{id, err: (.result.isError // false), text: .result.content[0].text}'
```

Tools: search_notes, read_notes, create_note, update_note (replace|append), delete_note, search_content, get_backlinks, list_tags (optional prefix). Tool failures come back as `result.isError: true` with the message in content text, not as JSON-RPC errors.

## Gotchas

- **Requests in one stdin burst are handled concurrently**; responses arrive out of order and read-after-write across tools races. For deterministic sequences (and for old-vs-new output diffing), `sleep 0.3` between requests and sort responses by id before comparing.
- The server exits when stdin closes; trailing `sleep` in the subshell keeps it open long enough to answer.
- Differential regression check: `git worktree add <tmp> <old-sha>`, build old binary, run the identical sequenced session against fresh identical vaults, `sed` the vault paths to a common token, diff.
- Good probes that all return clean errors: query `".*"` (regex metachars are literal via QuoteMeta), read path outside vault or under a dotdir (per-file "Access denied" inside content, not a call failure), update_note with bogus mode, delete_note on non-.md, double delete.
