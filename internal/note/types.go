package note

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
