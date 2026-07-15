# Design: Focused cleanup of internal/handler duplication

Date: 2026-07-14
Status: Approved (pending spec review)

## Background

The codebase is small and healthy (~1,150 lines of non-test Go, full CI, zero
TODO markers). A review found one real source of technical debt: four MCP tool
handlers in `internal/handler/handler.go` repeat the same ~30-line vault-walk
boilerplate. This design removes that duplication plus three small adjacent
gaps. It is a one-pass cleanup, not a staged refactor program.

## Goals

- Remove the duplicated walk/filter/permission boilerplate in the handler package.
- Modernize the directory walk from `filepath.Walk` to `filepath.WalkDir`.
- Close two small gaps: missing `ctx` cancellation check in `ReadNotes`,
  and missing tests for `internal/server`.
- Zero behavior change to tool inputs, outputs, or error messages.

## Non-goals

- No splitting of `handler.go` into per-tool files.
- No changes to the `internal/vault` public API.
- No new dependencies.
- No changes to tool schemas or the MCP wiring in `internal/server`.

## Changes

### 1. Shared walk helper (main change)

Add a private method on `Handler` in `internal/handler/handler.go`:

```go
func (h *Handler) walkMarkdownFiles(ctx context.Context, fn func(path string) error) error
```

The helper owns, in this order per entry:

1. `ctx.Err()` check; return the context error to abort the walk.
2. Walk-error handling: log at Debug via `h.logger` and skip the entry
   (matching current behavior).
3. Directories: `vault.IsPathAllowed` check; return `filepath.SkipDir` for
   disallowed (hidden) directories.
4. Files: filter with a case-insensitive `.md` suffix check, then
   `vault.IsPathAllowed`; skip disallowed files.
5. Invoke `fn(path)` for each allowed markdown file. The callback's returned
   error propagates, so `filepath.SkipAll` continues to work for result
   limits (`vault.MaxSearchResults`).

Implementation uses `filepath.WalkDir` (one less stat per file than
`filepath.Walk`). No handler currently uses `info os.FileInfo` from the walk
callback for anything except `IsDir()`, so `fs.DirEntry` is sufficient.

Callers rewritten to use the helper: `SearchNotes`, `SearchContent`,
`GetBacklinks`, `ListTags`. Each keeps only its tool-specific logic
(matching, reading file content where needed, accumulating results) inside
the callback.

### 2. Result-construction helper

Add a package-level helper:

```go
func textResult(text string) *mcp.CallToolResult
```

It replaces the eight identical
`&mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: ...}}}`
blocks across all handlers.

### 3. ctx check in ReadNotes

At the top of the per-path loop in `ReadNotes`, check `ctx.Err()` and return
the context error, matching the cancellation behavior the walk-based handlers
already have.

### 4. Tests for internal/server

New `internal/server/server_test.go` with minimal coverage of the wiring:
the server constructs without error and the expected tools are registered.
Kept small because the package is 68 lines of glue.

## Error handling

Unchanged by design. The helper reproduces the current per-entry behavior
exactly: unreadable paths are logged at Debug and skipped, disallowed
directories are pruned with `SkipDir`, and walk termination via `SkipAll`
or context error is propagated to the caller as today.

## Regression verification

Behavior must not drift. The proof is that existing tests pass unchanged,
verified at two levels:

1. **Local**: `make check` (fmt, vet, golangci-lint, `go test -race`) must
   pass before commit. Existing tests in `handler_test.go`, `vault_test.go`,
   and `util_test.go` must pass without modification. Additionally run
   `go fix -diff ./...` and confirm empty output, since CI enforces it.
2. **CI**: work lands on a feature branch pushed to GitHub with a PR into
   `main`, so both GitHub Actions jobs run: "Build and Test"
   (build, `go test -race` with coverage, vet, staticcheck v0.7.0,
   `go fix -diff` modernization check) and "Lint" (golangci-lint v2.10.1).
   Merge only when both jobs are green.

## Expected outcome

Roughly 150 lines of duplication removed from `handler.go` (about 630 to
about 450 lines), one new helper method, one small helper function, one new
test file. All existing tests pass unchanged; CI green on the PR.
