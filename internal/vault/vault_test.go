package vault

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nsega/mcp-obsidian/internal/testutil"
)

// TestIsPathAllowed tests the path validation function
func TestIsPathAllowed(t *testing.T) {
	tmpVault := testutil.SetupTestVault(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	v := &Vault{Path: tmpVault}

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
			got, testErr := v.IsPathAllowed(tt.path)
			if (testErr != nil) != tt.wantErr {
				t.Errorf("IsPathAllowed() error = %v, wantErr %v", testErr, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsPathAllowed() = %v, want %v", got, tt.want)
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
	defer func() {
		if cleanupErr := os.RemoveAll(homeVault); cleanupErr != nil {
			t.Logf("Failed to cleanup home vault: %v", cleanupErr)
		}
	}()

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

	v := &Vault{Path: homeVault}

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
			got, testErr := v.IsPathAllowed(tt.path)
			if (testErr != nil) != tt.wantErr {
				t.Errorf("IsPathAllowed() error = %v, wantErr %v", testErr, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsPathAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsPathAllowedWithTildeVault tests with a vault path that was originally tilde-expanded
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
	defer func() {
		if cleanupErr := os.RemoveAll(homeVault); cleanupErr != nil {
			t.Logf("Failed to cleanup home vault: %v", cleanupErr)
		}
	}()

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

	// Use expanded path, as vault.New() does
	v := &Vault{Path: homeVault}

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
			got, testErr := v.IsPathAllowed(tt.path)
			if (testErr != nil) != tt.wantErr {
				t.Errorf("IsPathAllowed() error = %v, wantErr %v", testErr, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsPathAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsPathAllowedWithNonExistentVault tests behavior with non-existent vault
func TestIsPathAllowedWithNonExistentVault(t *testing.T) {
	vaultPath := "/tmp/nonexistent-vault-path-12345"
	v := &Vault{Path: vaultPath}

	// Test with a file path when vault doesn't exist
	testPath := filepath.Join(vaultPath, "test.md")
	allowed, err := v.IsPathAllowed(testPath)

	// Should not error (vault non-existence is handled)
	if err != nil {
		t.Errorf("IsPathAllowed() unexpected error with non-existent vault: %v", err)
	}

	// Should allow since path would be within vault if it existed
	if !allowed {
		t.Error("IsPathAllowed() should allow path within non-existent vault")
	}

	// Test with a path outside the non-existent vault
	outsidePath := "/tmp/outside.md"
	allowed, err = v.IsPathAllowed(outsidePath)
	if err != nil {
		t.Errorf("IsPathAllowed() unexpected error: %v", err)
	}
	if allowed {
		t.Error("IsPathAllowed() should reject path outside vault")
	}
}

// TestIsPathAllowedWithSymlinks tests symlink handling
func TestIsPathAllowedWithSymlinks(t *testing.T) {
	tmpVault := testutil.SetupTestVault(t)
	defer testutil.CleanupTestVault(t, tmpVault)

	v := &Vault{Path: tmpVault}

	// Create a symlink to a valid file within vault
	symlinkSource := filepath.Join(tmpVault, "note1.md")
	symlinkTarget := filepath.Join(tmpVault, "symlink-to-note.md")
	err := os.Symlink(symlinkSource, symlinkTarget)
	if err != nil {
		t.Skipf("Symlinks not supported on this system: %v", err)
	}

	// Test that symlink to file within vault is allowed
	allowed, err := v.IsPathAllowed(symlinkTarget)
	if err != nil {
		t.Errorf("IsPathAllowed() unexpected error: %v", err)
	}
	if !allowed {
		t.Error("IsPathAllowed() should allow symlink to file within vault")
	}

	// Create symlink pointing outside vault
	outsideFile := filepath.Join(os.TempDir(), "outside-vault-file.md")
	err = os.WriteFile(outsideFile, []byte("outside"), 0644)
	if err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}
	defer func() {
		if cleanupErr := os.Remove(outsideFile); cleanupErr != nil {
			t.Logf("Failed to cleanup outside file: %v", cleanupErr)
		}
	}()

	outsideSymlink := filepath.Join(tmpVault, "symlink-outside.md")
	err = os.Symlink(outsideFile, outsideSymlink)
	if err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	// Test that symlink pointing outside vault is rejected
	allowed, err = v.IsPathAllowed(outsideSymlink)
	if err != nil {
		t.Errorf("IsPathAllowed() unexpected error: %v", err)
	}
	if allowed {
		t.Error("IsPathAllowed() should reject symlink pointing outside vault")
	}
}
