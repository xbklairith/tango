# Requirements: Claude Adapter v2

**Created:** 2026-03-17
**Status:** Draft

## Overview

Enhance Ari's Claude adapter by porting proven patterns from Paperclip's `claude-local` adapter. The current adapter works but has gaps: prompts are passed as positional args (size-limited), sessions fail to resume (cwd mismatch), agents can't have per-agent instruction files or skills, and several useful Claude CLI flags are missing. This feature closes those gaps while preserving Ari-specific strengths (secrets injection, budget limits, conversations, SSE logging).

## Research Summary

Compared Paperclip's `claude-local` adapter (`packages/adapters/claude-local/src/server/execute.ts`) with Ari's (`internal/adapter/claude/claude.go`). Key findings:
- Paperclip delivers prompts via stdin (`--print -`), avoiding arg length limits
- Paperclip uses `--append-system-prompt-file` for rich per-agent instruction markdown files
- Paperclip injects shared skills via `--add-dir` with symlinked `.claude/skills/`
- Paperclip validates session cwd before resuming, prevents context confusion
- Paperclip detects "unknown session" errors and max-turns exhaustion for smart retry
- Paperclip strips Claude Code nesting env vars to prevent recursive spawning
- Paperclip supports `--effort`, `--chrome`, `--max-turns`, and `extraArgs` passthrough

## Functional Requirements

### Event-Driven Requirements

- [REQ-001] WHEN the adapter executes a Claude CLI invocation THEN the system SHALL deliver the prompt via stdin using `--print -` instead of positional arguments
- [REQ-002] WHEN an agent has an `instructionsFilePath` configured in adapterConfig THEN the system SHALL inject that file via `--append-system-prompt-file`
- [REQ-003] WHEN the adapter builds CLI arguments THEN the system SHALL include `--add-dir` pointing to Ari's skills directory containing shared API knowledge (heartbeat procedure, endpoints)
- [REQ-004] WHEN resuming a session THEN the system SHALL validate that the session's saved cwd matches the current invocation cwd before passing `--resume`
- [REQ-005] WHEN a session resume fails with "no conversation found with session id" or "unknown session" error THEN the system SHALL retry once without `--resume` (fresh session). This REPLACES the current unconditional retry-on-failure behavior; only session-specific errors trigger a retry, not arbitrary failures
- [REQ-006] WHEN a run completes with `error_max_turns` or `stop_reason=max_turns` THEN the system SHALL clear the saved session state so the next run starts fresh
- [REQ-007] WHEN building the process environment THEN the system SHALL strip all env vars matching the prefix `CLAUDE_CODE_*` plus the single `CLAUDECODE` var to prevent nested Claude Code sessions. Prefix-based stripping future-proofs against new vars added by Claude CLI
- [REQ-008] WHEN `effort` is configured in adapterConfig THEN the system SHALL pass `--effort <low|medium|high>` to the Claude CLI
- [REQ-009] WHEN `chrome` is set to true in adapterConfig THEN the system SHALL pass `--chrome` to enable browser automation
- [REQ-010] WHEN `extraArgs` is configured as a string array in adapterConfig THEN the system SHALL append those arguments to the Claude CLI invocation
- [REQ-011] WHEN a Claude CLI run fails due to authentication issues (login required) THEN the system SHALL detect the error via pattern matching and return a structured `loginRequired` error with the login URL if available
- [REQ-012] WHEN parsing stream-json usage events THEN the system SHALL extract and persist `cache_read_input_tokens` alongside existing input/output token counts. This requires adding a `CachedInputTokens int` field to the `adapter.TokenUsage` struct
- [REQ-013] WHEN `maxTurnsPerRun` is configured in adapterConfig THEN the system SHALL pass `--max-turns <N>` to limit conversation depth per invocation

### State-Driven Requirements

- [REQ-014] WHILE building the invocation environment THEN the system SHALL resolve the working directory using priority: task workspace cwd > agent config workingDir > server cwd
- [REQ-015] WHILE persisting session state after a run THEN the system SHALL store `{sessionId, cwd}` as structured JSON instead of an opaque string, enabling cwd validation on resume

### Ubiquitous Requirements

- [REQ-016] The system SHALL always pass `--verbose` to the Claude CLI for richer stream-json event output. Note: `--verbose` compatibility with stream-json output should be manually verified against the target Claude CLI version before implementation
- [REQ-017] The system SHALL support configurable prompt templates per agent via `promptTemplate` in adapterConfig, with `{{agent.name}}`, `{{agent.role}}`, `{{squad.name}}`, `{{run.id}}` variables
- [REQ-018] The system SHALL redact environment variable values matching sensitive patterns (KEY, TOKEN, SECRET, PASSWORD, AUTHORIZATION) when logging adapter metadata

### Conditional Requirements

- [REQ-019] IF `workingDir` is not configured and the task has an associated project with a workspace cwd THEN the system SHALL use the project workspace cwd as the Claude CLI working directory. Workspace cwd resolution occurs in the run handler (not the adapter); the resolved cwd is passed to the adapter via a new `WorkingDir` field on `adapter.InvokeInput`
- [REQ-020] IF `instructionsFilePath` is a relative path THEN the system SHALL resolve it relative to the agent's configured `workingDir`

### Additional Event-Driven Requirements

- [REQ-029] WHEN `instructionsFilePath` is configured THEN the system SHALL merge the agent's existing system prompt into a temporary combined file and use only `--append-system-prompt-file`, avoiding the mutual exclusion conflict with `--append-system-prompt`
- [REQ-030] WHEN delivering the prompt via stdin THEN the system SHALL write to the process stdin pipe in a separate goroutine to prevent deadlocks with large prompts
- [REQ-031] WHEN parsing session state from the database THEN the system SHALL handle legacy bare-string session IDs (not JSON) by treating them as `{sessionId: <string>, cwd: ""}` for backward compatibility

## Non-Functional Requirements

### Performance

- [REQ-021] The system SHALL not add more than 50ms overhead to invocation startup from the new features (stdin delivery, env stripping, cwd resolution)

### Security

- [REQ-022] The system SHALL continue to enforce the existing env var blocklist (PATH, HOME, SHELL, USER, LD_PRELOAD, LD_LIBRARY_PATH, ARI_*, DYLD_*) for adapter config env vars
- [REQ-023] The system SHALL validate that `instructionsFilePath` does not contain path traversal segments (`..`)
- [REQ-024] The system SHALL not expose sensitive env var values in log output or SSE events

### Reliability

- [REQ-025] The system SHALL gracefully handle missing skills directory (skip `--add-dir` with a warning log)
- [REQ-026] The system SHALL gracefully handle missing instructions file (skip `--append-system-prompt-file` with a warning log)

### Maintainability

- [REQ-027] The system SHALL maintain existing test coverage for the claude adapter (all existing tests pass)
- [REQ-028] The system SHALL add unit tests for each new feature: stdin delivery, cwd validation, session detection, nesting prevention, login detection

## Constraints

- Must not break existing adapter interface (`adapter.Adapter`)
- Must not change the `Config` struct in a backward-incompatible way (new fields are optional with defaults)
- Must preserve Ari-specific features not in Paperclip: `--max-budget-usd`, `--allowedTools`, secrets injection, conversation support, SSE log streaming
- Skills directory structure must follow Claude Code conventions (`.claude/skills/{name}/SKILL.md`)

## Acceptance Criteria

- [ ] Prompt delivered via stdin; no positional arg for prompt text
- [ ] Agent with `instructionsFilePath` has instructions injected via `--append-system-prompt-file`
- [ ] Skills directory created with Ari API skill and injected via `--add-dir`
- [ ] Session resume skipped when cwd doesn't match; logs warning
- [ ] "Unknown session" error triggers automatic retry without `--resume`
- [ ] Max-turns result clears session state
- [ ] Claude Code nesting env vars stripped from child process
- [ ] `--effort`, `--chrome`, `--max-turns` flags passed when configured
- [ ] `extraArgs` appended to CLI invocation
- [ ] Login required errors detected and surfaced
- [ ] `cache_read_input_tokens` extracted and stored
- [ ] `--verbose` always passed
- [ ] Prompt template renders agent/squad/run variables
- [ ] Sensitive env vars redacted in logs
- [ ] Workspace cwd resolved from project when available
- [ ] Session state persisted as `{sessionId, cwd}` JSON, not opaque string
- [ ] Legacy bare-string session IDs parsed without error (backward compatibility)
- [ ] Stdin writes use separate goroutine to prevent deadlocks
- [ ] Merged instructions file used when both system prompt and instructionsFilePath are present
- [ ] All existing adapter tests pass
- [ ] New tests cover each feature

## Out of Scope

- Multi-adapter routing (using different adapters per task)
- MCP server injection into Claude CLI
- Git workspace cloning (Paperclip's workspace repo clone feature)
- Billing type detection (API vs subscription)
- OpenClaw gateway integration
- Agent config UI for new fields (separate feature)

## Dependencies

- Feature 20: Claude Adapter (existing implementation)
- Feature 11: Agent Runtime (adapter interface, run service)
- Feature 06: Projects & Goals (workspace cwd resolution). Graceful degradation: if Feature 06 is not yet implemented, workspace cwd resolution (REQ-014, REQ-019) falls back to agent config `workingDir` or server cwd. The adapter must not fail if project/workspace data is unavailable

## Risks & Assumptions

**Assumptions:**
- Claude CLI supports `--print -` for stdin prompt delivery in current version
- Claude CLI `--verbose` flag produces richer stream-json without breaking existing parsing
- Skills directory symlink approach works on macOS and Linux

**Risks:**
- `--append-system-prompt-file` and `--append-system-prompt` may be mutually exclusive in Claude CLI — mitigated by REQ-029 which merges into a single file
- Large instruction files may slow down Claude CLI startup
- Workspace cwd resolution requires project-issue relationship which may not always exist
- Temp file cleanup: the merged instructions file (REQ-029) and skills directory (REQ-003) must be reliably cleaned up after each invocation. Use `defer os.RemoveAll()` and handle cleanup on process signals to prevent accumulation of temp files

## References

- Paperclip claude-local adapter: `/Users/xb/builder/paperclip/packages/adapters/claude-local/src/server/execute.ts`
- Paperclip skill: `/Users/xb/builder/paperclip/skills/paperclip/SKILL.md`
- Ari claude adapter: `/Users/xb/builder/ari/internal/adapter/claude/claude.go`
- Ari run handler: `/Users/xb/builder/ari/internal/server/handlers/run_handler.go`
