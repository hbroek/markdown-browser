// Command lggcheck probes the LinkaGoGo API with your key, read-only, and
// prints the SHAPE of what comes back (field names and types, never values),
// so the backend's assumptions can be checked against the real service.
//
//	LGG_API_KEY=lgg_... go run ./tools/lggcheck
//
// Set LGG_URL to test another address. The key is read from the environment
// only and is sent only to that address.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"markdown-browser/internal/bookmarks"
	"markdown-browser/internal/bookmarks/linkagogo"
)

func main() {
	key := strings.TrimSpace(os.Getenv("LGG_API_KEY"))
	if key == "" {
		fmt.Fprintln(os.Stderr, "set LGG_API_KEY to your LinkaGoGo API key")
		os.Exit(2)
	}
	base := strings.TrimRight(os.Getenv("LGG_URL"), "/")
	if base == "" {
		base = "https://www.linkagogo.com/api/v1"
	}

	fmt.Println("== raw shapes ==")
	for _, path := range []string{"/folders", "/urls?page=1&page_size=3"} {
		status, body, err := get(base+path, key)
		fmt.Printf("GET %s -> %d %v\n", path, status, errText(err))
		if err != nil {
			continue
		}
		var v any
		if json.Unmarshal(body, &v) != nil {
			fmt.Printf("  (not JSON, %d bytes)\n", len(body))
			continue
		}
		describe("  ", v, 0)
	}

	fmt.Println("\n== through the backend ==")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cfg := bookmarks.Config{"url": base}
	if r := os.Getenv("LGG_ROOT"); r != "" {
		cfg["root"] = r
		fmt.Println("root folder:", r)
	}
	s, err := linkagogo.New(cfg, bookmarks.Deps{Authorize: func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+key) }})
	if err != nil {
		fmt.Println("setup:", err)
		os.Exit(1)
	}
	folders, err := s.Folders(ctx)
	fmt.Printf("Folders: %d %v\n", len(folders), errText(err))
	top, err := s.Bookmarks(ctx, "")
	fmt.Printf("Bookmarks at the top level: %d %v\n", len(top), errText(err))
	if len(top) > 0 {
		b := top[0]
		fmt.Printf("  first: title set=%v url set=%v keywords=%d description set=%v created set=%v updated set=%v\n",
			b.Title != "", b.URL != "", len(b.Keywords), b.Description != "", !b.Created.IsZero(), !b.Updated.IsZero())
		found, err := s.FindByURL(ctx, b.URL)
		fmt.Printf("  FindByURL(first) found=%v %v\n", found.ID == b.ID, errText(err))
	}
	hits, err := s.Search(ctx, []string{"the"})
	fmt.Printf("Search \"the\": %d results %v\n", len(hits), errText(err))
	fmt.Println("\nNothing was changed in your account.")
}

func get(url, key string) (int, []byte, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp.StatusCode, body, err
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return "ERROR: " + err.Error()
}

// describe prints names and types. Strings are described by traits that help
// with format questions (spaces, commas, quotes, length), not by content.
func describe(pad string, v any, depth int) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch c := t[k].(type) {
			case map[string]any, []any:
				fmt.Printf("%s%s: %s\n", pad, k, kind(c))
				if depth < 2 {
					describe(pad+"    ", c, depth+1)
				}
			default:
				fmt.Printf("%s%s: %s\n", pad, k, scalar(c))
			}
		}
	case []any:
		fmt.Printf("%s[%d items]; first item:\n", pad, len(t))
		if len(t) > 0 && depth < 3 {
			describe(pad+"    ", t[0], depth+1)
		}
	}
}

func kind(v any) string {
	switch t := v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return fmt.Sprintf("array(%d)", len(t))
	}
	return "?"
}

func scalar(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return fmt.Sprintf("bool (%v)", t)
	case float64:
		if t == float64(int64(t)) && t < 1e6 {
			return fmt.Sprintf("number (small integer %d)", int64(t)) // ids and counts; not private
		}
		return "number (large: id or timestamp)"
	case string:
		var traits []string
		if t == "" {
			traits = append(traits, "empty")
		}
		if strings.Contains(t, " ") {
			traits = append(traits, "has spaces")
		}
		if strings.Contains(t, ",") {
			traits = append(traits, "has commas")
		}
		if strings.Contains(t, `"`) {
			traits = append(traits, "has quotes")
		}
		if len(t) > 8 && len(t) < 40 && (strings.Contains(t, "T") || strings.Contains(t, "-")) && strings.Contains(t, ":") {
			traits = append(traits, "looks like a timestamp: "+t[:4]+"-..")
		}
		return fmt.Sprintf("string (%d chars%s)", len(t), prefix(traits))
	}
	return fmt.Sprintf("%T", v)
}

func prefix(t []string) string {
	if len(t) == 0 {
		return ""
	}
	return ", " + strings.Join(t, ", ")
}
