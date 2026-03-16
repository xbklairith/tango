# Tasks: Claude Adapter v2

**Created:** 2026-03-17
**Status:** In Progress

## Requirement Traceability

- Source requirements: [requirements.md](requirements.md)
- Design: [design.md](design.md)

### Requirements Coverage

| Requirement | Task(s) | Notes |
|---|---|---|
| REQ-001 | Task 2 | Stdin delivery |
| REQ-002 | Task 1, Task 8 | Config + instructions file |
| REQ-003 | Task 9 | Skills directory |
| REQ-004 | Task 4 | Session params |
| REQ-005 | Task 5 | Unknown session detection |
| REQ-006 | Task 0, Task 6 | ClearSession field + max turns |
| REQ-007 | Task 3 | Nesting prevention |
| REQ-008 | Task 1, Task 7 | Config + effort flag |
| REQ-009 | Task 1, Task 7 | Config + chrome flag |
| REQ-010 | Task 1, Task 7 | Config + extraArgs |
| REQ-011 | Task 0, Task 10 | LoginRequired/LoginURL fields + detection |
| REQ-012 | Task 0, Task 11 | CachedInputTokens field + extraction |
| REQ-013 | Task 1, Task 6 | Config + max turns |
| REQ-014 | Task 12 | Workspace cwd |
| REQ-015 | Task 4 | Session cwd validation |
| REQ-016 | Task 2 | Verbose flag |
| REQ-017 | Task 1, Task 13 | Config + prompt template |
| REQ-018 | Task 14 | Env redaction |
| REQ-019 | Task 0, Task 12 | WorkingDir field + resolution |
| REQ-020 | Task 8 | Instructions file |
| REQ-021 | Task 2 | Benchmark test: startup overhead < 50ms |
| REQ-022 | Task 3 | Regression test: existing blocked env keys still blocked |
| REQ-023 | Task 8 | Instructions file path traversal |
| REQ-024 | Task 14 | Sensitive env in logs |
| REQ-025 | Task 9 | Skills content |
| REQ-026 | Task 8 | Instructions + system prompt mutual exclusion |

## Progress Summary

- Total Tasks: 15
- Completed: 13/15
- In Progress: None (Tasks 4 session wiring + 12 workspace cwd in run_handler pending integration)
- Test Coverage: 30+ new tests

## Tasks (TDD: Red-Green-Refactor)

### Component 0: Adapter Interfaces

#### Task 0: Extend adapter interfaces

**Linked Requirements:** REQ-006, REQ-011, REQ-012, REQ-019

**RED Phase:**
- [ ] Write test that `adapter.InvokeResult` has `ClearSession bool`, `LoginRequired bool`, `LoginURL string` fields and they serialize to/from JSON correctly
- [ ] Write test that `adapter.TokenUsage` has `CachedInputTokens int` field and it serializes correctly
- [ ] Write test that `adapter.InvokeInput` has `WorkingDir string` field and it serializes correctly
- [ ] Verify all new fields default to zero values

**GREEN Phase:**
- [ ] Add `ClearSession bool` to `adapter.InvokeResult`
- [ ] Add `LoginRequired bool` to `adapter.InvokeResult`
- [ ] Add `LoginURL string` to `adapter.InvokeResult`
- [ ] Add `CachedInputTokens int` to `adapter.TokenUsage`
- [ ] Add `WorkingDir string` to `adapter.InvokeInput`

**REFACTOR Phase:**
- None needed — these are additive field additions with zero-value defaults.

**Acceptance Criteria:**
- [ ] All new fields exist on their respective structs
- [ ] JSON serialization round-trips correctly
- [ ] Zero-value defaults preserve backward compatibility (no behavior change for existing code)

---

### Component 1: Config

#### Task 1: Add new Config fields

**Linked Requirements:** REQ-008, REQ-009, REQ-010, REQ-013, REQ-017, REQ-002

**RED Phase:**
- [ ] Write test for parsing new config fields (effort, chrome, maxTurnsPerRun, extraArgs, instructionsFilePath, promptTemplate)
- [ ] Write test for effort validation (only "", "low", "medium", "high")
- [ ] Write test for instructionsFilePath traversal rejection

**GREEN Phase:**
- [ ] Add fields to Config struct in `config.go`
- [ ] Add validation in `parseConfig()`

**REFACTOR Phase:**
- [ ] Extract validation helpers

**Acceptance Criteria:**
- [ ] All new fields parsed from JSON
- [ ] Invalid effort values rejected
- [ ] Path traversal in instructionsFilePath rejected
- [ ] Missing/zero-value fields use safe defaults

---

### Component 2: Stdin Delivery

#### Task 2: Switch prompt delivery to stdin

**Linked Requirements:** REQ-001, REQ-016

**RED Phase:**
- [ ] List existing tests that will break: `TestExecute_BuildArgs_BasicFlags`, `TestExecute_NoSessionPersistenceAlwaysPresent` — update these first
- [ ] Fix the `--no-session-persistence` test inconsistency before proceeding
- [ ] Write test verifying `--print -` in args and no positional prompt arg
- [ ] Write test verifying `--verbose` always present in args

**GREEN Phase:**
- [ ] Change `buildArgs()`: replace `--print` with `--print -`, add `--verbose`, remove positional prompt
- [ ] Change `executeOnce()`: set `cmd.Stdin = strings.NewReader(prompt)`

**REFACTOR Phase:**
- [ ] Clean up prompt extraction from envVars (single path)

**Acceptance Criteria:**
- [ ] Claude CLI receives prompt via stdin
- [ ] No arg length limit issues with large prompts
- [ ] `--verbose` always present
- [ ] Existing tests updated/pass

---

#### Task 2 Addendum: Invocation startup benchmark

**Linked Requirements:** REQ-021

**RED Phase:**
- [ ] Write benchmark test asserting invocation startup overhead < 50ms (from `Execute()` entry to subprocess spawn)

**GREEN Phase:**
- [ ] Ensure no unnecessary allocations or I/O in the hot path before subprocess spawn

**REFACTOR Phase:**
- [ ] Profile and optimize if benchmark fails

**Acceptance Criteria:**
- [ ] Benchmark test passes: startup overhead < 50ms
- [ ] Benchmark is reproducible in CI

---

### Component 3: Nesting Prevention

#### Task 3: Strip Claude Code nesting env vars

**Linked Requirements:** REQ-007, REQ-022

**RED Phase:**
- [ ] Write test: all `CLAUDE_CODE_*` prefixed vars are removed from env
- [ ] Write test: `CLAUDECODE` var is removed from env
- [ ] Write regression test: existing blocked env keys (PATH, HOME, ARI_*) still blocked after nesting changes (REQ-022)

**GREEN Phase:**
- [ ] Add prefix-based stripping of `CLAUDE_CODE_*` vars to `buildEnv()`
- [ ] Add explicit stripping of `CLAUDECODE` to `buildEnv()`

**REFACTOR Phase:**
- [ ] Consolidate env stripping into a single pass with clear prefix/exact-match lists

**Acceptance Criteria:**
- [ ] All `CLAUDE_CODE_*` prefixed vars stripped from child process env
- [ ] `CLAUDECODE` stripped from child process env
- [ ] Existing blocked env keys (PATH, HOME, ARI_*) still blocked
- [ ] Other env vars unaffected

---

### Component 4: Session Params

#### Task 4: Structured session state with cwd validation

**Linked Requirements:** REQ-004, REQ-015

**RED Phase:**
- [ ] Write test for SessionParams JSON marshal/unmarshal
- [ ] Write test for parsing legacy bare-string session IDs (not JSON) for backward compat
- [ ] Write test: canResumeSession returns true when cwd matches
- [ ] Write test: canResumeSession returns false when cwd differs
- [ ] Write test: canResumeSession returns true when stored cwd is empty

**GREEN Phase:**
- [ ] Define `SessionParams` struct in `run_handler.go`
- [ ] Session JSON parsing happens in `run_handler.go`; only the `sessionId` string is passed to the adapter
- [ ] Change session persistence to store JSON `{sessionId, cwd}` instead of opaque string
- [ ] Add `canResumeSession()` function
- [ ] Wire cwd validation into `Invoke()` before passing session to adapter
- [ ] Handle backward compatibility: parse old opaque session strings as `{sessionId: <string>, cwd: ""}`

**REFACTOR Phase:**
- [ ] Ensure both task and conversation session paths use structured params

**Acceptance Criteria:**
- [ ] Session state stored as JSON with sessionId + cwd
- [ ] Resume blocked when cwd mismatch (warning logged)
- [ ] Old opaque session strings handled gracefully
- [ ] Both task and conversation sessions updated

---

### Component 5: Unknown Session Detection

#### Task 5: Smart retry on unknown session errors

> **Note:** This replaces the current unconditional retry-on-failure behavior. Update `TestExecute_SessionResume_Fallback` to reflect the narrowed retry condition.

**Linked Requirements:** REQ-005

**RED Phase:**
- [ ] Write test: `isUnknownSessionError` matches "no conversation found with session id"
- [ ] Write test: `isUnknownSessionError` matches "unknown session"
- [ ] Write test: `isUnknownSessionError` matches "session xyz not found"
- [ ] Write test: `isUnknownSessionError` does not match normal errors

**GREEN Phase:**
- [ ] Add `isUnknownSessionError()` in `parser.go`
- [ ] Update retry logic in `Execute()` to use specific detection instead of generic failure check

**REFACTOR Phase:**
- [ ] Compile regex once at package level

**Acceptance Criteria:**
- [ ] Unknown session errors trigger retry without --resume
- [ ] Normal failures still trigger generic retry (existing behavior)
- [ ] Warning logged when session error detected

---

### Component 6: Max Turns Detection

#### Task 6: Detect max turns and clear session

**Linked Requirements:** REQ-006, REQ-013

**RED Phase:**
- [ ] Write test: `isMaxTurnsResult` detects `subtype=error_max_turns`
- [ ] Write test: `isMaxTurnsResult` detects `stop_reason=max_turns`
- [ ] Write test: `--max-turns N` in args when configured
- [ ] Write test: session cleared (ClearSession flag) on max turns

**GREEN Phase:**
- [ ] Add `isMaxTurnsResult()` in `parser.go`
- [ ] `ClearSession` field on `adapter.InvokeResult` was already added in Task 0
- [ ] Add `--max-turns` to `buildArgs()` when `cfg.MaxTurnsPerRun > 0`
- [ ] Add detection logic: set `ClearSession = true` when max turns detected
- [ ] Handle ClearSession in `finalize()` — don't persist session state

**REFACTOR Phase:**
- [ ] Ensure max-turns detection works with both stream-json and result parsing

**Acceptance Criteria:**
- [ ] `--max-turns` passed to CLI when configured
- [ ] Max turns result detected from stream-json
- [ ] Session state NOT persisted after max turns (next run starts fresh)

---

### Component 7: CLI Flags

#### Task 7: Add --effort, --chrome, --extraArgs flags

**Linked Requirements:** REQ-008, REQ-009, REQ-010

**RED Phase:**
- [ ] Write test: `--effort low` in args when effort="low"
- [ ] Write test: no `--effort` when effort=""
- [ ] Write test: `--chrome` in args when chrome=true
- [ ] Write test: extra args appended last

**GREEN Phase:**
- [ ] Add flag generation to `buildArgs()`

**REFACTOR Phase:**
- [ ] Group optional flag logic clearly

**Acceptance Criteria:**
- [ ] All three flags correctly generated
- [ ] Extra args are always last (after all other flags)
- [ ] No flags added when config values are zero/empty

---

### Component 8: Instructions File

#### Task 8: Per-agent instructions file injection

**Linked Requirements:** REQ-002, REQ-020, REQ-023, REQ-026

**RED Phase:**
- [ ] Write test: `--append-system-prompt-file /path/to/file` in args
- [ ] Write test: relative path resolved against workingDir
- [ ] Write test: missing file skipped with warning (no --append-system-prompt-file)
- [ ] Write test: `..` traversal in resolved path rejected

**GREEN Phase:**
- [ ] Add `resolveInstructionsFile()` in `config.go`
- [ ] Add file existence check in `executeOnce()`
- [ ] Add `--append-system-prompt-file` to `buildArgs()` when file exists
- [ ] When `instructionsFilePath` is set AND agent has `systemPrompt`, create a temp file combining both, use only `--append-system-prompt-file`
- [ ] When no `instructionsFilePath`, continue using `--append-system-prompt` as today

**REFACTOR Phase:**
- [ ] Handle mutual exclusion: `--append-system-prompt` vs `--append-system-prompt-file`
- [ ] Clean up temp file (combined instructions) after invocation completes

**Acceptance Criteria:**
- [ ] Instructions file injected when configured and exists
- [ ] Relative paths resolved correctly
- [ ] Missing files don't crash — warning logged
- [ ] Path traversal prevented

---

### Component 9: Skills Directory

#### Task 9: Create and inject Ari skills

**Linked Requirements:** REQ-003, REQ-025

**RED Phase:**
- [ ] Write test: `--add-dir` in args pointing to skills directory
- [ ] Write test: missing skills dir skipped (no --add-dir, warning logged)

**GREEN Phase:**
- [ ] Use `go:embed` for SKILL.md content (embed the file directly into the Go binary)
- [ ] Create temp dir per run, write embedded SKILL.md content to `<tmpdir>/.claude/skills/ari/SKILL.md`
- [ ] Pass temp dir via `--add-dir` to `buildArgs()`
- [ ] Clean up temp dir in `defer` after invocation completes

**REFACTOR Phase:**
- [ ] Extract temp dir creation/cleanup into a helper

**Acceptance Criteria:**
- [ ] SKILL.md contains Ari heartbeat procedure and API reference
- [ ] `--add-dir` injected pointing to temp dir with embedded skill content
- [ ] Temp dir cleaned up after each invocation
- [ ] No dependency on filesystem paths at startup

---

### Component 10: Login Detection

#### Task 10: Detect login required errors

**Linked Requirements:** REQ-011

**RED Phase:**
- [ ] Write test: `detectLoginRequired` matches "not logged in"
- [ ] Write test: `detectLoginRequired` matches "please run `claude login`"
- [ ] Write test: `detectLoginRequired` extracts URL from output
- [ ] Write test: normal errors not detected as login required

**GREEN Phase:**
- [ ] Add `detectLoginRequired()` in `parser.go`
- [ ] `LoginRequired` and `LoginURL` fields on `adapter.InvokeResult` were already added in Task 0
- [ ] Call detection in `Execute()` when run fails
- [ ] Surface in `finalize()` as inbox alert with login URL

**REFACTOR Phase:**
- [ ] Compile regex once at package level

**Acceptance Criteria:**
- [ ] Login errors detected from stderr/stdout
- [ ] Login URL extracted when available
- [ ] Error surfaced to UI via inbox alert

---

### Component 11: Cached Token Tracking

#### Task 11: Extract cache_read_input_tokens

**Linked Requirements:** REQ-012

**RED Phase:**
- [ ] Write test: `cache_read_input_tokens` extracted from stream-json usage event
- [ ] Write test: missing field defaults to 0

**GREEN Phase:**
- [ ] `CachedInputTokens` field on `adapter.TokenUsage` was already added in Task 0
- [ ] Extract from `usage.cache_read_input_tokens` in parser
- [ ] Include in `usage_json` stored in DB

**REFACTOR Phase:**
- [ ] Update cost display to show cache savings

**Acceptance Criteria:**
- [ ] Cached tokens extracted and stored
- [ ] Backward compatible (old runs without field still work)

---

### Component 12: Workspace Cwd Resolution

#### Task 12: Resolve working directory from project workspace

**Linked Requirements:** REQ-014, REQ-019

**RED Phase:**
- [ ] Write test: task with project workspace → uses workspace cwd
- [ ] Write test: task without project → falls back to agent workingDir
- [ ] Write test: no workingDir → uses server cwd (empty string)

**GREEN Phase:**
- [ ] `WorkingDir` field on `adapter.InvokeInput` was already added in Task 0
- [ ] Add `resolveWorkingDir()` in `run_handler.go`
- [ ] Run handler populates `InvokeInput.WorkingDir`; adapter uses it with priority over `cfg.WorkingDir`
- [ ] Query project by task's projectId, get workspace cwd
- [ ] Wire into `buildInvokeInput()` to set resolved cwd

**REFACTOR Phase:**
- [ ] Cache project lookup (avoid repeated DB call per run)

**Acceptance Criteria:**
- [ ] Project workspace cwd used when available
- [ ] Falls back correctly through priority chain
- [ ] No crash when project/workspace doesn't exist

---

### Component 13: Prompt Template

#### Task 13: Configurable prompt template per agent

**Linked Requirements:** REQ-017

**RED Phase:**
- [ ] Write test: `renderTemplate` replaces `{{agent.name}}`, `{{agent.role}}`, etc.
- [ ] Write test: unknown variables left as-is (no crash)
- [ ] Write test: default template used when not configured

**GREEN Phase:**
- [ ] Add `renderTemplate()` function
- [ ] Use template output as prompt prefix in `buildInvokeInput()` instead of hardcoded `fmt.Sprintf`
- [ ] Default template: `"You are {{agent.name}}, a {{agent.role}} in squad {{squad.name}}."`

**REFACTOR Phase:**
- [ ] Ensure task/conversation-specific context still appended after template

**Acceptance Criteria:**
- [ ] Custom template renders correctly
- [ ] Default template matches current behavior
- [ ] Task/conversation context preserved

---

### Component 14: Env Redaction

#### Task 14: Redact sensitive env vars in logs

**Linked Requirements:** REQ-018, REQ-024

**RED Phase:**
- [ ] Write test: KEY, TOKEN, SECRET, PASSWORD values redacted
- [ ] Write test: non-sensitive values preserved
- [ ] Write test: case-insensitive matching (api_key, Api_Token)

**GREEN Phase:**
- [ ] Add `redactEnvForLog()` in `claude.go`
- [ ] Call before any env logging in executeOnce()

**REFACTOR Phase:**
- [ ] Compile regex once at package level

**Acceptance Criteria:**
- [ ] Sensitive env values show "***REDACTED***" in logs
- [ ] Non-sensitive values unchanged
- [ ] No sensitive data in SSE events

---

## Commit Strategy

After each completed task:
```bash
git add internal/adapter/claude/ internal/server/handlers/ internal/adapter/adapter.go
git commit -m "feat(claude-adapter): <task description>"
```

## Notes

### Implementation Notes

- Task 0 is **prerequisite** — extends adapter interfaces for all downstream tasks
- Tasks 1-7 are **high priority** — fix real problems with session resume and safety
- Tasks 8-11 are **medium priority** — add valuable features from Paperclip
- Tasks 12-14 are **low priority** — polish and convenience
- Each task is independently deployable (no task depends on a later task)
- Backward compatibility: old `adapterConfig` JSON without new fields works unchanged

### Future Improvements

- MCP server injection for agents
- Agent config UI for new fields (effort, chrome, instructions file)
- Workspace git clone support (Paperclip's repo workspace feature)
- Per-agent skills (not just shared Ari skill)
