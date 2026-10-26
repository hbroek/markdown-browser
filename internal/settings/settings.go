// Package settings reads the small user config file (settings.json).
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"markdown-browser/internal/bookmarks"
)

// Settings is the content of settings.json. A missing or unreadable file
// gives the defaults, so the file is only needed to change something.
type Settings struct {
	Bookmarks Backend `json:"bookmarks"`
}

// Backend names a bookmark backend and its settings, e.g.
// {"backend": "sqlite", "config": {"path": "/some/where/bookmarks.db"}}.
type Backend struct {
	Backend string           `json:"backend"`
	Config  bookmarks.Config `json:"config"`
}

// Load reads path and fills in defaults. dir is the app's config directory,
// used for the default SQLite file.
func Load(path, dir string) Settings {
	var s Settings
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s) // keep what parsed; fall back to defaults for the rest
	}
	if s.Bookmarks.Backend == "" {
		s.Bookmarks.Backend = "sqlite"
	}
	if s.Bookmarks.Backend == "sqlite" {
		if s.Bookmarks.Config == nil {
			s.Bookmarks.Config = bookmarks.Config{}
		}
		if s.Bookmarks.Config["path"] == "" {
			s.Bookmarks.Config["path"] = filepath.Join(dir, "bookmarks.db")
		}
	}
	return s
}
