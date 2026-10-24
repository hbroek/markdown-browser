// Package history keeps the visit log shown in the history dropdown.
package history

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const MaxEntries = 200

// Entry is one visited page.
type Entry struct {
	URL   string    `json:"url"`
	Title string    `json:"title"`
	Time  time.Time `json:"time"`
}

// Log is a most-recent-first list of visited pages, optionally saved to a file.
type Log struct {
	mu      sync.Mutex
	path    string // empty keeps the log in memory only
	entries []Entry
}

// Open loads the log at path. A missing or corrupt file gives an empty log.
func Open(path string) *Log {
	l := &Log{path: path}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &l.entries) != nil {
			l.entries = nil
		}
	}
	if len(l.entries) > MaxEntries {
		l.entries = l.entries[:MaxEntries]
	}
	return l
}

// DefaultPath is where the log lives in the user's config directory.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "markdown-browser", "history.json"), nil
}

// Add records a visit, moving an existing entry for the same URL to the top.
func (l *Log) Add(url, title string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if url == l.markdownURL() {
		return // the generated history page is not itself history
	}
	kept := make([]Entry, 0, len(l.entries)+1)
	kept = append(kept, Entry{URL: url, Title: title, Time: time.Now()})
	for _, e := range l.entries {
		if e.URL != url {
			kept = append(kept, e)
		}
	}
	if len(kept) > MaxEntries {
		kept = kept[:MaxEntries]
	}
	l.entries = kept
	l.save()
}

// Entries returns a copy of the log, most recent first.
func (l *Log) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Entry{}, l.entries...)
}

func (l *Log) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
	l.save()
}

// save writes the log atomically. Failures are ignored: history is a convenience.
func (l *Log) save() {
	if l.path == "" {
		return
	}
	data, err := json.MarshalIndent(l.entries, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(l.path), 0o755) != nil {
		return
	}
	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) != nil {
		return
	}
	if os.Rename(tmp, l.path) != nil {
		os.Remove(tmp)
	}
}

// markdownPath is where the full history page is written: next to the log, or
// in the temp directory for an in-memory log.
func (l *Log) markdownPath() string {
	if l.path == "" {
		return filepath.Join(os.TempDir(), "markdown-browser-history.md")
	}
	return filepath.Join(filepath.Dir(l.path), "history.md")
}

func (l *Log) markdownURL() string {
	p := filepath.ToSlash(l.markdownPath())
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// WriteMarkdown writes the full history as a markdown file and returns its path.
func (l *Log) WriteMarkdown() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	path := l.markdownPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(Markdown(l.entries)), 0o644)
}

// Markdown formats entries (most recent first) as a document grouped by day.
func Markdown(entries []Entry) string {
	var b strings.Builder
	b.WriteString("# History\n\n")
	if len(entries) == 0 {
		b.WriteString("No pages visited yet.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%d pages, most recent first.\n", len(entries))
	day := ""
	for _, e := range entries {
		local := e.Time.Local()
		if d := local.Format("Monday, 2 January 2006"); d != day {
			day = d
			fmt.Fprintf(&b, "\n## %s\n\n", day)
		}
		title := strings.Join(strings.Fields(e.Title), " ")
		if title == "" {
			title = e.URL
		}
		fmt.Fprintf(&b, "- %s [%s](<%s>)\n", local.Format("15:04"), mdEscape(title), e.URL)
	}
	return b.String()
}

var mdSpecial = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "&", `\&`, "~", `\~`, "|", `\|`,
)

func mdEscape(s string) string { return mdSpecial.Replace(s) }
