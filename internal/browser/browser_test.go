package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/fetch"
	"markdown-browser/internal/history"
	"markdown-browser/internal/page"
)

func testServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte("# Doc\n\n[next](/md2)"))
	})
	mux.HandleFunc("/md2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte("# Two"))
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<title>Plain</title><a href="/md">md</a>`))
	})
	mux.HandleFunc("/feed", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`<rss><channel><title>Feed</title><item><link>/1</link><source:markdown>**hi**</source:markdown></item></channel></rss>`))
	})
	return httptest.NewServer(mux)
}

func TestNavigateKinds(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	ctx := context.Background()

	p := b.Navigate(ctx, 1, srv.URL+"/md")
	if p.Kind != page.KindMarkdown || p.Title != "Doc" || !strings.Contains(p.HTML, srv.URL+"/md2") || p.Source == "" {
		t.Errorf("markdown page = %+v", p)
	}

	p = b.Navigate(ctx, 1, srv.URL+"/html")
	if p.Kind != page.KindFallback || p.Title != "Plain" || len(p.Links) != 1 {
		t.Errorf("fallback page = %+v", p)
	}

	p = b.Navigate(ctx, 1, srv.URL+"/missing")
	if p.Kind != page.KindError || p.Status != 404 || !strings.HasPrefix(p.Error, "404") {
		t.Errorf("error page = %+v", p)
	}

	p = b.Navigate(ctx, 1, "ftp://x")
	if p.Kind != page.KindError {
		t.Errorf("bad scheme page = %+v", p)
	}
}

func TestBackForwardReload(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	ctx := context.Background()

	if p := b.Reload(ctx, 1); p.Kind != page.KindError {
		t.Errorf("reload on empty = %+v", p)
	}

	b.Navigate(ctx, 1, srv.URL+"/md")
	p := b.Navigate(ctx, 1, srv.URL+"/md2")
	if !p.CanBack || p.CanForward {
		t.Errorf("state after two visits: %+v", p)
	}

	p = b.Back(ctx, 1)
	if p.Title != "Doc" || p.CanBack || !p.CanForward {
		t.Errorf("back = %+v", p)
	}
	if p = b.Reload(ctx, 1); p.Title != "Doc" || p.CanBack {
		t.Errorf("reload = %+v", p)
	}
	p = b.Forward(ctx, 1)
	if p.Title != "Two" || !p.CanBack || p.CanForward {
		t.Errorf("forward = %+v", p)
	}
	if p = b.Forward(ctx, 1); p.Kind != page.KindError || p.URL != srv.URL+"/md2" {
		t.Errorf("forward at end = %+v", p)
	}
}

func TestHistoryRecordsSuccessfulLoadsOnly(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	ctx := context.Background()

	b.Navigate(ctx, 1, srv.URL+"/md")
	b.Navigate(ctx, 1, srv.URL+"/missing")
	b.Navigate(ctx, 1, srv.URL+"/html")
	b.Back(ctx, 1) // lands on the failed entry: not recorded
	b.Reload(ctx, 1)

	got := b.History()
	if len(got) != 2 || got[0].Title != "Plain" || got[1].URL != srv.URL+"/md" || got[1].Title != "Doc" {
		t.Errorf("history = %+v", got)
	}
	b.ClearHistory()
	if len(b.History()) != 0 {
		t.Error("history not cleared")
	}
}

func TestNavigateFeed(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	p := New(history.Open(""), fetch.NewFetcher(nil, nil)).Navigate(context.Background(), 1, srv.URL+"/feed")
	if p.Kind != page.KindMarkdown || p.Title != "Feed" || !strings.Contains(p.HTML, "<strong>hi</strong>") || !strings.Contains(p.Source, "<rss>") {
		t.Errorf("feed page = %+v", p)
	}
}

func TestFullHistory(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	ctx := context.Background()
	b.Navigate(ctx, 1, srv.URL+"/md")

	p := b.FullHistory(ctx, 1)
	if p.Kind != page.KindMarkdown || p.Title != "History" || !strings.Contains(p.HTML, srv.URL+"/md") || !strings.Contains(p.HTML, "Doc") {
		t.Errorf("history page = %+v", p)
	}
	if !p.CanBack {
		t.Error("history page should be navigable back")
	}
	if n := len(b.History()); n != 1 {
		t.Errorf("history page was recorded: %d entries", n)
	}
}

func TestUnauthorizedPageOffersOrigin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "sid=ok" {
			w.Header().Set("Content-Type", "text/markdown")
			w.Write([]byte("# Private"))
			return
		}
		w.Header().Set("Www-Authenticate", "Bearer")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	secrets := &auth.MemorySecrets{}
	mgr := auth.NewManager(secrets, "")
	b := New(history.Open(""), fetch.NewFetcher(mgr, nil))
	ctx := context.Background()

	p := b.Navigate(ctx, 1, srv.URL)
	if p.Kind != page.KindError || p.Origin != srv.URL || p.AuthScheme != "bearer" {
		t.Fatalf("401 page = %+v", p)
	}

	if _, err := mgr.Save(auth.Credential{Origin: srv.URL, Kind: auth.Cookie, Cookie: "sid=ok"}); err != nil {
		t.Fatal(err)
	}
	if p = b.Reload(ctx, 1); p.Kind != page.KindMarkdown || p.Title != "Private" {
		t.Errorf("after saving credentials: %+v", p)
	}
}

func TestAddressBarBookmarkSearch(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	hist := history.Open("")
	b := New(hist, fetch.NewFetcher(nil, nil))
	var queries []string
	b.SetSearcher(func(_ context.Context, q string) (string, error) {
		queries = append(queries, q)
		return "# Found\n\n- [Hit](<https://hit.test/>)\n", nil
	})

	b.Navigate(context.Background(), 1, srv.URL+"/md")
	p := b.Navigate(context.Background(), 1, "rust  async")
	if p.Kind != page.KindMarkdown || p.URL != "bookmarks:search?q=rust+async" || !strings.Contains(p.HTML, "https://hit.test/") {
		t.Fatalf("search page = %+v", p)
	}
	if len(queries) != 1 || queries[0] != "rust async" {
		t.Errorf("queries = %q", queries)
	}
	if !p.CanBack {
		t.Error("search should be a history entry for Back")
	}
	if got := hist.Entries(); len(got) != 1 {
		t.Errorf("searches must not be recorded as visits: %+v", got)
	}

	if p := b.Back(context.Background(), 1); !strings.HasSuffix(p.URL, "/md") {
		t.Errorf("Back = %q", p.URL)
	}
	if p := b.Forward(context.Background(), 1); p.URL != "bookmarks:search?q=rust+async" || p.Kind != page.KindMarkdown {
		t.Errorf("Forward = %+v", p)
	}
	if p := b.Reload(context.Background(), 1); p.Kind != page.KindMarkdown || len(queries) != 3 {
		t.Errorf("Reload = %+v, queries %q", p, queries)
	}
	if p := b.Navigate(context.Background(), 1, "?"); p.URL != "bookmarks:search?q=" || queries[len(queries)-1] != "" {
		t.Errorf("bare ? = %+v, queries %q", p, queries)
	}
}

func TestSearchWithoutBackend(t *testing.T) {
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	if p := b.Navigate(context.Background(), 1, "anything goes"); p.Kind != page.KindError {
		t.Errorf("want error page, got %+v", p)
	}
}

func TestTabsHaveSeparateHistory(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	ctx := context.Background()
	one, two := b.NewTab(), b.NewTab()
	if one == two {
		t.Fatal("tabs share an id")
	}

	b.Navigate(ctx, one, srv.URL+"/md")
	b.Navigate(ctx, one, srv.URL+"/md2")
	if p := b.Navigate(ctx, two, srv.URL+"/html"); p.CanBack {
		t.Error("a new tab inherited history from another")
	}
	if p := b.Back(ctx, one); !strings.HasSuffix(p.URL, "/md") {
		t.Errorf("tab one went back to %q", p.URL)
	}
	if p := b.Reload(ctx, two); p.Title != "Plain" {
		t.Errorf("tab two reloaded %q", p.Title)
	}

	b.CloseTab(one)
	if p := b.Reload(ctx, one); p.Kind != page.KindError { // starts empty again
		t.Errorf("closed tab kept its page: %+v", p)
	}
	if got := len(b.History()); got == 0 {
		t.Error("visits from tabs were not recorded in the shared history")
	}
}

func TestMeaningSearchPage(t *testing.T) {
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	var asked []string
	b.SetMeaningSearcher(func(_ context.Context, q string) (string, error) {
		asked = append(asked, q)
		return "# Best matches\n\n- one\n", nil
	})
	p := b.Navigate(context.Background(), 1, "@ that  async article")
	if p.URL != "bookmarks:meaning?q=that+async+article" || p.Kind != page.KindMarkdown || len(asked) != 1 || asked[0] != "that async article" {
		t.Fatalf("page = %+v, asked %v", p, asked)
	}
	if got := b.Reload(context.Background(), 1); got.Kind != page.KindMarkdown || len(asked) != 2 {
		t.Errorf("reload = %+v", got)
	}
	if len(b.History()) != 0 {
		t.Errorf("a search by meaning was recorded as a visit: %+v", b.History())
	}
	empty := New(history.Open(""), fetch.NewFetcher(nil, nil))
	if p := empty.Navigate(context.Background(), 1, "@ x"); p.Kind != page.KindError {
		t.Errorf("without a searcher: %+v", p)
	}
}

func TestEditablePageCarriesEditAddressAndVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		if r.URL.Path == "/rw" {
			w.Header().Set("Allow", "GET, HEAD, PUT")
			w.Header().Set("ETag", `"v7"`)
		}
		w.Write([]byte("# Doc"))
	}))
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	if p := b.Navigate(context.Background(), 1, srv.URL+"/rw"); p.EditURL != srv.URL+"/rw" || p.ETag != `"v7"` {
		t.Errorf("editable page = %+v", p)
	}
	if p := b.Navigate(context.Background(), 1, srv.URL+"/ro"); p.EditURL != "" || p.ETag != "" {
		t.Errorf("read-only page offers editing: %+v", p)
	}
}
