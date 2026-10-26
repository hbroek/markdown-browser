package bookmarks

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

func init() {
	Register("memory", func(Config) (Store, error) { return NewMemory(), nil })
}

// Memory is a Store that keeps everything in process memory; used by tests
// and as a reference for backend behavior.
type Memory struct {
	mu        sync.Mutex
	next      int
	folders   map[string]Folder
	bookmarks map[string]Bookmark
}

func NewMemory() *Memory {
	return &Memory{folders: map[string]Folder{}, bookmarks: map[string]Bookmark{}}
}

func (m *Memory) newID() string {
	m.next++
	return strconv.Itoa(m.next)
}

func (m *Memory) hasFolder(id string) bool {
	_, ok := m.folders[id]
	return id == "" || ok
}

func (m *Memory) Folders(Context) ([]Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Folder, 0, len(m.folders))
	for _, f := range m.folders {
		out = append(out, f)
	}
	SortFolders(out)
	return out, nil
}

func (m *Memory) Bookmarks(_ Context, folderID string) ([]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasFolder(folderID) {
		return nil, fmt.Errorf("folder %s: %w", folderID, ErrNotFound)
	}
	out := []Bookmark{}
	for _, b := range m.bookmarks {
		if b.FolderID == folderID {
			out = append(out, b)
		}
	}
	SortBookmarks(out)
	return out, nil
}

func (m *Memory) Bookmark(_ Context, id string) (Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bookmarks[id]
	if !ok {
		return b, fmt.Errorf("bookmark %s: %w", id, ErrNotFound)
	}
	return b, nil
}

func (m *Memory) FindByURL(_ Context, url string) (Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := Key(url)
	var found []Bookmark
	for _, b := range m.bookmarks {
		if Key(b.URL) == key {
			found = append(found, b)
		}
	}
	if len(found) == 0 {
		return Bookmark{}, fmt.Errorf("%s: %w", url, ErrNotFound)
	}
	SortBookmarks(found)
	return found[0], nil
}

func (m *Memory) SaveBookmark(_ Context, b Bookmark) (Bookmark, error) {
	b, err := Clean(b)
	if err != nil {
		return b, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasFolder(b.FolderID) {
		return b, fmt.Errorf("folder %s: %w", b.FolderID, ErrNotFound)
	}
	now := time.Now().UTC()
	if b.ID == "" {
		b.ID = m.newID()
		b.Created = now
	} else {
		old, ok := m.bookmarks[b.ID]
		if !ok {
			return b, fmt.Errorf("bookmark %s: %w", b.ID, ErrNotFound)
		}
		b.Created = old.Created
	}
	b.Updated = now
	m.bookmarks[b.ID] = b
	return b, nil
}

func (m *Memory) DeleteBookmark(_ Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.bookmarks[id]; !ok {
		return fmt.Errorf("bookmark %s: %w", id, ErrNotFound)
	}
	delete(m.bookmarks, id)
	return nil
}

func (m *Memory) SaveFolder(_ Context, f Folder) (Folder, error) {
	f, err := CleanFolder(f)
	if err != nil {
		return f, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasFolder(f.ParentID) {
		return f, fmt.Errorf("folder %s: %w", f.ParentID, ErrNotFound)
	}
	if f.ID == "" {
		f.ID = m.newID()
	} else {
		if _, ok := m.folders[f.ID]; !ok {
			return f, fmt.Errorf("folder %s: %w", f.ID, ErrNotFound)
		}
		// Walk up from the new parent: reaching f means a cycle.
		for p := f.ParentID; p != ""; p = m.folders[p].ParentID {
			if p == f.ID {
				return f, fmt.Errorf("%w: a folder cannot move into itself", ErrInvalid)
			}
		}
	}
	m.folders[f.ID] = f
	return f, nil
}

func (m *Memory) DeleteFolder(_ Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.folders[id]; !ok {
		return fmt.Errorf("folder %s: %w", id, ErrNotFound)
	}
	doomed := map[string]bool{id: true}
	for grew := true; grew; {
		grew = false
		for _, f := range m.folders {
			if doomed[f.ParentID] && !doomed[f.ID] {
				doomed[f.ID] = true
				grew = true
			}
		}
	}
	for fid := range doomed {
		delete(m.folders, fid)
	}
	for bid, b := range m.bookmarks {
		if doomed[b.FolderID] {
			delete(m.bookmarks, bid)
		}
	}
	return nil
}

func (m *Memory) Search(_ Context, terms []string) ([]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Bookmark{}
	for _, b := range m.bookmarks {
		if Matches(b, terms) {
			out = append(out, b)
		}
	}
	SortBookmarks(out)
	return out, nil
}

func (m *Memory) Close() error { return nil }
