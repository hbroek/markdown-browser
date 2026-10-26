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

// Searcher turns a bookmark search query into a markdown page.
type Searcher func(ctx context.Context, query string) (string, error)

// searchPrefix starts the pseudo-URL of a bookmark search page, so Back,
// Forward and Reload can rebuild it: "bookmarks:search?q=rust+async".
const searchPrefix = "bookmarks:search?q="

// SearchURL is the address of the results page for query.
func SearchURL(query string) string { return searchPrefix + url.QueryEscape(query) }

func isSearchURL(u string) bool { return strings.HasPrefix(u, searchPrefix) }

// tab is one page with its own back/forward history. Its lock is held while a
// page loads, so a slow site only holds up its own tab.
type tab struct {
	mu    sync.Mutex
	stack *nav.Stack
}

type Browser struct {
	fetcher *fetch.Fetcher
	search  Searcher
	history *history.Log

	tabMu   sync.Mutex
	tabs    map[int]*tab
	nextTab int
}

func New(log *history.Log, fetcher *fetch.Fetcher) *Browser {
	return &Browser{history: log, fetcher: fetcher, tabs: map[int]*tab{}}
}

// NewTab opens an empty tab and returns its id. The history is shared by all tabs.
func (b *Browser) NewTab() int {
	b.tabMu.Lock()
	defer b.tabMu.Unlock()
	b.nextTab++
	b.tabs[b.nextTab] = &tab{stack: nav.NewStack()}
	return b.nextTab
}

// CloseTab forgets the tab's back/forward history.
func (b *Browser) CloseTab(id int) {
	b.tabMu.Lock()
	defer b.tabMu.Unlock()
	delete(b.tabs, id)
}

// tab returns the tab with this id, creating it if it is not known yet.
func (b *Browser) tab(id int) *tab {
	b.tabMu.Lock()
	defer b.tabMu.Unlock()
	t, ok := b.tabs[id]
	if !ok {
		t = &tab{stack: nav.NewStack()}
		b.tabs[id] = t
	}
	return t
}

// SetSearcher enables bookmark searches typed into the address bar.
func (b *Browser) SetSearcher(s Searcher) { b.search = s }

// History returns the visit log, most recent first.
func (b *Browser) History() []history.Entry { return b.history.Entries() }

func (b *Browser) ClearHistory() { b.history.Clear() }

// FullHistory writes the visit log as a markdown file and opens it.
func (b *Browser) FullHistory(ctx context.Context, tabID int) page.Page {
	path, err := b.history.WriteMarkdown()
	if err != nil {
		t := b.tab(tabID)
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.withState(page.Page{Kind: page.KindError, Error: "cannot write history: " + err.Error()})
	}
	return b.Navigate(ctx, tabID, path)
}

// record adds a successfully loaded page to the visit log.
func (b *Browser) record(p page.Page) page.Page {
	if p.Kind != page.KindError && !isSearchURL(p.URL) { // searches are not visits
		b.history.Add(p.URL, p.Title)
	}
	return p
}

// Navigate loads rawURL and makes it the current entry.
func (b *Browser) Navigate(ctx context.Context, tabID int, rawURL string) page.Page {
	t := b.tab(tabID)
	t.mu.Lock()
	defer t.mu.Unlock()
	if q, ok := fetch.SearchQuery(rawURL); ok {
		rawURL = SearchURL(q)
	}
	p := b.load(ctx, rawURL)
	t.stack.Visit(p.URL)
	return t.withState(b.record(p))
}

// Reload fetches the current entry again.
func (b *Browser) Reload(ctx context.Context, tabID int) page.Page {
	t := b.tab(tabID)
	t.mu.Lock()
	defer t.mu.Unlock()
	cur := t.stack.Current()
	if cur == "" {
		return t.withState(page.Page{Kind: page.KindError, Error: "nothing to reload"})
	}
	return t.withState(b.load(ctx, cur))
}

// Back loads the previous entry.
func (b *Browser) Back(ctx context.Context, tabID int) page.Page {
	t := b.tab(tabID)
	t.mu.Lock()
	defer t.mu.Unlock()
	u, ok := t.stack.Back()
	if !ok {
		return t.withState(page.Page{URL: t.stack.Current(), Kind: page.KindError, Error: "no earlier page"})
	}
	return t.withState(b.record(b.load(ctx, u)))
}

// Forward loads the next entry.
func (b *Browser) Forward(ctx context.Context, tabID int) page.Page {
	t := b.tab(tabID)
	t.mu.Lock()
	defer t.mu.Unlock()
	u, ok := t.stack.Forward()
	if !ok {
		return t.withState(page.Page{URL: t.stack.Current(), Kind: page.KindError, Error: "no later page"})
	}
	return t.withState(b.record(b.load(ctx, u)))
}

func (t *tab) withState(p page.Page) page.Page {
	p.CanBack = t.stack.CanBack()
	p.CanForward = t.stack.CanForward()
	return p
}

// load fetches rawURL and builds the Page to display. Failures become error pages.
func (b *Browser) load(ctx context.Context, rawURL string) page.Page {
	if isSearchURL(rawURL) {
		return b.loadSearch(ctx, rawURL)
	}
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

// loadSearch builds the bookmark search results page.
func (b *Browser) loadSearch(ctx context.Context, rawURL string) page.Page {
	p := page.Page{URL: rawURL}
	query, err := url.QueryUnescape(strings.TrimPrefix(rawURL, searchPrefix))
	if err == nil && b.search == nil {
		err = fmt.Errorf("bookmarks are not available")
	}
	var md string
	if err == nil {
		md, err = b.search(ctx, query)
	}
	if err != nil {
		p.Kind, p.Error = page.KindError, err.Error()
		return p
	}
	base, _ := url.Parse(rawURL)
	html, title, err := render.Markdown([]byte(md), base)
	if err != nil {
		p.Kind, p.Error = page.KindError, err.Error()
		return p
	}
	p.Kind, p.HTML, p.Source, p.Title = page.KindMarkdown, html, md, title
	return p
}
