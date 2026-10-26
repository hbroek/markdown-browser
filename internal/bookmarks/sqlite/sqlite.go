// Package sqlite is a bookmarks.Store backed by a local SQLite file, using the
// pure-Go modernc driver so cross-compiling needs no C toolchain.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"markdown-browser/internal/bookmarks"

	_ "modernc.org/sqlite"
)

func init() {
	bookmarks.Register("sqlite", func(cfg bookmarks.Config, _ bookmarks.Deps) (bookmarks.Store, error) {
		path := cfg["path"]
		if path == "" {
			return nil, errors.New("sqlite backend needs a path")
		}
		return Open(path)
	})
}

// migrations run in order; the index+1 is stored in PRAGMA user_version.
var migrations = []string{
	`CREATE TABLE folders (
		id        INTEGER PRIMARY KEY,
		parent_id INTEGER REFERENCES folders(id) ON DELETE CASCADE,
		name      TEXT NOT NULL
	);
	CREATE TABLE bookmarks (
		id          INTEGER PRIMARY KEY,
		folder_id   INTEGER REFERENCES folders(id) ON DELETE CASCADE,
		title       TEXT NOT NULL,
		url         TEXT NOT NULL,
		url_key     TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		keywords    TEXT NOT NULL DEFAULT '', -- one per line
		created     TEXT NOT NULL,
		updated     TEXT NOT NULL
	);
	CREATE INDEX bookmarks_folder ON bookmarks(folder_id);
	CREATE INDEX bookmarks_url_key ON bookmarks(url_key);
	CREATE INDEX folders_parent ON folders(parent_id);`,
}

type Store struct{ db *sql.DB }

// Open opens (creating if needed) the database at path; ":memory:" gives a
// throwaway database.
func Open(path string) (*Store, error) {
	dsn := "file:" + url.PathEscape(path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: pragmas are per connection and writes are serialized anyway.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("bookmarks database %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for ; version < len(migrations); version++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[version]); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// IDs are integers in the database and opaque strings outside it.

func notFound(kind, id string) error { return fmt.Errorf("%s %s: %w", kind, id, bookmarks.ErrNotFound) }

// dbID converts an ID to a nullable column value; "" is NULL (the root).
func dbID(kind, id string) (sql.NullInt64, error) {
	if id == "" {
		return sql.NullInt64{}, nil
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return sql.NullInt64{}, notFound(kind, id)
	}
	return sql.NullInt64{Int64: n, Valid: true}, nil
}

func strID(n sql.NullInt64) string {
	if !n.Valid {
		return ""
	}
	return strconv.FormatInt(n.Int64, 10)
}

func (s *Store) folderExists(q interface {
	QueryRow(string, ...any) *sql.Row
}, id string) (bool, error) {
	n, err := dbID("folder", id)
	if err != nil {
		return false, nil
	}
	if !n.Valid {
		return true, nil
	}
	var one int
	err = q.QueryRow("SELECT 1 FROM folders WHERE id = ?", n.Int64).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) Folders(bookmarks.Context) ([]bookmarks.Folder, error) {
	rows, err := s.db.Query("SELECT id, parent_id, name FROM folders")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []bookmarks.Folder{}
	for rows.Next() {
		var id int64
		var parent sql.NullInt64
		var f bookmarks.Folder
		if err := rows.Scan(&id, &parent, &f.Name); err != nil {
			return nil, err
		}
		f.ID, f.ParentID = strconv.FormatInt(id, 10), strID(parent)
		out = append(out, f)
	}
	bookmarks.SortFolders(out)
	return out, rows.Err()
}

const bookmarkColumns = "id, folder_id, title, url, description, keywords, created, updated"

type scanner interface{ Scan(...any) error }

func scanBookmark(r scanner) (bookmarks.Bookmark, error) {
	var (
		b                bookmarks.Bookmark
		id               int64
		folder           sql.NullInt64
		keywords         string
		created, updated string
	)
	if err := r.Scan(&id, &folder, &b.Title, &b.URL, &b.Description, &keywords, &created, &updated); err != nil {
		return b, err
	}
	b.ID, b.FolderID = strconv.FormatInt(id, 10), strID(folder)
	b.Keywords = []string{}
	if keywords != "" {
		b.Keywords = strings.Split(keywords, "\n")
	}
	b.Created, _ = time.Parse(time.RFC3339Nano, created)
	b.Updated, _ = time.Parse(time.RFC3339Nano, updated)
	return b, nil
}

func (s *Store) query(where string, args ...any) ([]bookmarks.Bookmark, error) {
	rows, err := s.db.Query("SELECT "+bookmarkColumns+" FROM bookmarks "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []bookmarks.Bookmark{}
	for rows.Next() {
		b, err := scanBookmark(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	bookmarks.SortBookmarks(out)
	return out, rows.Err()
}

func (s *Store) Bookmarks(_ bookmarks.Context, folderID string) ([]bookmarks.Bookmark, error) {
	if ok, err := s.folderExists(s.db, folderID); err != nil {
		return nil, err
	} else if !ok {
		return nil, notFound("folder", folderID)
	}
	n, _ := dbID("folder", folderID)
	if !n.Valid {
		return s.query("WHERE folder_id IS NULL")
	}
	return s.query("WHERE folder_id = ?", n.Int64)
}

func (s *Store) Bookmark(_ bookmarks.Context, id string) (bookmarks.Bookmark, error) {
	n, err := dbID("bookmark", id)
	if err != nil || !n.Valid {
		return bookmarks.Bookmark{}, notFound("bookmark", id)
	}
	b, err := scanBookmark(s.db.QueryRow("SELECT "+bookmarkColumns+" FROM bookmarks WHERE id = ?", n.Int64))
	if errors.Is(err, sql.ErrNoRows) {
		return b, notFound("bookmark", id)
	}
	return b, err
}

func (s *Store) FindByURL(_ bookmarks.Context, u string) (bookmarks.Bookmark, error) {
	list, err := s.query("WHERE url_key = ?", bookmarks.Key(u))
	if err != nil {
		return bookmarks.Bookmark{}, err
	}
	if len(list) == 0 {
		return bookmarks.Bookmark{}, fmt.Errorf("%s: %w", u, bookmarks.ErrNotFound)
	}
	return list[0], nil
}

func (s *Store) SaveBookmark(ctx bookmarks.Context, b bookmarks.Bookmark) (bookmarks.Bookmark, error) {
	b, err := bookmarks.Clean(b)
	if err != nil {
		return b, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return b, err
	}
	defer tx.Rollback()

	if ok, err := s.folderExists(tx, b.FolderID); err != nil {
		return b, err
	} else if !ok {
		return b, notFound("folder", b.FolderID)
	}
	folder, _ := dbID("folder", b.FolderID)
	now := time.Now().UTC()
	keywords := strings.Join(b.Keywords, "\n")

	if b.ID == "" {
		b.Created = now
		res, err := tx.Exec(`INSERT INTO bookmarks (folder_id, title, url, url_key, description, keywords, created, updated)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			folder, b.Title, b.URL, bookmarks.Key(b.URL), b.Description, keywords,
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if err != nil {
			return b, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return b, err
		}
		b.ID = strconv.FormatInt(id, 10)
	} else {
		id, err := dbID("bookmark", b.ID)
		if err != nil {
			return b, err
		}
		var created string
		err = tx.QueryRow("SELECT created FROM bookmarks WHERE id = ?", id.Int64).Scan(&created)
		if errors.Is(err, sql.ErrNoRows) {
			return b, notFound("bookmark", b.ID)
		} else if err != nil {
			return b, err
		}
		b.Created, _ = time.Parse(time.RFC3339Nano, created)
		if _, err := tx.Exec(`UPDATE bookmarks SET folder_id = ?, title = ?, url = ?, url_key = ?,
			description = ?, keywords = ?, updated = ? WHERE id = ?`,
			folder, b.Title, b.URL, bookmarks.Key(b.URL), b.Description, keywords,
			now.Format(time.RFC3339Nano), id.Int64); err != nil {
			return b, err
		}
	}
	b.Updated = now
	return b, tx.Commit()
}

func (s *Store) DeleteBookmark(_ bookmarks.Context, id string) error {
	n, err := dbID("bookmark", id)
	if err != nil || !n.Valid {
		return notFound("bookmark", id)
	}
	res, err := s.db.Exec("DELETE FROM bookmarks WHERE id = ?", n.Int64)
	if err != nil {
		return err
	}
	if c, _ := res.RowsAffected(); c == 0 {
		return notFound("bookmark", id)
	}
	return nil
}

func (s *Store) SaveFolder(_ bookmarks.Context, f bookmarks.Folder) (bookmarks.Folder, error) {
	f, err := bookmarks.CleanFolder(f)
	if err != nil {
		return f, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return f, err
	}
	defer tx.Rollback()

	if ok, err := s.folderExists(tx, f.ParentID); err != nil {
		return f, err
	} else if !ok {
		return f, notFound("folder", f.ParentID)
	}
	parent, _ := dbID("folder", f.ParentID)

	if f.ID == "" {
		res, err := tx.Exec("INSERT INTO folders (parent_id, name) VALUES (?, ?)", parent, f.Name)
		if err != nil {
			return f, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return f, err
		}
		f.ID = strconv.FormatInt(id, 10)
		return f, tx.Commit()
	}

	id, err := dbID("folder", f.ID)
	if err != nil {
		return f, err
	}
	if ok, err := s.folderExists(tx, f.ID); err != nil {
		return f, err
	} else if !ok {
		return f, notFound("folder", f.ID)
	}
	// Walk up from the new parent: reaching f means a cycle.
	for p := parent; p.Valid; {
		if p.Int64 == id.Int64 {
			return f, fmt.Errorf("%w: a folder cannot move into itself", bookmarks.ErrInvalid)
		}
		var next sql.NullInt64
		if err := tx.QueryRow("SELECT parent_id FROM folders WHERE id = ?", p.Int64).Scan(&next); err != nil {
			return f, err
		}
		p = next
	}
	if _, err := tx.Exec("UPDATE folders SET parent_id = ?, name = ? WHERE id = ?", parent, f.Name, id.Int64); err != nil {
		return f, err
	}
	return f, tx.Commit()
}

// DeleteFolder relies on ON DELETE CASCADE for subfolders and bookmarks.
func (s *Store) DeleteFolder(_ bookmarks.Context, id string) error {
	n, err := dbID("folder", id)
	if err != nil || !n.Valid {
		return notFound("folder", id)
	}
	res, err := s.db.Exec("DELETE FROM folders WHERE id = ?", n.Int64)
	if err != nil {
		return err
	}
	if c, _ := res.RowsAffected(); c == 0 {
		return notFound("folder", id)
	}
	return nil
}

// Search scans every bookmark in Go so matching is exactly bookmarks.Matches
// (Unicode-aware, unlike SQLite's LIKE); personal collections are small.
func (s *Store) Search(_ bookmarks.Context, terms []string) ([]bookmarks.Bookmark, error) {
	all, err := s.query("")
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, b := range all {
		if bookmarks.Matches(b, terms) {
			out = append(out, b)
		}
	}
	return out, nil
}
