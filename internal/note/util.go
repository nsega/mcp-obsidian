package note

import (
	"strings"
	"unicode"
)

// Slugify converts a title string into a URL/filename-safe slug
func Slugify(s string) string {
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

// GenerateFrontmatter creates YAML frontmatter with tags and timestamps
func GenerateFrontmatter(tags []string, created, updated string) string {
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

// ParseFrontmatterTags parses tags from YAML frontmatter content (without delimiters)
func ParseFrontmatterTags(frontmatter string, tagCounts map[string]int) {
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
