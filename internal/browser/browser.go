// Package browser ties fetching, rendering and navigation together.
package browser

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/fetch"
	"markdown-browser/internal/history"
	"markdown-browser/internal/nav"
	"markdown-browser/internal/page"
	"markdown-browser/internal/render"
)

type Browser struct {
	fetcher *fetch.Fetcher
	mu      sync.Mutex
	stack   *nav.Stack
	history *history.Log
}

func New(log *history.Log, fetcher *fetch.Fetcher) *Browser {
	return &Browser{stack: nav.NewStack(), history: log, fetcher: fetcher}
}

// History returns the visit log, most recent first.
func (b *Browser) History() []history.Entry { return b.history.Entries() }

func (b *Browser) ClearHistory() { b.history.Clear() }

// FullHistory writes the visit log as a markdown file and opens it.
func (b *Browser) FullHistory(ctx context.Context) page.Page {
	path, err := b.history.WriteMarkdown()
	if err != nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.withState(page.Page{Kind: page.KindError, Error: "cannot write history: " + err.Error()})
	}
	return b.Navigate(ctx, path)
}

// record adds a successfully loaded page to the visit log.
func (b *Browser) record(p page.Page) page.Page {
	if p.Kind != page.KindError {
		b.history.Add(p.URL, p.Title)
	}
	return p
}

// Navigate loads rawURL and makes it the current entry.
func (b *Browser) Navigate(ctx context.Context, rawURL string) page.Page {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.load(ctx, rawURL)
	b.stack.Visit(p.URL)
	return b.withState(b.record(p))
}

// Reload fetches the current entry again.
func (b *Browser) Reload(ctx context.Context) page.Page {
	b.mu.Lock()
	defer b.mu.Unlock()
	cur := b.stack.Current()
	if cur == "" {
		return b.withState(page.Page{Kind: page.KindError, Error: "nothing to reload"})
	}
	return b.withState(b.load(ctx, cur))
}

// Back loads the previous entry.
func (b *Browser) Back(ctx context.Context) page.Page {
	b.mu.Lock()
	defer b.mu.Unlock()
	u, ok := b.stack.Back()
	if !ok {
		return b.withState(page.Page{URL: b.stack.Current(), Kind: page.KindError, Error: "no earlier page"})
	}
	return b.withState(b.record(b.load(ctx, u)))
}

// Forward loads the next entry.
func (b *Browser) Forward(ctx context.Context) page.Page {
	b.mu.Lock()
	defer b.mu.Unlock()
	u, ok := b.stack.Forward()
	if !ok {
		return b.withState(page.Page{URL: b.stack.Current(), Kind: page.KindError, Error: "no later page"})
	}
	return b.withState(b.record(b.load(ctx, u)))
}

func (b *Browser) withState(p page.Page) page.Page {
	p.CanBack = b.stack.CanBack()
	p.CanForward = b.stack.CanForward()
	return p
}

// load fetches rawURL and builds the Page to display. Failures become error pages.
func (b *Browser) load(ctx context.Context, rawURL string) page.Page {
	res, err := b.fetcher.Get(ctx, rawURL)
	if err != nil {
		p := page.Page{URL: rawURL, Kind: page.KindError, Error: err.Error()}
		if u, nerr := fetch.Normalize(rawURL); nerr == nil {
			p.URL = u.String()
		}
		return p
	}
	p := page.Page{URL: res.URL, Status: res.Status, ContentType: res.ContentType}
	if res.Status >= 400 {
		p.Kind = page.KindError
		p.Error = fmt.Sprintf("%d %s", res.Status, http.StatusText(res.Status))
		if res.Status == http.StatusUnauthorized || res.Status == http.StatusForbidden {
			if u, err := url.Parse(res.URL); err == nil {
				p.Origin = auth.Origin(u)
			}
			if f := strings.Fields(res.WWWAuthenticate); len(f) > 0 {
				p.AuthScheme = strings.ToLower(f[0])
			}
		}
		return p
	}
	base, err := url.Parse(res.URL)
	if err != nil {
		p.Kind, p.Error = page.KindError, err.Error()
		return p
	}
	if render.IsFeed(res.Body) {
		if html, title, ok, err := render.Feed(res.Body, base); err != nil {
			p.Kind, p.Error = page.KindError, err.Error()
			return p
		} else if ok {
			p.Kind, p.HTML, p.Source, p.Title = page.KindMarkdown, html, string(res.Body), title
			if p.Title == "" {
				p.Title = res.URL
			}
			return p
		}
	}
	if fetch.IsMarkdown(res.ContentType) {
		html, title, err := render.Markdown(res.Body, base)
		if err != nil {
			p.Kind, p.Error = page.KindError, err.Error()
			return p
		}
		p.Kind, p.HTML, p.Source = page.KindMarkdown, html, string(res.Body)
		p.Title = title
	} else {
		p.Kind = page.KindFallback
		p.Title, p.Links = render.Fallback(res.Body, res.ContentType, base)
	}
	if p.Title == "" {
		p.Title = res.URL
	}
	return p
}
