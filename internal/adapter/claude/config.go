// Package claude implements the "claude_local" adapter which spawns the Claude Code CLI as a subprocess.
package claude

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
// If JSON is malformed, a warning is logged and defaults are applied.
func parseConfig(raw json.RawMessage) Config {
	var cfg Config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			slog.Warn("claude adapter: failed to parse adapterConfig, using defaults", "error", err)
		}
	}

	// Security: reject claudePath values whose base name does not start with "claude".
	// This prevents adapterConfig from executing arbitrary binaries (e.g., /bin/sh).
	// If the value is rejected we fall through to the default ("claude").
	if cfg.ClaudePath != "" && !strings.HasPrefix(filepath.Base(cfg.ClaudePath), "claude") {
		slog.Warn("claude adapter: claudePath does not resolve to a claude binary, using default",
			"claudePath", cfg.ClaudePath)
		cfg.ClaudePath = ""
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

// validateWorkingDir checks that dir is an absolute path with no ".." path segments.
// An empty string is valid (the adapter will use the default working directory).
func validateWorkingDir(dir string) error {
	if dir == "" {
		return nil
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("workingDir must be an absolute path: %s", dir)
	}
	// Use filepath.Clean then check individual segments so that patterns like
	// "foo..bar" (no path separator) are not falsely rejected.
	cleaned := filepath.Clean(dir)
	for _, seg := range strings.Split(cleaned, string(filepath.Separator)) {
		if seg == ".." {
			return fmt.Errorf("workingDir must not contain path traversal segments: %s", dir)
		}
	}
	return nil
}
