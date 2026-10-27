package chats

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPutGetPersistsAndIgnoresFragment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "conversations.json")
	s := Open(path)
	s.Put("https://x.test/a#intro", []Turn{
		{Role: "user", Text: "Hi?"},
		{Role: "assistant", Text: "Hello.", Source: "m", Millis: 1200},
		{Role: "error", Text: "boom"},
		{Role: "user", Text: "  "},
	})
	got := Open(path).Get("https://x.test/a#other") // a new Store reads the file
	if len(got) != 2 || got[1].Text != "Hello." || got[1].Source != "m" || got[1].Millis != 1200 {
		t.Fatalf("Get = %+v", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file = %v, %v; want mode 0600", info, err)
	}
	if len(s.Get("https://x.test/other")) != 0 {
		t.Error("another page got the conversation")
	}
}

func TestDeleteClearAndEmptyPut(t *testing.T) {
	s := Open("")
	s.Put("a", []Turn{{Role: "user", Text: "q"}})
	s.Put("b", []Turn{{Role: "user", Text: "q"}})
	s.Delete("a")
	if s.Len() != 1 {
		t.Errorf("Len = %d", s.Len())
	}
	s.Put("b", nil) // an empty conversation removes the entry
	if s.Len() != 0 {
		t.Errorf("Len = %d after empty Put", s.Len())
	}
	s.Put("c", []Turn{{Role: "user", Text: "q"}})
	s.Clear()
	if s.Len() != 0 {
		t.Error("Clear left conversations")
	}
}

func TestLimits(t *testing.T) {
	s := Open("")
	var many []Turn
	for i := 0; i < MaxTurns+10; i++ {
		many = append(many, Turn{Role: "user", Text: fmt.Sprint(i)})
	}
	s.Put("p", many)
	got := s.Get("p")
	if len(got) != MaxTurns || got[len(got)-1].Text != fmt.Sprint(MaxTurns+9) {
		t.Errorf("kept %d turns, last %q", len(got), got[len(got)-1].Text)
	}
	for i := 0; i < MaxPages+5; i++ {
		s.Put(fmt.Sprintf("page%d", i), []Turn{{Role: "user", Text: "q"}})
	}
	if s.Len() != MaxPages {
		t.Errorf("kept %d pages, want %d", s.Len(), MaxPages)
	}
	if len(s.Get("page0")) != 0 || len(s.Get(fmt.Sprintf("page%d", MaxPages+4))) == 0 {
		t.Error("the oldest pages should go first")
	}
}

func TestDamagedFileGivesEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(path, []byte("{not json"), 0o600)
	s := Open(path)
	if s.Len() != 0 {
		t.Error("damaged file was read")
	}
	s.Put("a", []Turn{{Role: "user", Text: "q"}}) // and it can be written over
	if Open(path).Len() != 1 {
		t.Error("could not recover")
	}
}
