package cookies

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"markdown-browser/internal/auth"
)

func mustURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

func names(cs []*http.Cookie) map[string]string {
	m := map[string]string{}
	for _, c := range cs {
		m[c.Name] = c.Value
	}
	return m
}

func TestPersistsAcrossRestartsEncrypted(t *testing.T) {
	secrets := &auth.MemorySecrets{}
	path := filepath.Join(t.TempDir(), "cookies.bin")
	u := mustURL("https://example.com/app")

	j := New(secrets, path)
	j.SetCookies(u, []*http.Cookie{
		{Name: "sid", Value: "topsecretvalue", Path: "/", Secure: true, HttpOnly: true},
		{Name: "long", Value: "1", Path: "/", MaxAge: 3600},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("topsecretvalue")) || bytes.Contains(data, []byte("example.com")) {
		t.Error("cookie file is not encrypted")
	}
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 { // no Unix modes on Windows
		t.Errorf("file mode = %v", info.Mode().Perm())
	}

	got := names(New(secrets, path).Cookies(u))
	if got["sid"] != "topsecretvalue" || got["long"] != "1" {
		t.Errorf("restored cookies = %v", got)
	}
}

func TestDeleteAndExpiry(t *testing.T) {
	secrets := &auth.MemorySecrets{}
	path := filepath.Join(t.TempDir(), "cookies.bin")
	u := mustURL("https://example.com/")

	j := New(secrets, path)
	j.SetCookies(u, []*http.Cookie{{Name: "a", Value: "1", Path: "/"}, {Name: "b", Value: "2", Path: "/"}})
	j.SetCookies(u, []*http.Cookie{{Name: "a", Value: "", Path: "/", MaxAge: -1}}) // server deletes a
	j.SetCookies(u, []*http.Cookie{{Name: "c", Value: "3", Path: "/", Expires: time.Now().Add(-time.Hour)}})

	got := names(New(secrets, path).Cookies(u))
	if len(got) != 1 || got["b"] != "2" {
		t.Errorf("restored = %v", got)
	}
}

func TestClearAndCorruptFile(t *testing.T) {
	secrets := &auth.MemorySecrets{}
	path := filepath.Join(t.TempDir(), "cookies.bin")
	u := mustURL("https://example.com/")

	j := New(secrets, path)
	j.SetCookies(u, []*http.Cookie{{Name: "a", Value: "1", Path: "/"}})
	j.Clear()
	if len(j.Cookies(u)) != 0 || len(New(secrets, path).Cookies(u)) != 0 {
		t.Error("Clear left cookies behind")
	}

	os.WriteFile(path, []byte("garbage garbage garbage garbage garbage"), 0o600)
	if len(New(secrets, path).Cookies(u)) != 0 {
		t.Error("corrupt file produced cookies")
	}
}

func TestMemoryOnlyWithoutPath(t *testing.T) {
	j := New(&auth.MemorySecrets{}, "")
	u := mustURL("https://example.com/")
	j.SetCookies(u, []*http.Cookie{{Name: "a", Value: "1", Path: "/"}})
	if names(j.Cookies(u))["a"] != "1" {
		t.Error("in-memory jar lost cookie")
	}
}
