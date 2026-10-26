package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"example.com/x", "https://example.com/x", false},
		{"  http://example.com  ", "http://example.com", false},
		{"https://example.com/a?b=1", "https://example.com/a?b=1", false},
		{"localhost:3000/x", "http://localhost:3000/x", false},
		{"henry.localhost:8080/timeline", "http://henry.localhost:8080/timeline", false},
		{"127.0.0.1:8080", "http://127.0.0.1:8080", false},
		{"https://localhost:8443/", "https://localhost:8443/", false},
		{"localhost.evil.com", "https://localhost.evil.com", false},
		{"/tmp/a.md", "file:///tmp/a.md", false},
		{"file:///tmp/a.md", "file:///tmp/a.md", false},
		{"", "", true},
		{"ftp://example.com", "", true},
		{"javascript:alert(1)", "", true},
		{"http://", "", true},
	}
	for _, tt := range tests {
		u, err := Normalize(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("Normalize(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && u.String() != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, u, tt.want)
		}
	}
}

func TestGetSendsAcceptAndReturnsBody(t *testing.T) {
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Write([]byte("# hi"))
	}))
	defer srv.Close()

	res, err := Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(accept, "text/markdown") {
		t.Errorf("Accept = %q", accept)
	}
	if string(res.Body) != "# hi" || res.Status != 200 || !IsMarkdown(res.ContentType) {
		t.Errorf("unexpected result %+v", res)
	}
}

func TestGetFollowsRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := Get(context.Background(), srv.URL+"/old")
	if err != nil {
		t.Fatal(err)
	}
	if res.URL != srv.URL+"/new" {
		t.Errorf("final URL = %q", res.URL)
	}
}

func TestGetNon2xxIsNotError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	res, err := Get(context.Background(), srv.URL)
	if err != nil || res.Status != 404 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestGetBodyCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, MaxBody+1))
	}))
	defer srv.Close()
	if _, err := Get(context.Background(), srv.URL); err == nil {
		t.Fatal("expected size error")
	}
}

func TestGetFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte("# file"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Get(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Body) != "# file" || !IsMarkdown(res.ContentType) {
		t.Errorf("unexpected result %+v", res)
	}
	if _, err := Get(context.Background(), path+".missing"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestIsMarkdown(t *testing.T) {
	yes := []string{"text/markdown", "text/x-markdown; charset=utf-8", "text/plain", "TEXT/Markdown"}
	no := []string{"text/html", "application/json", "", "garbage;;"}
	for _, ct := range yes {
		if !IsMarkdown(ct) {
			t.Errorf("IsMarkdown(%q) = false", ct)
		}
	}
	for _, ct := range no {
		if IsMarkdown(ct) {
			t.Errorf("IsMarkdown(%q) = true", ct)
		}
	}
}

func TestFilePathFor(t *testing.T) {
	win, _ := url.Parse("file:///C:/Users/me/a%20b.md")
	if got := filePathFor("windows", win); got != `C:\Users\me\a b.md` {
		t.Errorf("windows path = %q", got)
	}
	unix, _ := url.Parse("file:///home/me/a%20b.md")
	if got := filePathFor("linux", unix); got != "/home/me/a b.md" {
		t.Errorf("unix path = %q", got)
	}
}

func TestNormalizeWindowsDrivePath(t *testing.T) {
	if got := fileURL(`C:\docs\a.md`).String(); got != "file:///C:/docs/a.md" && got != "file:///C:%5Cdocs%5Ca.md" {
		t.Errorf("fileURL = %q", got)
	}
}

type fakeAuth struct{ host, cookie string }

func (f fakeAuth) Apply(req *http.Request) {
	if req.URL.Host == f.host {
		req.Header.Set("Cookie", f.cookie)
		req.Header.Set("Authorization", "Bearer "+f.cookie)
	}
}

func TestCredentialsAppliedAndNotLeakedOnRedirect(t *testing.T) {
	var gotCookie, gotAuth string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie, gotAuth = r.Header.Get("Cookie"), r.Header.Get("Authorization")
	}))
	defer other.Close()

	var firstCookie string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, other.URL, http.StatusFound)
			return
		}
		firstCookie = r.Header.Get("Cookie")
		w.Header().Set("Www-Authenticate", `Basic realm="x"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer origin.Close()

	ou, _ := url.Parse(origin.URL)
	f := NewFetcher(fakeAuth{host: ou.Host, cookie: "sid=1"}, nil)

	res, err := f.Get(context.Background(), origin.URL+"/page")
	if err != nil || firstCookie != "sid=1" || res.Status != 401 || !strings.HasPrefix(res.WWWAuthenticate, "Basic") {
		t.Fatalf("res=%+v err=%v firstCookie=%q", res, err, firstCookie)
	}

	if _, err := f.Get(context.Background(), origin.URL+"/redir"); err != nil {
		t.Fatal(err)
	}
	if gotCookie != "" || gotAuth != "" {
		t.Errorf("credentials leaked to another origin: cookie=%q auth=%q", gotCookie, gotAuth)
	}
}

func TestSearchQuery(t *testing.T) {
	tests := []struct {
		in, want string
		search   bool
	}{
		{"rust", "rust", true},
		{"rust async", "rust async", true},
		{"  rust   2.0 release ", "rust 2.0 release", true},
		{"?rust", "rust", true},
		{"?", "", true},
		{"? go.dev", "go.dev", true},
		{"example.com", "", false},
		{"localhost", "", false},
		{"localhost:3000", "", false},
		{"henry.localhost:8080/timeline", "", false},
		{"https://example.com/a b", "", false},
		{"/Users/me/My Notes/a.md", "", false},
		{"~/My Notes/a.md", "", false},
		{"javascript:alert(1)", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := SearchQuery(tt.in)
		if ok != tt.search || got != tt.want {
			t.Errorf("SearchQuery(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.search)
		}
	}
}
