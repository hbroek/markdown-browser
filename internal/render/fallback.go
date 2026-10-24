package render

import (
	"bytes"
	"net/url"

	"markdown-browser/internal/page"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

// Fallback extracts the title and the links from a non-markdown document.
func Fallback(body []byte, contentType string, base *url.URL) (string, []page.Link) {
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return "", nil
	}
	doc, err := xhtml.Parse(r)
	if err != nil {
		return "", nil
	}
	var title string
	var links []page.Link
	seen := map[string]bool{}

	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch n.DataAtom {
			case atom.Title:
				if title == "" {
					title = collapse(text(n))
				}
			case atom.A:
				if l, ok := link(n, base); ok && !seen[l.Href] {
					seen[l.Href] = true
					links = append(links, l)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return title, links
}

func link(n *xhtml.Node, base *url.URL) (page.Link, bool) {
	for _, a := range n.Attr {
		if a.Key != "href" {
			continue
		}
		ref, err := url.Parse(a.Val)
		if err != nil || a.Val == "" || a.Val[0] == '#' {
			return page.Link{}, false
		}
		abs := base.ResolveReference(ref)
		switch abs.Scheme {
		case "http", "https", "file":
		default:
			return page.Link{}, false
		}
		abs.Fragment = ""
		l := page.Link{Href: abs.String(), Text: collapse(text(n))}
		if l.Text == "" {
			l.Text = l.Href
		}
		return l, true
	}
	return page.Link{}, false
}
