package fetch

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func header(hs []Header, name string) string {
	for _, h := range hs {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func TestRedact(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"Authorization", "Bearer s3cret", "Bearer [redacted]"},
		{"Authorization", "s3cret", "[redacted]"},
		{"Cookie", "a=1; weft_session=abc", "a=[redacted]; weft_session=[redacted]"},
		{"Set-Cookie", "sid=abc; Path=/; HttpOnly", "sid=[redacted]; Path=/; HttpOnly"},
		{"Content-Type", "text/html", "text/html"},
	}
	for _, tt := range tests {
		if got := redact(tt.name, tt.in); got != tt.want {
			t.Errorf("redact(%q, %q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestLogRecordsRedirectHopsAndRedactsSecrets(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "topsecret", Path: "/"})
		http.Redirect(w, r, "/new", http.StatusSeeOther)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte("# hello"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	f := NewFetcher(fakeAuth{host: strings.TrimPrefix(srv.URL, "http://"), cookie: "weft=mysecret"}, jar)
	var seen []Entry
	f.Log.OnEntry = func(e Entry) { seen = append(seen, e) }

	if _, err := f.Get(context.Background(), srv.URL+"/old"); err != nil {
		t.Fatal(err)
	}
	got := f.Log.Entries()
	if len(got) != 2 || len(seen) != 2 {
		t.Fatalf("entries = %d, callbacks = %d", len(got), len(seen))
	}
	hop, final := got[0], got[1]
	if hop.Status != 303 || hop.URL != srv.URL+"/old" || header(hop.RespHeaders, "Location") != "/new" {
		t.Errorf("redirect hop = %+v", hop)
	}
	if final.Status != 200 || final.URL != srv.URL+"/new" || final.Bytes != int64(len("# hello")) || final.ContentType != "text/markdown" {
		t.Errorf("final = %+v", final)
	}
	if !strings.HasPrefix(header(hop.ReqHeaders, "Accept"), "text/markdown") {
		t.Errorf("request Accept header missing: %+v", hop.ReqHeaders)
	}
	if got := header(hop.ReqHeaders, "Cookie"); got != "weft=[redacted]" {
		t.Errorf("request Cookie = %q", got)
	}
	if got := header(hop.RespHeaders, "Set-Cookie"); !strings.HasPrefix(got, "sid=[redacted]") {
		t.Errorf("response Set-Cookie = %q", got)
	}
	// The jar's cookie is sent on the second hop, and shown redacted too.
	if got := header(final.ReqHeaders, "Cookie"); strings.Contains(got, "topsecret") || !strings.Contains(got, "sid=[redacted]") {
		t.Errorf("second hop Cookie = %q", got)
	}
}

func TestLogRecordsErrorsAndFiles(t *testing.T) {
	f := NewFetcher(nil, nil)

	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused
	if _, err := f.Get(context.Background(), url); err == nil {
		t.Fatal("expected error")
	}

	path := filepath.Join(t.TempDir(), "a.md")
	os.WriteFile(path, []byte("# hi"), 0o644)
	if _, err := f.Get(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	f.Get(context.Background(), path+".missing")

	got := f.Log.Entries()
	if len(got) != 3 {
		t.Fatalf("entries = %+v", got)
	}
	if got[0].Error == "" || got[0].Status != 0 {
		t.Errorf("failed request = %+v", got[0])
	}
	if got[1].Status != 200 || got[1].Proto != "file" || got[1].Bytes != 4 {
		t.Errorf("file entry = %+v", got[1])
	}
	if got[2].Error == "" {
		t.Errorf("missing file entry = %+v", got[2])
	}
	if got[0].ID >= got[1].ID || got[1].ID >= got[2].ID {
		t.Error("ids not increasing")
	}
}

func TestLogCapAndClear(t *testing.T) {
	l := NewLog()
	for i := 0; i < maxLogEntries+10; i++ {
		l.add(Entry{ID: l.newID()})
	}
	got := l.Entries()
	if len(got) != maxLogEntries || got[len(got)-1].ID != maxLogEntries+10 {
		t.Errorf("len=%d last=%d", len(got), got[len(got)-1].ID)
	}
	l.Clear()
	if len(l.Entries()) != 0 {
		t.Error("not cleared")
	}
}
