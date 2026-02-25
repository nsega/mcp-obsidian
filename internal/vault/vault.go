package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxSearchResults limits the number of search results to prevent excessive output
const MaxSearchResults = 200

// Vault represents an Obsidian vault directory with path validation
type Vault struct {
	Path string // resolved absolute path to the vault root
}

// New creates a new Vault, expanding ~ and resolving to an absolute path.
// Returns an error if the vault directory does not exist.
func New(rawPath string) (*Vault, error) {
	path := rawPath

	// Expand home directory
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	path = absPath

	// Check if vault path exists and is a directory
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("vault path error: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault path is not a directory: %s", path)
	}

	return &Vault{Path: path}, nil
}

// IsPathAllowed checks if a given path is within the allowed vault directory
// and doesn't access hidden files or directories
func (v *Vault) IsPathAllowed(path string) (bool, error) {
	// Expand home directory
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Resolve symlinks
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If the path doesn't exist, walk up to the nearest existing
		// ancestor, resolve its symlinks, then re-append the remainder.
		if os.IsNotExist(err) {
			resolvedPath = resolveWithAncestors(absPath)
		} else {
			return false, fmt.Errorf("failed to resolve symlinks: %w", err)
		}
	}

	// Get absolute vault path
	absVaultPath, err := filepath.Abs(v.Path)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute vault path: %w", err)
	}

	// Resolve vault symlinks
	resolvedVaultPath, err := filepath.EvalSymlinks(absVaultPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("failed to resolve vault symlinks: %w", err)
		}
		resolvedVaultPath = resolveWithAncestors(absVaultPath)
	}

	// Clean paths
	cleanPath := filepath.Clean(resolvedPath)
	cleanVaultPath := filepath.Clean(resolvedVaultPath)

	// Check if path is within vault (ensure separator boundary to prevent
	// prefix false-positives like /tmp/vault matching /tmp/vault-evil)
	if cleanPath != cleanVaultPath && !strings.HasPrefix(cleanPath, cleanVaultPath+string(filepath.Separator)) {
		return false, nil
	}

	// Check for hidden files/directories
	relPath, err := filepath.Rel(cleanVaultPath, cleanPath)
	if err != nil {
		return false, fmt.Errorf("failed to get relative path: %w", err)
	}

	pathParts := strings.SplitSeq(relPath, string(filepath.Separator))
	for part := range pathParts {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return false, nil
		}
	}

	return true, nil
}

// resolveWithAncestors resolves symlinks by walking up to the nearest existing
// ancestor directory and re-appending the remaining path segments.
func resolveWithAncestors(absPath string) string {
	current := absPath
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			// Found an existing ancestor — rejoin the tail
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding existing path
			return absPath
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}
