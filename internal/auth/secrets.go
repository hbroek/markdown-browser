package auth

import (
	"errors"
	"sync"

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
	v, err := keyring.Get(service, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (KeyringStore) Set(name, value string) error { return keyring.Set(service, name, value) }

func (KeyringStore) Delete(name string) error {
	err := keyring.Delete(service, name)
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
