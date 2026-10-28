package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Editable says whether the server of a response lets the document be replaced with PUT,
// and where: the Link rel="edit" address when it is on the same server, else the address
// the page came from. It is "" when the page cannot be edited. Allow only says what the
// resource supports; whether this user may is only known when saving.
func Editable(res *Result) string {
	if res == nil || res.Status >= 400 || res.Header == nil {
		return ""
	}
	base, err := url.Parse(res.URL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") {
		return ""
	}
	if !allows(res.Header, http.MethodPut) {
		return ""
	}
	for _, link := range res.Header.Values("Link") {
		for _, l := range parseLinks(link) {
			if !hasRel(l.rel, "edit") {
				continue
			}
			target, err := base.Parse(l.href)
			if err == nil && target.Scheme == base.Scheme && target.Host == base.Host {
				return target.String()
			}
		}
	}
	return res.URL
}

func allows(h http.Header, method string) bool {
	for _, v := range h.Values("Allow") {
		for _, m := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(m), method) {
				return true
			}
		}
	}
	return false
}

type link struct{ href, rel string }

// parseLinks reads a Link header value: <href>; rel="edit", <href2>; rel=next.
func parseLinks(v string) []link {
	var out []link
	for _, part := range splitLinks(v) {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "<") {
			continue
		}
		end := strings.Index(part, ">")
		if end < 0 {
			continue
		}
		l := link{href: part[1:end]}
		for _, param := range strings.Split(part[end+1:], ";") {
			k, val, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(strings.TrimSpace(k), "rel") {
				l.rel = strings.Trim(strings.TrimSpace(val), `"`)
			}
		}
		out = append(out, l)
	}
	return out
}

// splitLinks splits on the commas between links, not those inside <...> or quotes.
func splitLinks(v string) []string {
	var parts []string
	depth, quoted, start := false, false, 0
	for i, r := range v {
		switch {
		case r == '"':
			quoted = !quoted
		case quoted:
		case r == '<':
			depth = true
		case r == '>':
			depth = false
		case r == ',' && !depth:
			parts = append(parts, v[start:i])
			start = i + 1
		}
	}
	return append(parts, v[start:])
}

func hasRel(rels, want string) bool {
	for _, r := range strings.Fields(rels) {
		if strings.EqualFold(r, want) {
			return true
		}
	}
	return false
}

// ErrChanged means the document changed on the server after it was loaded.
var ErrChanged = errors.New("the document changed on the server after you opened it")

// Put replaces the document at rawURL with body, sending the saved credentials. etag is the
// ETag the page was loaded with; the server refuses the save if the document has changed since.
func (f *Fetcher) Put(ctx context.Context, rawURL string, body []byte, etag string) error {
	u, err := Normalize(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http and https documents can be saved")
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/markdown; charset=utf-8")
	req.Header.Set("User-Agent", userAgent)
	if etag != "" && !strings.HasPrefix(etag, "W/") { // If-Match needs a strong tag
		req.Header.Set("If-Match", etag)
	}
	if f.auth != nil {
		f.auth.Apply(req)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.Request.Method != http.MethodPut { // a redirect turned it into a GET: nothing was saved
		return fmt.Errorf("the server redirected the save to %s, which is not supported", resp.Request.URL)
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusPreconditionFailed:
		return ErrChanged
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("the server refused the save (%d): saved credentials for this site may be missing or lack write access", resp.StatusCode)
	}
	return fmt.Errorf("the server answered %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
}
