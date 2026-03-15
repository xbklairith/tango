package claude

import (
	"encoding/json"
	"testing"
)

func TestParseConfig_Defaults(t *testing.T) {
	cfg := parseConfig(nil)

	if cfg.ClaudePath != "claude" {
		t.Errorf("ClaudePath = %q, want %q", cfg.ClaudePath, "claude")
	}
	if cfg.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", cfg.Model, "sonnet")
	}
	if cfg.TimeoutSeconds != 3600 {
		t.Errorf("TimeoutSeconds = %d, want %d", cfg.TimeoutSeconds, 3600)
	}
	if cfg.MaxExcerptBytes != 65536 {
		t.Errorf("MaxExcerptBytes = %d, want %d", cfg.MaxExcerptBytes, 65536)
	}
	if cfg.SkipPermissions == nil || *cfg.SkipPermissions != true {
		t.Errorf("SkipPermissions = %v, want pointer to true", cfg.SkipPermissions)
	}
	if cfg.MaxBudgetUSD != 0 {
		t.Errorf("MaxBudgetUSD = %f, want %f", cfg.MaxBudgetUSD, 0.0)
	}
}

func TestParseConfig_AllFields(t *testing.T) {
	raw := json.RawMessage(`{
		"claudePath": "/usr/local/bin/claude",
		"model": "opus",
		"allowedTools": ["Read", "Write"],
		"workingDir": "/opt/workspace",
		"timeoutSeconds": 1800,
		"maxExcerptBytes": 32768,
		"maxBudgetUSD": 5.00,
		"skipPermissions": false,
		"env": {"ANTHROPIC_API_KEY": "sk-test"},
		"disableResumeOnError": true
	}`)

	cfg := parseConfig(raw)

	if cfg.ClaudePath != "/usr/local/bin/claude" {
		t.Errorf("ClaudePath = %q, want %q", cfg.ClaudePath, "/usr/local/bin/claude")
	}
	if cfg.Model != "opus" {
		t.Errorf("Model = %q, want %q", cfg.Model, "opus")
	}
	if len(cfg.AllowedTools) != 2 || cfg.AllowedTools[0] != "Read" || cfg.AllowedTools[1] != "Write" {
		t.Errorf("AllowedTools = %v, want [Read, Write]", cfg.AllowedTools)
	}
	if cfg.WorkingDir != "/opt/workspace" {
		t.Errorf("WorkingDir = %q, want %q", cfg.WorkingDir, "/opt/workspace")
	}
	if cfg.TimeoutSeconds != 1800 {
		t.Errorf("TimeoutSeconds = %d, want %d", cfg.TimeoutSeconds, 1800)
	}
	if cfg.MaxExcerptBytes != 32768 {
		t.Errorf("MaxExcerptBytes = %d, want %d", cfg.MaxExcerptBytes, 32768)
	}
	if cfg.MaxBudgetUSD != 5.00 {
		t.Errorf("MaxBudgetUSD = %f, want %f", cfg.MaxBudgetUSD, 5.00)
	}
	if cfg.SkipPermissions == nil || *cfg.SkipPermissions != false {
		t.Errorf("SkipPermissions = %v, want pointer to false", cfg.SkipPermissions)
	}
	if cfg.Env["ANTHROPIC_API_KEY"] != "sk-test" {
		t.Errorf("Env[ANTHROPIC_API_KEY] = %q, want %q", cfg.Env["ANTHROPIC_API_KEY"], "sk-test")
	}
	if !cfg.DisableResumeOnError {
		t.Error("DisableResumeOnError = false, want true")
	}
}

func TestParseConfig_PartialFields(t *testing.T) {
	raw := json.RawMessage(`{"model": "haiku", "maxBudgetUSD": 2.50}`)

	cfg := parseConfig(raw)

	if cfg.Model != "haiku" {
		t.Errorf("Model = %q, want %q", cfg.Model, "haiku")
	}
	if cfg.MaxBudgetUSD != 2.50 {
		t.Errorf("MaxBudgetUSD = %f, want %f", cfg.MaxBudgetUSD, 2.50)
	}
	// Defaults for unset fields
	if cfg.ClaudePath != "claude" {
		t.Errorf("ClaudePath = %q, want default %q", cfg.ClaudePath, "claude")
	}
	if cfg.TimeoutSeconds != 3600 {
		t.Errorf("TimeoutSeconds = %d, want default %d", cfg.TimeoutSeconds, 3600)
	}
	if cfg.MaxExcerptBytes != 65536 {
		t.Errorf("MaxExcerptBytes = %d, want default %d", cfg.MaxExcerptBytes, 65536)
	}
	if cfg.SkipPermissions == nil || *cfg.SkipPermissions != true {
		t.Errorf("SkipPermissions = %v, want pointer to true (default)", cfg.SkipPermissions)
	}
}

func TestParseConfig_EmptyJSON(t *testing.T) {
	raw := json.RawMessage(`{}`)

	cfg := parseConfig(raw)

	if cfg.ClaudePath != "claude" {
		t.Errorf("ClaudePath = %q, want %q", cfg.ClaudePath, "claude")
	}
	if cfg.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", cfg.Model, "sonnet")
	}
	if cfg.TimeoutSeconds != 3600 {
		t.Errorf("TimeoutSeconds = %d, want %d", cfg.TimeoutSeconds, 3600)
	}
	if cfg.MaxExcerptBytes != 65536 {
		t.Errorf("MaxExcerptBytes = %d, want %d", cfg.MaxExcerptBytes, 65536)
	}
	if cfg.SkipPermissions == nil || *cfg.SkipPermissions != true {
		t.Errorf("SkipPermissions = %v, want pointer to true", cfg.SkipPermissions)
	}
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{broken json!!!`)

	cfg := parseConfig(raw)

	// Defaults should be applied despite malformed JSON
	if cfg.ClaudePath != "claude" {
		t.Errorf("ClaudePath = %q, want %q", cfg.ClaudePath, "claude")
	}
	if cfg.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", cfg.Model, "sonnet")
	}
	if cfg.TimeoutSeconds != 3600 {
		t.Errorf("TimeoutSeconds = %d, want %d", cfg.TimeoutSeconds, 3600)
	}
	if cfg.MaxExcerptBytes != 65536 {
		t.Errorf("MaxExcerptBytes = %d, want %d", cfg.MaxExcerptBytes, 65536)
	}
	if cfg.SkipPermissions == nil || *cfg.SkipPermissions != true {
		t.Errorf("SkipPermissions = %v, want pointer to true", cfg.SkipPermissions)
	}
}

func TestParseConfig_SkipPermissionsFalse(t *testing.T) {
	raw := json.RawMessage(`{"skipPermissions": false}`)

	cfg := parseConfig(raw)

	if cfg.SkipPermissions == nil {
		t.Fatal("SkipPermissions is nil, want pointer to false")
	}
	if *cfg.SkipPermissions != false {
		t.Errorf("*SkipPermissions = %v, want false", *cfg.SkipPermissions)
	}
}

func TestValidateWorkingDir_Absolute(t *testing.T) {
	err := validateWorkingDir("/opt/agents/workspace")
	if err != nil {
		t.Errorf("unexpected error for absolute path: %v", err)
	}
}

func TestValidateWorkingDir_Relative(t *testing.T) {
	err := validateWorkingDir("relative/path")
	if err == nil {
		t.Error("expected error for relative path, got nil")
	}
}

func TestValidateWorkingDir_DotDot_Cleaned(t *testing.T) {
	// /opt/agents/../escape cleans to /opt/escape — no traversal segments after cleaning.
	// The new segment-based check (via filepath.Clean) accepts this, unlike the old
	// strings.Contains approach which would have rejected it.
	err := validateWorkingDir("/opt/agents/../escape")
	if err != nil {
		t.Errorf("unexpected error for path that cleans to valid absolute path: %v", err)
	}
}

func TestValidateWorkingDir_DotDotSubstring(t *testing.T) {
	// A path containing ".." as a substring but not as an actual segment (e.g., "foo..bar")
	// should be accepted. The old strings.Contains check was too aggressive.
	err := validateWorkingDir("/opt/foo..bar/workspace")
	if err != nil {
		t.Errorf("unexpected error for path with '..' substring (not a segment): %v", err)
	}
}

func TestValidateWorkingDir_RelativeDotDot(t *testing.T) {
	// Relative paths with ".." are rejected because they are not absolute.
	err := validateWorkingDir("../escape")
	if err == nil {
		t.Error("expected error for relative path with '..', got nil")
	}
}

func TestValidateWorkingDir_Empty(t *testing.T) {
	err := validateWorkingDir("")
	if err != nil {
		t.Errorf("unexpected error for empty string: %v", err)
	}
}
