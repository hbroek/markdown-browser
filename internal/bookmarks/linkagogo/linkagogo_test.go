package linkagogo

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"markdown-browser/internal/bookmarks"
	"markdown-browser/internal/bookmarks/storetest"
)

// fakeAPI is a stand-in for the LinkaGoGo REST API as documented. Where the
// docs are silent it picks the awkward answer, so the client cannot lean on it:
// folder_id=0 is ignored as a filter, PATCH ignores folder_id (moving takes
// POST /urls/move), and PUT replaces a folder (fields left out are lost).
type fakeAPI struct {
	mu            sync.Mutex
	next          int
	folders       map[int]map[string]any
	urls          map[int]map[string]any
	commaKeywords bool
	stringIDs     bool // IDs go out as JSON strings, not numbers
	requests      int
}

func newFake(comma, stringIDs bool) *fakeAPI {
	return &fakeAPI{folders: map[int]map[string]any{}, urls: map[int]map[string]any{}, commaKeywords: comma, stringIDs: stringIDs}
}

func (f *fakeAPI) id(n int) any {
	if f.stringIDs {
		return strconv.Itoa(n)
	}
	return n
}

func (f *fakeAPI) out(m map[string]any) map[string]any {
	c := map[string]any{}
	for k, v := range m {
		c[k] = v
	}
	c["id"] = f.id(m["id"].(int))
	return c
}

func num(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	case int:
		return n
	}
	return 0
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, detail string) {
	reply(w, status, map[string]string{"detail": detail})
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	body := func(r *http.Request) map[string]any {
		m := map[string]any{}
		json.NewDecoder(r.Body).Decode(&m)
		return m
	}
	// descendants returns id and every folder under it.
	var gone func(id int, set map[int]bool)
	gone = func(id int, set map[int]bool) {
		set[id] = true
		for cid, c := range f.folders {
			if num(c["folder_id"]) == id && !set[cid] {
				gone(cid, set)
			}
		}
	}

	mux.HandleFunc("GET /folders", func(w http.ResponseWriter, r *http.Request) {
		list := []any{}
		for _, m := range f.folders {
			list = append(list, f.out(m))
		}
		reply(w, 200, list)
	})
	mux.HandleFunc("POST /folders", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		if p := num(b["folder_id"]); p != 0 && f.folders[p] == nil {
			fail(w, 404, "Parent folder not found")
			return
		}
		f.next++
		f.folders[f.next] = map[string]any{"id": f.next, "name": b["name"], "folder_id": num(b["folder_id"]), "public": false}
		reply(w, 201, f.out(f.folders[f.next]))
	})
	mux.HandleFunc("PUT /folders/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		if f.folders[id] == nil {
			fail(w, 404, "Folder not found")
			return
		}
		b := body(r)
		pub, _ := b["public"].(bool) // not sent means false: the replace semantics
		f.folders[id] = map[string]any{"id": id, "name": b["name"], "folder_id": num(b["folder_id"]), "public": pub}
		reply(w, 200, f.out(f.folders[id]))
	})
	mux.HandleFunc("DELETE /folders/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		if f.folders[id] == nil {
			fail(w, 404, "Folder not found")
			return
		}
		set := map[int]bool{}
		gone(id, set)
		for fid := range set {
			delete(f.folders, fid)
		}
		for uid, u := range f.urls {
			if set[num(u["folder_id"])] {
				delete(f.urls, uid)
			}
		}
		reply(w, 200, map[string]bool{"ok": true})
	})

	mux.HandleFunc("GET /urls", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var terms, urlTerms []string
		for _, t := range strings.Fields(q.Get("q")) {
			if strings.HasPrefix(t, "url:") {
				urlTerms = append(urlTerms, strings.ToLower(t[4:]))
			} else {
				terms = append(terms, strings.ToLower(t))
			}
		}
		filter := -1
		if v := q.Get("folder_id"); v != "" && v != "0" { // 0 is "no filter" here
			filter = num(v)
		}
		var ids []int
		for id := range f.urls {
			ids = append(ids, id)
		}
		sortInts(ids)
		var rows []any
		for _, id := range ids {
			u := f.urls[id]
			if filter >= 0 && num(u["folder_id"]) != filter {
				continue
			}
			hay := strings.ToLower(fmt.Sprint(u["title"], "\n", u["url"], "\n", u["keywords"], "\n", u["comments"]))
			ok := true
			for _, t := range terms {
				ok = ok && strings.Contains(hay, t)
			}
			for _, t := range urlTerms {
				ok = ok && strings.Contains(strings.ToLower(fmt.Sprint(u["url"])), t)
			}
			if ok {
				rows = append(rows, f.out(u))
			}
		}
		page, _ := strconv.Atoi(q.Get("page"))
		size, _ := strconv.Atoi(q.Get("page_size"))
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 200 {
			size = 50
		}
		lo, hi := (page-1)*size, page*size
		if lo > len(rows) {
			lo = len(rows)
		}
		if hi > len(rows) {
			hi = len(rows)
		}
		reply(w, 200, map[string]any{"bookmarks": append([]any{}, rows[lo:hi]...), "total": len(rows), "page": page})
	})
	mux.HandleFunc("GET /urls/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		if f.urls[id] == nil {
			fail(w, 404, "Bookmark not found")
			return
		}
		reply(w, 200, f.out(f.urls[id]))
	})
	mux.HandleFunc("POST /urls", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		if p := num(b["folder_id"]); p != 0 && f.folders[p] == nil {
			fail(w, 404, "Folder not found")
			return
		}
		f.next++
		kw, _ := b["keywords"].(string)
		f.urls[f.next] = map[string]any{"id": f.next, "url": b["url"], "title": b["title"], "folder_id": num(b["folder_id"]),
			"keywords": kw, "comments": b["comments"], "created": time.Now().UTC().Format(time.RFC3339), "rating": 3, "reminder": b["reminder"]}
		reply(w, 201, f.out(f.urls[f.next]))
	})
	mux.HandleFunc("PATCH /urls/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		u := f.urls[id]
		if u == nil {
			fail(w, 404, "Bookmark not found")
			return
		}
		for k, v := range body(r) {
			if k != "folder_id" && k != "id" { // moving is not a PATCH here
				u[k] = v
			}
		}
		reply(w, 200, f.out(u))
	})
	mux.HandleFunc("POST /urls/move", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		for _, v := range b["ids"].([]any) {
			if u := f.urls[num(v)]; u != nil {
				u["folder_id"] = num(b["folder_id"])
			}
		}
		reply(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE /urls/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		if f.urls[id] == nil {
			fail(w, 404, "Bookmark not found")
			return
		}
		delete(f.urls, id)
		reply(w, 200, map[string]bool{"ok": true})
	})

	api := http.NewServeMux()
	api.Handle("/api/v1/", http.StripPrefix("/api/v1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		if r.Header.Get("Authorization") != "Bearer lgg_test" {
			fail(w, 401, "Invalid API key")
			return
		}
		mux.ServeHTTP(w, r)
	})))
	return api
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func bearer(r *http.Request) { r.Header.Set("Authorization", "Bearer lgg_test") }

func newStore(t *testing.T, f *fakeAPI) *Store {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	cfg := bookmarks.Config{"url": srv.URL + "/api/v1"}
	if f.commaKeywords {
		cfg["keywords"] = "comma"
	}
	s, err := New(cfg, bookmarks.Deps{Authorize: bearer})
	if err != nil {
		t.Fatal(err)
	}
	s.cache = map[string]cached{} // the suite writes then reads at once
	return s
}

func TestConformanceSpaceKeywordsNumericIDs(t *testing.T) {
	storetest.Run(t, func(t *testing.T) bookmarks.Store { return newStore(t, newFake(false, false)) })
}

func TestConformanceCommaKeywordsStringIDs(t *testing.T) {
	storetest.Run(t, func(t *testing.T) bookmarks.Store { return newStore(t, newFake(true, true)) })
}

func TestAuthErrors(t *testing.T) {
	f := newFake(false, false)
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	cfg := bookmarks.Config{"url": srv.URL + "/api/v1"}

	// No credential saved: fail before touching the network.
	s, _ := New(cfg, bookmarks.Deps{})
	if _, err := s.Folders(t.Context()); err == nil || !strings.Contains(err.Error(), "Bearer token") {
		t.Errorf("no key: %v", err)
	}
	if f.requests != 0 {
		t.Errorf("sent %d requests without a key", f.requests)
	}

	// A wrong key: the server's 401 becomes advice.
	s, _ = New(cfg, bookmarks.Deps{Authorize: func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }})
	if _, err := s.Folders(t.Context()); err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Errorf("bad key: %v", err)
	}
}

func TestRateLimitMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "86400")
		fail(w, 429, "rate limited")
	}))
	defer srv.Close()
	s, _ := New(bookmarks.Config{"url": srv.URL}, bookmarks.Deps{Authorize: bearer})
	if _, err := s.Folders(t.Context()); err == nil || !strings.Contains(err.Error(), "daily request limit") {
		t.Errorf("429: %v", err)
	}
}

func TestKeywordFormats(t *testing.T) {
	cases := []struct {
		comma bool
		in    []string
		wire  string
	}{
		{false, []string{"go", "machine learning", "x"}, `go "machine learning" x`},
		{true, []string{"go", "machine learning", "a,b"}, `go,machine learning,a b`},
	}
	for _, c := range cases {
		if got := formatKeywords(c.in, c.comma); got != c.wire {
			t.Errorf("format(%q, comma=%v) = %q, want %q", c.in, c.comma, got, c.wire)
		}
	}
	for _, in := range []string{`"go","machine learning"`, `["go","machine learning"]`} {
		_ = in
	}
	if got := parseKeywords(json.RawMessage(`"go \"machine learning\"  x"`), false); !reflect.DeepEqual(got, []string{"go", "machine learning", "x"}) {
		t.Errorf("space parse = %q", got)
	}
	if got := parseKeywords(json.RawMessage(`"go, machine learning ,x"`), true); !reflect.DeepEqual(got, []string{"go", "machine learning", "x"}) {
		t.Errorf("comma parse = %q", got)
	}
	if got := parseKeywords(json.RawMessage(`["go"," machine learning "]`), false); !reflect.DeepEqual(got, []string{"go", "machine learning"}) {
		t.Errorf("array parse = %q", got)
	}
	if got := parseKeywords(nil, false); got == nil || len(got) != 0 {
		t.Errorf("missing keywords = %#v", got)
	}
}

func TestMultiWordKeywordsRoundTrip(t *testing.T) {
	s := newStore(t, newFake(false, false))
	b, err := s.SaveBookmark(t.Context(), bookmarks.Bookmark{Title: "T", URL: "https://k.test/", Keywords: []string{"machine learning", "go"}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Bookmark(t.Context(), b.ID)
	if !reflect.DeepEqual(got.Keywords, []string{"machine learning", "go"}) {
		t.Errorf("keywords = %q", got.Keywords)
	}
}

func TestFolderRenameKeepsOtherFields(t *testing.T) {
	f := newFake(false, false)
	s := newStore(t, f)
	made, _ := s.SaveFolder(t.Context(), bookmarks.Folder{Name: "Shared"})
	f.folders[num(made.ID)]["public"] = true // set on the website, which this app does not know about

	made.Name = "Shared renamed"
	if _, err := s.SaveFolder(t.Context(), made); err != nil {
		t.Fatal(err)
	}
	if f.folders[num(made.ID)]["public"] != true {
		t.Errorf("renaming made the folder private: %+v", f.folders[num(made.ID)])
	}
}

func TestNewBookmarksNeverRemind(t *testing.T) {
	f := newFake(false, false)
	s := newStore(t, f)
	b, err := s.SaveBookmark(t.Context(), bookmarks.Bookmark{Title: "One", URL: "https://r.test/"})
	if err != nil {
		t.Fatal(err)
	}
	id := num(b.ID)
	if got := num(f.urls[id]["reminder"]); got != -9 {
		t.Errorf("reminder = %v, want -9 (never)", f.urls[id]["reminder"])
	}
	f.urls[id]["reminder"] = 7 // set on the website
	s.cache = map[string]cached{}
	b.Title = "Two"
	if _, err := s.SaveBookmark(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	if got := num(f.urls[id]["reminder"]); got != 7 {
		t.Errorf("edit changed the reminder to %v", got)
	}
}

func TestEditKeepsRatingAndCreated(t *testing.T) {
	f := newFake(false, false)
	s := newStore(t, f)
	b, _ := s.SaveBookmark(t.Context(), bookmarks.Bookmark{Title: "One", URL: "https://r.test/"})
	id := num(b.ID)
	f.urls[id]["created"] = "2020-05-01T10:00:00Z"
	f.urls[id]["rating"] = 5 // set on the website
	s.cache = map[string]cached{}

	b.Title = "Two"
	up, err := s.SaveBookmark(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	if f.urls[id]["rating"] != 5 || f.urls[id]["title"] != "Two" {
		t.Errorf("edit changed the wrong fields: %+v", f.urls[id])
	}
	if up.Created.Year() != 2020 {
		t.Errorf("created = %v, want 2020", up.Created)
	}
}

func TestPaginationAndFolderFilter(t *testing.T) {
	f := newFake(false, false)
	s := newStore(t, f)
	dev, _ := s.SaveFolder(t.Context(), bookmarks.Folder{Name: "Dev"})
	for i := 0; i < 450; i++ { // more than two pages
		f.next++
		folder := 0
		if i%3 == 0 {
			folder = num(dev.ID)
		}
		f.urls[f.next] = map[string]any{"id": f.next, "url": fmt.Sprintf("https://p.test/%d", i), "title": fmt.Sprintf("P%03d", i), "folder_id": folder, "keywords": "", "comments": ""}
	}
	root, err := s.Bookmarks(t.Context(), "")
	if err != nil || len(root) != 300 {
		t.Fatalf("root = %d, %v (the filter must not let other folders in)", len(root), err)
	}
	in, err := s.Bookmarks(t.Context(), dev.ID)
	if err != nil || len(in) != 150 {
		t.Fatalf("dev = %d, %v", len(in), err)
	}
	found, err := s.FindByURL(t.Context(), "https://P.test/449#x")
	if err != nil || found.Title != "P449" {
		t.Errorf("FindByURL = %+v, %v", found, err)
	}
	hits, err := s.Search(t.Context(), []string{"p44"})
	if err != nil || len(hits) != 10 {
		t.Errorf("search p44 = %d, %v", len(hits), err)
	}
}

func TestReadsAreCachedAndWritesClearTheCache(t *testing.T) {
	f := newFake(false, false)
	s := newStore(t, f)
	ctx := t.Context()
	s.Folders(ctx)
	before := f.requests
	s.Folders(ctx)
	if f.requests != before {
		t.Errorf("second read hit the network (%d -> %d requests)", before, f.requests)
	}
	s.SaveFolder(ctx, bookmarks.Folder{Name: "New"})
	got, _ := s.Folders(ctx)
	if len(got) != 1 {
		t.Errorf("a write must clear the cache: %+v", got)
	}
}

func TestBadConfig(t *testing.T) {
	for _, cfg := range []bookmarks.Config{{"url": "not a url"}, {"url": "ftp://x.test"}, {"keywords": "semicolon"}} {
		if _, err := New(cfg, bookmarks.Deps{}); err == nil {
			t.Errorf("New(%v) should fail", cfg)
		}
	}
	s, err := New(nil, bookmarks.Deps{})
	if err != nil || s.base != defaultURL {
		t.Errorf("defaults: %v, %v", s, err)
	}
	if !errors.Is(notFound("x", "1"), bookmarks.ErrNotFound) {
		t.Error("notFound must wrap ErrNotFound")
	}
}

// seedOutside puts things in the account that the app must never see or touch:
// a top-level bookmark, a top-level folder with a bookmark inside, and the
// folder chosen as the app's root (also holding an unrelated sibling).
func seedOutside(f *fakeAPI) (root int, outsideFolder, outsideBookmark int) {
	add := func(id int, kv map[string]any) { kv["id"] = id; f.folders[id] = kv }
	f.next = 100
	f.next++
	add(f.next, map[string]any{"name": "Other stuff", "folder_id": 0, "public": false})
	outsideFolder = f.next
	f.next++
	f.urls[f.next] = map[string]any{"id": f.next, "url": "https://outside.test/in-folder", "title": "Outside in folder", "folder_id": outsideFolder, "keywords": "", "comments": ""}
	outsideBookmark = f.next
	f.next++
	f.urls[f.next] = map[string]any{"id": f.next, "url": "https://outside.test/top", "title": "Outside top", "folder_id": 0, "keywords": "", "comments": ""}
	f.next++
	add(f.next, map[string]any{"name": "Markdown Browser", "folder_id": 0, "public": false})
	return f.next, outsideFolder, outsideBookmark
}

func newRootedStore(t *testing.T, f *fakeAPI, root int) *Store {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	cfg := bookmarks.Config{"url": srv.URL + "/api/v1", "root": strconv.Itoa(root)}
	if f.commaKeywords {
		cfg["keywords"] = "comma"
	}
	s, err := New(cfg, bookmarks.Deps{Authorize: bearer})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestConformanceInsideRootFolder(t *testing.T) {
	storetest.Run(t, func(t *testing.T) bookmarks.Store {
		f := newFake(false, false)
		root, _, _ := seedOutside(f)
		s := newRootedStore(t, f, root)
		s.cache = map[string]cached{}
		return s
	})
}

func TestRootFolderHidesAndProtectsTheRestOfTheAccount(t *testing.T) {
	f := newFake(true, true)
	root, outFolder, outBookmark := seedOutside(f)
	s := newRootedStore(t, f, root)
	ctx := t.Context()

	if got, _ := s.Folders(ctx); len(got) != 0 {
		t.Errorf("folders outside the root are visible: %+v", got)
	}
	if got, _ := s.Bookmarks(ctx, ""); len(got) != 0 {
		t.Errorf("bookmarks outside the root are visible: %+v", got)
	}
	if got, _ := s.Search(ctx, []string{"outside"}); len(got) != 0 {
		t.Errorf("search sees outside the root: %+v", got)
	}
	if _, err := s.FindByURL(ctx, "https://outside.test/top"); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("FindByURL outside: %v", err)
	}
	ob, od := strconv.Itoa(outBookmark), strconv.Itoa(outFolder)
	if _, err := s.Bookmark(ctx, ob); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("Bookmark outside: %v", err)
	}
	if err := s.DeleteBookmark(ctx, ob); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("DeleteBookmark outside: %v", err)
	}
	if err := s.DeleteFolder(ctx, od); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("DeleteFolder outside: %v", err)
	}
	if _, err := s.SaveFolder(ctx, bookmarks.Folder{ID: od, Name: "hijack"}); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("SaveFolder outside: %v", err)
	}
	if _, err := s.SaveBookmark(ctx, bookmarks.Bookmark{ID: ob, Title: "x", URL: "https://x.test/"}); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("SaveBookmark over an outside bookmark: %v", err)
	}
	if _, err := s.SaveBookmark(ctx, bookmarks.Bookmark{FolderID: od, Title: "x", URL: "https://x.test/"}); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("SaveBookmark into an outside folder: %v", err)
	}
	if err := s.DeleteFolder(ctx, strconv.Itoa(root)); !errors.Is(err, bookmarks.ErrNotFound) {
		t.Errorf("the root folder itself must not be deletable: %v", err)
	}

	// New things land inside the root, and the app sees them as top level.
	made, err := s.SaveBookmark(ctx, bookmarks.Bookmark{Title: "Mine", URL: "https://mine.test/"})
	if err != nil {
		t.Fatal(err)
	}
	if f.urls[num(made.ID)]["folder_id"] != root {
		t.Errorf("stored in folder %v, want the root %d", f.urls[num(made.ID)]["folder_id"], root)
	}
	if got, _ := s.Bookmarks(ctx, ""); len(got) != 1 || got[0].FolderID != "" {
		t.Errorf("top level = %+v", got)
	}
	sub, _ := s.SaveFolder(ctx, bookmarks.Folder{Name: "Sub"})
	if f.folders[num(sub.ID)]["folder_id"] != root {
		t.Errorf("folder stored under %v, want the root", f.folders[num(sub.ID)]["folder_id"])
	}

	if len(f.urls) != 3 || len(f.folders) != 3 { // 2 outside bookmarks + Mine; Other stuff, root, Sub
		t.Errorf("outside data changed: %d urls, %d folders", len(f.urls), len(f.folders))
	}
}

func TestMissingRootFolder(t *testing.T) {
	f := newFake(false, false)
	s := newRootedStore(t, f, 999)
	if _, err := s.Folders(t.Context()); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("err = %v", err)
	}
}

func TestRootCandidatesListEverything(t *testing.T) {
	f := newFake(false, false)
	root, _, _ := seedOutside(f)
	s := newRootedStore(t, f, root)
	all, err := s.RootCandidates(t.Context())
	if err != nil || len(all) != 2 {
		t.Errorf("candidates = %+v, %v", all, err)
	}
}
