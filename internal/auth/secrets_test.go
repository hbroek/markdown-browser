package auth

import (
	"errors"
	"strings"
	"testing"
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
