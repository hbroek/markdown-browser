package bookmarks_test

import (
	"reflect"
	"strings"
	"testing"

	"markdown-browser/internal/bookmarks"
)

func parseMD(t *testing.T, src string) *bookmarks.ImportFolder {
	t.Helper()
	root, err := bookmarks.ParseMarkdown(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseMarkdownHeadingsAndLists(t *testing.T) {
	root := parseMD(t, "# My links\n\n"+
		"- [Top](https://top.test/)\n"+
		"## Dev\n\n"+
		"- [Go](https://go.dev/) - The Go site\n"+
		"  Keywords: lang, go\n"+
		"- remember to read more\n"+ // plain text: not a bookmark, not a folder
		"- https://bare.test/\n"+
		"### Web\n"+
		"1. [Rust](<https://rust-lang.org/a b>) — Systems\n"+
		"## Ops\n"+
		"```\n- [Not](https://code.test/)\n```\n"+
		"- <https://auto.test/x>\n")
	if len(root.Bookmarks) != 1 || root.Bookmarks[0].Title != "Top" {
		t.Errorf("root = %+v", root.Bookmarks)
	}
	if len(root.Folders) != 2 || root.Folders[0].Name != "Dev" || root.Folders[1].Name != "Ops" {
		t.Fatalf("folders = %+v", root.Folders)
	}
	dev := root.Folders[0]
	if len(dev.Bookmarks) != 2 {
		t.Fatalf("dev bookmarks = %+v", dev.Bookmarks)
	}
	if g := dev.Bookmarks[0]; g.Title != "Go" || g.Description != "The Go site" || !reflect.DeepEqual(g.Keywords, []string{"lang", "go"}) {
		t.Errorf("go = %+v", g)
	}
	if b := dev.Bookmarks[1]; b.URL != "https://bare.test/" || b.Title != "" {
		t.Errorf("bare = %+v", b)
	}
	if len(dev.Folders) != 1 || dev.Folders[0].Name != "Web" || dev.Folders[0].Bookmarks[0].URL != "https://rust-lang.org/a b" || dev.Folders[0].Bookmarks[0].Description != "Systems" {
		t.Errorf("web = %+v", dev.Folders)
	}
	if ops := root.Folders[1]; len(ops.Bookmarks) != 1 || ops.Bookmarks[0].URL != "https://auto.test/x" {
		t.Errorf("ops = %+v", ops.Bookmarks) // the fenced example must be ignored
	}
}

func TestParseMarkdownNestedFoldersAndMultipleH1(t *testing.T) {
	root := parseMD(t, "# Dev\n- **Web**\n  - [A](https://a.test/)\n- Tools\n  - [B](https://b.test/)\n# Home\n- [C](https://c.test/)\n")
	// Several "#" headings are folders, not a title.
	if len(root.Folders) != 2 || root.Folders[0].Name != "Dev" || root.Folders[1].Name != "Home" {
		t.Fatalf("folders = %+v", root.Folders)
	}
	dev := root.Folders[0]
	if len(dev.Folders) != 2 || dev.Folders[0].Name != "Web" || dev.Folders[1].Name != "Tools" { // "Tools" has nested items
		t.Fatalf("dev folders = %+v", dev.Folders)
	}
	if dev.Folders[0].Bookmarks[0].Title != "A" || dev.Folders[1].Bookmarks[0].Title != "B" {
		t.Errorf("nested bookmarks = %+v", dev.Folders)
	}
	if root.Folders[1].Bookmarks[0].Title != "C" {
		t.Errorf("home = %+v", root.Folders[1])
	}
}

func TestMarkdownExportRoundTrip(t *testing.T) {
	ctx := t.Context()
	src := bookmarks.NewMemory()
	dev, _ := src.SaveFolder(ctx, bookmarks.Folder{Name: "Dev *& Ops*"})
	web, _ := src.SaveFolder(ctx, bookmarks.Folder{ParentID: dev.ID, Name: "Web"})
	src.SaveFolder(ctx, bookmarks.Folder{Name: "Empty"})
	src.SaveBookmark(ctx, bookmarks.Bookmark{FolderID: web.ID, Title: "A [odd] *title* — x", URL: "https://a.test/?x=1&y=2 z",
		Description: "Line one & <two>\nsecond", Keywords: []string{"go", "a,b", "under_score"}})
	src.SaveBookmark(ctx, bookmarks.Bookmark{Title: "Top", URL: "https://top.test/"})

	out, err := bookmarks.ExportMarkdown(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "# Bookmarks\n") {
		t.Errorf("header: %.40s", out)
	}

	dst := bookmarks.NewMemory()
	root, err := bookmarks.ParseMarkdown(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	res, err := bookmarks.Import(ctx, dst, root)
	if err != nil || res.Bookmarks != 2 || res.Folders != 3 {
		t.Fatalf("import = %+v, %v\n%s", res, err, out)
	}
	got, err := dst.FindByURL(ctx, "https://a.test/?x=1&y=2%20z")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got.Title != "A [odd] *title* — x" || got.Description != "Line one & <two> second" ||
		!reflect.DeepEqual(got.Keywords, []string{"go", "a b", "under_score"}) {
		t.Errorf("round trip lost data: %+v\n%s", got, out)
	}
	folders, _ := dst.Folders(ctx)
	byName := map[string]bookmarks.Folder{}
	for _, f := range folders {
		byName[f.Name] = f
	}
	if f := byName["Web"]; f.ParentID != byName["Dev *& Ops*"].ID || f.ID != got.FolderID {
		t.Errorf("folder nesting lost: %+v", folders)
	}
	if _, ok := byName["Empty"]; !ok {
		t.Errorf("empty folder lost: %+v", folders)
	}
}
