package render

import (
	"net/url"
	"strings"
	"testing"

	"markdown-browser/internal/fetch"
	"markdown-browser/internal/localfile"
	"markdown-browser/internal/page"
)

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestMarkdownBasics(t *testing.T) {
	src := "# Hello *World*\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n\n~~gone~~\n"
	out, title, err := Markdown([]byte(src), mustURL("https://example.com/"))
	if err != nil {
		t.Fatal(err)
	}
	if title != "Hello World" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"<table>", "<del>gone</del>", `type="checkbox"`, `id="hello-world"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestMarkdownStripsBOMAndUsesFirstH1(t *testing.T) {
	_, title, _ := Markdown([]byte("\xef\xbb\xbf# One\n\n# Two\n"), mustURL("https://example.com/"))
	if title != "One" {
		t.Errorf("title = %q", title)
	}
}

func TestMarkdownSanitizes(t *testing.T) {
	bad := []string{
		"<script>alert(1)</script>",
		`<img src=x onerror="alert(1)">`,
		`<a href="javascript:alert(1)" onclick="x()">x</a>`,
		"[x](javascript:alert(1))",
		"[x](data:text/html;base64,PHNjcmlwdD4=)",
		`<iframe src="https://evil.example"></iframe>`,
		`<style>body{display:none}</style>`,
	}
	for _, src := range bad {
		out, _, err := Markdown([]byte(src), mustURL("https://example.com/"))
		if err != nil {
			t.Fatal(err)
		}
		low := strings.ToLower(out)
		for _, needle := range []string{"<script", "onerror", "onclick", "javascript:", "data:", "<iframe", "<style"} {
			if strings.Contains(low, needle) {
				t.Errorf("source %q leaked %q: %s", src, needle, out)
			}
		}
	}
}

func TestMarkdownResolvesLinks(t *testing.T) {
	src := "[rel](../b.md) [abs](https://other.example/x) [frag](#sec) ![img](pic.png) ![file](file:///etc/passwd)"
	out, _, err := Markdown([]byte(src), mustURL("https://example.com/docs/a.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="https://example.com/b.md"`,
		`href="https://other.example/x"`,
		`href="#sec"`,
		`src="https://example.com/docs/pic.png"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "passwd") {
		t.Errorf("file image src not removed: %s", out)
	}
}

func TestMarkdownRelativeLinksFromFile(t *testing.T) {
	out, _, _ := Markdown([]byte("[n](notes.md)"), mustURL("file:///home/u/docs/index.md"))
	if !strings.Contains(out, `href="file:///home/u/docs/notes.md"`) {
		t.Errorf("got %s", out)
	}
}

func TestFallback(t *testing.T) {
	doc := `<html><head><title>
	  My   Page </title></head><body>
	<a href="/a">First</a>
	<a href="/a#frag">Dup</a>
	<a href="https://other.example/b"><b>Bold</b> link</a>
	<a href="#top">skip</a>
	<a href="mailto:x@y.z">skip</a>
	<a href="javascript:void(0)">skip</a>
	<a href="c.html"><img src="x.png"></a>
	<a>no href</a>
	</body></html>`
	title, links := Fallback([]byte(doc), "text/html; charset=utf-8", mustURL("https://example.com/dir/page"))
	if title != "My Page" {
		t.Errorf("title = %q", title)
	}
	want := []page.Link{
		{Text: "First", Href: "https://example.com/a"},
		{Text: "Bold link", Href: "https://other.example/b"},
		{Text: "https://example.com/dir/c.html", Href: "https://example.com/dir/c.html"},
	}
	if len(links) != len(want) {
		t.Fatalf("links = %+v", links)
	}
	for i := range want {
		if links[i] != want[i] {
			t.Errorf("links[%d] = %+v, want %+v", i, links[i], want[i])
		}
	}
}

func TestFallbackLatin1(t *testing.T) {
	title, _ := Fallback([]byte("<title>caf\xe9</title>"), "text/html; charset=iso-8859-1", mustURL("https://example.com/"))
	if title != "café" {
		t.Errorf("title = %q", title)
	}
}

func TestMarkdownLocalImages(t *testing.T) {
	out, _, _ := Markdown([]byte("![a](img/a.png) ![b](https://example.com/b.png)"), mustURL("file:///home/u/docs/index.md"))
	want := localfile.URL(fetch.FilePath(mustURL("file:///home/u/docs/img/a.png"))) // OS-specific path form
	if !strings.Contains(out, `src="`+want+`"`) {
		t.Errorf("local image not routed: %s", out)
	}
	if !strings.Contains(out, `src="https://example.com/b.png"`) {
		t.Errorf("remote image lost: %s", out)
	}
	// A remote page must not reach local files.
	out, _, _ = Markdown([]byte("![x](file:///home/u/secret.png)"), mustURL("https://example.com/"))
	if strings.Contains(out, "secret") {
		t.Errorf("remote page referenced a local file: %s", out)
	}
}
