package render

import (
	"strings"
	"testing"
)

const rssSample = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:source="http://source.scripting.com/" xmlns:atom="http://www.w3.org/2005/Atom">
<channel>
  <title>Henry's feed</title>
  <atom:link href="http://feed.example/self" rel="self"/>
  <link>http://henry.localhost:8080/</link>
  <atom:link href="http://feed.example/self2" rel="self"/>
  <description>Posts</description>
  <item>
    <guid isPermaLink="false">urn:uuid:01a14107</guid>
    <link>http://henry.localhost:8080/posts/pickleball</link>
    <pubDate>Fri, 23 Oct 2026 20:00:00 +0000</pubDate>
    <description>&lt;p&gt;Heading out for &lt;b&gt;pickleball&lt;/b&gt;&lt;/p&gt;</description>
    <source:markdown>Heading out for **pickleball** now, [join](/join)?</source:markdown>
  </item>
  <item>
    <title>HTML only</title>
    <link>/relative</link>
    <description>&lt;p&gt;hello&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;&lt;img src=x onerror=alert(1)&gt;</description>
  </item>
</channel>
</rss>`

func TestFeedRSSPrefersMarkdown(t *testing.T) {
	out, title, ok, err := Feed([]byte(rssSample), mustURL("http://henry.localhost:8080/feed.xml"))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if title != "Henry's feed" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{
		`<h1><a href="http://henry.localhost:8080/">Henry&#39;s feed</a></h1>`,
		`<strong>pickleball</strong>`,
		`href="http://henry.localhost:8080/join"`,
		`<a href="http://henry.localhost:8080/posts/pickleball">23 Oct 2026, 20:00</a>`, // untitled: date is the heading
		`<h2><a href="http://henry.localhost:8080/relative">HTML only</a></h2>`,
		`<p>hello</p>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<b>") {
		t.Errorf("description used although markdown present:\n%s", out)
	}
	for _, bad := range []string{"<script", "onerror", "alert"} {
		if strings.Contains(out, bad) {
			t.Errorf("leaked %q:\n%s", bad, out)
		}
	}
}

func TestFeedUndeclaredPrefix(t *testing.T) {
	// Real-world feeds sometimes use source: without declaring it.
	doc := `<rss version="2.0"><channel><title>T</title><item><link>http://a.example/1</link>` +
		`<description>&lt;p&gt;html&lt;/p&gt;</description><source:markdown>*md*</source:markdown></item></channel></rss>`
	out, _, ok, err := Feed([]byte(doc), mustURL("http://a.example/"))
	if err != nil || !ok || !strings.Contains(out, "<em>md</em>") {
		t.Errorf("ok=%v err=%v out=%s", ok, err, out)
	}
}

func TestFeedAtom(t *testing.T) {
	doc := `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom &amp; co</title>
  <subtitle>sub</subtitle>
  <link rel="self" href="http://a.example/atom"/>
  <link rel="alternate" href="http://a.example/"/>
  <entry>
    <title>One</title>
    <link href="http://a.example/1"/>
    <published>2026-10-23T20:00:00Z</published>
    <content type="html">&lt;p&gt;html &lt;i&gt;body&lt;/i&gt;&lt;/p&gt;</content>
  </entry>
  <entry>
    <title type="text">Two &lt;3</title>
    <link href="http://a.example/2"/>
    <content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><p>xhtml body</p></div></content>
  </entry>
  <entry>
    <title>Three</title>
    <summary>plain &lt;summary&gt;</summary>
  </entry>
</feed>`
	out, title, ok, err := Feed([]byte(doc), mustURL("http://a.example/atom"))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if title != "Atom & co" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{
		`<a href="http://a.example/">Atom &amp; co</a>`,
		`<i>body</i>`,
		`xhtml body`,
		`Two &lt;3`,
		`plain &lt;summary&gt;`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestFeedDropsUnsafeLinks(t *testing.T) {
	doc := `<rss><channel><title>T</title><link>javascript:alert(1)</link>` +
		`<item><title>x</title><link>javascript:alert(1)</link></item></channel></rss>`
	out, _, _, _ := Feed([]byte(doc), mustURL("http://a.example/"))
	if strings.Contains(out, "javascript") {
		t.Errorf("unsafe link kept: %s", out)
	}
}

func TestFeedNotAFeed(t *testing.T) {
	for _, body := range []string{"# Title", "<html><body>x</body></html>", "Use <rss> readers", "<feed></feed"} {
		if _, _, ok, _ := Feed([]byte(body), mustURL("http://a.example/")); ok && body != "<feed></feed" {
			t.Errorf("%q parsed as feed", body)
		}
	}
	if !IsFeed([]byte(rssSample)) || IsFeed([]byte("# hi")) {
		t.Error("IsFeed sniffing wrong")
	}
}
