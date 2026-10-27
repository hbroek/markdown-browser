package bookmarks

import (
	"fmt"
	"strings"
	"time"
)

// Visit is a page from the browsing history that matched a search.
type Visit struct {
	Title string
	URL   string
	Time  time.Time
}

var mdSpecial = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "&", `\&`, "~", `\~`, "|", `\|`,
)

var linkSafe = strings.NewReplacer("<", "%3C", ">", "%3E", " ", "%20", "\n", "%0A")

// SearchMarkdown formats search results as a markdown page: bookmarks first,
// then visited pages that are not bookmarked. An empty query lists all the
// bookmarks. folders is used only to show each bookmark's folder path.
func SearchMarkdown(query string, results []Bookmark, visits []Visit, folders []Folder) string {
	byID := make(map[string]Folder, len(folders))
	for _, f := range folders {
		byID[f.ID] = f
	}
	path := func(id string) string {
		parts := []string{}
		for seen := 0; id != "" && seen < len(folders)+1; seen++ {
			f, ok := byID[id]
			if !ok {
				break
			}
			parts = append([]string{f.Name}, parts...)
			id = f.ParentID
		}
		return strings.Join(append([]string{"Bookmarks"}, parts...), " / ")
	}

	saved := make(map[string]bool, len(results))
	for _, r := range results {
		saved[Key(r.URL)] = true
	}
	var history []Visit
	for _, v := range visits {
		if !saved[Key(v.URL)] {
			saved[Key(v.URL)] = true // one line per page, even if visited twice
			history = append(history, v)
		}
	}

	var b strings.Builder
	if strings.TrimSpace(query) == "" {
		b.WriteString("# All bookmarks\n\n")
	} else {
		fmt.Fprintf(&b, "# Results for \u201c%s\u201d\n\n", mdSpecial.Replace(query))
	}
	if len(results)+len(history) == 0 {
		b.WriteString("Nothing found in your bookmarks or history.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%s", count(len(results), "bookmark"))
	if len(history) > 0 {
		fmt.Fprintf(&b, ", %s from history", count(len(history), "page"))
	}
	b.WriteString(".\n\n")

	item := func(title, url, where string) {
		title = strings.Join(strings.Fields(title), " ")
		if title == "" {
			title = url
		}
		fmt.Fprintf(&b, "- [%s](<%s>)  \n  _%s_", mdSpecial.Replace(title), linkSafe.Replace(url), mdSpecial.Replace(where))
	}
	for _, r := range results {
		item(r.Title, r.URL, path(r.FolderID))
		if len(r.Keywords) > 0 {
			escaped := make([]string, len(r.Keywords))
			for i, k := range r.Keywords {
				escaped[i] = mdSpecial.Replace(k)
			}
			fmt.Fprintf(&b, " \u00B7 %s", strings.Join(escaped, ", "))
		}
		if d := strings.Join(strings.Fields(r.Description), " "); d != "" {
			fmt.Fprintf(&b, "  \n  %s", mdSpecial.Replace(d))
		}
		b.WriteString("\n")
	}
	for _, v := range history {
		item(v.Title, v.URL, "History \u00B7 "+v.Time.Local().Format("2 Jan 2006 15:04"))
		b.WriteString("\n")
	}
	return b.String()
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// Match is a page picked in a search by meaning.
type Match struct {
	Title    string
	URL      string
	Where    string // the folder path, or "History · date"
	Keywords []string
	Why      string // the model's reason
}

// MeaningMarkdown formats the pages a model picked for a description, best first.
// note says how the search was done (what was searched and by which model).
func MeaningMarkdown(query string, matches []Match, note string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Best matches for \u201c%s\u201d\n\n", mdSpecial.Replace(query))
	if len(matches) == 0 {
		b.WriteString("Nothing in your bookmarks or history seems to fit that description.\n")
	}
	for _, m := range matches {
		title := strings.Join(strings.Fields(m.Title), " ")
		if title == "" {
			title = m.URL
		}
		fmt.Fprintf(&b, "- [%s](<%s>)  \n  _%s_", mdSpecial.Replace(title), linkSafe.Replace(m.URL), mdSpecial.Replace(m.Where))
		if len(m.Keywords) > 0 {
			escaped := make([]string, len(m.Keywords))
			for i, k := range m.Keywords {
				escaped[i] = mdSpecial.Replace(k)
			}
			fmt.Fprintf(&b, " \u00B7 %s", strings.Join(escaped, ", "))
		}
		if w := strings.Join(strings.Fields(m.Why), " "); w != "" {
			fmt.Fprintf(&b, "  \n  %s", mdSpecial.Replace(w))
		}
		b.WriteString("\n")
	}
	if note != "" {
		fmt.Fprintf(&b, "\n---\n\n_%s_\n", mdSpecial.Replace(note))
	}
	return b.String()
}
