package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

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

// CreateNoteInput defines the input for the create_note tool
type CreateNoteInput struct {
	Title   string   `json:"title" jsonschema:"required,Title for the note used in filename slug and heading"`
	Content string   `json:"content" jsonschema:"Markdown body content of the note"`
	Folder  string   `json:"folder,omitempty" jsonschema:"Subfolder within the vault (e.g. 30_Permanent, 10_FleetingNote)"`
	Tags    []string `json:"tags,omitempty" jsonschema:"Frontmatter tags for the note"`
}

// CreateNoteOutput defines the output for the create_note tool
type CreateNoteOutput struct {
	Path string `json:"path" jsonschema:"Full path of the created note"`
}

// UpdateNoteInput defines the input for the update_note tool
type UpdateNoteInput struct {
	Path    string `json:"path" jsonschema:"required,Full path to the note to update"`
	Content string `json:"content" jsonschema:"required,New content to write or append"`
	Mode    string `json:"mode,omitempty" jsonschema:"Update mode: replace (default) or append"`
}

// UpdateNoteOutput defines the output for the update_note tool
type UpdateNoteOutput struct {
	Path string `json:"path" jsonschema:"Full path of the updated note"`
}

// DeleteNoteInput defines the input for the delete_note tool
type DeleteNoteInput struct {
	Path string `json:"path" jsonschema:"required,Full path to the note to delete"`
}

// DeleteNoteOutput defines the output for the delete_note tool
type DeleteNoteOutput struct {
	Path string `json:"path" jsonschema:"Full path of the deleted note"`
}

// SearchContentInput defines the input for the search_content tool
type SearchContentInput struct {
	Query string `json:"query" jsonschema:"required,Search query or regex pattern to match note content"`
}

// ContentMatch represents a single content search match
type ContentMatch struct {
	Path    string `json:"path" jsonschema:"File path of the matching note"`
	Snippet string `json:"snippet" jsonschema:"The matching line content"`
	Line    int    `json:"line" jsonschema:"Line number of the match"`
}

// SearchContentOutput defines the output for the search_content tool
type SearchContentOutput struct {
	Results []ContentMatch `json:"results" jsonschema:"List of matching content results"`
}

// GetBacklinksInput defines the input for the get_backlinks tool
type GetBacklinksInput struct {
	NoteName string `json:"note_name" jsonschema:"required,Note name without .md extension or path"`
}

// Backlink represents a single backlink reference
type Backlink struct {
	Path string `json:"path" jsonschema:"File path of the note containing the backlink"`
	Line string `json:"line" jsonschema:"Line content containing the wikilink"`
}

// GetBacklinksOutput defines the output for the get_backlinks tool
type GetBacklinksOutput struct {
	Results []Backlink `json:"results" jsonschema:"List of backlink references"`
}

// ListTagsInput defines the input for the list_tags tool
type ListTagsInput struct {
	Prefix string `json:"prefix,omitempty" jsonschema:"Optional prefix to filter tags"`
}

// TagCount represents a tag and its occurrence count
type TagCount struct {
	Tag   string `json:"tag" jsonschema:"Tag name"`
	Count int    `json:"count" jsonschema:"Number of occurrences"`
}

// ListTagsOutput defines the output for the list_tags tool
type ListTagsOutput struct {
	Tags []TagCount `json:"tags" jsonschema:"Sorted list of tags with counts"`
}

// slugify converts a title string into a URL/filename-safe slug
func slugify(s string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	result := b.String()
	return strings.TrimRight(result, "-")
}

// generateFrontmatter creates YAML frontmatter with tags and timestamps
func generateFrontmatter(tags []string, created, updated string) string {
	var b strings.Builder
	b.WriteString("---\n")
	if len(tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range tags {
			b.WriteString("  - ")
			b.WriteString(tag)
			b.WriteByte('\n')
		}
	}
	b.WriteString("created: ")
	b.WriteString(created)
	b.WriteByte('\n')
	b.WriteString("updated: ")
	b.WriteString(updated)
	b.WriteByte('\n')
	b.WriteString("---\n")
	return b.String()
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
		// If the path doesn't exist, walk up to the nearest existing
		// ancestor, resolve its symlinks, then re-append the remainder.
		if os.IsNotExist(err) {
			resolvedPath = resolveWithAncestors(absPath)
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
		resolvedVaultPath = resolveWithAncestors(absVaultPath)
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

// createNoteHandler implements the create_note tool
func createNoteHandler(ctx context.Context, req *mcp.CallToolRequest, input CreateNoteInput) (*mcp.CallToolResult, CreateNoteOutput, error) {
	if input.Title == "" {
		return nil, CreateNoteOutput{}, fmt.Errorf("title is required")
	}

	slug := slugify(input.Title)
	if slug == "" {
		return nil, CreateNoteOutput{}, fmt.Errorf("title produces an empty slug")
	}

	today := time.Now().Format("2006-01-02")
	filename := today + "_" + slug + ".md"

	dir := vaultPath
	if input.Folder != "" {
		dir = filepath.Join(vaultPath, input.Folder)
	}

	fullPath := filepath.Join(dir, filename)

	allowed, err := isPathAllowed(fullPath)
	if err != nil {
		return nil, CreateNoteOutput{}, fmt.Errorf("path validation error: %w", err)
	}
	if !allowed {
		return nil, CreateNoteOutput{}, fmt.Errorf("access denied: path is outside vault or is a hidden file")
	}

	if _, err := os.Stat(fullPath); err == nil {
		return nil, CreateNoteOutput{}, fmt.Errorf("file already exists: %s", fullPath)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, CreateNoteOutput{}, fmt.Errorf("failed to create directory: %w", err)
	}

	frontmatter := generateFrontmatter(input.Tags, today, today)

	var content strings.Builder
	content.WriteString(frontmatter)
	content.WriteString("\n# ")
	content.WriteString(input.Title)
	content.WriteByte('\n')
	if input.Content != "" {
		content.WriteByte('\n')
		content.WriteString(input.Content)
		content.WriteByte('\n')
	}

	if err := os.WriteFile(fullPath, []byte(content.String()), 0644); err != nil {
		return nil, CreateNoteOutput{}, fmt.Errorf("failed to write file: %w", err)
	}

	output := CreateNoteOutput{Path: fullPath}
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Created note: %s", fullPath)},
		},
	}
	return result, output, nil
}

// updateNoteHandler implements the update_note tool
func updateNoteHandler(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, UpdateNoteOutput, error) {
	allowed, err := isPathAllowed(input.Path)
	if err != nil {
		return nil, UpdateNoteOutput{}, fmt.Errorf("path validation error: %w", err)
	}
	if !allowed {
		return nil, UpdateNoteOutput{}, fmt.Errorf("access denied: path is outside vault or is a hidden file")
	}

	if _, err := os.Stat(input.Path); os.IsNotExist(err) {
		return nil, UpdateNoteOutput{}, fmt.Errorf("file does not exist: %s", input.Path)
	}

	mode := input.Mode
	if mode == "" {
		mode = "replace"
	}

	switch mode {
	case "replace":
		if err := os.WriteFile(input.Path, []byte(input.Content), 0644); err != nil {
			return nil, UpdateNoteOutput{}, fmt.Errorf("failed to write file: %w", err)
		}
	case "append":
		existing, err := os.ReadFile(input.Path)
		if err != nil {
			return nil, UpdateNoteOutput{}, fmt.Errorf("failed to read file: %w", err)
		}
		newContent := string(existing) + "\n\n" + input.Content
		if err := os.WriteFile(input.Path, []byte(newContent), 0644); err != nil {
			return nil, UpdateNoteOutput{}, fmt.Errorf("failed to write file: %w", err)
		}
	default:
		return nil, UpdateNoteOutput{}, fmt.Errorf("invalid mode: %s (must be 'replace' or 'append')", mode)
	}

	output := UpdateNoteOutput{Path: input.Path}
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Updated note (%s): %s", mode, input.Path)},
		},
	}
	return result, output, nil
}

// deleteNoteHandler implements the delete_note tool
func deleteNoteHandler(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, DeleteNoteOutput, error) {
	if !strings.HasSuffix(strings.ToLower(input.Path), ".md") {
		return nil, DeleteNoteOutput{}, fmt.Errorf("can only delete .md files")
	}

	allowed, err := isPathAllowed(input.Path)
	if err != nil {
		return nil, DeleteNoteOutput{}, fmt.Errorf("path validation error: %w", err)
	}
	if !allowed {
		return nil, DeleteNoteOutput{}, fmt.Errorf("access denied: path is outside vault or is a hidden file")
	}

	info, err := os.Stat(input.Path)
	if os.IsNotExist(err) {
		return nil, DeleteNoteOutput{}, fmt.Errorf("file does not exist: %s", input.Path)
	}
	if err != nil {
		return nil, DeleteNoteOutput{}, fmt.Errorf("failed to stat file: %w", err)
	}
	if info.IsDir() {
		return nil, DeleteNoteOutput{}, fmt.Errorf("cannot delete directories")
	}

	if err := os.Remove(input.Path); err != nil {
		return nil, DeleteNoteOutput{}, fmt.Errorf("failed to delete file: %w", err)
	}

	output := DeleteNoteOutput(input)
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Deleted note: %s", input.Path)},
		},
	}
	return result, output, nil
}

// searchContentHandler implements the search_content tool
func searchContentHandler(ctx context.Context, req *mcp.CallToolRequest, input SearchContentInput) (*mcp.CallToolResult, SearchContentOutput, error) {
	query := input.Query

	var re *regexp.Regexp
	var useRegex bool
	if compiled, err := regexp.Compile("(?i)" + query); err == nil {
		re = compiled
		useRegex = true
	}

	var results []ContentMatch

	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			allowed, checkErr := isPathAllowed(path)
			if checkErr != nil || !allowed {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		allowed, checkErr := isPathAllowed(path)
		if checkErr != nil || !allowed {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for lineNum, line := range lines {

			matched := false
			if useRegex {
				matched = re.MatchString(line)
			} else {
				matched = strings.Contains(strings.ToLower(line), strings.ToLower(query))
			}

			if matched {
				results = append(results, ContentMatch{
					Path:    path,
					Snippet: line,
					Line:    lineNum + 1,
				})
				if len(results) >= maxSearchResults {
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, SearchContentOutput{}, fmt.Errorf("failed to search content: %w", err)
	}

	output := SearchContentOutput{Results: results}

	textContent := fmt.Sprintf("Found %d matching lines", len(results))
	if len(results) > 0 {
		var lines []string
		for _, m := range results {
			lines = append(lines, fmt.Sprintf("%s:%d: %s", m.Path, m.Line, m.Snippet))
		}
		textContent += ":\n" + strings.Join(lines, "\n")
	}

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textContent},
		},
	}
	return result, output, nil
}

// getBacklinksHandler implements the get_backlinks tool
func getBacklinksHandler(ctx context.Context, req *mcp.CallToolRequest, input GetBacklinksInput) (*mcp.CallToolResult, GetBacklinksOutput, error) {
	if input.NoteName == "" {
		return nil, GetBacklinksOutput{}, fmt.Errorf("note_name is required")
	}

	noteName := strings.TrimSuffix(input.NoteName, ".md")
	wikilinkRe := regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

	var results []Backlink

	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			allowed, checkErr := isPathAllowed(path)
			if checkErr != nil || !allowed {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		allowed, checkErr := isPathAllowed(path)
		if checkErr != nil || !allowed {
			return nil
		}

		// Exclude the target note itself
		baseName := strings.TrimSuffix(filepath.Base(path), ".md")
		if strings.EqualFold(baseName, noteName) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			matches := wikilinkRe.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					linkTarget := strings.TrimSuffix(match[1], ".md")
					if strings.EqualFold(linkTarget, noteName) {
						results = append(results, Backlink{
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
		return nil, GetBacklinksOutput{}, fmt.Errorf("failed to search backlinks: %w", err)
	}

	output := GetBacklinksOutput{Results: results}

	textContent := fmt.Sprintf("Found %d backlinks to [[%s]]", len(results), noteName)
	if len(results) > 0 {
		var lines []string
		for _, bl := range results {
			lines = append(lines, fmt.Sprintf("%s: %s", bl.Path, bl.Line))
		}
		textContent += ":\n" + strings.Join(lines, "\n")
	}

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textContent},
		},
	}
	return result, output, nil
}

// listTagsHandler implements the list_tags tool
func listTagsHandler(ctx context.Context, req *mcp.CallToolRequest, input ListTagsInput) (*mcp.CallToolResult, ListTagsOutput, error) {
	tagCounts := make(map[string]int)

	// Regex for inline #tags (not inside code blocks)
	inlineTagRe := regexp.MustCompile(`(?:^|\s)#([a-zA-Z][a-zA-Z0-9_/-]*)`)

	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			allowed, checkErr := isPathAllowed(path)
			if checkErr != nil || !allowed {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		allowed, checkErr := isPathAllowed(path)
		if checkErr != nil || !allowed {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)

		// Parse frontmatter tags
		if strings.HasPrefix(content, "---\n") {
			endIdx := strings.Index(content[4:], "\n---\n")
			if endIdx >= 0 {
				frontmatter := content[4 : 4+endIdx]
				parseFrontmatterTags(frontmatter, tagCounts)
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
		return nil, ListTagsOutput{}, fmt.Errorf("failed to list tags: %w", err)
	}

	// Filter by prefix and sort
	var tags []TagCount
	for tag, count := range tagCounts {
		if input.Prefix == "" || strings.HasPrefix(tag, input.Prefix) {
			tags = append(tags, TagCount{Tag: tag, Count: count})
		}
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Tag < tags[j].Tag
	})

	output := ListTagsOutput{Tags: tags}

	textContent := fmt.Sprintf("Found %d unique tags", len(tags))
	if len(tags) > 0 {
		var lines []string
		for _, tc := range tags {
			lines = append(lines, fmt.Sprintf("#%s (%d)", tc.Tag, tc.Count))
		}
		textContent += ":\n" + strings.Join(lines, "\n")
	}

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textContent},
		},
	}
	return result, output, nil
}

// parseFrontmatterTags parses tags from YAML frontmatter content (without delimiters)
func parseFrontmatterTags(frontmatter string, tagCounts map[string]int) {
	lines := strings.Split(frontmatter, "\n")
	inTags := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for "tags:" header
		if trimmed == "tags:" {
			inTags = true
			continue
		}

		// Check for inline tags: tags: [tag1, tag2]
		if strings.HasPrefix(trimmed, "tags:") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "tags:"))
			if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
				inner := rest[1 : len(rest)-1]
				for _, tag := range strings.Split(inner, ",") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tagCounts[tag]++
					}
				}
			}
			inTags = false
			continue
		}

		// Parse block-style tag list items
		if inTags {
			if strings.HasPrefix(trimmed, "- ") {
				tag := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				if tag != "" {
					tagCounts[tag]++
				}
			} else if trimmed != "" {
				// No longer in tags block
				inTags = false
			}
		}
	}
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
		Version: "3.0.0",
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

	// Register create_note tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_note",
		Description: "Create a new note with YYYY-MM-DD_slug.md naming convention and YAML frontmatter. Supports tags and subfolder placement.",
	}, createNoteHandler)

	// Register update_note tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_note",
		Description: "Update an existing note's content. Supports 'replace' (overwrite) and 'append' modes.",
	}, updateNoteHandler)

	// Register delete_note tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_note",
		Description: "Delete a markdown note from the vault. Only .md files can be deleted.",
	}, deleteNoteHandler)

	// Register search_content tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_content",
		Description: "Full-text search across note bodies. Returns matching file paths, line numbers, and snippets. Supports regex patterns.",
	}, searchContentHandler)

	// Register get_backlinks tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_backlinks",
		Description: "Find all notes that link to a given note via [[wikilinks]]. Supports [[note]] and [[note|alias]] syntax.",
	}, getBacklinksHandler)

	// Register list_tags tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tags",
		Description: "List all tags found across the vault from YAML frontmatter and inline #tags. Optionally filter by prefix.",
	}, listTagsHandler)

	// Log server start to stderr (stdout is used for MCP communication)
	fmt.Fprintf(os.Stderr, "MCP Obsidian server starting...\n")
	fmt.Fprintf(os.Stderr, "Vault path: %s\n", vaultPath)

	// Run the server with stdio transport
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
