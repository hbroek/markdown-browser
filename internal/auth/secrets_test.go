package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExplainKeyring(t *testing.T) {
	if explainKeyring(nil) != nil {
		t.Error("nil became an error")
	}
	locked := errors.New("failed to unlock correct collection '/org/freedesktop/secrets/aliases/default'")
	got := explainKeyring(locked)
	if !errors.Is(got, locked) || !strings.Contains(got.Error(), "Seahorse") {
		t.Errorf("no advice added: %v", got)
	}
	other := errors.New("disk full")
	if explainKeyring(other) != other {
		t.Error("an unrelated error was reworded")
	}
}

func TestKeyringCallGivesUpWhenNobodyAnswers(t *testing.T) {
	old := keyringTimeout
	keyringTimeout = 50 * time.Millisecond
	defer func() {
		keyringTimeout = old
		keyringDown.mu.Lock()
		keyringDown.until = time.Time{}
		keyringDown.mu.Unlock()
	}()

	release := make(chan struct{})
	defer close(release)
	started := time.Now()
	_, err := callKeyring(func() (string, error) { <-release; return "late", nil }) // an unlock prompt nobody answers
	if err == nil || !strings.Contains(err.Error(), "Seahorse") || time.Since(started) > 2*time.Second {
		t.Fatalf("err = %v after %v", err, time.Since(started))
	}
	// For the pause calls fail at once instead of waiting out their own timeout.
	started = time.Now()
	if _, err := callKeyring(func() (string, error) { return "x", nil }); err == nil || time.Since(started) > 20*time.Millisecond {
		t.Errorf("second call: %v after %v", err, time.Since(started))
	}
}

type brokenSecrets struct{}

func (brokenSecrets) Get(string) (string, error) { return "", errors.New("no keyring") }
func (brokenSecrets) Set(string, string) error   { return errors.New("no keyring") }
func (brokenSecrets) Delete(string) error        { return errors.New("no keyring") }

func TestFallbackStoresInEncryptedFile(t *testing.T) {
	dir := t.TempDir()
	s := WithFallback{Primary: brokenSecrets{}, Fallback: NewFileStore(dir)}
	if err := s.Set("k", "lgg_secret"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get("k"); err != nil || v != "lgg_secret" {
		t.Fatalf("get = %q, %v", v, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "secrets.bin"))
	if len(raw) == 0 || strings.Contains(string(raw), "lgg_secret") {
		t.Error("secret is not encrypted on disk")
	}
	if v, err := (WithFallback{Primary: brokenSecrets{}, Fallback: NewFileStore(dir)}).Get("k"); err != nil || v != "lgg_secret" {
		t.Errorf("after reopening: %q, %v", v, err)
	}
	s.Delete("k")
	if _, err := s.Get("k"); err == nil {
		t.Error("deleted secret still readable")
	}
}

func TestFallbackPrefersWorkingKeyring(t *testing.T) {
	primary, file := &MemorySecrets{}, NewFileStore(t.TempDir())
	file.Set("k", "old")
	s := WithFallback{Primary: primary, Fallback: file}
	s.Set("k", "new")
	if v, _ := s.Get("k"); v != "new" {
		t.Errorf("got %q", v)
	}
	if _, err := file.Get("k"); err == nil {
		t.Error("stale copy left in the file")
	}
}
