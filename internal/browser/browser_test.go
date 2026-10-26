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

	p := b.Navigate(ctx, srv.URL+"/md")
	if p.Kind != page.KindMarkdown || p.Title != "Doc" || !strings.Contains(p.HTML, srv.URL+"/md2") || p.Source == "" {
		t.Errorf("markdown page = %+v", p)
	}

	p = b.Navigate(ctx, srv.URL+"/html")
	if p.Kind != page.KindFallback || p.Title != "Plain" || len(p.Links) != 1 {
		t.Errorf("fallback page = %+v", p)
	}

	p = b.Navigate(ctx, srv.URL+"/missing")
	if p.Kind != page.KindError || p.Status != 404 || !strings.HasPrefix(p.Error, "404") {
		t.Errorf("error page = %+v", p)
	}

	p = b.Navigate(ctx, "ftp://x")
	if p.Kind != page.KindError {
		t.Errorf("bad scheme page = %+v", p)
	}
}

func TestBackForwardReload(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	ctx := context.Background()

	if p := b.Reload(ctx); p.Kind != page.KindError {
		t.Errorf("reload on empty = %+v", p)
	}

	b.Navigate(ctx, srv.URL+"/md")
	p := b.Navigate(ctx, srv.URL+"/md2")
	if !p.CanBack || p.CanForward {
		t.Errorf("state after two visits: %+v", p)
	}

	p = b.Back(ctx)
	if p.Title != "Doc" || p.CanBack || !p.CanForward {
		t.Errorf("back = %+v", p)
	}
	if p = b.Reload(ctx); p.Title != "Doc" || p.CanBack {
		t.Errorf("reload = %+v", p)
	}
	p = b.Forward(ctx)
	if p.Title != "Two" || !p.CanBack || p.CanForward {
		t.Errorf("forward = %+v", p)
	}
	if p = b.Forward(ctx); p.Kind != page.KindError || p.URL != srv.URL+"/md2" {
		t.Errorf("forward at end = %+v", p)
	}
}

func TestHistoryRecordsSuccessfulLoadsOnly(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	ctx := context.Background()

	b.Navigate(ctx, srv.URL+"/md")
	b.Navigate(ctx, srv.URL+"/missing")
	b.Navigate(ctx, srv.URL+"/html")
	b.Back(ctx) // lands on the failed entry: not recorded
	b.Reload(ctx)

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
	p := New(history.Open(""), fetch.NewFetcher(nil, nil)).Navigate(context.Background(), srv.URL+"/feed")
	if p.Kind != page.KindMarkdown || p.Title != "Feed" || !strings.Contains(p.HTML, "<strong>hi</strong>") || !strings.Contains(p.Source, "<rss>") {
		t.Errorf("feed page = %+v", p)
	}
}

func TestFullHistory(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	ctx := context.Background()
	b.Navigate(ctx, srv.URL+"/md")

	p := b.FullHistory(ctx)
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

	p := b.Navigate(ctx, srv.URL)
	if p.Kind != page.KindError || p.Origin != srv.URL || p.AuthScheme != "bearer" {
		t.Fatalf("401 page = %+v", p)
	}

	if _, err := mgr.Save(auth.Credential{Origin: srv.URL, Kind: auth.Cookie, Cookie: "sid=ok"}); err != nil {
		t.Fatal(err)
	}
	if p = b.Reload(ctx); p.Kind != page.KindMarkdown || p.Title != "Private" {
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

	b.Navigate(context.Background(), srv.URL+"/md")
	p := b.Navigate(context.Background(), "rust  async")
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

	if p := b.Back(context.Background()); !strings.HasSuffix(p.URL, "/md") {
		t.Errorf("Back = %q", p.URL)
	}
	if p := b.Forward(context.Background()); p.URL != "bookmarks:search?q=rust+async" || p.Kind != page.KindMarkdown {
		t.Errorf("Forward = %+v", p)
	}
	if p := b.Reload(context.Background()); p.Kind != page.KindMarkdown || len(queries) != 3 {
		t.Errorf("Reload = %+v, queries %q", p, queries)
	}
	if p := b.Navigate(context.Background(), "?"); p.URL != "bookmarks:search?q=" || queries[len(queries)-1] != "" {
		t.Errorf("bare ? = %+v, queries %q", p, queries)
	}
}

func TestSearchWithoutBackend(t *testing.T) {
	b := New(history.Open(""), fetch.NewFetcher(nil, nil))
	if p := b.Navigate(context.Background(), "anything goes"); p.Kind != page.KindError {
		t.Errorf("want error page, got %+v", p)
	}
}
