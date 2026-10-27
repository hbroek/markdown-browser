package fetch

import (
	"path/filepath"
	"strings"
	"unicode"

	"markdown-browser/internal/auth"
)

// SearchQuery decides whether address bar input is a bookmark search rather
// than an address, and returns the query. It is a search when it starts with
// "?", when it has whitespace (unless it is a URL or an absolute path), or
// when it is a single bare word such as "rust" - no dot, colon or slash, and
// not a loopback name like "localhost".
func SearchQuery(input string) (string, bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "?") {
		return strings.TrimSpace(s[1:]), true
	}
	if strings.Contains(s, "://") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") || filepath.IsAbs(s) {
		return "", false
	}
	if strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		return strings.Join(strings.Fields(s), " "), true
	}
	if strings.ContainsAny(s, ".:/") || auth.IsLoopbackHost(s) {
		return "", false
	}
	return s, true
}

// MeaningQuery decides whether address bar input asks for a search by meaning:
// it starts with "@", as in "@that article about async rust". It returns what
// follows, which may be empty.
func MeaningQuery(input string) (string, bool) {
	s := strings.TrimSpace(input)
	if !strings.HasPrefix(s, "@") {
		return "", false
	}
	return strings.Join(strings.Fields(s[1:]), " "), true
}
