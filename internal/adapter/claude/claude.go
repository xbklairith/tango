package claude

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/xb/ari/internal/adapter"
)

// sensitiveEnvPattern matches env var names containing sensitive keywords.
var sensitiveEnvPattern = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET|PASSWORD|PASSWD|AUTHORIZATION|COOKIE)`)

// redactEnvForLog returns a copy of env with sensitive values replaced.
func redactEnvForLog(env map[string]string) map[string]string {
	redacted := make(map[string]string, len(env))
	for k, v := range env {
		if sensitiveEnvPattern.MatchString(k) {
			redacted[k] = "***REDACTED***"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

// renderTemplate replaces {{key}} placeholders with values from vars.
// Missing variables resolve to empty string.
func renderTemplate(tmpl string, vars map[string]string) string {
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	// Remove any remaining unreplaced placeholders
	for {
		start := strings.Index(result, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+2:]
	}
	return result
}

// deniedExtraArgs are CLI flags that extraArgs must not override for security.
var deniedExtraArgs = map[string]bool{
	"--dangerously-skip-permissions": true,
	"--allowedTools":                 true,
	"--append-system-prompt":         true,
	"--append-system-prompt-file":    true,
	"--resume":                       true,
	"--print":                        true,
	"--output-format":                true,
	"--model":                        true,
	"--max-budget-usd":               true,
}

// deniedExtraArgsWithValue are denied flags that also take a following value argument.
var deniedExtraArgsWithValue = map[string]bool{
	"--allowedTools":              true,
	"--append-system-prompt":      true,
	"--append-system-prompt-file": true,
	"--resume":                    true,
	"--output-format":             true,
	"--model":                     true,
	"--max-budget-usd":            true,
}

// filterExtraArgs removes security-sensitive flags from extra args.
func filterExtraArgs(args []string) []string {
	var filtered []string
	skip := false
	for _, arg := range args {
		if skip {
			skip = false
			continue
		}
		if deniedExtraArgs[arg] {
			slog.Warn("claude adapter: blocked security-sensitive extraArg", "arg", arg)
			if deniedExtraArgsWithValue[arg] {
				skip = true // skip the following value too
			}
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

// mergeInstructionsWithPrompt creates a temp file containing the instructions file content
// followed by the system prompt. Returns the temp file path. Caller must defer os.Remove().
func mergeInstructionsWithPrompt(instructionsPath, systemPrompt string) (string, error) {
	content, err := os.ReadFile(instructionsPath)
	if err != nil {
		return "", fmt.Errorf("reading instructions file: %w", err)
	}

	merged := string(content) + "\n\n" + systemPrompt

	tmpFile, err := os.CreateTemp("", "ari-instructions-*.md")
	if err != nil {
		return "", fmt.Errorf("creating temp instructions file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(merged); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("writing merged instructions: %w", err)
	}

	return tmpFile.Name(), nil
}

// removeArg removes a flag and its value from args (e.g., --append-system-prompt "value").
func removeArg(args []string, flag string) []string {
	var result []string
	skip := false
	for _, arg := range args {
		if skip {
			skip = false
			continue
		}
		if arg == flag {
			skip = true // skip the value too
			continue
		}
		result = append(result, arg)
	}
	return result
}

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

	// System prompt injection: --append-system-prompt-file and --append-system-prompt
	// are mutually exclusive. When instructionsFilePath is configured, we merge the
	// system prompt into a temp file and use --append-system-prompt-file only.
	// Otherwise, use --append-system-prompt as before.
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

	// Extra args always last — filtered through denylist to prevent privilege escalation
	if len(cfg.ExtraArgs) > 0 {
		args = append(args, filterExtraArgs(cfg.ExtraArgs)...)
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

	// Resolve working directory for instructions file resolution
	workingDir := input.WorkingDir
	if workingDir == "" {
		workingDir = cfg.WorkingDir
	}

	// Wire skills directory (REQ-003): create temp dir with embedded SKILL.md
	skillsDir, cleanupSkills, skillsErr := buildSkillsDir()
	if skillsErr != nil {
		slog.Warn("failed to create skills dir, skipping --add-dir", "error", skillsErr)
	} else {
		defer cleanupSkills()
		args = append(args, "--add-dir", skillsDir)
	}

	// Wire instructions file (REQ-002, REQ-029): resolve and inject
	if cfg.InstructionsFilePath != "" {
		resolvedPath, err := resolveInstructionsFile(cfg.InstructionsFilePath, workingDir)
		if err != nil {
			slog.Warn("skipping instructions file", "path", cfg.InstructionsFilePath, "error", err)
		} else {
			// When both instructions file and system prompt exist, merge into temp file
			if input.Agent.SystemPrompt != "" {
				tmpFile, err := mergeInstructionsWithPrompt(resolvedPath, input.Agent.SystemPrompt)
				if err != nil {
					slog.Warn("failed to merge instructions with system prompt", "error", err)
				} else {
					defer os.Remove(tmpFile)
					// Remove --append-system-prompt from args (mutual exclusion)
					args = removeArg(args, "--append-system-prompt")
					args = append(args, "--append-system-prompt-file", tmpFile)
				}
			} else {
				args = append(args, "--append-system-prompt-file", resolvedPath)
			}
		}
	}

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
	env := buildEnv(input, cfg)
	cmd.Env = env

	// Log env with redaction (REQ-018, REQ-024)
	if hooks.OnLogLine != nil {
		envMap := make(map[string]string)
		for _, e := range env {
			if k, v, ok := strings.Cut(e, "="); ok {
				envMap[k] = v
			}
		}
		slog.Debug("claude adapter env", "env", redactEnvForLog(envMap))
	}

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

	// Detect max turns exhaustion (REQ-006) — clear session so next run starts fresh
	clearSession := isMaxTurnsResult(collector)
	if clearSession {
		slog.Info("max turns reached, clearing session state")
	}

	return adapter.InvokeResult{
		Status:       status,
		ExitCode:     exitCode,
		Usage:        usage,
		CostUSD:      costUSD,
		SessionState: sessionState,
		Stdout:       stdout,
		Stderr:       stderr,
		ClearSession: clearSession,
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
