package fetch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEditable(t *testing.T) {
	res := func(url string, h map[string]string) *Result {
		hd := http.Header{}
		for k, v := range h {
			hd.Add(k, v)
		}
		return &Result{URL: url, Status: 200, Header: hd}
	}
	cases := []struct {
		name string
		res  *Result
		want string
	}{
		{"put allowed", res("https://x.test/a.md", map[string]string{"Allow": "GET, HEAD, put, DELETE"}), "https://x.test/a.md"},
		{"read only", res("https://x.test/a.md", map[string]string{"Allow": "GET, HEAD"}), ""},
		{"no Allow", res("https://x.test/a.md", nil), ""},
		{"edit link", res("https://x.test/view", map[string]string{"Allow": "GET, PUT", "Link": `<https://x.test/docs/a.md>; rel="edit", </next>; rel=next`}), "https://x.test/docs/a.md"},
		{"relative edit link", res("https://x.test/view", map[string]string{"Allow": "PUT", "Link": `</docs/a.md>; rel="alternate edit"`}), "https://x.test/docs/a.md"},
		{"edit link on another server is ignored", res("https://x.test/a.md", map[string]string{"Allow": "PUT", "Link": `<https://evil.test/a.md>; rel="edit"`}), "https://x.test/a.md"},
		{"error page", &Result{URL: "https://x.test/", Status: 404, Header: http.Header{"Allow": {"PUT"}}}, ""},
		{"file", res("file:///a.md", map[string]string{"Allow": "PUT"}), ""},
	}
	for _, c := range cases {
		if got := Editable(c.res); got != c.want {
			t.Errorf("%s: Editable = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPut(t *testing.T) {
	var gotBody, gotMatch, gotType, method string
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, gotMatch, gotType = r.Method, r.Header.Get("If-Match"), r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	f := NewFetcher(nil, nil)
	ctx := context.Background()

	if err := f.Put(ctx, srv.URL+"/a.md", []byte("# New"), `"v1"`); err != nil {
		t.Fatal(err)
	}
	if method != "PUT" || gotBody != "# New" || gotMatch != `"v1"` || gotType != "text/markdown; charset=utf-8" {
		t.Errorf("sent %s %q If-Match %q type %q", method, gotBody, gotMatch, gotType)
	}
	f.Put(ctx, srv.URL+"/a.md", []byte("x"), `W/"weak"`)
	if gotMatch != "" {
		t.Errorf("weak ETag sent as If-Match: %q", gotMatch)
	}
	status = http.StatusPreconditionFailed
	if err := f.Put(ctx, srv.URL+"/a.md", []byte("x"), `"v1"`); err != ErrChanged {
		t.Errorf("412 = %v", err)
	}
	status = http.StatusForbidden
	if err := f.Put(ctx, srv.URL+"/a.md", []byte("x"), ""); err == nil {
		t.Error("403 was reported as saved")
	}
	if err := f.Put(ctx, srv.URL+"/moved", []byte("x"), ""); err == nil {
		t.Error("a redirect, which turns the PUT into a GET, was reported as saved")
	}
	if err := f.Put(ctx, "/tmp/a.md", []byte("x"), ""); err == nil {
		t.Error("a local file was PUT")
	}
}
