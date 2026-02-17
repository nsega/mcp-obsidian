# Refactor mcp-obsidian: Split Monolithic main.go into Internal Packages

**Status: COMPLETED** - All 7 steps implemented, tested, and pushed.

## Context

All server logic (~970 lines) lived in a single `main.go` with a global `vaultPath` variable. This was decomposed into well-structured `internal/` packages following Go best practices, preserving all existing behavior and test coverage.

## Target Package Structure

```
main.go                          (~50 lines)  CLI args, wire-up, run
internal/
├── testutil/
│   └── testutil.go              (~50 lines)  Shared test vault setup/cleanup
├── note/
│   ├── types.go                 (~120 lines) All Input/Output/model structs
│   ├── util.go                  (~80 lines)  Slugify, GenerateFrontmatter, ParseFrontmatterTags
│   └── util_test.go             (~100 lines) Tests for util functions
├── vault/
│   ├── vault.go                 (~120 lines) Vault struct, New(), IsPathAllowed, resolveWithAncestors
│   └── vault_test.go            (~300 lines) Path validation tests
├── handler/
│   ├── handler.go               (~450 lines) Handler struct + 8 handler methods
│   └── handler_test.go          (~700 lines) All handler tests
└── server/
    └── server.go                (~60 lines)  NewServer(), tool registration
```

## Implementation Steps

### Step 1: Create `internal/testutil/testutil.go`

Extract from `main_test.go`:
- `SetupTestVault(t *testing.T) string` (export)
- `CleanupTestVault(t *testing.T, path string)` (export)

### Step 2: Create `internal/note/types.go`

Move all 16 structs from `main.go:27-135`:
- `SearchNotesInput`, `SearchNotesOutput`
- `ReadNotesInput`, `NoteContent`, `ReadNotesOutput`
- `CreateNoteInput`, `CreateNoteOutput`
- `UpdateNoteInput`, `UpdateNoteOutput`
- `DeleteNoteInput`, `DeleteNoteOutput`
- `SearchContentInput`, `ContentMatch`, `SearchContentOutput`
- `GetBacklinksInput`, `Backlink`, `GetBacklinksOutput`
- `ListTagsInput`, `TagCount`, `ListTagsOutput`

### Step 3: Create `internal/note/util.go` + `internal/note/util_test.go`

Move and export from `main.go`:
- `slugify` → `Slugify` (lines 138-152)
- `generateFrontmatter` → `GenerateFrontmatter` (lines 155-174)
- `parseFrontmatterTags` → `ParseFrontmatterTags` (lines 841-882)

Move tests from `main_test.go`:
- `TestSlugify`, `TestGenerateFrontmatter`, `TestParseFrontmatterTags`

### Step 4: Create `internal/vault/vault.go` + `internal/vault/vault_test.go`

**vault.go** — eliminate global state:
```go
type Vault struct {
    Path string // resolved absolute path
}

func New(rawPath string) (*Vault, error)              // expand ~, resolve abs, stat
func (v *Vault) IsPathAllowed(path string) (bool, error)
func (v *Vault) resolveWithAncestors(absPath string) string  // unexported helper
```

Also move `MaxSearchResults = 200` constant here.

Move from `main.go`: lines 176-268 (path security logic), tilde expansion from `main()` lines 894-905.

Move tests from `main_test.go`:
- `TestIsPathAllowed` (5 test functions, lines 68-401)
- Tests create `&vault.Vault{Path: tmpDir}` instead of setting global `vaultPath`

### Step 5: Create `internal/handler/handler.go` + `internal/handler/handler_test.go`

**handler.go**:
```go
type Handler struct {
    Vault *vault.Vault
}

func New(v *vault.Vault) *Handler
```

Convert 8 package-level handler functions to methods on `*Handler`:
- `searchNotesHandler` → `(h *Handler) SearchNotes`
- `readNotesHandler` → `(h *Handler) ReadNotes`
- `createNoteHandler` → `(h *Handler) CreateNote`
- `updateNoteHandler` → `(h *Handler) UpdateNote`
- `deleteNoteHandler` → `(h *Handler) DeleteNote`
- `searchContentHandler` → `(h *Handler) SearchContent`
- `getBacklinksHandler` → `(h *Handler) GetBacklinks`
- `listTagsHandler` → `(h *Handler) ListTags`

Replace `vaultPath` references with `h.Vault.Path`, and `isPathAllowed(p)` with `h.Vault.IsPathAllowed(p)`.

Move all handler tests from `main_test.go`. Tests construct handlers via:
```go
v := &vault.Vault{Path: tmpDir}
h := handler.New(v)
```

### Step 6: Create `internal/server/server.go`

Extract tool registration from `main()` lines 907-959:
```go
func New(h *handler.Handler, version string) *mcp.Server
```

Registers all 8 tools with descriptions and `h.SearchNotes`, `h.ReadNotes`, etc.

This works because `mcp.ToolHandlerFor[In, Out]` is `func(context.Context, *CallToolRequest, In) (*CallToolResult, Out, error)`, and Go method values like `h.SearchNotes` match this signature.

### Step 7: Rewrite `main.go` + delete `main_test.go`

Thin `main.go` (~50 lines): parse CLI args, call `vault.New()`, `handler.New()`, `server.New()`, run. Keep `Version` and `BuildTime` vars for ldflags injection (no Makefile changes needed).

Delete the old `main_test.go` — all tests now live under `internal/`.

## Dependency Graph (no cycles)

```
main → server → handler → vault
                        → note
                  note (pure types/utils, no dependencies)
                  testutil (test-only, used by vault_test + handler_test)
```

## Verification Results

1. `go build ./...` — PASS
2. `go test -v -race ./...` — PASS (all tests across note, vault, handler packages)
3. `make lint` — SKIPPED (pre-existing golangci-lint version mismatch with Go 1.25)
4. `make build` — PASS (ldflags inject Version/BuildTime correctly)
5. Manual MCP protocol test — pending (can verify via `make run`)
