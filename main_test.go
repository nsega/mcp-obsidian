package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// setupTestVault creates a temporary vault directory with test files
func setupTestVault(t *testing.T) string {
	t.Helper()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "obsidian-vault-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create test directory structure
	testFiles := map[string]string{
		"note1.md":                    "# Note 1\nThis is the first note.",
		"note2.md":                    "# Note 2\nThis is the second note.",
		"project-plan.md":             "# Project Plan\nProject planning document.",
		"daily-journal.md":            "# Daily Journal\nJournal entries.",
		"subfolder/nested-note.md":    "# Nested Note\nNote in subfolder.",
		"subfolder/project-update.md": "# Project Update\nUpdate in subfolder.",
		".hidden/secret.md":           "# Secret\nHidden file.",
		"subfolder/.hidden-note.md":   "# Hidden Note\nHidden file in subfolder.",
		"not-markdown.txt":            "This is a text file, not markdown.",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)

		// Create directory if it doesn't exist
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}

		// Write file
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	return tmpDir
}

// cleanupTestVault removes the temporary vault directory
func cleanupTestVault(t *testing.T, vaultPath string) {
	t.Helper()
	if err := os.RemoveAll(vaultPath); err != nil {
		t.Errorf("Failed to cleanup test vault: %v", err)
	}
}

// TestIsPathAllowed tests the path validation function
func TestIsPathAllowed(t *testing.T) {
	tmpVault := setupTestVault(t)
	defer cleanupTestVault(t, tmpVault)

	// Set global vaultPath for testing
	originalVaultPath := vaultPath
	vaultPath = tmpVault
	defer func() { vaultPath = originalVaultPath }()

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{
			name: "valid file in vault",
			path: filepath.Join(tmpVault, "note1.md"),
			want: true,
		},
		{
			name: "valid file in subfolder",
			path: filepath.Join(tmpVault, "subfolder", "nested-note.md"),
			want: true,
		},
		{
			name: "hidden file should be rejected",
			path: filepath.Join(tmpVault, ".hidden", "secret.md"),
			want: false,
		},
		{
			name: "hidden file in subfolder should be rejected",
			path: filepath.Join(tmpVault, "subfolder", ".hidden-note.md"),
			want: false,
		},
		{
			name: "file outside vault should be rejected",
			path: "/tmp/outside-vault.md",
			want: false,
		},
		{
			name: "path traversal attempt should be rejected",
			path: filepath.Join(tmpVault, "..", "escape.md"),
			want: false,
		},
		{
			name: "non-existent file in vault should be allowed",
			path: filepath.Join(tmpVault, "nonexistent-file.md"),
			want: true,
		},
		{
			name: "non-existent file outside vault should be rejected",
			path: "/tmp/nonexistent-outside.md",
			want: false,
		},
		{
			name: "path with dot segments within vault",
			path: filepath.Join(tmpVault, "subfolder", "..", "note1.md"),
			want: true,
		},
		{
			name: "relative path within vault",
			path: filepath.Join(tmpVault, ".", "note1.md"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, testErr := isPathAllowed(tt.path)
			if (testErr != nil) != tt.wantErr {
				t.Errorf("isPathAllowed() error = %v, wantErr %v", testErr, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isPathAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsPathAllowedWithTilde tests home directory expansion
func TestIsPathAllowedWithTilde(t *testing.T) {
	// Create a test vault in home directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	homeVault := filepath.Join(home, ".test-obsidian-vault-tilde")
	err = os.MkdirAll(homeVault, 0755)
	if err != nil {
		t.Fatalf("Failed to create home vault: %v", err)
	}
	defer func() { _ = os.RemoveAll(homeVault) }()

	// Create test files
	testFile := filepath.Join(homeVault, "test-note.md")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create hidden file
	hiddenDir := filepath.Join(homeVault, ".hidden")
	err = os.MkdirAll(hiddenDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create hidden dir: %v", err)
	}
	hiddenFile := filepath.Join(hiddenDir, "secret.md")
	err = os.WriteFile(hiddenFile, []byte("secret"), 0644)
	if err != nil {
		t.Fatalf("Failed to create hidden file: %v", err)
	}

	// Set vault to use tilde path
	originalVaultPath := vaultPath
	vaultPath = homeVault
	defer func() { vaultPath = originalVaultPath }()

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{
			name: "tilde path to valid file in vault",
			path: "~/.test-obsidian-vault-tilde/test-note.md",
			want: true,
		},
		{
			name: "tilde path to hidden file should be rejected",
			path: "~/.test-obsidian-vault-tilde/.hidden/secret.md",
			want: false,
		},
		{
			name: "tilde path to file outside vault",
			path: "~/.bashrc",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, testErr := isPathAllowed(tt.path)
			if (testErr != nil) != tt.wantErr {
				t.Errorf("isPathAllowed() error = %v, wantErr %v", testErr, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isPathAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Note: In production, main() expands vaultPath before isPathAllowed is called,
// so we simulate that behavior here by expanding vaultPath manually
func TestIsPathAllowedWithTildeVault(t *testing.T) {
	// Create a test vault in home directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	homeVault := filepath.Join(home, ".test-obsidian-vault-tilde2")
	err = os.MkdirAll(homeVault, 0755)
	if err != nil {
		t.Fatalf("Failed to create home vault: %v", err)
	}
	defer func() { _ = os.RemoveAll(homeVault) }()

	// Create test files
	testFile := filepath.Join(homeVault, "test-note.md")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	subdir := filepath.Join(homeVault, "subdir")
	err = os.MkdirAll(subdir, 0755)
	if err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	subdirFile := filepath.Join(subdir, "nested.md")
	err = os.WriteFile(subdirFile, []byte("nested content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create nested file: %v", err)
	}

	// Set vault path (simulating main() which would have already expanded tilde)
	// In production, main() expands ~ before setting vaultPath
	originalVaultPath := vaultPath
	vaultPath = homeVault // Use expanded path, as main() does
	defer func() { vaultPath = originalVaultPath }()

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{
			name: "absolute path to file in vault that was originally tilde path",
			path: testFile,
			want: true,
		},
		{
			name: "absolute path to nested file in vault",
			path: subdirFile,
			want: true,
		},
		{
			name: "tilde path to file in vault",
			path: "~/.test-obsidian-vault-tilde2/test-note.md",
			want: true,
		},
		{
			name: "path outside vault",
			path: "~/.bashrc",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, testErr := isPathAllowed(tt.path)
			if (testErr != nil) != tt.wantErr {
				t.Errorf("isPathAllowed() error = %v, wantErr %v", testErr, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isPathAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsPathAllowedWithNonExistentVault tests behavior with non-existent vault
func TestIsPathAllowedWithNonExistentVault(t *testing.T) {
	// Set vault to a non-existent directory
	originalVaultPath := vaultPath
	vaultPath = "/tmp/nonexistent-vault-path-12345"
	defer func() { vaultPath = originalVaultPath }()

	// Test with a file path when vault doesn't exist
	testPath := filepath.Join(vaultPath, "test.md")
	allowed, err := isPathAllowed(testPath)

	// Should not error (vault non-existence is handled)
	if err != nil {
		t.Errorf("isPathAllowed() unexpected error with non-existent vault: %v", err)
	}

	// Should allow since path would be within vault if it existed
	if !allowed {
		t.Error("isPathAllowed() should allow path within non-existent vault")
	}

	// Test with a path outside the non-existent vault
	outsidePath := "/tmp/outside.md"
	allowed, err = isPathAllowed(outsidePath)
	if err != nil {
		t.Errorf("isPathAllowed() unexpected error: %v", err)
	}
	if allowed {
		t.Error("isPathAllowed() should reject path outside vault")
	}
}

// TestIsPathAllowedWithSymlinks tests symlink handling
func TestIsPathAllowedWithSymlinks(t *testing.T) {
	tmpVault := setupTestVault(t)
	defer cleanupTestVault(t, tmpVault)

	// Set global vaultPath for testing
	originalVaultPath := vaultPath
	vaultPath = tmpVault
	defer func() { vaultPath = originalVaultPath }()

	// Create a symlink to a valid file within vault
	symlinkSource := filepath.Join(tmpVault, "note1.md")
	symlinkTarget := filepath.Join(tmpVault, "symlink-to-note.md")
	err := os.Symlink(symlinkSource, symlinkTarget)
	if err != nil {
		t.Skipf("Symlinks not supported on this system: %v", err)
	}

	// Test that symlink to file within vault is allowed
	allowed, err := isPathAllowed(symlinkTarget)
	if err != nil {
		t.Errorf("isPathAllowed() unexpected error: %v", err)
	}
	if !allowed {
		t.Error("isPathAllowed() should allow symlink to file within vault")
	}

	// Create symlink pointing outside vault
	outsideFile := filepath.Join(os.TempDir(), "outside-vault-file.md")
	err = os.WriteFile(outsideFile, []byte("outside"), 0644)
	if err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}
	defer func() { _ = os.Remove(outsideFile) }()

	outsideSymlink := filepath.Join(tmpVault, "symlink-outside.md")
	err = os.Symlink(outsideFile, outsideSymlink)
	if err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	// Test that symlink pointing outside vault is rejected
	allowed, err = isPathAllowed(outsideSymlink)
	if err != nil {
		t.Errorf("isPathAllowed() unexpected error: %v", err)
	}
	if allowed {
		t.Error("isPathAllowed() should reject symlink pointing outside vault")
	}
}

// TestSearchNotesHandler tests the search_notes tool
func TestSearchNotesHandler(t *testing.T) {
	tmpVault := setupTestVault(t)
	defer cleanupTestVault(t, tmpVault)

	// Set global vaultPath for testing
	originalVaultPath := vaultPath
	vaultPath = tmpVault
	defer func() { vaultPath = originalVaultPath }()

	tests := []struct {
		name            string
		input           SearchNotesInput
		wantContains    []string
		wantNotContains []string
		wantCount       int
		wantErr         bool
	}{
		{
			name: "search for 'note' should find multiple files",
			input: SearchNotesInput{
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
			input: SearchNotesInput{
				Query: "project",
			},
			wantCount: 2, // project-plan.md, project-update.md
			wantContains: []string{
				"project-plan.md",
				"project-update.md",
			},
		},
		{
			name: "regex search for files starting with 'note'",
			input: SearchNotesInput{
				Query: "^note",
			},
			wantCount: 2, // note1.md, note2.md
			wantContains: []string{
				"note1.md",
				"note2.md",
			},
			wantNotContains: []string{
				"nested-note.md",
			},
		},
		{
			name: "case insensitive search",
			input: SearchNotesInput{
				Query: "JOURNAL",
			},
			wantCount: 1, // daily-journal.md
			wantContains: []string{
				"daily-journal.md",
			},
		},
		{
			name: "search with no matches",
			input: SearchNotesInput{
				Query: "nonexistent",
			},
			wantCount: 0,
		},
		{
			name: "search should exclude hidden files",
			input: SearchNotesInput{
				Query: "secret",
			},
			wantCount: 0, // secret.md is in .hidden directory
		},
		{
			name: "search should only return .md files",
			input: SearchNotesInput{
				Query: "txt",
			},
			wantCount: 0, // not-markdown.txt should not be found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req := &mcp.CallToolRequest{}

			result, output, err := searchNotesHandler(ctx, req, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("searchNotesHandler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			// Check result count
			if len(output.Results) != tt.wantCount {
				t.Errorf("searchNotesHandler() got %d results, want %d. Results: %v",
					len(output.Results), tt.wantCount, output.Results)
			}

			// Check that expected files are present
			for _, expected := range tt.wantContains {
				found := false
				for _, result := range output.Results {
					if strings.Contains(result, expected) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("searchNotesHandler() results missing expected file: %s", expected)
				}
			}

			// Check that unexpected files are not present
			for _, notExpected := range tt.wantNotContains {
				for _, result := range output.Results {
					if strings.Contains(result, notExpected) {
						t.Errorf("searchNotesHandler() results contain unexpected file: %s", notExpected)
					}
				}
			}

			// Check that result content is properly formatted
			if result == nil {
				t.Error("searchNotesHandler() result is nil")
				return
			}

			if len(result.Content) == 0 {
				t.Error("searchNotesHandler() result content is empty")
			}
		})
	}
}

// TestSearchNotesMaxResults tests that search respects the max results limit
func TestSearchNotesMaxResults(t *testing.T) {
	tmpVault := setupTestVault(t)
	defer cleanupTestVault(t, tmpVault)

	// Create directory for test files
	manyDir := filepath.Join(tmpVault, "many")
	if err := os.MkdirAll(manyDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create many test files to exceed maxSearchResults
	for i := 0; i < maxSearchResults+50; i++ {
		// Create unique filenames using index
		path := filepath.Join(manyDir, fmt.Sprintf("test-note-%d.md", i))
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Set global vaultPath for testing
	originalVaultPath := vaultPath
	vaultPath = tmpVault
	defer func() { vaultPath = originalVaultPath }()

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := SearchNotesInput{Query: "test"}

	_, output, err := searchNotesHandler(ctx, req, input)
	if err != nil {
		t.Fatalf("searchNotesHandler() error = %v", err)
	}

	// Should not exceed maxSearchResults
	if len(output.Results) > maxSearchResults {
		t.Errorf("searchNotesHandler() returned %d results, should not exceed %d",
			len(output.Results), maxSearchResults)
	}
}

// TestReadNotesHandler tests the read_notes tool
func TestReadNotesHandler(t *testing.T) {
	tmpVault := setupTestVault(t)
	defer cleanupTestVault(t, tmpVault)

	// Set global vaultPath for testing
	originalVaultPath := vaultPath
	vaultPath = tmpVault
	defer func() { vaultPath = originalVaultPath }()

	tests := []struct {
		validate  func(t *testing.T, output ReadNotesOutput)
		name      string
		input     ReadNotesInput
		wantCount int
	}{
		{
			name: "read single file successfully",
			input: ReadNotesInput{
				Paths: []string{filepath.Join(tmpVault, "note1.md")},
			},
			wantCount: 1,
			validate: func(t *testing.T, output ReadNotesOutput) {
				if len(output.Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(output.Notes))
				}
				note := output.Notes[0]
				if note.Error != "" {
					t.Errorf("Expected no error, got: %s", note.Error)
				}
				if !strings.Contains(note.Content, "# Note 1") {
					t.Errorf("Expected content to contain '# Note 1', got: %s", note.Content)
				}
			},
		},
		{
			name: "read multiple files successfully",
			input: ReadNotesInput{
				Paths: []string{
					filepath.Join(tmpVault, "note1.md"),
					filepath.Join(tmpVault, "note2.md"),
					filepath.Join(tmpVault, "subfolder", "nested-note.md"),
				},
			},
			wantCount: 3,
			validate: func(t *testing.T, output ReadNotesOutput) {
				if len(output.Notes) != 3 {
					t.Fatalf("Expected 3 notes, got %d", len(output.Notes))
				}
				for i, note := range output.Notes {
					if note.Error != "" {
						t.Errorf("Note %d: Expected no error, got: %s", i, note.Error)
					}
					if note.Content == "" {
						t.Errorf("Note %d: Expected content, got empty string", i)
					}
				}
			},
		},
		{
			name: "read non-existent file should return error",
			input: ReadNotesInput{
				Paths: []string{filepath.Join(tmpVault, "nonexistent.md")},
			},
			wantCount: 1,
			validate: func(t *testing.T, output ReadNotesOutput) {
				if len(output.Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(output.Notes))
				}
				note := output.Notes[0]
				if note.Error == "" {
					t.Error("Expected error for non-existent file, got none")
				}
				if !strings.Contains(note.Error, "Failed to read file") {
					t.Errorf("Expected 'Failed to read file' error, got: %s", note.Error)
				}
			},
		},
		{
			name: "read file outside vault should return error",
			input: ReadNotesInput{
				Paths: []string{"/tmp/outside-vault.md"},
			},
			wantCount: 1,
			validate: func(t *testing.T, output ReadNotesOutput) {
				if len(output.Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(output.Notes))
				}
				note := output.Notes[0]
				if note.Error == "" {
					t.Error("Expected error for file outside vault, got none")
				}
				if !strings.Contains(note.Error, "Access denied") {
					t.Errorf("Expected 'Access denied' error, got: %s", note.Error)
				}
			},
		},
		{
			name: "read hidden file should return error",
			input: ReadNotesInput{
				Paths: []string{filepath.Join(tmpVault, ".hidden", "secret.md")},
			},
			wantCount: 1,
			validate: func(t *testing.T, output ReadNotesOutput) {
				if len(output.Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(output.Notes))
				}
				note := output.Notes[0]
				if note.Error == "" {
					t.Error("Expected error for hidden file, got none")
				}
				if !strings.Contains(note.Error, "Access denied") {
					t.Errorf("Expected 'Access denied' error, got: %s", note.Error)
				}
			},
		},
		{
			name: "read mixed valid and invalid files",
			input: ReadNotesInput{
				Paths: []string{
					filepath.Join(tmpVault, "note1.md"),             // valid
					filepath.Join(tmpVault, "nonexistent.md"),       // invalid - doesn't exist
					filepath.Join(tmpVault, ".hidden", "secret.md"), // invalid - hidden
				},
			},
			wantCount: 3,
			validate: func(t *testing.T, output ReadNotesOutput) {
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
			input: ReadNotesInput{
				Paths: []string{},
			},
			wantCount: 0,
			validate: func(t *testing.T, output ReadNotesOutput) {
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

			result, output, err := readNotesHandler(ctx, req, tt.input)
			if err != nil {
				t.Fatalf("readNotesHandler() error = %v", err)
			}

			// Check result is not nil
			if result == nil {
				t.Fatal("readNotesHandler() result is nil")
			}

			// Check result has content
			if len(result.Content) == 0 {
				t.Error("readNotesHandler() result content is empty")
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
	tmpVault := setupTestVault(t)
	defer cleanupTestVault(t, tmpVault)

	// Set global vaultPath for testing
	originalVaultPath := vaultPath
	vaultPath = tmpVault
	defer func() { vaultPath = originalVaultPath }()

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := ReadNotesInput{
		Paths: []string{
			filepath.Join(tmpVault, "note1.md"),
			filepath.Join(tmpVault, "note2.md"),
		},
	}

	result, _, err := readNotesHandler(ctx, req, input)
	if err != nil {
		t.Fatalf("readNotesHandler() error = %v", err)
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
