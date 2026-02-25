package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/note"
	"github.com/nsega/mcp-obsidian/internal/testutil"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

func newTestHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	tmpVault := testutil.SetupTestVault(t)
	v := &vault.Vault{Path: tmpVault}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(v, logger), tmpVault
}

// TestSearchNotesHandler tests the search_notes tool
func TestSearchNotesHandler(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	tests := []struct {
		name            string
		input           note.SearchNotesInput
		wantContains    []string
		wantNotContains []string
		wantCount       int
		wantErr         bool
	}{
		{
			name: "search for 'note' should find multiple files",
			input: note.SearchNotesInput{
				Query: "note",
			},
			wantCount: 3, // note1.md, note2.md, nested-note.md (not .hidden-note.md)
			wantContains: []string{
				"note1.md",
				"note2.md",
				"nested-note.md",
			},
			wantNotContains: []string{
				".hidden-note.md",
			},
		},
		{
			name: "search for 'project' should find project files",
			input: note.SearchNotesInput{
				Query: "project",
			},
			wantCount: 2, // project-plan.md, project-update.md
			wantContains: []string{
				"project-plan.md",
				"project-update.md",
			},
		},
		{
			name: "regex metacharacters are treated as literals",
			input: note.SearchNotesInput{
				Query: "^note",
			},
			wantCount: 0, // "^note" is not in any filename literally
		},
		{
			name: "case insensitive search",
			input: note.SearchNotesInput{
				Query: "JOURNAL",
			},
			wantCount: 1, // daily-journal.md
			wantContains: []string{
				"daily-journal.md",
			},
		},
		{
			name: "search with no matches",
			input: note.SearchNotesInput{
				Query: "nonexistent",
			},
			wantCount: 0,
		},
		{
			name: "search should exclude hidden files",
			input: note.SearchNotesInput{
				Query: "secret",
			},
			wantCount: 0, // secret.md is in .hidden directory
		},
		{
			name: "search should only return .md files",
			input: note.SearchNotesInput{
				Query: "txt",
			},
			wantCount: 0, // not-markdown.txt should not be found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req := &mcp.CallToolRequest{}

			result, output, err := h.SearchNotes(ctx, req, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SearchNotes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			// Check result count
			if len(output.Results) != tt.wantCount {
				t.Errorf("SearchNotes() got %d results, want %d. Results: %v",
					len(output.Results), tt.wantCount, output.Results)
			}

			// Check that expected files are present
			for _, expected := range tt.wantContains {
				found := false
				for _, r := range output.Results {
					if strings.Contains(r, expected) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("SearchNotes() results missing expected file: %s", expected)
				}
			}

			// Check that unexpected files are not present
			for _, notExpected := range tt.wantNotContains {
				for _, r := range output.Results {
					if strings.Contains(r, notExpected) {
						t.Errorf("SearchNotes() results contain unexpected file: %s", notExpected)
					}
				}
			}

			// Check that result content is properly formatted
			if result == nil {
				t.Error("SearchNotes() result is nil")
				return
			}

			if len(result.Content) == 0 {
				t.Error("SearchNotes() result content is empty")
			}
		})
	}
}

// TestSearchNotesMaxResults tests that search respects the max results limit
func TestSearchNotesMaxResults(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	// Create directory for test files
	manyDir := filepath.Join(tmpVault, "many")
	if err := os.MkdirAll(manyDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create many test files to exceed MaxSearchResults
	for i := range vault.MaxSearchResults + 50 {
		path := filepath.Join(manyDir, fmt.Sprintf("test-note-%d.md", i))
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := note.SearchNotesInput{Query: "test"}

	_, output, err := h.SearchNotes(ctx, req, input)
	if err != nil {
		t.Fatalf("SearchNotes() error = %v", err)
	}

	// Should not exceed MaxSearchResults
	if len(output.Results) > vault.MaxSearchResults {
		t.Errorf("SearchNotes() returned %d results, should not exceed %d",
			len(output.Results), vault.MaxSearchResults)
	}
}

// TestReadNotesHandler tests the read_notes tool
func TestReadNotesHandler(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	tests := []struct {
		validate  func(t *testing.T, output note.ReadNotesOutput)
		name      string
		input     note.ReadNotesInput
		wantCount int
	}{
		{
			name: "read single file successfully",
			input: note.ReadNotesInput{
				Paths: []string{filepath.Join(tmpVault, "note1.md")},
			},
			wantCount: 1,
			validate: func(t *testing.T, output note.ReadNotesOutput) {
				if len(output.Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(output.Notes))
				}
				n := output.Notes[0]
				if n.Error != "" {
					t.Errorf("Expected no error, got: %s", n.Error)
				}
				if !strings.Contains(n.Content, "# Note 1") {
					t.Errorf("Expected content to contain '# Note 1', got: %s", n.Content)
				}
			},
		},
		{
			name: "read multiple files successfully",
			input: note.ReadNotesInput{
				Paths: []string{
					filepath.Join(tmpVault, "note1.md"),
					filepath.Join(tmpVault, "note2.md"),
					filepath.Join(tmpVault, "subfolder", "nested-note.md"),
				},
			},
			wantCount: 3,
			validate: func(t *testing.T, output note.ReadNotesOutput) {
				if len(output.Notes) != 3 {
					t.Fatalf("Expected 3 notes, got %d", len(output.Notes))
				}
				for i, n := range output.Notes {
					if n.Error != "" {
						t.Errorf("Note %d: Expected no error, got: %s", i, n.Error)
					}
					if n.Content == "" {
						t.Errorf("Note %d: Expected content, got empty string", i)
					}
				}
			},
		},
		{
			name: "read non-existent file should return error",
			input: note.ReadNotesInput{
				Paths: []string{filepath.Join(tmpVault, "nonexistent.md")},
			},
			wantCount: 1,
			validate: func(t *testing.T, output note.ReadNotesOutput) {
				if len(output.Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(output.Notes))
				}
				n := output.Notes[0]
				if n.Error == "" {
					t.Error("Expected error for non-existent file, got none")
				}
				if !strings.Contains(n.Error, "Failed to read file") {
					t.Errorf("Expected 'Failed to read file' error, got: %s", n.Error)
				}
			},
		},
		{
			name: "read file outside vault should return error",
			input: note.ReadNotesInput{
				Paths: []string{"/tmp/outside-vault.md"},
			},
			wantCount: 1,
			validate: func(t *testing.T, output note.ReadNotesOutput) {
				if len(output.Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(output.Notes))
				}
				n := output.Notes[0]
				if n.Error == "" {
					t.Error("Expected error for file outside vault, got none")
				}
				if !strings.Contains(n.Error, "Access denied") {
					t.Errorf("Expected 'Access denied' error, got: %s", n.Error)
				}
			},
		},
		{
			name: "read hidden file should return error",
			input: note.ReadNotesInput{
				Paths: []string{filepath.Join(tmpVault, ".hidden", "secret.md")},
			},
			wantCount: 1,
			validate: func(t *testing.T, output note.ReadNotesOutput) {
				if len(output.Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(output.Notes))
				}
				n := output.Notes[0]
				if n.Error == "" {
					t.Error("Expected error for hidden file, got none")
				}
				if !strings.Contains(n.Error, "Access denied") {
					t.Errorf("Expected 'Access denied' error, got: %s", n.Error)
				}
			},
		},
		{
			name: "read mixed valid and invalid files",
			input: note.ReadNotesInput{
				Paths: []string{
					filepath.Join(tmpVault, "note1.md"),             // valid
					filepath.Join(tmpVault, "nonexistent.md"),       // invalid - doesn't exist
					filepath.Join(tmpVault, ".hidden", "secret.md"), // invalid - hidden
				},
			},
			wantCount: 3,
			validate: func(t *testing.T, output note.ReadNotesOutput) {
				if len(output.Notes) != 3 {
					t.Fatalf("Expected 3 notes, got %d", len(output.Notes))
				}

				// First note should be successful
				if output.Notes[0].Error != "" {
					t.Errorf("Note 0: Expected no error, got: %s", output.Notes[0].Error)
				}
				if !strings.Contains(output.Notes[0].Content, "# Note 1") {
					t.Errorf("Note 0: Expected content, got: %s", output.Notes[0].Content)
				}

				// Second note should have "Failed to read file" error
				if output.Notes[1].Error == "" {
					t.Error("Note 1: Expected error, got none")
				}

				// Third note should have "Access denied" error
				if output.Notes[2].Error == "" {
					t.Error("Note 2: Expected error, got none")
				}
				if !strings.Contains(output.Notes[2].Error, "Access denied") {
					t.Errorf("Note 2: Expected 'Access denied' error, got: %s", output.Notes[2].Error)
				}
			},
		},
		{
			name: "read empty path list",
			input: note.ReadNotesInput{
				Paths: []string{},
			},
			wantCount: 0,
			validate: func(t *testing.T, output note.ReadNotesOutput) {
				if len(output.Notes) != 0 {
					t.Fatalf("Expected 0 notes, got %d", len(output.Notes))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req := &mcp.CallToolRequest{}

			result, output, err := h.ReadNotes(ctx, req, tt.input)
			if err != nil {
				t.Fatalf("ReadNotes() error = %v", err)
			}

			// Check result is not nil
			if result == nil {
				t.Fatal("ReadNotes() result is nil")
			}

			// Check result has content
			if len(result.Content) == 0 {
				t.Error("ReadNotes() result content is empty")
			}

			// Run custom validation
			if tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

// TestReadNotesContentFormat tests that the text content is properly formatted
func TestReadNotesContentFormat(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := note.ReadNotesInput{
		Paths: []string{
			filepath.Join(tmpVault, "note1.md"),
			filepath.Join(tmpVault, "note2.md"),
		},
	}

	result, _, err := h.ReadNotes(ctx, req, input)
	if err != nil {
		t.Fatalf("ReadNotes() error = %v", err)
	}

	// Check that result has text content
	if len(result.Content) == 0 {
		t.Fatal("Expected result to have content")
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("Expected first content to be TextContent")
	}

	// Check that text contains proper markdown headers
	if !strings.Contains(textContent.Text, "##") {
		t.Error("Expected text content to contain markdown headers (##)")
	}

	// Check that both file paths are mentioned
	if !strings.Contains(textContent.Text, "note1.md") {
		t.Error("Expected text content to mention note1.md")
	}
	if !strings.Contains(textContent.Text, "note2.md") {
		t.Error("Expected text content to mention note2.md")
	}
}

// TestCreateNoteHandler tests the create_note tool
func TestCreateNoteHandler(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	today := time.Now().Format("2006-01-02")
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	t.Run("create note in root", func(t *testing.T) {
		input := note.CreateNoteInput{
			Title:   "My Test Note",
			Content: "Some content here.",
			Tags:    []string{"test", "demo"},
		}
		result, output, err := h.CreateNote(ctx, req, input)
		if err != nil {
			t.Fatalf("CreateNote() error = %v", err)
		}
		if result == nil {
			t.Fatal("result is nil")
		}

		expectedPath := filepath.Join(tmpVault, today+"_my-test-note.md")
		if output.Path != expectedPath {
			t.Errorf("output.Path = %q, want %q", output.Path, expectedPath)
		}

		data, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Fatalf("failed to read created file: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "# My Test Note") {
			t.Error("missing heading in created note")
		}
		if !strings.Contains(content, "- test") {
			t.Error("missing tag 'test' in frontmatter")
		}
		if !strings.Contains(content, "Some content here.") {
			t.Error("missing body content")
		}
	})

	t.Run("create note in subfolder", func(t *testing.T) {
		input := note.CreateNoteInput{
			Title:  "Subfolder Note",
			Folder: "00_Inbox",
		}
		_, output, err := h.CreateNote(ctx, req, input)
		if err != nil {
			t.Fatalf("CreateNote() error = %v", err)
		}

		expectedPath := filepath.Join(tmpVault, "00_Inbox", today+"_subfolder-note.md")
		if output.Path != expectedPath {
			t.Errorf("output.Path = %q, want %q", output.Path, expectedPath)
		}

		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Error("file was not created")
		}
	})

	t.Run("error on duplicate", func(t *testing.T) {
		input := note.CreateNoteInput{Title: "My Test Note"}
		_, _, err := h.CreateNote(ctx, req, input)
		if err == nil {
			t.Error("expected error for duplicate file, got nil")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("expected 'already exists' error, got: %v", err)
		}
	})

	t.Run("error on empty title", func(t *testing.T) {
		input := note.CreateNoteInput{Title: ""}
		_, _, err := h.CreateNote(ctx, req, input)
		if err == nil {
			t.Error("expected error for empty title, got nil")
		}
	})
}

// TestUpdateNoteHandler tests the update_note tool
func TestUpdateNoteHandler(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	t.Run("replace mode", func(t *testing.T) {
		notePath := filepath.Join(tmpVault, "note1.md")
		input := note.UpdateNoteInput{
			Path:    notePath,
			Content: "# Replaced Content\nNew body.",
			Mode:    "replace",
		}
		_, output, err := h.UpdateNote(ctx, req, input)
		if err != nil {
			t.Fatalf("UpdateNote() error = %v", err)
		}
		if output.Path != notePath {
			t.Errorf("output.Path = %q, want %q", output.Path, notePath)
		}

		data, err := os.ReadFile(notePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if string(data) != "# Replaced Content\nNew body." {
			t.Errorf("file content = %q, want replaced content", string(data))
		}
	})

	t.Run("append mode", func(t *testing.T) {
		notePath := filepath.Join(tmpVault, "daily-journal.md")
		input := note.UpdateNoteInput{
			Path:    notePath,
			Content: "Appended text.",
			Mode:    "append",
		}
		_, _, err := h.UpdateNote(ctx, req, input)
		if err != nil {
			t.Fatalf("UpdateNote() error = %v", err)
		}

		data, err := os.ReadFile(notePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "# Daily Journal") {
			t.Error("original content missing after append")
		}
		if !strings.HasSuffix(content, "\n\nAppended text.") {
			t.Errorf("appended content not found at end: %q", content)
		}
	})

	t.Run("default mode is replace", func(t *testing.T) {
		notePath := filepath.Join(tmpVault, "subfolder/project-update.md")
		input := note.UpdateNoteInput{
			Path:    notePath,
			Content: "Default replaced.",
		}
		_, _, err := h.UpdateNote(ctx, req, input)
		if err != nil {
			t.Fatalf("UpdateNote() error = %v", err)
		}

		data, err := os.ReadFile(notePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if string(data) != "Default replaced." {
			t.Errorf("file content = %q, want 'Default replaced.'", string(data))
		}
	})

	t.Run("error on nonexistent file", func(t *testing.T) {
		input := note.UpdateNoteInput{
			Path:    filepath.Join(tmpVault, "nonexistent.md"),
			Content: "test",
		}
		_, _, err := h.UpdateNote(ctx, req, input)
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("error on invalid mode", func(t *testing.T) {
		input := note.UpdateNoteInput{
			Path:    filepath.Join(tmpVault, "daily-journal.md"),
			Content: "test",
			Mode:    "invalid",
		}
		_, _, err := h.UpdateNote(ctx, req, input)
		if err == nil {
			t.Error("expected error for invalid mode")
		}
	})

	t.Run("error on path outside vault", func(t *testing.T) {
		input := note.UpdateNoteInput{
			Path:    "/tmp/outside.md",
			Content: "test",
		}
		_, _, err := h.UpdateNote(ctx, req, input)
		if err == nil {
			t.Error("expected error for path outside vault")
		}
	})
}

// TestDeleteNoteHandler tests the delete_note tool
func TestDeleteNoteHandler(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	t.Run("delete existing note", func(t *testing.T) {
		notePath := filepath.Join(tmpVault, "daily-journal.md")
		_, output, err := h.DeleteNote(ctx, req, note.DeleteNoteInput{Path: notePath})
		if err != nil {
			t.Fatalf("DeleteNote() error = %v", err)
		}
		if output.Path != notePath {
			t.Errorf("output.Path = %q, want %q", output.Path, notePath)
		}
		if _, err := os.Stat(notePath); !os.IsNotExist(err) {
			t.Error("file still exists after delete")
		}
	})

	t.Run("error on non-md file", func(t *testing.T) {
		_, _, err := h.DeleteNote(ctx, req, note.DeleteNoteInput{
			Path: filepath.Join(tmpVault, "not-markdown.txt"),
		})
		if err == nil {
			t.Error("expected error for non-.md file")
		}
		if !strings.Contains(err.Error(), ".md") {
			t.Errorf("expected .md error, got: %v", err)
		}
	})

	t.Run("error on nonexistent file", func(t *testing.T) {
		_, _, err := h.DeleteNote(ctx, req, note.DeleteNoteInput{
			Path: filepath.Join(tmpVault, "nonexistent.md"),
		})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("error on directory", func(t *testing.T) {
		// Create a directory ending in .md to test the directory guard
		dirPath := filepath.Join(tmpVault, "fake-dir.md")
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("failed to create test dir: %v", err)
		}
		_, _, err := h.DeleteNote(ctx, req, note.DeleteNoteInput{Path: dirPath})
		if err == nil {
			t.Error("expected error for directory")
		}
		if !strings.Contains(err.Error(), "directories") {
			t.Errorf("expected 'directories' error, got: %v", err)
		}
	})

	t.Run("error on path outside vault", func(t *testing.T) {
		_, _, err := h.DeleteNote(ctx, req, note.DeleteNoteInput{
			Path: "/tmp/outside.md",
		})
		if err == nil {
			t.Error("expected error for path outside vault")
		}
	})
}

// TestSearchContentHandler tests the search_content tool
func TestSearchContentHandler(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	tests := []struct {
		name         string
		query        string
		wantContains string
		wantMinCount int
	}{
		{
			name:         "search for content in body",
			query:        "first note",
			wantMinCount: 1,
			wantContains: "note1.md",
		},
		{
			name:         "case insensitive search",
			query:        "PROJECT PLAN",
			wantMinCount: 1,
			wantContains: "project-plan.md",
		},
		{
			name:         "regex metacharacters are treated as literals",
			query:        `^# Zettel \d`,
			wantMinCount: 0, // regex metacharacters are escaped, no literal match
		},
		{
			name:         "search across multiple files",
			query:        "subfolder",
			wantMinCount: 2,
		},
		{
			name:         "no matches",
			query:        "xyznonexistent123",
			wantMinCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := h.SearchContent(ctx, req, note.SearchContentInput{Query: tt.query})
			if err != nil {
				t.Fatalf("SearchContent() error = %v", err)
			}
			if result == nil {
				t.Fatal("result is nil")
			}
			if len(output.Results) < tt.wantMinCount {
				t.Errorf("got %d results, want at least %d", len(output.Results), tt.wantMinCount)
			}
			if tt.wantContains != "" {
				found := false
				for _, m := range output.Results {
					if strings.Contains(m.Path, tt.wantContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("results missing expected file containing %q", tt.wantContains)
				}
			}
			// Verify line numbers are positive
			for _, m := range output.Results {
				if m.Line <= 0 {
					t.Errorf("invalid line number %d for %s", m.Line, m.Path)
				}
			}
		})
	}
}

// TestGetBacklinksHandler tests the get_backlinks tool
func TestGetBacklinksHandler(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	t.Run("find backlinks to note1", func(t *testing.T) {
		result, output, err := h.GetBacklinks(ctx, req, note.GetBacklinksInput{NoteName: "note1"})
		if err != nil {
			t.Fatalf("GetBacklinks() error = %v", err)
		}
		if result == nil {
			t.Fatal("result is nil")
		}
		// note2.md has [[note1]], project-plan.md has [[note1|first note]]
		if len(output.Results) != 2 {
			t.Errorf("got %d backlinks, want 2. Results: %+v", len(output.Results), output.Results)
		}
	})

	t.Run("find backlinks to project-plan", func(t *testing.T) {
		_, output, err := h.GetBacklinks(ctx, req, note.GetBacklinksInput{NoteName: "project-plan"})
		if err != nil {
			t.Fatalf("GetBacklinks() error = %v", err)
		}
		// nested-note.md has [[project-plan]]
		if len(output.Results) != 1 {
			t.Errorf("got %d backlinks, want 1. Results: %+v", len(output.Results), output.Results)
		}
	})

	t.Run("find backlinks to zettel1", func(t *testing.T) {
		_, output, err := h.GetBacklinks(ctx, req, note.GetBacklinksInput{NoteName: "zettel1"})
		if err != nil {
			t.Fatalf("GetBacklinks() error = %v", err)
		}
		// zettel2.md has [[zettel1]]
		if len(output.Results) != 1 {
			t.Errorf("got %d backlinks, want 1. Results: %+v", len(output.Results), output.Results)
		}
	})

	t.Run("no backlinks", func(t *testing.T) {
		_, output, err := h.GetBacklinks(ctx, req, note.GetBacklinksInput{NoteName: "daily-journal"})
		if err != nil {
			t.Fatalf("GetBacklinks() error = %v", err)
		}
		if len(output.Results) != 0 {
			t.Errorf("got %d backlinks, want 0", len(output.Results))
		}
	})

	t.Run("handles .md suffix in input", func(t *testing.T) {
		_, output, err := h.GetBacklinks(ctx, req, note.GetBacklinksInput{NoteName: "note1.md"})
		if err != nil {
			t.Fatalf("GetBacklinks() error = %v", err)
		}
		if len(output.Results) != 2 {
			t.Errorf("got %d backlinks, want 2", len(output.Results))
		}
	})

	t.Run("error on empty name", func(t *testing.T) {
		_, _, err := h.GetBacklinks(ctx, req, note.GetBacklinksInput{NoteName: ""})
		if err == nil {
			t.Error("expected error for empty note_name")
		}
	})
}

// TestListTagsHandler tests the list_tags tool
func TestListTagsHandler(t *testing.T) {
	h, tmpVault := newTestHandler(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	t.Run("list all tags", func(t *testing.T) {
		result, output, err := h.ListTags(ctx, req, note.ListTagsInput{})
		if err != nil {
			t.Fatalf("ListTags() error = %v", err)
		}
		if result == nil {
			t.Fatal("result is nil")
		}
		if len(output.Tags) == 0 {
			t.Fatal("expected tags, got none")
		}

		// Check that known tags exist
		tagMap := make(map[string]int)
		for _, tc := range output.Tags {
			tagMap[tc.Tag] = tc.Count
		}

		// Frontmatter tags
		if _, ok := tagMap["zettelkasten"]; !ok {
			t.Error("missing frontmatter tag 'zettelkasten'")
		}
		if count := tagMap["productivity"]; count < 1 {
			t.Errorf("expected 'productivity' count >= 1, got %d", count)
		}

		// Inline tags
		if _, ok := tagMap["workflow"]; !ok {
			t.Error("missing inline tag 'workflow'")
		}
		if _, ok := tagMap["idea"]; !ok {
			t.Error("missing inline tag 'idea'")
		}

		// Verify sorted order
		for i := 1; i < len(output.Tags); i++ {
			if output.Tags[i].Tag < output.Tags[i-1].Tag {
				t.Errorf("tags not sorted: %q before %q", output.Tags[i-1].Tag, output.Tags[i].Tag)
			}
		}
	})

	t.Run("filter by prefix", func(t *testing.T) {
		_, output, err := h.ListTags(ctx, req, note.ListTagsInput{Prefix: "zettel"})
		if err != nil {
			t.Fatalf("ListTags() error = %v", err)
		}
		for _, tc := range output.Tags {
			if !strings.HasPrefix(tc.Tag, "zettel") {
				t.Errorf("tag %q does not match prefix 'zettel'", tc.Tag)
			}
		}
		if len(output.Tags) == 0 {
			t.Error("expected at least one tag with prefix 'zettel'")
		}
	})

	t.Run("prefix with no matches", func(t *testing.T) {
		_, output, err := h.ListTags(ctx, req, note.ListTagsInput{Prefix: "nonexistent"})
		if err != nil {
			t.Fatalf("ListTags() error = %v", err)
		}
		if len(output.Tags) != 0 {
			t.Errorf("expected 0 tags, got %d", len(output.Tags))
		}
	})
}
