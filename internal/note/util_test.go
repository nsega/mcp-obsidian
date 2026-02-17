package note

import (
	"testing"
)

// TestSlugify tests the Slugify helper function
func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"GTD Zettelkasten Flowchart", "gtd-zettelkasten-flowchart"},
		{"  spaces  everywhere  ", "spaces-everywhere"},
		{"special!@#chars$%^&*()", "special-chars"},
		{"already-slugified", "already-slugified"},
		{"UPPERCASE", "uppercase"},
		{"multiple---hyphens", "multiple-hyphens"},
		{"日本語テスト", "日本語テスト"},
		{"mix 日本語 and English", "mix-日本語-and-english"},
		{"", ""},
		{"---", ""},
		{"123 numbers", "123-numbers"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestGenerateFrontmatter tests the GenerateFrontmatter helper function
func TestGenerateFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		created string
		updated string
		want    string
		tags    []string
	}{
		{
			name:    "with tags",
			tags:    []string{"zettelkasten", "productivity"},
			created: "2026-02-15",
			updated: "2026-02-15",
			want:    "---\ntags:\n  - zettelkasten\n  - productivity\ncreated: 2026-02-15\nupdated: 2026-02-15\n---\n",
		},
		{
			name:    "without tags",
			tags:    nil,
			created: "2026-01-01",
			updated: "2026-01-01",
			want:    "---\ncreated: 2026-01-01\nupdated: 2026-01-01\n---\n",
		},
		{
			name:    "empty tags slice",
			tags:    []string{},
			created: "2026-03-01",
			updated: "2026-03-01",
			want:    "---\ncreated: 2026-03-01\nupdated: 2026-03-01\n---\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateFrontmatter(tt.tags, tt.created, tt.updated)
			if got != tt.want {
				t.Errorf("GenerateFrontmatter() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseFrontmatterTags tests the frontmatter tag parsing helper
func TestParseFrontmatterTags(t *testing.T) {
	tests := []struct {
		wantTags    map[string]int
		name        string
		frontmatter string
	}{
		{
			name:        "block style tags",
			frontmatter: "tags:\n  - alpha\n  - beta\ncreated: 2026-01-01",
			wantTags:    map[string]int{"alpha": 1, "beta": 1},
		},
		{
			name:        "inline style tags",
			frontmatter: "tags: [foo, bar, baz]\ncreated: 2026-01-01",
			wantTags:    map[string]int{"foo": 1, "bar": 1, "baz": 1},
		},
		{
			name:        "no tags field",
			frontmatter: "created: 2026-01-01\nupdated: 2026-01-01",
			wantTags:    map[string]int{},
		},
		{
			name:        "empty tags block",
			frontmatter: "tags:\ncreated: 2026-01-01",
			wantTags:    map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagCounts := make(map[string]int)
			ParseFrontmatterTags(tt.frontmatter, tagCounts)
			for tag, wantCount := range tt.wantTags {
				if got := tagCounts[tag]; got != wantCount {
					t.Errorf("tag %q count = %d, want %d", tag, got, wantCount)
				}
			}
			if len(tagCounts) != len(tt.wantTags) {
				t.Errorf("got %d tags, want %d. Tags: %v", len(tagCounts), len(tt.wantTags), tagCounts)
			}
		})
	}
}
