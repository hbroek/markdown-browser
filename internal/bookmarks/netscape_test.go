package bookmarks_test

import (
	"reflect"
	"strings"
	"testing"

	"markdown-browser/internal/bookmarks"
)

// A trimmed Chrome-style export: uppercase tags, unclosed <DT>/<p>, entities.
const chromeFile = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3 ADD_DATE="1700000000" PERSONAL_TOOLBAR_FOLDER="true">Bookmarks bar</H3>
    <DL><p>
        <DT><A HREF="https://go.dev/" ADD_DATE="1700000001" TAGS="lang, go">The Go &amp; Co</A>
        <DD>Official &lt;site&gt;
        <DT><H3>Dev</H3>
        <DL><p>
            <DT><A HREF="https://rust-lang.org/">Rust</A>
        </DL><p>
        <DT><A HREF="javascript:alert(1)">Bookmarklet</A>
    </DL><p>
    <DT><A HREF="https://top.test/">Top level</A>
</DL><p>
`

func TestParseNetscape(t *testing.T) {
	root, err := bookmarks.ParseNetscape(strings.NewReader(chromeFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Folders) != 1 || root.Folders[0].Name != "Bookmarks bar" {
		t.Fatalf("root folders = %+v", root.Folders)
	}
	if len(root.Bookmarks) != 1 || root.Bookmarks[0].Title != "Top level" {
		t.Errorf("root bookmarks = %+v", root.Bookmarks)
	}
	bar := root.Folders[0]
	if len(bar.Bookmarks) != 2 || len(bar.Folders) != 1 || bar.Folders[0].Name != "Dev" {
		t.Fatalf("bar = %+v", bar)
	}
	g := bar.Bookmarks[0]
	if g.Title != "The Go & Co" || g.URL != "https://go.dev/" || g.Description != "Official <site>" ||
		!reflect.DeepEqual(g.Keywords, []string{"lang", " go"}) {
		t.Errorf("go bookmark = %+v", g)
	}
	if dev := bar.Folders[0]; len(dev.Bookmarks) != 1 || dev.Bookmarks[0].URL != "https://rust-lang.org/" {
		t.Errorf("dev = %+v", dev)
	}
}

func TestImportMergesFoldersAndSkipsDuplicates(t *testing.T) {
	ctx := t.Context()
	s := bookmarks.NewMemory()
	existing, _ := s.SaveFolder(ctx, bookmarks.Folder{Name: "bookmarks BAR"}) // matches case-insensitively
	s.SaveBookmark(ctx, bookmarks.Bookmark{FolderID: existing.ID, Title: "Mine", URL: "https://go.dev"})

	root, _ := bookmarks.ParseNetscape(strings.NewReader(chromeFile))
	res, err := bookmarks.Import(ctx, s, root)
	if err != nil {
		t.Fatal(err)
	}
	want := bookmarks.ImportResult{Folders: 1, Bookmarks: 2, Duplicates: 1, Skipped: 1}
	if res != want {
		t.Errorf("result = %+v, want %+v", res, want)
	}
	if got, _ := s.FindByURL(ctx, "https://go.dev/"); got.Title != "Mine" {
		t.Errorf("duplicate overwrote the existing bookmark: %+v", got)
	}
	if folders, _ := s.Folders(ctx); len(folders) != 2 { // the existing one plus Dev
		t.Errorf("folders = %+v", folders)
	}

	again, _ := bookmarks.Import(ctx, s, root) // importing twice adds nothing
	if again.Bookmarks != 0 || again.Folders != 0 {
		t.Errorf("second import = %+v", again)
	}
}

func TestExportRoundTrip(t *testing.T) {
	ctx := t.Context()
	src := bookmarks.NewMemory()
	dev, _ := src.SaveFolder(ctx, bookmarks.Folder{Name: `Dev & "Ops"`})
	web, _ := src.SaveFolder(ctx, bookmarks.Folder{ParentID: dev.ID, Name: "Web"})
	src.SaveBookmark(ctx, bookmarks.Bookmark{FolderID: web.ID, Title: "A <b> & \"c\"", URL: "https://a.test/?x=1&y=2",
		Description: "Line one & <two>", Keywords: []string{"go", "a,b"}})
	src.SaveBookmark(ctx, bookmarks.Bookmark{Title: "Top", URL: "https://top.test/"})

	out, err := bookmarks.ExportNetscape(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "<!DOCTYPE NETSCAPE-Bookmark-file-1>") {
		t.Errorf("header: %.60s", out)
	}

	dst := bookmarks.NewMemory()
	root, err := bookmarks.ParseNetscape(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	res, err := bookmarks.Import(ctx, dst, root)
	if err != nil || res.Bookmarks != 2 || res.Folders != 2 {
		t.Fatalf("import = %+v, %v", res, err)
	}
	got, err := dst.FindByURL(ctx, "https://a.test/?x=1&y=2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != `A <b> & "c"` || got.Description != "Line one & <two>" || !reflect.DeepEqual(got.Keywords, []string{"go", "a b"}) {
		t.Errorf("round trip lost data: %+v", got)
	}
	folders, _ := dst.Folders(ctx)
	byName := map[string]bookmarks.Folder{}
	for _, f := range folders {
		byName[f.Name] = f
	}
	if f := byName["Web"]; f.ParentID != byName[`Dev & "Ops"`].ID || f.ID != got.FolderID {
		t.Errorf("folder nesting lost: %+v", folders)
	}
}
