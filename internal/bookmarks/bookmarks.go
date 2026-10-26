// Package bookmarks defines saved pages, folders and the Store interface that
// backends (SQLite, LinkaGoGo, ...) implement.
package bookmarks

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
)

// Folder groups bookmarks and other folders. An empty ParentID is the root.
// IDs are opaque strings so remote backends can use their own.
type Folder struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Name     string `json:"name"`
}

// Bookmark is a saved page. An empty FolderID is the root.
type Bookmark struct {
	ID          string    `json:"id"`
	FolderID    string    `json:"folderId"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Keywords    []string  `json:"keywords"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}

// Store is a bookmark backend. Callers treat IDs as opaque. Stores set
// Created and Updated; they do not enforce one bookmark per URL (callers use
// FindByURL first).
type Store interface {
	// Folders returns every folder, flat; callers build the hierarchy from ParentID.
	Folders(ctx Context) ([]Folder, error)
	// Bookmarks returns the bookmarks directly in a folder ("" is the root), by title.
	Bookmarks(ctx Context, folderID string) ([]Bookmark, error)
	Bookmark(ctx Context, id string) (Bookmark, error)
	// FindByURL matches on Key(url) and returns ErrNotFound when none is saved.
	FindByURL(ctx Context, url string) (Bookmark, error)
	// SaveBookmark creates the bookmark when ID is empty, otherwise updates it.
	SaveBookmark(ctx Context, b Bookmark) (Bookmark, error)
	DeleteBookmark(ctx Context, id string) error
	// SaveFolder creates the folder when ID is empty, otherwise renames or moves
	// it. Moving a folder into itself or a descendant is ErrInvalid.
	SaveFolder(ctx Context, f Folder) (Folder, error)
	// DeleteFolder removes the folder with all its subfolders and bookmarks.
	DeleteFolder(ctx Context, id string) error
	// Search returns bookmarks matching every term (see Matches), by title.
	Search(ctx Context, terms []string) ([]Bookmark, error)
	Close() error
}

// Clean validates and normalizes a bookmark before it is saved: trims text,
// requires an http, https or file URL, defaults the title to the URL, and
// drops empty and duplicate keywords (inner whitespace is collapsed).
func Clean(b Bookmark) (Bookmark, error) {
	b.URL = strings.TrimSpace(b.URL)
	if b.URL == "" {
		return b, fmt.Errorf("%w: url is required", ErrInvalid)
	}
	u, err := url.Parse(b.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "file") {
		return b, fmt.Errorf("%w: url must be http, https or file", ErrInvalid)
	}
	if (u.Scheme == "http" || u.Scheme == "https") && u.Host == "" {
		return b, fmt.Errorf("%w: url has no host", ErrInvalid)
	}
	b.Title = strings.TrimSpace(b.Title)
	if b.Title == "" {
		b.Title = b.URL
	}
	b.Description = strings.TrimSpace(b.Description)
	seen := map[string]bool{}
	keywords := []string{}
	for _, k := range b.Keywords {
		k = strings.Join(strings.Fields(k), " ") // also drops line breaks
		if k == "" || seen[strings.ToLower(k)] {
			continue
		}
		seen[strings.ToLower(k)] = true
		keywords = append(keywords, k)
	}
	b.Keywords = keywords
	return b, nil
}

// CleanFolder trims and validates a folder name.
func CleanFolder(f Folder) (Folder, error) {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return f, fmt.Errorf("%w: folder name is required", ErrInvalid)
	}
	return f, nil
}

// Key reduces a URL to the form used to decide whether two bookmarks are the
// same page: lower-case scheme and host, no fragment, "/" for an empty path.
func Key(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Path == "" && u.Host != "" {
		u.Path = "/"
	}
	return u.String()
}

// Matches reports whether every term appears (case-insensitively) in the
// bookmark's title, URL, description or one of its keywords.
func Matches(b Bookmark, terms []string) bool {
	hay := strings.ToLower(strings.Join(append([]string{b.Title, b.URL, b.Description}, b.Keywords...), "\n"))
	for _, t := range terms {
		if !strings.Contains(hay, strings.ToLower(t)) {
			return false
		}
	}
	return true
}

// SortBookmarks orders bookmarks by title, then ID.
func SortBookmarks(list []Bookmark) {
	sort.Slice(list, func(i, j int) bool {
		a, b := strings.ToLower(list[i].Title), strings.ToLower(list[j].Title)
		if a != b {
			return a < b
		}
		return list[i].ID < list[j].ID
	})
}

// SortFolders orders folders by name, then ID.
func SortFolders(list []Folder) {
	sort.Slice(list, func(i, j int) bool {
		a, b := strings.ToLower(list[i].Name), strings.ToLower(list[j].Name)
		if a != b {
			return a < b
		}
		return list[i].ID < list[j].ID
	})
}

// SaveUnique saves b, keeping one bookmark per page: a new bookmark whose URL
// is already saved updates that bookmark instead of adding a second one.
func SaveUnique(ctx Context, s Store, b Bookmark) (Bookmark, error) {
	if b.ID == "" {
		if existing, err := s.FindByURL(ctx, b.URL); err == nil {
			b.ID = existing.ID
		} else if !errors.Is(err, ErrNotFound) {
			return b, err
		}
	}
	return s.SaveBookmark(ctx, b)
}
