// Package auth stores per-site credentials in the OS keychain and attaches
// them to outgoing requests for exactly the origin they were saved for.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Kind string

const (
	Cookie Kind = "cookie" // raw Cookie header value
	Bearer Kind = "bearer"
	Basic  Kind = "basic"
)

// Credential is what the user saves for one origin. It never goes back to the
// frontend; the frontend only sees Info.
type Credential struct {
	Origin   string `json:"origin"`
	Kind     Kind   `json:"kind"`
	Cookie   string `json:"cookie,omitempty"`
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// Info is the non-secret description of a saved credential.
type Info struct {
	Origin   string `json:"origin"`
	Kind     Kind   `json:"kind"`
	Username string `json:"username,omitempty"`
}

// Manager implements fetch.Authorizer. Secrets live in the keychain; a small
// index of origins (no secrets) lives in a file so they can be listed.
type Manager struct {
	mu        sync.Mutex
	secrets   Secrets
	indexPath string // empty keeps the index in memory
	index     map[string]Info
	cache     map[string]Credential
}

func NewManager(secrets Secrets, indexPath string) *Manager {
	m := &Manager{secrets: secrets, indexPath: indexPath, index: map[string]Info{}, cache: map[string]Credential{}}
	if data, err := os.ReadFile(indexPath); err == nil {
		var infos []Info
		if json.Unmarshal(data, &infos) == nil {
			for _, i := range infos {
				m.index[i.Origin] = i
			}
		}
	}
	return m
}

// Origin returns scheme://host[:port] with default ports dropped.
func Origin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host
}

// NormalizeOrigin accepts "example.com", "https://example.com/path", etc.
func NormalizeOrigin(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", errors.New("site is required")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid site %q", input)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	return Origin(u), nil
}

// Allowed reports whether credentials may be sent to origin: https, or http
// only for loopback hosts, so secrets never cross the network in clear text.
func Allowed(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && IsLoopbackHost(u.Hostname())
}

// IsLoopbackHost reports whether host is localhost, *.localhost or a loopback IP.
func IsLoopbackHost(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func secretName(origin string) string { return "cred:" + origin }

func hasControl(s string) bool { return strings.ContainsAny(s, "\r\n\x00") }

func (c Credential) validate() error {
	for _, v := range []string{c.Cookie, c.Token, c.Username, c.Password} {
		if hasControl(v) {
			return errors.New("values must not contain line breaks")
		}
	}
	switch c.Kind {
	case Cookie:
		if strings.TrimSpace(c.Cookie) == "" {
			return errors.New("cookie value is required")
		}
	case Bearer:
		if strings.TrimSpace(c.Token) == "" {
			return errors.New("token is required")
		}
	case Basic:
		if c.Username == "" {
			return errors.New("username is required")
		}
		if strings.Contains(c.Username, ":") {
			return errors.New("username must not contain ':'")
		}
	default:
		return fmt.Errorf("unknown credential type %q", c.Kind)
	}
	return nil
}

// Save stores c in the keychain and returns the normalized origin.
func (m *Manager) Save(c Credential) (string, error) {
	origin, err := NormalizeOrigin(c.Origin)
	if err != nil {
		return "", err
	}
	if !Allowed(origin) {
		return "", errors.New("credentials are only sent over https (or http on localhost)")
	}
	c.Origin = origin
	if err := c.validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.secrets.Set(secretName(origin), string(data)); err != nil {
		return "", fmt.Errorf("cannot write to the keychain: %w", err)
	}
	m.cache[origin] = c
	m.index[origin] = Info{Origin: origin, Kind: c.Kind, Username: c.Username}
	m.saveIndex()
	return origin, nil
}

func (m *Manager) Delete(origin string) error {
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.secrets.Delete(secretName(origin)); err != nil {
		return err
	}
	delete(m.cache, origin)
	delete(m.index, origin)
	m.saveIndex()
	return nil
}

// List returns the saved credentials without their secrets.
func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.index))
	for _, i := range m.index {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Origin < out[b].Origin })
	return out
}

func (m *Manager) lookup(origin string) (Credential, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.index[origin]; !ok {
		return Credential{}, false
	}
	if c, ok := m.cache[origin]; ok {
		return c, true
	}
	raw, err := m.secrets.Get(secretName(origin))
	if err != nil {
		return Credential{}, false
	}
	var c Credential
	if json.Unmarshal([]byte(raw), &c) != nil {
		return Credential{}, false
	}
	m.cache[origin] = c
	return c, true
}

// Apply attaches the saved credential, if any, for req's exact origin.
func (m *Manager) Apply(req *http.Request) {
	origin := Origin(req.URL)
	if !Allowed(origin) {
		return
	}
	c, ok := m.lookup(origin)
	if !ok {
		return
	}
	switch c.Kind {
	case Cookie:
		req.Header.Set("Cookie", c.Cookie) // the cookie jar appends its own
	case Bearer:
		req.Header.Set("Authorization", "Bearer "+c.Token)
	case Basic:
		req.SetBasicAuth(c.Username, c.Password)
	}
}

func (m *Manager) saveIndex() {
	if m.indexPath == "" {
		return
	}
	infos := make([]Info, 0, len(m.index))
	for _, i := range m.index {
		infos = append(infos, i)
	}
	sort.Slice(infos, func(a, b int) bool { return infos[a].Origin < infos[b].Origin })
	data, err := json.MarshalIndent(infos, "", "  ")
	if err != nil || os.MkdirAll(filepath.Dir(m.indexPath), 0o755) != nil {
		return
	}
	tmp := m.indexPath + ".tmp"
	if os.WriteFile(tmp, data, 0o600) != nil {
		return
	}
	if os.Rename(tmp, m.indexPath) != nil {
		os.Remove(tmp)
	}
}

// ImportCookiesTxt turns a Netscape cookies.txt export into a Cookie
// credential for origin, keeping only the cookies that belong to that host.
// It returns how many cookies were imported.
func (m *Manager) ImportCookiesTxt(origin, text string) (int, error) {
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return 0, err
	}
	u, _ := url.Parse(origin)
	host, secure := u.Hostname(), u.Scheme == "https"

	var pairs []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimPrefix(line, "#HttpOnly_")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 7 {
			continue
		}
		domain := strings.ToLower(strings.TrimPrefix(f[0], "."))
		if host != domain && !strings.HasSuffix(host, "."+domain) {
			continue
		}
		if strings.EqualFold(f[3], "TRUE") && !secure {
			continue
		}
		if exp, err := strconv.ParseInt(f[4], 10, 64); err == nil && exp != 0 && time.Unix(exp, 0).Before(time.Now()) {
			continue
		}
		name, value := f[5], strings.Join(f[6:], "\t")
		if name == "" || hasControl(name) || hasControl(value) {
			continue
		}
		pairs = append(pairs, name+"="+value)
	}
	if len(pairs) == 0 {
		return 0, fmt.Errorf("no unexpired cookies for %s in that file", host)
	}
	_, err = m.Save(Credential{Origin: origin, Kind: Cookie, Cookie: strings.Join(pairs, "; ")})
	return len(pairs), err
}
