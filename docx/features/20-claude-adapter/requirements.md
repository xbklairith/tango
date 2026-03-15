# Requirements: Claude Adapter

**Created:** 2026-03-15
**Status:** Draft

## Overview

Implement the `claude_local` adapter that integrates Claude Code CLI as a first-class agent runtime for Ari. This is the flagship adapter — it spawns `claude` as a subprocess, injects system prompts and task context, parses structured output for real-time tool-call streaming, extracts cost/token usage, and supports session continuity via Claude's `--resume` flag.

The Claude adapter implements the `adapter.Adapter` interface defined in Feature 11 (Agent Runtime) and follows the same subprocess pattern as the `process` adapter, with Claude-specific enhancements: model selection, structured output parsing, cost extraction, allowed-tools configuration, and max-turns limits.

### Requirement ID Format

- Use sequential IDs: `REQ-CLA-001`, `REQ-CLA-002`, etc.
- Numbering is continuous across all categories.

---

## Functional Requirements

### Event-Driven Requirements (WHEN...THEN)

**Claude CLI Invocation**

- [REQ-CLA-001] WHEN the adapter's `Execute` method is called THEN the system SHALL spawn the `claude` CLI binary as a subprocess using `exec.CommandContext`, passing the task prompt via the `--print` flag (non-interactive mode) and the model via the `--model` flag.

- [REQ-CLA-002] WHEN the agent has a non-empty `systemPrompt` in its configuration THEN the system SHALL pass it to the Claude CLI via the `--system-prompt` flag so that Claude operates under the agent's persona and instructions.

- [REQ-CLA-003] WHEN the adapter builds the subprocess environment THEN the system SHALL inject the task prompt as the `ARI_PROMPT` environment variable AND pass it as the positional prompt argument to the `claude` CLI so the agent has full task context.

- [REQ-CLA-004] WHEN the agent's `adapterConfig` specifies a `model` field THEN the system SHALL pass it to the `claude` CLI via the `--model` flag. Supported values SHALL include `opus`, `sonnet`, and `haiku`. IF no model is specified THEN the system SHALL default to `sonnet`.

- [REQ-CLA-005] WHEN the agent's `adapterConfig` specifies a `maxTurns` value THEN the system SHALL pass it to the `claude` CLI via the `--max-turns` flag to limit the number of agentic turns per invocation.

- [REQ-CLA-006] WHEN the agent's `adapterConfig` specifies an `allowedTools` array THEN the system SHALL pass each tool to the `claude` CLI via the `--allowedTools` flag to restrict which tools the agent may use (e.g., `Read`, `Write`, `Edit`, `Bash`, `Grep`).

- [REQ-CLA-007] WHEN the agent's `adapterConfig` specifies a `workingDir` value THEN the system SHALL set the subprocess working directory to that path. IF no `workingDir` is specified THEN the system SHALL use the current working directory of the Ari process.

**Output Parsing**

- [REQ-CLA-008] WHEN the Claude CLI emits a line to stdout THEN the system SHALL parse it for structured content and forward it as a `LogLine` via `hooks.OnLogLine` for real-time SSE streaming to the agent console UI.

- [REQ-CLA-009] WHEN a stdout line contains a tool invocation pattern (e.g., tool name, input, and output markers from Claude's `--output-format json` output) THEN the system SHALL extract the tool name, input summary, and result into structured `LogLine.Fields` with keys `toolName`, `toolInput`, and `toolResult` so the console UI can render tool calls distinctly.

- [REQ-CLA-010] WHEN the Claude CLI emits JSON output (via `--output-format json`) THEN the system SHALL parse the JSON to extract `input_tokens`, `output_tokens`, and `model` fields into `InvokeResult.Usage` for cost accounting.

- [REQ-CLA-011] WHEN the Claude CLI's JSON output contains a `session_id` field THEN the system SHALL capture it as `InvokeResult.SessionState` so subsequent runs can resume the conversation context.

**Session Continuity**

- [REQ-CLA-012] WHEN the `InvokeInput.Run.SessionState` contains a non-empty session ID from a previous run THEN the system SHALL pass it to the `claude` CLI via the `--resume` flag to restore the previous conversation context.

- [REQ-CLA-013] WHEN a session resume is requested but the Claude CLI returns an error indicating the session is invalid or expired THEN the system SHALL fall back to a fresh session (no `--resume` flag) and log a warning, rather than failing the entire run.

**Environment Variables**

- [REQ-CLA-014] WHEN the adapter spawns the Claude CLI subprocess THEN the system SHALL inject all `ARI_*` environment variables from `InvokeInput.EnvVars` into the subprocess environment, including `ARI_API_URL`, `ARI_API_KEY`, `ARI_AGENT_ID`, `ARI_SQUAD_ID`, `ARI_RUN_ID`, `ARI_TASK_ID`, `ARI_WAKE_REASON`, and `ARI_PROMPT`.

- [REQ-CLA-015] WHEN the agent's `adapterConfig` specifies additional environment variables in an `env` map THEN the system SHALL merge them into the subprocess environment (adapter-config env vars take precedence over defaults but ARI_* vars always win).

**Timeout Handling**

- [REQ-CLA-016] WHEN the agent's `adapterConfig` specifies a `timeoutSeconds` value THEN the system SHALL enforce it as a context deadline on the subprocess. IF no timeout is specified THEN the system SHALL default to 3600 seconds (1 hour).

- [REQ-CLA-017] WHEN the context deadline is exceeded THEN the system SHALL kill the Claude CLI process group via `SIGKILL`, return `InvokeResult{Status: RunStatusTimedOut}`, and capture whatever stdout/stderr was produced before the timeout.

**Error Handling**

- [REQ-CLA-018] WHEN the Claude CLI exits with a non-zero exit code THEN the system SHALL return `InvokeResult{Status: RunStatusFailed}` with the exit code and stderr excerpt, enabling the run service to create an inbox alert.

- [REQ-CLA-019] WHEN the Claude CLI process crashes (signal, OOM, unexpected termination) THEN the system SHALL detect the abnormal exit via `exec.ExitError`, capture the signal information in stderr, and return `InvokeResult{Status: RunStatusFailed}`.

- [REQ-CLA-020] WHEN the Claude CLI emits rate-limit error messages (HTTP 429 or "rate limit" in stderr) THEN the system SHALL log a warning via `hooks.OnLogLine` with `level=warn` and include `rateLimited: true` in `LogLine.Fields` so the UI can display a rate-limit indicator.

- [REQ-CLA-021] WHEN the context is cancelled externally (graceful stop via `POST /api/agents/{id}/stop`) THEN the system SHALL send `SIGTERM` to the Claude CLI process group, wait up to 5 seconds for graceful shutdown, then `SIGKILL` if still running, and return `InvokeResult{Status: RunStatusCancelled}`.

---

### State-Driven Requirements (WHILE...the system SHALL)

- [REQ-CLA-022] WHILE the Claude CLI subprocess is running, the system SHALL continuously read stdout and stderr line by line, forwarding each line to `hooks.OnLogLine` in real time for SSE streaming, and accumulating both into bounded buffers (up to `maxExcerptBytes`, default 64 KB).

- [REQ-CLA-023] WHILE the Claude CLI subprocess is running and emitting tool-call output, the system SHALL call `hooks.OnStatusChange` with descriptive strings (e.g., `"tool:Read /path/to/file"`, `"tool:Bash executing"`, `"thinking"`) so the UI can show what the agent is currently doing.

---

### Ubiquitous Requirements (The system SHALL always)

- [REQ-CLA-024] The system SHALL register the Claude adapter in the adapter registry under the type identifier `"claude_local"` at server startup.

- [REQ-CLA-025] The system SHALL use `syscall.SysProcAttr{Setpgid: true}` when spawning the Claude CLI process to ensure all child processes can be killed as a group on timeout or cancellation.

- [REQ-CLA-026] The system SHALL capture stdout and stderr excerpts up to a configurable `maxExcerptBytes` (default 65536 / 64 KB) in the `InvokeResult`, consistent with the process adapter and REQ-037 from Feature 11.

- [REQ-CLA-027] The system SHALL pass `--output-format json` to the Claude CLI to ensure structured output that can be reliably parsed for token usage, session IDs, and tool-call information.

---

### Conditional Requirements (IF...THEN)

- [REQ-CLA-028] IF the `claude` binary is not found in `$PATH` (or the configured `claudePath`) during `TestEnvironment` THEN the system SHALL return `TestResult{Available: false, Message: "claude CLI not found at <path>"}` and the registry SHALL mark the adapter unavailable per REQ-049.

- [REQ-CLA-029] IF `TestEnvironment` is called with `TestLevelFull` THEN the system SHALL execute `claude --version` to verify the CLI is functional and parse the version string. IF the version is below the minimum supported version THEN it SHALL return `Available: false` with a descriptive message.

- [REQ-CLA-030] IF the agent's `adapterConfig` contains an `claudePath` override THEN the system SHALL use that path instead of searching `$PATH` for the `claude` binary.

- [REQ-CLA-031] IF the `--output-format json` output cannot be parsed (malformed JSON) THEN the system SHALL return `InvokeResult` with zero-valued `Usage` and empty `SessionState`, logging a warning. The run SHALL NOT fail due to parse errors in output metadata.

- [REQ-CLA-032] IF the agent's `adapterConfig` specifies `disableResumeOnError: true` THEN the system SHALL NOT attempt session resume even if `SessionState` is available, forcing a fresh session every run.

---

## Non-Functional Requirements

### Performance

- [REQ-CLA-033] The system SHALL forward stdout log lines to `hooks.OnLogLine` with less than 100ms latency from the time the line is emitted by the Claude CLI process.

- [REQ-CLA-034] The adapter SHALL support concurrent invocations (multiple agents using `claude_local` simultaneously) without shared mutable state — each `Execute` call is fully independent.

### Security

- [REQ-CLA-035] The system SHALL NOT pass the agent's `ARI_API_KEY` (Run Token) in command-line arguments — it SHALL only be injected as an environment variable to prevent exposure via `/proc/cmdline` or `ps` output.

- [REQ-CLA-036] The system SHALL sanitize the `workingDir` path to prevent directory traversal attacks — the path MUST be absolute and MUST NOT contain `..` segments.

### Reliability

- [REQ-CLA-037] The system SHALL handle the case where the Claude CLI binary is updated or removed between `TestEnvironment` (startup) and `Execute` (runtime) by returning a clear error in `Execute` rather than panicking.

- [REQ-CLA-038] The system SHALL ensure both stdout and stderr reader goroutines complete before `Execute` returns, preventing goroutine leaks and data races on the output buffers.

---

## Constraints

- The Claude adapter MUST implement the `adapter.Adapter` interface exactly as defined in `internal/adapter/adapter.go`.
- The Claude adapter MUST follow the same subprocess management patterns as the `process` adapter (process groups, signal handling, bounded output capture).
- The Claude CLI MUST be invoked in non-interactive mode (`--print` flag) — interactive TTY mode is not supported.
- Session state is an opaque string (Claude's session ID); the adapter MUST NOT interpret or modify it.
- The adapter MUST NOT store any state between `Execute` calls — all state flows through `InvokeInput` and `InvokeResult`.

---

## Acceptance Criteria

- [ ] `ClaudeAdapter` implements `adapter.Adapter` interface and is registered as `"claude_local"`.
- [ ] `Execute` spawns `claude` CLI with `--print`, `--model`, `--output-format json`, and `--system-prompt` flags.
- [ ] `ARI_*` environment variables are injected into the subprocess.
- [ ] `ARI_PROMPT` contains the full task prompt and is passed as the CLI prompt argument.
- [ ] Stdout is parsed line-by-line and forwarded to `hooks.OnLogLine` in real time.
- [ ] Tool-call patterns are extracted into structured `LogLine.Fields`.
- [ ] Token usage (input/output tokens, model) is extracted from JSON output.
- [ ] Session ID is extracted and returned as `SessionState`.
- [ ] `--resume` flag is passed when `SessionState` is available from a previous run.
- [ ] `--max-turns` and `--allowedTools` flags are passed when configured.
- [ ] Timeout kills the process group and returns `RunStatusTimedOut`.
- [ ] External cancellation sends `SIGTERM` then `SIGKILL` and returns `RunStatusCancelled`.
- [ ] `TestEnvironment(TestLevelBasic)` checks for `claude` binary in `$PATH`.
- [ ] `TestEnvironment(TestLevelFull)` runs `claude --version` and validates it.
- [ ] `Models()` returns Opus, Sonnet, and Haiku model definitions.
- [ ] Rate-limit errors in stderr are detected and logged with `rateLimited` field.
- [ ] All tests pass with `-race` flag.

---

## Out of Scope

- Claude API direct integration (HTTP to Anthropic API) — this adapter uses the CLI only.
- MCP (Model Context Protocol) server configuration — future feature.
- Custom tool definitions beyond Claude's built-in tools.
- Streaming partial responses mid-turn to conversations (agent posts complete reply).
- Multi-model orchestration within a single run (e.g., Haiku for planning, Opus for coding).
- CLAUDE.md file generation or management — system prompt is passed via flag.

---

## Dependencies

- Adapter interface: `internal/adapter/adapter.go` (Feature 11, implemented)
- Adapter registry: `internal/adapter/registry.go` (Feature 11, implemented)
- Process adapter: `internal/adapter/process/process.go` (Feature 11, pattern reference)
- Run service: `internal/server/handlers/run_handler.go` (invokes adapters)
- SSE hub: `internal/server/sse/` (log line streaming)
- Claude Code CLI: external binary, must be installed on the host

---

## Risks & Assumptions

**Assumptions:**
- Claude Code CLI is installed on the host and available in `$PATH` (or a configured path).
- Claude Code CLI supports `--print`, `--model`, `--system-prompt`, `--resume`, `--max-turns`, `--allowedTools`, and `--output-format json` flags.
- Claude Code CLI's JSON output format is stable and includes `input_tokens`, `output_tokens`, `model`, and `session_id` fields.
- The Claude CLI handles its own API key authentication (via `ANTHROPIC_API_KEY` env var or its own config) — Ari does not manage Anthropic API keys.

**Risks:**
- Claude CLI output format may change between versions, breaking the parser. Mitigation: version check in `TestEnvironment`, defensive parsing that degrades gracefully (REQ-CLA-031).
- Rate limits from Anthropic may cause long blocking waits inside `Execute`. Mitigation: timeout enforcement (REQ-CLA-016) and rate-limit detection (REQ-CLA-020).
- Large Claude outputs (e.g., file contents in tool results) may exceed the excerpt buffer. Mitigation: bounded buffer with truncation (REQ-CLA-026).
- Session IDs may become invalid between runs (e.g., Claude server-side expiry). Mitigation: fallback to fresh session (REQ-CLA-013).

---

## References

- PRD: `docx/core/01-PRODUCT.md` (section 8: Adapter System, `claude_local` in table 8.2)
- Adapter interface: `internal/adapter/adapter.go`
- Adapter registry: `internal/adapter/registry.go`
- Process adapter (pattern reference): `internal/adapter/process/process.go`
- Run service (adapter invocation): `internal/server/handlers/run_handler.go`
- Agent self-service API: `internal/server/handlers/agent_self_handler.go`
- Feature 11 requirements: `docx/features/11-agent-runtime/requirements.md`
- Feature 11 design: `docx/features/11-agent-runtime/design.md`
