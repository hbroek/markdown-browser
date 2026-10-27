// Package linkagogo is a bookmarks.Store backed by the LinkaGoGo REST API
// (https://www.linkagogo.com/api-docs).
//
// It signs in with the Bearer token saved for the API's origin (the key
// button: type "Bearer token", site https://www.linkagogo.com). Settings:
//
//	{"bookmarks": {"backend": "linkagogo", "config": {
//	    "root": "12345",                              // folder ID to keep bookmarks in
//	    "url": "https://www.linkagogo.com/api/v1",   // optional
//	    "keywords": "space"                           // or "comma"; optional
//	}}}
//
// root is the LinkaGoGo folder that is this app's top level: only it and what
// is inside it are ever listed, searched or changed. Without root the whole
// account is used.
//
// The API allows a few thousand requests a day per key, so reads are cached
// for a few seconds and any write clears the cache.
package linkagogo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"markdown-browser/internal/bookmarks"
)

const (
	defaultURL = "https://www.linkagogo.com/api/v1"
	pageSize   = 200
	maxPages   = 100
	cacheTTL   = 15 * time.Second
	maxBody    = 16 << 20
)

func init() {
	bookmarks.Register("linkagogo", func(cfg bookmarks.Config, deps bookmarks.Deps) (bookmarks.Store, error) {
		return New(cfg, deps)
	})
}

type Store struct {
	base      string
	origin    string // for the "save a token" hint
	client    *http.Client
	authorize func(*http.Request)
	comma     bool   // keywords are comma separated, not space separated
	root      string // LinkaGoGo folder ID that is the app's top level; "" is the whole account

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	body    []byte
	expires time.Time
}

// New builds a store from the backend settings.
func New(cfg bookmarks.Config, deps bookmarks.Deps) (*Store, error) {
	base := strings.TrimRight(cfg["url"], "/")
	if base == "" {
		base = defaultURL
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("linkagogo: bad url %q", cfg["url"])
	}
	s := &Store{
		base:      base,
		origin:    u.Scheme + "://" + u.Host,
		client:    &http.Client{Timeout: 20 * time.Second},
		authorize: deps.Authorize,
		root:      strings.TrimSpace(cfg["root"]),
		cache:     map[string]cached{},
	}
	switch cfg["keywords"] {
	case "", "space":
	case "comma":
		s.comma = true
	default:
		return nil, fmt.Errorf("linkagogo: keywords must be \"space\" or \"comma\", not %q", cfg["keywords"])
	}
	return s, nil
}

func (s *Store) Close() error { return nil }

// Origin is the site whose saved Bearer token signs requests, e.g. "https://www.linkagogo.com".
func (s *Store) Origin() string { return s.origin }

// apiError is a non-2xx answer.
type apiError struct {
	status int
	detail string
	origin string
}

func (e *apiError) Error() string {
	switch e.status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Sprintf("LinkaGoGo rejected the API key (%d). Save a Bearer token for %s with the key button; API keys need a Plus or Premium plan", e.status, e.origin)
	case http.StatusTooManyRequests:
		return "LinkaGoGo's daily request limit for this key is used up; try again tomorrow"
	}
	if e.detail != "" {
		return fmt.Sprintf("LinkaGoGo: %s (%d)", e.detail, e.status)
	}
	return fmt.Sprintf("LinkaGoGo: %d %s", e.status, http.StatusText(e.status))
}

func hasStatus(err error, status int) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.status == status
}

// call sends one request and returns the response body.
func (s *Store) call(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	u := s.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.authorize != nil {
		s.authorize(req)
	}
	if req.Header.Get("Authorization") == "" {
		return nil, fmt.Errorf("no LinkaGoGo API key could be read for %s: open Bookmark storage (the gear in the bookmarks panel) and enter the key again", s.origin)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LinkaGoGo: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("LinkaGoGo: %w", err)
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Detail any `json:"detail"`
		}
		_ = json.Unmarshal(data, &e)
		detail, _ := e.Detail.(string)
		return nil, &apiError{status: resp.StatusCode, detail: detail, origin: s.origin}
	}
	return data, nil
}

// get is call for reads, answered from the cache when it is fresh.
func (s *Store) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	key := path + "?" + query.Encode()
	s.mu.Lock()
	c, ok := s.cache[key]
	s.mu.Unlock()
	if ok && time.Now().Before(c.expires) {
		return c.body, nil
	}
	data, err := s.call(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[key] = cached{data, time.Now().Add(cacheTTL)}
	s.mu.Unlock()
	return data, nil
}

// write is call for changes; it forgets cached reads, whatever the outcome.
func (s *Store) write(ctx context.Context, method, path string, body any) ([]byte, error) {
	defer func() {
		s.mu.Lock()
		s.cache = map[string]cached{}
		s.mu.Unlock()
	}()
	return s.call(ctx, method, path, nil, body)
}

func notFound(kind, id string) error { return fmt.Errorf("%s %s: %w", kind, id, bookmarks.ErrNotFound) }

// --- scope ---
//
// With a root folder, IDs inside the app are relative to it: the root is "" (the
// top level), and anything outside the root's subtree does not exist. API IDs
// are what LinkaGoGo uses; app IDs are what the rest of the program sees.

// scope is the set of folders (API IDs) the app may touch.
type scope struct {
	idx map[string]obj
	in  map[string]bool
}

// has reports whether an API folder ID is in scope; "" is LinkaGoGo's top level.
func (sc *scope) has(s *Store, id string) bool {
	if id == "" {
		return s.root == ""
	}
	return sc.in[id]
}

func (s *Store) scope(ctx context.Context) (*scope, error) {
	idx, err := s.folderIndex(ctx)
	if err != nil {
		return nil, err
	}
	sc := &scope{idx: idx, in: map[string]bool{}}
	if s.root == "" {
		for id := range idx {
			sc.in[id] = true
		}
		return sc, nil
	}
	if _, ok := idx[s.root]; !ok {
		return nil, fmt.Errorf("the LinkaGoGo folder chosen for bookmarks (%s) no longer exists; choose another in Bookmark storage", s.root)
	}
	children := map[string][]string{}
	for id, o := range idx {
		children[toFolder(o).ParentID] = append(children[toFolder(o).ParentID], id)
	}
	var walk func(id string)
	walk = func(id string) {
		sc.in[id] = true
		for _, c := range children[id] {
			if !sc.in[c] {
				walk(c)
			}
		}
	}
	walk(s.root)
	return sc, nil
}

// toAPI turns an app folder ID into LinkaGoGo's.
func (s *Store) toAPI(id string) string {
	if id == "" {
		return s.root
	}
	return id
}

// toApp turns a LinkaGoGo folder ID into the app's.
func (s *Store) toApp(id string) string {
	if s.root != "" && id == s.root {
		return ""
	}
	return id
}

func (s *Store) appBookmark(b bookmarks.Bookmark) bookmarks.Bookmark {
	b.FolderID = s.toApp(b.FolderID)
	return b
}

// --- folders ---

func (s *Store) folderObjs(ctx context.Context) ([]obj, error) {
	data, err := s.get(ctx, "/folders", nil)
	if err != nil {
		return nil, err
	}
	return decodeList(data, "folders", "items", "results", "data")
}

func toFolder(o obj) bookmarks.Folder {
	return bookmarks.Folder{ID: o.id("id"), ParentID: o.id("folder_id", "parent_id", "parent"), Name: o.str("name", "title")}
}

func (s *Store) Folders(ctx bookmarks.Context) ([]bookmarks.Folder, error) {
	sc, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]bookmarks.Folder, 0, len(sc.in))
	for id, o := range sc.idx {
		if !sc.in[id] || (s.root != "" && id == s.root) {
			continue
		}
		f := toFolder(o)
		f.ParentID = s.toApp(f.ParentID)
		out = append(out, f)
	}
	bookmarks.SortFolders(out)
	return out, nil
}

// folderIndex returns every folder in the account by ID, with its raw object.
func (s *Store) folderIndex(ctx context.Context) (map[string]obj, error) {
	objs, err := s.folderObjs(ctx)
	if err != nil {
		return nil, err
	}
	idx := make(map[string]obj, len(objs))
	for _, o := range objs {
		idx[toFolder(o).ID] = o
	}
	return idx, nil
}

// RootCandidates lists every folder in the account, for choosing the root.
// Unlike Folders it ignores the configured root.
func (s *Store) RootCandidates(ctx context.Context) ([]bookmarks.Folder, error) {
	idx, err := s.folderIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]bookmarks.Folder, 0, len(idx))
	for _, o := range idx {
		out = append(out, toFolder(o))
	}
	bookmarks.SortFolders(out)
	return out, nil
}

func (s *Store) SaveFolder(ctx bookmarks.Context, f bookmarks.Folder) (bookmarks.Folder, error) {
	f, err := bookmarks.CleanFolder(f)
	if err != nil {
		return f, err
	}
	sc, err := s.scope(ctx)
	if err != nil {
		return f, err
	}
	parent := s.toAPI(f.ParentID)
	if !sc.has(s, parent) {
		return f, notFound("folder", f.ParentID)
	}

	if f.ID == "" {
		data, err := s.write(ctx, http.MethodPost, "/folders", map[string]any{"name": f.Name, "folder_id": jsonID(parent)})
		if err != nil {
			return f, err
		}
		o, err := decodeObj(data)
		if err != nil {
			return f, err
		}
		if inner, ok := o["folder"]; ok { // some APIs wrap the result
			if o2, err := decodeObj(inner); err == nil {
				o = o2
			}
		}
		if id := o.id("id"); id != "" {
			f.ID = id
			return f, nil
		}
		// No ID in the answer: find what was just made.
		all, err := s.folderObjs(ctx)
		if err != nil {
			return f, err
		}
		for _, c := range all {
			if cf := toFolder(c); cf.Name == f.Name && cf.ParentID == parent {
				f.ID = cf.ID
			}
		}
		if f.ID == "" {
			return f, errors.New("LinkaGoGo created the folder but did not say which")
		}
		return f, nil
	}

	cur, ok := sc.idx[f.ID]
	if !ok || !sc.in[f.ID] || f.ID == s.root {
		return f, notFound("folder", f.ID)
	}
	for p := parent; p != ""; p = toFolder(sc.idx[p]).ParentID { // walk up: reaching f means a loop
		if p == f.ID {
			return f, fmt.Errorf("%w: a folder cannot move into itself", bookmarks.ErrInvalid)
		}
	}
	// PUT replaces the folder, so send back everything but what changed
	// (visibility and anything else the API keeps on a folder).
	body := map[string]any{}
	for k, v := range cur {
		if k != "id" {
			body[k] = v
		}
	}
	body["name"] = f.Name
	body["folder_id"] = jsonID(parent)
	if _, err := s.write(ctx, http.MethodPut, "/folders/"+url.PathEscape(f.ID), body); err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return f, notFound("folder", f.ID)
		}
		return f, err
	}
	return f, nil
}

func (s *Store) DeleteFolder(ctx bookmarks.Context, id string) error {
	sc, err := s.scope(ctx)
	if err != nil {
		return err
	}
	if id == "" || id == s.root || !sc.in[id] {
		return notFound("folder", id)
	}
	if _, err := s.write(ctx, http.MethodDelete, "/folders/"+url.PathEscape(id), nil); err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return notFound("folder", id)
		}
		return err
	}
	return nil
}

// --- bookmarks ---

func (s *Store) toBookmark(o obj) bookmarks.Bookmark {
	b := bookmarks.Bookmark{
		ID:          o.id("id"),
		FolderID:    o.id("folder_id"),
		Title:       o.str("title", "name"),
		URL:         o.str("url", "href"),
		Description: o.str("comments", "description", "notes"),
		Keywords:    parseKeywords(o.raw("keywords", "tags"), s.comma),
		Created:     o.time("created", "created_at", "date_added", "add_date"),
		Updated:     o.time("modified", "updated", "updated_at", "modified_at", "last_modified"),
	}
	if b.Updated.IsZero() { // not every answer says when it last changed
		b.Updated = b.Created
	}
	return b
}

// list reads every page of GET /urls for query.
func (s *Store) list(ctx context.Context, query url.Values) ([]bookmarks.Bookmark, error) {
	var out []bookmarks.Bookmark
	firstOfPrev := "\x00"
	for page := 1; page <= maxPages; page++ {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("page", fmt.Sprint(page))
		q.Set("page_size", fmt.Sprint(pageSize))
		data, err := s.get(ctx, "/urls", q)
		if err != nil {
			return nil, err
		}
		objs, err := decodeList(data, "bookmarks", "urls", "items", "results", "data")
		if err != nil {
			return nil, err
		}
		if len(objs) == 0 {
			break
		}
		first := s.toBookmark(objs[0]).ID
		if first == firstOfPrev {
			break // the server ignored the page number and repeated itself
		}
		firstOfPrev = first
		for _, o := range objs {
			out = append(out, s.toBookmark(o))
		}
		if len(objs) < pageSize {
			break
		}
	}
	return out, nil
}

func (s *Store) Bookmarks(ctx bookmarks.Context, folderID string) ([]bookmarks.Bookmark, error) {
	sc, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	api := s.toAPI(folderID)
	if !sc.has(s, api) {
		return nil, notFound("folder", folderID)
	}
	all, err := s.list(ctx, url.Values{"folder_id": {fmt.Sprint(jsonID(api))}})
	if hasStatus(err, http.StatusBadRequest) || hasStatus(err, http.StatusUnprocessableEntity) {
		all, err = s.list(ctx, nil) // the server does not take the root as a filter
	}
	if err != nil {
		return nil, err
	}
	// Filter again: a server that ignores the filter must not leak other folders in.
	out := []bookmarks.Bookmark{}
	for _, b := range all {
		if b.FolderID == api {
			out = append(out, s.appBookmark(b))
		}
	}
	bookmarks.SortBookmarks(out)
	return out, nil
}

// rawBookmark fetches a bookmark in API terms and proves it is in scope.
func (s *Store) rawBookmark(ctx context.Context, id string) (bookmarks.Bookmark, error) {
	if id == "" {
		return bookmarks.Bookmark{}, notFound("bookmark", id)
	}
	data, err := s.get(ctx, "/urls/"+url.PathEscape(id), nil)
	if err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return bookmarks.Bookmark{}, notFound("bookmark", id)
		}
		return bookmarks.Bookmark{}, err
	}
	o, err := decodeObj(data)
	if err != nil {
		return bookmarks.Bookmark{}, err
	}
	if inner, ok := o["bookmark"]; ok {
		if o2, err := decodeObj(inner); err == nil {
			o = o2
		}
	}
	b := s.toBookmark(o)
	sc, err := s.scope(ctx)
	if err != nil {
		return b, err
	}
	if !sc.has(s, b.FolderID) {
		return bookmarks.Bookmark{}, notFound("bookmark", id) // outside the app's folder
	}
	return b, nil
}

func (s *Store) Bookmark(ctx bookmarks.Context, id string) (bookmarks.Bookmark, error) {
	b, err := s.rawBookmark(ctx, id)
	return s.appBookmark(b), err
}

// inScope keeps the bookmarks that live in the app's folder tree.
func (s *Store) inScope(ctx context.Context, list []bookmarks.Bookmark) ([]bookmarks.Bookmark, error) {
	sc, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]bookmarks.Bookmark, 0, len(list))
	for _, b := range list {
		if sc.has(s, b.FolderID) {
			out = append(out, s.appBookmark(b))
		}
	}
	return out, nil
}

// FindByURL searches by host (the API's url: operator) and compares whole
// addresses here, so the match rule is the same as every other backend.
func (s *Store) FindByURL(ctx bookmarks.Context, raw string) (bookmarks.Bookmark, error) {
	needle := ""
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil {
		needle = u.Hostname()
		if needle == "" {
			needle = u.Path[strings.LastIndex(u.Path, "/")+1:] // file: addresses have no host
		}
	}
	if needle == "" {
		return bookmarks.Bookmark{}, fmt.Errorf("%s: %w", raw, bookmarks.ErrNotFound)
	}
	found, err := s.list(ctx, url.Values{"q": {"url:" + strings.ToLower(needle)}})
	if err != nil {
		return bookmarks.Bookmark{}, err
	}
	if found, err = s.inScope(ctx, found); err != nil {
		return bookmarks.Bookmark{}, err
	}
	key := bookmarks.Key(raw)
	var same []bookmarks.Bookmark
	for _, b := range found {
		if bookmarks.Key(b.URL) == key {
			same = append(same, b)
		}
	}
	if len(same) == 0 {
		return bookmarks.Bookmark{}, fmt.Errorf("%s: %w", raw, bookmarks.ErrNotFound)
	}
	bookmarks.SortBookmarks(same)
	return same[0], nil
}

// noReminder is the API's code for "never remind". Only new bookmarks get it;
// editing leaves whatever reminder the user set on linkagogo.com alone.
const noReminder = -9

func (s *Store) SaveBookmark(ctx bookmarks.Context, b bookmarks.Bookmark) (bookmarks.Bookmark, error) {
	b, err := bookmarks.Clean(b)
	if err != nil {
		return b, err
	}
	sc, err := s.scope(ctx)
	if err != nil {
		return b, err
	}
	api := s.toAPI(b.FolderID)
	if !sc.has(s, api) {
		return b, notFound("folder", b.FolderID)
	}
	fields := map[string]any{
		"url":      b.URL,
		"title":    b.Title,
		"keywords": formatKeywords(b.Keywords, s.comma),
		"comments": b.Description,
	}

	if b.ID == "" {
		fields["folder_id"] = jsonID(api)
		fields["reminder"] = noReminder // the API's default is a daily reminder
		data, err := s.write(ctx, http.MethodPost, "/urls", fields)
		if err != nil {
			return b, err
		}
		o, err := decodeObj(data)
		if err != nil {
			return b, err
		}
		if inner, ok := o["bookmark"]; ok {
			if o2, err := decodeObj(inner); err == nil {
				o = o2
			}
		}
		made := s.toBookmark(o)
		if made.ID == "" { // the answer did not carry the new bookmark
			if made, err = s.FindByURL(ctx, b.URL); err != nil {
				return b, errors.New("LinkaGoGo saved the bookmark but did not say which")
			}
		}
		b.ID = made.ID
		b.Created, b.Updated = made.Created, made.Updated
		if b.Created.IsZero() {
			b.Created = time.Now().UTC()
		}
		if b.Updated.IsZero() {
			b.Updated = b.Created
		}
		return b, nil
	}

	cur, err := s.rawBookmark(ctx, b.ID) // proves it exists and is ours, and keeps its creation time
	if err != nil {
		return b, err
	}
	if _, err := s.write(ctx, http.MethodPatch, "/urls/"+url.PathEscape(b.ID), fields); err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return b, notFound("bookmark", b.ID)
		}
		return b, err
	}
	if cur.FolderID != api { // moving has its own call
		if _, err := s.write(ctx, http.MethodPost, "/urls/move", map[string]any{"ids": []any{jsonID(b.ID)}, "folder_id": jsonID(api)}); err != nil {
			return b, err
		}
	}
	b.Created = cur.Created
	b.Updated = time.Now().UTC()
	return b, nil
}

func (s *Store) DeleteBookmark(ctx bookmarks.Context, id string) error {
	if _, err := s.rawBookmark(ctx, id); err != nil { // only ours may be deleted
		return err
	}
	if _, err := s.write(ctx, http.MethodDelete, "/urls/"+url.PathEscape(id), nil); err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return notFound("bookmark", id)
		}
		return err
	}
	return nil
}

// Search passes the terms to the API's own search, which also understands its
// operators (tag:, url:, fav:yes, ...), so nothing is filtered again here
// beyond keeping to the app's folder.
func (s *Store) Search(ctx bookmarks.Context, terms []string) ([]bookmarks.Bookmark, error) {
	q := url.Values{}
	if len(terms) > 0 {
		q.Set("q", strings.Join(terms, " "))
	}
	found, err := s.list(ctx, q)
	if err != nil {
		return nil, err
	}
	out, err := s.inScope(ctx, found)
	if err != nil {
		return nil, err
	}
	bookmarks.SortBookmarks(out)
	return out, nil
}
