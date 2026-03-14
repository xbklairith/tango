package adapter

import (
	"fmt"
	"sync"
)

// Registry maps adapterType strings to Adapter implementations.
// Safe for concurrent reads after initialization.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
	unavail  map[string]string // adapterType → reason (from failed TestEnvironment)
}

// NewRegistry creates a new empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
		unavail:  make(map[string]string),
	}
}

// Register adds an adapter. Called once at startup, not concurrently.
func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Type()] = a
}

// Resolve returns the adapter for the given type, or an error if not found
// or marked unavailable.
func (r *Registry) Resolve(adapterType string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if reason, bad := r.unavail[adapterType]; bad {
		return nil, fmt.Errorf("adapter %q is unavailable: %s", adapterType, reason)
	}
	a, ok := r.adapters[adapterType]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for type %q", adapterType)
	}
	return a, nil
}

// MarkUnavailable records a startup failure for an adapter (REQ-049).
func (r *Registry) MarkUnavailable(adapterType, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unavail[adapterType] = reason
}
