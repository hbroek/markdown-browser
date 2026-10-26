package bookmarks_test

import (
	"strings"
	"testing"
	"time"

	"markdown-browser/internal/bookmarks"
	"markdown-browser/internal/bookmarks/storetest"
)

func TestMemoryStore(t *testing.T) {
	storetest.Run(t, func(*testing.T) bookmarks.Store { return bookmarks.NewMemory() })
}

func TestRegistry(t *testing.T) {
	s, err := bookmarks.Open("memory", nil)
	if err != nil || s == nil {
		t.Fatalf("Open(memory) = %v, %v", s, err)
	}
	if _, err := bookmarks.Open("nonesuch", nil); err == nil {
		t.Error("unknown backend should fail")
	}
	found := false
	for _, n := range bookmarks.Names() {
		found = found || n == "memory"
	}
	if !found {
		t.Errorf("Names() = %v", bookmarks.Names())
	}
}

func TestKey(t *testing.T) {
	cases := map[string]string{
		"https://Example.com":         "https://example.com/",
		"HTTPS://example.com/a#frag":  "https://example.com/a",
		" https://example.com/a?q=1 ": "https://example.com/a?q=1",
		"file:///tmp/a.md":            "file:///tmp/a.md",
	}
	for in, want := range cases {
		if got := bookmarks.Key(in); got != want {
			t.Errorf("Key(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSaveUnique(t *testing.T) {
	ctx := t.Context()
	s := bookmarks.NewMemory()
	a, err := bookmarks.SaveUnique(ctx, s, bookmarks.Bookmark{Title: "One", URL: "https://x.test/a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := bookmarks.SaveUnique(ctx, s, bookmarks.Bookmark{Title: "Renamed", URL: "https://X.test/a#top"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != a.ID || b.Title != "Renamed" {
		t.Errorf("same page should update: %+v then %+v", a, b)
	}
	if list, _ := s.Bookmarks(ctx, ""); len(list) != 1 {
		t.Errorf("%d bookmarks, want 1", len(list))
	}
	if c, _ := bookmarks.SaveUnique(ctx, s, bookmarks.Bookmark{Title: "Other", URL: "https://x.test/b"}); c.ID == a.ID {
		t.Error("different page reused an ID")
	}
}

func TestSearchMarkdown(t *testing.T) {
	folders := []bookmarks.Folder{{ID: "1", Name: "Dev"}, {ID: "2", ParentID: "1", Name: "Web"}}
	results := []bookmarks.Bookmark{
		{Title: "A [odd] *title*", URL: "https://a.test/x y", FolderID: "2", Keywords: []string{"go", "web"}, Description: "Some\nnotes"},
		{Title: "", URL: "https://b.test/"},
	}
	visits := []bookmarks.Visit{
		{Title: "Already saved", URL: "https://B.test/#top"}, // same page as a bookmark: dropped
		{Title: "Only visited", URL: "https://c.test/", Time: time.Date(2026, 10, 25, 14, 3, 0, 0, time.Local)},
		{Title: "Only visited again", URL: "https://c.test/"}, // same page twice: one line
	}
	md := bookmarks.SearchMarkdown("rust_async", results, visits, folders)
	for _, want := range []string{
		"# Results for \u201crust\\_async\u201d",
		"2 bookmarks, 1 page from history.",
		`- [A \[odd\] \*title\*](<https://a.test/x%20y>)`,
		"_Bookmarks / Dev / Web_ \u00B7 go, web",
		"Some notes",
		"- [https://b.test/](<https://b.test/>)  \n  _Bookmarks_",
		"- [Only visited](<https://c.test/>)  \n  _History \u00B7 25 Oct 2026 14:03_",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	for _, unwanted := range []string{"Already saved", "Only visited again"} {
		if strings.Contains(md, unwanted) {
			t.Errorf("%q should have been merged away:\n%s", unwanted, md)
		}
	}
	if !strings.Contains(bookmarks.SearchMarkdown("", nil, nil, nil), "# All bookmarks") {
		t.Error("empty query heading")
	}
	if !strings.Contains(bookmarks.SearchMarkdown("zzz", nil, nil, nil), "Nothing found") {
		t.Error("empty results message")
	}
	if md := bookmarks.SearchMarkdown("x", nil, visits[1:2], nil); !strings.Contains(md, "0 bookmarks, 1 page from history.") {
		t.Errorf("history-only results:\n%s", md)
	}
}
