// Package storetest is the conformance suite every bookmarks.Store backend
// must pass. Call Run from the backend's tests with a factory for a fresh,
// empty store.
package storetest

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"markdown-browser/internal/bookmarks"
)

var ctx = context.Background()

// Run executes the suite; each subtest gets its own store from open.
func Run(t *testing.T, open func(t *testing.T) bookmarks.Store) {
	cases := []struct {
		name string
		fn   func(t *testing.T, s bookmarks.Store)
	}{
		{"BookmarkCRUD", bookmarkCRUD},
		{"Validation", validation},
		{"FolderHierarchy", folderHierarchy},
		{"FolderCycleRejected", folderCycle},
		{"MissingParents", missingParents},
		{"MoveBookmark", moveBookmark},
		{"DeleteFolderCascades", deleteFolderCascades},
		{"FindByURL", findByURL},
		{"Search", search},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := open(t)
			t.Cleanup(func() { s.Close() })
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("unexpected error: %v", r)
				}
			}()
			c.fn(t, s)
		})
	}
}

// must unwraps a (value, error) call; a failure panics and Run reports it
// as a test failure for the subtest.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func folder(t *testing.T, s bookmarks.Store, parent, name string) bookmarks.Folder {
	t.Helper()
	return must(mustFolder(s, parent, name))
}

func mustFolder(s bookmarks.Store, parent, name string) (bookmarks.Folder, error) {
	return s.SaveFolder(ctx, bookmarks.Folder{ParentID: parent, Name: name})
}

func mark(t *testing.T, s bookmarks.Store, folder, title, url string, keywords ...string) bookmarks.Bookmark {
	t.Helper()
	return must(s.SaveBookmark(ctx, bookmarks.Bookmark{FolderID: folder, Title: title, URL: url, Keywords: keywords}))
}

func titles(list []bookmarks.Bookmark) []string {
	out := []string{}
	for _, b := range list {
		out = append(out, b.Title)
	}
	return out
}

func expectTitles(t *testing.T, got []bookmarks.Bookmark, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if g := titles(got); !reflect.DeepEqual(g, want) {
		t.Errorf("titles = %v, want %v", g, want)
	}
}

func bookmarkCRUD(t *testing.T, s bookmarks.Store) {
	b := must(s.SaveBookmark(ctx, bookmarks.Bookmark{
		Title: "Go", URL: "https://go.dev/", Description: "The Go site", Keywords: []string{"lang", "go"},
	}))
	if b.ID == "" || b.Created.IsZero() || b.Updated.IsZero() {
		t.Fatalf("created bookmark missing ID/timestamps: %+v", b)
	}
	got := must(s.Bookmark(ctx, b.ID))
	if got.Title != "Go" || got.URL != "https://go.dev/" || got.Description != "The Go site" ||
		!reflect.DeepEqual(got.Keywords, []string{"lang", "go"}) {
		t.Errorf("round trip = %+v", got)
	}

	b.Title = "The Go Programming Language"
	b.Created = got.Created.AddDate(-1, 0, 0) // callers cannot rewrite history
	up := must(s.SaveBookmark(ctx, b))
	if up.ID != b.ID {
		t.Errorf("update changed ID: %q -> %q", b.ID, up.ID)
	}
	got = must(s.Bookmark(ctx, b.ID))
	if got.Title != "The Go Programming Language" {
		t.Errorf("title = %q", got.Title)
	}
	if !got.Created.Equal(up.Created) || got.Updated.Before(got.Created) {
		t.Errorf("timestamps: created %v updated %v", got.Created, got.Updated)
	}
	if list := must(s.Bookmarks(ctx, "")); len(list) != 1 {
		t.Errorf("root has %d bookmarks, want 1", len(list))
	}

	if err := s.DeleteBookmark(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bookmark(ctx, b.ID); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("Bookmark after delete: %v", err)
	}
	if err := s.DeleteBookmark(ctx, b.ID); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if _, err := s.SaveBookmark(ctx, bookmarks.Bookmark{ID: "nope", URL: "https://x.test/"}); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("update of missing bookmark: %v", err)
	}
}

func validation(t *testing.T, s bookmarks.Store) {
	for _, u := range []string{"", "   ", "go.dev", "ftp://x.test/", "javascript:alert(1)", "https://"} {
		if _, err := s.SaveBookmark(ctx, bookmarks.Bookmark{Title: "x", URL: u}); !errors.Is(err, bookmarks.ErrInvalid) {
			t.Errorf("URL %q: err = %v, want ErrInvalid", u, err)
		}
	}
	b := mark(t, s, "", "  ", " https://a.test/ ", " x ", "", "X", "y")
	if b.Title != "https://a.test/" {
		t.Errorf("empty title should default to URL, got %q", b.Title)
	}
	if b.URL != "https://a.test/" {
		t.Errorf("URL not trimmed: %q", b.URL)
	}
	if !reflect.DeepEqual(b.Keywords, []string{"x", "y"}) {
		t.Errorf("keywords = %q, want [x y]", b.Keywords)
	}
	if _, err := s.SaveFolder(ctx, bookmarks.Folder{Name: "  "}); !errors.Is(err, bookmarks.ErrInvalid) {
		t.Errorf("blank folder name: %v", err)
	}
	if b := mark(t, s, "", "Local", "file:///tmp/a.md"); b.URL != "file:///tmp/a.md" {
		t.Errorf("file URL = %q", b.URL)
	}
}

func folderHierarchy(t *testing.T, s bookmarks.Store) {
	dev := folder(t, s, "", "Dev")
	web := folder(t, s, dev.ID, "Web")
	folder(t, s, "", "Cooking")

	got := must(s.Folders(ctx))
	if len(got) != 3 || got[0].Name != "Cooking" || got[1].Name != "Dev" || got[2].Name != "Web" {
		t.Fatalf("Folders = %+v", got)
	}
	if got[2].ParentID != dev.ID {
		t.Errorf("Web parent = %q, want %q", got[2].ParentID, dev.ID)
	}

	web.Name = "Frontend"
	web = must(s.SaveFolder(ctx, web))
	if web.Name != "Frontend" || web.ParentID != dev.ID {
		t.Errorf("rename = %+v", web)
	}
	web.ParentID = "" // move to root
	must(s.SaveFolder(ctx, web))
	for _, f := range must(s.Folders(ctx)) {
		if f.ID == web.ID && f.ParentID != "" {
			t.Errorf("folder not moved to root: %+v", f)
		}
	}

	mark(t, s, dev.ID, "B", "https://b.test/")
	mark(t, s, dev.ID, "a", "https://a.test/")
	mark(t, s, "", "Root", "https://r.test/")
	expectTitles(t, must(s.Bookmarks(ctx, dev.ID)), "a", "B")
	expectTitles(t, must(s.Bookmarks(ctx, "")), "Root")
}

func folderCycle(t *testing.T, s bookmarks.Store) {
	a := folder(t, s, "", "A")
	b := folder(t, s, a.ID, "B")
	c := folder(t, s, b.ID, "C")

	a.ParentID = c.ID
	if _, err := s.SaveFolder(ctx, a); !errors.Is(err, bookmarks.ErrInvalid) {
		t.Errorf("move A under its descendant: %v", err)
	}
	b.ParentID = b.ID
	if _, err := s.SaveFolder(ctx, b); !errors.Is(err, bookmarks.ErrInvalid) {
		t.Errorf("move B under itself: %v", err)
	}
}

func missingParents(t *testing.T, s bookmarks.Store) {
	if _, err := mustFolder(s, "nope", "x"); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("folder with missing parent: %v", err)
	}
	if _, err := s.SaveBookmark(ctx, bookmarks.Bookmark{FolderID: "nope", URL: "https://x.test/"}); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("bookmark in missing folder: %v", err)
	}
	if _, err := s.Bookmarks(ctx, "nope"); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("Bookmarks of missing folder: %v", err)
	}
	if err := s.DeleteFolder(ctx, "nope"); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("DeleteFolder missing: %v", err)
	}
	if _, err := s.SaveFolder(ctx, bookmarks.Folder{ID: "nope", Name: "x"}); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("update of missing folder: %v", err)
	}
}

func moveBookmark(t *testing.T, s bookmarks.Store) {
	a := folder(t, s, "", "A")
	b := folder(t, s, "", "B")
	bm := mark(t, s, a.ID, "Page", "https://p.test/")
	bm.FolderID = b.ID
	must(s.SaveBookmark(ctx, bm))
	expectTitles(t, must(s.Bookmarks(ctx, a.ID)))
	expectTitles(t, must(s.Bookmarks(ctx, b.ID)), "Page")
}

func deleteFolderCascades(t *testing.T, s bookmarks.Store) {
	keep := folder(t, s, "", "Keep")
	top := folder(t, s, "", "Top")
	sub := folder(t, s, top.ID, "Sub")
	mark(t, s, top.ID, "In top", "https://1.test/")
	deep := mark(t, s, sub.ID, "In sub", "https://2.test/")
	mark(t, s, keep.ID, "Kept", "https://3.test/")

	if err := s.DeleteFolder(ctx, top.ID); err != nil {
		t.Fatal(err)
	}
	folders := must(s.Folders(ctx))
	if len(folders) != 1 || folders[0].ID != keep.ID {
		t.Errorf("folders after delete = %+v", folders)
	}
	if _, err := s.Bookmark(ctx, deep.ID); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("nested bookmark survived: %v", err)
	}
	expectTitles(t, must(s.Search(ctx, []string{"test"})), "Kept")
}

func findByURL(t *testing.T, s bookmarks.Store) {
	bm := mark(t, s, "", "Docs", "https://Example.COM/docs")
	for _, u := range []string{"https://example.com/docs", "https://example.com/docs#intro", "  https://EXAMPLE.com/docs "} {
		if got, err := s.FindByURL(ctx, u); err != nil || got.ID != bm.ID {
			t.Errorf("FindByURL(%q) = %+v, %v", u, got, err)
		}
	}
	mark(t, s, "", "Home", "https://example.com")
	if got, err := s.FindByURL(ctx, "https://example.com/"); err != nil || got.Title != "Home" {
		t.Errorf("empty path should match %q: %+v, %v", "/", got, err)
	}
	for _, u := range []string{"https://example.com/other", "http://example.com/docs", "https://example.com/docs?x=1"} {
		if _, err := s.FindByURL(ctx, u); !errors.Is(err, bookmarks.ErrNotFound) {
			t.Errorf("FindByURL(%q): %v, want ErrNotFound", u, err)
		}
	}
}

func search(t *testing.T, s bookmarks.Store) {
	f := folder(t, s, "", "Dev")
	_ = must(s.SaveBookmark(ctx, bookmarks.Bookmark{FolderID: f.ID, Title: "Rust Book", URL: "https://doc.rust-lang.org/book/", Keywords: []string{"rust", "learning"}}))
	_ = must(s.SaveBookmark(ctx, bookmarks.Bookmark{Title: "Tokio", URL: "https://tokio.rs/", Description: "Async runtime for Rust", Keywords: []string{"async"}}))
	_ = must(s.SaveBookmark(ctx, bookmarks.Bookmark{Title: "Go Blog", URL: "https://go.dev/blog/", Keywords: []string{"go", "async"}}))

	cases := []struct {
		terms []string
		want  []string
	}{
		{[]string{"rust"}, []string{"Rust Book", "Tokio"}}, // title, keyword, description
		{[]string{"RUST", "async"}, []string{"Tokio"}},     // case-insensitive, all terms
		{[]string{"async"}, []string{"Go Blog", "Tokio"}},  // keyword, sorted by title
		{[]string{"go.dev"}, []string{"Go Blog"}},          // URL
		{[]string{"learn"}, []string{"Rust Book"}},         // substring
		{[]string{"rust", "go"}, []string{}},               // no bookmark has both
		{[]string{"nothing-matches"}, []string{}},
	}
	for _, c := range cases {
		expectTitles(t, must(s.Search(ctx, c.terms)), c.want...)
	}
}
