// Package chats keeps the "Ask this page" conversations, one per page, so they
// are still there when the page or the app is opened again.
package chats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	MaxPages = 50 // pages whose conversation is kept; the least recently used go first
	MaxTurns = 60 // messages kept per page
)

// Turn is one message. Only the text is kept: the HTML shown for an answer is
// made again from it when it is loaded, so the file never holds markup that is displayed as is.
type Turn struct {
	Role   string `json:"role"` // "user" or "assistant"
	Text   string `json:"text"`
	Source string `json:"source,omitempty"` // the model that wrote an answer
	Millis int64  `json:"millis,omitempty"`
}

type entry struct {
	Updated time.Time `json:"updated"`
	Seq     uint64    `json:"seq"` // grows with every save; the clock can tie, this cannot
	Turns   []Turn    `json:"turns"`
}

// Store maps a page address to its conversation, optionally saved to a file.
type Store struct {
	mu      sync.Mutex
	path    string // empty keeps everything in memory
	entries map[string]entry
	seq     uint64
}

// Open loads the file at path. A missing or damaged file gives an empty store.
func Open(path string) *Store {
	s := &Store{path: path, entries: map[string]entry{}}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &s.entries) != nil || s.entries == nil {
			s.entries = map[string]entry{}
		}
	}
	for _, e := range s.entries {
		if e.Seq > s.seq {
			s.seq = e.Seq
		}
	}
	return s
}

// key is the page without its #fragment: the same page whichever heading was linked.
func key(url string) string {
	u, _, _ := strings.Cut(strings.TrimSpace(url), "#")
	return u
}

// Get returns the saved conversation for a page, oldest message first.
func (s *Store) Get(url string) []Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Turn(nil), s.entries[key(url)].Turns...)
}

// Put saves the conversation for a page. Messages other than a user's or the
// assistant's, and empty ones, are dropped; an empty conversation deletes the entry.
func (s *Store) Put(url string, turns []Turn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []Turn
	for _, t := range turns {
		if (t.Role == "user" || t.Role == "assistant") && strings.TrimSpace(t.Text) != "" {
			kept = append(kept, t)
		}
	}
	if len(kept) > MaxTurns {
		kept = kept[len(kept)-MaxTurns:]
	}
	if len(kept) == 0 {
		delete(s.entries, key(url))
	} else {
		s.seq++
		s.entries[key(url)] = entry{Updated: time.Now(), Seq: s.seq, Turns: kept}
	}
	s.trim()
	s.save()
}

// Delete forgets one page's conversation.
func (s *Store) Delete(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key(url))
	s.save()
}

// Clear forgets every conversation.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = map[string]entry{}
	s.save()
}

// Len is the number of pages with a saved conversation.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *Store) trim() {
	if len(s.entries) <= MaxPages {
		return
	}
	urls := make([]string, 0, len(s.entries))
	for u := range s.entries {
		urls = append(urls, u)
	}
	sort.Slice(urls, func(i, j int) bool { return s.entries[urls[i]].Seq > s.entries[urls[j]].Seq }) // newest first
	for _, u := range urls[MaxPages:] {
		delete(s.entries, u)
	}
}

// save writes the file atomically. Failures are ignored: saving is a convenience.
func (s *Store) save() {
	if s.path == "" {
		return
	}
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil || os.MkdirAll(filepath.Dir(s.path), 0o755) != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) != nil { // private: it holds what the user asked about
		return
	}
	if os.Rename(tmp, s.path) != nil {
		os.Remove(tmp)
	}
}
