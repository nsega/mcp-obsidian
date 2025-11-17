package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxSearchResults = 200
)

var (
	vaultPath string
)

// SearchNotesInput defines the input for the search_notes tool
type SearchNotesInput struct {
	Query string `json:"query" jsonschema:"Search query or regex pattern to match note filenames"`
}

// SearchNotesOutput defines the output for the search_notes tool
type SearchNotesOutput struct {
	Results []string `json:"results" jsonschema:"List of matching note file paths"`
}

// ReadNotesInput defines the input for the read_notes tool
type ReadNotesInput struct {
	Paths []string `json:"paths" jsonschema:"Array of file paths to read"`
}

// ReadNotesOutput defines the output for the read_notes tool
type ReadNotesOutput struct {
	Notes []NoteContent `json:"notes" jsonschema:"Array of note contents"`
}

// NoteContent represents a single note's content
type NoteContent struct {
	Path    string `json:"path" jsonschema:"File path of the note"`
	Content string `json:"content" jsonschema:"Content of the note"`
	Error   string `json:"error,omitempty" jsonschema:"Error message if reading failed"`
}

// isPathAllowed checks if a given path is within the allowed vault directory
// and doesn't access hidden files or directories
func isPathAllowed(path string) (bool, error) {
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
		// If the file doesn't exist, check the parent directory
		if os.IsNotExist(err) {
			resolvedPath = absPath
		} else {
			return false, fmt.Errorf("failed to resolve symlinks: %w", err)
		}
	}

	// Get absolute vault path
	absVaultPath, err := filepath.Abs(vaultPath)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute vault path: %w", err)
	}

	// Resolve vault symlinks
	resolvedVaultPath, err := filepath.EvalSymlinks(absVaultPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("failed to resolve vault symlinks: %w", err)
		}
		resolvedVaultPath = absVaultPath
	}

	// Clean paths
	cleanPath := filepath.Clean(resolvedPath)
	cleanVaultPath := filepath.Clean(resolvedVaultPath)

	// Check if path is within vault
	if !strings.HasPrefix(cleanPath, cleanVaultPath) {
		return false, nil
	}

	// Check for hidden files/directories
	relPath, err := filepath.Rel(cleanVaultPath, cleanPath)
	if err != nil {
		return false, fmt.Errorf("failed to get relative path: %w", err)
	}

	pathParts := strings.Split(relPath, string(filepath.Separator))
	for _, part := range pathParts {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return false, nil
		}
	}

	return true, nil
}

// searchNotesHandler implements the search_notes tool
func searchNotesHandler(ctx context.Context, req *mcp.CallToolRequest, input SearchNotesInput) (*mcp.CallToolResult, SearchNotesOutput, error) {
	var results []string
	query := input.Query

	// Try to compile as regex, if it fails, use literal string matching
	var re *regexp.Regexp
	var useRegex bool
	if compiled, err := regexp.Compile("(?i)" + query); err == nil {
		re = compiled
		useRegex = true
	}

	// Walk through the vault directory
	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Skip directories
		if info.IsDir() {
			// Check if directory is allowed (not hidden)
			allowed, checkErr := isPathAllowed(path)
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
		allowed, checkErr := isPathAllowed(path)
		if checkErr != nil || !allowed {
			return nil
		}

		// Get filename
		filename := filepath.Base(path)

		// Match against query
		matched := false
		if useRegex {
			matched = re.MatchString(filename)
		} else {
			matched = strings.Contains(strings.ToLower(filename), strings.ToLower(query))
		}

		if matched {
			results = append(results, path)
			// Limit results
			if len(results) >= maxSearchResults {
				return filepath.SkipAll
			}
		}

		return nil
	})

	if err != nil {
		return nil, SearchNotesOutput{}, fmt.Errorf("failed to search notes: %w", err)
	}

	output := SearchNotesOutput{
		Results: results,
	}

	// Create text content for the result
	textContent := fmt.Sprintf("Found %d matching notes", len(results))
	if len(results) > 0 {
		textContent += ":\n" + strings.Join(results, "\n")
	}

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: textContent,
			},
		},
	}

	return result, output, nil
}

// readNotesHandler implements the read_notes tool
func readNotesHandler(ctx context.Context, req *mcp.CallToolRequest, input ReadNotesInput) (*mcp.CallToolResult, ReadNotesOutput, error) {
	notes := make([]NoteContent, 0, len(input.Paths))

	for _, path := range input.Paths {
		note := NoteContent{
			Path: path,
		}

		// Check if path is allowed
		allowed, err := isPathAllowed(path)
		if err != nil {
			note.Error = fmt.Sprintf("Path validation error: %v", err)
			notes = append(notes, note)
			continue
		}

		if !allowed {
			note.Error = "Access denied: path is outside vault or is a hidden file"
			notes = append(notes, note)
			continue
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			note.Error = fmt.Sprintf("Failed to read file: %v", err)
			notes = append(notes, note)
			continue
		}

		note.Content = string(content)
		notes = append(notes, note)
	}

	output := ReadNotesOutput{
		Notes: notes,
	}

	// Create text content for the result
	var textParts []string
	for _, note := range notes {
		if note.Error != "" {
			textParts = append(textParts, fmt.Sprintf("## %s\nError: %s", note.Path, note.Error))
		} else {
			textParts = append(textParts, fmt.Sprintf("## %s\n%s", note.Path, note.Content))
		}
	}

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: strings.Join(textParts, "\n\n"),
			},
		},
	}

	return result, output, nil
}

func main() {
	// Check command line arguments
	if len(os.Args) < 2 {
		log.Fatal("Usage: mcp-obsidian <vault-path>")
	}

	// Get vault path from command line
	vaultPath = os.Args[1]

	// Expand home directory
	if strings.HasPrefix(vaultPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get home directory: %v", err)
		}
		vaultPath = filepath.Join(home, vaultPath[1:])
	}

	// Check if vault path exists
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		log.Fatalf("Vault path does not exist: %s", vaultPath)
	}

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-obsidian",
		Version: "2.0.0",
	}, nil)

	// Register search_notes tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_notes",
		Description: "Search for notes by filename using case-insensitive matching. Supports regex patterns.",
	}, searchNotesHandler)

	// Register read_notes tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_notes",
		Description: "Read the content of one or more notes. Returns content with error handling per file.",
	}, readNotesHandler)

	// Log server start to stderr (stdout is used for MCP communication)
	fmt.Fprintf(os.Stderr, "MCP Obsidian server starting...\n")
	fmt.Fprintf(os.Stderr, "Vault path: %s\n", vaultPath)

	// Run the server with stdio transport
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
