package bookmarks

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Markdown import and export. The exported file is plain markdown that also
// reads well in the browser: folders are bold list items, bookmarks are links
// with an optional description after a dash and a "Keywords:" line.
//
//	# Bookmarks
//
//	- **Dev**
//	  - **Web**
//	    - [Go](<https://go.dev/>) — The Go site
//	      Keywords: lang, go
//	- [Top level](<https://top.test/>)
//
// The importer is more forgiving, so hand-written lists work too: headings
// (## and deeper, or # when the file has several) are folders, and a list
// item with a link is a bookmark. Items without a link are folders only when
// they are bold or have nested items.

var (
	headingRE  = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)
	itemRE     = regexp.MustCompile(`^([ \t]*)(?:[-*+]|\d+[.)])\s+(.*)$`)
	linkRE     = regexp.MustCompile(`^\[((?:\\.|[^\]\\])*)\]\((?:<([^>]*)>|([^)\s]*))(?:\s+"[^"]*")?\)`)
	autolinkRE = regexp.MustCompile(`^(?:<(https?://[^>\s]+)>|(https?://\S+))`)
	keywordsRE = regexp.MustCompile(`(?i)^keywords?:\s*(.*)$`)
	escapeRE   = regexp.MustCompile("\\\\([!-/:-@\\[-`{-~])") // backslash before ASCII punctuation
)

func unescapeMarkdown(s string) string { return escapeRE.ReplaceAllString(s, "$1") }

type mdLine struct {
	heading int    // 1-6 for a heading, else 0
	item    bool   // a list item
	indent  int    // list item indent in columns
	text    string // heading text, item text, or a continuation line
}

// ParseMarkdown reads bookmarks from a markdown file (see the format above).
func ParseMarkdown(r io.Reader) (*ImportFolder, error) {
	var lines []mdLine
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	fenced := false
	for sc.Scan() {
		raw := strings.TrimRight(sc.Text(), " \r")
		if strings.HasPrefix(strings.TrimSpace(raw), "```") || strings.HasPrefix(strings.TrimSpace(raw), "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || strings.TrimSpace(raw) == "" {
			continue
		}
		if m := headingRE.FindStringSubmatch(raw); m != nil {
			lines = append(lines, mdLine{heading: len(m[1]), text: m[2]})
		} else if m := itemRE.FindStringSubmatch(raw); m != nil {
			lines = append(lines, mdLine{item: true, indent: len(strings.ReplaceAll(m[1], "\t", "    ")), text: strings.TrimSpace(m[2])})
		} else {
			lines = append(lines, mdLine{text: strings.TrimSpace(raw)})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	// One "# Title" at the top is the document title, not a folder.
	h1s, first := 0, 0
	for _, l := range lines {
		if l.heading == 0 {
			continue
		}
		if first == 0 {
			first = l.heading
		}
		if l.heading == 1 {
			h1s++
		}
	}
	titleMode := h1s == 1 && first == 1

	root := &ImportFolder{}
	headings := make([]*ImportFolder, 8) // headings[depth]; depth 0 is the root
	headings[0] = root
	cur := root
	type listEntry struct {
		indent int
		f      *ImportFolder
	}
	var stack []listEntry
	var last *ImportFolder // folder holding the bookmark that continuation lines refer to
	lastIdx := -1

	for i, l := range lines {
		switch {
		case l.heading > 0:
			last = nil
			stack = nil
			depth := l.heading
			if titleMode {
				depth--
			}
			if depth == 0 {
				cur = root
				continue
			}
			parent := root
			for d := depth - 1; d >= 0; d-- {
				if headings[d] != nil {
					parent = headings[d]
					break
				}
			}
			f := &ImportFolder{Name: strings.TrimSpace(unescapeMarkdown(l.text))}
			parent.Folders = append(parent.Folders, f)
			headings[depth] = f
			for d := depth + 1; d < len(headings); d++ {
				headings[d] = nil
			}
			cur = f

		case l.item:
			last = nil
			for len(stack) > 0 && stack[len(stack)-1].indent >= l.indent {
				stack = stack[:len(stack)-1]
			}
			parent := cur
			if len(stack) > 0 {
				parent = stack[len(stack)-1].f
			}
			if b, ok := markdownBookmark(l.text); ok {
				parent.Bookmarks = append(parent.Bookmarks, b)
				last, lastIdx = parent, len(parent.Bookmarks)-1
				continue
			}
			name, bold := strings.TrimSpace(l.text), false
			for _, w := range []string{"**", "__"} {
				if len(name) > 2*len(w) && strings.HasPrefix(name, w) && strings.HasSuffix(name, w) {
					name, bold = strings.TrimSpace(name[len(w):len(name)-len(w)]), true
				}
			}
			nested := i+1 < len(lines) && lines[i+1].item && lines[i+1].indent > l.indent
			if name == "" || !(bold || nested) {
				continue // plain text, not a folder
			}
			f := &ImportFolder{Name: unescapeMarkdown(name)}
			parent.Folders = append(parent.Folders, f)
			stack = append(stack, listEntry{l.indent, f})

		default:
			if m := keywordsRE.FindStringSubmatch(l.text); m != nil && last != nil {
				var keywords []string
				for _, k := range strings.Split(m[1], ",") {
					keywords = append(keywords, unescapeMarkdown(strings.TrimSpace(k)))
				}
				last.Bookmarks[lastIdx].Keywords = keywords
			}
		}
	}
	return root, nil
}

// markdownBookmark reads "[title](url) - description" or a bare address.
func markdownBookmark(text string) (Bookmark, bool) {
	var b Bookmark
	var rest string
	if m := linkRE.FindStringSubmatch(text); m != nil {
		b.Title = strings.TrimSpace(unescapeMarkdown(m[1]))
		b.URL = m[2]
		if b.URL == "" {
			b.URL = m[3]
		}
		rest = text[len(m[0]):]
	} else if m := autolinkRE.FindStringSubmatch(text); m != nil {
		b.URL = m[1] + m[2]
		rest = text[len(m[0]):]
	} else {
		return b, false
	}
	rest = strings.TrimSpace(rest)
	for _, sep := range []string{"—", "–", "-", ":"} {
		if strings.HasPrefix(rest, sep) {
			rest = strings.TrimSpace(rest[len(sep):])
			break
		}
	}
	b.Description = unescapeMarkdown(rest)
	return b, true
}

// ExportMarkdown writes every folder and bookmark as a markdown file.
func ExportMarkdown(ctx Context, s Store) (string, error) {
	folders, err := s.Folders(ctx)
	if err != nil {
		return "", err
	}
	children := map[string][]Folder{}
	for _, f := range folders {
		children[f.ParentID] = append(children[f.ParentID], f)
	}

	var b strings.Builder
	b.WriteString("# Bookmarks\n\n")
	var write func(parent string, depth int) error
	write = func(parent string, depth int) error {
		pad := strings.Repeat("  ", depth)
		for _, f := range children[parent] {
			fmt.Fprintf(&b, "%s- **%s**\n", pad, mdSpecial.Replace(strings.Join(strings.Fields(f.Name), " ")))
			if err := write(f.ID, depth+1); err != nil {
				return err
			}
		}
		list, err := s.Bookmarks(ctx, parent)
		if err != nil {
			return err
		}
		for _, m := range list {
			title := strings.Join(strings.Fields(m.Title), " ")
			fmt.Fprintf(&b, "%s- [%s](<%s>)", pad, mdSpecial.Replace(title), linkSafe.Replace(m.URL))
			if d := strings.Join(strings.Fields(m.Description), " "); d != "" {
				fmt.Fprintf(&b, " — %s", mdSpecial.Replace(d))
			}
			b.WriteString("\n")
			if len(m.Keywords) > 0 {
				escaped := make([]string, len(m.Keywords))
				for i, k := range m.Keywords {
					escaped[i] = mdSpecial.Replace(strings.ReplaceAll(k, ",", " ")) // commas separate keywords
				}
				fmt.Fprintf(&b, "%s  Keywords: %s\n", pad, strings.Join(escaped, ", "))
			}
		}
		return nil
	}
	if err := write("", 0); err != nil {
		return "", err
	}
	return b.String(), nil
}
