package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/bookmarks"
	"markdown-browser/internal/settings"
)

// miniLinkaGoGo answers just enough of the API for the storage settings flow.
type miniLinkaGoGo struct {
	mu      sync.Mutex
	folders []map[string]any
	urls    []map[string]any
}

func (m *miniLinkaGoGo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Authorization") != "Bearer lgg_good" {
		w.WriteHeader(401)
		w.Write([]byte(`{"detail":"Invalid API key"}`))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	switch r.Method + " " + path {
	case "GET /folders":
		json.NewEncoder(w).Encode(m.folders)
	case "POST /folders":
		f := map[string]any{"id": len(m.folders) + 10, "name": body["name"], "folder_id": body["folder_id"]}
		m.folders = append(m.folders, f)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(f)
	case "GET /urls":
		json.NewEncoder(w).Encode(map[string]any{"bookmarks": append([]map[string]any{}, m.urls...)})
	case "POST /urls":
		u := map[string]any{"id": len(m.urls) + 100, "url": body["url"], "title": body["title"], "folder_id": body["folder_id"]}
		m.urls = append(m.urls, u)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(u)
	default:
		w.WriteHeader(404)
		w.Write([]byte(`{"detail":"not found"}`))
	}
}

func testApp(t *testing.T, api *miniLinkaGoGo) *App {
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	a := &App{
		auth:         auth.NewManager(&auth.MemorySecrets{}, ""),
		bookmarks:    bookmarks.NewMemory(),
		settings:     settings.Settings{Bookmarks: settings.Backend{Backend: "memory"}},
		settingsPath: filepath.Join(dir, "settings.json"),
		lggURL:       srv.URL + "/api/v1",
	}
	return a
}

func TestUseLinkaGoGoCreatesAndUsesRootFolder(t *testing.T) {
	api := &miniLinkaGoGo{folders: []map[string]any{{"id": 1, "name": "Existing", "folder_id": 0}}}
	a := testApp(t, api)

	folders, err := a.LinkaGoGoFolders("lgg_good")
	if err != nil || len(folders) != 1 || folders[0].Name != "Existing" {
		t.Fatalf("folders = %+v, %v", folders, err)
	}
	if a.BookmarkSettings().Connected {
		t.Error("listing folders with a typed key must not save the key")
	}

	if err := a.UseLinkaGoGo("lgg_good", "", "Markdown Browser"); err != nil {
		t.Fatal(err)
	}
	st := a.BookmarkSettings()
	if st.Backend != "linkagogo" || st.Root != "11" || !st.Connected || st.Error != "" {
		t.Errorf("settings = %+v", st)
	}
	saved := settings.Load(a.settingsPath, t.TempDir())
	if saved.Bookmarks.Backend != "linkagogo" || saved.Bookmarks.Config["root"] != "11" {
		t.Errorf("settings.json = %+v", saved)
	}
	if data, _ := readFile(a.settingsPath); strings.Contains(data, "lgg_good") {
		t.Errorf("the API key must never be written to settings.json:\n%s", data)
	}

	// The saved key now signs requests, and bookmarks land in the new folder.
	if _, err := a.SaveBookmark(bookmarks.Bookmark{Title: "Go", URL: "https://go.dev/"}); err != nil {
		t.Fatal(err)
	}
	if len(api.urls) != 1 || api.urls[0]["folder_id"] != float64(11) && api.urls[0]["folder_id"] != 11 {
		t.Errorf("server bookmarks = %+v", api.urls)
	}
	if got, err := a.BookmarksIn(""); err != nil || len(got) != 1 || got[0].Title != "Go" {
		t.Errorf("BookmarksIn = %+v, %v", got, err)
	}
}

func TestUseLinkaGoGoRejectsBadKeyAndChangesNothing(t *testing.T) {
	api := &miniLinkaGoGo{}
	a := testApp(t, api)
	err := a.UseLinkaGoGo("lgg_wrong", "", "Markdown Browser")
	if err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Fatalf("err = %v", err)
	}
	if st := a.BookmarkSettings(); st.Backend != "memory" || st.Connected {
		t.Errorf("a failed switch changed things: %+v", st)
	}
	if len(api.folders) != 0 {
		t.Errorf("a folder was created with a bad key: %+v", api.folders)
	}
	if _, err := a.LinkaGoGoFolders("bad key\nwith lines"); err == nil {
		t.Error("a key with whitespace should be refused")
	}
	if err := a.UseLinkaGoGo("lgg_good", "999", ""); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("unknown root folder: %v", err)
	}
	if _, err := a.BookmarksIn(""); err != nil {
		t.Errorf("the previous storage must keep working: %v", err)
	}
}

func TestUseLocalBookmarks(t *testing.T) {
	a := testApp(t, &miniLinkaGoGo{})
	dir := filepath.Dir(a.settingsPath)
	a.settings = settings.Settings{Bookmarks: settings.Backend{Backend: "linkagogo"}}
	if err := a.switchBookmarks("sqlite", bookmarks.Config{"path": filepath.Join(dir, "b.db")}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveBookmark(bookmarks.Bookmark{Title: "Local", URL: "https://l.test/"}); err != nil {
		t.Fatal(err)
	}
	if err := a.UseLocalBookmarks(); err != nil { // keeps the custom file
		t.Fatal(err)
	}
	if got, _ := a.BookmarksIn(""); len(got) != 1 {
		t.Errorf("custom sqlite path was lost: %+v", got)
	}
	if a.BookmarkSettings().Backend != "sqlite" {
		t.Errorf("settings = %+v", a.BookmarkSettings())
	}
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}
