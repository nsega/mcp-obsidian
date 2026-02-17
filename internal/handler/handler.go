package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/note"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

// Handler holds the vault reference and provides MCP tool handler methods
type Handler struct {
	Vault *vault.Vault
}

// New creates a new Handler with the given Vault
func New(v *vault.Vault) *Handler {
	return &Handler{Vault: v}
}

// SearchNotes implements the search_notes tool
func (h *Handler) SearchNotes(ctx context.Context, req *mcp.CallToolRequest, input note.SearchNotesInput) (*mcp.CallToolResult, note.SearchNotesOutput, error) {
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
	err := filepath.Walk(h.Vault.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Skip directories
		if info.IsDir() {
			// Check if directory is allowed (not hidden)
			allowed, checkErr := h.Vault.IsPathAllowed(path)
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
		allowed, checkErr := h.Vault.IsPathAllowed(path)
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

// ReadNotes implements the read_notes tool
func (h *Handler) ReadNotes(ctx context.Context, req *mcp.CallToolRequest, input note.ReadNotesInput) (*mcp.CallToolResult, note.ReadNotesOutput, error) {
	notes := make([]note.NoteContent, 0, len(input.Paths))

	for _, path := range input.Paths {
		n := note.NoteContent{
			Path: path,
		}

		// Check if path is allowed
		allowed, err := h.Vault.IsPathAllowed(path)
		if err != nil {
			n.Error = fmt.Sprintf("Path validation error: %v", err)
			notes = append(notes, n)
			continue
		}

		if !allowed {
			n.Error = "Access denied: path is outside vault or is a hidden file"
			notes = append(notes, n)
			continue
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			n.Error = fmt.Sprintf("Failed to read file: %v", err)
			notes = append(notes, n)
			continue
		}

		n.Content = string(content)
		notes = append(notes, n)
	}

	output := note.ReadNotesOutput{
		Notes: notes,
	}

	// Create text content for the result
	var textParts []string
	for _, n := range notes {
		if n.Error != "" {
			textParts = append(textParts, fmt.Sprintf("## %s\nError: %s", n.Path, n.Error))
		} else {
			textParts = append(textParts, fmt.Sprintf("## %s\n%s", n.Path, n.Content))
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

// CreateNote implements the create_note tool
func (h *Handler) CreateNote(ctx context.Context, req *mcp.CallToolRequest, input note.CreateNoteInput) (*mcp.CallToolResult, note.CreateNoteOutput, error) {
	if input.Title == "" {
		return nil, note.CreateNoteOutput{}, fmt.Errorf("title is required")
	}

	slug := note.Slugify(input.Title)
	if slug == "" {
		return nil, note.CreateNoteOutput{}, fmt.Errorf("title produces an empty slug")
	}

	today := time.Now().Format("2006-01-02")
	filename := today + "_" + slug + ".md"

	dir := h.Vault.Path
	if input.Folder != "" {
		dir = filepath.Join(h.Vault.Path, input.Folder)
	}

	fullPath := filepath.Join(dir, filename)

	allowed, err := h.Vault.IsPathAllowed(fullPath)
	if err != nil {
		return nil, note.CreateNoteOutput{}, fmt.Errorf("path validation error: %w", err)
	}
	if !allowed {
		return nil, note.CreateNoteOutput{}, fmt.Errorf("access denied: path is outside vault or is a hidden file")
	}

	if _, err := os.Stat(fullPath); err == nil {
		return nil, note.CreateNoteOutput{}, fmt.Errorf("file already exists: %s", fullPath)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, note.CreateNoteOutput{}, fmt.Errorf("failed to create directory: %w", err)
	}

	frontmatter := note.GenerateFrontmatter(input.Tags, today, today)

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
		return nil, note.CreateNoteOutput{}, fmt.Errorf("failed to write file: %w", err)
	}

	output := note.CreateNoteOutput{Path: fullPath}
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Created note: %s", fullPath)},
		},
	}
	return result, output, nil
}

// UpdateNote implements the update_note tool
func (h *Handler) UpdateNote(ctx context.Context, req *mcp.CallToolRequest, input note.UpdateNoteInput) (*mcp.CallToolResult, note.UpdateNoteOutput, error) {
	allowed, err := h.Vault.IsPathAllowed(input.Path)
	if err != nil {
		return nil, note.UpdateNoteOutput{}, fmt.Errorf("path validation error: %w", err)
	}
	if !allowed {
		return nil, note.UpdateNoteOutput{}, fmt.Errorf("access denied: path is outside vault or is a hidden file")
	}

	if _, err := os.Stat(input.Path); os.IsNotExist(err) {
		return nil, note.UpdateNoteOutput{}, fmt.Errorf("file does not exist: %s", input.Path)
	}

	mode := input.Mode
	if mode == "" {
		mode = "replace"
	}

	switch mode {
	case "replace":
		if err := os.WriteFile(input.Path, []byte(input.Content), 0644); err != nil {
			return nil, note.UpdateNoteOutput{}, fmt.Errorf("failed to write file: %w", err)
		}
	case "append":
		existing, err := os.ReadFile(input.Path)
		if err != nil {
			return nil, note.UpdateNoteOutput{}, fmt.Errorf("failed to read file: %w", err)
		}
		newContent := string(existing) + "\n\n" + input.Content
		if err := os.WriteFile(input.Path, []byte(newContent), 0644); err != nil {
			return nil, note.UpdateNoteOutput{}, fmt.Errorf("failed to write file: %w", err)
		}
	default:
		return nil, note.UpdateNoteOutput{}, fmt.Errorf("invalid mode: %s (must be 'replace' or 'append')", mode)
	}

	output := note.UpdateNoteOutput{Path: input.Path}
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Updated note (%s): %s", mode, input.Path)},
		},
	}
	return result, output, nil
}

// DeleteNote implements the delete_note tool
func (h *Handler) DeleteNote(ctx context.Context, req *mcp.CallToolRequest, input note.DeleteNoteInput) (*mcp.CallToolResult, note.DeleteNoteOutput, error) {
	if !strings.HasSuffix(strings.ToLower(input.Path), ".md") {
		return nil, note.DeleteNoteOutput{}, fmt.Errorf("can only delete .md files")
	}

	allowed, err := h.Vault.IsPathAllowed(input.Path)
	if err != nil {
		return nil, note.DeleteNoteOutput{}, fmt.Errorf("path validation error: %w", err)
	}
	if !allowed {
		return nil, note.DeleteNoteOutput{}, fmt.Errorf("access denied: path is outside vault or is a hidden file")
	}

	info, err := os.Stat(input.Path)
	if os.IsNotExist(err) {
		return nil, note.DeleteNoteOutput{}, fmt.Errorf("file does not exist: %s", input.Path)
	}
	if err != nil {
		return nil, note.DeleteNoteOutput{}, fmt.Errorf("failed to stat file: %w", err)
	}
	if info.IsDir() {
		return nil, note.DeleteNoteOutput{}, fmt.Errorf("cannot delete directories")
	}

	if err := os.Remove(input.Path); err != nil {
		return nil, note.DeleteNoteOutput{}, fmt.Errorf("failed to delete file: %w", err)
	}

	output := note.DeleteNoteOutput(input)
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Deleted note: %s", input.Path)},
		},
	}
	return result, output, nil
}

// SearchContent implements the search_content tool
func (h *Handler) SearchContent(ctx context.Context, req *mcp.CallToolRequest, input note.SearchContentInput) (*mcp.CallToolResult, note.SearchContentOutput, error) {
	query := input.Query

	var re *regexp.Regexp
	var useRegex bool
	if compiled, err := regexp.Compile("(?i)" + query); err == nil {
		re = compiled
		useRegex = true
	}

	var results []note.ContentMatch

	err := filepath.Walk(h.Vault.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			allowed, checkErr := h.Vault.IsPathAllowed(path)
			if checkErr != nil || !allowed {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		allowed, checkErr := h.Vault.IsPathAllowed(path)
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

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textContent},
		},
	}
	return result, output, nil
}

// GetBacklinks implements the get_backlinks tool
func (h *Handler) GetBacklinks(ctx context.Context, req *mcp.CallToolRequest, input note.GetBacklinksInput) (*mcp.CallToolResult, note.GetBacklinksOutput, error) {
	if input.NoteName == "" {
		return nil, note.GetBacklinksOutput{}, fmt.Errorf("note_name is required")
	}

	noteName := strings.TrimSuffix(input.NoteName, ".md")
	wikilinkRe := regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

	var results []note.Backlink

	err := filepath.Walk(h.Vault.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			allowed, checkErr := h.Vault.IsPathAllowed(path)
			if checkErr != nil || !allowed {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		allowed, checkErr := h.Vault.IsPathAllowed(path)
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

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textContent},
		},
	}
	return result, output, nil
}

// ListTags implements the list_tags tool
func (h *Handler) ListTags(ctx context.Context, req *mcp.CallToolRequest, input note.ListTagsInput) (*mcp.CallToolResult, note.ListTagsOutput, error) {
	tagCounts := make(map[string]int)

	// Regex for inline #tags (not inside code blocks)
	inlineTagRe := regexp.MustCompile(`(?:^|\s)#([a-zA-Z][a-zA-Z0-9_/-]*)`)

	err := filepath.Walk(h.Vault.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			allowed, checkErr := h.Vault.IsPathAllowed(path)
			if checkErr != nil || !allowed {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		allowed, checkErr := h.Vault.IsPathAllowed(path)
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

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textContent},
		},
	}
	return result, output, nil
}
