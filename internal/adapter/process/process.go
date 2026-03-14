// Package process implements the "process" adapter which spawns shell commands as subprocesses.
package process

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/xb/ari/internal/adapter"
)

// DefaultTimeoutSeconds is the default max runtime for an agent process.
const DefaultTimeoutSeconds = 3600

// DefaultMaxExcerptBytes is the default max bytes captured from stdout/stderr.
const DefaultMaxExcerptBytes = 65536

// Config is the JSON schema for the process adapter's adapterConfig.
type Config struct {
	Command         string   `json:"command"`
	Args            []string `json:"args"`
	WorkingDir      string   `json:"workingDir"`
	TimeoutSeconds  int      `json:"timeoutSeconds"`
	MaxExcerptBytes int      `json:"maxExcerptBytes"`
}

// ProcessAdapter implements adapter.Adapter for local subprocess execution.
type ProcessAdapter struct{}

// New creates a new ProcessAdapter.
func New() *ProcessAdapter { return &ProcessAdapter{} }

// Type returns the adapter type identifier.
func (p *ProcessAdapter) Type() string { return "process" }

// Execute spawns a subprocess, injects ARI_* env vars, captures output, and blocks until completion.
func (p *ProcessAdapter) Execute(ctx context.Context, input adapter.InvokeInput, hooks adapter.Hooks) (adapter.InvokeResult, error) {
	cfg := parseConfig(input.Agent.AdapterConfig, input.Agent.ShortName)

	// Enforce run timeout
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, cfg.Command, cfg.Args...)
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}

	// Inject environment: inherit current env + ARI_* overrides
	cmd.Env = append(os.Environ(), envMapToSlice(input.EnvVars)...)

	// Process group: allows killing all children on cancel
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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
		return adapter.InvokeResult{Status: adapter.RunStatusFailed}, fmt.Errorf("start: %w", err)
	}

	// Stream stdout for real-time SSE log lines
	done := make(chan struct{}, 2)
	go func() {
		streamLines(stdoutPipe, &stdoutBuf, cfg.MaxExcerptBytes, hooks.OnLogLine, "info")
		done <- struct{}{}
	}()
	go func() {
		streamLines(stderrPipe, &stderrBuf, cfg.MaxExcerptBytes, nil, "error")
		done <- struct{}{}
	}()

	// Wait for both readers to finish before calling cmd.Wait()
	<-done
	<-done

	waitErr := cmd.Wait()

	// Determine status
	var status adapter.RunStatus
	exitCode := 0
	switch {
	case runCtx.Err() == context.DeadlineExceeded:
		// Kill the process group so no orphaned children remain
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		status = adapter.RunStatusTimedOut
	case ctx.Err() != nil:
		// External cancellation (graceful stop, REQ-011)
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
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

	// The agent writes its session state to stdout as the last JSON line:
	// {"ari_session_state": "<opaque blob>"}
	sessionState := extractSessionState(stdout)

	return adapter.InvokeResult{
		Status:       status,
		ExitCode:     exitCode,
		Stdout:       stdout,
		Stderr:       stderr,
		SessionState: sessionState,
	}, nil
}

// TestEnvironment checks runtime prerequisites.
func (p *ProcessAdapter) TestEnvironment(_ adapter.TestLevel) (adapter.TestResult, error) {
	return adapter.TestResult{Available: true, Message: "process adapter always available"}, nil
}

// Models returns nil since the process adapter is model-agnostic.
func (p *ProcessAdapter) Models() []adapter.ModelDefinition { return nil }

// parseConfig extracts Config from adapterConfig JSON, applying defaults.
func parseConfig(raw json.RawMessage, fallbackCommand string) Config {
	var cfg Config
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	if cfg.Command == "" {
		cfg.Command = fallbackCommand
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if cfg.MaxExcerptBytes == 0 {
		cfg.MaxExcerptBytes = DefaultMaxExcerptBytes
	}
	return cfg
}

// envMapToSlice converts a map of env vars to KEY=VALUE format.
func envMapToSlice(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+"="+v)
	}
	return result
}

// streamLines reads lines from r, writes to buf (up to maxBytes), and calls onLine for each line.
func streamLines(r io.Reader, buf *bytes.Buffer, maxBytes int, onLine func(adapter.LogLine), level string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if buf.Len() < maxBytes {
			remaining := maxBytes - buf.Len()
			if len(line)+1 > remaining {
				buf.WriteString(line[:remaining])
			} else {
				buf.WriteString(line)
				buf.WriteByte('\n')
			}
		}
		if onLine != nil {
			onLine(adapter.LogLine{
				Level:     level,
				Message:   line,
				Timestamp: time.Now(),
			})
		}
	}
}

// truncate returns s truncated to maxBytes.
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}

// extractSessionState looks for the last JSON line with "ari_session_state" key.
func extractSessionState(stdout string) string {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var obj map[string]string
		if err := json.Unmarshal([]byte(line), &obj); err == nil {
			if state, ok := obj["ari_session_state"]; ok {
				return state
			}
		}
		break
	}
	return ""
}
