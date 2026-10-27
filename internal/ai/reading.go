package ai

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

// Outline returns a markdown outline of a page: its sections with a line on each.
func Outline(ctx context.Context, c Chatter, title, pageURL, text string) (string, error) {
	system := "You make an outline of a web page for a reader who wants to find their way around a long page. " +
		"Reply in Markdown: a nested bullet list of the page's sections and main points in the order they appear, " +
		"at most 3 levels deep. Start each top-level bullet with the section's name in bold, followed by a dash and one short line saying what it covers. " +
		"Be faithful to the page; do not add facts that are not in it. " + injectionNote
	return c.Chat(ctx, system, pageMessage(title, pageURL, text))
}

// languageName checks the name of a language the page is translated into.
func languageName(lang string) (string, error) {
	if strings.ContainsAny(lang, "\r\n\t") {
		return "", errors.New("that is not a language name")
	}
	lang = strings.Join(strings.Fields(lang), " ")
	if lang == "" || len([]rune(lang)) > 40 {
		return "", errors.New("choose a language to translate into")
	}
	for _, r := range lang {
		if !unicode.IsLetter(r) && !unicode.IsSpace(r) && r != '-' && r != '(' && r != ')' {
			return "", errors.New("that is not a language name")
		}
	}
	return lang, nil
}

// Translate returns the page text in another language, keeping its Markdown.
func Translate(ctx context.Context, c Chatter, lang, title, pageURL, text string) (string, error) {
	lang, err := languageName(lang)
	if err != nil {
		return "", err
	}
	system := "You translate web pages for a reader in a markdown browser. Translate the whole page text into " + lang + ". " +
		"Keep the structure: headings, lists, tables, links and code stay as they are, and only the human language is translated. " +
		"Do not summarize, explain or add notes, and reply with the translation only. " + injectionNote
	return c.Chat(ctx, system, pageMessage(title, pageURL, text))
}

// Digest returns a briefing of a feed: what is new, grouped by topic, with links.
func Digest(ctx context.Context, c Chatter, title, pageURL, text string) (string, error) {
	system := "You write a digest of an RSS or Atom feed for a reader who has no time to open every entry. " +
		"Reply in Markdown: one sentence on what the feed is about and what has been happening, then the entries grouped under short topic headings (###). " +
		"Under each heading list the entries as bullets: a Markdown link using the entry's Link as [its title](link), then a dash and one line on why it matters. " +
		"Put the most important topics first, merge entries about the same thing, and leave out trivial ones. " +
		"Only use links that appear in the text. Be faithful to the feed; do not add facts that are not in it. " + injectionNote
	return c.Chat(ctx, system, pageMessage(title, pageURL, text))
}
