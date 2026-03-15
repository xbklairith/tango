# Tasks: Agent Self-Service API (Extended)

**Feature:** 15-agent-self-service-extended
**Created:** 2026-03-15
**Status:** Complete

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-ASS-001 through REQ-ASS-093

## Implementation Approach

Work bottom-up through the dependency graph: new sqlc queries and indexes first (assignment, goal-linked, and squad-scoped children queries), then the terminated agent guard and role-based scoping helper (`resolveAgentIDs`), then each GET endpoint (assignments, team, budget, goals), then POST endpoints (inbox creation, cost reporting via BudgetEnforcementService), and finally integration tests covering all roles and edge cases. All endpoints extend the existing `AgentSelfHandler` (with new `BudgetEnforcementService` dependency) and reuse the existing Run Token middleware -- no new middleware is required; one migration is needed for indexes.

## Progress Summary

- Total Tasks: 10
- Completed: 9/10
- In Progress: Task 10 (integration tests deferred)
- Test Coverage: Existing tests pass; integration tests pending

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 01 — SQL Queries, Indexes, and ListAgentChildrenBySquad

**Requirements:** REQ-ASS-001, REQ-ASS-002, REQ-ASS-003, REQ-ASS-004, REQ-ASS-005, REQ-ASS-030, REQ-ASS-031, REQ-ASS-032, REQ-ASS-034
**Estimated time:** 60 min

#### Context

Add new sqlc queries for listing/counting assignments by single agent and by agent ID arrays (for lead role), for listing goal-linked issues by agent, agent IDs, and squad, a batch `GetGoalsByIDs` query (H-3 fix), and a squad-scoped `ListAgentChildrenBySquad` query (C-1 fix). Also add a goose migration for new indexes (H-5, DB fixes).

#### RED — Write Failing Tests

Write `internal/database/db/agent_self_queries_test.go`:

1. `TestListAssignmentsByAgent` — insert issues with various statuses/types for an agent, verify correct filtering by status, type, and pagination (limit/offset).
2. `TestCountAssignmentsByAgent` — verify count matches filtered results.
3. `TestListAssignmentsByAgentIDs` — insert issues for multiple agents, query with an array of agent IDs, verify all matching issues returned with correct filtering.
4. `TestCountAssignmentsByAgentIDs` — verify count matches filtered results for multi-agent query.
5. `TestListGoalLinkedIssuesByAgent` — insert issues with and without `goal_id`, verify only issues with non-null `goal_id` returned, ordered by `goal_id` then `created_at DESC`.
6. `TestListGoalLinkedIssuesByAgentIDs` — verify multi-agent variant returns correct results.
7. `TestListGoalLinkedIssuesBySquad` — verify squad-wide variant returns all issues with `goal_id` set.
8. `TestGetGoalsByIDs` — insert 3 goals, query with 2 IDs, verify only matching goals returned ordered by title.
9. `TestListAgentChildrenBySquad` — create agents with same parent across two squads, verify only children in the specified squad are returned (C-1 fix).

#### GREEN — Implement

Add query to `internal/database/queries/agents.sql`:

- `ListAgentChildrenBySquad` — filter by `squad_id` AND `parent_agent_id` (C-1 fix)

Add queries to `internal/database/queries/issues.sql`:

- `ListAssignmentsByAgent` — filter by squad_id, agent_id, optional status/type, with limit/offset
- `CountAssignmentsByAgent` — matching count query
- `ListAssignmentsByAgentIDs` — filter by squad_id, agent_ids array, optional status/type, with limit/offset
- `CountAssignmentsByAgentIDs` — matching count query

Add queries to `internal/database/queries/goals.sql`:

- `GetGoalsByIDs` — `SELECT * FROM goals WHERE id = ANY(@ids::UUID[]) ORDER BY title` (H-3 fix)
- `ListGoalLinkedIssuesByAgent` — issues with non-null goal_id for a single agent
- `ListGoalLinkedIssuesByAgentIDs` — issues with non-null goal_id for agent ID array
- `ListGoalLinkedIssuesBySquad` — all issues with non-null goal_id in squad

Add goose migration `internal/database/migrations/NNNN_add_self_service_indexes.sql`:

```sql
-- +goose Up
CREATE INDEX idx_issues_squad_assignee ON issues(squad_id, assignee_agent_id, created_at DESC);
CREATE INDEX idx_cost_events_agent_period ON cost_events(agent_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_issues_squad_assignee;
DROP INDEX IF EXISTS idx_cost_events_agent_period;
```

Run `make sqlc` to regenerate Go code.

#### REFACTOR

Verify generated code compiles cleanly. Confirm query parameter types match expected Go types (especially `UUID[]` array handling).

#### Files

- Modify: `internal/database/queries/agents.sql`
- Modify: `internal/database/queries/issues.sql`
- Modify: `internal/database/queries/goals.sql`
- Create: `internal/database/migrations/NNNN_add_self_service_indexes.sql`
- Regenerate: `internal/database/db/` (via `make sqlc`)
- Create: `internal/database/db/agent_self_queries_test.go`

---

### [x] Task 02 — Terminated Agent Guard and Role-Based Scoping Helper

**Requirements:** REQ-ASS-070, REQ-ASS-071, REQ-ASS-072, REQ-ASS-073, REQ-ASS-074
**Estimated time:** 30 min

#### Context

Implement the `requireActiveAgent` guard that checks the agent's DB status and returns 403 for terminated agents (H-7 fix). Also implement the `resolveAgentIDs` method on `AgentSelfHandler` that determines which agent IDs to scope queries to based on the authenticated agent's role. Captain returns nil (squad-wide), lead returns self + direct children (using `ListAgentChildrenBySquad`), member returns self only.

#### RED — Write Failing Tests

Write `internal/server/handlers/agent_self_resolve_test.go`:

1. `TestRequireActiveAgent_Active` — verify active agent returns successfully.
2. `TestRequireActiveAgent_Terminated` — verify terminated agent returns 403 with "Agent terminated".
3. `TestRequireActiveAgent_Paused` — verify paused agent is allowed (not terminated).
4. `TestResolveAgentIDs_Captain` — verify nil slice returned (signals squad-wide).
5. `TestResolveAgentIDs_Lead` — set up lead with 3 children, verify returned slice contains lead ID + all 3 child IDs.
6. `TestResolveAgentIDs_Lead_NoChildren` — lead with no children, verify only lead ID returned.
7. `TestResolveAgentIDs_Member` — verify single-element slice with agent's own ID.

#### GREEN — Implement

Add to `internal/server/handlers/agent_self_handler.go`:

```go
func (h *AgentSelfHandler) requireActiveAgent(ctx context.Context, w http.ResponseWriter, agentID uuid.UUID) (db.Agent, bool)
```

- Fetches agent by ID
- If status == "terminated", writes 403 response and returns `(agent, false)`
- Otherwise returns `(agent, true)`

```go
func (h *AgentSelfHandler) resolveAgentIDs(ctx context.Context, identity auth.AgentIdentity) ([]uuid.UUID, error)
```

- `captain`: return `nil, nil`
- `lead`: fetch `ListAgentChildrenBySquad(squad_id, agent.ID)`, build `[]uuid.UUID{identity.AgentID, child1.ID, ...}`
- `member`: return `[]uuid.UUID{identity.AgentID}, nil`

#### REFACTOR

Ensure both helpers are methods on the handler so they have access to `h.queries`.

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go`
- Create: `internal/server/handlers/agent_self_resolve_test.go`

---

### [x] Task 03 — GET /api/agent/me/assignments Endpoint

**Requirements:** REQ-ASS-001, REQ-ASS-002, REQ-ASS-003, REQ-ASS-004, REQ-ASS-005, REQ-ASS-006, REQ-ASS-070, REQ-ASS-071, REQ-ASS-072, REQ-ASS-090, REQ-ASS-091, REQ-ASS-092, REQ-ASS-093
**Estimated time:** 45 min

#### Context

Implement the assignments endpoint that returns issues assigned to the authenticated agent (or subtree for lead, all assigned for captain) with optional status/type filtering and pagination. Uses `resolveAgentIDs` to determine scope and dispatches to the appropriate query variant. Includes `pipelineId` and `currentStageId` fields in the response (XH-1 fix).

#### RED — Write Failing Tests

Write `internal/server/handlers/agent_self_assignments_test.go`:

1. `TestGetAssignments_MemberOwnIssues` — member gets only own assignments.
2. `TestGetAssignments_FilterByStatus` — pass `?status=in_progress`, verify only matching issues returned.
3. `TestGetAssignments_FilterByType` — pass `?type=task`, verify filtering.
4. `TestGetAssignments_Pagination` — insert 5 issues, request `?limit=2&offset=2`, verify 2 items returned with total=5.
5. `TestGetAssignments_LimitClamped` — pass `?limit=200`, verify clamped to 100.
6. `TestGetAssignments_DefaultLimit` — omit limit, verify default of 50 applied.
7. `TestGetAssignments_LeadSeesChildren` — lead with children who have assignments, verify all included.
8. `TestGetAssignments_CaptainSeesAllAssigned` — captain sees all assigned squad issues (H-1 clarification).
9. `TestGetAssignments_NoRunToken` — request without token returns 401.
10. `TestGetAssignments_TerminatedAgent` — terminated agent returns 403.
11. `TestGetAssignments_ResponseShape` — verify JSON keys match camelCase spec (id, identifier, type, title, status, priority, pipelineId, currentStageId, etc.).

#### GREEN — Implement

Add to `internal/server/handlers/agent_self_handler.go`:

- `GetAssignments(w, r)` handler method
- Call `requireActiveAgent` at the start
- Parse query params: `status`, `type`, `limit` (default 50, max 100), `offset` (default 0)
- Call `resolveAgentIDs` to determine scope
- If nil (captain): use existing squad-level issue queries
- If single ID (member): use `ListAssignmentsByAgent` / `CountAssignmentsByAgent`
- If multiple IDs (lead): use `ListAssignmentsByAgentIDs` / `CountAssignmentsByAgentIDs`
- Build `assignmentsResponse` with `assignments` array (including `pipelineId`, `currentStageId`) and `total` count
- Register route in `RegisterRoutes`

#### REFACTOR

Extract query param parsing helpers if reusable. Ensure consistent error responses for invalid params.

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go`
- Create: `internal/server/handlers/agent_self_assignments_test.go`

---

### [x] Task 04 — GET /api/agent/me/team Endpoint

**Requirements:** REQ-ASS-010, REQ-ASS-011, REQ-ASS-012, REQ-ASS-013, REQ-ASS-014, REQ-ASS-070, REQ-ASS-074, REQ-ASS-091, REQ-ASS-092, REQ-ASS-093
**Estimated time:** 45 min

#### Context

Implement the team endpoint that returns the agent's immediate team context based on hierarchy and role. Members see parent + siblings, leads add children, captains see only `self` + `allAgents` (M-1 fix: simplified captain response omits parent/siblings/children). Uses `ListAgentChildrenBySquad` (C-1 fix) instead of `ListAgentChildren`.

#### RED — Write Failing Tests

Write `internal/server/handlers/agent_self_team_test.go`:

1. `TestGetTeam_Member` — member with parent and siblings, verify `self`, `parent`, `siblings` populated, `children` empty.
2. `TestGetTeam_Member_NoParent` — top-level member (edge case), verify `parent` is null.
3. `TestGetTeam_Lead` — lead with parent, siblings, and children, verify all fields populated.
4. `TestGetTeam_Lead_NoChildren` — lead with no direct children, verify `children` is empty array.
5. `TestGetTeam_Captain` — captain, verify only `self` and `allAgents` fields returned; `parent`, `siblings`, `children` are omitted (M-1 fix).
6. `TestGetTeam_AgentFieldShape` — verify each agent entry contains: id, name, shortName, role, status, parentAgentId.
7. `TestGetTeam_SiblingsExcludesSelf` — verify agent does not appear in its own siblings list.
8. `TestGetTeam_TerminatedAgent` — terminated agent returns 403.
9. `TestGetTeam_NoRunToken` — request without token returns 401.

#### GREEN — Implement

Add to `internal/server/handlers/agent_self_handler.go`:

- `GetTeam(w, r)` handler method
- Call `requireActiveAgent` at the start
- Fetch self agent via `GetAgentByID`
- Role-based logic:
  - `member`: fetch parent (if `parent_agent_id` set), fetch siblings via `ListAgentChildrenBySquad(squad_id, parent_id)` excluding self
  - `lead`: same as member + fetch children via `ListAgentChildrenBySquad(squad_id, self.ID)`
  - `captain`: fetch all via `ListAgentsBySquad`, return only `self` + `allAgents`
- Build `teamResponse` with `teamAgent` structs
- Register route in `RegisterRoutes`

#### REFACTOR

Extract `toTeamAgent(agent db.Agent) teamAgent` mapper function for DRY conversion.

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go`
- Create: `internal/server/handlers/agent_self_team_test.go`

---

### [x] Task 05 — GET /api/agent/me/budget Endpoint

**Requirements:** REQ-ASS-020, REQ-ASS-021, REQ-ASS-022, REQ-ASS-023, REQ-ASS-024, REQ-ASS-073, REQ-ASS-074, REQ-ASS-091, REQ-ASS-092, REQ-ASS-093
**Estimated time:** 30 min

#### Context

Implement the budget endpoint that returns the agent's spend, budget limit, remaining, threshold status, and billing period. Always returns only the requesting agent's own budget regardless of role. Uses existing `GetAgentByID` and `GetAgentMonthlySpend` queries. Reuses `domain.BillingPeriod()` and `domain.ComputeThreshold()` directly (M-2/M-3 fix: no local utility function duplication).

#### RED — Write Failing Tests

Write `internal/server/handlers/agent_self_budget_test.go`:

1. `TestGetBudget_WithLimit` — agent with budget_monthly_cents=10000 and spent=4200, verify spentCents=4200, budgetCents=10000, remainingCents=5800, thresholdStatus="ok".
2. `TestGetBudget_NullLimit` — agent with no budget set, verify budgetCents=null, remainingCents=null, thresholdStatus="ok".
3. `TestGetBudget_Warning80Percent` — agent at exactly 80% spend, verify thresholdStatus="warning".
4. `TestGetBudget_Exceeded100Percent` — agent at 100% spend, verify thresholdStatus="exceeded".
5. `TestGetBudget_Over100Percent` — agent over 100% spend, verify thresholdStatus="exceeded", remainingCents is negative (or zero).
6. `TestGetBudget_PeriodDates` — verify periodStart is first of current month UTC, periodEnd is first of next month UTC.
7. `TestGetBudget_CaptainOwnBudgetOnly` — captain role still returns only own budget, not squad-wide.
8. `TestGetBudget_TerminatedAgent` — terminated agent returns 403.
9. `TestGetBudget_NoRunToken` — request without token returns 401.

#### GREEN — Implement

Add to `internal/server/handlers/agent_self_handler.go`:

- `GetBudget(w, r)` handler method
- Call `requireActiveAgent` at the start
- Use `domain.BillingPeriod(time.Now())` for period calculation (M-2 fix)
- Fetch agent via `GetAgentByID`, get spend via `GetAgentMonthlySpend` for current period
- Use `domain.ComputeThreshold(budgetCentsPtr, spentCents)` for threshold (M-3 fix)
- Compute remainingCents (nil if no budget)
- Build `budgetResponse` and return
- Register route in `RegisterRoutes`

#### REFACTOR

No local `budgetThreshold()` or `currentBillingPeriod()` utility functions needed. The domain package provides both.

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go`
- Create: `internal/server/handlers/agent_self_budget_test.go`

---

### [x] Task 06 — GET /api/agent/me/goals Endpoint

**Requirements:** REQ-ASS-030, REQ-ASS-031, REQ-ASS-032, REQ-ASS-033, REQ-ASS-034, REQ-ASS-070, REQ-ASS-071, REQ-ASS-072, REQ-ASS-074, REQ-ASS-091, REQ-ASS-092, REQ-ASS-093
**Estimated time:** 45 min

#### Context

Implement the goals endpoint that returns goals linked to the agent's assigned issues via `goal_id`. Goals are deduplicated, each listing the related issue identifiers. Uses `resolveAgentIDs` for role-based scoping, the goal-linked queries from Task 01, and the batch `GetGoalsByIDs` query (H-3 fix: avoids N+1 per-goal lookups).

#### RED — Write Failing Tests

Write `internal/server/handlers/agent_self_goals_test.go`:

1. `TestGetGoals_MemberOwnGoals` — member with 2 issues referencing different goals, verify both goals returned with correct related issue identifiers.
2. `TestGetGoals_Deduplicated` — member with 3 issues, 2 referencing the same goal, verify goal appears once with both issue identifiers listed.
3. `TestGetGoals_NoGoals` — member with issues that have no goal_id, verify empty goals array.
4. `TestGetGoals_LeadSeesChildrenGoals` — lead sees goals from own and children's issues.
5. `TestGetGoals_CaptainSeesAllGoals` — captain sees all squad goals (uses `ListGoalsBySquad`).
6. `TestGetGoals_ResponseShape` — verify each goal has: id, title, description, status, relatedIssues.
7. `TestGetGoals_BatchFetch` — verify goals are fetched via `GetGoalsByIDs` (not per-goal lookup).
8. `TestGetGoals_TerminatedAgent` — terminated agent returns 403.
9. `TestGetGoals_NoRunToken` — request without token returns 401.

#### GREEN — Implement

Add to `internal/server/handlers/agent_self_handler.go`:

- `GetGoals(w, r)` handler method
- Call `requireActiveAgent` at the start
- Call `resolveAgentIDs` to determine scope
- If nil (captain): fetch all goals via `ListGoalsBySquad`, fetch all goal-linked issues via `ListGoalLinkedIssuesBySquad`
- If single/multiple IDs: use `ListGoalLinkedIssuesByAgent` or `ListGoalLinkedIssuesByAgentIDs`
- Collect unique goal IDs, fetch ALL via `GetGoalsByIDs` in one batch (H-3 fix: required, not optional)
- Group issue identifiers by goal ID, build `goalsResponse`
- Register route in `RegisterRoutes`

#### REFACTOR

Verify no N+1 queries remain. `GetGoalsByIDs` must be the only mechanism for fetching goal records.

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go`
- Create: `internal/server/handlers/agent_self_goals_test.go`

---

### [x] Task 07 — POST /api/agent/me/inbox Endpoint

**Requirements:** REQ-ASS-040, REQ-ASS-041, REQ-ASS-042, REQ-ASS-043, REQ-ASS-044, REQ-ASS-074, REQ-ASS-091, REQ-ASS-092, REQ-ASS-093
**Estimated time:** 45 min

#### Context

Implement the inbox creation endpoint that lets agents create inbox items to request human assistance. Validates category (rejects `alert`), maps fields from the Run Token identity. SSE event is `inbox.item.created` (XC-2 fix: reuses InboxService naming, NOT a custom `inbox.agent_created`). Activity log action is `inbox.item.created`. The inbox `type` field is set to `"agent_request"` which is valid as free-text (M-5 note).

#### RED — Write Failing Tests

Write `internal/server/handlers/agent_self_inbox_test.go`:

1. `TestCreateInbox_Question` — POST valid question, verify 201 with correct field mapping (squad_id, requested_by_agent_id, related_agent_id, related_run_id from token).
2. `TestCreateInbox_Approval` — POST approval category, verify success.
3. `TestCreateInbox_Decision` — POST decision category, verify success.
4. `TestCreateInbox_AlertRejected` — POST with category=alert, verify 400 with code `VALIDATION_ERROR` and message about agents not being allowed to create alerts.
5. `TestCreateInbox_MissingTitle` — POST without title, verify 400.
6. `TestCreateInbox_TitleTooLong` — POST with 501-char title, verify 400.
7. `TestCreateInbox_DefaultUrgency` — POST without urgency, verify defaults to "normal".
8. `TestCreateInbox_WithOptionalFields` — POST with urgency, relatedIssueId, and payload, verify all persisted.
9. `TestCreateInbox_RelatedIssueWrongSquad` — POST with relatedIssueId from different squad, verify 403.
10. `TestCreateInbox_RelatedIssueNotFound` — POST with nonexistent relatedIssueId, verify 404.
11. `TestCreateInbox_SSEEmitted` — verify `inbox.item.created` SSE event emitted (XC-2 fix).
12. `TestCreateInbox_ActivityLogged` — verify activity entry with action `inbox.item.created`.
13. `TestCreateInbox_TerminatedAgent` — terminated agent returns 403.
14. `TestCreateInbox_NoRunToken` — request without token returns 401.

#### GREEN — Implement

Add to `internal/server/handlers/agent_self_handler.go`:

- `CreateInboxItem(w, r)` handler method
- Call `requireActiveAgent` at the start
- Parse and validate request body (category, title, body, urgency, relatedIssueId, payload)
- Reject `alert` category with 400
- If `relatedIssueId` provided, validate issue exists and belongs to agent's squad
- Begin transaction, insert inbox item, log activity (`inbox.item.created`), commit
- Emit SSE `inbox.item.created` (XC-2 fix)
- Return 201 with created item
- Register route in `RegisterRoutes`

#### REFACTOR

Reuse existing `InboxService.Create()` if available from Feature 12, passing appropriate parameters. Otherwise implement directly.

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go`
- Create: `internal/server/handlers/agent_self_inbox_test.go`

---

### [x] Task 08 — POST /api/agent/me/cost Endpoint

**Requirements:** REQ-ASS-060, REQ-ASS-061, REQ-ASS-062, REQ-ASS-063, REQ-ASS-064, REQ-ASS-065, REQ-ASS-073, REQ-ASS-074, REQ-ASS-091, REQ-ASS-092, REQ-ASS-093
**Estimated time:** 45 min

#### Context

Implement the cost reporting endpoint that lets agents self-report external tool usage costs. Delegates to `BudgetEnforcementService.RecordAndEnforce()` (C-3/C-4 fix) for cost recording, budget threshold checking, auto-pause, and inbox alert creation. This automatically handles both agent-level and squad-level budget enforcement. Per-event maximum of 100000 cents ($1000) is enforced (C-2 fix).

#### Acceptance Criteria

- `amountCents` must be > 0 AND <= 100000 (C-2 fix: per-event maximum)
- Cost event is recorded via `BudgetEnforcementService.RecordAndEnforce()` (C-3 fix)
- Both agent-level AND squad-level budget thresholds are enforced (C-4 fix)
- SSE event name is `cost.event.created` (XC-2 fix, NOT `cost.agent_reported`)
- Activity log action is `cost.event.created`
- Terminated agents receive 403

#### RED — Write Failing Tests

Write `internal/server/handlers/agent_self_cost_test.go`:

1. `TestReportCost_Success` — POST valid cost event, verify 201 with agent_id and squad_id from token.
2. `TestReportCost_AllFields` — POST with all optional fields (inputTokens, outputTokens, metadata), verify persisted.
3. `TestReportCost_DefaultOptionals` — POST without optional fields, verify defaults (inputTokens=0, outputTokens=0, metadata={}).
4. `TestReportCost_ZeroAmount` — POST with amountCents=0, verify 400.
5. `TestReportCost_NegativeAmount` — POST with amountCents=-5, verify 400.
6. `TestReportCost_ExceedsPerEventMax` — POST with amountCents=100001, verify 400 with message about per-event maximum (C-2 fix).
7. `TestReportCost_AtPerEventMax` — POST with amountCents=100000, verify 201 (boundary).
8. `TestReportCost_MissingEventType` — POST without eventType, verify 400.
9. `TestReportCost_EventTypeTooLong` — POST with 51-char eventType, verify 400.
10. `TestReportCost_MissingModel` — POST without model, verify 400.
11. `TestReportCost_ModelTooLong` — POST with 101-char model, verify 400.
12. `TestReportCost_DelegatesToBudgetService` — verify `BudgetEnforcementService.RecordAndEnforce()` is called (C-3 fix).
13. `TestReportCost_Triggers80PercentAlert` — agent at 79% budget, report cost that crosses 80%, verify inbox alert created (via BudgetEnforcementService).
14. `TestReportCost_Triggers100PercentAlert` — agent at 99% budget, report cost that crosses 100%, verify inbox alert created and agent auto-paused (via BudgetEnforcementService).
15. `TestReportCost_SquadBudgetEnforced` — verify squad-level budget is also checked (C-4 fix, handled by BudgetEnforcementService).
16. `TestReportCost_SSEEmitted` — verify `cost.event.created` SSE event emitted (XC-2 fix).
17. `TestReportCost_ActivityLogged` — verify activity entry with action `cost.event.created`.
18. `TestReportCost_TerminatedAgent` — terminated agent returns 403.
19. `TestReportCost_NoRunToken` — request without token returns 401.

#### GREEN — Implement

Add to `internal/server/handlers/agent_self_handler.go`:

- `ReportCost(w, r)` handler method
- Call `requireActiveAgent` at the start
- Parse and validate request body:
  - `amountCents` > 0 AND <= 100000 (C-2 fix)
  - `eventType` 1-50 chars
  - `model` 1-100 chars
  - Optional: inputTokens, outputTokens, metadata
- Build `db.InsertCostEventParams` with agent_id and squad_id from token
- Call `h.budgetService.RecordAndEnforce(ctx, params)` (C-3/C-4 fix)
- Log activity: action `cost.event.created`, entity_type `cost_event`
- Emit SSE `cost.event.created` (XC-2 fix)
- Return 201 with `result.CostEvent`
- Register route in `RegisterRoutes`

#### REFACTOR

Verify no hand-rolled transaction flow exists. All cost recording and threshold logic must go through `BudgetEnforcementService.RecordAndEnforce()`.

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go`
- Create: `internal/server/handlers/agent_self_cost_test.go`

---

### [x] Task 09 — Route Registration, Constructor Change, and Server Wiring

**Requirements:** All (integration wiring)
**Estimated time:** 30 min

#### Context

Register all new routes in `AgentSelfHandler.RegisterRoutes`, update the handler constructor to accept `BudgetEnforcementService` (H-6 fix), and update server initialization to pass it. Verify the server starts cleanly with the new routes and all existing tests still pass.

#### RED — Write Failing Tests

1. Verify that `GET /api/agent/me/assignments` is routable (returns non-404).
2. Verify that `GET /api/agent/me/team` is routable.
3. Verify that `GET /api/agent/me/budget` is routable.
4. Verify that `GET /api/agent/me/goals` is routable.
5. Verify that `POST /api/agent/me/inbox` is routable.
6. Verify that `POST /api/agent/me/cost` is routable.
7. Verify all routes return 401 without a Run Token (not 404).

#### GREEN — Implement

Update `AgentSelfHandler` struct and constructor (H-6 fix):

```go
type AgentSelfHandler struct {
    queries       *db.Queries
    dbConn        *sql.DB
    sseHub        *sse.Hub
    budgetService *BudgetEnforcementService  // NEW
}

func NewAgentSelfHandler(
    q *db.Queries,
    dbConn *sql.DB,
    sseHub *sse.Hub,
    budgetService *BudgetEnforcementService,  // NEW
) *AgentSelfHandler
```

Modify `AgentSelfHandler.RegisterRoutes`:

```go
mux.HandleFunc("GET /api/agent/me/assignments", h.GetAssignments)
mux.HandleFunc("GET /api/agent/me/team", h.GetTeam)
mux.HandleFunc("GET /api/agent/me/budget", h.GetBudget)
mux.HandleFunc("GET /api/agent/me/goals", h.GetGoals)
mux.HandleFunc("POST /api/agent/me/inbox", h.CreateInboxItem)
mux.HandleFunc("POST /api/agent/me/cost", h.ReportCost)
```

Update server initialization in `internal/server/server.go` or `cmd/ari/run.go` to pass the existing `BudgetEnforcementService` instance to `NewAgentSelfHandler`.

#### REFACTOR

Verify no route conflicts with existing endpoints. Ensure consistent middleware chain (Run Token auth) applies to all new routes. Verify all existing callers of `NewAgentSelfHandler` are updated.

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go` (struct, constructor, RegisterRoutes)
- Modify: `internal/server/server.go` or `cmd/ari/run.go` (constructor call site)

---

### [ ] Task 10 — Integration Tests: Full Endpoint Coverage

**Requirements:** REQ-ASS-080, REQ-ASS-081, REQ-ASS-082, REQ-ASS-083, REQ-ASS-090, REQ-ASS-091, REQ-ASS-092
**Estimated time:** 60 min

#### Context

Write comprehensive integration tests covering all new endpoints with real HTTP requests against a test server with embedded database. Each endpoint must be tested for success, auth failure, terminated agent guard, squad scoping, and role-based filtering. These tests validate end-to-end behavior including middleware, handler, database, and response format.

#### RED — Write Failing Tests

Create `internal/server/handlers/agent_self_extended_integration_test.go`:

**Setup fixture:**
- Create 2 squads (for cross-squad isolation tests)
- In squad 1: create captain, lead (child of captain), 2 members (children of lead)
- Create a terminated agent in squad 1 (for H-7 tests)
- Create issues assigned to each agent, some with goals
- Create cost events for budget testing
- Mint Run Tokens for each agent role (including terminated agent)

**Auth and guard tests:**
1. `TestIntegration_AllEndpoints_RejectMissingToken` — all 6 endpoints return 401 without Run Token.
2. `TestIntegration_AllEndpoints_RejectWrongSquad` — agent from squad 2 cannot access squad 1 data.
3. `TestIntegration_AllEndpoints_RejectTerminatedAgent` — terminated agent gets 403 on all endpoints (H-7 fix).

**Assignments tests:**
4. `TestIntegration_Assignments_MemberOwn` — member sees only own issues.
5. `TestIntegration_Assignments_LeadSubtree` — lead sees own + children issues.
6. `TestIntegration_Assignments_CaptainAllAssigned` — captain sees all assigned squad issues.
7. `TestIntegration_Assignments_Pagination` — verify limit, offset, total work correctly.
8. `TestIntegration_Assignments_Filters` — verify status and type filters.
9. `TestIntegration_Assignments_PipelineFields` — verify pipelineId and currentStageId in response.

**Team tests:**
10. `TestIntegration_Team_MemberView` — member sees parent + siblings, no children.
11. `TestIntegration_Team_LeadView` — lead sees parent + siblings + children.
12. `TestIntegration_Team_CaptainView` — captain sees only self + allAgents (M-1 fix).

**Budget tests:**
13. `TestIntegration_Budget_OwnOnly` — all roles see only own budget.
14. `TestIntegration_Budget_ThresholdStatuses` — verify ok/warning/exceeded thresholds (uses domain.ComputeThreshold).
15. `TestIntegration_Budget_NullBudget` — agent with no budget returns null fields.

**Goals tests:**
16. `TestIntegration_Goals_MemberOwn` — member sees only goals linked to own issues.
17. `TestIntegration_Goals_CaptainAll` — captain sees all squad goals.
18. `TestIntegration_Goals_Deduplicated` — multiple issues with same goal, goal appears once.
19. `TestIntegration_Goals_BatchFetch` — verify no N+1 (uses GetGoalsByIDs).

**Inbox creation tests:**
20. `TestIntegration_Inbox_CreateQuestion` — agent creates question inbox item, verify 201.
21. `TestIntegration_Inbox_RejectAlert` — agent cannot create alert, verify 400.
22. `TestIntegration_Inbox_Validation` — missing fields return 400.
23. `TestIntegration_Inbox_SSEEvent` — verify SSE event name is `inbox.item.created` (XC-2 fix).

**Cost reporting tests:**
24. `TestIntegration_Cost_SelfReport` — agent reports cost, verify 201 with correct agent_id.
25. `TestIntegration_Cost_PerEventMax` — amount > 100000 returns 400 (C-2 fix).
26. `TestIntegration_Cost_BudgetAlert80` — cost pushes agent past 80%, verify inbox alert created via BudgetEnforcementService.
27. `TestIntegration_Cost_BudgetAlert100` — cost pushes agent past 100%, verify inbox alert created and agent auto-paused via BudgetEnforcementService.
28. `TestIntegration_Cost_SquadBudgetEnforced` — squad-level budget enforcement works (C-4 fix).
29. `TestIntegration_Cost_ValidationErrors` — negative amount, missing fields return 400.
30. `TestIntegration_Cost_SSEEvent` — verify SSE event name is `cost.event.created` (XC-2 fix).

#### GREEN — Implement

All handler code should already be implemented in Tasks 01-09. This task validates the full stack works together. Fix any issues discovered during integration testing.

#### REFACTOR

Ensure test helpers are reusable. Extract common fixture setup. Verify all tests are independent and can run in parallel.

#### Files

- Create: `internal/server/handlers/agent_self_extended_integration_test.go`

---

## Requirement Coverage Matrix

| Requirement    | Task(s)         |
|----------------|-----------------|
| REQ-ASS-001    | 01, 03, 10      |
| REQ-ASS-002    | 01, 03, 10      |
| REQ-ASS-003    | 01, 03, 10      |
| REQ-ASS-004    | 01, 03, 10      |
| REQ-ASS-005    | 01, 03, 10      |
| REQ-ASS-006    | 03, 10          |
| REQ-ASS-010    | 04, 10          |
| REQ-ASS-011    | 04, 10          |
| REQ-ASS-012    | 04, 10          |
| REQ-ASS-013    | 04, 10          |
| REQ-ASS-014    | 04, 10          |
| REQ-ASS-020    | 05, 10          |
| REQ-ASS-021    | 05, 10          |
| REQ-ASS-022    | 05, 10          |
| REQ-ASS-023    | 05, 10          |
| REQ-ASS-024    | 05, 10          |
| REQ-ASS-030    | 01, 06, 10      |
| REQ-ASS-031    | 01, 06, 10      |
| REQ-ASS-032    | 01, 06, 10      |
| REQ-ASS-033    | 06, 10          |
| REQ-ASS-034    | 01, 06, 10      |
| REQ-ASS-040    | 07, 10          |
| REQ-ASS-041    | 07, 10          |
| REQ-ASS-042    | 07, 10          |
| REQ-ASS-043    | 07, 10          |
| REQ-ASS-044    | 07, 10          |
| REQ-ASS-050    | N/A (pre-existing) |
| REQ-ASS-060    | 08, 10          |
| REQ-ASS-061    | 08, 10          |
| REQ-ASS-062    | 08, 10          |
| REQ-ASS-063    | 08, 10          |
| REQ-ASS-064    | 08, 10          |
| REQ-ASS-065    | 08, 10          |
| REQ-ASS-070    | 02, 03, 06, 10  |
| REQ-ASS-071    | 02, 03, 06, 10  |
| REQ-ASS-072    | 02, 03, 06, 10  |
| REQ-ASS-073    | 05, 08, 10      |
| REQ-ASS-074    | 02, 03, 04, 05, 06, 07, 08, 10 |
| REQ-ASS-080    | 10              |
| REQ-ASS-081    | 07, 10          |
| REQ-ASS-082    | 08, 10          |
| REQ-ASS-083    | 10              |
| REQ-ASS-090    | 03, 04, 05, 06, 07, 08, 10 |
| REQ-ASS-091    | 03, 04, 05, 06, 07, 08, 10 |
| REQ-ASS-092    | 03, 04, 05, 06, 07, 08, 10 |
| REQ-ASS-093    | 03, 04, 05, 06, 07, 08, 10 |
