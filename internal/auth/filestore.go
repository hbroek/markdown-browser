package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// FileStore keeps secrets in an AES-GCM encrypted file, for machines with no system
// keyring. The key sits in a second file next to it (both 0600), so this stops
// casual reading and stray copies of the folder, not another process of the same user.
type FileStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileStore(dir string) *FileStore { return &FileStore{dir: dir} }

func (s *FileStore) key() ([]byte, error) {
	path := filepath.Join(s.dir, "secrets.key")
	if k, err := os.ReadFile(path); err == nil && len(k) == 32 {
		return k, nil
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, err
	}
	return k, os.WriteFile(path, k, 0o600)
}

func (s *FileStore) gcm() (cipher.AEAD, error) {
	k, err := s.key()
	if err != nil {
		return nil, err
	}
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func (s *FileStore) load() (map[string]string, error) {
	m := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(s.dir, "secrets.bin"))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	g, err := s.gcm()
	if err != nil {
		return nil, err
	}
	if len(raw) < g.NonceSize() {
		return nil, errors.New("the secrets file is damaged")
	}
	plain, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	if err != nil {
		return nil, errors.New("the secrets file cannot be decrypted; enter the keys again")
	}
	return m, json.Unmarshal(plain, &m)
}

func (s *FileStore) save(m map[string]string) error {
	g, err := s.gcm()
	if err != nil {
		return err
	}
	plain, _ := json.Marshal(m)
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "secrets.bin"), g.Seal(nonce, nonce, plain, nil), 0o600)
}

func (s *FileStore) Get(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return "", err
	}
	v, ok := m[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *FileStore) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		m = map[string]string{} // unreadable: start over rather than refuse to save
	}
	m[name] = value
	return s.save(m)
}

func (s *FileStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return nil
	}
	if _, ok := m[name]; !ok {
		return nil
	}
	delete(m, name)
	return s.save(m)
}

// WithFallback uses primary, and the fallback when primary cannot be reached (no
// keyring running, locked, silent). A secret found in neither is ErrNotFound.
type WithFallback struct{ Primary, Fallback Secrets }

func (w WithFallback) Get(name string) (string, error) {
	v, err := w.Primary.Get(name)
	if err == nil {
		return v, nil
	}
	if fv, ferr := w.Fallback.Get(name); ferr == nil {
		return fv, nil
	}
	return "", err
}

func (w WithFallback) Set(name, value string) error {
	if err := w.Primary.Set(name, value); err != nil {
		return w.Fallback.Set(name, value)
	}
	w.Fallback.Delete(name) // an older copy there must not shadow this one
	return nil
}

func (w WithFallback) Delete(name string) error {
	err := w.Primary.Delete(name)
	if ferr := w.Fallback.Delete(name); err == nil {
		err = ferr
	}
	return err
}
