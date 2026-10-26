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
	AI        AI      `json:"ai"`
}

// AI is the language model provider (see package ai). The API key is not here:
// it is kept in the system keychain.
type AI struct {
	Preset  string `json:"preset"` // id of an ai.Preset, "custom" for any other
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	// Consent is the origin the user agreed to send page text to. Servers on
	// this computer need no consent.
	Consent string `json:"consent"`
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
	return Resolve(s, dir)
}

// LoadRaw reads path without filling in defaults, so a save does not pin them.
func LoadRaw(path string) Settings {
	var s Settings
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

// Resolve fills in defaults (the SQLite file under dir) without changing s.
func Resolve(s Settings, dir string) Settings {
	cfg := bookmarks.Config{}
	for k, v := range s.Bookmarks.Config {
		cfg[k] = v
	}
	s.Bookmarks.Config = cfg
	if s.Bookmarks.Backend == "" {
		s.Bookmarks.Backend = "sqlite"
	}
	if s.Bookmarks.Backend == "sqlite" && cfg["path"] == "" {
		cfg["path"] = filepath.Join(dir, "bookmarks.db")
	}
	return s
}

// Save writes s to path, replacing the file in one step.
func Save(path string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
