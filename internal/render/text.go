package render

import (
	"bytes"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

// Text returns the readable text of an HTML document and its title: no
// scripts, styles or hidden parts, with a line break after each block.
func Text(body []byte, contentType string) (title, text string) {
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return "", ""
	}
	doc, err := xhtml.Parse(r)
	if err != nil {
		return "", ""
	}
	var sb strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			if f := strings.Join(strings.Fields(n.Data), " "); f != "" {
				if s := sb.String(); s != "" && !strings.HasSuffix(s, "\n") && !strings.HasSuffix(s, " ") {
					sb.WriteByte(' ')
				}
				sb.WriteString(f)
			}
			return
		}
		if n.Type == xhtml.ElementNode {
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Noscript, atom.Svg, atom.Template, atom.Iframe:
				return
			case atom.Title:
				if title == "" {
					title = collapse(nodeText(n))
				}
				return
			}
			if hidden(n) {
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == xhtml.ElementNode && isBlock(n.DataAtom) && !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteByte('\n')
		}
	}
	walk(doc)
	return title, strings.TrimSpace(sb.String())
}

func nodeText(n *xhtml.Node) string { return text(n) } // text is shadowed inside Text

func hidden(n *xhtml.Node) bool {
	for _, a := range n.Attr {
		if a.Key == "hidden" || (a.Key == "aria-hidden" && a.Val == "true") {
			return true
		}
	}
	return false
}

func isBlock(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Br, atom.Li, atom.Ul, atom.Ol, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6,
		atom.Tr, atom.Table, atom.Section, atom.Article, atom.Header, atom.Footer, atom.Blockquote, atom.Pre, atom.Hr, atom.Main:
		return true
	}
	return false
}
