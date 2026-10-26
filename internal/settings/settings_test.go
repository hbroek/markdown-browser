package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	dir := t.TempDir()
	s := Load(filepath.Join(dir, "settings.json"), dir)
	if s.Bookmarks.Backend != "sqlite" || s.Bookmarks.Config["path"] != filepath.Join(dir, "bookmarks.db") {
		t.Errorf("defaults = %+v", s)
	}
}

func TestOverridesAndCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte(`{"bookmarks":{"backend":"sqlite","config":{"path":"/x/b.db"}}}`), 0o600)
	if s := Load(path, dir); s.Bookmarks.Config["path"] != "/x/b.db" {
		t.Errorf("override ignored: %+v", s)
	}
	os.WriteFile(path, []byte(`{not json`), 0o600)
	if s := Load(path, dir); s.Bookmarks.Backend != "sqlite" {
		t.Errorf("corrupt file should give defaults: %+v", s)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "settings.json")
	in := Settings{Bookmarks: Backend{Backend: "linkagogo", Config: map[string]string{"root": "42"}}}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out := Load(path, dir)
	if out.Bookmarks.Backend != "linkagogo" || out.Bookmarks.Config["root"] != "42" {
		t.Errorf("round trip = %+v", out)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("temp file left behind")
	}
}
