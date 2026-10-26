package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/bookmarks"
	"markdown-browser/internal/bookmarks/linkagogo"
	_ "markdown-browser/internal/bookmarks/sqlite"
	"markdown-browser/internal/browser"
	"markdown-browser/internal/cookies"
	"markdown-browser/internal/fetch"
	"markdown-browser/internal/history"
	"markdown-browser/internal/page"
	"markdown-browser/internal/settings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App holds the state shared with the frontend through Wails bindings.
type App struct {
	ctx     context.Context
	browser *browser.Browser
	auth    *auth.Manager
	jar     *cookies.Jar
	fetcher *fetch.Fetcher

	bmMu         sync.RWMutex
	bookmarks    bookmarks.Store // nil when the backend failed to open
	bookmarksErr error
	settings     settings.Settings
	settingsPath string
	lggURL       string // LinkaGoGo API address; empty is the real one (tests set it)
}

func NewApp() *App {
	// On error the files stay in memory / the working directory is not used.
	dir, _ := os.UserConfigDir()
	if dir != "" {
		dir = filepath.Join(dir, "markdown-browser")
	}
	file := func(name string) string {
		if dir == "" {
			return ""
		}
		return filepath.Join(dir, name)
	}

	keychain := auth.KeyringStore{}
	a := &App{
		auth: auth.NewManager(keychain, file("credentials.json")),
		jar:  cookies.New(keychain, file("cookies.bin")),
	}
	a.fetcher = fetch.NewFetcher(a.auth, a.jar)
	a.browser = browser.New(history.Open(file("history.json")), a.fetcher)

	a.settingsPath = file("settings.json")
	a.settings = settings.Load(a.settingsPath, dir)
	a.bookmarks, a.bookmarksErr = bookmarks.Open(a.settings.Bookmarks.Backend, a.settings.Bookmarks.Config, bookmarks.Deps{Authorize: a.auth.Apply})
	a.browser.SetSearcher(a.searchPage)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Push each request to the console as it completes.
	a.fetcher.Log.OnEntry = func(e fetch.Entry) { runtime.EventsEmit(ctx, "net", e) }
}

// Each page-level call names the tab it acts on; NewTab hands out the ids.
func (a *App) NewTab() int                            { return a.browser.NewTab() }
func (a *App) CloseTab(tab int)                       { a.browser.CloseTab(tab) }
func (a *App) Navigate(tab int, url string) page.Page { return a.browser.Navigate(a.ctx, tab, url) }
func (a *App) Reload(tab int) page.Page               { return a.browser.Reload(a.ctx, tab) }
func (a *App) Back(tab int) page.Page                 { return a.browser.Back(a.ctx, tab) }
func (a *App) Forward(tab int) page.Page              { return a.browser.Forward(a.ctx, tab) }

func (a *App) History() []history.Entry { return a.browser.History() }
func (a *App) ClearHistory()            { a.browser.ClearHistory() }

// FullHistory opens the whole visit log as a generated markdown page.
func (a *App) FullHistory(tab int) page.Page { return a.browser.FullHistory(a.ctx, tab) }

// Credentials lists saved credentials without their secrets.
func (a *App) Credentials() []auth.Info { return a.auth.List() }

// SaveCredential stores a credential in the OS keychain and returns the
// normalized site origin it applies to.
func (a *App) SaveCredential(c auth.Credential) (string, error) { return a.auth.Save(c) }

func (a *App) DeleteCredential(origin string) error { return a.auth.Delete(origin) }

// ImportCookiesTxt saves the cookies for origin found in a cookies.txt export.
func (a *App) ImportCookiesTxt(origin, text string) (int, error) {
	return a.auth.ImportCookiesTxt(origin, text)
}

// ClearCookies forgets the cookies that sites have set.
func (a *App) ClearCookies() { a.jar.Clear() }

// NetworkLog returns the recent requests shown in the console.
func (a *App) NetworkLog() []fetch.Entry { return a.fetcher.Log.Entries() }

func (a *App) ClearNetworkLog() { a.fetcher.Log.Clear() }

// Bookmark bindings. They report a backend that failed to open as an error
// instead of stopping the browser from starting.

func (a *App) store() (bookmarks.Store, error) {
	a.bmMu.RLock()
	defer a.bmMu.RUnlock()
	if a.bookmarks == nil {
		return nil, fmt.Errorf("bookmarks unavailable: %w", a.bookmarksErr)
	}
	return a.bookmarks, nil
}

func (a *App) bookmarkCtx() context.Context {
	if a.ctx == nil {
		return context.Background()
	}
	return a.ctx
}

// BookmarkFolders lists every folder, flat; the frontend nests them by ParentID.
func (a *App) BookmarkFolders() ([]bookmarks.Folder, error) {
	s, err := a.store()
	if err != nil {
		return nil, err
	}
	return s.Folders(a.bookmarkCtx())
}

// BookmarksIn lists the bookmarks directly in a folder ("" is the root).
func (a *App) BookmarksIn(folderID string) ([]bookmarks.Bookmark, error) {
	s, err := a.store()
	if err != nil {
		return nil, err
	}
	return s.Bookmarks(a.bookmarkCtx(), folderID)
}

// BookmarkFor returns the saved bookmark for a URL, or nil when it is not saved.
func (a *App) BookmarkFor(url string) (*bookmarks.Bookmark, error) {
	s, err := a.store()
	if err != nil {
		return nil, err
	}
	b, err := s.FindByURL(a.bookmarkCtx(), url)
	if errors.Is(err, bookmarks.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// SaveBookmark creates or updates a bookmark; a new one for an
// already-saved URL updates the existing bookmark.
func (a *App) SaveBookmark(b bookmarks.Bookmark) (bookmarks.Bookmark, error) {
	s, err := a.store()
	if err != nil {
		return b, err
	}
	return bookmarks.SaveUnique(a.bookmarkCtx(), s, b)
}

func (a *App) DeleteBookmark(id string) error {
	s, err := a.store()
	if err != nil {
		return err
	}
	return s.DeleteBookmark(a.bookmarkCtx(), id)
}

// SaveBookmarkFolder creates (empty ID), renames or moves a folder.
func (a *App) SaveBookmarkFolder(f bookmarks.Folder) (bookmarks.Folder, error) {
	s, err := a.store()
	if err != nil {
		return f, err
	}
	return s.SaveFolder(a.bookmarkCtx(), f)
}

// DeleteBookmarkFolder removes a folder with everything inside it.
func (a *App) DeleteBookmarkFolder(id string) error {
	s, err := a.store()
	if err != nil {
		return err
	}
	return s.DeleteFolder(a.bookmarkCtx(), id)
}

// SearchBookmarks finds bookmarks containing every word of the query.
func (a *App) SearchBookmarks(query string) ([]bookmarks.Bookmark, error) {
	s, err := a.store()
	if err != nil {
		return nil, err
	}
	return s.Search(a.bookmarkCtx(), strings.Fields(query))
}

// matchingVisits returns the history entries (most recent first) that contain
// every term in their title or address. No terms means no history: listing
// everything is what Full history is for.
func (a *App) matchingVisits(terms []string) []bookmarks.Visit {
	if len(terms) == 0 {
		return nil
	}
	var out []bookmarks.Visit
	for _, e := range a.browser.History() {
		if bookmarks.Matches(bookmarks.Bookmark{Title: e.Title, URL: e.URL}, terms) {
			out = append(out, bookmarks.Visit{Title: e.Title, URL: e.URL, Time: e.Time})
		}
	}
	return out
}

// searchPage is the address bar's bookmark and history search: a markdown page of results.
func (a *App) searchPage(ctx context.Context, query string) (string, error) {
	s, err := a.store()
	if err != nil {
		return "", err
	}
	results, err := s.Search(ctx, strings.Fields(query))
	if err != nil {
		return "", err
	}
	folders, err := s.Folders(ctx)
	if err != nil {
		return "", err
	}
	return bookmarks.SearchMarkdown(query, results, a.matchingVisits(strings.Fields(query)), folders), nil
}

// maxImportBytes bounds the bookmark file read on import.
const maxImportBytes = 64 << 20

// ImportBookmarks asks for a bookmark file and adds its folders and
// bookmarks: an HTML export from any browser, or a markdown list (.md,
// .markdown, .txt; see internal/bookmarks/markdown_io.go). Canceling the
// dialog is not an error; the result says so.
func (a *App) ImportBookmarks() (bookmarks.ImportResult, error) {
	s, err := a.store()
	if err != nil {
		return bookmarks.ImportResult{}, err
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import bookmarks",
		Filters: []runtime.FileFilter{{DisplayName: "Bookmark files (*.html, *.htm, *.md)", Pattern: "*.html;*.htm;*.md;*.markdown;*.txt"}},
	})
	if err != nil || path == "" {
		return bookmarks.ImportResult{Canceled: err == nil}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return bookmarks.ImportResult{}, err
	}
	defer f.Close()
	parse := bookmarks.ParseNetscape
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".txt":
		parse = bookmarks.ParseMarkdown
	}
	root, err := parse(io.LimitReader(f, maxImportBytes))
	if err != nil {
		return bookmarks.ImportResult{}, err
	}
	return bookmarks.Import(a.bookmarkCtx(), s, root)
}

// ExportBookmarks asks where to save and writes every bookmark as an HTML
// bookmark file (format "html") or a markdown list ("markdown"). It returns
// the path, or "" when the dialog was canceled.
func (a *App) ExportBookmarks(format string) (string, error) {
	s, err := a.store()
	if err != nil {
		return "", err
	}
	name, pattern, export := "bookmarks.html", "*.html", bookmarks.ExportNetscape
	switch format {
	case "html":
	case "markdown":
		name, pattern, export = "bookmarks.md", "*.md", bookmarks.ExportMarkdown
	default:
		return "", fmt.Errorf("unknown export format %q", format)
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export bookmarks",
		DefaultFilename: name,
		Filters:         []runtime.FileFilter{{DisplayName: "Bookmark files (" + pattern + ")", Pattern: pattern}},
	})
	if err != nil || path == "" {
		return "", err
	}
	out, err := export(a.bookmarkCtx(), s)
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(out), 0o644)
}

// --- bookmark storage settings ---

// BookmarkSettings describes where bookmarks are kept.
type BookmarkSettings struct {
	Backend   string `json:"backend"`   // "sqlite" or "linkagogo"
	Root      string `json:"root"`      // LinkaGoGo folder used as the top level; "" is the whole account
	Connected bool   `json:"connected"` // a LinkaGoGo API key is saved
	Error     string `json:"error"`     // why the current storage is unavailable, if it is
}

func (a *App) BookmarkSettings() BookmarkSettings {
	a.bmMu.RLock()
	defer a.bmMu.RUnlock()
	out := BookmarkSettings{Backend: a.settings.Bookmarks.Backend, Root: a.settings.Bookmarks.Config["root"]}
	if a.bookmarksErr != nil {
		out.Error = a.bookmarksErr.Error()
	}
	origin := linkagogoOrigin(a.lggURL)
	for _, c := range a.auth.List() {
		if c.Origin == origin && c.Kind == auth.Bearer {
			out.Connected = true
		}
	}
	return out
}

func linkagogoOrigin(apiURL string) string {
	s, err := linkagogo.New(bookmarks.Config{"url": apiURL}, bookmarks.Deps{})
	if err != nil {
		return ""
	}
	return s.Origin()
}

// linkagogoStore opens a LinkaGoGo store. With apiKey it signs with that key,
// which is used only for this store and not saved; without, with the saved one.
func (a *App) linkagogoStore(apiKey, root string) (*linkagogo.Store, error) {
	apiKey = strings.TrimSpace(apiKey)
	if strings.ContainsAny(apiKey, " \t\r\n") {
		return nil, errors.New("the API key must not contain spaces or line breaks")
	}
	authorize := a.auth.Apply
	if apiKey != "" {
		authorize = func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+apiKey) }
	}
	cfg := bookmarks.Config{}
	if a.lggURL != "" {
		cfg["url"] = a.lggURL
	}
	if root != "" {
		cfg["root"] = root
	}
	return linkagogo.New(cfg, bookmarks.Deps{Authorize: authorize})
}

// LinkaGoGoFolders lists every folder in the account so one can be chosen as
// the app's top level. A key typed in the dialog is tried without being saved.
func (a *App) LinkaGoGoFolders(apiKey string) ([]bookmarks.Folder, error) {
	s, err := a.linkagogoStore(apiKey, "")
	if err != nil {
		return nil, err
	}
	return s.RootCandidates(a.bookmarkCtx())
}

// UseLinkaGoGo keeps bookmarks in LinkaGoGo under the folder rootID ("" is the
// whole account), or under a new top-level folder called newFolder. The key
// and folder are tried first; nothing is saved or switched if they fail.
func (a *App) UseLinkaGoGo(apiKey, rootID, newFolder string) error {
	ctx := a.bookmarkCtx()
	trial, err := a.linkagogoStore(apiKey, "")
	if err != nil {
		return err
	}
	if name := strings.TrimSpace(newFolder); name != "" {
		f, err := trial.SaveFolder(ctx, bookmarks.Folder{Name: name})
		if err != nil {
			return err
		}
		rootID = f.ID
	}
	if rootID != "" {
		if trial, err = a.linkagogoStore(apiKey, rootID); err != nil {
			return err
		}
	}
	if _, err := trial.Folders(ctx); err != nil { // proves the key works and the folder exists
		return err
	}
	if key := strings.TrimSpace(apiKey); key != "" {
		if _, err := a.auth.Save(auth.Credential{Origin: trial.Origin(), Kind: auth.Bearer, Token: key}); err != nil {
			return err
		}
	}

	cfg := bookmarks.Config{}
	if rootID != "" {
		cfg["root"] = rootID
	}
	if a.lggURL != "" {
		cfg["url"] = a.lggURL
	}
	return a.switchBookmarks("linkagogo", cfg)
}

// UseLocalBookmarks goes back to the SQLite file on this computer.
func (a *App) UseLocalBookmarks() error {
	cfg := bookmarks.Config{}
	a.bmMu.RLock()
	if a.settings.Bookmarks.Backend == "sqlite" {
		cfg = a.settings.Bookmarks.Config // keep a custom file path
	}
	a.bmMu.RUnlock()
	return a.switchBookmarks("sqlite", cfg)
}

// switchBookmarks opens the backend, saves the choice and starts using it.
func (a *App) switchBookmarks(backend string, cfg bookmarks.Config) error {
	a.bmMu.RLock()
	aiCfg := a.settings.AI
	a.bmMu.RUnlock()
	next := settings.Settings{Bookmarks: settings.Backend{Backend: backend, Config: cfg}, AI: aiCfg}
	dir := ""
	if a.settingsPath != "" {
		dir = filepath.Dir(a.settingsPath)
	}
	resolved := settings.Resolve(next, dir)
	store, err := bookmarks.Open(resolved.Bookmarks.Backend, resolved.Bookmarks.Config, bookmarks.Deps{Authorize: a.auth.Apply})
	if err != nil {
		return err
	}
	if a.settingsPath != "" {
		if err := settings.Save(a.settingsPath, next); err != nil {
			store.Close()
			return err
		}
	}
	a.bmMu.Lock()
	old := a.bookmarks
	a.bookmarks, a.bookmarksErr, a.settings = store, nil, resolved
	a.bmMu.Unlock()
	if old != nil {
		old.Close()
	}
	return nil
}
