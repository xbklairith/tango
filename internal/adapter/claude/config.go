// Package claude implements the "claude_local" adapter which spawns the Claude Code CLI as a subprocess.
package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// DefaultClaudePath is the default binary name for the Claude CLI.
const DefaultClaudePath = "claude"

// DefaultModel is the default Claude model used when none is specified.
const DefaultModel = "sonnet"

// DefaultTimeoutSeconds is the default max runtime for a Claude CLI invocation.
const DefaultTimeoutSeconds = 3600

// DefaultMaxExcerptBytes is the default max bytes captured from stdout/stderr.
const DefaultMaxExcerptBytes = 65536

// Config is the JSON schema for the claude_local adapter's adapterConfig.
type Config struct {
	ClaudePath           string            `json:"claudePath"`
	Model                string            `json:"model"`
	AllowedTools         []string          `json:"allowedTools"`
	WorkingDir           string            `json:"workingDir"`
	TimeoutSeconds       int               `json:"timeoutSeconds"`
	MaxExcerptBytes      int               `json:"maxExcerptBytes"`
	MaxBudgetUSD         float64           `json:"maxBudgetUSD"`
	SkipPermissions      *bool             `json:"skipPermissions"`      // pointer to distinguish unset from false
	Env                  map[string]string `json:"env"`
	DisableResumeOnError bool              `json:"disableResumeOnError"`
}

// parseConfig extracts Config from adapterConfig JSON, applying defaults for missing fields.
// Malformed JSON is silently ignored and defaults are applied.
func parseConfig(raw json.RawMessage) Config {
	var cfg Config
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	if cfg.ClaudePath == "" {
		cfg.ClaudePath = DefaultClaudePath
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if cfg.MaxExcerptBytes == 0 {
		cfg.MaxExcerptBytes = DefaultMaxExcerptBytes
	}
	// SkipPermissions defaults to true (headless agents need this).
	if cfg.SkipPermissions == nil {
		t := true
		cfg.SkipPermissions = &t
	}
	return cfg
}

// validateWorkingDir checks that dir is an absolute path with no ".." segments.
// An empty string is valid (the adapter will use the default working directory).
func validateWorkingDir(dir string) error {
	if dir == "" {
		return nil
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("workingDir must be an absolute path: %s", dir)
	}
	if strings.Contains(dir, "..") {
		return fmt.Errorf("workingDir must not contain '..' segments: %s", dir)
	}
	return nil
}
