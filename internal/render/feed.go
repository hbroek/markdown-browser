package render

import (
	"bytes"
	"encoding/xml"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html/charset"
)

// feedItem is the common shape of an RSS item or an Atom entry.
type feedItem struct {
	Title, Link, Date string
	Markdown          string // <source:markdown>, preferred when present
	HTML              string // content:encoded, description, or Atom content
}

type feedDoc struct {
	Title, Link, Description string
	Items                    []feedItem
}

type rssDoc struct {
	Channel struct {
		Title       string    `xml:"title"`
		Links       []string  `xml:"link"` // atom:link also matches, with no text
		Description string    `xml:"description"`
		Items       []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title          string   `xml:"title"`
	Links          []string `xml:"link"`
	GUID           string   `xml:"guid"`
	PubDate        string   `xml:"pubDate"`
	Description    string   `xml:"description"`
	ContentEncoded string   `xml:"encoded"`
	Markdown       string   `xml:"markdown"` // any namespace prefix: source:markdown
}

type atomText struct {
	Type  string `xml:"type,attr"`
	Text  string `xml:",chardata"`
	Inner string `xml:",innerxml"`
}

func (t atomText) html() string {
	switch t.Type {
	case "html":
		return t.Text
	case "xhtml":
		return t.Inner
	}
	return html.EscapeString(t.Text)
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomDoc struct {
	Title    atomText   `xml:"title"`
	Subtitle atomText   `xml:"subtitle"`
	Links    []atomLink `xml:"link"`
	Entries  []struct {
		Title     atomText   `xml:"title"`
		Links     []atomLink `xml:"link"`
		Updated   string     `xml:"updated"`
		Published string     `xml:"published"`
		Summary   atomText   `xml:"summary"`
		Content   atomText   `xml:"content"`
		Markdown  string     `xml:"markdown"`
	} `xml:"entry"`
}

// IsFeed reports whether body looks like an RSS or Atom document.
func IsFeed(body []byte) bool {
	head := body
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(head, []byte("<rss")) || bytes.Contains(head, []byte("<feed"))
}

func parseFeed(body []byte) (*feedDoc, bool) {
	d := xml.NewDecoder(bytes.NewReader(body))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	d.CharsetReader = charset.NewReaderLabel
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, false
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "rss":
			var doc rssDoc
			if d.DecodeElement(&doc, &start) != nil {
				return nil, false
			}
			return fromRSS(doc), true
		case "feed":
			var doc atomDoc
			if d.DecodeElement(&doc, &start) != nil {
				return nil, false
			}
			return fromAtom(doc), true
		}
		return nil, false
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func fromRSS(doc rssDoc) *feedDoc {
	f := &feedDoc{
		Title:       strings.TrimSpace(doc.Channel.Title),
		Link:        firstNonEmpty(doc.Channel.Links...),
		Description: strings.TrimSpace(doc.Channel.Description),
	}
	for _, it := range doc.Channel.Items {
		link := firstNonEmpty(it.Links...)
		if link == "" && strings.HasPrefix(strings.TrimSpace(it.GUID), "http") {
			link = strings.TrimSpace(it.GUID)
		}
		f.Items = append(f.Items, feedItem{
			Title:    strings.TrimSpace(it.Title),
			Link:     link,
			Date:     strings.TrimSpace(it.PubDate),
			Markdown: it.Markdown,
			HTML:     firstNonEmpty(it.ContentEncoded, it.Description),
		})
	}
	return f
}

func atomHref(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "" || l.Rel == "alternate" {
			return l.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

func fromAtom(doc atomDoc) *feedDoc {
	f := &feedDoc{
		Title:       strings.TrimSpace(plainText(doc.Title.html())),
		Link:        atomHref(doc.Links),
		Description: strings.TrimSpace(plainText(doc.Subtitle.html())),
	}
	for _, e := range doc.Entries {
		f.Items = append(f.Items, feedItem{
			Title:    strings.TrimSpace(plainText(e.Title.html())),
			Link:     atomHref(e.Links),
			Date:     firstNonEmpty(e.Published, e.Updated),
			Markdown: e.Markdown,
			HTML:     firstNonEmpty(e.Content.html(), e.Summary.html()),
		})
	}
	return f
}

// plainText strips markup from a short HTML fragment such as a feed title.
func plainText(s string) string {
	return collapse(html.UnescapeString(bluemondayStrip.Sanitize(s)))
}

var bluemondayStrip = bluemonday.StrictPolicy()

var dateLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "2006-01-02",
}

func formatDate(s string) string {
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2 Jan 2006, 15:04")
		}
	}
	return s
}

// safeLink resolves raw against base and keeps only http and https links.
func safeLink(raw string, base *url.URL) string {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || raw == "" {
		return ""
	}
	abs := base.ResolveReference(ref)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	return abs.String()
}

// Feed renders an RSS or Atom document as HTML plus the feed title. An item's
// <source:markdown> is used when present; otherwise its HTML body is sanitized.
// ok is false when body is not a feed.
func Feed(body []byte, base *url.URL) (out, title string, ok bool, err error) {
	doc, ok := parseFeed(body)
	if !ok {
		return "", "", false, nil
	}
	var b strings.Builder
	title = doc.Title
	heading := html.EscapeString(firstNonEmpty(doc.Title, base.String()))
	if l := safeLink(doc.Link, base); l != "" {
		heading = `<a href="` + html.EscapeString(l) + `">` + heading + `</a>`
	}
	b.WriteString("<h1>" + heading + "</h1>\n")
	if doc.Description != "" {
		b.WriteString(`<p class="feed-description">` + html.EscapeString(doc.Description) + "</p>\n")
	}

	for _, it := range doc.Items {
		var content string
		switch {
		case strings.TrimSpace(it.Markdown) != "":
			content, _, err = Markdown([]byte(it.Markdown), base)
		case strings.TrimSpace(it.HTML) != "":
			content, err = SanitizeHTML(it.HTML, base)
		}
		if err != nil {
			return "", "", false, err
		}

		date := formatDate(it.Date)
		link := safeLink(it.Link, base)
		label := firstNonEmpty(it.Title, date, link, "Untitled")
		label = html.EscapeString(label)
		if link != "" {
			label = `<a href="` + html.EscapeString(link) + `">` + label + `</a>`
		}
		b.WriteString(`<article class="feed-item"><h2>` + label + "</h2>\n")
		if date != "" && it.Title != "" {
			b.WriteString(`<p class="feed-date">` + html.EscapeString(date) + "</p>\n")
		}
		b.WriteString(content + "</article>\n")
	}
	if len(doc.Items) == 0 {
		b.WriteString(`<p class="feed-description">This feed has no items.</p>` + "\n")
	}
	return b.String(), title, true, nil
}
