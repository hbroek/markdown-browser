package auth

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

const service = "markdown-browser"

// ErrNotFound is returned by Secrets.Get when nothing is stored under name.
var ErrNotFound = errors.New("secret not found")

// Secrets is a small store for sensitive values.
type Secrets interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}

// KeyringStore keeps secrets in the OS keychain (macOS Keychain, Windows
// Credential Manager, or the Linux Secret Service).
type KeyringStore struct{}

func (KeyringStore) Get(name string) (string, error) {
	v, err := callKeyring(func() (string, error) { return keyring.Get(service, name) })
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (KeyringStore) Set(name, value string) error {
	_, err := callKeyring(func() (struct{}, error) { return struct{}{}, keyring.Set(service, name, value) })
	return explainKeyring(err)
}

// keyringTimeout is how long a keyring call may take. A locked Linux keyring waits
// for someone to answer an unlock prompt, which may be hidden behind a window or never
// shown; without a limit the whole app waits with it.
var keyringTimeout = 30 * time.Second

var keyringDown struct {
	mu    sync.Mutex
	until time.Time
}

// keyringSilent is what to do when the keyring did not answer in time.
const keyringSilent = "the system keyring did not answer. A password prompt to unlock it may be waiting behind this window " +
	"(look in the taskbar), or the keyring is not running. Unlock the \"login\" keyring in \"Passwords and Keys\" (Seahorse), then try again"

// callKeyring runs one keyring call and gives up after keyringTimeout. After a timeout,
// calls fail at once for a minute, so every request does not wait out its own timeout.
func callKeyring[T any](fn func() (T, error)) (T, error) {
	var zero T
	keyringDown.mu.Lock()
	down := time.Now().Before(keyringDown.until)
	keyringDown.mu.Unlock()
	if down {
		return zero, errors.New(keyringSilent)
	}
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1) // buffered: a call that finishes late must not block
	go func() {
		v, err := fn()
		done <- result{v, err}
	}()
	select {
	case r := <-done:
		return r.v, r.err
	case <-time.After(keyringTimeout):
		keyringDown.mu.Lock()
		keyringDown.until = time.Now().Add(time.Minute)
		keyringDown.mu.Unlock()
		return zero, errors.New(keyringSilent)
	}
}

// explainKeyring adds what to do when the Linux Secret Service is missing,
// locked or has no default keyring: the usual case on a desktop without
// GNOME, or a fresh account that never created a "login" keyring.
func explainKeyring(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, sign := range []string{"org.freedesktop", "unlock", "dbus", "D-Bus", "secret service", "Secret Service"} {
		if strings.Contains(msg, sign) {
			return fmt.Errorf("%w\n\nOn Linux, secrets need a Secret Service keyring that is running and unlocked. "+
				"Install gnome-keyring (or enable KeePassXC's Secret Service), open \"Passwords and Keys\" (Seahorse), "+
				"create a keyring, make it the default and unlock it, then try again", err)
		}
	}
	return err
}

func (KeyringStore) Delete(name string) error {
	_, err := callKeyring(func() (struct{}, error) { return struct{}{}, keyring.Delete(service, name) })
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// MemorySecrets is an in-memory Secrets, used in tests.
type MemorySecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *MemorySecrets) Get(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *MemorySecrets) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[name] = value
	return nil
}

func (s *MemorySecrets) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
	return nil
}
