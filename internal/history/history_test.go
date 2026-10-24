package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddOrderAndDedupe(t *testing.T) {
	l := Open("")
	l.Add("a", "A")
	l.Add("b", "B")
	l.Add("a", "A2")
	got := l.Entries()
	if len(got) != 2 || got[0].URL != "a" || got[0].Title != "A2" || got[1].URL != "b" {
		t.Errorf("entries = %+v", got)
	}
}

func TestCap(t *testing.T) {
	l := Open("")
	for i := 0; i < MaxEntries+25; i++ {
		l.Add(fmt.Sprint("u", i), "")
	}
	got := l.Entries()
	if len(got) != MaxEntries || got[0].URL != fmt.Sprint("u", MaxEntries+24) {
		t.Errorf("len=%d first=%q", len(got), got[0].URL)
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "history.json")
	l := Open(path)
	l.Add("a", "A")
	l.Add("b", "B")

	got := Open(path).Entries()
	if len(got) != 2 || got[0].URL != "b" || got[1].Title != "A" {
		t.Errorf("reloaded = %+v", got)
	}

	l.Clear()
	if n := len(Open(path).Entries()); n != 0 {
		t.Errorf("after clear: %d entries on disk", n)
	}
}

func TestCorruptOrMissingFile(t *testing.T) {
	dir := t.TempDir()
	if n := len(Open(filepath.Join(dir, "missing.json")).Entries()); n != 0 {
		t.Errorf("missing file: %d entries", n)
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o644)
	l := Open(bad)
	if n := len(l.Entries()); n != 0 {
		t.Errorf("corrupt file: %d entries", n)
	}
	l.Add("a", "A") // recovers by overwriting
	if n := len(Open(bad).Entries()); n != 1 {
		t.Errorf("after recovery: %d entries", n)
	}
}

func TestMarkdown(t *testing.T) {
	day1 := time.Date(2026, 10, 23, 20, 1, 0, 0, time.Local)
	day2 := time.Date(2026, 10, 22, 9, 30, 0, 0, time.Local)
	out := Markdown([]Entry{
		{URL: "https://a.example/x y", Title: "A [*weird*] title", Time: day1},
		{URL: "https://b.example/", Title: "", Time: day1.Add(-time.Hour)},
		{URL: "https://c.example/", Title: "C", Time: day2},
	})
	for _, want := range []string{
		"# History",
		"3 pages",
		"## Friday, 23 October 2026",
		"## Thursday, 22 October 2026",
		`- 20:01 [A \[\*weird\*\] title](<https://a.example/x y>)`,
		"- 19:01 [https://b.example/](<https://b.example/>)",
		"- 09:30 [C](<https://c.example/>)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "## Friday") != 1 {
		t.Errorf("days not grouped:\n%s", out)
	}
	if !strings.Contains(Markdown(nil), "No pages visited yet") {
		t.Error("empty history text missing")
	}
}

func TestWriteMarkdownAndSelfExclusion(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "history.json"))
	l.Add("https://a.example/", "A")
	path, err := l.WriteMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "[A](<https://a.example/>)") {
		t.Errorf("file = %s", data)
	}
	l.Add(l.markdownURL(), "History")
	if n := len(l.Entries()); n != 1 {
		t.Errorf("history page recorded itself: %d entries", n)
	}
}
