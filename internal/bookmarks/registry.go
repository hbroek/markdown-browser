package bookmarks

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// Context is the request context passed to every Store method.
type Context = context.Context

// Config holds backend settings (a file path, a server URL, ...).
type Config map[string]string

// Deps are app services a backend may need. Authorize attaches the user's saved
// credential to a request (a no-op or nil when there is none).
type Deps struct {
	Authorize func(*http.Request)
}

// Factory opens a backend from its settings.
type Factory func(cfg Config, deps Deps) (Store, error)

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
func Open(name string, cfg Config, deps Deps) (Store, error) {
	regMu.Lock()
	f, ok := factories[name]
	regMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown bookmark backend %q", name)
	}
	return f(cfg, deps)
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
