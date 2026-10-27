// Package imgproxy serves images from web servers the webview cannot reach on its
// own: plain http addresses, which it refuses to load, and servers on this computer
// such as name.localhost that want the credentials saved for them.
package imgproxy

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"markdown-browser/internal/auth"
	"markdown-browser/internal/fetch"
)

// Prefix is the URL path the handler answers on.
const Prefix = "/_img"

// Wanted reports whether an image at u should be loaded through the proxy.
func Wanted(u *url.URL) bool {
	return u.Scheme == "http" || (u.Scheme == "https" && auth.IsLoopbackHost(u.Hostname()))
}

// URL returns the webview URL that serves the image at raw.
func URL(raw string) string {
	return Prefix + "?url=" + url.QueryEscape(raw)
}

// Handler fetches images with f, so they carry the credentials saved for their server.
func Handler(f *fetch.Fetcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != Prefix || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		res, err := f.GetImage(r.Context(), r.URL.Query().Get("url"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if res.Status < 200 || res.Status > 299 {
			http.Error(w, "the server answered "+strconv.Itoa(res.Status), http.StatusBadGateway)
			return
		}
		ct := res.ContentType
		if !strings.HasPrefix(strings.ToLower(ct), "image/") {
			http.Error(w, "not an image", http.StatusUnsupportedMediaType)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'") // an SVG must not run anything
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.Write(res.Body)
	})
}
