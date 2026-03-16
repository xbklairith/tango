package claude

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/xb/ari/internal/adapter"
)

// blockedEnvKeys are system-critical environment variable names that adapterConfig.env
// must not override. input.EnvVars (from Ari runtime) is exempt from this restriction.
var blockedEnvKeys = map[string]bool{
	"PATH":            true,
	"HOME":            true,
	"SHELL":           true,
	"USER":            true,
	"LD_PRELOAD":      true,
	"LD_LIBRARY_PATH": true,
}

// blockedEnvPrefixes are prefixes that adapterConfig.env must not set.
// input.EnvVars (from Ari runtime) is exempt from this restriction.
var blockedEnvPrefixes = []string{"ARI_", "DYLD_"}

// Compile-time interface check.
var _ adapter.Adapter = (*ClaudeAdapter)(nil)

// ClaudeAdapter implements adapter.Adapter for Claude Code CLI execution.
// It is stateless — all configuration is passed per-invocation via AdapterConfig.
type ClaudeAdapter struct{}

// New creates a new ClaudeAdapter.
func New() *ClaudeAdapter { return &ClaudeAdapter{} }

// Type returns the adapter type identifier.
func (c *ClaudeAdapter) Type() string { return "claude_local" }

// Models returns the AI models available through the Claude CLI.
func (c *ClaudeAdapter) Models() []adapter.ModelDefinition {
	return []adapter.ModelDefinition{
		{ID: "opus", Name: "Claude Opus", Provider: "anthropic"},
		{ID: "sonnet", Name: "Claude Sonnet", Provider: "anthropic"},
		{ID: "haiku", Name: "Claude Haiku", Provider: "anthropic"},
	}
}

// TestEnvironment checks runtime prerequisites for the Claude CLI.
// At TestLevelBasic, it checks that the claude binary exists in $PATH.
// At TestLevelFull, it runs "claude --version" with a 10-second timeout.
func (c *ClaudeAdapter) TestEnvironment(level adapter.TestLevel) (adapter.TestResult, error) {
	claudePath := DefaultClaudePath
	path, err := exec.LookPath(claudePath)
	if err != nil {
		return adapter.TestResult{
			Available: false,
			Message:   fmt.Sprintf("claude CLI not found in $PATH: %v", err),
		}, nil
	}

	if level == adapter.TestLevelBasic {
		return adapter.TestResult{
			Available: true,
			Message:   fmt.Sprintf("claude CLI found at %s", path),
		}, nil
	}

	// Full: run claude --version with a 10-second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return adapter.TestResult{
			Available: false,
			Message:   fmt.Sprintf("claude --version failed: %v", err),
		}, nil
	}

	version := strings.TrimSpace(string(out))
	return adapter.TestResult{
		Available: true,
		Message:   fmt.Sprintf("claude CLI version: %s at %s", version, path),
	}, nil
}

// buildArgs constructs CLI arguments for the Claude CLI invocation.
func buildArgs(cfg Config, input adapter.InvokeInput, useResume bool) []string {
	args := []string{
		"--print", "-",                   // non-interactive mode, read prompt from stdin
		"--output-format", "stream-json", // structured streaming output
		"--verbose",                      // richer stream-json output
		"--model", cfg.Model,             // model selection
	}

	// Append system prompt — preserves Claude Code's built-in prompt
	if input.Agent.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", input.Agent.SystemPrompt)
	}

	// Skip permissions for headless execution
	if cfg.SkipPermissions != nil && *cfg.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	// Budget-based cost limit
	if cfg.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", cfg.MaxBudgetUSD))
	}

	// Allowed tools
	if len(cfg.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(cfg.AllowedTools, ","))
	}

	// Effort level
	if cfg.Effort != "" {
		args = append(args, "--effort", cfg.Effort)
	}

	// Browser automation
	if cfg.Chrome {
		args = append(args, "--chrome")
	}

	// Max conversation turns
	if cfg.MaxTurnsPerRun > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.MaxTurnsPerRun))
	}

	// Session resume
	if useResume && input.Run.SessionState != "" {
		args = append(args, "--resume", input.Run.SessionState)
	}

	// Extra args always last
	if len(cfg.ExtraArgs) > 0 {
		args = append(args, cfg.ExtraArgs...)
	}

	return args
}

// buildEnv merges environment variables with proper precedence using a map for dedup.
// Precedence (later overrides earlier): base OS env < adapterConfig.env < input.EnvVars.
// adapterConfig.env is blocked from setting ARI_*, DYLD_*, and system-critical keys
// (PATH, HOME, SHELL, USER, LD_PRELOAD, LD_LIBRARY_PATH) to prevent privilege escalation.
// input.EnvVars (from Ari runtime) is trusted and may override anything.
func buildEnv(input adapter.InvokeInput, cfg Config) []string {
	envMap := make(map[string]string)

	// 1. Base OS environment (lowest precedence)
	for _, entry := range os.Environ() {
		if k, v, ok := strings.Cut(entry, "="); ok {
			envMap[k] = v
		}
	}

	// 2. Adapter-config env vars (medium precedence, with blocklist)
	for k, v := range cfg.Env {
		if isBlockedEnvKey(k) {
			continue
		}
		envMap[k] = v
	}

	// 3. Input env vars from Ari runtime (highest precedence, no restrictions)
	for k, v := range input.EnvVars {
		envMap[k] = v
	}

	// 4. Strip Claude Code nesting vars to prevent recursive spawning (REQ-007)
	nestingPrefixes := []string{"CLAUDE_CODE_", "CLAUDECODE"}
	for key := range envMap {
		for _, prefix := range nestingPrefixes {
			if strings.HasPrefix(key, prefix) || key == prefix {
				delete(envMap, key)
				break
			}
		}
	}

	// Convert map to slice
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}
	return env
}

// isBlockedEnvKey returns true if the given key must not be set by adapterConfig.env.
func isBlockedEnvKey(key string) bool {
	if blockedEnvKeys[key] {
		return true
	}
	for _, prefix := range blockedEnvPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// executeOnce runs a single invocation of the Claude CLI and returns the result.
func (c *ClaudeAdapter) executeOnce(ctx context.Context, cfg Config, input adapter.InvokeInput, hooks adapter.Hooks, useResume bool) (adapter.InvokeResult, error) {
	args := buildArgs(cfg, input, useResume)

	// Enforce run timeout
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, cfg.ClaudePath, args...)

	// Working directory: input.WorkingDir (from run handler) > cfg.WorkingDir > default
	if input.WorkingDir != "" {
		cmd.Dir = input.WorkingDir
	} else if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}

	// Inject environment: inherit + adapter-config env + ARI_* overrides
	cmd.Env = buildEnv(input, cfg)

	// Process group so we can signal the entire tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Graceful two-phase shutdown: on context cancellation Go sends SIGTERM to
	// the process group first (via cmd.Cancel). If the process hasn't exited
	// after WaitDelay, Go automatically sends SIGKILL.
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second

	// Set up stdin pipe for prompt delivery (REQ-001)
	prompt := input.EnvVars["ARI_PROMPT"]
	if prompt == "" {
		prompt = input.Prompt
	}
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return adapter.InvokeResult{Status: adapter.RunStatusFailed}, fmt.Errorf("stdin pipe: %w", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return adapter.InvokeResult{Status: adapter.RunStatusFailed}, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return adapter.InvokeResult{Status: adapter.RunStatusFailed}, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return adapter.InvokeResult{Status: adapter.RunStatusFailed}, fmt.Errorf("start claude: %w", err)
	}

	// Write prompt to stdin in a goroutine to prevent deadlocks (REQ-030)
	go func() {
		defer stdinPipe.Close()
		if _, err := io.WriteString(stdinPipe, prompt); err != nil {
			slog.Warn("failed to write prompt to stdin", "error", err)
		}
	}()

	// Stream stdout and stderr concurrently
	collector := &eventCollector{}
	done := make(chan struct{}, 2)
	go func() {
		streamAndParseEvents(stdoutPipe, &stdoutBuf, cfg.MaxExcerptBytes, hooks, collector)
		done <- struct{}{}
	}()
	go func() {
		streamStderr(stderrPipe, &stderrBuf, cfg.MaxExcerptBytes, hooks)
		done <- struct{}{}
	}()

	// Wait for both readers to finish before cmd.Wait()
	<-done
	<-done

	waitErr := cmd.Wait()

	// Determine status (same pattern as process adapter)
	var status adapter.RunStatus
	exitCode := 0
	switch {
	case runCtx.Err() == context.DeadlineExceeded:
		status = adapter.RunStatusTimedOut
	case ctx.Err() != nil:
		// External cancellation — cmd.Cancel sent SIGTERM to the process group.
		// If still running after WaitDelay (5s), Go escalates to SIGKILL.
		status = adapter.RunStatusCancelled
	case waitErr != nil:
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		status = adapter.RunStatusFailed
	default:
		status = adapter.RunStatusSucceeded
	}

	stdout := truncate(stdoutBuf.String(), cfg.MaxExcerptBytes)
	stderr := truncate(stderrBuf.String(), cfg.MaxExcerptBytes)

	// Extract usage, session state, and cost from collected events
	usage, sessionState, costUSD := collector.extract()

	return adapter.InvokeResult{
		Status:       status,
		ExitCode:     exitCode,
		Usage:        usage,
		CostUSD:      costUSD,
		SessionState: sessionState,
		Stdout:       stdout,
		Stderr:       stderr,
	}, nil
}

// Execute spawns the Claude CLI subprocess and blocks until completion.
// It handles config parsing, argument building, subprocess management,
// stream-json event parsing, and session resume with fallback.
func (c *ClaudeAdapter) Execute(ctx context.Context, input adapter.InvokeInput, hooks adapter.Hooks) (adapter.InvokeResult, error) {
	cfg := parseConfig(input.Agent.AdapterConfig)

	// Validate workingDir
	if err := validateWorkingDir(cfg.WorkingDir); err != nil {
		return adapter.InvokeResult{Status: adapter.RunStatusFailed}, err
	}

	// Determine whether to use --resume
	useResume := input.Run.SessionState != "" && !cfg.DisableResumeOnError

	result, err := c.executeOnce(ctx, cfg, input, hooks, useResume)
	if err != nil {
		return result, err
	}

	// Smart session error detection (REQ-005): only retry on unknown session errors,
	// not arbitrary failures.
	if useResume && result.Status == adapter.RunStatusFailed {
		if isUnknownSessionError(result.Stderr, result.Stdout) {
			if hooks.OnLogLine != nil {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "warn",
					Message:   fmt.Sprintf("unknown session error (session: %s), retrying without --resume", input.Run.SessionState),
					Timestamp: time.Now(),
				})
			}
			result, err = c.executeOnce(ctx, cfg, input, hooks, false)
		}
	}

	// Detect login required (REQ-011)
	if result.Status == adapter.RunStatusFailed {
		if loginRequired, loginURL := detectLoginRequired(result.Stderr, result.Stdout); loginRequired {
			result.LoginRequired = true
			result.LoginURL = loginURL
		}
	}

	return result, err
}

// truncate returns s truncated to at most maxBytes without splitting multi-byte
// UTF-8 sequences. The result is always valid UTF-8.
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for len(s) > 0 && !utf8.Valid([]byte(s)) {
		s = s[:len(s)-1]
	}
	return s
}
