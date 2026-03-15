# Tasks: Cost Events & Budget Enforcement

**Created:** 2026-03-15
**Status:** Complete

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-001 through REQ-037

## Implementation Approach

1. DB migration + sqlc queries (cost_events table, aggregation queries)
2. Domain models + pure logic (CostEvent, ThresholdStatus, BillingPeriod, validation)
3. BudgetEnforcementService (RecordAndEnforce, ReEvaluateAgent, ReEvaluateSquad, CheckResumeAllowed)
4. CostHandler endpoints (POST cost-events, GET summary, GET by-agent, GET agent/me/budget)
5. Agent status machine integration (auto-pause guard + resume guard)
6. React budget UI components (BudgetBar, CostSummaryCard, AgentCostTable, dashboard widget)

## Progress Summary

- Total Tasks: 9
- Completed: 8
- Skipped: 1 (Task 8 — React UI, backend-only focus)
- Test Coverage: Integration tests in cost_integration_test.go; unit tests in cost_test.go

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 1 — Database Migration: `cost_events` Table

**Requirements:** REQ-001, REQ-016, REQ-018, REQ-020, REQ-021, REQ-022, REQ-031

**Estimated time:** 30 min

#### RED — Write Failing Test

Write a migration smoke-test that asserts the table and its constraints exist after
running `goose up`. Add to `internal/database/migrations/migration_test.go` (or the
nearest existing migration test file):

```
TestCostEventsTableExists
  - Apply all migrations against a clean embedded-postgres instance.
  - Assert cost_events table exists.
  - Assert INSERT with cost_cents = -1 fails (check constraint).
  - Assert INSERT with event_type = 'invalid' fails (check constraint).
  - Assert INSERT without squad_id / agent_id / event_type / cost_cents fails (NOT NULL).
  - Assert UPDATE on a cost_events row fails (no UPDATE route — test via SQL directly).
  - Assert indexes idx_cost_events_squad_created, idx_cost_events_agent_created,
    idx_cost_events_squad_agent_created exist in pg_indexes.
```

Run `make test` → tests fail (table does not exist).

#### GREEN — Implement Minimum Code

Create `internal/database/migrations/20260314000011_create_cost_events.sql`:

```sql
-- +goose Up
CREATE TABLE cost_events (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id         UUID         NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id         UUID         NOT NULL REFERENCES agents(id),
    heartbeat_run_id UUID,
    issue_id         UUID         REFERENCES issues(id),
    project_id       UUID         REFERENCES projects(id),
    goal_id          UUID         REFERENCES goals(id),
    event_type       VARCHAR(50)  NOT NULL
                     CHECK (event_type IN ('llm_call', 'tool_use', 'embedding', 'other')),
    model            VARCHAR(100),
    provider         VARCHAR(50),
    input_tokens     INT          CHECK (input_tokens IS NULL OR input_tokens >= 0),
    output_tokens    INT          CHECK (input_tokens IS NULL OR output_tokens >= 0),
    cost_cents       BIGINT       NOT NULL CHECK (cost_cents >= 0),
    billing_code     VARCHAR(100),
    usage_json       JSONB,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_cost_events_squad_created       ON cost_events (squad_id, created_at DESC);
CREATE INDEX idx_cost_events_agent_created       ON cost_events (agent_id, created_at DESC);
CREATE INDEX idx_cost_events_squad_agent_created ON cost_events (squad_id, agent_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS cost_events;
```

Run `make test` → migration tests pass.

#### REFACTOR

- Confirm `heartbeat_run_id` has no FK constraint (dependency on feature 11 is deferred).
- Add a SQL comment block explaining append-only semantics and REQ-018.
- Ensure `-- +goose Down` only drops `cost_events` (no cascade side-effects on other tables).

#### Acceptance Criteria

- [ ] Migration applies cleanly with `goose up` against empty and existing databases.
- [ ] `cost_events` table has all 18 columns listed in the requirements schema.
- [ ] `cost_cents >= 0` check constraint prevents negative values.
- [ ] `event_type` check constraint limits to the four valid values.
- [ ] All three indexes exist after migration.
- [ ] `goose down` cleanly removes the table.
- [ ] No FK on `heartbeat_run_id` (bare UUID column).

#### Files to Create / Modify

- **Create:** `internal/database/migrations/20260314000011_create_cost_events.sql`

---

### [x] Task 2 — sqlc Queries: Cost Event Persistence + Aggregation

**Requirements:** REQ-001, REQ-002, REQ-005, REQ-009, REQ-010, REQ-015, REQ-031, REQ-037

**Estimated time:** 45 min

#### RED — Write Failing Test

In `internal/database/queries/cost_events_test.go` (new file), using the embedded-postgres
test helper pattern from existing query tests:

```
TestInsertCostEvent_MinimalFields
  - Insert a cost event with only required fields (squad_id, agent_id, event_type, cost_cents).
  - Assert returned row has a non-nil UUID id and created_at.

TestInsertCostEvent_AllFields
  - Insert with model, provider, input_tokens, output_tokens, billing_code, usage_json.
  - Assert all fields round-trip correctly.

TestInsertCostEvent_DuplicateClientID
  - Insert with explicit id = uuid.New().
  - Insert again with same id → expect unique-constraint error (pq error code 23505).

TestGetCostEventByID
  - Insert an event, then GetCostEventByID → same row returned.

TestGetAgentMonthlySpend_Empty
  - No events for agent → returns 0.

TestGetAgentMonthlySpend_SumCurrentMonth
  - Insert three events this month → sum returned correctly.
  - Insert one event last month → not included in sum.

TestGetSquadMonthlySpend_SumCurrentMonth
  - Multiple agents in squad → spend summed across all agents.

TestGetAgentCostBreakdown
  - Two agents; agent A has 300 cents, agent B has 100 cents.
  - Verify order (descending by spend), agent names, budget fields.

TestListRunningIdleAgentsBySquad
  - Create agents in statuses: running, idle, active, paused, terminated.
  - Verify only running + idle are returned.
```

Run `make sqlc && make test` → tests fail (queries file missing / sqlc types missing).

#### GREEN — Implement Minimum Code

1. Create `internal/database/queries/cost_events.sql` with all six named queries:
   - `InsertCostEvent :one`
   - `GetCostEventByID :one`
   - `GetAgentMonthlySpend :one`
   - `GetSquadMonthlySpend :one`
   - `GetAgentCostBreakdown :many`
   - `GetCostEventsByAgent :many`
   - `ListRunningIdleAgentsBySquad :many`

2. Run `make sqlc` to regenerate `internal/database/db/cost_events.sql.go`.

Run `make test` → query tests pass.

#### REFACTOR

- Verify `GetAgentMonthlySpend` and `GetSquadMonthlySpend` use `COALESCE(SUM(...), 0)::BIGINT`
  to prevent NULL returns on empty result sets.
- Confirm `GetAgentCostBreakdown` LEFT JOINs `cost_events` so agents with zero spend still appear.
- Add `WHERE a.status != 'terminated'` to `GetAgentCostBreakdown` (exclude terminated agents).
- Confirm `@period_start` / `@period_end` parameter names are consistent across all time-range
  queries (no accidental mixing with positional parameters).

#### Acceptance Criteria

- [ ] `make sqlc` generates without errors.
- [ ] All seven query functions are present in the generated `db` package.
- [ ] `GetAgentMonthlySpend` returns 0 (not NULL) when no events exist.
- [ ] `GetAgentCostBreakdown` includes agents with zero spend for the period.
- [ ] `InsertCostEvent` with a duplicate client-supplied `id` returns a `23505` pg error.
- [ ] Time-range filtering excludes events outside the billing period window.

#### Files to Create / Modify

- **Create:** `internal/database/queries/cost_events.sql`
- **Auto-generated:** `internal/database/db/cost_events.sql.go` (via `make sqlc`)

---

### [x] Task 3 — Domain Models: CostEvent, ThresholdStatus, BillingPeriod, Validation

**Requirements:** REQ-016, REQ-022, REQ-003, REQ-004, REQ-005, REQ-006, REQ-023, REQ-024, REQ-028

**Estimated time:** 45 min

#### RED — Write Failing Test

Create `internal/domain/cost_event_test.go`:

```
TestComputeThresholdStatus
  - nil budget → "ok"
  - budget = 0 → "ok" (zero budget treated as unlimited)
  - spend=0,   budget=1000 → "ok"
  - spend=799, budget=1000 → "ok"
  - spend=800, budget=1000 → "warning"  (exactly 80%)
  - spend=999, budget=1000 → "warning"
  - spend=1000, budget=1000 → "exceeded" (exactly 100%)
  - spend=1500, budget=1000 → "exceeded" (over budget)

TestBillingPeriod
  - 2026-03-15T12:00:00Z → start=2026-03-01T00:00:00Z, end=2026-04-01T00:00:00Z
  - 2026-12-31T23:59:59Z → start=2026-12-01T00:00:00Z, end=2027-01-01T00:00:00Z
  - 2026-01-01T00:00:00Z → start=2026-01-01T00:00:00Z, end=2026-02-01T00:00:00Z
  - Non-UTC input is converted to UTC first.

TestValidateCreateCostEventRequest_InvalidEventType
  - eventType = "foobar" → error message contains "eventType"

TestValidateCreateCostEventRequest_NegativeCostCents
  - costCents = -1 → error

TestValidateCreateCostEventRequest_NegativeTokens
  - inputTokens = -1 → error
  - outputTokens = -1 → error

TestValidateCreateCostEventRequest_FieldLengths
  - model > 100 chars → error
  - provider > 50 chars → error
  - billingCode > 100 chars → error

TestValidateCreateCostEventRequest_InvalidUsageJSON
  - usageJson = []byte("not json") → error

TestValidateCreateCostEventRequest_AllOptionalNil
  - eventType="llm_call", costCents=0, all optionals nil → no error
```

Run `make test` → tests fail (file does not exist).

#### GREEN — Implement Minimum Code

Create `internal/domain/cost_event.go` with:
- `CostEventType` constants: `llm_call`, `tool_use`, `embedding`, `other`
- `ThresholdStatus` constants: `ok`, `warning`, `exceeded`
- `CostEvent` struct (domain model)
- `CreateCostEventRequest` struct
- `ValidateCreateCostEventRequest(req) error`
- `ComputeThresholdStatus(spendCents int64, budgetCents *int64) ThresholdStatus`
- `BillingPeriod(t time.Time) (start, end time.Time)`

Run `make test` → all domain unit tests pass.

#### REFACTOR

- Ensure `ComputeThresholdStatus` has no floating-point precision issues at boundary values
  (use `spendCents*100 >= budgetCents*80` integer arithmetic, or document float64 acceptability
  for this range of values).
- Keep `BillingPeriod` a pure function (no `time.Now()` inside it) — caller always passes `t`.
- Export `validCostEventTypes` map for use in HTTP handler validation.

#### Acceptance Criteria

- [ ] All unit tests in `cost_event_test.go` pass.
- [ ] `ComputeThresholdStatus` boundary values (800/1000, 1000/1000) return correct tiers.
- [ ] `BillingPeriod` correctly handles year-boundary (Dec → Jan).
- [ ] `ValidateCreateCostEventRequest` returns descriptive errors for each invalid field.
- [ ] `CostEvent` struct JSON tags use camelCase matching the API spec.

#### Files to Create / Modify

- **Create:** `internal/domain/cost_event.go`
- **Create:** `internal/domain/cost_event_test.go`

---

### [x] Task 4 — BudgetEnforcementService: RecordAndEnforce

**Requirements:** REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-013,
REQ-015, REQ-019, REQ-023, REQ-024, REQ-036, REQ-037

**Estimated time:** 60 min

#### RED — Write Failing Test

Create `internal/budget/service_test.go` using the embedded-postgres test environment
(follow the `makeEnv` pattern from handler integration tests for DB setup):

```
TestRecordAndEnforce_HappyPath
  - Create squad + agent (no budget set).
  - Call RecordAndEnforce with cost_cents=500.
  - Assert returned event has correct fields, Duplicate=false.
  - Assert agent status unchanged.

TestRecordAndEnforce_IdempotentDuplicate
  - Call RecordAndEnforce with a clientID (UUID).
  - Call again with same clientID.
  - Assert second call returns Duplicate=true, same event ID.
  - Assert only one row in cost_events.

TestRecordAndEnforce_AgentBudgetExceeded_AutoPause
  - Set agent budget_monthly_cents = 100.
  - Call RecordAndEnforce with cost_cents = 100 and agent status = running.
  - After commit, GET agent → status = paused.

TestRecordAndEnforce_AgentBudgetExceeded_AlreadyPaused
  - Set agent budget_monthly_cents = 100, status = paused.
  - Call RecordAndEnforce with cost_cents = 100.
  - Assert service does NOT try to double-pause (no error, no status change).

TestRecordAndEnforce_AgentBudgetWarning_NoStatusChange
  - Set agent budget_monthly_cents = 100.
  - Call RecordAndEnforce with cost_cents = 80 (exactly 80%).
  - After commit, GET agent → status unchanged.

TestRecordAndEnforce_NoBudget_SkipsEnforcement
  - Agent has no budget (NULL).
  - Call with cost_cents = 999999.
  - Assert agent not paused (REQ-023).

TestRecordAndEnforce_SquadBudgetExceeded_PausesAllActiveAgents
  - Set squad budget_monthly_cents = 200.
  - Create two agents (running, idle); one already paused.
  - Call RecordAndEnforce with cost_cents = 200.
  - After commit: running + idle agents are paused; already-paused agent unchanged.

TestRecordAndEnforce_SquadNoBudget_SkipsSquadEnforcement
  - Squad has no budget; agent has budget exceeded.
  - Assert only agent-level enforcement fires (REQ-024).

TestRecordAndEnforce_TransactionRollbackOnError
  - Force a DB error during enforcement (e.g., close DB mid-transaction).
  - Assert returned error is non-nil.
  - Assert no cost_events row was persisted (insert was rolled back).
```

Run `make test` → tests fail (package does not exist).

#### GREEN — Implement Minimum Code

Create `internal/budget/service.go` with:
- `Service` struct with `db *sql.DB` and `queries *db.Queries` fields
- `New(conn *sql.DB, q *db.Queries) *Service`
- `RecordParams` struct
- `RecordResult` struct (Event + Duplicate flag)
- `RecordAndEnforce(ctx, qtx *db.Queries, params RecordParams) (RecordResult, error)`
  following the five-step flow from the design:
  1. InsertCostEvent (handle 23505 for idempotency)
  2. Fetch agent budget + status
  3. Agent-level enforcement (pause if >= 100%, log warning at >= 80%)
  4. Fetch squad budget
  5. Squad-level enforcement (pause all running/idle agents if >= 100%)
- `isPgUniqueViolation(err error) bool` helper

Run `make test` → service tests pass.

#### REFACTOR

- Extract a `pauseAgent(ctx, qtx, agentID uuid.UUID) error` private method to reduce
  duplication between agent-level and squad-level pause logic.
- Replace raw `slog.Info` budget-warning stubs with clearly labelled `// TODO(inbox):` comments
  per design, so they are easy to locate when feature 09 (activity log) lands.
- Ensure `ValidateStatusTransition` is called before each `UpdateAgent(paused)` to prevent
  invalid transitions (e.g., do not attempt to pause an already-paused agent).

#### Acceptance Criteria

- [ ] All service unit tests pass.
- [ ] Auto-pause fires correctly at exactly 100% spend (not at 99%, not missing at 101%).
- [ ] Idempotent duplicate submission returns `Duplicate=true` without inserting a new row.
- [ ] Squad-level hard stop pauses all running/idle agents in a single transaction.
- [ ] No agent is double-paused (validate transition before updating).
- [ ] A DB error mid-enforcement returns an error and leaves no cost_events row persisted.

#### Files to Create / Modify

- **Create:** `internal/budget/service.go`
- **Create:** `internal/budget/service_test.go`

---

### [x] Task 5 — BudgetEnforcementService: ReEvaluateAgent, ReEvaluateSquad, CheckResumeAllowed

**Requirements:** REQ-007, REQ-008, REQ-014, REQ-023, REQ-024, REQ-025, REQ-026, REQ-027

**Estimated time:** 45 min

#### RED — Write Failing Test

Add to `internal/budget/service_test.go`:

```
TestReEvaluateAgent_BudgetReducedBelowSpend_Pauses
  - Record 150 cents spend. Then PATCH agent budget to 100.
  - Call ReEvaluateAgent → agent is auto-paused.

TestReEvaluateAgent_BudgetIncreasedAboveSpend_AgentStaysPaused
  - Agent is paused (was auto-paused). Budget raised from 100 to 200.
  - Current spend = 100.
  - Call ReEvaluateAgent → agent remains paused (REQ-027: manual resume required).

TestReEvaluateAgent_BudgetRemoved_NoAction
  - Budget set to NULL.
  - Call ReEvaluateAgent → no error, agent not paused (REQ-023).

TestReEvaluateSquad_BudgetExceeded_PausesActiveAgents
  - Squad has two running agents, total spend 200, new budget 150.
  - Call ReEvaluateSquad → both agents paused.

TestReEvaluateSquad_BudgetRemoved_NoAction
  - Squad budget set to NULL.
  - Call ReEvaluateSquad → no error, no agents paused.

TestCheckResumeAllowed_AgentBudgetStillExceeded_ReturnsError
  - Agent spend = 100, budget = 100 (100%).
  - CheckResumeAllowed → error (budget still exceeded).

TestCheckResumeAllowed_AgentBudgetIncreased_ReturnsNil
  - Agent spend = 100, budget = 200.
  - CheckResumeAllowed → nil (resume is allowed).

TestCheckResumeAllowed_SquadBudgetStillExceeded_ReturnsError
  - Squad spend = 100, budget = 100 (100%), agent budget = nil.
  - CheckResumeAllowed → error (squad budget exceeded).

TestCheckResumeAllowed_NoBudgets_ReturnsNil
  - Both agent and squad have NULL budgets.
  - CheckResumeAllowed → nil (REQ-023, REQ-024).
```

Run `make test` → new test cases fail (methods do not exist).

#### GREEN — Implement Minimum Code

Add to `internal/budget/service.go`:

- `ReEvaluateAgent(ctx context.Context, qtx *db.Queries, agentID uuid.UUID) error`
  - Fetch agent; if budget is NULL, return nil (REQ-023).
  - Compute current month spend.
  - If exceeded: pause agent if running/idle.
  - If OK: resolve alerts TODO stub.
- `ReEvaluateSquad(ctx context.Context, qtx *db.Queries, squadID uuid.UUID) error`
  - Fetch squad; if budget is NULL, return nil (REQ-024).
  - Compute squad monthly spend.
  - If exceeded: pause all running/idle agents.
- `CheckResumeAllowed(ctx context.Context, qtx *db.Queries, agentID uuid.UUID) error`
  - Fetch agent; if agent budget is set and spend >= 100% → return error "agent budget exceeded".
  - Fetch agent's squad; if squad budget is set and squad spend >= 100% → return error "squad budget exceeded".
  - Otherwise return nil.

Run `make test` → all tests pass.

#### REFACTOR

- `CheckResumeAllowed` and `ReEvaluateAgent` both compute agent monthly spend; extract a
  `agentMonthlySpend(ctx, qtx, agentID) (int64, error)` private helper to avoid duplication.
- Ensure `ReEvaluateAgent` does NOT auto-resume a paused agent when budget is raised — it only
  pauses or resolves warnings. The agent remains paused until a human resumes it (REQ-027).

#### Acceptance Criteria

- [ ] `ReEvaluateAgent` auto-pauses an agent whose spend now exceeds a newly reduced budget.
- [ ] `ReEvaluateAgent` does NOT auto-resume a paused agent when budget is raised.
- [ ] `ReEvaluateSquad` pauses all running/idle squad agents when squad budget is exceeded.
- [ ] `CheckResumeAllowed` blocks resume when either agent or squad budget is still exceeded.
- [ ] `CheckResumeAllowed` returns nil when no budgets are set (unlimited spend case).

#### Files to Create / Modify

- **Modify:** `internal/budget/service.go` (add three methods)
- **Modify:** `internal/budget/service_test.go` (add test cases)

---

### [x] Task 6 — CostHandler: API Endpoints

**Requirements:** REQ-009, REQ-010, REQ-011, REQ-012, REQ-017, REQ-029, REQ-030,
REQ-033, REQ-034, REQ-036, REQ-037

**Estimated time:** 60 min

#### RED — Write Failing Test

Create `internal/server/handlers/cost_integration_test.go` using the existing
`makeEnv(t, auth.ModeAuthenticated, false)` test helper pattern:

```
TestRecordCostEvent_HappyPath_Returns201
  - Create squad + member user (owner) + agent (captain, status=active).
  - POST /api/squads/{id}/cost-events with valid payload.
  - Assert 201, response body has id, agentId, eventType, costCents, createdAt.

TestRecordCostEvent_IdempotentReplay_Returns200
  - POST with explicit id (UUID).
  - POST same id again.
  - Assert second response is 200, same event body, only one DB row.

TestRecordCostEvent_TerminatedAgent_Returns409
  - Transition agent to terminated.
  - POST cost event → 409, code=AGENT_TERMINATED.

TestRecordCostEvent_CrossSquadAgent_Returns403
  - Create second squad + second agent.
  - POST to first squad's cost-events URL using second agent's id.
  - Assert 403, code=FORBIDDEN.

TestRecordCostEvent_InvalidPayload_Returns400
  - POST with eventType="bad" → 400, code=VALIDATION_ERROR.
  - POST with costCents=-1 → 400.

TestRecordCostEvent_Unauthenticated_Returns401
  - POST without auth header → 401.

TestRecordCostEvent_AgentAutoPausedAt100Percent
  - Agent budget_monthly_cents = 100, status = active.
  - POST cost event cost_cents=100.
  - GET /api/agents/{id} → status = paused.

TestRecordCostEvent_SquadAutoPausesAllAgentsAt100Percent
  - Squad budget_monthly_cents = 200. Two agents (running + idle).
  - POST cost event cost_cents=200.
  - GET both agents → both paused.

TestGetCostSummary_NoBudget
  - GET /api/squads/{id}/costs/summary.
  - Assert budgetMonthlyCents=null, percentUtilised=null, thresholdStatus="ok".

TestGetCostSummary_WithSpend
  - Record two events (30 + 20 cents). Budget = 100.
  - GET summary → spendCents=50, percentUtilised=50.0, thresholdStatus="ok".

TestGetCostSummary_ExceededBudget
  - Record event 100 cents. Budget = 100.
  - GET summary → thresholdStatus="exceeded".

TestGetCostByAgent_BreakdownAndOrdering
  - Two agents: A spends 300 cents, B spends 100 cents.
  - GET /api/squads/{id}/costs/by-agent.
  - Assert items[0].agentId = A (higher spend first), items[1].agentId = B.
  - Assert periodStart and periodEnd present.

TestGetCostByAgent_Unauthenticated_Returns401
  - GET without auth → 401.

TestGetAgentBudget_ReturnsNotImplemented
  - GET /api/agent/me/budget → 501, code=NOT_IMPLEMENTED
    (pending Run Token implementation in feature 11).
```

Run `make test` → tests fail (routes not registered).

#### GREEN — Implement Minimum Code

Create `internal/server/handlers/cost_handler.go` with:
- `CostHandler` struct: `queries *db.Queries`, `dbConn *sql.DB`, `budgetSvc *budget.Service`
- `NewCostHandler(q, dbConn, svc) *CostHandler`
- `RegisterRoutes(mux *http.ServeMux)` — registers all four routes
- `RecordCostEvent(w, r)` — full implementation per design (auth check, agent validation,
  terminated check, TX open, RecordAndEnforce, commit, 201 or 200 for duplicate)
- `GetCostSummary(w, r)` — membership check, squad fetch, spend aggregation, threshold calc
- `GetCostByAgent(w, r)` — membership check, GetAgentCostBreakdown, compute per-agent threshold
- `GetAgentBudget(w, r)` — return 501 (stub; full impl in feature 11)
- `dbCostEventToResponse(e db.CostEvent) costEventResponse` helper

Wire in `cmd/ari/run.go`:
```go
budgetSvc := budget.New(db, queries)
costHandler := handlers.NewCostHandler(queries, db, budgetSvc)
```
Register `costHandler` with the server's route registrar list.

Run `make test` → all cost handler integration tests pass.

#### REFACTOR

- Extract a `requireSquadMembership(w, r, squadID) bool` helper shared across `RecordCostEvent`,
  `GetCostSummary`, and `GetCostByAgent` (the auth + membership check pattern is identical).
- Ensure `parseSquadID` is already in the shared handler utilities or add it.
- `GetCostByAgent` response: ensure agents with zero spend and no budget still have
  `thresholdStatus: "ok"` (not empty string).

#### Acceptance Criteria

- [ ] `POST /api/squads/{id}/cost-events` returns 201 on first submission and 200 on replay.
- [ ] Terminated agent returns 409 with `AGENT_TERMINATED` code.
- [ ] Cross-squad agent returns 403 with `FORBIDDEN` code.
- [ ] Invalid payload returns 400 with `VALIDATION_ERROR` code.
- [ ] Auto-pause at 100% budget is visible via subsequent GET agent status.
- [ ] `GET /api/squads/{id}/costs/summary` returns correct spend, percentage, and threshold.
- [ ] `GET /api/squads/{id}/costs/by-agent` returns agents sorted by spend descending.
- [ ] `GET /api/agent/me/budget` returns 501 (feature 11 stub).
- [ ] All endpoints return 401 when unauthenticated.

#### Files to Create / Modify

- **Create:** `internal/server/handlers/cost_handler.go`
- **Create:** `internal/server/handlers/cost_integration_test.go`
- **Modify:** `cmd/ari/run.go` (wire BudgetService + CostHandler)

---

### [x] Task 7 — Agent Status Machine Integration: Resume Guard + Budget Re-evaluation on PATCH

**Requirements:** REQ-007, REQ-008, REQ-014, REQ-025, REQ-027, REQ-035

**Estimated time:** 45 min

#### RED — Write Failing Test

Add to `internal/server/handlers/cost_integration_test.go`:

```
TestResumeGuard_BudgetStillExceeded_Returns409
  - Agent budget = 100, status = active.
  - POST cost event cost_cents = 100 → agent is auto-paused.
  - POST /api/agents/{id}/transition with {status: "active"} → 409, code=BUDGET_EXCEEDED.

TestResumeGuard_BudgetIncreased_ResumeAllowed
  - Agent budget = 100, status = active.
  - POST cost event cost_cents = 100 → agent paused.
  - PATCH /api/agents/{id} with budgetMonthlyCents = 200.
  - POST /api/agents/{id}/transition {status: "active"} → 200 (allowed).

TestPatchAgentBudget_ReEvaluatesAndPausesIfExceeded
  - Agent has no budget; record 150 cents spend.
  - PATCH /api/agents/{id} with budgetMonthlyCents = 100.
  - GET /api/agents/{id} → status = paused (re-evaluation fired within same TX).

TestPatchAgentBudget_AgentRemainsOwnerPaused_AfterBudgetIncrease
  - Agent budget = 100, 100 cents recorded, agent is paused.
  - PATCH budget to 200.
  - GET agent → still paused (REQ-027: manual resume required, no auto-resume).
  - POST transition {status:"active"} → 200 (now allowed since spend < budget).

TestPatchSquadBudget_ReEvaluatesSquad
  - Two running agents; squad has no budget; record 200 cents total spend.
  - PATCH /api/squads/{id}/budgets with budgetMonthlyCents = 150.
  - GET both agents → both paused.
```

Run `make test` → new tests fail (resume guard not implemented, PATCH handlers not
calling ReEvaluate).

#### GREEN — Implement Minimum Code

1. **Resume guard in agent transition handler** (`internal/server/handlers/agent_handler.go`):
   - In `TransitionAgentStatus`, after `ValidateStatusTransition` passes and before the
     `UpdateAgent` query, add:
     ```go
     if req.Status == domain.AgentStatusActive {
         if err := h.budgetSvc.CheckResumeAllowed(ctx, h.queries, agentID); err != nil {
             writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error(), Code: "BUDGET_EXCEEDED"})
             return
         }
     }
     ```
   - Inject `budgetSvc *budget.Service` into `AgentHandler`; update `NewAgentHandler` signature.

2. **Budget re-evaluation on `PATCH /api/agents/{id}`** (`agent_handler.go`):
   - Convert the single `UpdateAgent` query call into a transaction.
   - After `UpdateAgent` succeeds and if `req.SetBudget == true`, call
     `budgetSvc.ReEvaluateAgent(ctx, qtx, agentID)` in the same transaction.

3. **Budget re-evaluation on `PATCH /api/squads/{id}/budgets`** (`squad_handler.go`):
   - Similarly wrap in a transaction and call `budgetSvc.ReEvaluateSquad(ctx, qtx, squadID)`.
   - Inject `budgetSvc` into `SquadHandler`; update `NewSquadHandler` signature.

4. Update `cmd/ari/run.go` to pass `budgetSvc` to `NewAgentHandler` and `NewSquadHandler`.

Run `make test` → all new tests pass.

#### REFACTOR

- Ensure `SetBudget` sentinel field on `UpdateAgentRequest` is set correctly in the handler
  when the JSON payload contains `"budgetMonthlyCents"` (including setting to null to remove
  the budget).
- Validate that the transaction wrapping in the PATCH handlers correctly uses `defer tx.Rollback()`
  and only commits if all steps succeed (same pattern as `RecordCostEvent`).
- `CheckResumeAllowed` must query spend within the same transaction snapshot to avoid
  TOCTOU races; ensure `qtx` is passed, not the raw `h.queries`.

#### Acceptance Criteria

- [ ] `POST /api/agents/{id}/transition` to active returns 409 when budget is still exceeded.
- [ ] Transition to active is allowed after budget is increased above current spend.
- [ ] `PATCH /api/agents/{id}` with a lowered budget auto-pauses the agent if spend exceeds new limit.
- [ ] `PATCH /api/agents/{id}` with a raised budget does NOT auto-resume the agent (REQ-027).
- [ ] `PATCH /api/squads/{id}/budgets` with a lowered budget pauses all over-budget agents.
- [ ] All mutations are atomic (budget update + re-evaluation in one transaction).

#### Files to Create / Modify

- **Modify:** `internal/server/handlers/agent_handler.go`
  (inject budgetSvc, add resume guard, wrap PATCH in TX with ReEvaluateAgent)
- **Modify:** `internal/server/handlers/squad_handler.go`
  (inject budgetSvc, wrap PATCH budgets in TX with ReEvaluateSquad)
- **Modify:** `cmd/ari/run.go` (pass budgetSvc to agent + squad handlers)
- **Modify:** `internal/server/handlers/cost_integration_test.go` (add new test cases)

---

### [SKIPPED] Task 8 — React Budget UI Components

> **Note:** Skipped — backend-only implementation focus. No files exist under `web/src/features/costs/`. React UI components (BudgetBar, CostSummaryCard, AgentCostTable, use-cost-summary hooks) remain unimplemented.

**Requirements:** REQ-009, REQ-010, REQ-012 (UI surface for budget data)

**Estimated time:** 60 min

#### RED — Write Failing Test

This task uses component-level tests (vitest + @testing-library/react). Create
`web/src/features/costs/__tests__/budget-bar.test.tsx` and
`web/src/features/costs/__tests__/cost-summary-card.test.tsx`:

```
BudgetBar
  - renders with spend=0, budget=1000: bar width ~ 0%, color class "ok"
  - renders with spend=800, budget=1000: bar width ~80%, color class "warning"
  - renders with spend=1000, budget=1000: bar width 100%, color class "exceeded"
  - renders with budget=null: shows "No budget set" text, no progress bar

CostSummaryCard
  - renders squad name, spend formatted as dollars (e.g., "$1.25" for 125 cents)
  - renders period label "March 2026"
  - renders BudgetBar with correct percentage when budget is set
  - renders "No budget set" when budgetMonthlyCents is null

AgentCostRow
  - renders agentName, spend amount, threshold status badge
  - badge has correct variant: "ok" → neutral, "warning" → yellow, "exceeded" → red
```

Run `npm test --prefix web` → tests fail (components do not exist).

#### GREEN — Implement Minimum Code

Create `web/src/features/costs/` directory with:

1. **`budget-bar.tsx`** — A progress bar component:
   - Props: `spendCents: number`, `budgetCents: number | null`
   - If `budgetCents` is null or 0: render `<span>No budget set</span>`
   - Otherwise: render a `<div>` progress bar using Tailwind width classes
   - Color: green for `ok` (< 80%), yellow for `warning` (80–99%), red for `exceeded` (100%+)

2. **`cost-summary-card.tsx`** — Squad-level spend summary:
   - Props: `squadId: string`, `spendCents: number`, `budgetMonthlyCents: number | null`,
     `percentUtilised: number | null`, `thresholdStatus: string`,
     `periodStart: string`, `periodEnd: string`
   - Displays spend in dollars (cents / 100), formatted to 2 decimal places
   - Shows billing period as "Month YYYY" label
   - Embeds `<BudgetBar>`

3. **`agent-cost-table.tsx`** — Per-agent breakdown table:
   - Props: `items: AgentCostSummary[]`, `periodStart: string`, `periodEnd: string`
   - Each row: agent name, spend (dollars), budget (dollars or "—"), threshold status badge
   - Threshold badge: shadcn/ui `Badge` with variant based on status

4. **`use-cost-summary.ts`** — React Query hook:
   - `useCostSummary(squadId: string)` → fetches `GET /api/squads/{id}/costs/summary`
   - `useAgentCostBreakdown(squadId: string)` → fetches `GET /api/squads/{id}/costs/by-agent`

5. **Add to `dashboard-page.tsx`** — render `<CostSummaryCard>` using `useCostSummary` for the
   current squad (if a squad is selected in context).

Run `npm test --prefix web` → component tests pass.
Run `make ui-build` → no TypeScript errors.

#### REFACTOR

- Extract a `formatCents(cents: number): string` utility to `web/src/lib/format.ts` and reuse
  in both `CostSummaryCard` and `AgentCostTable`.
- Ensure `BudgetBar` uses `aria-valuenow` / `aria-valuemin` / `aria-valuemax` for accessibility.
- Add loading skeleton states to `CostSummaryCard` and `AgentCostTable` for when the query
  is in flight.

#### Acceptance Criteria

- [ ] `BudgetBar` renders correct width and color class at 0%, 80%, 100% spend.
- [ ] `BudgetBar` shows "No budget set" when `budgetCents` is null.
- [ ] `CostSummaryCard` formats spend in USD dollars (not raw cents).
- [ ] `AgentCostTable` shows a status badge with correct color per threshold tier.
- [ ] `useCostSummary` and `useAgentCostBreakdown` hooks call the correct API URLs.
- [ ] Dashboard page renders the `CostSummaryCard` widget.
- [ ] `make ui-build` produces zero TypeScript errors.

#### Files to Create / Modify

- **Create:** `web/src/features/costs/budget-bar.tsx`
- **Create:** `web/src/features/costs/cost-summary-card.tsx`
- **Create:** `web/src/features/costs/agent-cost-table.tsx`
- **Create:** `web/src/features/costs/use-cost-summary.ts`
- **Create:** `web/src/features/costs/__tests__/budget-bar.test.tsx`
- **Create:** `web/src/features/costs/__tests__/cost-summary-card.test.tsx`
- **Modify:** `web/src/features/dashboard/dashboard-page.tsx`
- **Modify:** `web/src/lib/format.ts` (add `formatCents`)

---

### [x] Task 9 — End-to-End: Full Budget Lifecycle Verification

> **Note:** Lifecycle tests implemented within `internal/server/handlers/cost_integration_test.go` rather than as a separate `cost_budget_lifecycle_test.go` file. Tests cover agent budget lifecycle (record → warn → pause → block resume → increase → resume), squad enforcement, and budget enforcement at 80%/100% thresholds.

**Requirements:** REQ-001 through REQ-037 (full acceptance criteria pass)

**Estimated time:** 45 min

#### RED — Write Failing Test

Add `internal/server/handlers/cost_budget_lifecycle_test.go` (integration tests):

```
TestBudgetLifecycle_AgentFullCycle
  Purpose: verify the complete agent budget lifecycle end-to-end.
  Steps:
  1. Create squad (budget_monthly_cents = null), create owner user, create agent (active).
  2. Record cost event: 79 cents → agent still active.
  3. Record cost event: 1 cent (total = 80) → agent still active (warning only).
  4. Record cost event: 20 cents (total = 100) → agent auto-paused.
  5. GET /api/squads/{id}/costs/summary → spendCents=100, thresholdStatus="exceeded".
  6. Attempt POST /api/agents/{id}/transition {status:"active"} → 409 BUDGET_EXCEEDED.
  7. PATCH /api/agents/{id} budgetMonthlyCents=200 → 200 OK.
  8. GET /api/agents/{id} → still paused (no auto-resume).
  9. POST /api/agents/{id}/transition {status:"active"} → 200 OK (budget now allows resume).
  10. GET /api/agents/{id} → status=active.

TestBudgetLifecycle_SquadFullCycle
  Purpose: verify squad-level enforcement end-to-end.
  Steps:
  1. Create squad (budget_monthly_cents=150), two agents (running + idle).
  2. Record 100 cents for agent A → squad total = 100, both agents still running/idle.
  3. Record 50 cents for agent B → squad total = 150, both agents auto-paused.
  4. GET /api/squads/{id}/costs/by-agent → both agents show paused-related spend.
  5. PATCH /api/squads/{id}/budgets budgetMonthlyCents=300 → 200 OK.
  6. GET both agents → still paused (REQ-027).
  7. Manually resume both agents via transition endpoint → 200 OK each.

TestCostEvents_Immutability
  - POST a cost event, capture the event id.
  - Attempt PUT /api/squads/{id}/cost-events/{event-id} → 405 Method Not Allowed.
  - Attempt DELETE /api/squads/{id}/cost-events/{event-id} → 405 Method Not Allowed.
  - Assert no update or delete routes exist (REQ-018).

TestCostEvents_CrossSquadIsolation
  - Create two squads, each with an agent.
  - Record events for squad A.
  - GET /api/squads/{squad-B-id}/costs/summary → spendCents=0 (squad B sees nothing from A).
  - Attempt to record event for squad A agent via squad B URL → 403.
```

Run `make test` → at least some of these tests fail if the lifecycle has gaps.

#### GREEN — Fix Any Gaps Identified

This task is validation-focused. If all previous tasks were implemented correctly, most tests
should pass immediately. Address any gaps:

- If `TestCostEvents_Immutability` reveals a wildcard route that matches DELETE/PUT, ensure
  the router only registers the four explicitly named routes.
- If cross-squad isolation fails, verify `GetSquadMonthlySpend` uses `squad_id` in the WHERE clause.
- If the squad lifecycle resume test fails, re-check that `CheckResumeAllowed` queries the
  squad's spend with the up-to-date budget value after the PATCH.

#### REFACTOR

- Run `make test` to confirm all 9 tasks' tests pass together without interference.
- Run `make build` to confirm the binary compiles cleanly.
- Run `make ui-build` to confirm the frontend compiles cleanly.
- Review SQL query plans (via `EXPLAIN`) for `GetAgentMonthlySpend` and `GetSquadMonthlySpend`
  to confirm indexes are being used (not required to automate, but worth logging in comments).

#### Acceptance Criteria

- [ ] Complete agent budget lifecycle (record → warn → pause → block resume → increase → resume) passes.
- [ ] Complete squad budget lifecycle (record → pause all → increase → manual resume) passes.
- [ ] No UPDATE or DELETE routes exist for cost events (405 on any attempt).
- [ ] Cross-squad spend data is completely isolated (REQ-017).
- [ ] `make test` passes all tests with zero failures.
- [ ] `make build` produces a clean binary.
- [ ] `make ui-build` produces zero TypeScript errors.

#### Files to Create / Modify

- **Create:** `internal/server/handlers/cost_budget_lifecycle_test.go`
- **Fix any gaps** identified in prior tasks.

---

## Requirement Coverage Matrix

| Requirement | Task(s) |
|-------------|---------|
| REQ-001 (record cost event) | Task 2, Task 4, Task 6 |
| REQ-002 (compute running monthly spend) | Task 2, Task 4 |
| REQ-003 (soft alert at 80% — agent) | Task 4 (TODO stub), Task 9 |
| REQ-004 (auto-pause at 100% — agent) | Task 4, Task 6, Task 9 |
| REQ-005 (soft alert at 80% — squad) | Task 4 (TODO stub) |
| REQ-006 (auto-pause at 100% — squad) | Task 4, Task 6, Task 9 |
| REQ-007 (re-evaluate on agent budget PATCH) | Task 5, Task 7 |
| REQ-008 (re-evaluate on squad budget PATCH) | Task 5, Task 7 |
| REQ-009 (GET costs/summary) | Task 6, Task 9 |
| REQ-010 (GET costs/by-agent) | Task 2, Task 6, Task 9 |
| REQ-011 (POST cost-events) | Task 4, Task 6 |
| REQ-012 (GET agent/me/budget — stub) | Task 6 |
| REQ-013 (accept events while running) | Task 6 |
| REQ-014 (block resume while budget exceeded) | Task 5, Task 7, Task 9 |
| REQ-015 (compute spend via SUM, not cache) | Task 2, Task 4 |
| REQ-016 (store cost_cents as non-negative int) | Task 1, Task 3 |
| REQ-017 (squad-scoped reads) | Task 2, Task 9 |
| REQ-018 (immutable — no UPDATE/DELETE) | Task 1, Task 9 |
| REQ-019 (atomic transaction) | Task 4, Task 6 |
| REQ-020 (agent_id on every event) | Task 1, Task 2 |
| REQ-021 (optional allocation fields) | Task 1, Task 2, Task 3 |
| REQ-022 (UTC calendar month) | Task 3 |
| REQ-023 (skip enforcement if no agent budget) | Task 4, Task 5 |
| REQ-024 (skip squad enforcement if no squad budget) | Task 4, Task 5 |
| REQ-025 (allow resume after budget increase) | Task 5, Task 7, Task 9 |
| REQ-026 (resolve warning after budget increase) | Task 5 (TODO stub) |
| REQ-027 (no auto-resume — manual required) | Task 5, Task 7, Task 9 |
| REQ-028 (optional token fields) | Task 1, Task 2, Task 3 |
| REQ-029 (reject terminated agent — 409) | Task 6 |
| REQ-030 (reject cross-squad — 403) | Task 6, Task 9 |
| REQ-031 (indexes for performance) | Task 1 |
| REQ-032 (TX under 500ms at p95) | Task 1 (indexes), Task 4 |
| REQ-033 (Run Token auth — interim session JWT) | Task 6 |
| REQ-034 (squad membership for reads) | Task 6 |
| REQ-035 (owner role for budget PATCH) | Task 7 |
| REQ-036 (rollback on enforcement failure) | Task 4, Task 6 |
| REQ-037 (idempotent submission) | Task 2, Task 4, Task 6 |

---

## Notes

### Blockers

- **Inbox/alert system** (REQ-003, REQ-005, REQ-026): The inbox table does not yet exist.
  All soft-alert logic is stubbed with `// TODO(inbox):` comments and `slog.Info` fallbacks.
  These stubs are fully covered in Tasks 4 and 5 and will be wired when the inbox feature lands.
- **Run Token JWT** (REQ-012, REQ-033): Feature 11 implements the Run Token. The
  `GET /api/agent/me/budget` endpoint returns 501 until then. `POST /api/squads/:id/cost-events`
  accepts session JWT as an interim measure.
- **Activity log** (REQ-004, REQ-006): Feature 09 implements the activity log.
  `agent.budget_paused` entries are stubbed as `// TODO(feature-09):` and fall back to `slog.Info`.

### Future Improvements

- Wire activity log entries (feature 09) in `RecordAndEnforce` and `ReEvaluateAgent`.
- Wire inbox alert creation (future inbox feature) at 80% threshold.
- Implement `GET /api/agent/me/budget` fully once Run Token JWT is available (feature 11).
- Cost forecasting and multi-currency support are explicitly out of scope for v1.
