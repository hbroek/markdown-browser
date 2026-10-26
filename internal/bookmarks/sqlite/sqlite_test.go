package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"markdown-browser/internal/bookmarks"
	"markdown-browser/internal/bookmarks/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) bookmarks.Store {
		s, err := Open(filepath.Join(t.TempDir(), "bookmarks.db"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestPersistsAcrossOpens(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "bookmarks.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := s.SaveFolder(ctx, bookmarks.Folder{Name: "Dev"})
	if _, err := s.SaveBookmark(ctx, bookmarks.Bookmark{FolderID: f.ID, Title: "Go", URL: "https://go.dev/", Keywords: []string{"lang"}}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path) // migrations must be a no-op the second time
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.Bookmarks(ctx, f.ID)
	if err != nil || len(list) != 1 || list[0].Title != "Go" || len(list[0].Keywords) != 1 {
		t.Fatalf("reopened bookmarks = %+v, %v", list, err)
	}
}

func TestRegistered(t *testing.T) {
	s, err := bookmarks.Open("sqlite", bookmarks.Config{"path": ":memory:"}, bookmarks.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := bookmarks.Open("sqlite", nil, bookmarks.Deps{}); err == nil {
		t.Error("missing path should fail")
	}
}

func TestOpenCreatesMissingFolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not", "yet", "there", "bookmarks.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open in a missing folder: %v", err)
	}
	s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database not created: %v", err)
	}
}
