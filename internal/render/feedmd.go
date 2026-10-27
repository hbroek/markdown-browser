package render

import (
	"fmt"
	"net/url"
	"strings"
)

// FeedMarkdown renders a feed as a markdown document: the feed title, then each
// entry with a linked heading, its date and its body. ok is false when body is not a feed.
func FeedMarkdown(body []byte, base *url.URL) (md string, ok bool) {
	doc, ok := parseFeed(body)
	if !ok {
		return "", false
	}
	var b strings.Builder
	heading := firstNonEmpty(plainText(doc.Title), base.String())
	if l := safeLink(doc.Link, base); l != "" {
		heading = "[" + escapeLinkText(heading) + "](<" + l + ">)"
	}
	b.WriteString("# " + heading + "\n\n")
	if d := plainText(doc.Description); d != "" {
		b.WriteString(d + "\n\n")
	}
	for _, it := range doc.Items {
		label := firstNonEmpty(plainText(it.Title), formatDate(it.Date), "Untitled")
		if l := safeLink(it.Link, base); l != "" {
			label = "[" + escapeLinkText(label) + "](<" + l + ">)"
		}
		fmt.Fprintf(&b, "## %s\n\n", label)
		if d := formatDate(it.Date); d != "" && it.Title != "" {
			b.WriteString("*" + d + "*\n\n")
		}
		content := strings.TrimSpace(it.Markdown)
		if content == "" {
			content = plainText(it.HTML)
		}
		if content != "" {
			b.WriteString(content + "\n\n")
		}
	}
	return b.String(), true
}

func escapeLinkText(s string) string {
	return strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(s)
}
