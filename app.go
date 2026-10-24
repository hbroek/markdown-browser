package main

import (
	"context"
	"os"
	"path/filepath"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/browser"
	"markdown-browser/internal/cookies"
	"markdown-browser/internal/fetch"
	"markdown-browser/internal/history"
	"markdown-browser/internal/page"
)

// App holds the state shared with the frontend through Wails bindings.
type App struct {
	ctx     context.Context
	browser *browser.Browser
	auth    *auth.Manager
	jar     *cookies.Jar
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
	a.browser = browser.New(history.Open(file("history.json")), fetch.NewFetcher(a.auth, a.jar))
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
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
