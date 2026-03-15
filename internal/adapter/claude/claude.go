package claude

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/xb/ari/internal/adapter"
)

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
		"--print",                        // non-interactive mode
		"--output-format", "stream-json", // structured streaming output
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

	// Disable local session persistence — Ari manages sessions
	args = append(args, "--no-session-persistence")

	// Budget-based cost limit
	if cfg.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", cfg.MaxBudgetUSD))
	}

	// Allowed tools
	if len(cfg.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(cfg.AllowedTools, ","))
	}

	// Session resume
	if useResume && input.Run.SessionState != "" {
		args = append(args, "--resume", input.Run.SessionState)
	}

	// Task prompt as positional argument
	prompt := input.EnvVars["ARI_PROMPT"]
	if prompt == "" {
		prompt = input.Prompt
	}
	if prompt != "" {
		args = append(args, prompt)
	}

	return args
}

// buildEnv merges environment variables with proper precedence:
// base OS env < adapterConfig.env < input.EnvVars (ARI_* vars)
func buildEnv(input adapter.InvokeInput, cfg Config) []string {
	env := os.Environ()

	// Adapter-config env vars (lower precedence)
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	// ARI_* / input env vars (highest precedence)
	for k, v := range input.EnvVars {
		env = append(env, k+"="+v)
	}

	return env
}

// executeOnce runs a single invocation of the Claude CLI and returns the result.
func (c *ClaudeAdapter) executeOnce(ctx context.Context, cfg Config, input adapter.InvokeInput, hooks adapter.Hooks, useResume bool) (adapter.InvokeResult, error) {
	args := buildArgs(cfg, input, useResume)

	// Enforce run timeout
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, cfg.ClaudePath, args...)
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}

	// Inject environment: inherit + adapter-config env + ARI_* overrides
	cmd.Env = buildEnv(input, cfg)

	// Process group for clean kill
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Override the default cancel behavior to kill the entire process group
	// instead of just the direct process. This ensures child processes (e.g.,
	// shell-spawned subprocesses) are also killed on timeout/cancellation.
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
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
		// External cancellation — the cmd.Cancel already sent SIGKILL to process group.
		// For graceful stop, we would prefer SIGTERM first, but since exec.CommandContext
		// already triggered Cancel, the process group is killed.
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

	// Extract usage and session state from collected events
	usage, sessionState := collector.extract()

	return adapter.InvokeResult{
		Status:       status,
		ExitCode:     exitCode,
		Usage:        usage,
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

	// Session resume fallback: if --resume was used and the run failed,
	// retry once without --resume (fresh session)
	if useResume && result.Status == adapter.RunStatusFailed {
		if hooks.OnLogLine != nil {
			hooks.OnLogLine(adapter.LogLine{
				Level:     "warn",
				Message:   fmt.Sprintf("session resume failed (session: %s), retrying without --resume", input.Run.SessionState),
				Timestamp: time.Now(),
			})
		}
		result, err = c.executeOnce(ctx, cfg, input, hooks, false)
	}

	return result, err
}

// truncate returns s truncated to maxBytes.
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}
