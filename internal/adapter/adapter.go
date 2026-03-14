// Package adapter defines interfaces for agent runtime adapters.
package adapter

// Adapter is the interface that agent runtime adapters must implement.
type Adapter interface {
	// Name returns the adapter's identifier (e.g., "claude-code", "openai").
	Name() string
}
