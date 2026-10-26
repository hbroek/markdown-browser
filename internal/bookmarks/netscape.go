package bookmarks

import (
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	xhtml "golang.org/x/net/html"
)

// The Netscape bookmark file is the HTML format every browser imports and
// exports: nested <DL> lists, <DT><H3> folders and <DT><A HREF> bookmarks,
// with an optional <DD> description and a TAGS attribute for keywords.

// ImportFolder is a folder read from a bookmark file; the unnamed root holds
// the top level.
type ImportFolder struct {
	Name      string
	Folders   []*ImportFolder
	Bookmarks []Bookmark
}

// ImportResult counts what an import did.
type ImportResult struct {
	Folders    int  `json:"folders"`    // folders created
	Bookmarks  int  `json:"bookmarks"`  // bookmarks added
	Duplicates int  `json:"duplicates"` // already saved, left unchanged
	Skipped    int  `json:"skipped"`    // not an http, https or file address
	Canceled   bool `json:"canceled"`   // the user closed the file dialog
}

// ParseNetscape reads a bookmark file. It is forgiving about the format's
// unclosed tags, which is why it walks tokens instead of building a tree.
func ParseNetscape(r io.Reader) (*ImportFolder, error) {
	z := xhtml.NewTokenizer(r)
	root := &ImportFolder{}
	stack := []*ImportFolder{root}
	top := func() *ImportFolder { return stack[len(stack)-1] }

	var pending *ImportFolder // folder whose <DL> comes next
	var last *ImportFolder    // where the latest bookmark went, for its <DD>
	lastIdx := -1
	inDD := false

	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			if err := z.Err(); !errors.Is(err, io.EOF) {
				return nil, err
			}
			return root, nil

		case xhtml.TextToken:
			if inDD && last != nil {
				if d := strings.TrimSpace(string(z.Text())); d != "" {
					last.Bookmarks[lastIdx].Description = d
				}
			}
			inDD = false

		case xhtml.EndTagToken:
			inDD = false
			if name, _ := z.TagName(); string(name) == "dl" && len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}

		case xhtml.StartTagToken:
			inDD = false
			tok := z.Token()
			switch tok.Data {
			case "dl":
				if pending != nil {
					stack = append(stack, pending)
					pending = nil
				} else {
					stack = append(stack, top()) // the file's outermost list
				}
			case "h3":
				name := strings.TrimSpace(textUntil(z, "h3"))
				if name == "" {
					name = "Untitled folder"
				}
				pending = &ImportFolder{Name: name}
				top().Folders = append(top().Folders, pending)
			case "a":
				b := Bookmark{}
				for _, a := range tok.Attr {
					switch a.Key {
					case "href":
						b.URL = a.Val
					case "tags":
						b.Keywords = strings.Split(a.Val, ",")
					}
				}
				b.Title = strings.TrimSpace(textUntil(z, "a"))
				top().Bookmarks = append(top().Bookmarks, b)
				last, lastIdx = top(), len(top().Bookmarks)-1
			case "dd":
				inDD = true
			}
		}
	}
}

// textUntil collects the text up to the closing tag.
func textUntil(z *xhtml.Tokenizer, tag string) string {
	var b strings.Builder
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return b.String()
		case xhtml.TextToken:
			b.Write(z.Text())
		case xhtml.EndTagToken:
			if name, _ := z.TagName(); string(name) == tag {
				return b.String()
			}
		}
	}
}

// Import adds the file's folders and bookmarks to the store. Folders merge
// with existing ones of the same name; a page that is already bookmarked is
// left alone rather than overwritten.
func Import(ctx Context, s Store, root *ImportFolder) (ImportResult, error) {
	var res ImportResult
	existing, err := s.Folders(ctx)
	if err != nil {
		return res, err
	}
	type key struct{ parent, name string }
	ids := make(map[key]string, len(existing))
	for _, f := range existing {
		ids[key{f.ParentID, strings.ToLower(f.Name)}] = f.ID
	}

	var walk func(f *ImportFolder, id string) error
	walk = func(f *ImportFolder, id string) error {
		for _, b := range f.Bookmarks {
			if clean, err := Clean(b); err != nil {
				if errors.Is(err, ErrInvalid) {
					res.Skipped++
					continue
				}
				return err
			} else if _, err := s.FindByURL(ctx, clean.URL); err == nil {
				res.Duplicates++
				continue
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
			b.FolderID = id
			if _, err := s.SaveBookmark(ctx, b); err != nil {
				return err
			}
			res.Bookmarks++
		}
		for _, sub := range f.Folders {
			k := key{id, strings.ToLower(sub.Name)}
			subID, ok := ids[k]
			if !ok {
				created, err := s.SaveFolder(ctx, Folder{ParentID: id, Name: sub.Name})
				if err != nil {
					return err
				}
				subID = created.ID
				ids[k] = subID
				res.Folders++
			}
			if err := walk(sub, subID); err != nil {
				return err
			}
		}
		return nil
	}
	return res, walk(root, "")
}

// ExportNetscape writes every folder and bookmark as a bookmark file.
func ExportNetscape(ctx Context, s Store) (string, error) {
	folders, err := s.Folders(ctx)
	if err != nil {
		return "", err
	}
	children := map[string][]Folder{}
	for _, f := range folders {
		children[f.ParentID] = append(children[f.ParentID], f)
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n")
	b.WriteString("<!-- This is an automatically generated file. -->\n")
	b.WriteString("<META HTTP-EQUIV=\"Content-Type\" CONTENT=\"text/html; charset=UTF-8\">\n")
	b.WriteString("<TITLE>Bookmarks</TITLE>\n<H1>Bookmarks</H1>\n<DL><p>\n")

	var write func(parent string, depth int) error
	write = func(parent string, depth int) error {
		pad := strings.Repeat("    ", depth)
		for _, f := range children[parent] {
			fmt.Fprintf(&b, "%s<DT><H3>%s</H3>\n%s<DL><p>\n", pad, html.EscapeString(f.Name), pad)
			if err := write(f.ID, depth+1); err != nil {
				return err
			}
			fmt.Fprintf(&b, "%s</DL><p>\n", pad)
		}
		list, err := s.Bookmarks(ctx, parent)
		if err != nil {
			return err
		}
		for _, m := range list {
			tags := make([]string, len(m.Keywords))
			for i, k := range m.Keywords {
				tags[i] = strings.ReplaceAll(k, ",", " ") // commas separate TAGS
			}
			fmt.Fprintf(&b, "%s<DT><A HREF=\"%s\" ADD_DATE=\"%d\"", pad, html.EscapeString(m.URL), m.Created.Unix())
			if len(tags) > 0 {
				fmt.Fprintf(&b, " TAGS=\"%s\"", html.EscapeString(strings.Join(tags, ",")))
			}
			fmt.Fprintf(&b, ">%s</A>\n", html.EscapeString(m.Title))
			if m.Description != "" {
				fmt.Fprintf(&b, "%s<DD>%s\n", pad, html.EscapeString(m.Description))
			}
		}
		return nil
	}
	if err := write("", 1); err != nil {
		return "", err
	}
	b.WriteString("</DL><p>\n")
	return b.String(), nil
}
