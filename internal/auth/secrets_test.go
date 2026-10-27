package auth

import (
	"errors"
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
	// For the next minute calls fail at once instead of waiting out their own timeout.
	started = time.Now()
	if _, err := callKeyring(func() (string, error) { return "x", nil }); err == nil || time.Since(started) > 20*time.Millisecond {
		t.Errorf("second call: %v after %v", err, time.Since(started))
	}
}
