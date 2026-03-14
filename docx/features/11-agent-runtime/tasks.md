# Tasks: Agent Runtime

**Created:** 2026-03-15
**Status:** Ready for Implementation

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-001 through REQ-059

## Implementation Approach

Work bottom-up through the dependency graph: database schema first, then the adapter layer, then runtime services (token, SSE hub, wakeup queue, run service), then HTTP handlers, and finally server wiring. Each layer's tests stand alone using mocks or an embedded test DB.

## Progress Summary

- Total Tasks: 10
- Completed: 0
- In Progress: None
- Test Coverage: TBD

---

## Tasks (TDD: Red-Green-Refactor)

---

### Task 01 — Database Migration: Runtime Tables

**Requirements:** REQ-006, REQ-013, REQ-026, REQ-027, REQ-057, REQ-058
**Estimated time:** 45 min

#### Context

All runtime state lives in four new tables (`wakeup_requests`, `heartbeat_runs`, `agent_task_sessions`, `agent_conversation_sessions`) and two new columns on `issues` (`checkout_run_id`, `execution_locked_at`). The migration must be idempotent via `-- +goose Down` and follow the existing naming convention.

#### RED — Write Failing Tests

Write `internal/database/database_test.go` assertions (or a migration smoke test) that verify:

1. After `RunMigrations()`, the table `wakeup_requests` exists and has columns: `id`, `squad_id`, `agent_id`, `invocation_source`, `status`, `context_json`, `created_at`, `dispatched_at`, `discarded_at`.
2. The table `heartbeat_runs` exists with all specified columns including `usage_json JSONB` and `session_id_before / session_id_after TEXT`.
3. The table `issues` has columns `checkout_run_id UUID` and `execution_locked_at TIMESTAMPTZ`.
4. The tables `agent_task_sessions` and `agent_conversation_sessions` exist with the `uq_task_session` and `uq_conversation_session` unique constraints.
5. The partial unique index `uq_wakeup_pending_per_agent` on `wakeup_requests(agent_id) WHERE status = 'pending'` prevents duplicate pending wakeups for the same agent.
6. `DROP TABLE heartbeat_runs` cascade removes the FK from `issues.checkout_run_id` (verify `-- +goose Down` is clean).

Tests fail because the migration file does not yet exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/database/migrations/20260315000011_create_runtime_tables.sql` containing the full `-- +goose Up` and `-- +goose Down` blocks exactly as specified in the design:
   - `wakeup_invocation_source` ENUM type
   - `wakeup_request_status` ENUM type
   - `wakeup_requests` table + partial unique index + pending index
   - `heartbeat_run_status` ENUM type
   - `heartbeat_runs` table + three indexes (agent, squad, active partial)
   - `ALTER TABLE issues ADD COLUMN checkout_run_id ... ADD COLUMN execution_locked_at ...` + index
   - `agent_task_sessions` table + unique constraint
   - `agent_conversation_sessions` table + unique constraint
2. Verify `make sqlc` still succeeds (no new queries yet; just schema).
3. Run `make test` — migration smoke tests pass.

#### REFACTOR

- Confirm the `-- +goose Down` block drops objects in correct dependency order (child tables before parent, columns before types).
- Add a comment block at the top of the migration documenting the feature it supports.

#### Acceptance Criteria

- [ ] Migration file exists at `internal/database/migrations/20260315000011_create_runtime_tables.sql`
- [ ] All five new DB objects are created by `-- +goose Up`
- [ ] `-- +goose Down` cleanly reverses all changes (verified by running up then down)
- [ ] `make test` passes with no new failures
- [ ] `make sqlc` succeeds

#### Files to Create / Modify

- **Create:** `internal/database/migrations/20260315000011_create_runtime_tables.sql`

---

### Task 02 — Adapter Interface Expansion + Registry

**Requirements:** REQ-035, REQ-039, REQ-042, REQ-049
**Estimated time:** 45 min

#### Context

Replace the current stub in `internal/adapter/adapter.go` with the full interface (`Execute`, `TestEnvironment`, `Models`), all supporting types (`InvokeInput`, `InvokeResult`, `Hooks`, `LogLine`, `AgentContext`, `SquadContext`, `RunContext`, `ConversationContext`, `TokenUsage`, `ModelDefinition`, `RunStatus`, `TestLevel`, `TestResult`), and the `Registry` type with `Register`, `Resolve`, and `MarkUnavailable`.

#### RED — Write Failing Tests

Create `internal/adapter/registry_test.go`:

1. `TestRegistry_RegisterAndResolve` — register a stub adapter, call `Resolve(type)`, verify non-nil and correct `Type()` returned.
2. `TestRegistry_ResolveUnknown` — `Resolve("nonexistent")` returns an error; verify error message contains "no adapter registered".
3. `TestRegistry_MarkUnavailable` — register an adapter, call `MarkUnavailable(type, "reason")`, then `Resolve(type)` returns an error containing "unavailable".
4. `TestRegistry_ConcurrentReads` — launch 50 goroutines all calling `Resolve()` simultaneously; verify no data race (run with `-race`).

Tests fail because `Registry` does not exist in the current stub.

#### GREEN — Implement Minimum to Pass

1. Replace `internal/adapter/adapter.go` entirely with the expanded interface and all types from the design.
2. Create `internal/adapter/registry.go` implementing `Registry` with `sync.RWMutex`-protected `adapters` and `unavail` maps.
3. All four registry tests pass. No other test regressions (run `make test`).

#### REFACTOR

- Ensure all exported types have doc comments.
- Verify `adapter.go` has no import cycles (it imports only `context`, `time`, `github.com/google/uuid`).
- Move `Registry` to `registry.go` if not already separate.

#### Acceptance Criteria

- [ ] `adapter.Adapter` interface defines `Execute`, `TestEnvironment`, `Models`
- [ ] All supporting types defined: `InvokeInput`, `InvokeResult`, `Hooks`, `LogLine`, `AgentContext`, `SquadContext`, `RunContext`, `ConversationContext`, `TokenUsage`, `ModelDefinition`
- [ ] `Registry.Register`, `Registry.Resolve`, `Registry.MarkUnavailable` implemented
- [ ] `TestRegistry_ConcurrentReads` passes with `-race`
- [ ] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/adapter/adapter.go` (full replacement)
- **Create:** `internal/adapter/registry.go`
- **Create:** `internal/adapter/registry_test.go`

---

### Task 03 — Process Adapter

**Requirements:** REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-028, REQ-037
**Estimated time:** 60 min

#### Context

The `process` adapter spawns a shell command as a subprocess, injects `ARI_*` env vars, captures stdout/stderr excerpts (default 64 KB each), enforces run timeout, and extracts session state from the last JSON line of stdout (`{"ari_session_state": "<blob>"}`). It uses `syscall.SysProcAttr{Setpgid: true}` to kill all child processes on cancellation.

#### RED — Write Failing Tests

Create `internal/adapter/process/process_test.go`:

1. `TestProcessAdapter_SuccessfulRun` — execute `echo hello`, verify `Status=RunStatusSucceeded`, `ExitCode=0`, `Stdout` contains `"hello"`.
2. `TestProcessAdapter_NonZeroExit` — execute `exit 42` via `sh -c`, verify `Status=RunStatusFailed`, `ExitCode=42`.
3. `TestProcessAdapter_ContextCancellation` — start `sleep 60`, cancel context after 50ms, verify `Status=RunStatusCancelled` returned within 200ms and the process is no longer running.
4. `TestProcessAdapter_Timeout` — configure `TimeoutSeconds=1`, execute `sleep 10`, verify `Status=RunStatusTimedOut` after ~1s.
5. `TestProcessAdapter_SessionStateExtraction` — stdout ends with `{"ari_session_state":"abc123"}`, verify `InvokeResult.SessionState == "abc123"`.
6. `TestProcessAdapter_EnvVarsInjected` — execute `sh -c 'echo $ARI_AGENT_ID'`, pass `EnvVars["ARI_AGENT_ID"]="test-id"`, verify stdout contains `"test-id"`.
7. `TestProcessAdapter_StdoutExcerptTruncated` — emit more than `MaxExcerptBytes` of output, verify `Stdout` is truncated to exactly `MaxExcerptBytes`.
8. `TestProcessAdapter_Type` — verify `Type() == "process"`.
9. `TestProcessAdapter_TestEnvironment` — verify returns `Available=true`.

Tests fail because `internal/adapter/process/` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/adapter/process/process.go` with:
   - `Config` struct (Command, Args, WorkingDir, TimeoutSeconds, MaxExcerptBytes)
   - `ProcessAdapter` struct implementing `adapter.Adapter`
   - `New()` constructor
   - `Execute()`: parse config from `input.Agent.AdapterConfig`, apply defaults, build `exec.Cmd`, set `SysProcAttr{Setpgid: true}`, inject env vars, stream stdout/stderr with goroutines into bounded buffers, call `cmd.Wait()`, determine `RunStatus` from context errors and exit code, extract session state.
   - `streamLines()` helper: reads lines from a pipe, appends to buffer (up to max), calls `hooks.OnLogLine` if non-nil.
   - `extractSessionState()` helper: scan stdout lines in reverse for last valid `{"ari_session_state":...}` JSON object.
   - `truncate()` helper.
   - `TestEnvironment()`: always returns `Available=true`.
   - `Models()`: returns nil.
2. All nine tests pass.

#### REFACTOR

- Extract env injection into a `envMapToSlice(map[string]string) []string` helper.
- Ensure `streamLines` goroutines are always waited on before `Execute` returns (to avoid stdout/stderr race).
- Document the security note: command must be an absolute path to a trusted binary.

#### Acceptance Criteria

- [ ] `ProcessAdapter` satisfies `adapter.Adapter` interface
- [ ] `Type()` returns `"process"`
- [ ] Context cancellation kills the process group and returns `RunStatusCancelled`
- [ ] Timeout fires before context cancellation and returns `RunStatusTimedOut`
- [ ] Session state is extracted from last JSON line in stdout
- [ ] Stdout/stderr truncated to `MaxExcerptBytes` (default 65536)
- [ ] `make test` passes with `-race`

#### Files to Create / Modify

- **Create:** `internal/adapter/process/process.go`
- **Create:** `internal/adapter/process/process_test.go`

---

### Task 04 — Run Token JWT (Mint, Validate, Revoke)

**Requirements:** REQ-012, REQ-034, REQ-041, REQ-053, REQ-054
**Estimated time:** 45 min

#### Context

`RunTokenService` mints HS256 JWTs with claims `sub` (agentId), `squad_id`, `run_id`, `role`, `typ="run_token"`, and `exp` (48h). It maintains a `sync.Map` revocation list keyed by `runID`. A background goroutine sweeps expired entries hourly. The auth middleware must be extended to accept Run Token `Authorization: Bearer` headers, validate revocation, and inject an `AgentIdentity` into the request context.

#### RED — Write Failing Tests

Create `internal/runtime/token/token_test.go`:

1. `TestRunTokenService_MintAndValidate` — mint a token for known IDs, validate it, verify all claims (`sub`, `squad_id`, `run_id`, `role`, `typ`, `exp` within 48h window).
2. `TestRunTokenService_WrongTypRejected` — mint a regular user JWT using the same signing key but without `typ=run_token`, pass to `Validate()`, expect error.
3. `TestRunTokenService_ExpiredTokenRejected` — mint a token with `exp` in the past (using a helper that allows custom expiry), verify `Validate()` returns error.
4. `TestRunTokenService_RevokeAndIsRevoked` — mint, call `Revoke(runID)`, verify `IsRevoked(runID)` returns `true`.
5. `TestRunTokenService_NotRevokedByDefault` — new run ID, verify `IsRevoked` returns `false`.
6. `TestRunTokenService_SweepExpiredRevocations` — revoke two run IDs, one with an already-expired token, call sweep, verify the expired entry is removed and the valid one remains.

Tests fail because `internal/runtime/token/` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/runtime/token/token.go`:
   - `RunTokenClaims` struct embedding `jwt.RegisteredClaims` with `SquadID`, `RunID`, `Role`, `TokenType` fields.
   - `RunTokenService` struct with `signingKey []byte` and `revoked sync.Map`.
   - `NewRunTokenService(signingKey []byte) *RunTokenService`
   - `Mint(agentID, squadID, runID uuid.UUID, role string) (string, error)` — builds claims with 48h TTL, signs with HS256.
   - `Validate(tokenString string) (*RunTokenClaims, error)` — parses, verifies signature, checks `TokenType == "run_token"`, returns parsed claims.
   - `Revoke(runID uuid.UUID)` — stores `{runID.String(): claims}` (store claims for sweep use) in `sync.Map`.
   - `IsRevoked(runID uuid.UUID) bool`.
   - `SweepExpiredRevocations()` — ranges `sync.Map`, deletes entries where stored expiry < now.
   - (background goroutine for hourly sweep to be wired in Task 10)
2. All six tests pass.

#### REFACTOR

- Store the full `RunTokenClaims` (or just expiry time) in the revocation map — not just a marker — so `SweepExpiredRevocations` can inspect the expiry without re-parsing the JWT string.
- Add a `WithClock` option or an injectable `now()` function to make sweep tests deterministic.

#### Acceptance Criteria

- [ ] `Mint` produces a valid HS256 JWT with all required claims
- [ ] `Validate` rejects expired tokens, wrong signing key, wrong `typ`
- [ ] `Revoke` + `IsRevoked` round-trip works
- [ ] `SweepExpiredRevocations` removes entries with `exp < now()` only
- [ ] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/runtime/token/token.go`
- **Create:** `internal/runtime/token/token_test.go`

---

### Task 05 — SSE Hub (Fan-out, Keep-alive, Initial Snapshot)

**Requirements:** REQ-020 through REQ-025, REQ-030, REQ-031, REQ-040, REQ-045, REQ-046, REQ-050, REQ-055, REQ-059
**Estimated time:** 60 min

#### Context

`sse.Hub` maintains a `map[squadID]map[*Subscriber]struct{}` protected by `sync.RWMutex`. `Publish` is non-blocking — if a subscriber's buffered channel (size 64) is full, the event is silently dropped for that subscriber only (REQ-059). `SSEHandler.Stream` sets SSE headers, disables the HTTP write deadline via `http.ResponseController`, sends an initial agent status snapshot, and sends a `: ping` keep-alive every 15 seconds.

#### RED — Write Failing Tests

**Hub unit tests** — create `internal/server/sse/hub_test.go`:

1. `TestHub_SubscribeAndReceive` — subscribe to a squad, publish an event, verify it arrives on `sub.Ch` within 10ms.
2. `TestHub_SlowSubscriberDoesNotBlock` — subscribe two clients to the same squad, fill one channel to capacity, publish one more event, verify the fast subscriber still receives it promptly (no deadlock).
3. `TestHub_UnsubscribeClosesChannel` — subscribe, unsubscribe, verify `sub.Ch` is closed.
4. `TestHub_CrossSquadIsolation` — subscribe client A to squad-1, subscribe client B to squad-2, publish to squad-1 only, verify client B receives nothing.
5. `TestHub_MonotonicEventIDs` — publish three events, verify each `Event.ID` is strictly increasing.
6. `TestHub_UnsubscribeUnknownNoOp` — call `Unsubscribe` on a subscriber that was already removed; verify no panic.

**SSE handler integration test** — extend `internal/server/handlers/sse_integration_test.go` (new file, follows existing handler test patterns):

7. `TestSSEHandler_InitialAgentSnapshot` — connect an SSE client to `/api/squads/{id}/events/stream`, verify first SSE event has `event: agent.status.changed` for each agent in the squad.
8. `TestSSEHandler_KeepAlivePing` — use a short ticker override (e.g., 100ms), verify a `: ping` comment line is written within 200ms.
9. `TestSSEHandler_EventDeliveredAfterConnect` — connect SSE client, publish an event to the hub, verify the event arrives in the stream.
10. `TestSSEHandler_DisconnectNoError` — cancel request context, verify no error logged and goroutines cleaned up.

Tests fail because `internal/server/sse/` does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/server/sse/hub.go`:
   - `Event{ID int64, Type string, Data any}`, `Subscriber{SquadID uuid.UUID, Ch chan Event}`.
   - `Hub` with `sync.RWMutex`, `subscribers map[uuid.UUID]map[*Subscriber]struct{}`, `atomic.Int64 counter`.
   - `NewHub()`, `Subscribe(squadID)`, `Unsubscribe(s)`, `Publish(squadID, eventType, data)`.
   - `Publish` uses `select { case sub.Ch <- evt: default: }` for non-blocking send.
2. Create `internal/server/handlers/sse_handler.go`:
   - `SSEHandler{hub *sse.Hub, queries *db.Queries}`.
   - `NewSSEHandler(queries, hub)`.
   - `Stream(w, r)`: parse `squadID` from path, authenticate (verify squad membership), set SSE headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`), disable write deadline with `http.NewResponseController(w).SetWriteDeadline(time.Time{})`, subscribe to hub, call `sendAgentSnapshot`, start select loop with 15s keep-alive ticker.
   - `sendAgentSnapshot(w, flusher, squadID)`: query all agents for the squad, emit one `agent.status.changed` event per agent.
   - `writeSSEEvent(w, flusher, evt)`: writes `id: %d\nevent: %s\ndata: %s\n\n` format.
3. Register route in `internal/server/routes.go`: `GET /api/squads/{id}/events/stream`.
4. All tests pass.

#### REFACTOR

- Extract the `writeSSEEvent` helper into `internal/server/sse/write.go` for reuse by any handler.
- Ensure `Unsubscribe` uses a deferred call in `Stream` to guarantee cleanup on any exit path.
- Add a `SubscriberCount(squadID)` method to `Hub` to support future monitoring.

#### Acceptance Criteria

- [ ] `Hub.Publish` is non-blocking; slow subscriber never blocks other subscribers
- [ ] `Hub.Unsubscribe` closes the subscriber's channel
- [ ] Events are scoped per squad; no cross-squad leakage
- [ ] `Stream` sends `: ping` every 15 seconds
- [ ] `Stream` sends initial `agent.status.changed` snapshot on connect
- [ ] Client disconnect (context cancelled) logs no error and cleans up subscriber
- [ ] Write deadline disabled for SSE connections via `http.ResponseController`
- [ ] `make test` passes with `-race`

#### Files to Create / Modify

- **Create:** `internal/server/sse/hub.go`
- **Create:** `internal/server/sse/hub_test.go`
- **Create:** `internal/server/handlers/sse_handler.go`
- **Create:** `internal/server/handlers/sse_integration_test.go`
- **Modify:** `internal/server/routes.go` (add SSE route)

---

### Task 06 — Wakeup Queue (WakeupService + WakeupProcessor)

**Requirements:** REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-032, REQ-043, REQ-044, REQ-051, REQ-058
**Estimated time:** 60 min

#### Context

`WakeupService.Enqueue` inserts a `wakeup_requests` row with `INSERT ... ON CONFLICT DO NOTHING` (deduplication via the partial unique index). `WakeupProcessor` runs a 500ms poll loop that counts active `HeartbeatRun` records per squad, enforces `maxPerSquad` (default 3), pulls the next `pending` request in priority order (`assignment=0 > inbox_resolved=1 > conversation_message=2 > timer=3 > on_demand=4`), marks it `dispatched`, validates the agent is not `paused`/`terminated`, and calls `RunService.Invoke()` in a goroutine.

#### RED — Write Failing Tests

Add SQL queries first (needed by the tests). Write failing compilation-level tests by referencing non-existent types, then fill them in.

**`internal/runtime/wakeup/service_test.go`:**

1. `TestWakeupService_Enqueue` — enqueue a request, verify a row exists in `wakeup_requests` with `status=pending`.
2. `TestWakeupService_DeduplicationOnConflict` — enqueue twice for the same agent, verify only one row exists (second insert silently ignored).
3. `TestWakeupService_EnqueueDifferentAgents` — two agents in same squad each get their own pending row.

**`internal/runtime/wakeup/processor_test.go`:**

4. `TestProcessor_PriorityOrdering` — insert two pending wakeups for the same squad: one `on_demand` and one `assignment`; run one dispatch cycle; verify the `assignment` row is dispatched first (mock `RunService` records which was called first).
5. `TestProcessor_ConcurrencyLimit` — insert four pending wakeups for a squad; configure `maxPerSquad=3`; stub `HeartbeatRun` count at 3; run one dispatch cycle; verify zero new dispatches (limit respected).
6. `TestProcessor_PausedAgentDiscarded` — insert a pending wakeup for a `paused` agent; run dispatch; verify the wakeup is marked `discarded` and `RunService.Invoke` is NOT called.
7. `TestProcessor_TerminatedAgentDiscarded` — same as above for `terminated` agent.
8. `TestProcessor_DispatchedAfterCurrentRunCompletes` — agent is `running` (has a running `HeartbeatRun`); a new pending wakeup exists; concurrency limit not reached; verify dispatch still happens (queued while running is allowed per REQ-043 — the pending wakeup is kept for next cycle when the slot frees).

Tests fail because neither package exists.

#### GREEN — Implement Minimum to Pass

1. Create `internal/database/queries/wakeup_requests.sql` with named queries:
   - `EnqueueWakeupRequest :exec` — `INSERT ... ON CONFLICT DO NOTHING`
   - `NextPendingWakeupForSquad :one` — selects next pending by priority order (use `CASE WHEN invocation_source = 'assignment' THEN 0 ... END` for ordering)
   - `MarkWakeupDispatched :exec`, `MarkWakeupDiscarded :exec`
   - `CountActiveRunsForSquad :one` — count `heartbeat_runs` with `status IN ('queued','running')` for a squad
   - `GetPendingWakeupsForSquad :many` — used by processor to scan
2. Run `make sqlc` to generate Go code.
3. Create `internal/runtime/wakeup/service.go`:
   - `Service{queries *db.Queries}`, `NewService(queries)`.
   - `Enqueue(ctx, agentID, squadID uuid.UUID, source string, ctxJSON map[string]any) error`.
4. Create `internal/runtime/wakeup/processor.go`:
   - `Processor{db, queries, registry, sseHub, runSvc, maxPerSquad, pollInterval}`.
   - `NewProcessor(...)`, `Start(ctx)`, `pollLoop(ctx)`, `dispatch(ctx)`.
   - `dispatch` loads distinct squads with pending wakeups, checks concurrency per squad, resolves the adapter (discard if `ADAPTER_NOT_FOUND`), loads agent status (discard if paused/terminated), marks dispatched, calls `runSvc.Invoke` in a goroutine.
5. All eight tests pass.

#### REFACTOR

- Move priority mapping `var wakeupPriority = map[string]int{...}` to a named constant block in the `wakeup` package.
- The `dispatch` method should not return errors — log them internally with `slog`; callers must not depend on per-dispatch errors.
- Extract a `resolveAndValidate` helper that performs adapter lookup + agent status check and returns `(shouldDiscard bool, err error)`.

#### Acceptance Criteria

- [ ] `Enqueue` is idempotent: double-enqueue for same agent produces one row
- [ ] Priority order: `assignment` dispatched before `on_demand` in same squad
- [ ] Concurrency cap: no dispatch when `activeRuns >= maxPerSquad`
- [ ] `paused` and `terminated` agents have their wakeups discarded, not dispatched
- [ ] `make sqlc` succeeds with new queries
- [ ] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/database/queries/wakeup_requests.sql`
- **Create:** `internal/runtime/wakeup/service.go`
- **Create:** `internal/runtime/wakeup/processor.go`
- **Create:** `internal/runtime/wakeup/service_test.go`
- **Create:** `internal/runtime/wakeup/processor_test.go`
- **Modify:** `internal/database/queries/heartbeat_runs.sql` (CountActiveRunsForSquad — create new file if needed)
- **Run:** `make sqlc`

---

### Task 07 — HeartbeatRun Lifecycle + RunService

**Requirements:** REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-028, REQ-029, REQ-038, REQ-047, REQ-057
**Estimated time:** 60 min

#### Context

`RunService.Invoke` is the core orchestration function. It:
1. Resolves adapter; returns `ADAPTER_NOT_FOUND` if missing.
2. Loads agent, squad, and session state from DB.
3. Builds `InvokeInput` with all `ARI_*` env vars.
4. Inserts `HeartbeatRun` (status=queued), emits `heartbeat.run.queued` SSE.
5. Mints Run Token JWT, updates run to `running`, emits `heartbeat.run.started` SSE.
6. Updates agent to `running`, emits `agent.status.changed` SSE.
7. Calls `adapter.Execute` (via `safeExecute` panic-recovery wrapper), forwarding log lines as `heartbeat.run.log` SSE.
8. Finalizes: writes result, upserts session state, inserts `CostEvent`, transitions agent status, emits SSE.

`RunService.Stop` cancels the run context (transitions agent to `paused`), revokes the Run Token.
`RunService.DrainAll` cancels all active contexts and waits via `sync.WaitGroup`.
Startup audit function `auditStaleRuns` marks any `queued`/`running` runs as `cancelled`.

#### RED — Write Failing Tests

Create `internal/runtime/run_service_test.go` using the embedded test DB and a mock adapter:

1. `TestRunService_FullInvokeCycle` — mock adapter returns `RunStatusSucceeded`; verify: `HeartbeatRun` created with `status=queued`, then `running`, then `succeeded`; agent transitions `idle→running→idle`; three SSE events emitted (`heartbeat.run.queued`, `heartbeat.run.started`, `heartbeat.run.finished`).
2. `TestRunService_AdapterNotFound` — no adapter registered for agent's type; verify `Invoke` returns error with `ADAPTER_NOT_FOUND`, agent stays `idle`, no `HeartbeatRun` created.
3. `TestRunService_FailedRun` — mock adapter returns `RunStatusFailed`, exit code 1; verify `HeartbeatRun.Status=failed`, agent transitions to `error`, inbox alert created with `type=agent_error`.
4. `TestRunService_TimedOutRun` — mock adapter returns `RunStatusTimedOut`; verify `HeartbeatRun.Status=timed_out`, agent to `error`, inbox alert created.
5. `TestRunService_StopCancelsRun` — start a long-running mock adapter (blocks until context cancelled); call `Stop(agentID)`; verify `HeartbeatRun.Status=cancelled`, agent to `paused`, Run Token revoked.
6. `TestRunService_SessionStateRoundtrip` — mock adapter returns `SessionState="state-abc"`; verify it is upserted in `agent_task_sessions`; on a second Invoke for the same agent+issue, verify `InvokeInput.Run.SessionState == "state-abc"`.
7. `TestRunService_CostEventRecorded` — mock adapter returns `Usage{InputTokens:100, OutputTokens:50}`; verify a `cost_events` row is inserted linked to `agentID`, `heartbeatRunID`, `squadID`.
8. `TestRunService_BudgetAutoPause` — set agent `budget_monthly_cents=1`; mock cost event pushes spend over 100%; verify agent auto-paused, `agent.status.changed` SSE emitted, inbox alert created.
9. `TestRunService_PanicRecovery` — mock adapter panics; verify `HeartbeatRun.Status=failed`, agent to `error`, no server crash.
10. `TestRunService_AuditStaleRuns` — insert `HeartbeatRun` in `running` status, insert agent in `running`; call `auditStaleRuns`; verify run becomes `cancelled`, agent becomes `error`.
11. `TestRunService_EnvVarsInjected` — capture `InvokeInput.EnvVars` in mock adapter; verify `ARI_API_URL`, `ARI_API_KEY`, `ARI_AGENT_ID`, `ARI_SQUAD_ID`, `ARI_RUN_ID`, `ARI_WAKE_REASON` are all present.
12. `TestRunService_DrainAll` — start two concurrent invocations with blocking mock adapters; call `DrainAll(ctx)`; verify both complete and `sync.WaitGroup` is unblocked.

Tests fail because `internal/runtime/` package does not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/database/queries/heartbeat_runs.sql` with:
   - `CreateHeartbeatRun :one`, `UpdateHeartbeatRunStatus :exec`, `GetHeartbeatRun :one`
   - `CancelStaleRuns :execrows`, `ResetStaleRunningAgents :exec`
   - `GetRunningHeartbeatRunForAgent :one` (for Stop)
2. Create `internal/database/queries/sessions.sql` with:
   - `UpsertTaskSession :exec`, `GetTaskSession :one`
   - `UpsertConversationSession :exec`, `GetConversationSession :one`
3. Run `make sqlc`.
4. Create `internal/runtime/run_service.go`:
   - `RunService{db, queries, registry, tokenSvc, sseHub, cfg, mu sync.Mutex, active map[uuid.UUID]context.CancelFunc, wg sync.WaitGroup}`.
   - `NewRunService(...)`.
   - `Invoke(ctx, req WakeupRequest) error` — implement the 13-step sequence.
   - `safeExecute(ctx, adapter, input, hooks)` — panic-recovery wrapper.
   - `Stop(ctx, agentID) error`.
   - `DrainAll(ctx)`.
   - `checkBudget(ctx, agentID)` — 80% warning, 100% hard-pause.
   - `createInboxAlert(ctx, agentID, alertType, summary string)`.
   - `auditStaleRuns(ctx, queries)` — standalone function.
5. All twelve tests pass.

#### REFACTOR

- Extract `buildInvokeInput(agent, squad, run, wakeupReq, sessionState) InvokeInput` as a pure function (easy to test in isolation).
- Extract `buildEnvVars(input InvokeInput, token string, apiURL string) map[string]string` for clear env injection.
- Ensure `active` map cleanup (delete entry) happens in a `defer` inside the goroutine, not only in the success path.

#### Acceptance Criteria

- [ ] `Invoke` follows the 13-step sequence: queued → running → finalize
- [ ] `safeExecute` catches panics and returns `RunStatusFailed`
- [ ] `Stop` cancels the run, transitions agent to `paused`, revokes Run Token
- [ ] `DrainAll` signals all active runs and waits for completion
- [ ] Session state is loaded before and persisted after each run
- [ ] `CostEvent` inserted for every run with non-zero token usage
- [ ] Budget auto-pause fires at 100% spend
- [ ] `auditStaleRuns` marks orphaned runs `cancelled` and agents `error`
- [ ] `make test` passes with `-race`

#### Files to Create / Modify

- **Create:** `internal/database/queries/heartbeat_runs.sql`
- **Create:** `internal/database/queries/sessions.sql`
- **Create:** `internal/runtime/run_service.go`
- **Create:** `internal/runtime/run_service_test.go`
- **Run:** `make sqlc`

---

### Task 08 — CAS Task Checkout and Release Endpoints

**Requirements:** REQ-013, REQ-014, REQ-015, REQ-016, REQ-017, REQ-033, REQ-052, REQ-056
**Estimated time:** 60 min

#### Context

`POST /api/issues/{id}/checkout` uses `SELECT ... FOR UPDATE` inside a transaction to atomically acquire the lock. The caller must be an agent (Run Token) and the `agentId` in the body must match the token's `sub` claim. `POST /api/issues/{id}/release` clears the lock using a conditional `UPDATE ... WHERE checkout_run_id = $runId`. A stale lock sweep background goroutine in `RunService` clears locks held longer than `StaleCheckoutAge` (default 2h).

#### RED — Write Failing Tests

Create `internal/server/handlers/checkout_integration_test.go` (follows existing `issue_integration_test.go` pattern, but uses a Run Token for auth):

1. `TestCheckoutIssue_HappyPath` — issue in `todo` status; checkout with valid Run Token; verify `200`, issue `status=in_progress`, `checkout_run_id` set, `issue.updated` SSE emitted.
2. `TestCheckoutIssue_ConflictWithOtherRun` — two agents, first checks out; second calls checkout; verify `409` with `code=CHECKOUT_CONFLICT`.
3. `TestCheckoutIssue_IdempotentReacquire` — same run calls checkout twice; verify `200` both times, no data change on second call.
4. `TestCheckoutIssue_WrongStatus` — issue status not in `expectedStatuses`; verify `422`.
5. `TestCheckoutIssue_AgentIdMismatch` — token `sub` does not match body `agentId`; verify `403`.
6. `TestCheckoutIssue_UserTokenRejected` — caller has a user session (not a Run Token); verify `403`.
7. `TestReleaseIssue_HappyPath` — checkout then release with `targetStatus=done`; verify `200`, `checkout_run_id=NULL`, `execution_locked_at=NULL`, `status=done`, `issue.updated` SSE.
8. `TestReleaseIssue_NotLockOwner` — different run ID in release body; verify `409`.
9. `TestStaleLockSweep` — insert issue with `checkout_run_id` set and `execution_locked_at` more than `StaleCheckoutAge` ago; run sweep; verify lock cleared, `status=todo`, `issue.updated` SSE emitted.

Tests fail because the handlers and SQL queries do not exist.

#### GREEN — Implement Minimum to Pass

1. Add to `internal/database/queries/issues.sql`:
   - Comments explaining that checkout is implemented as a raw transaction in the handler, not a sqlc named query, due to dynamic `expectedStatuses`.
   - `ReleaseIssue :one` — `UPDATE issues SET checkout_run_id=NULL, execution_locked_at=NULL, status=$2 WHERE id=$1 AND checkout_run_id=$3 RETURNING *`.
   - `ClearStaleCheckouts :many` — returns `(id, squad_id)` pairs for each stale lock released.
2. Run `make sqlc`.
3. Create `internal/server/handlers/checkout_handler.go` (or add methods to `IssueHandler`):
   - `CheckoutIssue(w, r)`:
     - Extract Run Token from context (agent identity); reject with `403` if caller is a user.
     - Parse and validate request body (`agentId`, `runId`, `expectedStatuses`).
     - Verify `agentId == tokenClaims.Sub` (REQ-056).
     - Open a `db.BeginTx(...)` transaction.
     - `SELECT id, status, checkout_run_id, squad_id FROM issues WHERE id = $1 FOR UPDATE`.
     - Check status in `expectedStatuses`; if not, rollback, return `422`.
     - If `checkout_run_id == runId`: commit, return `200` idempotent.
     - If `checkout_run_id != NULL`: rollback, return `409 CHECKOUT_CONFLICT`.
     - `UPDATE issues SET status='in_progress', checkout_run_id=$runId, execution_locked_at=now() RETURNING *`.
     - Commit, publish `issue.updated` SSE, return `200`.
   - `ReleaseIssue(w, r)`:
     - Extract Run Token.
     - Parse body (`runId`, `targetStatus`).
     - Call `queries.ReleaseIssue(...)`.
     - If 0 rows affected: return `409`.
     - Publish `issue.updated` SSE, return `200`.
4. Add stale lock sweep to `RunService`: a goroutine started in `NewRunService` that ticks every 5 minutes, calls `queries.ClearStaleCheckouts(ctx, staleAge)`, emits `issue.updated` SSE for each returned row.
5. Register routes: `mux.HandleFunc("POST /api/issues/{id}/checkout", checkoutHandler.CheckoutIssue)` and `POST /api/issues/{id}/release`.
6. All nine tests pass.

#### REFACTOR

- Extract the transaction logic inside `CheckoutIssue` into a `checkoutIssue(ctx, tx, issueID, runID, expectedStatuses)` helper that returns a typed result (acquired, idempotent, conflict, precondition_failed) so the HTTP handler stays thin.
- Ensure the stale sweep logs each cleared issue ID at `slog.Info` level.

#### Acceptance Criteria

- [ ] Checkout uses `SELECT ... FOR UPDATE` inside a DB transaction
- [ ] Returns `409 CHECKOUT_CONFLICT` when another run holds the lock
- [ ] Returns `200` idempotently when same run calls checkout again
- [ ] `agentId` in body must match Run Token `sub`; mismatch returns `403`
- [ ] User tokens (non-Run Token) return `403`
- [ ] Release clears lock only if caller holds it; otherwise `409`
- [ ] Stale lock sweep runs in background and emits `issue.updated` SSE
- [ ] `make test` passes

#### Files to Create / Modify

- **Modify:** `internal/database/queries/issues.sql` (add ReleaseIssue, ClearStaleCheckouts)
- **Create:** `internal/server/handlers/checkout_handler.go`
- **Create:** `internal/server/handlers/checkout_integration_test.go`
- **Modify:** `internal/runtime/run_service.go` (add stale lock sweep goroutine)
- **Modify:** `internal/server/routes.go` (add checkout/release routes)
- **Run:** `make sqlc`

---

### Task 09 — Wake / Stop HTTP Endpoints + Auth Middleware Extension

**Requirements:** REQ-001, REQ-011, REQ-012, REQ-034, REQ-036, REQ-042, REQ-044
**Estimated time:** 45 min

#### Context

`POST /api/agents/{id}/wake` calls `WakeupService.Enqueue` and returns `{runId, agentId, status}`. It validates the agent exists and is not `paused`/`terminated`. `POST /api/agents/{id}/stop` calls `RunService.Stop`. The auth middleware must be extended to handle `Bearer <run_token>` in addition to user session cookies, inject `AgentIdentity` into context, check revocation list, and reject tokens for paused/terminated agents.

#### RED — Write Failing Tests

**Handler integration tests** — extend `internal/server/handlers/agent_integration_test.go` or create `internal/server/handlers/runtime_integration_test.go`:

1. `TestWakeAgent_Success` — wake an `idle` agent; verify `200`, response contains `runId` and `status=queued`; a `wakeup_requests` row exists with `status=pending`.
2. `TestWakeAgent_AdapterNotFound` — agent has an unregistered `adapterType`; verify `422` with `code=ADAPTER_NOT_FOUND`.
3. `TestWakeAgent_PausedAgent` — agent has `status=paused`; verify `409` (wakeup discarded per REQ-044).
4. `TestWakeAgent_TerminatedAgent` — agent has `status=terminated`; verify `409`.
5. `TestStopAgent_Success` — wake an agent (so a run is active), then stop it; verify `200`, agent `status=paused`.
6. `TestStopAgent_NothingRunning` — stop an idle agent; verify `200` and agent remains `idle` (graceful no-op).

**Auth middleware tests** — extend `internal/auth/middleware_test.go`:

7. `TestMiddleware_RunTokenAccepted` — request with `Authorization: Bearer <valid_run_token>`; verify handler receives `AgentIdentity` in context.
8. `TestMiddleware_RunTokenRevoked` — token in revocation list; verify `401`.
9. `TestMiddleware_PausedAgentRunTokenRejected` — agent is `paused` in DB; verify `401` even for a non-revoked token (REQ-034).
10. `TestMiddleware_RunTokenWrongType` — `typ` claim is not `run_token`; verify the middleware does not accept it as a Run Token.

Tests fail because the `WakeAgent`/`StopAgent` handlers and the Run Token middleware branch do not exist.

#### GREEN — Implement Minimum to Pass

1. Create `internal/server/handlers/runtime_handler.go`:
   - `RuntimeHandler{queries *db.Queries, wakeupSvc *wakeup.Service, runSvc *runtime.RunService}`.
   - `NewRuntimeHandler(...)`.
   - `WakeAgent(w, r)`: load agent, check status (return `409` if paused/terminated), call `wakeupSvc.Enqueue(...)`, return `{runId:"", agentId, status:"queued"}` (runId is pending, actual UUID assigned when dispatched).
   - `StopAgent(w, r)`: call `runSvc.Stop(agentID)`, return updated agent.
2. Extend `internal/auth/middleware.go`:
   - In `Authenticate` (or equivalent), after extracting `Authorization: Bearer` token, attempt to parse as `RunTokenClaims` (check `typ == "run_token"`).
   - If Run Token: validate signature, check `IsRevoked`, check agent `status` not paused/terminated in DB, inject `AgentIdentity{AgentID, SquadID, RunID, Role}` into context.
   - If not Run Token: fall through to existing user session logic.
   - Extend `internal/auth/types.go` with `AgentIdentity` struct and a new context key.
3. Register routes in `internal/server/routes.go`: `POST /api/agents/{id}/wake`, `POST /api/agents/{id}/stop`.
4. All ten tests pass.

#### REFACTOR

- Extract `extractRunToken(r *http.Request, tokenSvc *token.RunTokenService, queries *db.Queries) (*AgentIdentity, error)` into `internal/auth/run_token.go` to keep the middleware readable.
- `WakeAgent` must not call `runSvc.Invoke` directly — it only enqueues. The actual dispatch is `WakeupProcessor`'s responsibility (separation of concerns).

#### Acceptance Criteria

- [ ] `POST /api/agents/{id}/wake` enqueues a wakeup, returns `{runId, agentId, status}`
- [ ] `POST /api/agents/{id}/wake` returns `422` for unregistered adapter type
- [ ] `POST /api/agents/{id}/wake` returns `409` for paused/terminated agent
- [ ] `POST /api/agents/{id}/stop` cancels active run and transitions agent to `paused`
- [ ] Run Token middleware accepts `Bearer <run_token>` and injects `AgentIdentity`
- [ ] Revoked Run Tokens return `401`
- [ ] Run Tokens for paused/terminated agents return `401`
- [ ] `make test` passes

#### Files to Create / Modify

- **Create:** `internal/server/handlers/runtime_handler.go`
- **Create:** `internal/server/handlers/runtime_integration_test.go`
- **Modify:** `internal/auth/middleware.go` (add Run Token branch)
- **Modify:** `internal/auth/types.go` (add `AgentIdentity`, context key)
- **Create:** `internal/auth/run_token.go`
- **Modify:** `internal/server/routes.go` (add wake/stop routes)

---

### Task 10 — Server Initialization Wiring

**Requirements:** REQ-029, REQ-049, REQ-057
**Estimated time:** 45 min

#### Context

Wire all runtime components in `cmd/ari/run.go` in the correct dependency order: startup audit → adapter registry (with environment checks) → SSE hub → Run Token service → Run service → Wakeup service + processor → inject into handlers → register routes → graceful shutdown drain. Runtime config fields (`MaxRunsPerSquad`, `StaleCheckoutAge`, `AgentDrainTimeout`) must be added to `internal/config/config.go` with defaults.

#### RED — Write Failing Tests

Create `internal/server/server_integration_test.go` (or extend existing `server_test.go`) for end-to-end smoke tests:

1. `TestServer_WakeStopSSECycle` — start the full test server, wake an agent with a mock adapter, subscribe to SSE, verify `agent.status.changed` events `idle→running` arrive, call stop, verify `running→paused` events arrive.
2. `TestServer_CheckoutReleaseSSE` — agent with active run calls checkout, then release; verify `issue.updated` SSE events for both operations.
3. `TestServer_SSEInitialSnapshot` — connect SSE before any run; verify synthetic `agent.status.changed` events for all agents in the squad are received immediately.
4. `TestServer_StartupAuditClearsStaleRuns` — insert a `running` HeartbeatRun before server starts; start server; verify on startup the run is `cancelled` and agent is `error`.
5. `TestServer_AdapterUnavailableOnStartup` — register an adapter whose `TestEnvironment` returns `Available=false`; verify wake returns `422 ADAPTER_NOT_FOUND`.

Also add config tests in `internal/config/config_test.go`:

6. `TestConfig_RuntimeDefaults` — verify `MaxRunsPerSquad=3`, `StaleCheckoutAge=2h`, `AgentDrainTimeout=30s` when not set.

Tests fail because the wiring in `cmd/ari/run.go` is incomplete and config fields are missing.

#### GREEN — Implement Minimum to Pass

1. Extend `internal/config/config.go`:
   - Add `MaxRunsPerSquad int` (default 3), `StaleCheckoutAge time.Duration` (default 2h), `AgentDrainTimeout time.Duration` (default 30s).
   - Populate defaults in the config loading function.
2. Modify `cmd/ari/run.go` with the full wiring sequence from the design:
   ```
   auditStaleRuns(ctx, queries)
   registry := adapter.NewRegistry()
   processAdapter := process.New()
   registry.Register(processAdapter)
   // env check loop → registry.MarkUnavailable on failure
   sseHub := sse.NewHub()
   runTokenSvc := token.NewRunTokenService(signingKey)
   runSvc := runtime.NewRunService(db, queries, registry, runTokenSvc, sseHub, cfg)
   wakeupSvc := wakeup.NewService(queries)
   wakeupProc := wakeup.NewProcessor(db, queries, registry, sseHub, runSvc, cfg)
   wakeupProc.Start(ctx)
   runtimeHandler := handlers.NewRuntimeHandler(queries, wakeupSvc, runSvc)
   sseHandler := handlers.NewSSEHandler(queries, sseHub)
   ```
3. Pass `runtimeHandler` and `sseHandler` into `server.New()` or the router setup; ensure routes are registered.
4. Add graceful shutdown drain before `http.Server.Shutdown`:
   ```go
   drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.AgentDrainTimeout)
   defer drainCancel()
   runSvc.DrainAll(drainCtx)
   ```
5. Start the Run Token revocation sweep goroutine: `go runTokenSvc.SweepLoop(ctx)` with 1h interval.
6. Pass `runTokenSvc` to the auth middleware so it can check revocations.
7. All six tests pass. Run full test suite: `make test`.

#### REFACTOR

- Extract the adapter registration block into a `registerAdapters(cfg Config) *adapter.Registry` function in `cmd/ari/run.go` to keep `runServer()` readable.
- Ensure all background goroutines (`wakeupProc.Start`, stale lock sweep, token sweep) are cancelled by the server's root context when shutdown begins.
- Add `slog.Info("runtime initialized", "adapters", registry.RegisteredTypes())` to confirm startup state.

#### Acceptance Criteria

- [ ] `cmd/ari/run.go` wires all runtime components in correct dependency order
- [ ] `auditStaleRuns` called before wakeup processor starts
- [ ] Adapter environment checks run at startup; failed adapters marked unavailable (not fatal)
- [ ] Graceful shutdown drains active runs before `http.Server.Shutdown`
- [ ] Run Token revocation sweep goroutine started with server context
- [ ] `MaxRunsPerSquad`, `StaleCheckoutAge`, `AgentDrainTimeout` config fields have correct defaults
- [ ] Full `make test` suite passes with no regressions
- [ ] `make build` succeeds

#### Files to Create / Modify

- **Modify:** `internal/config/config.go` (add runtime config fields + defaults)
- **Modify:** `cmd/ari/run.go` (full runtime wiring)
- **Modify:** `internal/server/server.go` (accept and wire new handlers)
- **Modify:** `internal/server/routes.go` (confirm all routes registered)
- **Modify:** `internal/auth/middleware.go` (inject `RunTokenService` dependency)
- **Create:** `internal/server/server_integration_test.go`
- **Modify:** `internal/config/config_test.go`

---

## Notes

### Blockers

- `make sqlc` must be re-run after each task that adds new SQL queries (Tasks 01, 06, 07, 08).
- Run Token middleware extension (Task 09) depends on Task 04 (token package).
- `RunService` (Task 07) depends on Task 04 (token), Task 05 (SSE hub), and Task 06 (wakeup types for `WakeupRequest` input type).
- All integration tests require the migration from Task 01 to be applied to the embedded test DB.

### Dependency Order

```
Task 01 (DB migration)
  └── Task 02 (Adapter interface)
        ├── Task 03 (Process adapter)       [unblocks adapter tests]
        └── Task 04 (Run Token)             [unblocks middleware]
              └── Task 05 (SSE Hub)         [can start after Task 01]
                    └── Task 06 (Wakeup queue)
                          └── Task 07 (RunService)
                                ├── Task 08 (Checkout endpoints)
                                └── Task 09 (Wake/Stop endpoints + auth)
                                      └── Task 10 (Server wiring)
```

Tasks 03, 04, and 05 can be parallelised once Task 02 is complete.

### Future Improvements

- Docker/container adapter
- Multi-node distributed execution
- Agent autoscaling
- Custom adapter plugins via external process
- Streaming partial agent output mid-turn (WebSocket upgrade)
- Parallel pipeline stages
