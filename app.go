package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/bookmarks"
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

	bookmarks    bookmarks.Store // nil when the backend failed to open
	bookmarksErr error
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

	cfg := settings.Load(file("settings.json"), dir)
	a.bookmarks, a.bookmarksErr = bookmarks.Open(cfg.Bookmarks.Backend, cfg.Bookmarks.Config)
	a.browser.SetSearcher(a.searchPage)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Push each request to the console as it completes.
	a.fetcher.Log.OnEntry = func(e fetch.Entry) { runtime.EventsEmit(ctx, "net", e) }
}

func (a *App) Navigate(url string) page.Page { return a.browser.Navigate(a.ctx, url) }
func (a *App) Reload() page.Page             { return a.browser.Reload(a.ctx) }
func (a *App) Back() page.Page               { return a.browser.Back(a.ctx) }
func (a *App) Forward() page.Page            { return a.browser.Forward(a.ctx) }

func (a *App) History() []history.Entry { return a.browser.History() }
func (a *App) ClearHistory()            { a.browser.ClearHistory() }

// FullHistory opens the whole visit log as a generated markdown page.
func (a *App) FullHistory() page.Page { return a.browser.FullHistory(a.ctx) }

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
