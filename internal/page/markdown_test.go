package page

import (
	"strings"
	"testing"
)

func TestMarkdownOfAFallbackPage(t *testing.T) {
	p := Page{Kind: KindFallback, URL: "https://x.test/a", Title: "My [page]", Links: []Link{
		{Text: "One\n thing", Href: "https://x.test/1"},
		{Text: "dup", Href: "https://x.test/1"},
		{Text: "", Href: "https://x.test/2"},
		{Text: "[x]", Href: "https://x.test/3"},
	}}
	got := p.Markdown()
	for _, want := range []string{"# My [page]\n", "Source: <https://x.test/a>", "- [One thing](<https://x.test/1>)", "- [https://x.test/2](<https://x.test/2>)", `- [\[x\]](<https://x.test/3>)`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Count(got, "https://x.test/1") != 1 {
		t.Errorf("a repeated link was listed twice:\n%s", got)
	}
}

func TestMarkdownPageKeepsItsSource(t *testing.T) {
	if got := (Page{Kind: KindMarkdown, Source: "# Hi\n"}).Markdown(); got != "# Hi\n" {
		t.Errorf("got %q", got)
	}
}

func TestFileName(t *testing.T) {
	for title, want := range map[string]string{
		"The Big Undo!":  "the-big-undo.md",
		"  ":             "page.md",
		"Über / Café":    "über-café.md",
		"../../etc/pass": "etc-pass.md",
	} {
		if got := (Page{Title: title}).FileName(); got != want {
			t.Errorf("FileName(%q) = %q, want %q", title, got, want)
		}
	}
}
