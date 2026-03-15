# Tasks: Claude Adapter

**Created:** 2026-03-15
**Status:** Not started

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-CLA-001 through REQ-CLA-038

## Implementation Approach

Work bottom-up: config and helpers first, then the core adapter struct and Execute method, then output parsing, then TestEnvironment and Models, then registration and integration. Each layer's tests stand alone using the actual `claude` binary or mock scripts.

## Progress Summary

- Total Tasks: 6
- Completed: 0
- In Progress: None
- Remaining: All
- Test Coverage: TBD

---

## Tasks (TDD: Red-Green-Refactor)

---

### [ ] Task 01 — Config Parsing and Defaults

**Requirements:** REQ-CLA-004, REQ-CLA-005, REQ-CLA-006, REQ-CLA-007, REQ-CLA-016, REQ-CLA-026, REQ-CLA-030, REQ-CLA-032, REQ-CLA-036
**Estimated time:** 30 min

#### Context

The Claude adapter config is a JSON struct with optional fields that all have sensible defaults. The `parseConfig` function must unmarshal `adapterConfig` JSON, apply defaults for missing fields, and validate the `workingDir` (absolute path, no `..` segments). This is a standalone unit with no external dependencies.

#### RED — Write Failing Tests

Create `internal/adapter/claude/config_test.go`:

1. `TestParseConfig_Defaults` — pass `nil` raw JSON, verify `ClaudePath == "claude"`, `Model == "sonnet"`, `TimeoutSeconds == 3600`, `MaxExcerptBytes == 65536`.
2. `TestParseConfig_AllFields` — pass full JSON with all fields set, verify each field is correctly parsed.
3. `TestParseConfig_PartialFields` — pass JSON with only `model` and `maxTurns` set, verify those are parsed and all others get defaults.
4. `TestParseConfig_EmptyJSON` — pass `{}`, verify all defaults are applied.
5. `TestParseConfig_InvalidJSON` — pass malformed JSON, verify defaults are applied (no error — defensive parsing).
6. `TestValidateWorkingDir_Absolute` — absolute path without `..` is valid.
7. `TestValidateWorkingDir_Relative` — relative path returns error.
8. `TestValidateWorkingDir_DotDot` — path containing `..` returns error.
9. `TestValidateWorkingDir_Empty` — empty string is valid (uses default).

Tests fail because `internal/adapter/claude/` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/adapter/claude/config.go` with:
   - `Config` struct with all fields and JSON tags.
   - Constants: `DefaultClaudePath`, `DefaultModel`, `DefaultTimeoutSeconds`, `DefaultMaxExcerptBytes`.
   - `parseConfig(raw json.RawMessage) Config` — unmarshal + apply defaults.
   - `validateWorkingDir(dir string) error` — check absolute path, no `..` segments.
2. All nine tests pass.

#### REFACTOR

- Ensure all exported types and constants have doc comments.
- Verify the Config struct JSON tags match the schema documented in design.md.

#### Acceptance Criteria

- [ ] `Config` struct defined with all fields from design.md
- [ ] `parseConfig` applies defaults for all missing fields
- [ ] `parseConfig` handles nil, empty, partial, and malformed JSON
- [ ] `validateWorkingDir` rejects relative paths and `..` segments
- [ ] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/adapter/claude/config.go`
- **Create:** `internal/adapter/claude/config_test.go`

---

### [ ] Task 02 — Output Parser (Token Usage, Session State, Tool Calls)

**Requirements:** REQ-CLA-009, REQ-CLA-010, REQ-CLA-011, REQ-CLA-020, REQ-CLA-031
**Estimated time:** 45 min

#### Context

The Claude CLI with `--output-format json` emits structured JSON output containing token usage, session ID, and tool-call information. The parser must extract these reliably and degrade gracefully on malformed output. This is a pure-function unit with no external dependencies — testable with string inputs.

#### RED — Write Failing Tests

Create `internal/adapter/claude/parser_test.go`:

1. `TestParseClaudeOutput_FullJSON` — stdout contains a valid JSON line with `input_tokens`, `output_tokens`, `model`, and `session_id`. Verify all fields extracted into `TokenUsage` and `SessionState`.
2. `TestParseClaudeOutput_NoJSON` — stdout is plain text with no JSON. Verify zero `TokenUsage` and empty `SessionState`.
3. `TestParseClaudeOutput_MalformedJSON` — stdout contains `{broken json`. Verify zero `TokenUsage` and empty `SessionState` (REQ-CLA-031).
4. `TestParseClaudeOutput_JSONWithoutUsageFields` — stdout contains valid JSON but without token fields. Verify zero `TokenUsage`.
5. `TestParseClaudeOutput_MultipleJSONLines` — stdout has multiple JSON lines; the last one with usage fields should be used.
6. `TestParseClaudeOutput_MixedContent` — stdout has plain text lines interspersed with JSON. Verify the JSON usage line is found.
7. `TestParseLogLine_PlainText` — plain text line returns `LogLine` with `level="info"` and `Message` set.
8. `TestParseLogLine_ToolCall` — JSON line with `tool_name` field returns `LogLine` with `Fields["toolName"]` set.
9. `TestParseLogLine_ToolCallWithInput` — JSON line with `tool_name` and `tool_input` fields returns both in `Fields`.
10. `TestDetectToolCall_ToolLine` — JSON line with `tool_name` returns `"tool:<name>"`.
11. `TestDetectToolCall_NonToolLine` — plain text returns empty string.
12. `TestDetectRateLimit_429` — line containing "429" detected as rate limit.
13. `TestDetectRateLimit_RateLimitText` — line containing "rate limit" (case-insensitive) detected.
14. `TestDetectRateLimit_NormalLine` — normal stderr line not detected as rate limit.

Tests fail because `internal/adapter/claude/parser.go` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/adapter/claude/parser.go` with:
   - `claudeJSONOutput` struct for JSON deserialization.
   - `parseClaudeOutput(stdout string) (adapter.TokenUsage, string)` — scans lines in reverse for usage JSON.
   - `parseLogLine(line string) adapter.LogLine` — converts raw line to structured log line, extracting tool-call fields from JSON.
   - `detectToolCall(line string) string` — returns `"tool:<name>"` or empty.
   - `isRateLimitError(line string) bool` — checks for rate-limit indicators.
2. All fourteen tests pass.

#### REFACTOR

- Ensure JSON parsing is defensive: never panic on unexpected shapes.
- Add doc comments explaining the expected Claude CLI output format.

#### Acceptance Criteria

- [ ] `parseClaudeOutput` extracts `TokenUsage` and `SessionState` from valid JSON
- [ ] `parseClaudeOutput` returns zero values on malformed or missing JSON (no error)
- [ ] `parseLogLine` extracts `toolName`, `toolInput`, `toolResult` into `LogLine.Fields`
- [ ] `detectToolCall` returns descriptive strings for tool-call lines
- [ ] Rate-limit detection works for both "429" and "rate limit" patterns
- [ ] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/adapter/claude/parser.go`
- **Create:** `internal/adapter/claude/parser_test.go`

---

### [ ] Task 03 — ClaudeAdapter Struct, Type, Models, TestEnvironment

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

- [ ] `ClaudeAdapter` satisfies `adapter.Adapter` interface (compile-time check)
- [ ] `Type()` returns `"claude_local"`
- [ ] `Models()` returns 3 Anthropic models
- [ ] `TestEnvironment(Basic)` checks `exec.LookPath`
- [ ] `TestEnvironment(Full)` runs `claude --version`
- [ ] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/adapter/claude/claude.go`
- **Create:** `internal/adapter/claude/claude_test.go`

---

### [ ] Task 04 — Execute Method (Core Subprocess Logic)

**Requirements:** REQ-CLA-001, REQ-CLA-002, REQ-CLA-003, REQ-CLA-014, REQ-CLA-015, REQ-CLA-016, REQ-CLA-017, REQ-CLA-018, REQ-CLA-019, REQ-CLA-021, REQ-CLA-022, REQ-CLA-025, REQ-CLA-035, REQ-CLA-037, REQ-CLA-038
**Estimated time:** 90 min

#### Context

The `Execute` method is the core of the Claude adapter. It builds CLI arguments, spawns the `claude` subprocess, streams stdout/stderr, handles timeout and cancellation, and returns `InvokeResult`. This task wires together the config (Task 01), parser (Task 02), and adapter struct (Task 03). Tests use mock shell scripts (via `sh -c`) to simulate Claude CLI behavior since the real CLI may not be available in CI.

#### RED — Write Failing Tests

Add to `internal/adapter/claude/claude_test.go`:

1. `TestExecute_BuildArgs_BasicFlags` — create an input with model, system prompt, and prompt. Mock `claude` with a shell script that echoes its args. Verify `--print`, `--output-format json`, `--model sonnet`, `--system-prompt` are present.
2. `TestExecute_BuildArgs_WithResume` — set `SessionState` in input. Verify `--resume <session>` is in args.
3. `TestExecute_BuildArgs_WithMaxTurns` — set `maxTurns: 10` in config. Verify `--max-turns 10` is in args.
4. `TestExecute_BuildArgs_WithAllowedTools` — set `allowedTools: ["Read","Write"]`. Verify `--allowedTools Read,Write` is in args.
5. `TestExecute_SuccessfulRun` — mock `claude` as `sh -c 'echo {"result":"ok","session_id":"sess123","input_tokens":100,"output_tokens":50,"model":"sonnet"}'`. Verify `Status=succeeded`, `ExitCode=0`, `Usage.InputTokens=100`, `SessionState="sess123"`.
6. `TestExecute_NonZeroExit` — mock with `sh -c 'exit 1'`. Verify `Status=failed`, `ExitCode=1`.
7. `TestExecute_Timeout` — set `timeoutSeconds=1`, mock with `sleep 60`. Verify `Status=timed_out` returned within ~2s.
8. `TestExecute_ContextCancellation` — start mock `sleep 60`, cancel context after 100ms. Verify `Status=cancelled` within 500ms.
9. `TestExecute_EnvVarsInjected` — mock with `sh -c 'echo $ARI_AGENT_ID'`. Pass `ARI_AGENT_ID=test-123` in EnvVars. Verify stdout contains `test-123`.
10. `TestExecute_ApiKeyNotInArgs` — mock with script that echoes `$0 $@` (shows all args). Verify `ARI_API_KEY` does not appear in output (REQ-CLA-035).
11. `TestExecute_WorkingDirSet` — set `workingDir` to `/tmp`. Mock with `pwd`. Verify stdout contains `/tmp`.
12. `TestExecute_WorkingDirRelativeRejected` — set `workingDir` to `./relative`. Verify `Execute` returns error with `RunStatusFailed`.
13. `TestExecute_StdoutExcerptTruncated` — emit more than `maxExcerptBytes`. Verify `Stdout` is truncated.
14. `TestExecute_HooksOnLogLineCalled` — verify `hooks.OnLogLine` is called for each stdout line.
15. `TestExecute_HooksOnStatusChangeCalled` — emit a JSON tool-call line. Verify `hooks.OnStatusChange` is called with `"tool:<name>"`.
16. `TestExecute_ConcurrentExecutions` — launch 3 Execute calls simultaneously with `-race`. Verify no data races.

Tests fail because `Execute` is currently a stub.

#### GREEN — Implement Minimum to Pass

1. Replace the `Execute` stub in `internal/adapter/claude/claude.go` with the full implementation:
   - Call `parseConfig` to extract config from `input.Agent.AdapterConfig`.
   - Call `validateWorkingDir` if `cfg.WorkingDir` is non-empty.
   - Call `buildArgs` to construct CLI arguments.
   - Create `exec.CommandContext` with timeout context.
   - Set `cmd.Dir`, `cmd.Env` (via `buildEnv`), `cmd.SysProcAttr`.
   - Create stdout/stderr pipes.
   - `cmd.Start()` — return failed on error (REQ-CLA-037).
   - Launch two goroutines: `streamAndParse` for stdout, `streamStderr` for stderr.
   - Wait for both goroutines via done channel (REQ-CLA-038).
   - `cmd.Wait()`.
   - Determine `RunStatus` from context errors and exit code.
   - For cancellation: SIGTERM → 5s → SIGKILL (REQ-CLA-021).
   - Call `parseClaudeOutput` on stdout for usage and session state (REQ-CLA-010, REQ-CLA-011).
   - Build and return `InvokeResult`.
2. Create `buildArgs` method on `ClaudeAdapter`.
3. Create `buildEnv` function.
4. All sixteen tests pass with `-race`.

#### REFACTOR

- Extract the SIGTERM-then-SIGKILL pattern into a `gracefulKill(pid int, timeout time.Duration)` helper.
- Ensure the 5-second SIGKILL timer is properly cleaned up when the process exits before the timer fires.
- Verify no goroutine leaks by checking goroutine count before/after Execute.

#### Acceptance Criteria

- [ ] `Execute` spawns `claude` (or mock) with correct flags
- [ ] System prompt passed via `--system-prompt` flag
- [ ] Session resume via `--resume` flag when session state is available
- [ ] ARI_* env vars injected into subprocess environment
- [ ] ARI_API_KEY never appears in command-line arguments
- [ ] Timeout kills process group and returns `RunStatusTimedOut`
- [ ] Context cancellation sends SIGTERM, then SIGKILL after 5s
- [ ] Stdout/stderr streamed in real time via hooks
- [ ] Token usage and session state extracted from JSON output
- [ ] Working directory validation rejects relative paths and `..`
- [ ] No data races under concurrent execution (`-race`)
- [ ] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/adapter/claude/claude.go` (replace Execute stub)

---

### [ ] Task 05 — Session Resume with Fallback

**Requirements:** REQ-CLA-012, REQ-CLA-013, REQ-CLA-032
**Estimated time:** 30 min

#### Context

When a previous session state is available, the adapter passes `--resume <session_id>` to Claude. If the session is invalid or expired, Claude may return an error. The adapter must detect this and retry without `--resume`, falling back to a fresh session. The `disableResumeOnError` config option forces fresh sessions always.

#### RED — Write Failing Tests

Add to `internal/adapter/claude/claude_test.go`:

1. `TestExecute_SessionResume_Success` — set `SessionState="sess-abc"` in input, mock `claude` that checks for `--resume` flag and returns success. Verify the flag was passed and run succeeds.
2. `TestExecute_SessionResume_Fallback` — mock `claude` that fails with exit code 1 when `--resume` is passed, succeeds without it. Verify the adapter retries without `--resume` and the second attempt succeeds.
3. `TestExecute_SessionResume_FallbackLogsWarning` — same scenario as above. Verify `hooks.OnLogLine` is called with `level="warn"` and a message about session resume failure.
4. `TestExecute_DisableResumeOnError` — set `disableResumeOnError: true` in config and `SessionState="sess-abc"` in input. Verify `--resume` is NOT passed even though session state is available.
5. `TestExecute_NoSessionState_NoResumeFlag` — set empty `SessionState`. Verify `--resume` is NOT in the args.

Tests fail because the retry-on-resume-failure logic is not yet implemented.

#### GREEN — Implement Minimum to Pass

1. Modify `Execute` in `claude.go` to:
   - Track whether `--resume` was used.
   - If the first execution fails AND `--resume` was used, log a warning and retry without `--resume`.
   - Limit retry to exactly one attempt (no infinite loops).
   - Skip `--resume` entirely when `cfg.DisableResumeOnError` is true.
2. All five tests pass.

#### REFACTOR

- Extract the "run with optional retry" logic into a helper method to keep `Execute` clean.
- Ensure the retry path properly resets stdout/stderr buffers.
- Ensure the retry respects the original timeout (not double the timeout).

#### Acceptance Criteria

- [ ] `--resume` passed when session state is available and `disableResumeOnError` is false
- [ ] Failed resume triggers exactly one retry without `--resume`
- [ ] Warning logged on resume fallback
- [ ] `disableResumeOnError: true` prevents `--resume` from being added
- [ ] Retry respects original timeout context
- [ ] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/adapter/claude/claude.go`

---

### [ ] Task 06 — Adapter Registration and Server Integration

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

- [ ] `claude.New()` is registered in the adapter registry at server startup
- [ ] `TestEnvironment` is called for Claude adapter at startup
- [ ] Missing `claude` binary results in `MarkUnavailable`, not a startup crash
- [ ] `Resolve("claude_local")` returns the Claude adapter when available
- [ ] `make build` and `make test` pass
- [ ] Server starts and runs without error when `claude` is not installed

#### Files to Create / Modify

- **Modify:** Server initialization code (e.g., `cmd/ari/run.go` or equivalent where adapters are registered)
- **Modify:** `internal/adapter/claude/claude_test.go` (add registration tests)
