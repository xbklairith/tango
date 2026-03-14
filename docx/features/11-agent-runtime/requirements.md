# Requirements: Agent Runtime

**Created:** 2026-03-15
**Status:** Draft

## Overview

Implement concrete agent runtime adapters that execute agent workloads, manage run/stop lifecycle commands, and stream real-time status updates via Server-Sent Events (SSE). Currently only the adapter interface stub (`internal/adapter/adapter.go`) exists — this feature adds the full execution layer: adapter contracts, wakeup queue, heartbeat run lifecycle, atomic task checkout, real-time SSE streaming, and an agent console UI.

Ari is a **control plane**, not an execution plane. It orchestrates when and how agents are invoked; the agents themselves run externally (as subprocesses, containers, or HTTP endpoints) and call back into the Ari API to report progress.

### Requirement ID Format

- Use sequential IDs: `REQ-001`, `REQ-002`, etc.
- Numbering is continuous across all categories.

---

## Functional Requirements

### Event-Driven Requirements (WHEN...THEN)

**Agent Invocation**

- [REQ-001] WHEN a user calls `POST /api/agents/{id}/wake` THEN the system SHALL create a `WakeupRequest` record with `invocationSource=on_demand`, transition the agent status to `running`, mint a short-lived Run Token JWT (48h TTL), and dispatch execution through the registered adapter for that agent's `adapterType`.

- [REQ-002] WHEN a heartbeat timer fires for an agent THEN the system SHALL create a `WakeupRequest` with `invocationSource=timer`, deduplicate against any pending wakeup for the same agent, and invoke the adapter if no wakeup is already queued or running.

- [REQ-003] WHEN an issue is assigned to an agent (assignee changes to this agent's ID) THEN the system SHALL create a `WakeupRequest` with `invocationSource=assignment` and `ARI_WAKE_REASON=issue_assigned` so the agent wakes with the triggering issue in context.

- [REQ-004] WHEN a user resolves an inbox item that was created by an agent THEN the system SHALL create a `WakeupRequest` with `invocationSource=inbox_resolved` and inject the resolution payload as `ARI_WAKE_REASON=inbox_resolved` in the agent's environment.

- [REQ-005] WHEN a user sends a message on a conversation issue THEN the system SHALL create a `WakeupRequest` with `invocationSource=conversation_message`, inject `ARI_WAKE_REASON=conversation_message` and `ARI_CONVERSATION_ID=<issue_id>`, and pass the full comment thread as `ConversationContext` to the adapter.

**Adapter Dispatch**

- [REQ-006] WHEN the system dispatches a wakeup THEN the system SHALL create a `HeartbeatRun` record with `status=queued`, then transition it to `status=running` once the adapter accepts the invocation.

- [REQ-007] WHEN the adapter completes execution successfully THEN the system SHALL update the `HeartbeatRun` with `status=succeeded`, capture `exitCode`, `usageJson` (token counts, model, provider), `sessionIdAfter`, `stdoutExcerpt`, and `stderrExcerpt`, and transition the agent status from `running` to `idle`.

- [REQ-008] WHEN the adapter returns a non-zero exit code or an execution error THEN the system SHALL update the `HeartbeatRun` with `status=failed`, record `exitCode` and `stderrExcerpt`, transition the agent status to `error`, and create an inbox alert of `category=alert` and `type=agent_error` with the error summary.

- [REQ-009] WHEN an agent execution exceeds its configured timeout THEN the system SHALL terminate the adapter invocation, update the `HeartbeatRun` with `status=timed_out`, transition the agent to `error`, and create an inbox alert of `type=agent_error`.

- [REQ-010] WHEN the system spawns an agent THEN the system SHALL inject the following environment variables into the adapter's invocation: `ARI_API_URL`, `ARI_API_KEY` (Run Token JWT), `ARI_AGENT_ID`, `ARI_SQUAD_ID`, `ARI_RUN_ID`, `ARI_TASK_ID` (if assignment-triggered), `ARI_CONVERSATION_ID` (if conversation-triggered), and `ARI_WAKE_REASON`.

**Agent Stop**

- [REQ-011] WHEN a user calls `POST /api/agents/{id}/stop` THEN the system SHALL signal the adapter to gracefully terminate the running invocation, update the `HeartbeatRun` with `status=cancelled`, and transition the agent status to `paused`.

- [REQ-012] WHEN a user pauses an agent via `POST /api/agents/{id}/transition` with `status=paused` THEN the system SHALL prevent any new wakeup from being dispatched for that agent and add its Run Token to the server-side revocation list so in-flight API calls from the agent are rejected with 401.

**Task Checkout (CAS)**

- [REQ-013] WHEN an agent calls `POST /api/issues/{id}/checkout` THEN the system SHALL atomically acquire an execution lock on the issue using a Compare-And-Swap database transaction: update `checkout_run_id` and `execution_locked_at` only if the issue's current status matches one of `expectedStatuses` AND `checkout_run_id` IS NULL.

- [REQ-014] WHEN a task checkout succeeds THEN the system SHALL return HTTP 200 with the updated issue, set `status=in_progress`, and emit an `issue.updated` SSE event to all squad subscribers.

- [REQ-015] WHEN a task checkout fails because another agent already holds the lock THEN the system SHALL return HTTP 409 with `code=CHECKOUT_CONFLICT` — the calling agent MUST NOT retry.

- [REQ-016] WHEN a task checkout is called by an agent that already holds the lock for that issue THEN the system SHALL return HTTP 200 idempotently without modifying the record.

- [REQ-017] WHEN an agent calls `POST /api/issues/{id}/release` THEN the system SHALL atomically clear `checkout_run_id` and `execution_locked_at`, set `status=todo` (or caller-specified target status), and emit an `issue.updated` SSE event.

**Task Completion**

- [REQ-018] WHEN an agent updates an issue to `status=done` via `PATCH /api/issues/{id}` THEN the system SHALL release the checkout lock, emit an `issue.updated` SSE event, and if the issue is part of a pipeline, automatically advance to the next pipeline stage.

- [REQ-019] WHEN a pipeline's final stage is completed THEN the system SHALL set the issue `status=done`, clear the pipeline stage, and emit an `issue.updated` SSE event.

**SSE Event Emission**

- [REQ-020] WHEN an agent's status changes THEN the system SHALL emit an `agent.status.changed` SSE event to all active subscribers on the agent's squad stream (`GET /api/squads/{id}/events/stream`).

- [REQ-021] WHEN a `HeartbeatRun` transitions to `queued`, `running`, `succeeded`, or `failed` THEN the system SHALL emit the corresponding `heartbeat.run.queued`, `heartbeat.run.started`, or `heartbeat.run.finished` SSE event.

- [REQ-022] WHEN an agent posts a comment on a conversation issue THEN the system SHALL emit a `conversation.agent.replied` SSE event containing the comment body so the UI can display it without polling.

- [REQ-023] WHEN an agent begins processing a conversation message (session spawned) THEN the system SHALL emit a `conversation.agent.typing` SSE event so the UI can show a typing indicator.

- [REQ-024] WHEN a new inbox item is created THEN the system SHALL emit an `inbox.item.created` SSE event with `category`, `urgency`, and the item ID.

- [REQ-025] WHEN an agent's monthly spend reaches 80% of its budget THEN the system SHALL emit a `cost.threshold.warning` SSE event and create an inbox alert of `type=budget_warning`.

**Session Persistence**

- [REQ-026] WHEN an adapter returns a non-empty `sessionIdAfter` THEN the system SHALL persist it as the agent's current session state for the relevant context (task-scoped: `AgentTaskSession(agentId, issueId, sessionState)` or conversation-scoped: `AgentConversationSession(agentId, issueId, sessionState)`).

- [REQ-027] WHEN a subsequent wakeup is dispatched for the same agent and the same issue (or conversation) THEN the system SHALL populate `sessionIdBefore` on the `HeartbeatRun` from the most recently stored session state and pass it in `InvokeInput.Run.SessionState` so the agent can resume context.

**Agent Crashes / Errors**

- [REQ-028] WHEN the adapter process exits unexpectedly (crash, OOM, signal) THEN the system SHALL detect the abnormal termination, mark the `HeartbeatRun` as `status=failed`, transition the agent to `error`, and emit an `agent.status.changed` SSE event.

- [REQ-029] WHEN the server shuts down while an agent is running THEN the system SHALL perform a graceful drain: signal all active adapters to stop, wait up to a configurable drain timeout (default 30s), then mark any still-running `HeartbeatRun` records as `status=cancelled` in the database on the next startup.

---

### State-Driven Requirements (WHILE...the system SHALL)

- [REQ-030] WHILE an agent has `status=running`, the system SHALL keep the SSE connection alive for all subscribers on that squad's event stream and forward any structured log lines emitted by the adapter as `heartbeat.run.log` events in real time.

- [REQ-031] WHILE an SSE client is connected to `/api/squads/{id}/events/stream`, the system SHALL send a keep-alive comment line (`: ping`) every 15 seconds to prevent proxy and client timeouts.

- [REQ-032] WHILE the wakeup queue contains pending `WakeupRequest` records, the system SHALL process them in priority order (assignment > inbox_resolved > conversation_message > timer > on_demand) with a configurable maximum concurrency per squad (default: 3 simultaneous heartbeat runs per squad).

- [REQ-033] WHILE a task checkout lock is held and no `release` has been called within the agent's maximum run duration, the system SHALL treat it as a stale lock and automatically release it, transitioning the issue back to `todo` and emitting an `issue.updated` SSE event.

- [REQ-034] WHILE an agent is in `paused` or `terminated` status, the system SHALL reject any Run Token JWT issued for that agent with HTTP 401, preventing the agent from calling the Ari API even if the token has not expired.

---

### Ubiquitous Requirements (The system SHALL always)

- [REQ-035] The system SHALL route all agent execution through the `adapter.Adapter` interface — no direct subprocess or HTTP calls outside of an adapter implementation.

- [REQ-036] The system SHALL enforce the defined agent status machine for every status transition:
  - `pending_approval` → `active`
  - `active` → `idle` | `paused`
  - `idle` → `running` | `paused`
  - `running` → `idle` | `error` | `paused`
  - `paused` → `active`
  - any → `terminated` (user-initiated; terminal, no further transitions)
  - `error` → no automatic recovery (requires user intervention to resume via `active`)

- [REQ-037] The system SHALL persist agent stdout and stderr as excerpts (up to a configurable max bytes, default 64 KB each) in the `HeartbeatRun` record for later retrieval.

- [REQ-038] The system SHALL record a `CostEvent` for every completed `HeartbeatRun` that reports non-zero token usage, linking it to `agentId`, `heartbeatRunId`, `squadId`, and any issue or project context from the run.

- [REQ-039] The system SHALL maintain an adapter registry that maps `adapterType` string identifiers to registered `Adapter` implementations, enabling pluggable adapters without code changes to the core server.

- [REQ-040] The system SHALL enforce squad-level data isolation: all SSE subscribers, wakeup queues, and heartbeat runs are scoped to a single `squadId`; cross-squad events SHALL never be delivered to a subscriber.

- [REQ-041] The system SHALL generate one new Run Token JWT per adapter invocation; Run Tokens MUST NOT be reused across heartbeat runs.

---

### Conditional Requirements (IF...THEN)

- [REQ-042] IF no adapter is registered for the agent's `adapterType` THEN the system SHALL reject the wakeup request with HTTP 422 (`code=ADAPTER_NOT_FOUND`) and leave the agent in its current status.

- [REQ-043] IF an agent has `status=running` when a new wakeup is triggered THEN the system SHALL queue the new `WakeupRequest` rather than dispatching it immediately; it SHALL be dispatched after the current run completes.

- [REQ-044] IF an agent has `status=paused` or `status=terminated` when a wakeup is triggered THEN the system SHALL discard the `WakeupRequest` and not invoke the adapter.

- [REQ-045] IF an SSE client disconnects THEN the system SHALL remove the subscriber from the in-memory fan-out registry and stop writing to its response writer; no error SHALL be logged for normal disconnections.

- [REQ-046] IF a new SSE client connects THEN the system SHALL immediately send a synthetic `agent.status.changed` event for each agent in the squad so the client has a current snapshot without needing to call the REST API.

- [REQ-047] IF an agent's monthly spend has reached 100% of its `budgetMonthlyCents` THEN the system SHALL auto-pause the agent (transition to `paused`), cancel any queued wakeups, create an inbox alert of `type=budget_warning`, and emit an `agent.status.changed` SSE event.

- [REQ-048] IF a conversation already has an active agent session (a `HeartbeatRun` in `running` state for that conversation issue) THEN the system SHALL queue the new message-triggered wakeup and process it after the current session completes, maintaining message ordering.

- [REQ-049] IF an adapter's `TestEnvironment` check fails at server startup THEN the system SHALL log a warning but continue starting; adapters that fail environment checks SHALL be marked unavailable and return `REQ-042` errors until the environment is corrected.

---

## Adapter Interface Contract

The `adapter.Adapter` interface (to be expanded from the current stub) SHALL define the following contract:

```go
type Adapter interface {
    // Type returns the adapter's identifier (e.g., "claude_local", "process").
    Type() string

    // Execute spawns the agent and blocks until the run completes or context is cancelled.
    Execute(ctx context.Context, input InvokeInput, hooks Hooks) (InvokeResult, error)

    // TestEnvironment verifies the runtime is available and correctly configured.
    TestEnvironment(level TestLevel) (TestResult, error)

    // Models returns the list of AI models this adapter can use.
    Models() []ModelDefinition
}

type InvokeInput struct {
    Agent     AgentContext           // agent ID, role, config, system prompt
    Squad     SquadContext           // squad ID, name, budget state
    Run       RunContext             // run ID, sessionIdBefore, wake reason, task ID
    EnvVars   map[string]string      // ARI_* env vars to inject
    Prompt    string                 // system-generated initial prompt
    Conversation *ConversationContext // non-nil when wake_reason=conversation_message
}

type ConversationContext struct {
    IssueID      string         // conversation issue ID
    Messages     []CommentEntry // full thread (or last N if thread is long)
    SessionState string         // previous session state for this conversation
}

type InvokeResult struct {
    Status       RunStatus  // succeeded | failed | cancelled | timed_out
    ExitCode     int
    Usage        TokenUsage // InputTokens, OutputTokens, Model, Provider
    SessionState string     // sessionIdAfter — opaque state blob
    Stdout       string     // captured stdout excerpt
    Stderr       string     // captured stderr excerpt
}

type Hooks struct {
    // OnLogLine is called for each structured log line emitted during execution.
    // Implementations SHALL call this to enable real-time SSE streaming.
    OnLogLine func(line LogLine)

    // OnStatusChange is called when the adapter detects a sub-status change
    // (e.g., tool use started, waiting for approval).
    OnStatusChange func(detail string)
}
```

---

## SSE Event Format

All events on `GET /api/squads/{id}/events/stream` follow the SSE wire format:

```
event: <event-type>
data: <JSON payload>
id: <monotonic-event-id>

```

### Event Types and Payloads

| Event Type | Trigger | Key Payload Fields |
|---|---|---|
| `agent.status.changed` | Agent status transition | `agentId`, `from`, `to`, `runId?` |
| `heartbeat.run.queued` | WakeupRequest dispatched | `runId`, `agentId`, `invocationSource` |
| `heartbeat.run.started` | Adapter accepts invocation | `runId`, `agentId`, `startedAt` |
| `heartbeat.run.finished` | Adapter execution complete | `runId`, `agentId`, `status`, `exitCode`, `finishedAt` |
| `heartbeat.run.log` | Real-time log line from adapter | `runId`, `agentId`, `level`, `message`, `timestamp` |
| `issue.updated` | Issue fields changed | `issueId`, `identifier`, `changes` (field diff) |
| `conversation.agent.typing` | Agent session spawned for conversation | `conversationId`, `agentId` |
| `conversation.agent.replied` | Agent posted comment | `conversationId`, `agentId`, `commentId` |
| `inbox.item.created` | New inbox item | `itemId`, `category`, `urgency`, `title` |
| `inbox.item.resolved` | User resolved inbox item | `itemId`, `resolvedByUserId`, `resolution` |
| `cost.threshold.warning` | Budget 80% or 100% reached | `agentId`, `thresholdPct`, `spentCents`, `budgetCents` |
| `activity.appended` | New activity log entry | `actorType`, `actorId`, `action`, `entityType`, `entityId` |

Keep-alive:
```
: ping

```
(sent every 15 seconds on idle connections; no `event:` or `data:` line)

---

## Task Checkout Flow

```
Agent                           Ari Server                    Database
  |                                 |                              |
  |  POST /issues/{id}/checkout     |                              |
  |  { agentId, expectedStatuses,   |                              |
  |    runId }                      |                              |
  |-------------------------------->|                              |
  |                                 |  BEGIN TRANSACTION           |
  |                                 |  SELECT ... FOR UPDATE       |
  |                                 |  WHERE id = {id}             |
  |                                 |------------------------------>|
  |                                 |  <- row locked               |
  |                                 |                              |
  |                                 |  IF status NOT IN expected   |
  |                                 |     OR checkout_run_id != NULL|
  |                                 |  THEN ROLLBACK → 409         |
  |                                 |                              |
  |                                 |  UPDATE issues SET           |
  |                                 |    status = 'in_progress',   |
  |                                 |    checkout_run_id = {runId},|
  |                                 |    execution_locked_at = NOW()|
  |                                 |  RETURNING *                 |
  |                                 |------------------------------>|
  |                                 |  COMMIT                      |
  |                                 |                              |
  |  200 OK { issue }               |  EMIT issue.updated SSE      |
  |<--------------------------------|                              |
  |                                 |                              |
  | ... agent works ...             |                              |
  |                                 |                              |
  |  POST /issues/{id}/release      |                              |
  |  { targetStatus }               |                              |
  |-------------------------------->|                              |
  |                                 |  UPDATE issues SET           |
  |                                 |    checkout_run_id = NULL,   |
  |                                 |    execution_locked_at = NULL,|
  |                                 |    status = {targetStatus}   |
  |                                 |------------------------------>|
  |  200 OK { issue }               |  EMIT issue.updated SSE      |
  |<--------------------------------|                              |
```

- Checkout is **atomic**: uses `SELECT ... FOR UPDATE` inside a transaction.
- The `runId` in the checkout request MUST match the agent's current `HeartbeatRun.id`.
- A 409 response means the task is owned by another agent. The calling agent MUST NOT retry.
- Stale locks (checkout held past max run duration with no release) are cleared by a background sweep.

---

## Non-Functional Requirements

### Performance

- [REQ-050] The system SHALL support at least 50 concurrent SSE subscriber connections per squad without degrading event delivery latency beyond 500ms.

- [REQ-051] The system SHALL dispatch a wakeup request (from trigger event to adapter `Execute()` call) within 2 seconds under normal load.

- [REQ-052] The system SHALL complete a task checkout transaction within 100ms for 99% of requests under normal PostgreSQL load.

### Security

- [REQ-053] The system SHALL mint a new HS256 JWT Run Token for every adapter invocation; tokens MUST include `sub` (agentId), `squad_id`, `run_id`, `role`, and `exp` (48h from issuance).

- [REQ-054] The system SHALL maintain a server-side revocation list of Run Tokens for paused and terminated agents; revoked tokens SHALL be rejected with HTTP 401 on every API call regardless of signature validity.

- [REQ-055] The system SHALL scope all SSE streams to a single squad: subscribers MUST be authenticated (valid user session or agent Run Token for `authenticated` deployment mode) and MUST only receive events for their own squad.

- [REQ-056] The system SHALL validate that the `agentId` in a checkout request matches the agent identity extracted from the caller's Run Token — an agent CANNOT check out tasks on behalf of another agent.

### Reliability

- [REQ-057] The system SHALL perform an at-startup audit of `HeartbeatRun` records in `queued` or `running` state and mark them `cancelled` if the server was not cleanly shut down, preventing ghost locks.

- [REQ-058] The system SHALL persist `WakeupRequest` records to the database before dispatching to the adapter so that requests survive server restarts and can be replayed.

- [REQ-059] SSE event delivery SHALL be best-effort: a slow or disconnected subscriber MUST NOT block event delivery to other subscribers on the same squad stream.

---

## Constraints

- All agent execution MUST go through the `adapter.Adapter` interface — no direct subprocess or HTTP calls outside an adapter.
- Agent status transitions MUST follow the state machine defined in `internal/domain/agent.go` (`ValidateStatusTransition`).
- Task checkout MUST use a database-level `SELECT ... FOR UPDATE` transaction (CAS pattern) — no application-level mutex.
- SSE endpoint MUST support multiple concurrent subscribers per squad (fan-out).
- Run Tokens MUST NOT be reused across heartbeat runs.
- The `process` adapter MUST NOT be used with untrusted agent configs without explicit sandbox configuration.

---

## Acceptance Criteria

- [ ] The `adapter.Adapter` interface is fully defined with `Execute`, `TestEnvironment`, `Models`, and `Hooks`.
- [ ] At least one concrete adapter is implemented: `process` (shell command subprocess).
- [ ] `POST /api/agents/{id}/wake` starts the agent, creates a `HeartbeatRun`, and transitions agent to `running`.
- [ ] `POST /api/agents/{id}/stop` gracefully terminates the running invocation and transitions agent to `paused`.
- [ ] Agent status transitions emit `agent.status.changed` SSE events in real time.
- [ ] `GET /api/squads/{id}/events/stream` serves SSE with keep-alive pings every 15s.
- [ ] SSE initial snapshot delivers current agent statuses on connect.
- [ ] `POST /api/issues/{id}/checkout` atomically locks a task; returns 409 on conflict.
- [ ] `POST /api/issues/{id}/release` releases the checkout lock.
- [ ] Run Token JWT is injected as `ARI_API_KEY`; all `ARI_*` env vars are present in the spawned process.
- [ ] Session state (`sessionIdAfter`) is persisted and restored on subsequent runs.
- [ ] `HeartbeatRun` records are marked `cancelled` on startup if left in `running` state.
- [ ] Budget auto-pause fires when spend reaches 100% of `budgetMonthlyCents`.
- [ ] Agent console UI streams real-time log lines from the running heartbeat.

---

## Out of Scope

- Multi-node distributed execution (single-host only for v1).
- Custom adapter plugin system via dynamic linking or external process.
- Agent scaling or autoscaling.
- Container orchestration (Kubernetes, ECS).
- Parallel pipeline stages (sequential only in v1).
- Streaming partial agent output mid-turn in conversations (agent posts complete reply as a comment).

---

## Dependencies

- Agent status machine: `internal/domain/agent.go` (`ValidateStatusTransition`, status constants).
- Adapter interface stub: `internal/adapter/adapter.go` (to be expanded per REQ-035).
- Agent handler: `internal/server/handlers/agent_handler.go` (existing CRUD and transition endpoints).
- Issue handler: `internal/server/handlers/issue_handler.go` (checkout/release endpoints to be added).
- Database issues queries: `internal/database/queries/issues.sql` (checkout CAS query to be added).
- Cost events (feature 10): `CostEvent` records linked to `HeartbeatRun`.
- SSE infrastructure: new `internal/server/sse/` package (no existing implementation).

---

## Risks & Assumptions

**Assumptions:**
- Agents run as subprocesses on the same host for v1 (the `process` adapter covers this).
- SSE is sufficient for real-time updates; WebSocket is not needed.
- PostgreSQL `SELECT ... FOR UPDATE` provides sufficient task checkout atomicity without application-level locking.
- Agent output is stored as DB text excerpts (not a log file system) for v1.

**Risks:**
- Long-running agent subprocesses require proper cleanup on server restart; the startup audit (REQ-057) mitigates stale DB state but OS-level orphaned processes may need `pgid`-based process group kill.
- Subprocess execution has security implications (arbitrary command execution); the `process` adapter MUST document sandboxing requirements clearly.
- SSE fan-out at scale (50+ concurrent connections) requires careful goroutine and channel management to avoid head-of-line blocking (REQ-059).
- Token revocation list grows unboundedly without a cleanup sweep; expiry-based eviction is needed.

---

## References

- PRD: `docx/core/01-PRODUCT.md` (sections 5.1, 7, 8, 10.2)
- Adapter interface stub: `internal/adapter/adapter.go`
- Agent domain model: `internal/domain/agent.go`
- Agent handler: `internal/server/handlers/agent_handler.go`
- Issues SQL queries: `internal/database/queries/issues.sql`
