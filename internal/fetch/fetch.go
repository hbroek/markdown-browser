// Package fetch loads documents over http, https and file.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// Accept asks servers for markdown first.
	Accept = "text/markdown, text/x-markdown;q=0.9, application/rss+xml;q=0.5, application/atom+xml;q=0.5, text/plain;q=0.3, */*;q=0.1"

	MaxBody = 10 << 20
	Timeout = 15 * time.Second

	userAgent = "markdown-browser/0.1"
)

// Result is a fetched document.
type Result struct {
	URL         string // final URL after redirects
	Status      int
	ContentType string
	Body        []byte

	WWWAuthenticate string // challenge sent with a 401, if any
}

// Authorizer attaches saved credentials to a request.
type Authorizer interface {
	Apply(req *http.Request)
}

// Fetcher loads documents, sending saved credentials and cookies.
type Fetcher struct {
	client *http.Client
	auth   Authorizer
}

// NewFetcher returns a Fetcher. auth and jar may be nil.
func NewFetcher(auth Authorizer, jar http.CookieJar) *Fetcher {
	f := &Fetcher{auth: auth}
	f.client = &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			// Credentials belong to one origin: drop them, then add whatever
			// is saved for the redirect target.
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			if auth != nil {
				auth.Apply(req)
			}
			return nil
		},
	}
	return f
}

var defaultFetcher = NewFetcher(nil, nil)

// Normalize turns address bar input into an absolute http, https or file URL.
func Normalize(input string) (*url.URL, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, errors.New("empty address")
	}
	if strings.HasPrefix(s, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		s = filepath.Join(home, s[2:])
	}
	if filepath.IsAbs(s) || strings.HasPrefix(s, "/") {
		return fileURL(s), nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http", "https":
		if u.Host == "" {
			return nil, fmt.Errorf("missing host in %q", input)
		}
	case "file":
	default:
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	return u, nil
}

func fileURL(path string) *url.URL {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") { // Windows drive path: C:/x -> /C:/x
		p = "/" + p
	}
	return &url.URL{Scheme: "file", Path: p}
}

// FilePath returns the local filesystem path for a file URL.
func FilePath(u *url.URL) string {
	return filePathFor(runtime.GOOS, u)
}

func filePathFor(goos string, u *url.URL) string {
	p := u.Path
	if goos == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:] // /C:/x -> C:/x
	}
	if goos == "windows" {
		p = strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

// Get fetches rawURL with no credentials or cookies.
func Get(ctx context.Context, rawURL string) (*Result, error) {
	return defaultFetcher.Get(ctx, rawURL)
}

// Get fetches rawURL. A non-2xx status is not an error; the caller decides.
func (f *Fetcher) Get(ctx context.Context, rawURL string) (*Result, error) {
	u, err := Normalize(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "file" {
		return getFile(u)
	}
	return f.getHTTP(ctx, u)
}

func (f *Fetcher) getHTTP(ctx context.Context, u *url.URL) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", Accept)
	req.Header.Set("User-Agent", userAgent)
	if f.auth != nil {
		f.auth.Apply(req)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readCapped(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Result{
		URL:         resp.Request.URL.String(),
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,

		WWWAuthenticate: resp.Header.Get("Www-Authenticate"),
	}, nil
}

func getFile(u *url.URL) (*Result, error) {
	f, err := os.Open(FilePath(u))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	body, err := readCapped(f)
	if err != nil {
		return nil, err
	}
	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(u.Path)))
	if isMarkdownExt(u.Path) {
		ct = "text/markdown; charset=utf-8"
	}
	return &Result{URL: u.String(), Status: http.StatusOK, ContentType: ct, Body: body}, nil
}

func readCapped(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBody {
		return nil, fmt.Errorf("document larger than %d MB", MaxBody>>20)
	}
	return body, nil
}

// IsMarkdown reports whether a response should be rendered as markdown.
// text/plain counts, because many hosts (e.g. raw GitHub) serve .md that way.
func IsMarkdown(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mt {
	case "text/markdown", "text/x-markdown", "text/plain":
		return true
	}
	return false
}

func isMarkdownExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	}
	return false
}
