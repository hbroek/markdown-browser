package page

import (
	"strings"
	"unicode"
)

// Markdown is the page as a markdown document for saving: the page's own source
// when it is markdown, otherwise a document with its title, address and links.
func (p Page) Markdown() string {
	if p.Kind == KindMarkdown {
		return p.Source
	}
	var b strings.Builder
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = p.URL
	}
	b.WriteString("# " + oneLine(title) + "\n\n")
	if p.URL != "" {
		b.WriteString("Source: <" + p.URL + ">\n\n")
	}
	if len(p.Links) > 0 {
		b.WriteString("## Links\n\n")
	}
	seen := map[string]bool{}
	for _, l := range p.Links {
		if l.Href == "" || seen[l.Href] {
			continue
		}
		seen[l.Href] = true
		text := oneLine(l.Text)
		if text == "" {
			text = l.Href
		}
		b.WriteString("- [" + strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(text) + "](<" + l.Href + ">)\n")
	}
	return b.String()
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// FileName suggests a file name for saving the page: its title as a slug, plus .md.
func (p Page) FileName() string {
	name := strings.TrimSpace(p.Title)
	if name == "" {
		name = "page"
	}
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if r := []rune(slug); len(r) > 60 {
		slug = strings.Trim(string(r[:60]), "-")
	}
	if slug == "" {
		slug = "page"
	}
	return slug + ".md"
}
