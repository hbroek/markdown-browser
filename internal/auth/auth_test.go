package auth

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func req(t *testing.T, raw string) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct {
		in, want string
		bad      bool
	}{
		{"Example.com", "https://example.com", false},
		{"https://example.com:443/a/b?c", "https://example.com", false},
		{"http://localhost:8080/x", "http://localhost:8080", false},
		{"http://henry.localhost:8080", "http://henry.localhost:8080", false},
		{"", "", true},
		{"ftp://example.com", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeOrigin(tt.in)
		if (err != nil) != tt.bad || got != tt.want {
			t.Errorf("NormalizeOrigin(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func TestAllowed(t *testing.T) {
	yes := []string{"https://example.com", "http://localhost:3000", "http://127.0.0.1:8080", "http://henry.localhost:8080", "http://[::1]:80"}
	no := []string{"http://example.com", "http://192.168.1.5", "http://localhost.evil.com"}
	for _, o := range yes {
		if !Allowed(o) {
			t.Errorf("Allowed(%q) = false", o)
		}
	}
	for _, o := range no {
		if Allowed(o) {
			t.Errorf("Allowed(%q) = true", o)
		}
	}
}

func TestSaveApplyExactOrigin(t *testing.T) {
	m := NewManager(&MemorySecrets{}, "")
	origin, err := m.Save(Credential{Origin: "https://Example.com/login", Kind: Cookie, Cookie: "sid=abc"})
	if err != nil || origin != "https://example.com" {
		t.Fatalf("Save = %q, %v", origin, err)
	}

	r := req(t, "https://example.com/page")
	m.Apply(r)
	if got := r.Header.Get("Cookie"); got != "sid=abc" {
		t.Errorf("Cookie = %q", got)
	}

	for _, other := range []string{"https://sub.example.com/", "https://example.com:8443/", "https://evil.com/", "http://example.com/"} {
		r := req(t, other)
		m.Apply(r)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("credential leaked to %s", other)
		}
	}
}

func TestBearerAndBasic(t *testing.T) {
	m := NewManager(&MemorySecrets{}, "")
	m.Save(Credential{Origin: "https://a.example", Kind: Bearer, Token: "tok"})
	m.Save(Credential{Origin: "https://b.example", Kind: Basic, Username: "u", Password: "p:w"})

	r := req(t, "https://a.example/")
	m.Apply(r)
	if r.Header.Get("Authorization") != "Bearer tok" {
		t.Errorf("bearer = %q", r.Header.Get("Authorization"))
	}
	r = req(t, "https://b.example/")
	m.Apply(r)
	if u, p, ok := r.BasicAuth(); !ok || u != "u" || p != "p:w" {
		t.Errorf("basic = %q %q %v", u, p, ok)
	}
}

func TestSaveValidation(t *testing.T) {
	m := NewManager(&MemorySecrets{}, "")
	bad := []Credential{
		{Origin: "http://example.com", Kind: Cookie, Cookie: "a=b"},          // cleartext http
		{Origin: "https://example.com", Kind: Cookie, Cookie: "a=b\r\nX: y"}, // header injection
		{Origin: "https://example.com", Kind: Bearer},
		{Origin: "https://example.com", Kind: Basic, Username: "a:b"},
		{Origin: "https://example.com", Kind: "weird"},
		{Origin: "", Kind: Bearer, Token: "t"},
	}
	for i, c := range bad {
		if _, err := m.Save(c); err == nil {
			t.Errorf("case %d accepted: %+v", i, c)
		}
	}
	if len(m.List()) != 0 {
		t.Error("rejected credentials were stored")
	}
}

func TestListDeleteAndPersistedIndex(t *testing.T) {
	secrets := &MemorySecrets{}
	index := filepath.Join(t.TempDir(), "creds.json")
	m := NewManager(secrets, index)
	m.Save(Credential{Origin: "https://b.example", Kind: Basic, Username: "bob", Password: "x"})
	m.Save(Credential{Origin: "https://a.example", Kind: Cookie, Cookie: "s=1"})

	// A new manager (app restart) sees the index and reads secrets from the keychain.
	m2 := NewManager(secrets, index)
	list := m2.List()
	if len(list) != 2 || list[0].Origin != "https://a.example" || list[1].Username != "bob" {
		t.Fatalf("list = %+v", list)
	}
	r := req(t, "https://a.example/")
	m2.Apply(r)
	if r.Header.Get("Cookie") != "s=1" {
		t.Error("credential not restored from keychain")
	}

	if err := m2.Delete("a.example"); err != nil {
		t.Fatal(err)
	}
	r = req(t, "https://a.example/")
	m2.Apply(r)
	if r.Header.Get("Cookie") != "" || len(NewManager(secrets, index).List()) != 1 {
		t.Error("delete did not remove the credential")
	}
}

func TestListNeverExposesSecrets(t *testing.T) {
	m := NewManager(&MemorySecrets{}, "")
	m.Save(Credential{Origin: "https://a.example", Kind: Bearer, Token: "supersecret"})
	if strings.Contains(strings.ToLower(string(mustJSON(t, m.List()))), "supersecret") {
		t.Error("List leaks the token")
	}
}

func TestImportCookiesTxt(t *testing.T) {
	future := time.Now().Add(time.Hour).Unix()
	txt := strings.Join([]string{
		"# Netscape HTTP Cookie File",
		"example.com\tTRUE\t/\tTRUE\t" + itoa(future) + "\tsid\tabc",
		".example.com\tTRUE\t/\tFALSE\t0\tpref\tdark",
		"#HttpOnly_example.com\tFALSE\t/\tTRUE\t" + itoa(future) + "\ttoken\tt0k",
		"example.com\tTRUE\t/\tFALSE\t1\told\texpired",
		"other.com\tTRUE\t/\tFALSE\t0\tnope\tx",
		"notexample.com\tTRUE\t/\tFALSE\t0\tnope2\tx",
	}, "\r\n")
	m := NewManager(&MemorySecrets{}, "")
	n, err := m.ImportCookiesTxt("https://example.com/whatever", txt)
	if err != nil || n != 3 {
		t.Fatalf("imported %d, %v", n, err)
	}
	r := req(t, "https://example.com/")
	m.Apply(r)
	if got := r.Header.Get("Cookie"); got != "sid=abc; pref=dark; token=t0k" {
		t.Errorf("Cookie = %q", got)
	}

	if _, err := m.ImportCookiesTxt("https://nothing.example", txt); err == nil {
		t.Error("expected error when no cookies match")
	}
}

func TestOrigin(t *testing.T) {
	u, _ := url.Parse("HTTPS://Example.COM:443/x")
	if got := Origin(u); got != "https://example.com" {
		t.Errorf("Origin = %q", got)
	}
}
