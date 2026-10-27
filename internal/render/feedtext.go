package render

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	maxFeedTextItems = 40
	maxFeedItemRunes = 700
)

// FeedText renders a feed as plain text for a language model: one block per
// entry with its title, date, address and the start of its body. ok is false
// when body is not a feed.
func FeedText(body []byte, base *url.URL) (title, text string, ok bool) {
	doc, ok := parseFeed(body)
	if !ok {
		return "", "", false
	}
	var b strings.Builder
	if doc.Description != "" {
		b.WriteString(plainText(doc.Description) + "\n\n")
	}
	for i, it := range doc.Items {
		if i == maxFeedTextItems {
			fmt.Fprintf(&b, "[%d more entries were left out.]\n", len(doc.Items)-i)
			break
		}
		fmt.Fprintf(&b, "## %d. %s\n", i+1, firstNonEmpty(plainText(it.Title), "Untitled"))
		if d := formatDate(it.Date); d != "" {
			b.WriteString("Date: " + d + "\n")
		}
		if l := safeLink(it.Link, base); l != "" {
			b.WriteString("Link: " + l + "\n")
		}
		content := strings.TrimSpace(it.Markdown)
		if content == "" {
			content = plainText(it.HTML)
		}
		if r := []rune(content); len(r) > maxFeedItemRunes {
			content = string(r[:maxFeedItemRunes]) + "…"
		}
		b.WriteString(content + "\n\n")
	}
	return doc.Title, b.String(), true
}
