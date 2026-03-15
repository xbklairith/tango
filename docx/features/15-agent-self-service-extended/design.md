# Feature 15: Agent Self-Service API (Extended) -- Technical Design

## 1. Architecture Overview

All new endpoints extend the existing `AgentSelfHandler` in
`internal/server/handlers/agent_self_handler.go`. Authentication is handled by
the existing Run Token middleware which injects `auth.AgentIdentity` into the
request context. No new middleware is required.

```
Run Token (JWT)
  |
  v
RequireRunToken middleware  (existing)
  |
  v
AgentSelfHandler            (extended, now takes BudgetEnforcementService)
  |
  v
db.Queries / sqlc           (new queries added)
  |
  v
PostgreSQL                   (no schema changes, new indexes added)
```

### Route Registration

Add to `AgentSelfHandler.RegisterRoutes`:

```go
mux.HandleFunc("GET /api/agent/me/assignments", h.GetAssignments)
mux.HandleFunc("GET /api/agent/me/team", h.GetTeam)
mux.HandleFunc("GET /api/agent/me/budget", h.GetBudget)
mux.HandleFunc("GET /api/agent/me/goals", h.GetGoals)
mux.HandleFunc("POST /api/agent/me/inbox", h.CreateInboxItem)
mux.HandleFunc("POST /api/agent/me/cost", h.ReportCost)
```

### Terminated Agent Guard

Every handler method must check the agent's current DB status at the start.
If `agent.Status == "terminated"`, return 403 with `"Agent terminated"`. This
is implemented as a shared helper:

```go
func (h *AgentSelfHandler) requireActiveAgent(ctx context.Context, agentID uuid.UUID) (db.Agent, error)
```

Returns the agent record if active, or writes an error response and returns
an error if terminated.

---

## 2. Handler Dependency Changes

The `AgentSelfHandler` struct gains a `BudgetEnforcementService` dependency:

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
) *AgentSelfHandler {
    return &AgentSelfHandler{
        queries:       q,
        dbConn:        dbConn,
        sseHub:        sseHub,
        budgetService: budgetService,
    }
}
```

The server initialization code (in `server.go` or `cmd/ari/run.go`) must be
updated to pass the existing `BudgetEnforcementService` instance to the
constructor.

---

## 3. API Endpoints

### 3.1 GET /api/agent/me/assignments

Returns all issues assigned to the authenticated agent (or subtree for
lead/captain), with optional filtering and pagination.

**Query Parameters:**

| Param    | Type   | Default | Description                             |
|----------|--------|---------|-----------------------------------------|
| status   | string | (none)  | Filter by issue status                  |
| type     | string | (none)  | Filter by issue type                    |
| limit    | int    | 50      | Page size (max 100)                     |
| offset   | int    | 0       | Page offset                             |

**Role-based scoping:**
- `member`: Only own assignments (`assignee_agent_id = agent_id`)
- `lead`: Own assignments plus assignments of direct children
- `captain`: All assigned issues in the squad (issues where `assignee_agent_id IS NOT NULL`). The captain can use the existing `GET /api/squads/{id}/issues` for the full unfiltered view including unassigned issues.

**Response (200):**

```json
{
  "assignments": [
    {
      "id": "uuid",
      "identifier": "ARI-42",
      "type": "task",
      "title": "Implement feature X",
      "status": "in_progress",
      "priority": "high",
      "description": "...",
      "projectId": "uuid|null",
      "goalId": "uuid|null",
      "assigneeAgentId": "uuid|null",
      "pipelineId": "uuid|null",
      "currentStageId": "uuid|null",
      "createdAt": "2026-03-15T...",
      "updatedAt": "2026-03-15T..."
    }
  ],
  "total": 42
}
```

> **Note:** `pipelineId` and `currentStageId` are added by the Feature 14
> migration. If Feature 15 is implemented before Feature 14, these fields will
> be null.

**SQL query (new):** `ListAssignmentsByAgent`

```sql
-- name: ListAssignmentsByAgent :many
SELECT * FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = @agent_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;
```

```sql
-- name: CountAssignmentsByAgent :one
SELECT count(*) FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = @agent_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'));
```

For `lead` role, the handler will first fetch child agent IDs via
`ListAgentChildrenBySquad` (squad-scoped variant -- see C-1 fix below), then
use the multi-agent query:

```sql
-- name: ListAssignmentsByAgentIDs :many
SELECT * FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = ANY(@agent_ids::UUID[])
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAssignmentsByAgentIDs :one
SELECT count(*) FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = ANY(@agent_ids::UUID[])
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'));
```

For `captain` role, use the existing `ListIssuesBySquad` / `CountIssuesBySquad`
queries with no agent filter.

---

### 3.2 GET /api/agent/me/team

Returns the agent's team context based on hierarchy.

**Member/Lead Response (200):**

```json
{
  "self": {
    "id": "uuid",
    "name": "Builder",
    "shortName": "builder",
    "role": "lead",
    "status": "running",
    "parentAgentId": "uuid|null"
  },
  "parent": {
    "id": "uuid",
    "name": "Captain",
    "shortName": "captain",
    "role": "captain",
    "status": "idle",
    "parentAgentId": null
  },
  "siblings": [ ... ],
  "children": [ ... ]
}
```

**Captain response variant (200) -- simplified (M-1 fix):**

```json
{
  "self": { ... },
  "allAgents": [
    {
      "id": "uuid",
      "name": "Agent X",
      "shortName": "agentx",
      "role": "member",
      "status": "idle",
      "parentAgentId": "uuid"
    }
  ]
}
```

> Captain response returns only `self` + `allAgents`. The `parent`, `siblings`,
> and `children` fields are omitted to avoid redundancy since `allAgents`
> already contains the full hierarchy.

**Implementation:**

1. Fetch authenticated agent via `GetAgentByID` (handler re-fetches for full record).
2. **Terminated check**: if agent status is `terminated`, return 403.
3. If agent has `parent_agent_id`, fetch parent via `GetAgentByID` and siblings
   via `ListAgentChildrenBySquad(squad_id, parent_agent_id)`, filtering out self.
4. Fetch children via `ListAgentChildrenBySquad(squad_id, agent.ID)`.
5. **Captain override**: fetch all squad agents via `ListAgentsBySquad` and
   return `self` + `allAgents` only.

**SQL queries used:**
- `GetAgentByID` (existing)
- `ListAgentChildrenBySquad` (new -- see C-1 fix)
- `ListAgentsBySquad` (existing, captain only)

---

### 3.3 GET /api/agent/me/budget

Returns the agent's spend and budget for the current billing period.

**Response (200):**

```json
{
  "spentCents": 4200,
  "budgetCents": 10000,
  "remainingCents": 5800,
  "thresholdStatus": "ok",
  "periodStart": "2026-03-01T00:00:00Z",
  "periodEnd": "2026-04-01T00:00:00Z"
}
```

When the agent has no budget set:

```json
{
  "spentCents": 4200,
  "budgetCents": null,
  "remainingCents": null,
  "thresholdStatus": "ok",
  "periodStart": "2026-03-01T00:00:00Z",
  "periodEnd": "2026-04-01T00:00:00Z"
}
```

**Threshold logic -- reuses `domain.ComputeThreshold()` (M-2/M-3 fix):**

```go
// Reuse existing domain functions -- do NOT duplicate:
periodStart, periodEnd := domain.BillingPeriod(time.Now())
threshold, _ := domain.ComputeThreshold(budgetCentsPtr, spentCents)
// threshold is one of: domain.ThresholdOK, domain.ThresholdWarning, domain.ThresholdExceeded
```

No local `budgetThreshold()` or `currentBillingPeriod()` utility functions are
needed. Use `domain.BillingPeriod(time.Now())` and `domain.ComputeThreshold()`
directly.

**SQL queries used (existing):**
- `GetAgentByID` -- to read `budget_monthly_cents`
- `GetAgentMonthlySpend` -- to read current period spend

---

### 3.4 GET /api/agent/me/goals

Returns goals linked to the agent's assigned issues.

**Response (200):**

```json
{
  "goals": [
    {
      "id": "uuid",
      "title": "Launch MVP",
      "description": "...",
      "status": "active",
      "relatedIssues": ["ARI-1", "ARI-5"]
    }
  ]
}
```

**Implementation:**

1. Fetch all issues assigned to the agent (or subtree for lead, all for
   captain) that have a non-null `goal_id`.
2. Collect unique goal IDs.
3. Fetch all goals in one batch via `GetGoalsByIDs` (H-3 fix -- avoids N+1).
4. Build response, grouping issue identifiers under each goal.

**SQL query (new):** `GetGoalsByIDs` -- batch goal fetch

```sql
-- name: GetGoalsByIDs :many
SELECT * FROM goals
WHERE id = ANY(@ids::UUID[])
ORDER BY title;
```

**SQL query (new):** `ListGoalLinkedIssuesByAgent`

```sql
-- name: ListGoalLinkedIssuesByAgent :many
SELECT i.goal_id, i.identifier
FROM issues i
WHERE i.squad_id = @squad_id
  AND i.assignee_agent_id = @agent_id
  AND i.goal_id IS NOT NULL
ORDER BY i.goal_id, i.created_at DESC;
```

```sql
-- name: ListGoalLinkedIssuesByAgentIDs :many
SELECT i.goal_id, i.identifier
FROM issues i
WHERE i.squad_id = @squad_id
  AND i.assignee_agent_id = ANY(@agent_ids::UUID[])
  AND i.goal_id IS NOT NULL
ORDER BY i.goal_id, i.created_at DESC;
```

For captain: fetch all goals via existing `ListGoalsBySquad` and all issues
with goal_id set (use `ListGoalLinkedIssuesBySquad`):

```sql
-- name: ListGoalLinkedIssuesBySquad :many
SELECT i.goal_id, i.identifier
FROM issues i
WHERE i.squad_id = @squad_id
  AND i.goal_id IS NOT NULL
ORDER BY i.goal_id, i.created_at DESC;
```

**Existing queries reused:**
- `ListGoalsBySquad` (captain path)

---

### 3.5 POST /api/agent/me/inbox

Creates an inbox item on behalf of the agent to request human assistance.

**Request body:**

```json
{
  "category": "question",
  "title": "Need clarification on deployment target",
  "body": "Should I deploy to staging or production?",
  "urgency": "normal",
  "relatedIssueId": "uuid",
  "payload": { "key": "value" }
}
```

**Validation:**

| Field          | Required | Constraints                                        |
|----------------|----------|----------------------------------------------------|
| category       | yes      | One of: `approval`, `question`, `decision`         |
| title          | yes      | 1-500 characters                                   |
| body           | no       | Free text                                          |
| urgency        | no       | `critical`, `normal`, `low` (default: `normal`)    |
| relatedIssueId | no       | Valid UUID; must belong to agent's squad            |
| payload        | no       | JSON object                                        |

**Rejected categories:** `alert` returns 400 with code `VALIDATION_ERROR` and
message "agents cannot create alert inbox items".

**Field mapping:**

| Inbox column            | Source                                 |
|-------------------------|----------------------------------------|
| squad_id                | identity.SquadID                       |
| category                | req.Category                           |
| type                    | `"agent_request"`                      |
| urgency                 | req.Urgency (default "normal")         |
| title                   | req.Title                              |
| body                    | req.Body                               |
| payload                 | req.Payload (default `{}`)             |
| requested_by_agent_id   | identity.AgentID                       |
| related_agent_id        | identity.AgentID                       |
| related_issue_id        | req.RelatedIssueId (if provided)       |
| related_run_id          | identity.RunID                         |

> **Note (M-5):** The inbox `type` field is free-text (not a DB enum), so
> `"agent_request"` is a valid value without any migration.

**Response (201):** The created inbox item.

**Transaction flow:**

1. Validate request body.
2. If `relatedIssueId` provided, verify issue exists and belongs to squad.
3. Begin transaction.
4. Insert inbox item via `CreateInboxItem`.
5. Log activity: action `inbox.item.created`, entity_type `inbox_item`.
6. Commit.
7. Emit SSE event `inbox.item.created` (reusing the existing event name from
   InboxService.Create() -- XC-2 fix: do NOT emit a custom
   `inbox.agent_created` event).

**Existing SQL used:** `CreateInboxItem`, `GetIssueByID` (for validation)

---

### 3.6 POST /api/agent/me/reply (Already Implemented)

No changes. Documented here for completeness. See existing implementation in
`agent_self_handler.go`.

---

### 3.7 POST /api/agent/me/cost

Allows an agent to self-report a cost event from external tool usage.

**Request body:**

```json
{
  "amountCents": 150,
  "eventType": "external_api_call",
  "model": "gpt-4o",
  "inputTokens": 1200,
  "outputTokens": 800,
  "metadata": { "tool": "web_search", "provider": "serp" }
}
```

**Validation:**

| Field        | Required | Constraints                                     |
|--------------|----------|-------------------------------------------------|
| amountCents  | yes      | Positive integer (> 0), max 100000 ($1000/event)|
| eventType    | yes      | 1-50 characters                                 |
| model        | yes      | 1-100 characters                                |
| inputTokens  | no       | Non-negative integer (default 0)                |
| outputTokens | no       | Non-negative integer (default 0)                |
| metadata     | no       | JSON object (default `{}`)                      |

> **C-2 fix:** Per-event maximum of 100000 cents ($1000) prevents runaway
> cost reporting from a single event.

**Field mapping:**

| cost_events column | Source               |
|--------------------|----------------------|
| squad_id           | identity.SquadID     |
| agent_id           | identity.AgentID     |
| amount_cents       | req.AmountCents      |
| event_type         | req.EventType        |
| model              | req.Model            |
| input_tokens       | req.InputTokens      |
| output_tokens      | req.OutputTokens     |
| metadata           | req.Metadata         |

**Response (201):** The created cost event record.

**Implementation -- delegates to BudgetEnforcementService (C-3/C-4 fix):**

The handler does NOT implement its own transaction flow for cost recording
and threshold checking. Instead, it delegates to the existing
`BudgetEnforcementService.RecordAndEnforce()` which:

1. Inserts the cost event in a transaction.
2. Checks agent budget thresholds (80% warning, 100% exceeded + auto-pause).
3. Checks squad budget thresholds (80% warning, 100% exceeded + squad-wide pause).
4. Creates inbox alerts via InboxService for threshold crossings.
5. Commits the transaction.

```go
func (h *AgentSelfHandler) ReportCost(w http.ResponseWriter, r *http.Request) {
    // 1. Validate request body (including amountCents <= 100000)
    // 2. Build db.InsertCostEventParams from identity + request
    // 3. Call h.budgetService.RecordAndEnforce(ctx, params)
    // 4. Log activity: action "cost.event.created", entity_type "cost_event"
    // 5. Emit SSE event "cost.event.created"
    // 6. Return 201 with created cost event from result.CostEvent
}
```

This automatically handles both agent-level and squad-level budget enforcement
(fixing C-4) since `BudgetEnforcementService.RecordAndEnforce()` already
checks both.

**Existing services used:**
- `BudgetEnforcementService.RecordAndEnforce()` (handles insert + threshold + alerts)

---

## 4. Role-Based Filtering Matrix

| Endpoint                    | captain                          | lead                       | member         |
|-----------------------------|----------------------------------|----------------------------|----------------|
| GET /me/assignments         | All assigned issues in squad     | Own + children assignments | Own only       |
| GET /me/team                | self + allAgents only            | Parent + siblings + children| Parent + siblings |
| GET /me/budget              | Own budget only                  | Own budget only            | Own budget only|
| GET /me/goals               | All squad goals                  | Own + children goals       | Own only       |
| POST /me/inbox              | Own squad                        | Own squad                  | Own squad      |
| POST /me/cost               | Own agent                        | Own agent                  | Own agent      |

**Implementation pattern:**

```go
func (h *AgentSelfHandler) resolveAgentIDs(ctx context.Context, identity auth.AgentIdentity) ([]uuid.UUID, error) {
    switch identity.Role {
    case "captain":
        return nil, nil // nil signals "all in squad"
    case "lead":
        children, err := h.queries.ListAgentChildrenBySquad(ctx, db.ListAgentChildrenBySquadParams{
            SquadID:       identity.SquadID,
            ParentAgentID: uuid.NullUUID{UUID: identity.AgentID, Valid: true},
        })
        if err != nil {
            return nil, err
        }
        ids := []uuid.UUID{identity.AgentID}
        for _, c := range children {
            ids = append(ids, c.ID)
        }
        return ids, nil
    default: // member
        return []uuid.UUID{identity.AgentID}, nil
    }
}
```

A `nil` return means "no agent filter -- use squad-level queries." A non-nil
slice means "use agent ID array queries."

---

## 5. Domain Changes

No new domain types required. The handler will define local response structs
following the existing pattern in `agent_self_handler.go`.

Reuse existing domain functions directly:
- `domain.BillingPeriod(time.Now())` for billing period calculation
- `domain.ComputeThreshold(budgetCents, spentCents)` for threshold status

New response structs:

```go
type assignmentItem struct {
    ID              uuid.UUID  `json:"id"`
    Identifier      string     `json:"identifier"`
    Type            string     `json:"type"`
    Title           string     `json:"title"`
    Status          string     `json:"status"`
    Priority        string     `json:"priority"`
    Description     *string    `json:"description"`
    ProjectID       *uuid.UUID `json:"projectId"`
    GoalID          *uuid.UUID `json:"goalId"`
    AssigneeAgentID *uuid.UUID `json:"assigneeAgentId"`
    PipelineID      *uuid.UUID `json:"pipelineId"`
    CurrentStageID  *uuid.UUID `json:"currentStageId"`
    CreatedAt       string     `json:"createdAt"`
    UpdatedAt       string     `json:"updatedAt"`
}

type assignmentsResponse struct {
    Assignments []assignmentItem `json:"assignments"`
    Total       int64            `json:"total"`
}

type teamResponse struct {
    Self      teamAgent    `json:"self"`
    Parent    *teamAgent   `json:"parent,omitempty"`    // omitted for captain
    Siblings  []teamAgent  `json:"siblings,omitempty"`  // omitted for captain
    Children  []teamAgent  `json:"children,omitempty"`  // omitted for captain
    AllAgents []teamAgent  `json:"allAgents,omitempty"` // captain only
}

type budgetResponse struct {
    SpentCents      int64   `json:"spentCents"`
    BudgetCents     *int64  `json:"budgetCents"`
    RemainingCents  *int64  `json:"remainingCents"`
    ThresholdStatus string  `json:"thresholdStatus"`
    PeriodStart     string  `json:"periodStart"`
    PeriodEnd       string  `json:"periodEnd"`
}

type goalsResponse struct {
    Goals []goalItem `json:"goals"`
}

type goalItem struct {
    ID            uuid.UUID `json:"id"`
    Title         string    `json:"title"`
    Description   *string   `json:"description"`
    Status        string    `json:"status"`
    RelatedIssues []string  `json:"relatedIssues"`
}
```

---

## 6. New SQL Queries

All new queries go in `internal/database/queries/issues.sql` (assignment
queries), `internal/database/queries/goals.sql` (goal-linked queries), and
`internal/database/queries/agents.sql` (squad-scoped children query).

### agents.sql additions

**C-1 fix:** The existing `ListAgentChildren` query has no `squad_id` filter,
which could leak cross-squad data if a `parent_agent_id` UUID collides. Add a
new squad-scoped variant:

```sql
-- name: ListAgentChildrenBySquad :many
SELECT * FROM agents
WHERE squad_id = @squad_id
  AND parent_agent_id = @parent_agent_id
ORDER BY created_at ASC;
```

Use `ListAgentChildrenBySquad` in all Feature 15 code instead of the original
`ListAgentChildren`.

### issues.sql additions

```sql
-- name: ListAssignmentsByAgent :many
SELECT * FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = @agent_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAssignmentsByAgent :one
SELECT count(*) FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = @agent_id
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'));

-- name: ListAssignmentsByAgentIDs :many
SELECT * FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = ANY(@agent_ids::UUID[])
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAssignmentsByAgentIDs :one
SELECT count(*) FROM issues
WHERE squad_id = @squad_id
  AND assignee_agent_id = ANY(@agent_ids::UUID[])
  AND (sqlc.narg('filter_status')::issue_status IS NULL OR status = sqlc.narg('filter_status'))
  AND (sqlc.narg('filter_type')::issue_type IS NULL     OR type = sqlc.narg('filter_type'));
```

### goals.sql additions

```sql
-- name: GetGoalsByIDs :many
SELECT * FROM goals
WHERE id = ANY(@ids::UUID[])
ORDER BY title;

-- name: ListGoalLinkedIssuesByAgent :many
SELECT i.goal_id, i.identifier
FROM issues i
WHERE i.squad_id = @squad_id
  AND i.assignee_agent_id = @agent_id
  AND i.goal_id IS NOT NULL
ORDER BY i.goal_id, i.created_at DESC;

-- name: ListGoalLinkedIssuesByAgentIDs :many
SELECT i.goal_id, i.identifier
FROM issues i
WHERE i.squad_id = @squad_id
  AND i.assignee_agent_id = ANY(@agent_ids::UUID[])
  AND i.goal_id IS NOT NULL
ORDER BY i.goal_id, i.created_at DESC;

-- name: ListGoalLinkedIssuesBySquad :many
SELECT i.goal_id, i.identifier
FROM issues i
WHERE i.squad_id = @squad_id
  AND i.goal_id IS NOT NULL
ORDER BY i.goal_id, i.created_at DESC;
```

### New Indexes

Add the following indexes in a new goose migration:

```sql
-- H-5: Composite index for ANY(@agent_ids) assignment queries
CREATE INDEX idx_issues_squad_assignee ON issues(squad_id, assignee_agent_id, created_at DESC);

-- DB: Covering index for GetAgentMonthlySpend SUM query
CREATE INDEX idx_cost_events_agent_period ON cost_events(agent_id, created_at);
```

No other schema changes required -- all tables and columns already exist.

---

## 7. Error Handling

All endpoints follow the existing error convention:

```json
{"error": "Human-readable message", "code": "ERROR_CODE"}
```

| Scenario                              | Status | Code                      |
|---------------------------------------|--------|---------------------------|
| Missing / invalid Run Token           | 401    | UNAUTHENTICATED           |
| Agent terminated                      | 403    | FORBIDDEN                 |
| Agent not found in database           | 404    | NOT_FOUND                 |
| Issue not found (inbox validation)    | 404    | NOT_FOUND                 |
| Issue in wrong squad (inbox)          | 403    | FORBIDDEN                 |
| Invalid request body                  | 400    | VALIDATION_ERROR          |
| Agent creates alert inbox item        | 400    | VALIDATION_ERROR          |
| amountCents <= 0                      | 400    | VALIDATION_ERROR          |
| amountCents > 100000                  | 400    | VALIDATION_ERROR          |
| limit exceeds 100                     | 400    | VALIDATION_ERROR          |
| Internal error                        | 500    | INTERNAL_ERROR            |

---

## 8. Handler Updates to AgentSelfHandler

The existing `AgentSelfHandler` struct gains a `budgetService` field (see
section 2). All new handlers follow the same pattern as existing ones:

1. Extract `AgentIdentity` from context.
2. Fetch agent and check terminated status (`requireActiveAgent`).
3. Validate request (query params or body).
4. Determine role-based scope (call `resolveAgentIDs`).
5. Execute queries.
6. Build response.
7. For mutations: delegate to service (BudgetEnforcementService for cost),
   log activity, emit SSE.

Add the `resolveAgentIDs` helper method to the handler (uses
`ListAgentChildrenBySquad`).

Use `domain.BillingPeriod(time.Now())` directly for billing period calculation
(no local utility function needed).

Use `domain.ComputeThreshold()` directly for threshold status (no local
utility function needed).

---

## 9. Testing Strategy

### Unit Tests

File: `internal/server/handlers/agent_self_handler_test.go` (extend existing)

- Test `resolveAgentIDs()` for each role.
- Test terminated agent guard returns 403.

### Integration Tests

File: `internal/server/handlers/agent_self_integration_test.go` (new)

Each endpoint needs tests for:

1. **Happy path**: Authenticated agent gets expected data.
2. **Auth failure**: Request without Run Token returns 401.
3. **Terminated agent**: Returns 403.
4. **Squad scoping**: Agent cannot see data from another squad.
5. **Role filtering**:
   - Captain sees all squad data.
   - Lead sees own + children data.
   - Member sees only own data.
6. **Pagination**: Assignments endpoint respects limit/offset and returns
   correct total.
7. **Validation**: Invalid payloads for POST endpoints return 400.

Specific test cases:

| Test                                     | Endpoint               |
|------------------------------------------|------------------------|
| Assignments returns all statuses         | GET /me/assignments    |
| Assignments filters by status            | GET /me/assignments    |
| Assignments filters by type              | GET /me/assignments    |
| Assignments paginates correctly          | GET /me/assignments    |
| Assignments includes pipeline fields     | GET /me/assignments    |
| Lead sees children assignments           | GET /me/assignments    |
| Captain sees all assigned squad issues   | GET /me/assignments    |
| Team returns parent/siblings/children    | GET /me/team           |
| Captain team returns self + allAgents only| GET /me/team          |
| Member team has no children              | GET /me/team           |
| Budget with set limit                    | GET /me/budget         |
| Budget with null limit                   | GET /me/budget         |
| Budget threshold warning at 80%          | GET /me/budget         |
| Budget threshold exceeded at 100%        | GET /me/budget         |
| Goals linked via issues                  | GET /me/goals          |
| Goals deduplicated                       | GET /me/goals          |
| Goals fetched via batch query            | GET /me/goals          |
| Captain sees all goals                   | GET /me/goals          |
| Create inbox question                    | POST /me/inbox         |
| Create inbox approval                    | POST /me/inbox         |
| Reject inbox alert category              | POST /me/inbox         |
| Inbox validates relatedIssueId squad     | POST /me/inbox         |
| SSE event is inbox.item.created          | POST /me/inbox         |
| Cost self-report success                 | POST /me/cost          |
| Cost per-event max 100000               | POST /me/cost          |
| Cost delegates to BudgetEnforcementService| POST /me/cost         |
| Cost triggers 80% alert                  | POST /me/cost          |
| Cost triggers 100% alert + auto-pause    | POST /me/cost          |
| Cost SSE event is cost.event.created     | POST /me/cost          |
| Cost rejects negative amount             | POST /me/cost          |
| Terminated agent returns 403             | ALL                    |
| All endpoints reject missing token       | ALL                    |

### Test Fixtures

Use the existing test helpers for:
- Creating squads, agents (captain/lead/member hierarchy), issues, goals.
- Minting Run Tokens via `RunTokenService.Mint()`.
- Making authenticated HTTP requests against a test server.

---

## 10. Implementation Order

| Task | Description                           | Estimated Effort |
|------|---------------------------------------|------------------|
| 15.1 | SQL queries + indexes + ListAgentChildrenBySquad | S     |
| 15.2 | GET /me/assignments + SQL queries    | S                |
| 15.3 | GET /me/team                         | S                |
| 15.4 | GET /me/budget (uses domain.*)       | S                |
| 15.5 | GET /me/goals + GetGoalsByIDs batch  | M                |
| 15.6 | POST /me/inbox                       | M                |
| 15.7 | POST /me/cost + BudgetEnforcementService | M           |
| 15.8 | Role-based filtering (resolveAgentIDs + wiring) | M   |
| 15.9 | Route registration + constructor change | S             |
| 15.10| Integration tests                    | L                |

Tasks 15.2-15.6 can be implemented in any order. Task 15.8 should be done
early (or in parallel) since it provides the `resolveAgentIDs` helper used by
15.2 and 15.5. Task 15.10 should be done last or incrementally alongside each
endpoint.
