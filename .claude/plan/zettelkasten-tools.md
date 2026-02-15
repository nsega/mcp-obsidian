# Plan: Add Zettelkasten MCP Tools to mcp-obsidian

## Context

The current mcp-obsidian server only has 2 read-only tools (`search_notes`, `read_notes`). To use this MCP via Claude Code for managing an Obsidian vault following the user's GTD + Zettelkasten workflow, we need write operations, content search, backlink discovery, and tag management.

### User's Vault Structure (actual)
```
nsega-notebook/
  00_Inbox/          # Uncategorized captures
  10_FleetingNote/   # Daily notes, meetings, brain dumps
  20_Literature/     # Books/, Articles/, AI-Research/
  30_Permanent/      # Refined Zettelkasten notes
  40_MOC/            # Maps of Content (MOC_*.md)
  50_OKRs/
  60_Diary/
  61_WeeklyReview/
  90_Templates/      # Note templates
```

### User's Filename Convention
`YYYY-MM-DD_title-slug.md` (e.g. `2026-02-15_gtd-zettelkasten-flowchart.md`)

### User's Frontmatter Pattern (from MOC/templates)
```yaml
---
tags:
  - tag1
  - tag2
created: YYYY-MM-DD
updated: YYYY-MM-DD
---
```

## New Tools (6 total)

### 1. `create_note`
Create a new note with the user's naming convention and frontmatter.

**Input:**
```go
type CreateNoteInput struct {
    Title   string   `json:"title"`            // Required. Used for filename slug and heading
    Content string   `json:"content"`           // Markdown body
    Folder  string   `json:"folder,omitempty"`  // Subfolder (e.g. "30_Permanent", "10_FleetingNote")
    Tags    []string `json:"tags,omitempty"`    // Frontmatter tags
}
```

**Behavior:**
- Filename: `YYYY-MM-DD_{slugified-title}.md` (today's date)
- Frontmatter: `tags`, `created`, `updated` fields
- Content placed after frontmatter with `# {Title}` heading
- Create subfolder via `os.MkdirAll` if it doesn't exist
- Error if file already exists (no silent overwrite)
- Validate path with `isPathAllowed()`

**Helper:** `slugify(s string) string` — lowercase, replace spaces/special chars with `-`, collapse multiple hyphens

### 2. `update_note`
Modify an existing note's content.

**Input:**
```go
type UpdateNoteInput struct {
    Path    string `json:"path"`              // Full path to the note
    Content string `json:"content"`           // New content
    Mode    string `json:"mode,omitempty"`    // "replace" (default) or "append"
}
```

**Behavior:**
- `replace`: overwrite entire file content
- `append`: read existing, add `\n\n` + new content, write back
- Validate with `isPathAllowed()`, verify file exists

### 3. `delete_note`
Remove a note from the vault.

**Input:**
```go
type DeleteNoteInput struct {
    Path string `json:"path"` // Full path to the note
}
```

**Behavior:**
- Only delete `.md` files (safety guard)
- Validate with `isPathAllowed()`, verify file exists
- Use `os.Remove()` (files only, never directories)

### 4. `search_content`
Full-text search across note bodies (current `search_notes` only matches filenames).

**Input:**
```go
type SearchContentInput struct {
    Query string `json:"query"` // Search query or regex
}
```

**Output includes:** path, matching line number, snippet (the matching line)

**Behavior:**
- Walk vault, read each `.md` file, search line-by-line
- Case-insensitive regex (fallback to literal match, same pattern as `search_notes`)
- Respect `maxSearchResults` limit
- Skip hidden files via `isPathAllowed()`

### 5. `get_backlinks`
Find all notes that link to a given note via `[[wikilinks]]`.

**Input:**
```go
type GetBacklinksInput struct {
    NoteName string `json:"note_name"` // Note name (without .md or path)
}
```

**Output includes:** source file path, line containing the wikilink

**Behavior:**
- Walk all `.md` files, scan for `[[...]]` patterns
- Regex: `\[\[([^\]|]+)(?:\|[^\]]+)?\]\]` (handles `[[note]]` and `[[note|alias]]`)
- Case-insensitive match against `NoteName` (with and without `.md`)
- Exclude the target note itself from results

### 6. `list_tags`
List all tags found across the vault.

**Input:**
```go
type ListTagsInput struct {
    Prefix string `json:"prefix,omitempty"` // Optional filter prefix
}
```

**Output:** sorted, deduplicated tag list + count

**Behavior:**
- Parse YAML frontmatter `tags:` (both `- tag` block and `[tag1, tag2]` inline formats)
- Parse inline `#tag` occurrences (skip code blocks)
- Filter by prefix if provided
- Simple string parsing (no YAML library dependency)

## Entire CLI Integration

[Entire](https://github.com/entireio/cli) is a developer platform CLI that hooks into git workflows to capture AI agent sessions on every push, linking code changes to the agent transcripts that produced them.

### Current State
- `entire enable` has already been run — `.entire/` directory exists with `settings.json` and `.gitignore`
- `.entire/settings.json` is team-shared config (should be committed)
- `.entire/.gitignore` excludes local-only files (`tmp/`, `settings.local.json`, `metadata/`, `logs/`)
- The repo's root `.gitignore` does **not** yet track `.entire/` properly — the entire `.entire/` directory shows as untracked

### What Needs to Be Done

1. **Commit `.entire/` config files to git**
   - `git add .entire/settings.json .entire/.gitignore` — these are team-shared, meant to be committed
   - `.entire/.gitignore` already excludes local files (`logs/`, `tmp/`, `settings.local.json`, `metadata/`)

2. **Verify hooks are active**
   - Run `entire status` to confirm the session capture is working
   - Strategy is `manual-commit` — checkpoints are created on each `git commit`

### Entire CLI Configuration (already in place)

```json
// .entire/settings.json (team-shared, committed)
{
  "strategy": "manual-commit",
  "enabled": true,
  "telemetry": true
}
```

```gitignore
# .entire/.gitignore (excludes local-only files)
tmp/
settings.local.json
metadata/
logs/
```

### Entire CLI Commands Reference

| Command | Purpose |
|---------|---------|
| `entire enable` | Install git and agent hooks (already done) |
| `entire disable` | Remove hooks (preserves history) |
| `entire status` | Show current session info and config |
| `entire rewind` | Restore code to a previous checkpoint |
| `entire resume <branch>` | Checkout branch and restore session metadata |
| `entire doctor` | Diagnose and repair stuck sessions |

## Files to Modify

| File | Changes |
|------|---------|
| `main.go` | Add 6 input/output structs, 6 handler functions, `slugify()` + `generateFrontmatter()` helpers, 6 `mcp.AddTool()` registrations, bump version to `3.0.0`, add `"bufio"`, `"sort"`, `"time"`, and `"unicode"` imports |
| `main_test.go` | Add tests for all 6 tools + helpers, extend `setupTestVault()` with Zettelkasten-style fixtures |
| `.entire/settings.json` | Already exists — commit to git |
| `.entire/.gitignore` | Already exists — commit to git |

No new external Go dependencies.

## Implementation Order

0. **Entire CLI setup** — commit `.entire/settings.json` and `.entire/.gitignore` to git, verify `entire status`
1. **Helpers** — `slugify()`, `generateFrontmatter()` (+ tests)
2. **`create_note`** — foundation for testing other tools (+ tests)
3. **`update_note`** — simple file I/O with two modes (+ tests)
4. **`delete_note`** — simplest handler (+ tests)
5. **`search_content`** — mirrors existing `search_notes` pattern (+ tests)
6. **`get_backlinks`** — wikilink parsing (+ tests)
7. **`list_tags`** — frontmatter + inline tag parsing (+ tests)
8. **Version bump** — `2.0.0` → `3.0.0`

## Verification

1. `entire status` — Entire CLI hooks active and capturing sessions
2. `go build ./...` — compiles without errors
3. `go test ./... -race -timeout 30s` — all new tests pass
4. `make lint` — no linting issues
5. Manual test via MCP Inspector or Claude Code with the actual vault
