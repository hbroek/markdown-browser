package imgproxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/fetch"
)

func TestWanted(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://example.com/a.png":          true, // the webview refuses plain http
		"http://henry.localhost:8080/m.jpg": true,
		"https://henry.localhost/m.jpg":     true,
		"https://example.com/a.png":         false, // loads directly
		"file:///tmp/a.png":                 false,
	} {
		u, _ := url.Parse(raw)
		if got := Wanted(u); got != want {
			t.Errorf("Wanted(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestHandlerSendsCredentialsAndOnlyServesImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "sid=ok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<script>x</script>"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()

	mgr := auth.NewManager(&auth.MemorySecrets{}, "")
	h := Handler(fetch.NewFetcher(mgr, nil))
	get := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, URL(target), nil))
		return rec
	}

	if rec := get(srv.URL + "/m.png"); rec.Code != http.StatusBadGateway {
		t.Errorf("without credentials: %d", rec.Code)
	}
	if _, err := mgr.Save(auth.Credential{Origin: srv.URL, Kind: auth.Cookie, Cookie: "sid=ok"}); err != nil {
		t.Fatal(err)
	}
	rec := get(srv.URL + "/m.png")
	if rec.Code != 200 || rec.Body.String() != "PNGDATA" || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Content-Security-Policy") == "" {
		t.Errorf("with credentials: %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if rec := get(srv.URL + "/page.html"); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a page was served as an image: %d", rec.Code)
	}
	if rec := get("ftp://x/a.png"); rec.Code != http.StatusBadGateway {
		t.Errorf("ftp: %d", rec.Code)
	}
}
