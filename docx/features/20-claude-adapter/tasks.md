# Tasks: Claude Adapter

**Created:** 2026-03-15
**Status:** Complete

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-CLA-001 through REQ-CLA-041

## Implementation Approach

Work bottom-up: prerequisite fix first, then config and helpers, then stream-json event types and parser, then the core adapter struct and Execute method, then TestEnvironment and Models, then registration and integration. Each layer's tests stand alone using the actual `claude` binary or mock scripts.

## Progress Summary

- Total Tasks: 7
- Completed: 7
- In Progress: None
- Remaining: None
- Test Coverage: TBD

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 00 — Prerequisite: Fix OnLogLine Hook to Forward Fields

**Requirements:** Prerequisite for REQ-CLA-009, REQ-CLA-020, REQ-CLA-023
**Estimated time:** 10 min

#### Context

The current `OnLogLine` hook in `run_handler.go` (line ~155) does NOT forward `line.Fields` to the SSE payload. It only sends `level`, `message`, and `timestamp`. For tool-call UI rendering to work, `line.Fields` MUST be included. Without this fix, all the structured `LogLine.Fields` data produced by the Claude adapter (tool names, inputs, rate-limit indicators, event types) would be silently dropped before reaching the UI.

#### RED — Write Failing Tests

1. `TestOnLogLine_ForwardsFields` — in `run_handler_test.go` or equivalent: create a mock SSE hub, set up the OnLogLine hook, call it with a LogLine containing `Fields: {"toolName": "Read"}`, verify the published SSE payload includes `"fields": {"toolName": "Read"}`.
2. `TestOnLogLine_NilFieldsOmitted` — call OnLogLine with nil Fields, verify the published payload does NOT contain a `"fields"` key.

#### GREEN — Implement Minimum to Pass

1. Modify the `OnLogLine` hook in `internal/server/handlers/run_handler.go` to include `line.Fields`:

```go
// BEFORE:
OnLogLine: func(line adapter.LogLine) {
    s.sseHub.Publish(wakeup.SquadID, "heartbeat.run.log", map[string]any{
        "runId":     run.ID,
        "agentId":   agent.ID,
        "level":     line.Level,
        "message":   line.Message,
        "timestamp": line.Timestamp.Format("2006-01-02T15:04:05Z"),
    })
},

// AFTER:
OnLogLine: func(line adapter.LogLine) {
    payload := map[string]any{
        "runId":     run.ID,
        "agentId":   agent.ID,
        "level":     line.Level,
        "message":   line.Message,
        "timestamp": line.Timestamp.Format("2006-01-02T15:04:05Z"),
    }
    if line.Fields != nil {
        payload["fields"] = line.Fields
    }
    s.sseHub.Publish(wakeup.SquadID, "heartbeat.run.log", payload)
},
```

#### Acceptance Criteria

- [x] `line.Fields` is forwarded in the SSE payload when non-nil
- [x] `fields` key is omitted from payload when `line.Fields` is nil (no empty object)
- [x] `make test` passes
- [x] No regressions in existing SSE event handling

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/run_handler.go`

---

### [x] Task 01 — Config Parsing and Defaults

**Requirements:** REQ-CLA-004, REQ-CLA-006, REQ-CLA-007, REQ-CLA-016, REQ-CLA-026, REQ-CLA-030, REQ-CLA-032, REQ-CLA-036, REQ-CLA-039, REQ-CLA-041
**Estimated time:** 30 min

#### Context

The Claude adapter config is a JSON struct with optional fields that all have sensible defaults. The `parseConfig` function must unmarshal `adapterConfig` JSON, apply defaults for missing fields, and validate the `workingDir` (absolute path, no `..` segments). This is a standalone unit with no external dependencies. Note: `maxTurns` has been removed; replaced by `maxBudgetUSD` for cost-based limits. `skipPermissions` defaults to `true` for headless agents.

#### RED — Write Failing Tests

Create `internal/adapter/claude/config_test.go`:

1. `TestParseConfig_Defaults` — pass `nil` raw JSON, verify `ClaudePath == "claude"`, `Model == "sonnet"`, `TimeoutSeconds == 3600`, `MaxExcerptBytes == 65536`, `*SkipPermissions == true`, `MaxBudgetUSD == 0`.
2. `TestParseConfig_AllFields` — pass full JSON with all fields set, verify each field is correctly parsed including `maxBudgetUSD`, `skipPermissions`.
3. `TestParseConfig_PartialFields` — pass JSON with only `model` and `maxBudgetUSD` set, verify those are parsed and all others get defaults.
4. `TestParseConfig_EmptyJSON` — pass `{}`, verify all defaults are applied.
5. `TestParseConfig_InvalidJSON` — pass malformed JSON, verify defaults are applied (no error — defensive parsing).
6. `TestParseConfig_SkipPermissionsFalse` — pass `{"skipPermissions": false}`, verify `*SkipPermissions == false`.
7. `TestValidateWorkingDir_Absolute` — absolute path without `..` is valid.
8. `TestValidateWorkingDir_Relative` — relative path returns error.
9. `TestValidateWorkingDir_DotDot` — path containing `..` returns error.
10. `TestValidateWorkingDir_Empty` — empty string is valid (uses default).

Tests fail because `internal/adapter/claude/` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/adapter/claude/config.go` with:
   - `Config` struct with all fields and JSON tags (no `MaxTurns` — removed).
   - Constants: `DefaultClaudePath`, `DefaultModel`, `DefaultTimeoutSeconds`, `DefaultMaxExcerptBytes`.
   - `parseConfig(raw json.RawMessage) Config` — unmarshal + apply defaults. `SkipPermissions` defaults to `true` via pointer nil check.
   - `validateWorkingDir(dir string) error` — check absolute path, no `..` segments.
2. All ten tests pass.

#### REFACTOR

- Ensure all exported types and constants have doc comments.
- Verify the Config struct JSON tags match the schema documented in design.md.

#### Acceptance Criteria

- [x] `Config` struct defined with all fields from design.md (no `MaxTurns`)
- [x] `parseConfig` applies defaults for all missing fields
- [x] `parseConfig` handles nil, empty, partial, and malformed JSON
- [x] `SkipPermissions` defaults to `true` when not set
- [x] `validateWorkingDir` rejects relative paths and `..` segments
- [x] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/adapter/claude/config.go`
- **Create:** `internal/adapter/claude/config_test.go`

---

### [x] Task 02 — Stream-JSON Event Types and Parser

**Requirements:** REQ-CLA-008, REQ-CLA-009, REQ-CLA-010, REQ-CLA-011, REQ-CLA-020, REQ-CLA-031
**Estimated time:** 60 min

#### Context

The Claude CLI with `--output-format stream-json` emits one JSON event per stdout line. Each event has a `type` discriminator field. The parser must handle the key event types: `system` (init with session_id), `assistant` (with tool-use content blocks), `tool_result`, `result` (final cost/usage summary), and `rate_limit_event`. This is a pure-function unit with no external dependencies — testable with string inputs.

The old `claudeJSONOutput` struct and `parseClaudeOutput` function are replaced by the `claudeEvent` type hierarchy and `eventCollector` pattern.

#### RED — Write Failing Tests

Create `internal/adapter/claude/parser_test.go`:

1. `TestParseEvent_SystemInit` — parse `{"type":"system","subtype":"init","session_id":"sess-123","model":"claude-sonnet-4-6"}`. Verify session_id and model are extracted.
2. `TestParseEvent_AssistantWithToolUse` — parse `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/src/main.go"}}]}}`. Verify tool name and input are extracted.
3. `TestParseEvent_AssistantWithText` — parse assistant event with text content block. Verify text is extracted.
4. `TestParseEvent_AssistantWithMixedContent` — parse assistant event with both text and tool_use blocks. Verify both are handled.
5. `TestParseEvent_ToolResult` — parse `{"type":"tool_result","tool_use_id":"toolu_01","content":"file contents"}`. Verify fields extracted.
6. `TestParseEvent_Result` — parse `{"type":"result","subtype":"success","session_id":"sess-123","total_cost_usd":0.086,"usage":{"input_tokens":5000,"output_tokens":1200},"modelUsage":{"claude-sonnet-4-6":{"inputTokens":5000,"outputTokens":1200,"costUSD":0.086}}}`. Verify `total_cost_usd`, usage tokens, and modelUsage extracted.
7. `TestParseEvent_RateLimitEvent` — parse `{"type":"rate_limit_event","rate_limit_info":{"retryAfterMs":5000}}`. Verify rate limit info extracted.
8. `TestParseEvent_MalformedJSON` — parse `{broken json`. Verify graceful skip (no panic, no error propagation).
9. `TestParseEvent_UnknownType` — parse `{"type":"unknown_future_event"}`. Verify no crash, forwarded as debug.
10. `TestEventCollector_ExtractUsage` — feed system/init and result events to collector. Verify `extract()` returns correct `TokenUsage` and `sessionState`.
11. `TestEventCollector_NoResultEvent` — collector with only system/init event. Verify `extract()` returns session ID but zero usage.
12. `TestEventCollector_EmptyCollector` — empty collector returns zero usage and empty session.
13. `TestStreamAndParseEvents_FullStream` — simulate a complete stream (system/init → assistant → tool_result → assistant → result). Verify hooks.OnLogLine called for each event with correct fields. Verify collector has correct session_id and result.
14. `TestStreamAndParseEvents_ToolCallFields` — verify that tool-use events produce LogLine with `Fields["toolName"]` and `Fields["toolInput"]`.
15. `TestStreamAndParseEvents_RateLimitDetected` — include a rate_limit_event in the stream. Verify LogLine with `Fields["rateLimited"] == true`.
16. `TestStreamStderr_RateLimitFallback` — stderr containing "rate limit". Verify LogLine with `Fields["rateLimited"] == true` and `Fields["source"] == "stderr"`.
17. `TestStreamStderr_429Fallback` — stderr containing "429". Same verification.
18. `TestStreamStderr_NormalLine` — normal stderr. Verify `level == "error"`, no `rateLimited` field.

Tests fail because `internal/adapter/claude/parser.go` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/adapter/claude/parser.go` with:
   - `claudeEvent` struct with type discriminator and type-specific fields.
   - `claudeMessage`, `claudeContentBlock`, `claudeUsage`, `claudeModelUse`, `claudeRateLimitInfo` types.
   - `eventCollector` struct with `sessionID` and `resultEvent` fields, plus `extract()` method.
   - `streamAndParseEvents(r io.Reader, buf *bytes.Buffer, maxBytes int, hooks adapter.Hooks, collector *eventCollector)` — reads lines, parses each as claudeEvent, dispatches by type.
   - `streamStderr(r io.Reader, buf *bytes.Buffer, maxBytes int, hooks adapter.Hooks)` — reads stderr, detects rate limits as fallback.
2. All eighteen tests pass.

#### REFACTOR

- Ensure JSON parsing is defensive: never panic on unexpected shapes.
- Add doc comments explaining the stream-json event format and each event type.
- Verify the `claudeEvent` struct handles all documented event types.

#### Acceptance Criteria

- [x] `claudeEvent` type hierarchy correctly models all stream-json event types
- [x] `eventCollector.extract()` returns `TokenUsage` and `SessionState` from collected events
- [x] `streamAndParseEvents` extracts `toolName`, `toolInput` from assistant tool-use content blocks into `LogLine.Fields`
- [x] `streamAndParseEvents` detects `rate_limit_event` and produces LogLine with `rateLimited: true`
- [x] `streamStderr` detects rate limits via "429" and "rate limit" patterns (fallback)
- [x] Malformed JSON events are skipped gracefully (REQ-CLA-031)
- [x] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/adapter/claude/parser.go`
- **Create:** `internal/adapter/claude/parser_test.go`

---

### [x] Task 03 — ClaudeAdapter Struct, Type, Models, TestEnvironment

**Requirements:** REQ-CLA-024, REQ-CLA-028, REQ-CLA-029, REQ-CLA-034
**Estimated time:** 30 min

#### Context

The `ClaudeAdapter` struct implements `adapter.Adapter`. This task creates the struct, constructor, `Type()`, `Models()`, and `TestEnvironment()` methods — everything except `Execute()`. `TestEnvironment` checks for the `claude` binary at basic level and runs `claude --version` at full level.

#### RED — Write Failing Tests

Create `internal/adapter/claude/claude_test.go`:

1. `TestClaudeAdapter_Type` — verify `Type() == "claude_local"`.
2. `TestClaudeAdapter_Models` — verify `Models()` returns exactly 3 entries: opus, sonnet, haiku. Verify each has `Provider == "anthropic"`.
3. `TestClaudeAdapter_ImplementsInterface` — compile-time check: `var _ adapter.Adapter = (*claude.ClaudeAdapter)(nil)`.
4. `TestClaudeAdapter_TestEnvironmentBasic_Found` — if `claude` is in PATH, verify `Available == true`. (Skip if not available.)
5. `TestClaudeAdapter_TestEnvironmentBasic_NotFound` — create adapter with config pointing to `/nonexistent/claude`, verify `Available == false` and message contains "not found".

Tests fail because `internal/adapter/claude/claude.go` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/adapter/claude/claude.go` with:
   - `ClaudeAdapter` struct (empty — stateless).
   - `New() *ClaudeAdapter` constructor.
   - `Type() string` returning `"claude_local"`.
   - `Models() []adapter.ModelDefinition` returning opus, sonnet, haiku.
   - `TestEnvironment(level TestLevel) (TestResult, error)` — basic: `exec.LookPath("claude")`; full: `claude --version`.
   - `Execute()` stub returning `RunStatusFailed` with "not implemented" error (will be filled in Task 04).
2. All five tests pass.

#### REFACTOR

- Add doc comments on all exported methods.
- Ensure `TestEnvironment` uses a 10-second timeout for `claude --version` to avoid hanging.

#### Acceptance Criteria

- [x] `ClaudeAdapter` satisfies `adapter.Adapter` interface (compile-time check)
- [x] `Type()` returns `"claude_local"`
- [x] `Models()` returns 3 Anthropic models
- [x] `TestEnvironment(Basic)` checks `exec.LookPath`
- [x] `TestEnvironment(Full)` runs `claude --version`
- [x] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/adapter/claude/claude.go`
- **Create:** `internal/adapter/claude/claude_test.go`

---

### [x] Task 04 — Execute Method (Core Subprocess Logic)

**Requirements:** REQ-CLA-001, REQ-CLA-002, REQ-CLA-003, REQ-CLA-014, REQ-CLA-015, REQ-CLA-016, REQ-CLA-017, REQ-CLA-018, REQ-CLA-019, REQ-CLA-021, REQ-CLA-022, REQ-CLA-025, REQ-CLA-035, REQ-CLA-037, REQ-CLA-038, REQ-CLA-039, REQ-CLA-040, REQ-CLA-041
**Estimated time:** 90 min

#### Context

The `Execute` method is the core of the Claude adapter. It builds CLI arguments, spawns the `claude` subprocess, streams stdout/stderr, handles timeout and cancellation, and returns `InvokeResult`. This task wires together the config (Task 01), parser (Task 02), and adapter struct (Task 03). Tests use mock shell scripts (via `sh -c`) to simulate Claude CLI behavior since the real CLI may not be available in CI.

#### RED — Write Failing Tests

Add to `internal/adapter/claude/claude_test.go`:

1. `TestExecute_BuildArgs_BasicFlags` — create an input with model, system prompt, and prompt. Mock `claude` with a shell script that echoes its args. Verify `--print`, `--output-format stream-json`, `--model sonnet`, `--append-system-prompt`, `--dangerously-skip-permissions`, `--no-session-persistence` are present.
2. `TestExecute_BuildArgs_WithResume` — set `SessionState` in input. Verify `--resume <session>` is in args.
3. `TestExecute_BuildArgs_WithMaxBudget` — set `maxBudgetUSD: 5.00` in config. Verify `--max-budget-usd 5.00` is in args.
4. `TestExecute_BuildArgs_WithAllowedTools` — set `allowedTools: ["Read","Write"]`. Verify `--allowedTools Read,Write` is in args.
5. `TestExecute_BuildArgs_SkipPermissionsFalse` — set `skipPermissions: false` in config. Verify `--dangerously-skip-permissions` is NOT in args.
6. `TestExecute_SuccessfulRun` — mock `claude` to emit stream-json events: a system/init event, an assistant event, and a result event with `total_cost_usd: 0.086`. Verify `Status=succeeded`, `ExitCode=0`, `Usage.InputTokens`, `SessionState` from init event.
7. `TestExecute_NonZeroExit` — mock with `sh -c 'exit 1'`. Verify `Status=failed`, `ExitCode=1`.
8. `TestExecute_Timeout` — set `timeoutSeconds=1`, mock with `sleep 60`. Verify `Status=timed_out` returned within ~2s.
9. `TestExecute_ContextCancellation` — start mock `sleep 60`, cancel context after 100ms. Verify `Status=cancelled` within 500ms.
10. `TestExecute_EnvVarsInjected` — mock with `sh -c 'echo $ARI_AGENT_ID'`. Pass `ARI_AGENT_ID=test-123` in EnvVars. Verify stdout contains `test-123`.
11. `TestExecute_ApiKeyNotInArgs` — mock with script that echoes `$0 $@` (shows all args). Verify `ARI_API_KEY` does not appear in output (REQ-CLA-035).
12. `TestExecute_WorkingDirSet` — set `workingDir` to `/tmp`. Mock with `pwd`. Verify stdout contains `/tmp` or `/private/tmp`.
13. `TestExecute_WorkingDirRelativeRejected` — set `workingDir` to `./relative`. Verify `Execute` returns error with `RunStatusFailed`.
14. `TestExecute_StdoutExcerptTruncated` — emit more than `maxExcerptBytes`. Verify `Stdout` is truncated.
15. `TestExecute_HooksOnLogLineCalled` — verify `hooks.OnLogLine` is called for each stream-json event.
16. `TestExecute_HooksOnStatusChangeCalled` — emit a stream-json assistant event with tool_use. Verify `hooks.OnStatusChange` is called with `"tool:<name>"`.
17. `TestExecute_ConcurrentExecutions` — launch 3 Execute calls simultaneously with `-race`. Verify no data races.
18. `TestExecute_AppendSystemPromptUsed` — verify `--append-system-prompt` (NOT `--system-prompt`) is used when system prompt is provided.
19. `TestExecute_NoSessionPersistenceAlwaysPresent` — verify `--no-session-persistence` is always in args.

Tests fail because `Execute` is currently a stub.

#### GREEN — Implement Minimum to Pass

1. Replace the `Execute` stub in `internal/adapter/claude/claude.go` with the full implementation:
   - Call `parseConfig` to extract config from `input.Agent.AdapterConfig`.
   - Call `validateWorkingDir` if `cfg.WorkingDir` is non-empty.
   - Call `buildArgs` to construct CLI arguments (using `--append-system-prompt`, `--output-format stream-json`, `--dangerously-skip-permissions`, `--no-session-persistence`, `--max-budget-usd`).
   - Create `exec.CommandContext` with timeout context.
   - Set `cmd.Dir`, `cmd.Env` (via `buildEnv`), `cmd.SysProcAttr`.
   - Create stdout/stderr pipes.
   - `cmd.Start()` — return failed on error (REQ-CLA-037).
   - Create `eventCollector`.
   - Launch two goroutines: `streamAndParseEvents` for stdout (with collector), `streamStderr` for stderr.
   - Wait for both goroutines via done channel (REQ-CLA-038).
   - `cmd.Wait()`.
   - Determine `RunStatus` from context errors and exit code.
   - For cancellation: SIGTERM then 5s then SIGKILL (REQ-CLA-021).
   - Call `collector.extract()` for usage and session state (REQ-CLA-010, REQ-CLA-011).
   - Build and return `InvokeResult`.
2. Create `buildArgs` method on `ClaudeAdapter`.
3. Create `buildEnv` function.
4. All nineteen tests pass with `-race`.

#### REFACTOR

- Extract the SIGTERM-then-SIGKILL pattern into a `gracefulKill(pid int, timeout time.Duration)` helper.
- Ensure the 5-second SIGKILL timer is properly cleaned up when the process exits before the timer fires.
- Verify no goroutine leaks by checking goroutine count before/after Execute.

#### Acceptance Criteria

- [x] `Execute` spawns `claude` (or mock) with correct flags
- [x] `--append-system-prompt` used (NOT `--system-prompt`) to preserve Claude Code's built-in capabilities
- [x] `--output-format stream-json` used (NOT `json`)
- [x] `--dangerously-skip-permissions` included when `skipPermissions` is true (default)
- [x] `--no-session-persistence` always included
- [x] `--max-budget-usd` passed when `maxBudgetUSD > 0`
- [x] Session resume via `--resume` flag when session state is available
- [x] ARI_* env vars injected into subprocess environment
- [x] ARI_API_KEY never appears in command-line arguments
- [x] Timeout kills process group and returns `RunStatusTimedOut`
- [x] Context cancellation sends SIGTERM, then SIGKILL after 5s
- [x] Stdout/stderr streamed in real time via hooks
- [x] Token usage and session state extracted from collected stream-json events
- [x] Working directory validation rejects relative paths and `..`
- [x] No data races under concurrent execution (`-race`)
- [x] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/adapter/claude/claude.go` (replace Execute stub)

---

### [x] Task 05 — Session Resume with Fallback

**Requirements:** REQ-CLA-012, REQ-CLA-013, REQ-CLA-032
**Estimated time:** 30 min

#### Context

When a previous session state is available, the adapter passes `--resume <session_id>` to Claude. If the session is invalid or expired, Claude may return an error. The adapter detects this and retries once without `--resume`, falling back to a fresh session. The `disableResumeOnError` config option forces fresh sessions always. NOTE: Retry logic is simplified — the adapter only does one fallback attempt; higher-level retries are handled by the run service.

#### RED — Write Failing Tests

Add to `internal/adapter/claude/claude_test.go`:

1. `TestExecute_SessionResume_Success` — set `SessionState="sess-abc"` in input, mock `claude` that checks for `--resume` flag and returns success with a system/init event. Verify the flag was passed and run succeeds.
2. `TestExecute_SessionResume_Fallback` — mock `claude` that fails with exit code 1 when `--resume` is passed, succeeds without it. Verify the adapter retries without `--resume` and the second attempt succeeds.
3. `TestExecute_SessionResume_FallbackLogsWarning` — same scenario as above. Verify `hooks.OnLogLine` is called with `level="warn"` and a message about session resume failure.
4. `TestExecute_DisableResumeOnError` — set `disableResumeOnError: true` in config and `SessionState="sess-abc"` in input. Verify `--resume` is NOT passed even though session state is available.
5. `TestExecute_NoSessionState_NoResumeFlag` — set empty `SessionState`. Verify `--resume` is NOT in the args.

Tests fail because the retry-on-resume-failure logic is not yet implemented.

#### GREEN — Implement Minimum to Pass

1. Modify `Execute` in `claude.go` to:
   - Track whether `--resume` was used.
   - If the first execution fails AND `--resume` was used, log a warning and retry once without `--resume`.
   - Limit retry to exactly one attempt (no infinite loops).
   - Skip `--resume` entirely when `cfg.DisableResumeOnError` is true.
2. All five tests pass.

#### REFACTOR

- Extract the "run with optional retry" logic into a helper method to keep `Execute` clean.
- Ensure the retry path properly resets stdout/stderr buffers and creates a fresh eventCollector.
- Ensure the retry respects the original timeout (not double the timeout).

#### Acceptance Criteria

- [x] `--resume` passed when session state is available and `disableResumeOnError` is false
- [x] Failed resume triggers exactly one retry without `--resume`
- [x] Warning logged on resume fallback
- [x] `disableResumeOnError: true` prevents `--resume` from being added
- [x] Retry respects original timeout context
- [x] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/adapter/claude/claude.go`

---

### [x] Task 06 — Adapter Registration and Server Integration

**Requirements:** REQ-CLA-024, REQ-CLA-028
**Estimated time:** 30 min

#### Context

The Claude adapter must be registered in the adapter registry at server startup so that agents with `adapterType = "claude_local"` are dispatched to it. This task wires the adapter into the existing server initialization flow (where the process adapter is already registered) and runs `TestEnvironment` at startup.

#### RED — Write Failing Tests

1. `TestRegistry_ClaudeAdapterRegistered` — in `internal/adapter/claude/claude_test.go`: create a registry, register the Claude adapter, call `Resolve("claude_local")`, verify non-nil and `Type() == "claude_local"`.
2. `TestRegistry_ClaudeAndProcessCoexist` — register both `process.New()` and `claude.New()`, resolve each by type, verify both are returned correctly.
3. Verify that the server initialization code in `cmd/ari/` registers `claude_local` — this may be an integration-level check via `grep` in CI or a simple smoke test.

Tests fail because the Claude adapter is not yet registered in the server.

#### GREEN — Implement Minimum to Pass

1. In the server initialization code (find where `process.New()` is registered and add `claude.New()` alongside it):
   - Import `github.com/xb/ari/internal/adapter/claude`.
   - Call `registry.Register(claude.New())`.
   - Call `TestEnvironment(adapter.TestLevelBasic)` for the Claude adapter.
   - If unavailable, call `registry.MarkUnavailable("claude_local", result.Message)`.
2. All tests pass. The server starts without error even when `claude` binary is not installed (marked unavailable gracefully).

#### REFACTOR

- Ensure adapter registration is idempotent and order-independent.
- Verify that `make build` succeeds with the new import.
- Run `make test` to confirm no regressions.

#### Acceptance Criteria

- [x] `claude.New()` is registered in the adapter registry at server startup
- [x] `TestEnvironment` is called for Claude adapter at startup
- [x] Missing `claude` binary results in `MarkUnavailable`, not a startup crash
- [x] `Resolve("claude_local")` returns the Claude adapter when available
- [x] `make build` and `make test` pass
- [x] Server starts and runs without error when `claude` is not installed

#### Files to Create / Modify

- **Modify:** Server initialization code (e.g., `cmd/ari/run.go` or equivalent where adapters are registered)
- **Modify:** `internal/adapter/claude/claude_test.go` (add registration tests)
