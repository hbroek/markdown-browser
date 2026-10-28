package main

import (
	"errors"
	"fmt"
	"net/url"

	"markdown-browser/internal/fetch"
	"markdown-browser/internal/page"
)

// SaveSource saves edited markdown back to the server the page came from, then loads the
// page again so the tab shows (and remembers the version of) what is stored now. editURL and
// etag are what the page was loaded with; the server only gets the text if it is still that version.
func (a *App) SaveSource(tab int, pageURL, editURL, etag, text string) (page.Page, error) {
	pu, err1 := url.Parse(pageURL)
	eu, err2 := url.Parse(editURL)
	if err := errors.Join(err1, err2); err != nil || pu.Host == "" || pu.Scheme != eu.Scheme || pu.Host != eu.Host {
		return page.Page{}, errors.New("this document can only be saved to the server it came from")
	}
	if len(text) > fetch.MaxBody {
		return page.Page{}, fmt.Errorf("document larger than %d MB", fetch.MaxBody>>20)
	}
	if err := a.fetcher.Put(a.ctx, editURL, []byte(text), etag); err != nil {
		return page.Page{}, err
	}
	a.pageMu.Lock()
	delete(a.pageCache, pageURL) // the AI must not answer from the old text
	a.pageMu.Unlock()
	return a.browser.Reload(a.ctx, tab), nil
}
