package bookmarks

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Context is the request context passed to every Store method.
type Context = context.Context

// Config holds backend settings (a file path, a server URL, ...).
type Config map[string]string

// Factory opens a backend from its settings.
type Factory func(cfg Config) (Store, error)

var (
	regMu     sync.Mutex
	factories = map[string]Factory{}
)

// Register makes a backend available under name. Backends call it from init.
func Register(name string, f Factory) {
	regMu.Lock()
	defer regMu.Unlock()
	factories[name] = f
}

// Open returns the named backend.
func Open(name string, cfg Config) (Store, error) {
	regMu.Lock()
	f, ok := factories[name]
	regMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown bookmark backend %q", name)
	}
	return f(cfg)
}

// Names lists the registered backends.
func Names() []string {
	regMu.Lock()
	defer regMu.Unlock()
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
