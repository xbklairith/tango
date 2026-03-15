# Design: Claude Adapter

**Created:** 2026-03-15
**Status:** Ready for Implementation

---

## Architecture Overview

The Claude adapter (`claude_local`) is a concrete implementation of the `adapter.Adapter` interface that spawns the Claude Code CLI (`claude`) as a subprocess. It follows the same subprocess management pattern as the `process` adapter but adds Claude-specific behavior: model selection, structured JSON output parsing, token/cost extraction, session resume, tool-call log parsing, and max-turns configuration.

### Component Relationships

```
RunService.Invoke()
        |
        v
  AdapterRegistry.Resolve("claude_local")
        |
        v
  ClaudeAdapter.Execute(ctx, input, hooks)
        |
        v
  Build CLI args:
    claude --print --model <model> --output-format json
           --system-prompt <prompt> [--resume <session>]
           [--max-turns N] [--allowedTools tool1,tool2]
           "<ARI_PROMPT>"
        |
        v
  exec.CommandContext(runCtx, claudePath, args...)
    .Dir = workingDir
    .Env = os.Environ() + ARI_* + adapterConfig.env
    .SysProcAttr = {Setpgid: true}
        |
        +---> goroutine: streamStdout → parse lines → hooks.OnLogLine
        +---> goroutine: streamStderr → buffer + rate-limit detection
        |
        v
  cmd.Wait()
        |
        v
  Parse JSON output → TokenUsage + SessionState
  Determine RunStatus from exit code / context
        |
        v
  Return InvokeResult
```

### Relationship to Process Adapter

The Claude adapter reuses several patterns from `internal/adapter/process/process.go`:

| Concern | Process Adapter | Claude Adapter |
|---------|----------------|----------------|
| Subprocess spawn | `exec.CommandContext` | Same |
| Process group kill | `Setpgid: true` + SIGKILL/SIGTERM | Same |
| Env var injection | `envMapToSlice` | Same (+ adapter-config env merge) |
| Output capture | Bounded buffer + `streamLines` | Same (+ JSON parsing layer) |
| Session state | Last JSON line in stdout | Claude's `session_id` from JSON output |
| Timeout | `context.WithTimeout` | Same |
| Token usage | Not extracted | Parsed from `--output-format json` |
| Model selection | N/A | `--model` flag |

---

## Claude Adapter Implementation

**Package:** `internal/adapter/claude/`
**adapterType:** `"claude_local"`

### AdapterConfig Schema (JSON)

```json
{
  "claudePath": "/usr/local/bin/claude",
  "model": "sonnet",
  "maxTurns": 25,
  "allowedTools": ["Read", "Write", "Edit", "Bash", "Grep", "Glob"],
  "workingDir": "/opt/agents/workspace",
  "timeoutSeconds": 3600,
  "maxExcerptBytes": 65536,
  "env": {
    "ANTHROPIC_API_KEY": "sk-ant-...",
    "CUSTOM_VAR": "value"
  },
  "disableResumeOnError": false
}
```

All fields are optional:
- `claudePath` defaults to `"claude"` (found via `$PATH`).
- `model` defaults to `"sonnet"`. Valid values: `"opus"`, `"sonnet"`, `"haiku"`.
- `maxTurns` defaults to 0 (no limit — Claude decides when to stop).
- `allowedTools` defaults to nil (all tools allowed).
- `workingDir` defaults to the Ari process working directory.
- `timeoutSeconds` defaults to 3600 (1 hour).
- `maxExcerptBytes` defaults to 65536 (64 KB per REQ-CLA-026).
- `env` defaults to nil (no extra env vars).
- `disableResumeOnError` defaults to false.

### Config Struct

```go
// internal/adapter/claude/config.go
package claude

// Config is the JSON schema for the claude_local adapter's adapterConfig.
type Config struct {
    ClaudePath          string            `json:"claudePath"`
    Model               string            `json:"model"`
    MaxTurns            int               `json:"maxTurns"`
    AllowedTools        []string          `json:"allowedTools"`
    WorkingDir          string            `json:"workingDir"`
    TimeoutSeconds      int               `json:"timeoutSeconds"`
    MaxExcerptBytes     int               `json:"maxExcerptBytes"`
    Env                 map[string]string `json:"env"`
    DisableResumeOnError bool            `json:"disableResumeOnError"`
}

const (
    DefaultClaudePath      = "claude"
    DefaultModel           = "sonnet"
    DefaultTimeoutSeconds  = 3600
    DefaultMaxExcerptBytes = 65536
)

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
    return cfg
}
```

### Adapter Struct and Interface Implementation

```go
// internal/adapter/claude/claude.go
package claude

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "syscall"
    "time"

    "github.com/xb/ari/internal/adapter"
)

// ClaudeAdapter implements adapter.Adapter for Claude Code CLI execution.
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
```

### Execute Method

```go
func (c *ClaudeAdapter) Execute(ctx context.Context, input adapter.InvokeInput, hooks adapter.Hooks) (adapter.InvokeResult, error) {
    cfg := parseConfig(input.Agent.AdapterConfig)

    // Validate workingDir (REQ-CLA-036)
    if cfg.WorkingDir != "" {
        if !filepath.IsAbs(cfg.WorkingDir) || strings.Contains(cfg.WorkingDir, "..") {
            return adapter.InvokeResult{Status: adapter.RunStatusFailed},
                fmt.Errorf("workingDir must be absolute and contain no '..' segments: %s", cfg.WorkingDir)
        }
    }

    // Build CLI arguments
    args := c.buildArgs(cfg, input)

    // Enforce run timeout (REQ-CLA-016)
    runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
    defer cancel()

    cmd := exec.CommandContext(runCtx, cfg.ClaudePath, args...)
    if cfg.WorkingDir != "" {
        cmd.Dir = cfg.WorkingDir
    }

    // Inject environment: inherit + adapter-config env + ARI_* overrides (REQ-CLA-014, REQ-CLA-015)
    cmd.Env = buildEnv(cfg.Env, input.EnvVars)

    // Process group for clean kill (REQ-CLA-025)
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
        // Handle case where claude binary was removed after TestEnvironment (REQ-CLA-037)
        return adapter.InvokeResult{Status: adapter.RunStatusFailed}, fmt.Errorf("start claude: %w", err)
    }

    // Stream stdout and stderr concurrently (REQ-CLA-022, REQ-CLA-038)
    done := make(chan struct{}, 2)
    go func() {
        streamAndParse(stdoutPipe, &stdoutBuf, cfg.MaxExcerptBytes, hooks)
        done <- struct{}{}
    }()
    go func() {
        streamStderr(stderrPipe, &stderrBuf, cfg.MaxExcerptBytes, hooks)
        done <- struct{}{}
    }()

    // Wait for both readers to finish before cmd.Wait() (REQ-CLA-038)
    <-done
    <-done

    waitErr := cmd.Wait()

    // Determine status (same pattern as process adapter)
    var status adapter.RunStatus
    exitCode := 0
    switch {
    case runCtx.Err() == context.DeadlineExceeded:
        if cmd.Process != nil {
            _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
        }
        status = adapter.RunStatusTimedOut
    case ctx.Err() != nil:
        // External cancellation — graceful stop (REQ-CLA-021)
        if cmd.Process != nil {
            _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
            // Give 5 seconds for graceful shutdown, then SIGKILL
            time.AfterFunc(5*time.Second, func() {
                if cmd.Process != nil {
                    _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
                }
            })
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

    // Parse Claude's JSON output for usage and session state (REQ-CLA-010, REQ-CLA-011)
    usage, sessionState := parseClaudeOutput(stdout)

    return adapter.InvokeResult{
        Status:       status,
        ExitCode:     exitCode,
        Usage:        usage,
        SessionState: sessionState,
        Stdout:       stdout,
        Stderr:       stderr,
    }, nil
}
```

### CLI Argument Building

```go
func (c *ClaudeAdapter) buildArgs(cfg Config, input adapter.InvokeInput) []string {
    args := []string{
        "--print",                   // non-interactive mode
        "--output-format", "json",   // structured output (REQ-CLA-027)
        "--model", cfg.Model,        // model selection (REQ-CLA-004)
    }

    // System prompt (REQ-CLA-002)
    if input.Agent.SystemPrompt != "" {
        args = append(args, "--system-prompt", input.Agent.SystemPrompt)
    }

    // Session resume (REQ-CLA-012)
    if input.Run.SessionState != "" && !cfg.DisableResumeOnError {
        args = append(args, "--resume", input.Run.SessionState)
    }

    // Max turns (REQ-CLA-005)
    if cfg.MaxTurns > 0 {
        args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.MaxTurns))
    }

    // Allowed tools (REQ-CLA-006)
    if len(cfg.AllowedTools) > 0 {
        args = append(args, "--allowedTools", strings.Join(cfg.AllowedTools, ","))
    }

    // Task prompt as positional argument (REQ-CLA-003)
    prompt := input.EnvVars["ARI_PROMPT"]
    if prompt == "" {
        prompt = input.Prompt
    }
    if prompt != "" {
        args = append(args, prompt)
    }

    return args
}
```

### Environment Building

```go
// buildEnv merges environment variables with proper precedence:
// base OS env < adapterConfig.env < ARI_* vars
func buildEnv(adapterEnv, ariEnv map[string]string) []string {
    env := os.Environ()

    // Adapter-config env vars (lower precedence)
    for k, v := range adapterEnv {
        if !strings.HasPrefix(k, "ARI_") { // ARI_* always wins
            env = append(env, k+"="+v)
        }
    }

    // ARI_* vars (highest precedence)
    for k, v := range ariEnv {
        env = append(env, k+"="+v)
    }

    return env
}
```

### Stdout Parsing — Tool Calls and Log Lines

The Claude CLI with `--output-format json` emits a JSON object on completion. During execution, it may emit progress lines to stdout. The adapter parses these in real time for the agent console.

```go
// streamAndParse reads stdout line by line, parses for tool-call patterns,
// and forwards structured log lines via hooks.
func streamAndParse(r io.Reader, buf *bytes.Buffer, maxBytes int, hooks adapter.Hooks) {
    scanner := bufio.NewScanner(r)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

    for scanner.Scan() {
        line := scanner.Text()

        // Accumulate into bounded buffer
        if buf.Len() < maxBytes {
            remaining := maxBytes - buf.Len()
            if len(line)+1 > remaining {
                buf.WriteString(line[:remaining])
            } else {
                buf.WriteString(line)
                buf.WriteByte('\n')
            }
        }

        // Forward to SSE via hooks (REQ-CLA-008)
        if hooks.OnLogLine != nil {
            logLine := parseLogLine(line)
            hooks.OnLogLine(logLine)
        }

        // Detect tool calls for status change (REQ-CLA-023)
        if hooks.OnStatusChange != nil {
            if detail := detectToolCall(line); detail != "" {
                hooks.OnStatusChange(detail)
            }
        }
    }
}

// parseLogLine converts a raw stdout line into a structured LogLine.
// If the line is JSON (tool invocation from --output-format json), extract fields.
func parseLogLine(line string) adapter.LogLine {
    ll := adapter.LogLine{
        Level:     "info",
        Message:   line,
        Timestamp: time.Now(),
    }

    // Attempt JSON parse for structured tool-call output (REQ-CLA-009)
    if strings.HasPrefix(strings.TrimSpace(line), "{") {
        var obj map[string]any
        if err := json.Unmarshal([]byte(line), &obj); err == nil {
            fields := make(map[string]any)
            if toolName, ok := obj["tool_name"].(string); ok {
                fields["toolName"] = toolName
                ll.Message = fmt.Sprintf("Tool: %s", toolName)
            }
            if toolInput, ok := obj["tool_input"]; ok {
                fields["toolInput"] = toolInput
            }
            if toolResult, ok := obj["tool_result"]; ok {
                fields["toolResult"] = toolResult
            }
            if len(fields) > 0 {
                ll.Fields = fields
            }
        }
    }

    return ll
}

// detectToolCall checks if a line indicates a tool invocation and returns
// a human-readable status string, or empty string if not a tool call.
func detectToolCall(line string) string {
    trimmed := strings.TrimSpace(line)
    if strings.HasPrefix(trimmed, "{") {
        var obj map[string]any
        if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
            if toolName, ok := obj["tool_name"].(string); ok {
                return fmt.Sprintf("tool:%s", toolName)
            }
        }
    }
    return ""
}
```

### Stderr Streaming — Rate-Limit Detection

```go
// streamStderr reads stderr, accumulates into buffer, and detects rate-limit errors.
func streamStderr(r io.Reader, buf *bytes.Buffer, maxBytes int, hooks adapter.Hooks) {
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

        // Detect rate limiting (REQ-CLA-020)
        if hooks.OnLogLine != nil {
            lower := strings.ToLower(line)
            if strings.Contains(lower, "rate limit") || strings.Contains(lower, "429") {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "warn",
                    Message:   line,
                    Timestamp: time.Now(),
                    Fields:    map[string]any{"rateLimited": true},
                })
            } else {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "error",
                    Message:   line,
                    Timestamp: time.Now(),
                })
            }
        }
    }
}
```

### Claude JSON Output Parsing

When Claude CLI runs with `--output-format json`, the final output is a JSON object containing usage information and session metadata. The adapter extracts these on completion.

```go
// claudeJSONOutput represents the structured output from `claude --output-format json`.
type claudeJSONOutput struct {
    Result       string `json:"result"`
    SessionID    string `json:"session_id"`
    InputTokens  int    `json:"input_tokens"`
    OutputTokens int    `json:"output_tokens"`
    Model        string `json:"model"`
    CostUSD      float64 `json:"cost_usd"`
}

// parseClaudeOutput extracts token usage and session state from Claude's JSON output.
// Returns zero-valued usage and empty session if parsing fails (REQ-CLA-031).
func parseClaudeOutput(stdout string) (adapter.TokenUsage, string) {
    // Claude's JSON output is typically the last JSON object in stdout
    lines := strings.Split(strings.TrimSpace(stdout), "\n")
    for i := len(lines) - 1; i >= 0; i-- {
        line := strings.TrimSpace(lines[i])
        if !strings.HasPrefix(line, "{") {
            continue
        }
        var output claudeJSONOutput
        if err := json.Unmarshal([]byte(line), &output); err == nil {
            if output.InputTokens > 0 || output.OutputTokens > 0 || output.SessionID != "" {
                return adapter.TokenUsage{
                    InputTokens:  output.InputTokens,
                    OutputTokens: output.OutputTokens,
                    Model:        output.Model,
                    Provider:     "anthropic",
                }, output.SessionID
            }
        }
        // If we found a JSON line but it didn't have our fields, keep searching
    }

    return adapter.TokenUsage{}, ""
}
```

### TestEnvironment

```go
func (c *ClaudeAdapter) TestEnvironment(level adapter.TestLevel) (adapter.TestResult, error) {
    // Basic: check that claude binary exists in PATH (REQ-CLA-028)
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

    // Full: run claude --version and validate (REQ-CLA-029)
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
```

### Helper Functions

```go
func truncate(s string, maxBytes int) string {
    if len(s) <= maxBytes {
        return s
    }
    return s[:maxBytes]
}
```

---

## Registration in Adapter Registry

At server startup, the Claude adapter is registered alongside the process adapter:

```go
// In cmd/ari/run.go or server initialization

import (
    "github.com/xb/ari/internal/adapter"
    "github.com/xb/ari/internal/adapter/claude"
    "github.com/xb/ari/internal/adapter/process"
)

func setupAdapters() *adapter.Registry {
    registry := adapter.NewRegistry()

    // Register built-in adapters
    registry.Register(process.New())
    registry.Register(claude.New())

    // Test environments at startup (REQ-049)
    for _, a := range []adapter.Adapter{process.New(), claude.New()} {
        result, err := a.TestEnvironment(adapter.TestLevelBasic)
        if err != nil || !result.Available {
            msg := "unknown error"
            if result.Message != "" {
                msg = result.Message
            }
            slog.Warn("adapter unavailable", "type", a.Type(), "reason", msg)
            registry.MarkUnavailable(a.Type(), msg)
        }
    }

    return registry
}
```

---

## Integration with Agent Console UI

The Claude adapter's real-time log lines flow through the existing SSE infrastructure:

```
ClaudeAdapter.Execute()
    |
    hooks.OnLogLine(LogLine{
        Level: "info",
        Message: "Tool: Read /src/main.go",
        Fields: {"toolName": "Read", "toolInput": "/src/main.go"},
    })
    |
    v
RunService → SSE Hub
    |
    v
event: heartbeat.run.log
data: {"runId":"...","agentId":"...","level":"info","message":"Tool: Read /src/main.go",
       "fields":{"toolName":"Read","toolInput":"/src/main.go"},"timestamp":"..."}
    |
    v
Agent Console UI → renders tool call card with icon, file path, and expandable output
```

The UI can distinguish tool-call log lines from plain text by checking for `fields.toolName` in the SSE payload. This enables rich rendering:

- **Tool calls**: show tool icon, name, input summary, and collapsible output
- **Thinking**: show a "thinking..." indicator when no tool call is active
- **Rate limits**: show a warning badge when `fields.rateLimited` is true
- **Errors**: show stderr lines in red with error styling

---

## Session File Management

Claude Code stores session data server-side (Anthropic's infrastructure). The adapter only needs to track the opaque `session_id` string:

1. **First run**: No `--resume` flag. Claude starts a fresh session. JSON output includes `session_id`.
2. **Adapter returns**: `InvokeResult.SessionState = session_id`.
3. **RunService persists**: Stores in `agent_task_sessions` or `agent_conversation_sessions` table.
4. **Next run**: `InvokeInput.Run.SessionState` is populated. Adapter passes `--resume <session_id>`.
5. **Resume failure**: If Claude returns an error, adapter retries without `--resume` (REQ-CLA-013).

No local files need to be managed — session state is a string that flows through the database.

---

## Error Handling Matrix

| Scenario | Detection | Status | Action |
|----------|-----------|--------|--------|
| Successful run | exit code 0 | `succeeded` | Extract usage + session |
| Non-zero exit | `exec.ExitError` | `failed` | Capture stderr, create inbox alert |
| Timeout exceeded | `context.DeadlineExceeded` | `timed_out` | SIGKILL process group |
| Graceful stop | `ctx.Err() != nil` | `cancelled` | SIGTERM → 5s → SIGKILL |
| Claude binary missing | `cmd.Start()` fails | `failed` | Return error immediately |
| Rate limit (429) | stderr contains "rate limit" | ongoing | Log warning, continue |
| Invalid session resume | Claude error output | retry | Fall back to fresh session |
| Malformed JSON output | JSON parse error | N/A | Zero usage, empty session (no fail) |
| OOM / signal kill | `exec.ExitError` with signal | `failed` | Capture signal info |

---

## File Structure

```
internal/adapter/claude/
    claude.go          # ClaudeAdapter struct, Execute, Type, Models, TestEnvironment
    config.go          # Config struct, parseConfig, defaults
    parser.go          # parseClaudeOutput, parseLogLine, detectToolCall, streamAndParse, streamStderr
    claude_test.go     # Unit tests
```

---

## Security Considerations

- **No secrets in CLI args**: `ARI_API_KEY` is passed only as an environment variable, never as a command-line argument (REQ-CLA-035). Environment variables are not visible via `ps`.
- **Working directory validation**: `workingDir` must be absolute with no `..` segments (REQ-CLA-036).
- **Anthropic API key**: Managed by Claude CLI's own configuration (`~/.claude/` or `ANTHROPIC_API_KEY` env var). Ari does not manage or inject Anthropic credentials — only Ari's own Run Token.
- **Process isolation**: Each `Execute` call spawns an independent subprocess with its own process group. No shared state between invocations.
