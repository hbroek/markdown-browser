// Package localfile serves local images to the webview for pages opened from disk.
package localfile

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// Prefix is the URL path the handler answers on.
const Prefix = "/_local"

var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// URL returns the webview URL that serves the local file at path.
func URL(path string) string {
	return Prefix + "?path=" + url.QueryEscape(path)
}

// Handler serves image files only, by absolute path.
var Handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != Prefix || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	path := r.URL.Query().Get("path")
	ct, ok := imageTypes[strings.ToLower(filepath.Ext(path))]
	if !ok || !filepath.IsAbs(path) {
		http.Error(w, "not an image", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	http.ServeFile(w, r, path)
})
