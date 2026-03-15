# Design: Claude Adapter

**Created:** 2026-03-15
**Status:** Ready for Implementation

---

## Architecture Overview

The Claude adapter (`claude_local`) is a concrete implementation of the `adapter.Adapter` interface that spawns the Claude Code CLI (`claude`) as a subprocess. It follows the same subprocess management pattern as the `process` adapter but adds Claude-specific behavior: model selection, stream-json output parsing (one JSON event per line), token/cost extraction, session resume, tool-call content block parsing, permission skipping for automated agents, session persistence control, and budget-based cost limits.

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
    claude --print --model <model> --output-format stream-json
           --append-system-prompt <prompt>
           --dangerously-skip-permissions
           --no-session-persistence
           [--resume <session>]
           [--max-budget-usd N.NN]
           [--allowedTools tool1,tool2]
           "<ARI_PROMPT>"
        |
        v
  exec.CommandContext(runCtx, claudePath, args...)
    .Dir = workingDir
    .Env = os.Environ() + ARI_* + adapterConfig.env
    .SysProcAttr = {Setpgid: true}
        |
        +---> goroutine: streamStdout → parse stream-json events → hooks.OnLogLine
        +---> goroutine: streamStderr → buffer + rate-limit detection (fallback)
        |
        v
  cmd.Wait()
        |
        v
  Extract from accumulated events:
    - session_id from system/init event
    - total_cost_usd + usage from result event
    - tool calls from assistant events (already streamed via hooks)
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
| Output capture | Bounded buffer + `streamLines` | Same (+ stream-json event parsing) |
| Session state | Last JSON line in stdout | `session_id` from `system/init` event |
| Timeout | `context.WithTimeout` | Same |
| Token usage | Not extracted | Parsed from `result` event |
| Cost tracking | N/A | `total_cost_usd` + `modelUsage` from `result` event |
| Model selection | N/A | `--model` flag |
| Permissions | N/A | `--dangerously-skip-permissions` (headless mode) |
| Session persistence | N/A | `--no-session-persistence` (Ari manages sessions) |

---

## Stream-JSON Event Format

The Claude CLI with `--output-format stream-json` emits one JSON event per stdout line. Each event has a `type` discriminator field. The key event types are:

### 1. System Init Event
Emitted at the start of a session.
```json
{"type":"system","subtype":"init","session_id":"abc-123","model":"claude-sonnet-4-6"}
```
- **Used for:** Capturing `session_id` for session continuity (REQ-CLA-011).

### 2. Assistant Event
Emitted when the assistant produces output. Contains content blocks which may include text or tool use.
```json
{
  "type":"assistant",
  "message":{
    "content":[
      {"type":"text","text":"Let me read the file."},
      {"type":"tool_use","id":"toolu_01ABC","name":"Read","input":{"file_path":"/src/main.go"}}
    ],
    "usage":{"input_tokens":150,"output_tokens":42}
  }
}
```
- **Used for:** Extracting tool calls from `content` blocks where `type == "tool_use"` (REQ-CLA-009). The `name` field is the tool name and `input` contains tool parameters.

### 3. Tool Result Event
Emitted after a tool execution completes.
```json
{"type":"tool_result","tool_use_id":"toolu_01ABC","content":"file contents here..."}
```
- **Used for:** Informational logging only. Tool results may be large.

### 4. Result Event (Final Summary)
Emitted once at the end of a run with cost and usage summary.
```json
{
  "type":"result",
  "subtype":"success",
  "session_id":"abc-123",
  "total_cost_usd":0.086,
  "usage":{"input_tokens":5000,"output_tokens":1200},
  "modelUsage":{
    "claude-sonnet-4-6":{"inputTokens":5000,"outputTokens":1200,"costUSD":0.086}
  }
}
```
- **Used for:** Extracting `total_cost_usd`, token usage breakdown, and per-model cost (REQ-CLA-010). Also provides a final `session_id` confirmation.

### 5. Rate Limit Event
Emitted when rate limiting is encountered.
```json
{"type":"rate_limit_event","rate_limit_info":{"retryAfterMs":5000}}
```
- **Used for:** Rate-limit detection and UI warning (REQ-CLA-020).

---

## Claude Adapter Implementation

**Package:** `internal/adapter/claude/`
**adapterType:** `"claude_local"`

### AdapterConfig Schema (JSON)

```json
{
  "claudePath": "/usr/local/bin/claude",
  "model": "sonnet",
  "allowedTools": ["Read", "Write", "Edit", "Bash", "Grep", "Glob"],
  "workingDir": "/opt/agents/workspace",
  "timeoutSeconds": 3600,
  "maxExcerptBytes": 65536,
  "maxBudgetUSD": 5.00,
  "skipPermissions": true,
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
- `allowedTools` defaults to nil (all tools allowed).
- `workingDir` defaults to the Ari process working directory.
- `timeoutSeconds` defaults to 3600 (1 hour).
- `maxExcerptBytes` defaults to 65536 (64 KB per REQ-CLA-026).
- `maxBudgetUSD` defaults to 0 (no budget limit — agent-level budget enforcement still applies via Ari's cost system).
- `skipPermissions` defaults to `true`. When true, passes `--dangerously-skip-permissions` for headless execution.
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
    AllowedTools        []string          `json:"allowedTools"`
    WorkingDir          string            `json:"workingDir"`
    TimeoutSeconds      int               `json:"timeoutSeconds"`
    MaxExcerptBytes     int               `json:"maxExcerptBytes"`
    MaxBudgetUSD        float64           `json:"maxBudgetUSD"`
    SkipPermissions     *bool             `json:"skipPermissions"` // pointer to distinguish unset from false
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
    // SkipPermissions defaults to true (headless agents need this)
    if cfg.SkipPermissions == nil {
        t := true
        cfg.SkipPermissions = &t
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
    // eventCollector accumulates key events (system/init, result) for post-run extraction
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

    // Extract usage and session state from collected events (REQ-CLA-010, REQ-CLA-011)
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
```

### CLI Argument Building

```go
func (c *ClaudeAdapter) buildArgs(cfg Config, input adapter.InvokeInput) []string {
    args := []string{
        "--print",                          // non-interactive mode
        "--output-format", "stream-json",   // structured streaming output (REQ-CLA-027)
        "--model", cfg.Model,               // model selection (REQ-CLA-004)
    }

    // Append system prompt — preserves Claude Code's built-in prompt (REQ-CLA-002)
    if input.Agent.SystemPrompt != "" {
        args = append(args, "--append-system-prompt", input.Agent.SystemPrompt)
    }

    // Skip permissions for headless execution (REQ-CLA-039)
    if cfg.SkipPermissions != nil && *cfg.SkipPermissions {
        args = append(args, "--dangerously-skip-permissions")
    }

    // Disable local session persistence — Ari manages sessions (REQ-CLA-040)
    args = append(args, "--no-session-persistence")

    // Session resume (REQ-CLA-012)
    if input.Run.SessionState != "" && !cfg.DisableResumeOnError {
        args = append(args, "--resume", input.Run.SessionState)
    }

    // Budget-based cost limit (REQ-CLA-041)
    if cfg.MaxBudgetUSD > 0 {
        args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", cfg.MaxBudgetUSD))
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

### Stream-JSON Event Types and Parsing

The Claude CLI with `--output-format stream-json` emits one JSON event per stdout line. Each event has a `type` field used as a discriminator. The adapter parses these in real time for the agent console and collects key events for post-run extraction.

```go
// claudeEvent represents a single stream-json event from the Claude CLI.
// The Type field is the discriminator; other fields are populated based on the event type.
type claudeEvent struct {
    Type    string `json:"type"`    // "system", "assistant", "tool_result", "result", "rate_limit_event"
    Subtype string `json:"subtype"` // e.g., "init" for system, "success"/"error" for result

    // system/init fields
    SessionID string `json:"session_id"`
    Model     string `json:"model"`

    // assistant fields
    Message *claudeMessage `json:"message"`

    // tool_result fields
    ToolUseID string `json:"tool_use_id"`
    Content   string `json:"content"`

    // result fields
    TotalCostUSD float64                    `json:"total_cost_usd"`
    Usage        *claudeUsage               `json:"usage"`
    ModelUsage   map[string]*claudeModelUse `json:"modelUsage"`

    // rate_limit_event fields
    RateLimitInfo *claudeRateLimitInfo `json:"rate_limit_info"`
}

type claudeMessage struct {
    Content []claudeContentBlock `json:"content"`
    Usage   *claudeUsage         `json:"usage"`
}

type claudeContentBlock struct {
    Type  string         `json:"type"`  // "text" or "tool_use"
    Text  string         `json:"text"`  // for type == "text"
    ID    string         `json:"id"`    // for type == "tool_use"
    Name  string         `json:"name"`  // for type == "tool_use"
    Input map[string]any `json:"input"` // for type == "tool_use"
}

type claudeUsage struct {
    InputTokens  int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
}

type claudeModelUse struct {
    InputTokens  int     `json:"inputTokens"`
    OutputTokens int     `json:"outputTokens"`
    CostUSD      float64 `json:"costUSD"`
}

type claudeRateLimitInfo struct {
    RetryAfterMs int `json:"retryAfterMs"`
}

// eventCollector accumulates key events during streaming for post-run extraction.
type eventCollector struct {
    sessionID    string
    resultEvent  *claudeEvent
}

// extract returns token usage and session state from collected events.
func (ec *eventCollector) extract() (adapter.TokenUsage, string) {
    usage := adapter.TokenUsage{Provider: "anthropic"}

    if ec.resultEvent != nil {
        if ec.resultEvent.Usage != nil {
            usage.InputTokens = ec.resultEvent.Usage.InputTokens
            usage.OutputTokens = ec.resultEvent.Usage.OutputTokens
        }
        usage.Model = ec.resultEvent.Model
        // Session ID from result event takes precedence (final confirmation)
        if ec.resultEvent.SessionID != "" {
            ec.sessionID = ec.resultEvent.SessionID
        }
    }

    return usage, ec.sessionID
}
```

### Stdout Parsing — Stream-JSON Events

```go
// streamAndParseEvents reads stdout line by line, parsing each as a stream-json event,
// forwarding structured log lines via hooks, and collecting key events.
func streamAndParseEvents(r io.Reader, buf *bytes.Buffer, maxBytes int, hooks adapter.Hooks, collector *eventCollector) {
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

        // Parse the stream-json event
        trimmed := strings.TrimSpace(line)
        if !strings.HasPrefix(trimmed, "{") {
            // Not a JSON line — forward as plain text
            if hooks.OnLogLine != nil {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "info",
                    Message:   line,
                    Timestamp: time.Now(),
                })
            }
            continue
        }

        var event claudeEvent
        if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
            // Malformed JSON — skip event, log warning (REQ-CLA-031)
            if hooks.OnLogLine != nil {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "warn",
                    Message:   fmt.Sprintf("failed to parse stream-json event: %s", line),
                    Timestamp: time.Now(),
                })
            }
            continue
        }

        // Process event by type
        switch event.Type {
        case "system":
            if event.Subtype == "init" && event.SessionID != "" {
                collector.sessionID = event.SessionID
            }
            if hooks.OnLogLine != nil {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "info",
                    Message:   fmt.Sprintf("session initialized (model: %s)", event.Model),
                    Timestamp: time.Now(),
                    Fields:    map[string]any{"eventType": "system", "sessionId": event.SessionID},
                })
            }

        case "assistant":
            if event.Message != nil {
                for _, block := range event.Message.Content {
                    switch block.Type {
                    case "tool_use":
                        // Extract tool call into structured fields (REQ-CLA-009)
                        fields := map[string]any{
                            "eventType": "assistant",
                            "toolName":  block.Name,
                            "toolInput": block.Input,
                        }
                        if hooks.OnLogLine != nil {
                            hooks.OnLogLine(adapter.LogLine{
                                Level:     "info",
                                Message:   fmt.Sprintf("Tool: %s", block.Name),
                                Timestamp: time.Now(),
                                Fields:    fields,
                            })
                        }
                        // Update status for UI (REQ-CLA-023)
                        if hooks.OnStatusChange != nil {
                            detail := fmt.Sprintf("tool:%s", block.Name)
                            // Try to add a useful summary from input
                            if filePath, ok := block.Input["file_path"].(string); ok {
                                detail = fmt.Sprintf("tool:%s %s", block.Name, filePath)
                            } else if cmd, ok := block.Input["command"].(string); ok {
                                if len(cmd) > 50 {
                                    cmd = cmd[:50] + "..."
                                }
                                detail = fmt.Sprintf("tool:%s %s", block.Name, cmd)
                            }
                            hooks.OnStatusChange(detail)
                        }
                    case "text":
                        if hooks.OnLogLine != nil {
                            hooks.OnLogLine(adapter.LogLine{
                                Level:     "info",
                                Message:   block.Text,
                                Timestamp: time.Now(),
                                Fields:    map[string]any{"eventType": "assistant"},
                            })
                        }
                    }
                }
            }

        case "tool_result":
            if hooks.OnLogLine != nil {
                // Truncate large tool results for the log line
                content := event.Content
                if len(content) > 200 {
                    content = content[:200] + "...(truncated)"
                }
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "debug",
                    Message:   fmt.Sprintf("Tool result [%s]: %s", event.ToolUseID, content),
                    Timestamp: time.Now(),
                    Fields:    map[string]any{"eventType": "tool_result", "toolUseId": event.ToolUseID},
                })
            }

        case "result":
            collector.resultEvent = &event
            if hooks.OnLogLine != nil {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "info",
                    Message:   fmt.Sprintf("Run complete (cost: $%.4f)", event.TotalCostUSD),
                    Timestamp: time.Now(),
                    Fields: map[string]any{
                        "eventType":    "result",
                        "totalCostUSD": event.TotalCostUSD,
                    },
                })
            }

        case "rate_limit_event":
            // Primary rate-limit detection via stream-json (REQ-CLA-020)
            if hooks.OnLogLine != nil {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "warn",
                    Message:   "Rate limit encountered",
                    Timestamp: time.Now(),
                    Fields:    map[string]any{"rateLimited": true, "eventType": "rate_limit_event"},
                })
            }

        default:
            // Unknown event type — forward as-is
            if hooks.OnLogLine != nil {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "debug",
                    Message:   line,
                    Timestamp: time.Now(),
                    Fields:    map[string]any{"eventType": event.Type},
                })
            }
        }
    }
}
```

### Stderr Streaming — Rate-Limit Detection (Fallback)

```go
// streamStderr reads stderr, accumulates into buffer, and detects rate-limit errors.
// This is a fallback — primary rate-limit detection is via stream-json rate_limit_event.
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

        // Detect rate limiting in stderr as fallback (REQ-CLA-020)
        if hooks.OnLogLine != nil {
            lower := strings.ToLower(line)
            if strings.Contains(lower, "rate limit") || strings.Contains(lower, "429") {
                hooks.OnLogLine(adapter.LogLine{
                    Level:     "warn",
                    Message:   line,
                    Timestamp: time.Now(),
                    Fields:    map[string]any{"rateLimited": true, "source": "stderr"},
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
        Message: "Tool: Read",
        Fields: {"toolName": "Read", "toolInput": {"file_path": "/src/main.go"}, "eventType": "assistant"},
    })
    |
    v
RunService → SSE Hub
    |
    v
event: heartbeat.run.log
data: {"runId":"...","agentId":"...","level":"info","message":"Tool: Read",
       "fields":{"toolName":"Read","toolInput":{"file_path":"/src/main.go"},"eventType":"assistant"},"timestamp":"..."}
    |
    v
Agent Console UI → renders tool call card with icon, file path, and expandable output
```

**NOTE:** The current `OnLogLine` hook in `run_handler.go` does NOT forward `line.Fields` to the SSE payload. This must be fixed (see Prerequisites in requirements.md) to add `fields` to the publish payload when `line.Fields != nil`.

The UI can distinguish event types by checking `fields.eventType` and tool-call log lines by checking for `fields.toolName` in the SSE payload. This enables rich rendering:

- **Tool calls**: show tool icon, name, input summary, and collapsible output
- **Thinking**: show a "thinking..." indicator when no tool call is active
- **Rate limits**: show a warning badge when `fields.rateLimited` is true
- **Errors**: show stderr lines in red with error styling
- **Cost**: show running cost from `result` events

---

## Session File Management

Claude Code stores session data server-side (Anthropic's infrastructure). The adapter only needs to track the opaque `session_id` string. The `--no-session-persistence` flag prevents Claude from also saving sessions locally (Ari manages sessions via its database).

1. **First run**: No `--resume` flag. Claude starts a fresh session. The `system/init` event includes `session_id`.
2. **Adapter captures**: `session_id` from the `system/init` event (and confirmed in `result` event).
3. **Adapter returns**: `InvokeResult.SessionState = session_id`.
4. **RunService persists**: Stores in `agent_task_sessions` or `agent_conversation_sessions` table.
5. **Next run**: `InvokeInput.Run.SessionState` is populated. Adapter passes `--resume <session_id>`.
6. **Resume failure**: If Claude returns an error, adapter falls back to fresh session (REQ-CLA-013). Run service handles higher-level retries.

No local files need to be managed — session state is a string that flows through the database.

---

## Error Handling Matrix

| Scenario | Detection | Status | Action |
|----------|-----------|--------|--------|
| Successful run | exit code 0 | `succeeded` | Extract usage + session from collected events |
| Non-zero exit | `exec.ExitError` | `failed` | Capture stderr, create inbox alert |
| Timeout exceeded | `context.DeadlineExceeded` | `timed_out` | SIGKILL process group |
| Graceful stop | `ctx.Err() != nil` | `cancelled` | SIGTERM then 5s then SIGKILL |
| Claude binary missing | `cmd.Start()` fails | `failed` | Return error immediately |
| Rate limit (stream-json) | `rate_limit_event` in stdout | ongoing | Log warning with `rateLimited` field, continue |
| Rate limit (stderr fallback) | stderr contains "rate limit" or "429" | ongoing | Log warning, continue |
| Invalid session resume | Claude error output | retry | Fall back to fresh session (one attempt) |
| Malformed event line | JSON parse error | N/A | Skip event, log warning, continue (REQ-CLA-031) |
| OOM / signal kill | `exec.ExitError` with signal | `failed` | Capture signal info |

---

## File Structure

```
internal/adapter/claude/
    claude.go          # ClaudeAdapter struct, Execute, Type, Models, TestEnvironment
    config.go          # Config struct, parseConfig, defaults
    parser.go          # claudeEvent types, eventCollector, streamAndParseEvents, streamStderr
    claude_test.go     # Unit tests
```

---

## Security Considerations

- **No secrets in CLI args**: `ARI_API_KEY` is passed only as an environment variable, never as a command-line argument (REQ-CLA-035). Environment variables are not visible via `ps`.
- **Working directory validation**: `workingDir` must be absolute with no `..` segments (REQ-CLA-036).
- **Anthropic API key**: Managed by Claude CLI's own configuration (`~/.claude/` or `ANTHROPIC_API_KEY` env var). Ari does not manage or inject Anthropic credentials — only Ari's own Run Token.
- **Process isolation**: Each `Execute` call spawns an independent subprocess with its own process group. No shared state between invocations.
- **Permission skipping**: `--dangerously-skip-permissions` is required for headless agent execution (no TTY for approval). This is the intended usage pattern for automated agents.
