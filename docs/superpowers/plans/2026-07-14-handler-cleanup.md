# Handler Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the duplicated vault-walk boilerplate from `internal/handler`, modernize to `filepath.WalkDir`, add a missing ctx check to `ReadNotes`, and add tests for `internal/server`, with zero behavior change.

**Architecture:** All changes are internal to `internal/handler` plus one new test file in `internal/server`. A private `walkMarkdownFiles` method on `Handler` absorbs the walk/filter/permission logic shared by four tool handlers; a package-level `textResult` helper absorbs the repeated result construction. Existing tests are the regression net and must pass unmodified.

**Tech Stack:** Go 1.26.2, `github.com/modelcontextprotocol/go-sdk` v1.6.1 (already a dependency), standard library only.

**Spec:** `docs/superpowers/specs/2026-07-14-handler-cleanup-design.md`

## Global Constraints

- No new dependencies. `go.mod` must not change.
- Zero behavior change: tool inputs, outputs, and error message strings stay byte-identical. Existing tests in `internal/handler/handler_test.go`, `internal/vault/vault_test.go`, and `internal/note/util_test.go` must pass WITHOUT modification (Task 4 adds one new test; it changes no existing ones).
- All work happens on branch `refactor/handler-walk-dedup`, created in Task 1. Never commit to `main`.
- Commit messages follow Conventional Commits 1.0.0. No em-dashes anywhere (code comments, commit messages, docs). Every commit message ends with the two footer lines shown in each commit step.
- Run tests from the repo root: `/Users/naokisega/src/github.com/nsega/mcp-obsidian`.
- Merge gate (Task 6): `make check` locally, `go fix -diff ./...` empty, and both GitHub Actions jobs ("Build and Test", "Lint") green on the PR.

---

### Task 1: Create branch and add the textResult helper

**Files:**
- Modify: `internal/handler/handler.go` (add helper after `New`, replace 8 result-construction blocks)

**Interfaces:**
- Consumes: nothing new.
- Produces: `func textResult(text string) *mcp.CallToolResult` (package-level, unexported, in package `handler`). Tasks 2 and 3 use it in rewritten handlers.

- [ ] **Step 1: Create the feature branch**

```bash
git checkout -b refactor/handler-walk-dedup main
```

- [ ] **Step 2: Add the textResult helper**

In `internal/handler/handler.go`, immediately after the `New` function (after line 32), add:

```go
// textResult wraps a plain text message in a CallToolResult.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}
```

- [ ] **Step 3: Replace all 8 result-construction blocks**

Each handler currently ends with a block of this shape (field layouts vary slightly; there are 8 total, one per return in `SearchNotes`, `ReadNotes`, `CreateNote`, `UpdateNote`, `DeleteNote`, `SearchContent`, `GetBacklinks`, `ListTags`):

```go
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: textContent,
			},
		},
	}

	return result, output, nil
```

Replace each with a direct call, preserving the exact same text argument each block used:

```go
	return textResult(textContent), output, nil
```

The 8 replacements and their exact text arguments:

| Handler | Text argument |
|---|---|
| `SearchNotes` | `textContent` |
| `ReadNotes` | `strings.Join(textParts, "\n\n")` |
| `CreateNote` | `fmt.Sprintf("Created note: %s", fullPath)` |
| `UpdateNote` | `fmt.Sprintf("Updated note (%s): %s", mode, input.Path)` |
| `DeleteNote` | `fmt.Sprintf("Deleted note: %s", input.Path)` |
| `SearchContent` | `textContent` |
| `GetBacklinks` | `textContent` |
| `ListTags` | `textContent` |

- [ ] **Step 4: Run the full test suite to verify no behavior change**

Run: `go test -race ./...`
Expected: all packages `ok`, zero failures, no test file edited.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/handler.go
git commit -m "$(cat <<'EOF'
refactor(handler): extract textResult helper

Replace eight identical CallToolResult construction blocks with a
single helper. No behavior change.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_014tRnTw2hxp7uvJjEXZBKwe
EOF
)"
```

---

### Task 2: Add walkMarkdownFiles helper and migrate SearchNotes

**Files:**
- Modify: `internal/handler/handler.go` (add helper after `textResult`, rewrite `SearchNotes`, add `io/fs` import)

**Interfaces:**
- Consumes: `textResult` from Task 1.
- Produces: `func (h *Handler) walkMarkdownFiles(ctx context.Context, fn func(path string) error) error`. Task 3 migrates three more handlers onto it. The callback receives only allowed, non-hidden `.md` file paths; errors returned by `fn` (including `filepath.SkipAll`) propagate to the walk.

- [ ] **Step 1: Add the walkMarkdownFiles helper**

In `internal/handler/handler.go`, add `"io/fs"` to the import block (between `"io"` and `"log/slog"`), then add after `textResult`:

```go
// walkMarkdownFiles walks the vault and invokes fn for every allowed markdown
// file. It owns context cancellation, walk-error skipping, hidden-directory
// pruning, the .md filter, and per-file permission checks, so tool handlers
// only implement their per-file logic. Errors returned by fn, including
// filepath.SkipAll for result limits, propagate to the walk.
func (h *Handler) walkMarkdownFiles(ctx context.Context, fn func(path string) error) error {
	return filepath.WalkDir(h.vault.Path, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			h.logger.Debug("skipping path", "path", path, "error", err)
			return nil
		}

		if d.IsDir() {
			// Check if directory is allowed (not hidden)
			allowed, checkErr := h.vault.IsPathAllowed(path)
			if checkErr != nil || !allowed {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process markdown files
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		// Check if file is allowed
		allowed, checkErr := h.vault.IsPathAllowed(path)
		if checkErr != nil || !allowed {
			return nil
		}

		return fn(path)
	})
}
```

- [ ] **Step 2: Rewrite SearchNotes on top of the helper**

Replace the entire `SearchNotes` method body so the method reads:

```go
// SearchNotes implements the search_notes tool
func (h *Handler) SearchNotes(ctx context.Context, req *mcp.CallToolRequest, input note.SearchNotesInput) (*mcp.CallToolResult, note.SearchNotesOutput, error) {
	var results []string

	// Use case-insensitive literal matching via regexp.QuoteMeta to prevent
	// regex injection from user-provided queries
	re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(input.Query))

	err := h.walkMarkdownFiles(ctx, func(path string) error {
		if re.MatchString(filepath.Base(path)) {
			results = append(results, path)
			// Limit results
			if len(results) >= vault.MaxSearchResults {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return nil, note.SearchNotesOutput{}, fmt.Errorf("failed to search notes: %w", err)
	}

	output := note.SearchNotesOutput{
		Results: results,
	}

	textContent := fmt.Sprintf("Found %d matching notes", len(results))
	if len(results) > 0 {
		textContent += ":\n" + strings.Join(results, "\n")
	}

	return textResult(textContent), output, nil
}
```

Note: the `query := input.Query` local and the `os.FileInfo` walk callback disappear; behavior is identical.

- [ ] **Step 3: Run the search tests, then the full suite**

Run: `go test -race -run 'TestSearchNotes' ./internal/handler/ -v`
Expected: `TestSearchNotesHandler` and `TestSearchNotesMaxResults` PASS (max-results proves `SkipAll` still propagates).

Run: `go test -race ./...`
Expected: all packages `ok`.

- [ ] **Step 4: Commit**

```bash
git add internal/handler/handler.go
git commit -m "$(cat <<'EOF'
refactor(handler): add walkMarkdownFiles helper, migrate SearchNotes

Extract the shared vault-walk, hidden-path pruning, .md filtering, and
permission checks into one helper built on filepath.WalkDir. SearchNotes
is the first caller. No behavior change.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_014tRnTw2hxp7uvJjEXZBKwe
EOF
)"
```

---

### Task 3: Migrate SearchContent, GetBacklinks, and ListTags

**Files:**
- Modify: `internal/handler/handler.go` (rewrite three methods)

**Interfaces:**
- Consumes: `walkMarkdownFiles` and `textResult` exactly as defined in Tasks 1 and 2.
- Produces: nothing new; after this task no handler calls `filepath.Walk` and the `os.FileInfo`-style callbacks are gone.

- [ ] **Step 1: Rewrite SearchContent**

```go
// SearchContent implements the search_content tool
func (h *Handler) SearchContent(ctx context.Context, req *mcp.CallToolRequest, input note.SearchContentInput) (*mcp.CallToolResult, note.SearchContentOutput, error) {
	// Use case-insensitive literal matching via regexp.QuoteMeta to prevent
	// regex injection from user-provided queries
	re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(input.Query))

	var results []note.ContentMatch

	err := h.walkMarkdownFiles(ctx, func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			h.logger.Debug("skipping unreadable file", "path", path, "error", err)
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for lineNum, line := range lines {
			if re.MatchString(line) {
				results = append(results, note.ContentMatch{
					Path:    path,
					Snippet: line,
					Line:    lineNum + 1,
				})
				if len(results) >= vault.MaxSearchResults {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, note.SearchContentOutput{}, fmt.Errorf("failed to search content: %w", err)
	}

	output := note.SearchContentOutput{Results: results}

	textContent := fmt.Sprintf("Found %d matching lines", len(results))
	if len(results) > 0 {
		var lines []string
		for _, m := range results {
			lines = append(lines, fmt.Sprintf("%s:%d: %s", m.Path, m.Line, m.Snippet))
		}
		textContent += ":\n" + strings.Join(lines, "\n")
	}

	return textResult(textContent), output, nil
}
```

- [ ] **Step 2: Rewrite GetBacklinks**

```go
// GetBacklinks implements the get_backlinks tool
func (h *Handler) GetBacklinks(ctx context.Context, req *mcp.CallToolRequest, input note.GetBacklinksInput) (*mcp.CallToolResult, note.GetBacklinksOutput, error) {
	if input.NoteName == "" {
		return nil, note.GetBacklinksOutput{}, fmt.Errorf("note_name is required")
	}

	noteName := strings.TrimSuffix(input.NoteName, ".md")
	wikilinkRe := regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

	var results []note.Backlink

	err := h.walkMarkdownFiles(ctx, func(path string) error {
		// Exclude the target note itself
		baseName := strings.TrimSuffix(filepath.Base(path), ".md")
		if strings.EqualFold(baseName, noteName) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			h.logger.Debug("skipping unreadable file", "path", path, "error", err)
			return nil
		}

		lines := strings.SplitSeq(string(data), "\n")
		for line := range lines {
			matches := wikilinkRe.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					linkTarget := strings.TrimSuffix(match[1], ".md")
					if strings.EqualFold(linkTarget, noteName) {
						results = append(results, note.Backlink{
							Path: path,
							Line: line,
						})
						break // one match per line is enough
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, note.GetBacklinksOutput{}, fmt.Errorf("failed to search backlinks: %w", err)
	}

	output := note.GetBacklinksOutput{Results: results}

	textContent := fmt.Sprintf("Found %d backlinks to [[%s]]", len(results), noteName)
	if len(results) > 0 {
		var lines []string
		for _, bl := range results {
			lines = append(lines, fmt.Sprintf("%s: %s", bl.Path, bl.Line))
		}
		textContent += ":\n" + strings.Join(lines, "\n")
	}

	return textResult(textContent), output, nil
}
```

- [ ] **Step 3: Rewrite ListTags**

```go
// ListTags implements the list_tags tool
func (h *Handler) ListTags(ctx context.Context, req *mcp.CallToolRequest, input note.ListTagsInput) (*mcp.CallToolResult, note.ListTagsOutput, error) {
	tagCounts := make(map[string]int)

	// Regex for inline #tags (not inside code blocks)
	inlineTagRe := regexp.MustCompile(`(?:^|\s)#([a-zA-Z][a-zA-Z0-9_/-]*)`)

	err := h.walkMarkdownFiles(ctx, func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			h.logger.Debug("skipping unreadable file", "path", path, "error", err)
			return nil
		}

		content := string(data)

		// Parse frontmatter tags
		if strings.HasPrefix(content, "---\n") {
			endIdx := strings.Index(content[4:], "\n---\n")
			if endIdx >= 0 {
				frontmatter := content[4 : 4+endIdx]
				note.ParseFrontmatterTags(frontmatter, tagCounts)
			}
		}

		// Parse inline #tags (skip code blocks)
		lines := strings.Split(content, "\n")
		inCodeBlock := false
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock {
				continue
			}
			matches := inlineTagRe.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					tagCounts[match[1]]++
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, note.ListTagsOutput{}, fmt.Errorf("failed to list tags: %w", err)
	}

	// Filter by prefix and sort
	var tags []note.TagCount
	for tag, count := range tagCounts {
		if input.Prefix == "" || strings.HasPrefix(tag, input.Prefix) {
			tags = append(tags, note.TagCount{Tag: tag, Count: count})
		}
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Tag < tags[j].Tag
	})

	output := note.ListTagsOutput{Tags: tags}

	textContent := fmt.Sprintf("Found %d unique tags", len(tags))
	if len(tags) > 0 {
		var lines []string
		for _, tc := range tags {
			lines = append(lines, fmt.Sprintf("#%s (%d)", tc.Tag, tc.Count))
		}
		textContent += ":\n" + strings.Join(lines, "\n")
	}

	return textResult(textContent), output, nil
}
```

- [ ] **Step 4: Verify no handler still calls filepath.Walk and imports are clean**

Run: `grep -n 'filepath.Walk(' internal/handler/handler.go`
Expected: no output (only `filepath.WalkDir` remains, inside the helper).

Run: `gofmt -l . && go vet ./...`
Expected: no output from gofmt, vet passes. If the compiler reports an unused import, remove it; `io`, `time`, `sort`, and `os` are all still used (`New`, `CreateNote`, `ListTags`, file reads).

- [ ] **Step 5: Run the affected handler tests, then the full suite**

Run: `go test -race -run 'TestSearchContentHandler|TestGetBacklinksHandler|TestListTagsHandler' ./internal/handler/ -v`
Expected: all three PASS.

Run: `go test -race ./...`
Expected: all packages `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/handler.go
git commit -m "$(cat <<'EOF'
refactor(handler): migrate remaining walk handlers to walkMarkdownFiles

SearchContent, GetBacklinks, and ListTags now share the helper. This
removes the last filepath.Walk usage and the duplicated walk
boilerplate. No behavior change.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_014tRnTw2hxp7uvJjEXZBKwe
EOF
)"
```

---

### Task 4: Add ctx cancellation check to ReadNotes (TDD)

**Files:**
- Modify: `internal/handler/handler_test.go` (new test at end of file, add `"errors"` import)
- Modify: `internal/handler/handler.go` (`ReadNotes` loop)

**Interfaces:**
- Consumes: `newTestHandler(t)` helper already in `handler_test.go` (returns `(*Handler, string)`).
- Produces: `ReadNotes` returns `ctx.Err()` when the context is cancelled, matching the walk-based handlers.

- [ ] **Step 1: Write the failing test**

Add `"errors"` to the import block of `internal/handler/handler_test.go` (between `"context"` and `"fmt"`), then append at the end of the file:

```go
// TestReadNotesContextCancelled verifies ReadNotes aborts on a cancelled context
func TestReadNotesContextCancelled(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := h.ReadNotes(ctx, nil, note.ReadNotesInput{
		Paths: []string{filepath.Join(tmpVault, "note1.md")},
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -race -run TestReadNotesContextCancelled ./internal/handler/ -v`
Expected: FAIL with `expected context.Canceled, got <nil>` (current code reads the file successfully and returns nil error).

- [ ] **Step 3: Add the ctx check to ReadNotes**

In `ReadNotes` in `internal/handler/handler.go`, add a check as the first statement inside the `for _, path := range input.Paths` loop:

```go
	for _, path := range input.Paths {
		if err := ctx.Err(); err != nil {
			return nil, note.ReadNotesOutput{}, err
		}
```

The rest of the loop body is unchanged.

- [ ] **Step 4: Run the test to verify it passes, then the full suite**

Run: `go test -race -run TestReadNotesContextCancelled ./internal/handler/ -v`
Expected: PASS.

Run: `go test -race ./...`
Expected: all packages `ok` (in particular `TestReadNotesHandler` and `TestReadNotesContentFormat` still pass; they use `context.Background()` and are unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/handler/handler.go internal/handler/handler_test.go
git commit -m "$(cat <<'EOF'
fix(handler): honor context cancellation in ReadNotes

The walk-based handlers already abort on a cancelled context; ReadNotes
did not. Check ctx.Err() before each file read.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_014tRnTw2hxp7uvJjEXZBKwe
EOF
)"
```

---

### Task 5: Add tests for internal/server

**Files:**
- Create: `internal/server/server_test.go`

**Interfaces:**
- Consumes: `server.New(h *handler.Handler, version string, logger *slog.Logger) *mcp.Server`, `handler.New(v *vault.Vault, logger *slog.Logger) *Handler`, `testutil.SetupTestVault(t)` / `testutil.CleanupTestVault(t, path)`, and go-sdk v1.6.1 test plumbing: `mcp.NewInMemoryTransports()` (returns two symmetric transports; connect the server side first), `(*mcp.Server).Connect`, `mcp.NewClient`, `(*mcp.Client).Connect`, `(*mcp.ClientSession).ListTools`.
- Produces: characterization coverage for the wiring. This is a test-only task; the test is expected to pass immediately.

- [ ] **Step 1: Write the test file**

Create `internal/server/server_test.go`:

```go
package server_test

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/handler"
	"github.com/nsega/mcp-obsidian/internal/server"
	"github.com/nsega/mcp-obsidian/internal/testutil"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

// TestNewRegistersAllTools connects a client over an in-memory transport and
// verifies the server exposes exactly the expected tool set
func TestNewRegistersAllTools(t *testing.T) {
	tmpVault := testutil.SetupTestVault(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	v, err := vault.New(tmpVault)
	if err != nil {
		t.Fatalf("vault.New failed: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := handler.New(v, logger)
	srv := server.New(h, "test", logger)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect failed: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect failed: %v", err)
	}
	defer clientSession.Close()

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	want := []string{
		"create_note",
		"delete_note",
		"get_backlinks",
		"list_tags",
		"read_notes",
		"search_content",
		"search_notes",
		"update_note",
	}

	var got []string
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("registered tools = %v, want %v", got, want)
	}

	for _, tool := range res.Tools {
		if tool.Description == "" {
			t.Errorf("tool %q has an empty description", tool.Name)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

Run: `go test -race ./internal/server/ -v`
Expected: `TestNewRegistersAllTools` PASS. If `ListTools` hangs or the connect order matters differently in this SDK version, connect the server before the client (as written) and consult `go doc github.com/modelcontextprotocol/go-sdk/mcp NewInMemoryTransports`.

- [ ] **Step 3: Run the full suite**

Run: `go test -race ./...`
Expected: all packages `ok`; `internal/server` no longer reports `[no test files]`.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server_test.go
git commit -m "$(cat <<'EOF'
test(server): verify tool registration over in-memory transport

Cover the previously untested server wiring: all eight tools are
registered with non-empty descriptions.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_014tRnTw2hxp7uvJjEXZBKwe
EOF
)"
```

---

### Task 6: Local gates, PR, and CI verification

**Files:**
- None created or modified (verification and delivery only).

**Interfaces:**
- Consumes: the branch `refactor/handler-walk-dedup` with Tasks 1 through 5 committed.
- Produces: a green PR into `main`. Merge only after both CI jobs pass.

- [ ] **Step 1: Run the full local gate**

Run: `make check`
Expected: fmt, vet, golangci-lint, and `go test -race` all pass, ending with `All checks passed!`. If golangci-lint is not installed locally, the Makefile falls back to fmt and vet; CI still runs the real linter, so proceed but do not skip Step 4.

- [ ] **Step 2: Run the modernization check CI enforces**

Run: `OUTPUT=$(go fix -diff ./... 2>&1); if [ -n "$OUTPUT" ]; then echo "$OUTPUT"; else echo "clean"; fi`
Expected: `clean`. If a diff prints, run `go fix ./...`, re-run `go test -race ./...`, and amend the relevant commit.

- [ ] **Step 3: Push the branch and open the PR**

```bash
git push -u origin refactor/handler-walk-dedup
gh pr create --base main --title "refactor(handler): dedupe vault-walk boilerplate" --body "$(cat <<'EOF'
## Summary
- Extract shared walkMarkdownFiles helper (filepath.WalkDir) used by search_notes, search_content, get_backlinks, and list_tags
- Extract textResult helper for CallToolResult construction
- Honor context cancellation in read_notes
- Add internal/server tests: tool registration verified over an in-memory transport

## Regression proof
- Zero behavior change intended; all pre-existing tests pass unmodified
- Spec: docs/superpowers/specs/2026-07-14-handler-cleanup-design.md

🤖 Generated with [Claude Code](https://claude.com/claude-code)

https://claude.ai/code/session_014tRnTw2hxp7uvJjEXZBKwe
EOF
)"
```

- [ ] **Step 4: Watch CI to completion**

Run: `gh pr checks --watch`
Expected: both jobs green: "Build and Test" (build, `go test -race` with coverage, vet, staticcheck v0.7.0, `go fix -diff`) and "Lint" (golangci-lint v2.10.1). If any check fails, fix on the branch, push, and re-watch. Do NOT merge with a red check.

- [ ] **Step 5: Report and hand back for merge**

Post the PR URL and CI status to the user. Merging into `main` is the user's call.
