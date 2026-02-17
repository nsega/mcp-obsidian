package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// SetupTestVault creates a temporary vault directory with test files
func SetupTestVault(t *testing.T) string {
	t.Helper()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "obsidian-vault-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create test directory structure
	testFiles := map[string]string{
		"note1.md":                    "# Note 1\nThis is the first note.",
		"note2.md":                    "# Note 2\nThis is the second note.\nSee also [[note1]] for details.",
		"project-plan.md":             "# Project Plan\nProject planning document.\nRelated: [[note1|first note]]",
		"daily-journal.md":            "# Daily Journal\nJournal entries.",
		"subfolder/nested-note.md":    "# Nested Note\nNote in subfolder.\nLinks to [[project-plan]].",
		"subfolder/project-update.md": "# Project Update\nUpdate in subfolder.",
		".hidden/secret.md":           "# Secret\nHidden file.",
		"subfolder/.hidden-note.md":   "# Hidden Note\nHidden file in subfolder.",
		"not-markdown.txt":            "This is a text file, not markdown.",
		"30_Permanent/zettel1.md":     "---\ntags:\n  - zettelkasten\n  - productivity\ncreated: 2026-01-01\nupdated: 2026-01-01\n---\n\n# Zettel 1\nA permanent note about #workflow and #productivity.",
		"30_Permanent/zettel2.md":     "---\ntags: [zettelkasten, reading]\ncreated: 2026-01-15\nupdated: 2026-01-15\n---\n\n# Zettel 2\nAnother note with #reading tag.\nSee [[zettel1]] for context.",
		"10_FleetingNote/fleeting.md": "---\ntags:\n  - fleeting\ncreated: 2026-02-01\nupdated: 2026-02-01\n---\n\n# Fleeting Thought\nQuick capture with #idea tag.",
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

// CleanupTestVault removes the temporary vault directory
func CleanupTestVault(t *testing.T, vaultPath string) {
	t.Helper()
	if err := os.RemoveAll(vaultPath); err != nil {
		t.Errorf("Failed to cleanup test vault: %v", err)
	}
}
