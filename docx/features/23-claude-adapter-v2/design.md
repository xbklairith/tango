# Design: Claude Adapter v2

**Created:** 2026-03-17
**Status:** Draft

## Architecture Overview

This is an enhancement to the existing `claude_local` adapter — not a rewrite. Changes are confined to 5 files: `claude.go` (execution), `config.go` (new config fields), `parser.go` (new detections), `run_handler.go` (session params + workspace cwd), and `adapter.go` (InvokeInput). The adapter interface stays unchanged.

## System Context

- **Depends On:** `adapter.Adapter` interface (unchanged), `adapter.InvokeInput` (new field), `run_handler.go` (session storage, prompt building), project/workspace DB queries
- **Used By:** `RunService.Invoke()`, all agent runs via Claude CLI
- **External Dependencies:** Claude Code CLI (`claude` binary)

**New field on `adapter.InvokeInput`:**

```go
type InvokeInput struct {
    // ... existing fields ...

    // WorkingDir is the resolved working directory for this run.
    // Populated by run_handler; takes priority over cfg.WorkingDir in the adapter.
    WorkingDir string
}
```

The run handler's `resolveWorkingDir()` result is set on `InvokeInput.WorkingDir`. The adapter uses this with higher priority than `cfg.WorkingDir`.

**Note on `SquadContext.Name`:** `buildInvokeInput()` must populate `Squad.Name` from the squad already fetched in the run handler (see `run_handler.go` lines ~483/567 where the squad is queried). No additional DB call is needed — just wire `squad.Name` into the `SquadContext` struct.

## Component Changes

### 1. Config (`config.go`)

**New fields in `Config` struct:**

```go
type Config struct {
    // ... existing fields unchanged ...

    // New fields (all optional with zero-value defaults)
    InstructionsFilePath string   `json:"instructionsFilePath"` // REQ-002, REQ-020
    Effort               string   `json:"effort"`               // REQ-008: low|medium|high
    Chrome               bool     `json:"chrome"`               // REQ-009
    MaxTurnsPerRun       int      `json:"maxTurnsPerRun"`       // REQ-013
    ExtraArgs            []string `json:"extraArgs"`            // REQ-010
    PromptTemplate       string   `json:"promptTemplate"`       // REQ-017
}
```

**Validation:**
- `instructionsFilePath`: must be absolute or relative (resolved against workingDir), no `..` traversal after resolution
- `effort`: must be one of `""`, `"low"`, `"medium"`, `"high"`
- `maxTurnsPerRun`: must be >= 0 (0 = unlimited)

### 2. Execution (`claude.go`)

**Changes to `buildArgs()`:**

```
Before: claude --print --output-format stream-json --model <model> [--append-system-prompt <sp>] [prompt as positional]
After:  claude --print - --output-format stream-json --verbose --model <model> [--effort <e>] [--chrome] [--max-turns <n>] [--append-system-prompt-file <path>] [--add-dir <skills>] [--resume <id>] [extraArgs...]
```

Key changes:
- `--print` → `--print -` (read prompt from stdin)
- Add `--verbose` always (REQ-016)
- Remove positional prompt arg; write to cmd.Stdin instead
- Add `--effort`, `--chrome`, `--max-turns` when configured
- Add system prompt flag with mutual exclusion:
  - When `instructionsFilePath` is set: create a temp file combining the instructions file content + the agent's system prompt, then use `--append-system-prompt-file <tempPath>` only
  - When `instructionsFilePath` is NOT set: use `--append-system-prompt <systemPrompt>` as today
  - The two flags are mutually exclusive — never pass both
- Add `--add-dir <skillsDir>` pointing to Ari's skills
- Append `extraArgs` last

**Changes to `buildEnv()`:**

```go
// Strip Claude Code nesting vars via prefix-based blocklist (REQ-007)
// Instead of hardcoding specific vars, strip ALL vars matching these prefixes.
// This is future-proof against new Claude Code env vars being added.
nestingPrefixes := []string{"CLAUDE_CODE_", "CLAUDECODE"}
for key := range envMap {
    for _, prefix := range nestingPrefixes {
        if strings.HasPrefix(key, prefix) || key == prefix {
            delete(envMap, key)
            break
        }
    }
}
```

**Changes to `executeOnce()`:**

```go
// Write prompt to stdin via pipe + goroutine (not strings.NewReader).
// A pipe ensures the child process sees EOF after the prompt is fully written,
// and the goroutine prevents deadlock if the process exits before reading all input.
stdinPipe, err := cmd.StdinPipe()
if err != nil {
    return nil, fmt.Errorf("stdin pipe: %w", err)
}
// ... start cmd ...
go func() {
    defer stdinPipe.Close()
    if _, err := io.WriteString(stdinPipe, prompt); err != nil {
        // Stdin write failure: close pipe and let process exit naturally.
        // The process will see EOF and terminate; we handle the exit code below.
        log.Warn().Err(err).Msg("failed to write prompt to stdin")
    }
}()
```

**Changes to `Execute()` (retry logic):**

```go
// Smart session error detection (REQ-005)
if useResume && result.Status == adapter.RunStatusFailed {
    if isUnknownSessionError(result.Stderr, result.Stdout) {
        // Retry without resume
        result, err = c.executeOnce(ctx, cfg, input, hooks, false)
    }
}
```

### 3. Parser (`parser.go`)

**New detection functions:**

```go
// isUnknownSessionError checks stderr/stdout for session-related errors (REQ-005)
func isUnknownSessionError(stderr, stdout string) bool {
    pattern := regexp.MustCompile(`(?i)(no conversation found with session id|unknown session|session .* not found)`)
    return pattern.MatchString(stderr) || pattern.MatchString(stdout)
}

// isMaxTurnsResult checks if run ended due to max turns (REQ-006)
func isMaxTurnsResult(result *eventCollector) bool {
    // Check subtype == "error_max_turns" or stop_reason == "max_turns"
}

// detectLoginRequired checks for auth errors (REQ-011)
func detectLoginRequired(stderr, stdout string) (bool, string) {
    // Returns (requiresLogin, loginURL)
    // Pattern: "not logged in|please log in|login required|unauthorized"
}
```

**Enhanced usage extraction:**

```go
// Add CachedInputTokens to adapter.TokenUsage (REQ-012)
type TokenUsage struct {
    InputTokens       int    `json:"inputTokens"`
    CachedInputTokens int    `json:"cachedInputTokens"` // NEW
    OutputTokens      int    `json:"outputTokens"`
    Model             string `json:"model"`
    Provider          string `json:"provider"`
}
```

### 4. Session State (`run_handler.go`)

**Structured session params (REQ-015):**

```go
// Before: session state = opaque string (just sessionId)
// After: session state = JSON {"sessionId": "...", "cwd": "/path/..."}

type SessionParams struct {
    SessionID string `json:"sessionId"`
    Cwd       string `json:"cwd"`
}

// parseSessionParams handles both new JSON format and legacy bare strings.
// Legacy format: the entire string is a bare session ID (no JSON structure).
// New format: JSON object with sessionId and cwd fields.
func parseSessionParams(raw string) SessionParams {
    raw = strings.TrimSpace(raw)
    if raw == "" {
        return SessionParams{}
    }
    var params SessionParams
    if err := json.Unmarshal([]byte(raw), &params); err != nil {
        // Legacy bare string — treat entire value as session ID
        return SessionParams{SessionID: raw}
    }
    return params
}
```

**Cwd validation on resume (REQ-004):**

```go
func (s *RunService) canResumeSession(params SessionParams, currentCwd string) bool {
    if params.SessionID == "" {
        return false
    }
    if params.Cwd == "" {
        return true // no cwd recorded, allow resume
    }
    return filepath.Clean(params.Cwd) == filepath.Clean(currentCwd)
}
```

**Workspace cwd resolution (REQ-014, REQ-019):**

```go
// Priority: task project workspace > agent workingDir > server cwd
func (s *RunService) resolveWorkingDir(ctx context.Context, agent db.Agent, taskID *uuid.UUID, cfg claude.Config) string {
    // 1. Check task's project workspace
    if taskID != nil {
        issue, _ := s.queries.GetIssueByID(ctx, *taskID)
        if issue.ProjectID.Valid {
            project, _ := s.queries.GetProjectByID(ctx, issue.ProjectID.UUID)
            if project.WorkspaceCwd.Valid && project.WorkspaceCwd.String != "" {
                return project.WorkspaceCwd.String
            }
        }
    }
    // 2. Agent config
    if cfg.WorkingDir != "" {
        return cfg.WorkingDir
    }
    // 3. Server cwd (implicit — cmd.Dir stays empty)
    return ""
}
```

### 5. Skills Directory

**Approach:** Use `go:embed` to embed the SKILL.md content at compile time. At runtime, generate a temp directory per run with the embedded content, then clean up after the run completes.

**Embedded source file:**

```
data/skills/
  .claude/
    skills/
      ari/
        SKILL.md          # Ari API skill (heartbeat, endpoints, rules)
```

**Content of `SKILL.md`** contains:
- Authentication instructions (`ARI_API_KEY`, `ARI_API_URL`)
- Heartbeat procedure (check assignments, checkout, do work, update status)
- Key API endpoints table
- Comment formatting rules
- Critical rules (always checkout, never retry 409, etc.)

**Embedding and per-run temp dir:**

```go
//go:embed data/skills/.claude/skills/ari/SKILL.md
var skillContent string

// buildSkillsDir creates a temp directory with the embedded skill content
// for a single run. Caller must defer cleanup.
func buildSkillsDir() (dir string, cleanup func(), err error) {
    tmpDir, err := os.MkdirTemp("", "ari-skills-*")
    if err != nil {
        return "", nil, err
    }
    skillDir := filepath.Join(tmpDir, ".claude", "skills", "ari")
    os.MkdirAll(skillDir, 0o755)
    os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644)
    return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
}
```

**Injection:**

```go
// In buildArgs(), after other flags:
if skillsDir != "" {
    args = append(args, "--add-dir", skillsDir)
}
// In executeOnce(), manage lifecycle:
skillsDir, cleanupSkills, err := buildSkillsDir()
if err != nil {
    log.Warn().Err(err).Msg("failed to create skills dir, skipping --add-dir")
} else {
    defer cleanupSkills()
}
```

### 6. Prompt Template (`config.go` + `run_handler.go`)

**Default template:**
```
You are {{agent.name}}, a {{agent.role}} in squad {{squad.name}}.
```

**Rendering:** Simple `{{key}}` replacement, flat namespace. Available variables:
- `agent.name`, `agent.role`, `agent.id`, `agent.shortName`
- `squad.name`, `squad.id`
- `run.id`, `run.wakeReason`

**Resolution rules:**
- Missing variables resolve to **empty string** (no error, no placeholder left behind)
- No escaping is performed on variable values — they are inserted verbatim

The rendered template replaces the hardcoded `fmt.Sprintf` in `buildInvokeInput()`. Task-specific context (issue title, API examples) still appended after the template.

### 7. Env Redaction (`claude.go`)

```go
var sensitiveEnvPattern = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET|PASSWORD|PASSWD|AUTHORIZATION|COOKIE)`)

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
```

## Data Flow

```
RunService.Invoke()
     │
     ├── resolveWorkingDir(task project → agent config → server cwd)
     ├── loadSessionParams(JSON with sessionId + cwd)
     ├── canResumeSession(check cwd match)
     │
     ▼
ClaudeAdapter.Execute()
     │
     ├── parseConfig (new fields: effort, chrome, maxTurns, extraArgs, instructionsFilePath)
     ├── resolveInstructionsFile (absolute or relative to workingDir)
     ├── buildSkillsDir (ensure data/skills/.claude/skills/ari/SKILL.md exists)
     ├── buildArgs (stdin, verbose, effort, chrome, maxTurns, instructions file, skills dir, extraArgs)
     ├── buildEnv (strip nesting vars, redact for logs)
     ├── executeOnce (prompt via cmd.Stdin)
     │     │
     │     ├── streamAndParseEvents (extract cachedInputTokens, detect maxTurns)
     │     └── return InvokeResult
     │
     ├── IF failed + unknown session error → retry without resume
     ├── IF maxTurns → set clearSession flag
     ├── IF login required → set loginRequired error
     │
     └── return InvokeResult { SessionState: JSON{sessionId, cwd}, ClearSession, LoginRequired }
```

## Error Handling

| Error | Detection | Response |
|-------|-----------|----------|
| Stdin write failure | `io.WriteString` returns error | Close pipe and let process exit naturally; process sees EOF and terminates, exit code handled normally |
| Unknown session | Regex on stderr/stdout | Retry without `--resume` (REQ-005) |
| Max turns exhausted | `subtype=error_max_turns` or `stop_reason=max_turns` | Clear session, return succeeded (REQ-006) |
| Login required | Regex on stderr/stdout + URL extraction | Return structured error with loginUrl (REQ-011) |
| Missing instructions file | `os.Stat` check | Skip flag, log warning (REQ-026) |
| Missing skills dir | `os.Stat` check | Skip `--add-dir`, log warning (REQ-025) |
| Invalid cwd for resume | Cwd mismatch | Skip `--resume`, log warning (REQ-004) |

**Note on retry behavior change:** Task 5 (unknown session detection) replaces the current "retry on any failure" logic with targeted "retry on unknown session error only". All other failures are returned immediately without retry.

## Security Considerations

- **Env blocklist preserved:** PATH, HOME, SHELL, USER, LD_PRELOAD, LD_LIBRARY_PATH, ARI_*, DYLD_* (REQ-022)
- **Nesting prevention:** Strip all `CLAUDE_CODE_*` and `CLAUDECODE` env vars via prefix matching (REQ-007)
- **Instructions file validation:** No `..` traversal, must resolve to existing file (REQ-023)
- **Log redaction:** Sensitive env var values masked in debug logs (REQ-024)
- **Claude path validation:** Existing check preserved (base name must start with "claude")

## Testing Strategy

### Unit Tests

- `TestBuildArgs_StdinMode` — verify `--print -` and no positional prompt
- `TestBuildArgs_Verbose` — verify `--verbose` always present
- `TestBuildArgs_Effort` — verify `--effort` flag for each valid value
- `TestBuildArgs_Chrome` — verify `--chrome` flag
- `TestBuildArgs_MaxTurns` — verify `--max-turns N`
- `TestBuildArgs_ExtraArgs` — verify extra args appended last
- `TestBuildArgs_InstructionsFile` — verify `--append-system-prompt-file`
- `TestBuildArgs_SkillsDir` — verify `--add-dir`
- `TestBuildEnv_StripNestingVars` — verify CLAUDECODE etc. removed
- `TestBuildEnv_RedactSensitive` — verify KEY/TOKEN/SECRET masked
- `TestIsUnknownSessionError` — patterns matched/not matched
- `TestIsMaxTurnsResult` — subtype and stop_reason detection
- `TestDetectLoginRequired` — login patterns and URL extraction
- `TestCanResumeSession_CwdMatch` — same cwd allows resume
- `TestCanResumeSession_CwdMismatch` — different cwd blocks resume
- `TestCanResumeSession_NoCwd` — empty cwd allows resume
- `TestSessionParams_MarshalUnmarshal` — JSON round-trip
- `TestResolveInstructionsFile_Absolute` — absolute path used as-is
- `TestResolveInstructionsFile_Relative` — resolved against workingDir
- `TestResolveInstructionsFile_Traversal` — `..` rejected
- `TestPromptTemplate_Render` — variable substitution
- `TestSessionParams_LegacyBareString` — bare string (non-JSON) parsed as session ID
- `TestPromptTemplate_MissingVar` — missing variable resolves to empty string
- `TestBuildEnv_BlocklistPreserved` — regression: env blocklist (PATH, HOME, etc.) still enforced after prefix-based nesting strip

### Integration Tests

- `TestExecute_StdinDelivery` — full execution with stdin prompt
- `TestExecute_SessionResumeWithCwd` — resume succeeds when cwd matches
- `TestExecute_SessionResumeCwdMismatch` — skips resume, starts fresh
- `TestExecute_MaxTurnsCleared` — session cleared after max turns

## Files to Modify

| File | Changes |
|------|---------|
| `internal/adapter/claude/config.go` | Add new Config fields, validation for effort/instructions |
| `internal/adapter/claude/claude.go` | Stdin delivery, buildArgs changes, buildEnv nesting strip, env redaction |
| `internal/adapter/claude/parser.go` | isUnknownSessionError, isMaxTurnsResult, detectLoginRequired, cachedInputTokens |
| `internal/adapter/adapter.go` | Add CachedInputTokens to TokenUsage, add ClearSession/LoginRequired to InvokeResult, add WorkingDir to InvokeInput |
| `internal/server/handlers/run_handler.go` | SessionParams struct, canResumeSession, resolveWorkingDir, prompt template rendering |
| `internal/adapter/claude/config_test.go` | Tests for new config fields |
| `internal/adapter/claude/claude_test.go` | Tests for stdin, nesting, args |
| `internal/adapter/claude/parser_test.go` | Tests for new detection functions |
| `data/skills/.claude/skills/ari/SKILL.md` | New file: Ari API skill for agents |

## Implementation Order

1. **Config changes** — add new fields to Config struct (no behavior change yet)
2. **Stdin delivery** — switch from positional arg to stdin (REQ-001)
3. **Nesting prevention** — strip env vars in buildEnv (REQ-007)
4. **Session params** — structured JSON with cwd, validation on resume (REQ-004, REQ-015)
5. **Unknown session detection** — smart retry in parser (REQ-005)
6. **Max turns** — detection and session clearing (REQ-006, REQ-013)
7. **CLI flags** — effort, chrome, maxTurns, verbose, extraArgs (REQ-008-010, REQ-016)
8. **Instructions file** — validation and injection (REQ-002, REQ-020)
9. **Skills directory** — create Ari skill, inject via --add-dir (REQ-003)
10. **Login detection** — pattern matching and error surfacing (REQ-011)
11. **Cached tokens** — extract from stream-json (REQ-012)
12. **Workspace cwd** — resolve from project (REQ-014, REQ-019)
13. **Prompt template** — configurable per agent (REQ-017)
14. **Env redaction** — mask sensitive values in logs (REQ-018)

## Alternatives Considered

### Alternative: Full Rewrite Matching Paperclip Architecture

**Description:** Port Paperclip's entire adapter including template engine, workspace manager, and skill build system.

**Pros:** Feature parity, cleaner codebase
**Cons:** Massive scope, breaks existing tests, loses Ari-specific features

**Rejected Because:** Incremental enhancement preserves stability while adding the most impactful features. Can always do a full rewrite later if needed.

### Alternative: Use --append-system-prompt (inline) Instead of File

**Description:** Keep using inline system prompt instead of file-based injection.

**Pros:** Simpler, no file management
**Cons:** Can't support rich markdown instructions, limited by arg length, can't version instruction files

**Rejected Because:** Per-agent instruction files are one of the highest-value features from Paperclip. File-based approach enables agents to have detailed, versioned specialization documents.
