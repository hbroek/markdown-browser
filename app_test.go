package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/bookmarks"
	"markdown-browser/internal/fetch"
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
	// Registered after TempDir, so it runs first: Windows cannot delete an open database.
	t.Cleanup(func() {
		a.bmMu.Lock()
		defer a.bmMu.Unlock()
		if a.bookmarks != nil {
			a.bookmarks.Close()
		}
	})
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

// aiServer answers chat completions with `reply` and records the last request.
func aiServer(t *testing.T, reply string, got *string, auth *string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if got != nil {
			*got = string(data)
		}
		if auth != nil {
			*auth = r.Header.Get("Authorization")
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func pageServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<title>Async Rust</title><script>evil()</script><h1>Async</h1><p>Futures are lazy.</p>`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSaveAIKeepsKeyOutOfSettingsAndNeedsConsentForRemote(t *testing.T) {
	a := testApp(t, &miniLinkaGoGo{})
	a.fetcher = fetch.NewFetcher(a.auth, nil)

	// A server that is not on this computer needs consent.
	err := a.SaveAI(AIInput{Preset: "openai", BaseURL: "https://api.openai.com/v1", Model: "m"}, "sk-secret")
	if err == nil || !strings.Contains(err.Error(), "allow sending") {
		t.Fatalf("remote without consent: %v", err)
	}
	if err := a.SaveAI(AIInput{Preset: "openai", BaseURL: "https://api.openai.com/v1", Model: "m"}, ""); err == nil {
		t.Error("a preset that needs a key was saved without one")
	}
	if err := a.SaveAI(AIInput{Preset: "openai", BaseURL: "https://api.openai.com/v1", Model: "m", Consent: true}, "sk-secret"); err != nil {
		t.Fatal(err)
	}
	info := a.AISettings()
	if !info.Configured || !info.HasKey || !info.Consented || info.Model != "m" {
		t.Errorf("info = %+v", info)
	}
	data, _ := readFile(a.settingsPath)
	if strings.Contains(data, "sk-secret") || !strings.Contains(data, "api.openai.com") {
		t.Errorf("settings.json = %s", data)
	}
	// A saved key is reused when the dialog is saved again without typing one.
	if err := a.SaveAI(AIInput{Preset: "openai", BaseURL: "https://api.openai.com/v1", Model: "m2", Consent: true}, ""); err != nil {
		t.Errorf("resave: %v", err)
	}
}

func TestAIFeaturesNeedAProvider(t *testing.T) {
	a := testApp(t, &miniLinkaGoGo{})
	if _, err := a.SummarizePage("https://x.test/"); err == nil || !strings.Contains(err.Error(), "AI") {
		t.Errorf("SummarizePage without a provider: %v", err)
	}
}

func TestSummarizeAndSuggestUseThePageText(t *testing.T) {
	a := testApp(t, &miniLinkaGoGo{})
	a.fetcher = fetch.NewFetcher(a.auth, nil)
	var sent, authHeader string
	llm := aiServer(t, "- **Futures** are lazy", &sent, &authHeader)
	pages := pageServer(t)
	if err := a.SaveAI(AIInput{Preset: "custom", BaseURL: llm.URL + "/v1", Model: "m"}, ""); err != nil { // loopback: no consent, no key
		t.Fatal(err)
	}

	sum, err := a.SummarizePage(pages.URL)
	if err != nil || !strings.Contains(sum.HTML, "<strong>Futures</strong>") || sum.Source != "m" || sum.Millis < 0 {
		t.Fatalf("SummarizePage = %+v, %v", sum, err)
	}
	if !strings.Contains(sent, "Futures are lazy") || strings.Contains(sent, "evil()") || !strings.Contains(sent, "Async Rust") {
		t.Errorf("model was sent %s", sent)
	}
	if authHeader != "" {
		t.Errorf("sent credentials to a keyless server: %q", authHeader)
	}

	llm2 := aiServer(t, "```json\n{\"title\":\"Async Rust\",\"description\":\"About futures.\",\"keywords\":[\"rust\",\"async\"]}\n```", nil, nil)
	if err := a.SaveAI(AIInput{Preset: "custom", BaseURL: llm2.URL + "/v1"}, ""); err != nil {
		t.Fatal(err)
	}
	s, err := a.SuggestBookmark(pages.URL)
	if err != nil || s.Description != "About futures." || len(s.Keywords) != 2 {
		t.Errorf("SuggestBookmark = %+v, %v", s, err)
	}
	if _, err := a.SummarizePage("bookmarks:search?q=x"); err == nil {
		t.Error("summarized a search page")
	}
}

func TestSwitchingBookmarkStorageKeepsAISettings(t *testing.T) {
	a := testApp(t, &miniLinkaGoGo{})
	if err := a.SaveAI(AIInput{Preset: "ollama", BaseURL: "http://127.0.0.1:11434/v1", Model: "llama3.2"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := a.UseLocalBookmarks(); err != nil {
		t.Fatal(err)
	}
	if got := a.AISettings(); got.Model != "llama3.2" {
		t.Errorf("AI settings lost: %+v", got)
	}
	if data, _ := readFile(a.settingsPath); !strings.Contains(data, "llama3.2") {
		t.Errorf("settings.json lost the AI section: %s", data)
	}
}

func TestTestAIReportsServerErrors(t *testing.T) {
	a := testApp(t, &miniLinkaGoGo{})
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, `{"error":{"message":"no"}}`, 401) }))
	defer bad.Close()
	if _, err := a.TestAI(AIInput{BaseURL: bad.URL + "/v1"}, "k"); err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Errorf("TestAI = %v", err)
	}
}

func TestSummarySourceNamesTheModelOrProvider(t *testing.T) {
	a := testApp(t, &miniLinkaGoGo{})
	for _, tc := range []struct {
		in   AIInput
		want string
	}{
		{AIInput{Preset: "ollama", BaseURL: "http://127.0.0.1:11434/v1", Model: "llama3.2"}, "llama3.2"},
		{AIInput{Preset: "apple", BaseURL: "http://127.0.0.1:1976/v1"}, "Apple on-device (fm serve)"},
		{AIInput{Preset: "custom", BaseURL: "http://127.0.0.1:8080/v1"}, "127.0.0.1:8080"},
	} {
		if err := a.SaveAI(tc.in, ""); err != nil {
			t.Fatal(err)
		}
		if got := a.aiSource(); got != tc.want {
			t.Errorf("source = %q, want %q", got, tc.want)
		}
	}
}
