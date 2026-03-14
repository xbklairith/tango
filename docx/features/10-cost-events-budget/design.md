# Design: Cost Events & Budget Enforcement

**Created:** 2026-03-15
**Status:** Ready for Implementation

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Data Model](#data-model)
3. [sqlc Queries](#sqlc-queries)
4. [Budget Enforcement Service](#budget-enforcement-service)
5. [API Endpoints](#api-endpoints)
6. [Handler Implementation](#handler-implementation)
7. [Integration Points](#integration-points)
8. [Data Flow Diagrams](#data-flow-diagrams)
9. [Error Handling](#error-handling)
10. [Testing Strategy](#testing-strategy)

---

## Architecture Overview

### Cost Event Flow

```
Agent Runtime
    │
    │  POST /api/squads/:id/cost-events
    │  Authorization: Bearer <run-token>
    ▼
CostHandler.RecordEvent
    │
    ▼
BudgetEnforcementService.RecordAndEnforce(ctx, tx, params)
    │
    ├─ 1. INSERT cost_events row
    │
    ├─ 2. SELECT SUM(cost_cents) for agent this UTC month
    │
    ├─ 3a. agent.budget_monthly_cents IS NULL → skip agent enforcement
    │
    ├─ 3b. spend >= 100% → UPDATE agents SET status='paused' WHERE status IN ('running','idle')
    │       + (future) INSERT activity_log (action='agent.budget_paused')
    │       + (future) INSERT inbox_items (type='budget_warning')
    │
    ├─ 3c. spend >= 80% < 100% → (future) INSERT inbox_items (type='budget_warning') if not exists
    │
    ├─ 4. SELECT SUM(cost_cents) for squad this UTC month (all agents)
    │
    ├─ 5a. squad.budget_monthly_cents IS NULL → skip squad enforcement
    │
    ├─ 5b. squad spend >= 100% → UPDATE all running/idle agents SET status='paused'
    │       + (future) activity log per agent + inbox alert
    │
    ├─ 5c. squad spend >= 80% < 100% → (future) squad-level inbox alert if not exists
    │
    └─ COMMIT / ROLLBACK on any failure → caller gets 500, may retry
```

### Budget Re-evaluation Flow (PATCH agent/squad budget)

When `PATCH /api/agents/:id` or `PATCH /api/squads/:id/budgets` changes a budget value,
the same `BudgetEnforcementService.ReEvaluate(ctx, tx, agentID/squadID)` is called within the
same DB transaction that updates the budget column. This ensures REQ-007 and REQ-008 atomicity.

### Integration with Existing Budget Columns

Both `squads.budget_monthly_cents` and `agents.budget_monthly_cents` already exist in the DB
(migrations `20260314000003_create_squads.sql` and `20260314000005_create_agents.sql`).
This feature adds only the `cost_events` table and the enforcement logic that reads those columns.

---

## Data Model

### Migration

**File:** `internal/database/migrations/20260314000011_create_cost_events.sql`

```sql
-- +goose Up

CREATE TABLE cost_events (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id         UUID         NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id         UUID         NOT NULL REFERENCES agents(id),
    heartbeat_run_id UUID,        -- nullable; references heartbeat_runs(id) once that table exists
    issue_id         UUID         REFERENCES issues(id),
    project_id       UUID         REFERENCES projects(id),
    goal_id          UUID         REFERENCES goals(id),
    event_type       VARCHAR(50)  NOT NULL
                     CHECK (event_type IN ('llm_call', 'tool_use', 'embedding', 'other')),
    model            VARCHAR(100),
    provider         VARCHAR(50),
    input_tokens     INT          CHECK (input_tokens IS NULL OR input_tokens >= 0),
    output_tokens    INT          CHECK (output_tokens IS NULL OR output_tokens >= 0),
    cost_cents       BIGINT       NOT NULL CHECK (cost_cents >= 0),
    billing_code     VARCHAR(100),
    usage_json       JSONB,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Squad-scoped monthly aggregation (REQ-031)
CREATE INDEX idx_cost_events_squad_created
    ON cost_events (squad_id, created_at DESC);

-- Per-agent monthly aggregation (REQ-031)
CREATE INDEX idx_cost_events_agent_created
    ON cost_events (agent_id, created_at DESC);

-- Agent-within-squad composite (REQ-031)
CREATE INDEX idx_cost_events_squad_agent_created
    ON cost_events (squad_id, agent_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS cost_events;
```

**Design notes:**
- `heartbeat_run_id` is a bare UUID (no FK constraint) until feature 11 creates the
  `heartbeat_runs` table. This avoids a hard dependency on feature 11's migration order.
- No `updated_at` column and no `UPDATE`/`DELETE` routes — the table is append-only (REQ-018).
- Idempotency on client-supplied `id` is enforced via the PRIMARY KEY (REQ-037): a duplicate
  INSERT returns a `23505` unique-violation error which the handler maps to `200 OK`.

### Domain Model

**File:** `internal/domain/cost_event.go`

```go
package domain

import (
    "encoding/json"
    "fmt"
    "time"

    "github.com/google/uuid"
)

// CostEventType enumerates the kinds of cost-generating operations.
type CostEventType string

const (
    CostEventTypeLLMCall   CostEventType = "llm_call"
    CostEventTypeToolUse   CostEventType = "tool_use"
    CostEventTypeEmbedding CostEventType = "embedding"
    CostEventTypeOther     CostEventType = "other"
)

var validCostEventTypes = map[CostEventType]bool{
    CostEventTypeLLMCall:   true,
    CostEventTypeToolUse:   true,
    CostEventTypeEmbedding: true,
    CostEventTypeOther:     true,
}

// ThresholdStatus represents the current budget utilisation tier.
type ThresholdStatus string

const (
    ThresholdStatusOK       ThresholdStatus = "ok"
    ThresholdStatusWarning  ThresholdStatus = "warning"
    ThresholdStatusExceeded ThresholdStatus = "exceeded"
)

// CostEvent is the domain model for a single cost-generating event.
type CostEvent struct {
    ID             uuid.UUID       `json:"id"`
    SquadID        uuid.UUID       `json:"squadId"`
    AgentID        uuid.UUID       `json:"agentId"`
    HeartbeatRunID *uuid.UUID      `json:"heartbeatRunId,omitempty"`
    IssueID        *uuid.UUID      `json:"issueId,omitempty"`
    ProjectID      *uuid.UUID      `json:"projectId,omitempty"`
    GoalID         *uuid.UUID      `json:"goalId,omitempty"`
    EventType      CostEventType   `json:"eventType"`
    Model          *string         `json:"model,omitempty"`
    Provider       *string         `json:"provider,omitempty"`
    InputTokens    *int32          `json:"inputTokens,omitempty"`
    OutputTokens   *int32          `json:"outputTokens,omitempty"`
    CostCents      int64           `json:"costCents"`
    BillingCode    *string         `json:"billingCode,omitempty"`
    UsageJSON      json.RawMessage `json:"usageJson,omitempty"`
    CreatedAt      time.Time       `json:"createdAt"`
}

// CreateCostEventRequest is the parsed request body for POST /api/squads/:id/cost-events.
type CreateCostEventRequest struct {
    // ID is optional; if provided by the caller it enables idempotent submission (REQ-037).
    ID             *uuid.UUID      `json:"id,omitempty"`
    AgentID        uuid.UUID       `json:"agentId"`
    HeartbeatRunID *uuid.UUID      `json:"heartbeatRunId,omitempty"`
    IssueID        *uuid.UUID      `json:"issueId,omitempty"`
    ProjectID      *uuid.UUID      `json:"projectId,omitempty"`
    GoalID         *uuid.UUID      `json:"goalId,omitempty"`
    EventType      CostEventType   `json:"eventType"`
    Model          *string         `json:"model,omitempty"`
    Provider       *string         `json:"provider,omitempty"`
    InputTokens    *int32          `json:"inputTokens,omitempty"`
    OutputTokens   *int32          `json:"outputTokens,omitempty"`
    CostCents      int64           `json:"costCents"`
    BillingCode    *string         `json:"billingCode,omitempty"`
    UsageJSON      json.RawMessage `json:"usageJson,omitempty"`
}

// ValidateCreateCostEventRequest validates the request body for recording a cost event.
func ValidateCreateCostEventRequest(req CreateCostEventRequest) error {
    if !validCostEventTypes[req.EventType] {
        return fmt.Errorf("eventType must be one of: llm_call, tool_use, embedding, other")
    }
    if req.CostCents < 0 {
        return fmt.Errorf("costCents must be >= 0")
    }
    if req.InputTokens != nil && *req.InputTokens < 0 {
        return fmt.Errorf("inputTokens must be >= 0")
    }
    if req.OutputTokens != nil && *req.OutputTokens < 0 {
        return fmt.Errorf("outputTokens must be >= 0")
    }
    if req.Model != nil && len(*req.Model) > 100 {
        return fmt.Errorf("model must not exceed 100 characters")
    }
    if req.Provider != nil && len(*req.Provider) > 50 {
        return fmt.Errorf("provider must not exceed 50 characters")
    }
    if req.BillingCode != nil && len(*req.BillingCode) > 100 {
        return fmt.Errorf("billingCode must not exceed 100 characters")
    }
    if req.UsageJSON != nil && !json.Valid(req.UsageJSON) {
        return fmt.Errorf("usageJson must be valid JSON")
    }
    return nil
}

// ComputeThresholdStatus returns the threshold tier for the given spend and budget.
// If budgetCents is nil (no limit set), always returns ThresholdStatusOK.
func ComputeThresholdStatus(spendCents int64, budgetCents *int64) ThresholdStatus {
    if budgetCents == nil || *budgetCents == 0 {
        return ThresholdStatusOK
    }
    pct := float64(spendCents) / float64(*budgetCents)
    switch {
    case pct >= 1.0:
        return ThresholdStatusExceeded
    case pct >= 0.8:
        return ThresholdStatusWarning
    default:
        return ThresholdStatusOK
    }
}

// BillingPeriod returns the start (inclusive) and end (exclusive) of the
// UTC calendar month containing t, per REQ-022.
func BillingPeriod(t time.Time) (start, end time.Time) {
    t = t.UTC()
    start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
    end = start.AddDate(0, 1, 0)
    return
}
```

---

## sqlc Queries

**File:** `internal/database/queries/cost_events.sql`

```sql
-- name: InsertCostEvent :one
-- REQ-001, REQ-011, REQ-037
-- A duplicate id (client-generated UUID) returns a unique-violation error (23505),
-- which the handler maps to 200 OK with the existing row.
INSERT INTO cost_events (
    id, squad_id, agent_id, heartbeat_run_id,
    issue_id, project_id, goal_id,
    event_type, model, provider,
    input_tokens, output_tokens,
    cost_cents, billing_code, usage_json
) VALUES (
    COALESCE(@id, gen_random_uuid()),
    @squad_id, @agent_id, @heartbeat_run_id,
    @issue_id, @project_id, @goal_id,
    @event_type, @model, @provider,
    @input_tokens, @output_tokens,
    @cost_cents, @billing_code, @usage_json
)
RETURNING *;

-- name: GetCostEventByID :one
-- Used for idempotency check after a duplicate-key error.
SELECT * FROM cost_events
WHERE id = @id;

-- name: GetAgentMonthlySpend :one
-- REQ-002, REQ-015 — sum for the current UTC calendar month.
SELECT COALESCE(SUM(cost_cents), 0)::BIGINT AS spend_cents
FROM cost_events
WHERE agent_id   = @agent_id
  AND created_at >= @period_start
  AND created_at <  @period_end;

-- name: GetSquadMonthlySpend :one
-- REQ-005, REQ-006, REQ-009, REQ-015 — squad-level total.
SELECT COALESCE(SUM(cost_cents), 0)::BIGINT AS spend_cents
FROM cost_events
WHERE squad_id   = @squad_id
  AND created_at >= @period_start
  AND created_at <  @period_end;

-- name: GetAgentCostBreakdown :many
-- REQ-010 — per-agent spend for the squad this month, joined with agent name and budget.
SELECT
    a.id                   AS agent_id,
    a.name                 AS agent_name,
    a.budget_monthly_cents,
    COALESCE(SUM(ce.cost_cents), 0)::BIGINT AS spend_cents
FROM agents a
LEFT JOIN cost_events ce
       ON ce.agent_id   = a.id
      AND ce.squad_id   = a.squad_id
      AND ce.created_at >= @period_start
      AND ce.created_at <  @period_end
WHERE a.squad_id = @squad_id
  AND a.status  != 'terminated'
GROUP BY a.id, a.name, a.budget_monthly_cents
ORDER BY spend_cents DESC;

-- name: GetCostEventsByAgent :many
-- REQ-010 (detail view) — paginated list for one agent.
SELECT * FROM cost_events
WHERE agent_id = @agent_id
  AND squad_id = @squad_id
ORDER BY created_at DESC
LIMIT  @lim
OFFSET @off;

-- name: ListRunningIdleAgentsBySquad :many
-- Used by squad-level hard-stop to find all agents that need pausing.
SELECT id, status FROM agents
WHERE squad_id = @squad_id
  AND status IN ('running', 'idle');
```

---

## Budget Enforcement Service

**File:** `internal/budget/service.go`

```go
package budget

import (
    "context"
    "database/sql"
    "fmt"
    "log/slog"
    "time"

    "github.com/google/uuid"
    "github.com/lib/pq"

    "github.com/xb/ari/internal/database/db"
    "github.com/xb/ari/internal/domain"
)

// Service handles cost event recording and budget threshold enforcement.
// All mutations happen inside a single DB transaction supplied by the caller.
type Service struct {
    db      *sql.DB
    queries *db.Queries
}

// New creates a Service.
func New(conn *sql.DB, q *db.Queries) *Service {
    return &Service{db: conn, queries: q}
}

// RecordParams contains all data needed to record a cost event.
type RecordParams struct {
    SquadID        uuid.UUID
    AgentID        uuid.UUID
    HeartbeatRunID *uuid.UUID
    IssueID        *uuid.UUID
    ProjectID      *uuid.UUID
    GoalID         *uuid.UUID
    EventType      domain.CostEventType
    Model          *string
    Provider       *string
    InputTokens    *int32
    OutputTokens   *int32
    CostCents      int64
    BillingCode    *string
    UsageJSON      []byte
    // ClientID is set when the caller provides an idempotency UUID.
    ClientID *uuid.UUID
}

// RecordResult is returned by RecordAndEnforce.
type RecordResult struct {
    Event     db.CostEvent
    Duplicate bool // true when the event already existed (idempotent replay)
}

// RecordAndEnforce inserts the cost event and enforces budget thresholds, all
// within a single database transaction. REQ-011, REQ-019, REQ-036.
//
// Callers MUST open a transaction, call this method, then commit.
// If this method returns a non-nil error, the transaction must be rolled back.
func (s *Service) RecordAndEnforce(ctx context.Context, qtx *db.Queries, params RecordParams) (RecordResult, error) {
    now := time.Now().UTC()
    periodStart, periodEnd := domain.BillingPeriod(now)

    // --- 1. Insert cost event ---
    insertParams := db.InsertCostEventParams{
        SquadID:   params.SquadID,
        AgentID:   params.AgentID,
        EventType: string(params.EventType),
        CostCents: params.CostCents,
    }
    if params.ClientID != nil {
        insertParams.ID = uuid.NullUUID{UUID: *params.ClientID, Valid: true}
    }
    if params.HeartbeatRunID != nil {
        insertParams.HeartbeatRunID = uuid.NullUUID{UUID: *params.HeartbeatRunID, Valid: true}
    }
    if params.IssueID != nil {
        insertParams.IssueID = uuid.NullUUID{UUID: *params.IssueID, Valid: true}
    }
    if params.ProjectID != nil {
        insertParams.ProjectID = uuid.NullUUID{UUID: *params.ProjectID, Valid: true}
    }
    if params.GoalID != nil {
        insertParams.GoalID = uuid.NullUUID{UUID: *params.GoalID, Valid: true}
    }
    if params.Model != nil {
        insertParams.Model = sql.NullString{String: *params.Model, Valid: true}
    }
    if params.Provider != nil {
        insertParams.Provider = sql.NullString{String: *params.Provider, Valid: true}
    }
    if params.InputTokens != nil {
        insertParams.InputTokens = sql.NullInt32{Int32: *params.InputTokens, Valid: true}
    }
    if params.OutputTokens != nil {
        insertParams.OutputTokens = sql.NullInt32{Int32: *params.OutputTokens, Valid: true}
    }
    if params.BillingCode != nil {
        insertParams.BillingCode = sql.NullString{String: *params.BillingCode, Valid: true}
    }
    if params.UsageJSON != nil {
        insertParams.UsageJson = params.UsageJSON
    }

    event, err := qtx.InsertCostEvent(ctx, insertParams)
    if err != nil {
        // Idempotent replay: duplicate client-supplied UUID (REQ-037)
        var pqErr *pq.Error
        if pq.ErrorCode("23505").Name() == "" || (ok := false; ok) {
            _ = ok
        }
        if isPgUniqueViolation(err) && params.ClientID != nil {
            existing, fetchErr := qtx.GetCostEventByID(ctx, *params.ClientID)
            if fetchErr != nil {
                return RecordResult{}, fmt.Errorf("budget: fetch existing event: %w", fetchErr)
            }
            return RecordResult{Event: existing, Duplicate: true}, nil
        }
        return RecordResult{}, fmt.Errorf("budget: insert cost event: %w", err)
    }

    // --- 2. Fetch agent to get budget limit and current status ---
    agent, err := qtx.GetAgentByID(ctx, params.AgentID)
    if err != nil {
        return RecordResult{}, fmt.Errorf("budget: get agent: %w", err)
    }

    // --- 3. Agent-level enforcement ---
    if agent.BudgetMonthlyCents.Valid {
        agentSpend, err := qtx.GetAgentMonthlySpend(ctx, db.GetAgentMonthlySpendParams{
            AgentID:     params.AgentID,
            PeriodStart: periodStart,
            PeriodEnd:   periodEnd,
        })
        if err != nil {
            return RecordResult{}, fmt.Errorf("budget: get agent monthly spend: %w", err)
        }

        budget := agent.BudgetMonthlyCents.Int64
        status := domain.ComputeThresholdStatus(agentSpend, &budget)

        switch status {
        case domain.ThresholdStatusExceeded:
            // REQ-004: auto-pause the agent if it is running or idle.
            currentStatus := domain.AgentStatus(agent.Status)
            if currentStatus == domain.AgentStatusRunning || currentStatus == domain.AgentStatusIdle {
                if err := domain.ValidateStatusTransition(currentStatus, domain.AgentStatusPaused); err == nil {
                    _, err = qtx.UpdateAgent(ctx, db.UpdateAgentParams{
                        ID:     params.AgentID,
                        Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
                    })
                    if err != nil {
                        return RecordResult{}, fmt.Errorf("budget: auto-pause agent: %w", err)
                    }
                    slog.Info("agent auto-paused by budget enforcement",
                        "agent_id", params.AgentID, "spend_cents", agentSpend, "budget_cents", budget)
                    // TODO(feature-09): insert activity_log row (action=agent.budget_paused)
                    // TODO(inbox): insert inbox_items row (type=budget_warning) if not exists
                }
            }
        case domain.ThresholdStatusWarning:
            // REQ-003: create soft alert if not already present.
            // TODO(inbox): insert inbox_items row (type=budget_warning) if not exists for this agent/month
            slog.Info("agent budget warning threshold reached",
                "agent_id", params.AgentID, "spend_cents", agentSpend, "budget_cents", budget)
        }
    }

    // --- 4. Squad-level enforcement ---
    squad, err := qtx.GetSquadByID(ctx, params.SquadID)
    if err != nil {
        return RecordResult{}, fmt.Errorf("budget: get squad: %w", err)
    }

    if squad.BudgetMonthlyCents.Valid {
        squadSpend, err := qtx.GetSquadMonthlySpend(ctx, db.GetSquadMonthlySpendParams{
            SquadID:     params.SquadID,
            PeriodStart: periodStart,
            PeriodEnd:   periodEnd,
        })
        if err != nil {
            return RecordResult{}, fmt.Errorf("budget: get squad monthly spend: %w", err)
        }

        squadBudget := squad.BudgetMonthlyCents.Int64
        squadStatus := domain.ComputeThresholdStatus(squadSpend, &squadBudget)

        switch squadStatus {
        case domain.ThresholdStatusExceeded:
            // REQ-006: auto-pause all running/idle agents in the squad.
            activeAgents, err := qtx.ListRunningIdleAgentsBySquad(ctx, params.SquadID)
            if err != nil {
                return RecordResult{}, fmt.Errorf("budget: list active agents: %w", err)
            }
            for _, a := range activeAgents {
                _, err = qtx.UpdateAgent(ctx, db.UpdateAgentParams{
                    ID:     a.ID,
                    Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
                })
                if err != nil {
                    return RecordResult{}, fmt.Errorf("budget: auto-pause squad agent %s: %w", a.ID, err)
                }
                slog.Info("agent auto-paused by squad budget enforcement",
                    "agent_id", a.ID, "squad_id", params.SquadID,
                    "spend_cents", squadSpend, "budget_cents", squadBudget)
                // TODO(feature-09): insert activity_log row per agent
            }
            // TODO(inbox): insert squad-level inbox alert if not exists
        case domain.ThresholdStatusWarning:
            // REQ-005: create squad-level soft alert.
            // TODO(inbox): insert squad-level inbox_items row if not exists for this month
            slog.Info("squad budget warning threshold reached",
                "squad_id", params.SquadID, "spend_cents", squadSpend, "budget_cents", squadBudget)
        }
    }

    return RecordResult{Event: event}, nil
}

// ReEvaluateAgent re-runs threshold checks after an agent budget change (REQ-007).
// Must be called inside the same transaction as the budget UPDATE.
func (s *Service) ReEvaluateAgent(ctx context.Context, qtx *db.Queries, agentID uuid.UUID) error {
    agent, err := qtx.GetAgentByID(ctx, agentID)
    if err != nil {
        return fmt.Errorf("budget: re-evaluate agent: get agent: %w", err)
    }
    if !agent.BudgetMonthlyCents.Valid {
        // Budget removed: resolve any outstanding warnings.
        // TODO(inbox): resolve budget_warning inbox items for this agent
        return nil
    }

    now := time.Now().UTC()
    periodStart, periodEnd := domain.BillingPeriod(now)

    spend, err := qtx.GetAgentMonthlySpend(ctx, db.GetAgentMonthlySpendParams{
        AgentID:     agentID,
        PeriodStart: periodStart,
        PeriodEnd:   periodEnd,
    })
    if err != nil {
        return fmt.Errorf("budget: re-evaluate agent: get spend: %w", err)
    }

    budget := agent.BudgetMonthlyCents.Int64
    status := domain.ComputeThresholdStatus(spend, &budget)

    switch status {
    case domain.ThresholdStatusExceeded:
        currentStatus := domain.AgentStatus(agent.Status)
        if currentStatus == domain.AgentStatusRunning || currentStatus == domain.AgentStatusIdle {
            _, err = qtx.UpdateAgent(ctx, db.UpdateAgentParams{
                ID:     agentID,
                Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
            })
            if err != nil {
                return fmt.Errorf("budget: re-evaluate agent: auto-pause: %w", err)
            }
        }
    case domain.ThresholdStatusOK:
        // Budget increased enough to drop below 80%: resolve any alerts.
        // TODO(inbox): resolve budget_warning inbox items for this agent
    }
    return nil
}

// ReEvaluateSquad re-runs threshold checks after a squad budget change (REQ-008).
func (s *Service) ReEvaluateSquad(ctx context.Context, qtx *db.Queries, squadID uuid.UUID) error {
    squad, err := qtx.GetSquadByID(ctx, squadID)
    if err != nil {
        return fmt.Errorf("budget: re-evaluate squad: get squad: %w", err)
    }
    if !squad.BudgetMonthlyCents.Valid {
        // TODO(inbox): resolve squad-level budget_warning inbox items
        return nil
    }

    now := time.Now().UTC()
    periodStart, periodEnd := domain.BillingPeriod(now)

    spend, err := qtx.GetSquadMonthlySpend(ctx, db.GetSquadMonthlySpendParams{
        SquadID:     squadID,
        PeriodStart: periodStart,
        PeriodEnd:   periodEnd,
    })
    if err != nil {
        return fmt.Errorf("budget: re-evaluate squad: get spend: %w", err)
    }

    squadBudget := squad.BudgetMonthlyCents.Int64
    status := domain.ComputeThresholdStatus(spend, &squadBudget)

    if status == domain.ThresholdStatusExceeded {
        activeAgents, err := qtx.ListRunningIdleAgentsBySquad(ctx, squadID)
        if err != nil {
            return fmt.Errorf("budget: re-evaluate squad: list agents: %w", err)
        }
        for _, a := range activeAgents {
            _, err = qtx.UpdateAgent(ctx, db.UpdateAgentParams{
                ID:     a.ID,
                Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
            })
            if err != nil {
                return fmt.Errorf("budget: re-evaluate squad: pause agent %s: %w", a.ID, err)
            }
        }
    }
    return nil
}

// isPgUniqueViolation returns true if err is a PostgreSQL unique-constraint violation.
func isPgUniqueViolation(err error) bool {
    var pqErr *pq.Error
    return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
```

> **Note on `errors` import:** the snippet above uses `errors.As`; add `"errors"` to the import block.

### Budget Re-evaluation Hook in Existing Handlers

**`PATCH /api/agents/:id`** — after the `UpdateAgent` query succeeds, if `req.SetBudget` is true,
call `budgetSvc.ReEvaluateAgent(ctx, qtx, agentID)` in the same transaction.

**`PATCH /api/squads/:id/budgets`** — after `UpdateSquad` succeeds, call
`budgetSvc.ReEvaluateSquad(ctx, qtx, squadID)` in the same transaction.

Both of these require converting the existing single-query calls into transactions that wrap
the existing `UpdateAgent`/`UpdateSquad` queries plus the re-evaluation call. The `dbConn *sql.DB`
field already present on both handlers is used for `BeginTx`.

---

## API Endpoints

### `POST /api/squads/{id}/cost-events`

Records a cost event for an agent in this squad and enforces budget thresholds.

**Auth:** Run Token JWT (Bearer token, `sub` = agent UUID, `squad` = squad UUID claim).
Pending the Run Token implementation in feature 11, this endpoint temporarily accepts
a standard session JWT from a squad member.

**Request:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",   // optional — idempotency key
  "agentId": "...",
  "eventType": "llm_call",
  "model": "claude-3-5-sonnet-20241022",
  "provider": "anthropic",
  "inputTokens": 1024,
  "outputTokens": 512,
  "costCents": 47,
  "issueId": "...",          // optional
  "projectId": "...",        // optional
  "goalId": "...",           // optional
  "billingCode": "sprint-42", // optional
  "usageJson": {}             // optional
}
```

**Responses:**
| Status | Condition |
|--------|-----------|
| `201 Created` | Event recorded; body is the full `CostEvent` object |
| `200 OK` | Duplicate `id` — returns existing event (REQ-037) |
| `400 Bad Request` | Validation failure |
| `403 Forbidden` | `agentId` does not belong to this squad (REQ-030) |
| `409 Conflict` | Agent status is `terminated` (REQ-029) |
| `500 Internal Server Error` | Enforcement failed; event was rolled back — safe to retry (REQ-036) |

---

### `GET /api/squads/{id}/costs/summary`

Returns squad-level spend summary for the current UTC calendar month.

**Auth:** Session JWT, squad membership required (any role).

**Response:**
```json
{
  "squadId": "...",
  "spendCents": 12500,
  "budgetMonthlyCents": 50000,
  "percentUtilised": 25.0,
  "thresholdStatus": "ok",
  "periodStart": "2026-03-01T00:00:00Z",
  "periodEnd": "2026-04-01T00:00:00Z"
}
```

`budgetMonthlyCents` is `null` when no budget is set. `percentUtilised` is `null` when
there is no budget. `thresholdStatus` is always present (`"ok"` when there is no budget).

---

### `GET /api/squads/{id}/costs/by-agent`

Returns per-agent cost breakdown for the squad this UTC calendar month.

**Auth:** Session JWT, squad membership required (any role).

**Response:**
```json
{
  "items": [
    {
      "agentId": "...",
      "agentName": "Aria",
      "spendCents": 8300,
      "budgetMonthlyCents": 20000,
      "percentUtilised": 41.5,
      "thresholdStatus": "ok"
    }
  ],
  "periodStart": "2026-03-01T00:00:00Z",
  "periodEnd": "2026-04-01T00:00:00Z"
}
```

---

### `GET /api/agent/me/budget`

Returns the calling agent's own budget status. Gated by Run Token JWT.

**Auth:** Run Token JWT (pending feature 11). Agent identity is taken from the token `sub` claim.

**Response:**
```json
{
  "agentId": "...",
  "spendCents": 3100,
  "budgetMonthlyCents": 20000,
  "percentUtilised": 15.5,
  "thresholdStatus": "ok",
  "periodStart": "2026-03-01T00:00:00Z",
  "periodEnd": "2026-04-01T00:00:00Z"
}
```

`budgetMonthlyCents` is `null` when no budget is set for the agent.

---

## Handler Implementation

**File:** `internal/server/handlers/cost_handler.go`

```go
package handlers

import (
    "database/sql"
    "encoding/json"
    "errors"
    "log/slog"
    "net/http"
    "time"

    "github.com/google/uuid"

    "github.com/xb/ari/internal/auth"
    "github.com/xb/ari/internal/budget"
    "github.com/xb/ari/internal/database/db"
    "github.com/xb/ari/internal/domain"
)

// CostHandler handles cost event recording and budget summary endpoints.
type CostHandler struct {
    queries   *db.Queries
    dbConn    *sql.DB
    budgetSvc *budget.Service
}

// NewCostHandler creates a CostHandler.
func NewCostHandler(q *db.Queries, dbConn *sql.DB, svc *budget.Service) *CostHandler {
    return &CostHandler{queries: q, dbConn: dbConn, budgetSvc: svc}
}

// RegisterRoutes registers cost-related routes.
func (h *CostHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /api/squads/{id}/cost-events",    h.RecordCostEvent)
    mux.HandleFunc("GET  /api/squads/{id}/costs/summary",  h.GetCostSummary)
    mux.HandleFunc("GET  /api/squads/{id}/costs/by-agent", h.GetCostByAgent)
    mux.HandleFunc("GET  /api/agent/me/budget",            h.GetAgentBudget)
}

// --- Request / Response Types ---

type costEventResponse struct {
    ID             string          `json:"id"`
    SquadID        string          `json:"squadId"`
    AgentID        string          `json:"agentId"`
    HeartbeatRunID *string         `json:"heartbeatRunId,omitempty"`
    IssueID        *string         `json:"issueId,omitempty"`
    ProjectID      *string         `json:"projectId,omitempty"`
    GoalID         *string         `json:"goalId,omitempty"`
    EventType      string          `json:"eventType"`
    Model          *string         `json:"model,omitempty"`
    Provider       *string         `json:"provider,omitempty"`
    InputTokens    *int32          `json:"inputTokens,omitempty"`
    OutputTokens   *int32          `json:"outputTokens,omitempty"`
    CostCents      int64           `json:"costCents"`
    BillingCode    *string         `json:"billingCode,omitempty"`
    UsageJSON      json.RawMessage `json:"usageJson,omitempty"`
    CreatedAt      string          `json:"createdAt"`
}

type costSummaryResponse struct {
    SquadID            string   `json:"squadId"`
    SpendCents         int64    `json:"spendCents"`
    BudgetMonthlyCents *int64   `json:"budgetMonthlyCents"`
    PercentUtilised    *float64 `json:"percentUtilised"`
    ThresholdStatus    string   `json:"thresholdStatus"`
    PeriodStart        string   `json:"periodStart"`
    PeriodEnd          string   `json:"periodEnd"`
}

type agentCostSummary struct {
    AgentID            string   `json:"agentId"`
    AgentName          string   `json:"agentName"`
    SpendCents         int64    `json:"spendCents"`
    BudgetMonthlyCents *int64   `json:"budgetMonthlyCents"`
    PercentUtilised    *float64 `json:"percentUtilised"`
    ThresholdStatus    string   `json:"thresholdStatus"`
}

type costByAgentResponse struct {
    Items       []agentCostSummary `json:"items"`
    PeriodStart string             `json:"periodStart"`
    PeriodEnd   string             `json:"periodEnd"`
}

type agentBudgetResponse struct {
    AgentID            string   `json:"agentId"`
    SpendCents         int64    `json:"spendCents"`
    BudgetMonthlyCents *int64   `json:"budgetMonthlyCents"`
    PercentUtilised    *float64 `json:"percentUtilised"`
    ThresholdStatus    string   `json:"thresholdStatus"`
    PeriodStart        string   `json:"periodStart"`
    PeriodEnd          string   `json:"periodEnd"`
}

// --- Handlers ---

// RecordCostEvent handles POST /api/squads/{id}/cost-events.
// REQ-011, REQ-019, REQ-029, REQ-030, REQ-033, REQ-036, REQ-037.
func (h *CostHandler) RecordCostEvent(w http.ResponseWriter, r *http.Request) {
    squadID, ok := parseSquadID(w, r)
    if !ok {
        return
    }

    // Auth: Run Token scoped to this squad (REQ-033).
    // TODO(feature-11): replace with RunTokenFromContext once Run Tokens are implemented.
    // Interim: accept session JWT + squad membership.
    identity, ok := auth.UserFromContext(r.Context())
    if !ok {
        writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
        return
    }
    _, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
        UserID: identity.UserID, SquadID: squadID,
    })
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
            return
        }
        slog.Error("cost: check membership", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    var req domain.CreateCostEventRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
        return
    }
    if err := domain.ValidateCreateCostEventRequest(req); err != nil {
        writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
        return
    }

    // Verify agentId belongs to this squad (REQ-030).
    agent, err := h.queries.GetAgentByID(r.Context(), req.AgentID)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeJSON(w, http.StatusForbidden, errorResponse{Error: "Agent not found in this squad", Code: "FORBIDDEN"})
            return
        }
        slog.Error("cost: get agent", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }
    if agent.SquadID != squadID {
        writeJSON(w, http.StatusForbidden, errorResponse{Error: "Agent does not belong to this squad", Code: "FORBIDDEN"})
        return
    }

    // Reject terminated agents (REQ-029).
    if domain.AgentStatus(agent.Status) == domain.AgentStatusTerminated {
        writeJSON(w, http.StatusConflict, errorResponse{Error: "Agent is terminated", Code: "AGENT_TERMINATED"})
        return
    }

    // Transaction: insert + enforce (REQ-019).
    tx, err := h.dbConn.BeginTx(r.Context(), nil)
    if err != nil {
        slog.Error("cost: begin tx", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }
    defer tx.Rollback()

    qtx := h.queries.WithTx(tx)
    result, err := h.budgetSvc.RecordAndEnforce(r.Context(), qtx, budget.RecordParams{
        SquadID:        squadID,
        AgentID:        req.AgentID,
        HeartbeatRunID: req.HeartbeatRunID,
        IssueID:        req.IssueID,
        ProjectID:      req.ProjectID,
        GoalID:         req.GoalID,
        EventType:      req.EventType,
        Model:          req.Model,
        Provider:       req.Provider,
        InputTokens:    req.InputTokens,
        OutputTokens:   req.OutputTokens,
        CostCents:      req.CostCents,
        BillingCode:    req.BillingCode,
        UsageJSON:      req.UsageJSON,
        ClientID:       req.ID,
    })
    if err != nil {
        // Rollback is deferred. Return 500 — caller may retry safely (REQ-036).
        slog.Error("cost: record and enforce", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    if err := tx.Commit(); err != nil {
        slog.Error("cost: commit tx", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    status := http.StatusCreated
    if result.Duplicate {
        status = http.StatusOK
    }
    writeJSON(w, status, dbCostEventToResponse(result.Event))
}

// GetCostSummary handles GET /api/squads/{id}/costs/summary.
// REQ-009, REQ-034.
func (h *CostHandler) GetCostSummary(w http.ResponseWriter, r *http.Request) {
    squadID, ok := parseSquadID(w, r)
    if !ok {
        return
    }
    identity, ok := auth.UserFromContext(r.Context())
    if !ok {
        writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
        return
    }
    _, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
        UserID: identity.UserID, SquadID: squadID,
    })
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
            return
        }
        slog.Error("cost: check membership", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    squad, err := h.queries.GetSquadByID(r.Context(), squadID)
    if err != nil {
        writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
        return
    }

    now := time.Now().UTC()
    periodStart, periodEnd := domain.BillingPeriod(now)

    spend, err := h.queries.GetSquadMonthlySpend(r.Context(), db.GetSquadMonthlySpendParams{
        SquadID: squadID, PeriodStart: periodStart, PeriodEnd: periodEnd,
    })
    if err != nil {
        slog.Error("cost: get squad spend", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    var budgetPtr *int64
    if squad.BudgetMonthlyCents.Valid {
        budgetPtr = &squad.BudgetMonthlyCents.Int64
    }
    thresholdStatus := domain.ComputeThresholdStatus(spend, budgetPtr)

    resp := costSummaryResponse{
        SquadID:            squad.ID.String(),
        SpendCents:         spend,
        BudgetMonthlyCents: budgetPtr,
        ThresholdStatus:    string(thresholdStatus),
        PeriodStart:        periodStart.Format(time.RFC3339),
        PeriodEnd:          periodEnd.Format(time.RFC3339),
    }
    if budgetPtr != nil && *budgetPtr > 0 {
        pct := float64(spend) / float64(*budgetPtr) * 100
        resp.PercentUtilised = &pct
    }
    writeJSON(w, http.StatusOK, resp)
}

// GetCostByAgent handles GET /api/squads/{id}/costs/by-agent.
// REQ-010, REQ-034.
func (h *CostHandler) GetCostByAgent(w http.ResponseWriter, r *http.Request) {
    squadID, ok := parseSquadID(w, r)
    if !ok {
        return
    }
    identity, ok := auth.UserFromContext(r.Context())
    if !ok {
        writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
        return
    }
    _, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
        UserID: identity.UserID, SquadID: squadID,
    })
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
            return
        }
        slog.Error("cost: check membership", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    now := time.Now().UTC()
    periodStart, periodEnd := domain.BillingPeriod(now)

    rows, err := h.queries.GetAgentCostBreakdown(r.Context(), db.GetAgentCostBreakdownParams{
        SquadID: squadID, PeriodStart: periodStart, PeriodEnd: periodEnd,
    })
    if err != nil {
        slog.Error("cost: get agent breakdown", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    items := make([]agentCostSummary, 0, len(rows))
    for _, row := range rows {
        item := agentCostSummary{
            AgentID:    row.AgentID.String(),
            AgentName:  row.AgentName,
            SpendCents: row.SpendCents,
        }
        if row.BudgetMonthlyCents.Valid {
            b := row.BudgetMonthlyCents.Int64
            item.BudgetMonthlyCents = &b
            status := domain.ComputeThresholdStatus(row.SpendCents, &b)
            item.ThresholdStatus = string(status)
            if b > 0 {
                pct := float64(row.SpendCents) / float64(b) * 100
                item.PercentUtilised = &pct
            }
        } else {
            item.ThresholdStatus = string(domain.ThresholdStatusOK)
        }
        items = append(items, item)
    }

    writeJSON(w, http.StatusOK, costByAgentResponse{
        Items:       items,
        PeriodStart: periodStart.Format(time.RFC3339),
        PeriodEnd:   periodEnd.Format(time.RFC3339),
    })
}

// GetAgentBudget handles GET /api/agent/me/budget.
// REQ-012, REQ-033.
func (h *CostHandler) GetAgentBudget(w http.ResponseWriter, r *http.Request) {
    // TODO(feature-11): replace with Run Token extraction once implemented.
    // The Run Token JWT carries sub=agentID and squad=squadID.
    // Interim: accept session JWT — return 501 until Run Tokens exist.
    writeJSON(w, http.StatusNotImplemented, errorResponse{
        Error: "Run Token auth not yet implemented; available in feature 11",
        Code:  "NOT_IMPLEMENTED",
    })
}

// --- Helper ---

func dbCostEventToResponse(e db.CostEvent) costEventResponse {
    resp := costEventResponse{
        ID:        e.ID.String(),
        SquadID:   e.SquadID.String(),
        AgentID:   e.AgentID.String(),
        EventType: e.EventType,
        CostCents: e.CostCents,
        CreatedAt: e.CreatedAt.Format(time.RFC3339),
    }
    if e.HeartbeatRunID.Valid {
        s := e.HeartbeatRunID.UUID.String()
        resp.HeartbeatRunID = &s
    }
    if e.IssueID.Valid {
        s := e.IssueID.UUID.String()
        resp.IssueID = &s
    }
    if e.ProjectID.Valid {
        s := e.ProjectID.UUID.String()
        resp.ProjectID = &s
    }
    if e.GoalID.Valid {
        s := e.GoalID.UUID.String()
        resp.GoalID = &s
    }
    if e.Model.Valid {
        resp.Model = &e.Model.String
    }
    if e.Provider.Valid {
        resp.Provider = &e.Provider.String
    }
    if e.InputTokens.Valid {
        v := e.InputTokens.Int32
        resp.InputTokens = &v
    }
    if e.OutputTokens.Valid {
        v := e.OutputTokens.Int32
        resp.OutputTokens = &v
    }
    if e.BillingCode.Valid {
        resp.BillingCode = &e.BillingCode.String
    }
    if e.UsageJson != nil {
        resp.UsageJSON = e.UsageJson
    }
    return resp
}
```

### Wiring in `cmd/ari/run.go`

```go
budgetSvc := budget.New(db, queries)
costHandler := handlers.NewCostHandler(queries, db, budgetSvc)
// pass costHandler as an extra RouteRegistrar to server.New(...)
```

---

## Integration Points

### Agent Status Machine

`BudgetEnforcementService.RecordAndEnforce` calls `domain.ValidateStatusTransition` before
updating an agent's status to `paused`. This ensures auto-pause only fires from `running` or `idle`
(the transition table in `internal/domain/agent.go` allows `running → paused` and `idle → paused`).
A `paused` agent is not re-paused, and an `active` agent that has not started running yet is not
touched by the hard-stop.

### Resume Guard (REQ-014, REQ-025, REQ-027)

The existing `POST /api/agents/{id}/transition` handler validates transitions using
`ValidateStatusTransition`. A budget-paused agent trying to go back to `running` or `idle`
from `paused` is blocked by the domain model: `paused → active` is the only allowed transition,
and that requires an explicit owner action. There is no domain change needed here — the requirement
is already satisfied by the existing `paused → active` (not `paused → running`) path.

However, REQ-014 requires that the resume also check that the new budget allows the transition.
This is enforced by adding a budget guard inside the `TransitionAgentStatus` handler:

```go
// In TransitionAgentStatus, after ValidateStatusTransition passes:
if req.Status == domain.AgentStatusActive || req.Status == domain.AgentStatusIdle {
    // Re-check that budget no longer exceeds 100% (REQ-014).
    if err := budget.CheckResumeAllowed(ctx, qtx, agentID); err != nil {
        writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error(), Code: "BUDGET_EXCEEDED"})
        return
    }
}
```

`budget.CheckResumeAllowed` queries agent + squad monthly spend and returns an error if either
still exceeds 100% of its respective budget.

### Activity Log (Feature 09)

The TODO comments in `RecordAndEnforce` mark where `activity_log` rows will be inserted when
feature 09 is implemented. The activity log entry shape for an auto-pause event:

```
action:     "agent.budget_paused"
entity_type: "agent"
entity_id:  <agent UUID>
squad_id:   <squad UUID>
actor_type: "system"
metadata:   {"spend_cents": N, "budget_cents": N, "threshold": "exceeded"}
```

Because the activity log write is inside the same transaction as the cost event insert and
agent status update, the log entry is guaranteed to be consistent (or rolled back together).

### Inbox Alerts (Future)

Soft alerts (80% threshold) create `inbox_items` rows with `type=budget_warning`,
`category=alert`, `entity_id=<agent or squad UUID>`, `squad_id=<squad UUID>`.
The query first checks for an existing unresolved alert in the current month to avoid duplicates
(REQ-003, REQ-005). When a budget is increased enough to drop below 80% (REQ-026), the alert
is resolved by setting `resolved_at = now()` on the inbox item.

---

## Data Flow Diagrams

### Cost Event Recording + Enforcement

```
POST /api/squads/{id}/cost-events
         │
         ▼
  [Auth middleware]
  Session/Run-Token JWT ─── fail → 401
         │
         ▼
  parseSquadID ─────────── invalid → 404
         │
         ▼
  GetAgentByID ─────────── not found → 403
  check SquadID match ───── mismatch → 403
  check agent.status ─────── terminated → 409
         │
         ▼
  BeginTx
         │
         ├── InsertCostEvent ─────────── dup id → fetch existing → 200 OK
         │
         ├── GetAgentByID (in tx)
         ├── GetAgentMonthlySpend
         │     ├── >= 100%  → UpdateAgent(paused) + (todo) activity_log + inbox
         │     └── >= 80%   → (todo) inbox_items warning
         │
         ├── GetSquadByID (in tx)
         ├── GetSquadMonthlySpend
         │     ├── >= 100%  → ListRunningIdleAgentsBySquad → UpdateAgent*(paused) each
         │     └── >= 80%   → (todo) squad inbox_items warning
         │
         └── Commit ─────────── fail → Rollback → 500 (retry safe)
                │
                ▼
         201 Created  (or 200 OK if duplicate)
```

### Budget Update Re-evaluation

```
PATCH /api/agents/{id}  (with budgetMonthlyCents)
         │
         ▼
  BeginTx
         │
         ├── UpdateAgent (new budget)
         │
         └── budgetSvc.ReEvaluateAgent(ctx, qtx, agentID)
               ├── budget = NULL → (todo) resolve alerts → done
               ├── spend >= 100% → UpdateAgent(paused) if running/idle
               └── spend < 80%  → (todo) resolve alerts
         │
         Commit
```

---

## Error Handling

### Transaction Rollback on Enforcement Failure

The `defer tx.Rollback()` pattern used throughout the codebase (visible in `squad_handler.go`)
ensures that if `RecordAndEnforce` returns an error at any point — whether from the insert,
the spend aggregation query, or the agent status update — the entire transaction is rolled back.
The caller receives `500 Internal Server Error` and may retry. The cost event will not be
persisted without the enforcement check completing (REQ-036).

### Idempotent Submission (REQ-037)

When a client-supplied `id` collides with an existing row, the INSERT returns a PostgreSQL
`23505` unique-violation error. The handler detects this, fetches the existing row via
`GetCostEventByID`, and returns `200 OK` with that row. This is safe within the transaction
because `GetCostEventByID` reads with the same snapshot.

### Rejected Events for Terminated Agents (REQ-029)

The `RecordCostEvent` handler checks `agent.Status == terminated` before opening the transaction.
This is a pre-flight check, not inside the transaction, which is acceptable because:
- Terminated status is a terminal state (cannot be reversed).
- There is no TOCTOU risk: an agent cannot transition out of `terminated`.

### Cross-Squad Access (REQ-030)

The handler verifies `agent.SquadID == squadID` after `GetAgentByID`. If they differ, it returns
`403 Forbidden` with code `FORBIDDEN` — the caller cannot distinguish "agent not found" from
"agent in a different squad" (information leakage prevention).

### Budget Re-evaluation Failure

If `ReEvaluateAgent` or `ReEvaluateSquad` fails within the budget-update transaction, the
`UpdateAgent`/`UpdateSquad` query is also rolled back. The PATCH handler returns `500`. The
budget value is not changed. The operator can retry the PATCH.

---

## Testing Strategy

### Unit Tests — `internal/domain/cost_event_test.go`

Test `ComputeThresholdStatus`:
```
- nil budget → always "ok"
- spend = 0, budget = 1000 → "ok"
- spend = 799, budget = 1000 → "ok"
- spend = 800, budget = 1000 → "warning"
- spend = 999, budget = 1000 → "warning"
- spend = 1000, budget = 1000 → "exceeded"
- spend = 1500, budget = 1000 → "exceeded"
- spend = 0, budget = 0 → "ok" (zero budget treated as no limit)
```

Test `BillingPeriod`:
```
- 2026-03-15 → start=2026-03-01, end=2026-04-01
- 2026-12-31 → start=2026-12-01, end=2027-01-01
- 2026-01-01 → start=2026-01-01, end=2026-02-01
```

Test `ValidateCreateCostEventRequest`:
```
- invalid eventType → error
- costCents = -1 → error
- inputTokens = -1 → error
- model too long (> 100 chars) → error
- invalid usageJson → error
- all optional fields nil → ok
```

### Integration Tests — `internal/server/handlers/cost_integration_test.go`

Follow the pattern in `agent_integration_test.go`: use `makeEnv(t, auth.ModeAuthenticated, false)`
to spin up an in-process test server against an embedded PostgreSQL instance.

**Test cases:**

1. **RecordCostEvent — happy path**
   - Create squad, member, agent (captain, status=active).
   - POST valid cost event → 201; verify response fields.

2. **RecordCostEvent — idempotent replay**
   - POST cost event with explicit `id`.
   - POST same `id` again → 200 with identical response body.

3. **RecordCostEvent — terminated agent**
   - Terminate agent via PATCH.
   - POST cost event → 409 with code `AGENT_TERMINATED`.

4. **RecordCostEvent — cross-squad agent**
   - Create second squad + second agent.
   - POST cost event to first squad with agentId from second squad → 403.

5. **Agent auto-pause at 100%**
   - Set agent `budget_monthly_cents = 100`.
   - POST cost event with `cost_cents = 100`.
   - GET `/api/agents/{id}` → status is `paused`.

6. **Agent warning at 80%**
   - Set agent `budget_monthly_cents = 100`.
   - POST cost event with `cost_cents = 80`.
   - GET `/api/agents/{id}` → status unchanged (still active/running).
   - (When inbox is implemented: verify inbox item created.)

7. **Squad auto-pause at 100%**
   - Create two agents. Set squad `budget_monthly_cents = 100`.
   - POST cost events totalling 100 cents across agents.
   - GET both agents → both are `paused`.

8. **GetCostSummary — no budget set**
   - `budgetMonthlyCents` is null, `percentUtilised` is null, `thresholdStatus` is `"ok"`.

9. **GetCostSummary — with spend**
   - Record events, verify `spendCents`, `percentUtilised`, `thresholdStatus`.

10. **GetCostByAgent — correct breakdown**
    - Multiple agents with different spends; verify order and amounts.

11. **Budget re-evaluation on PATCH agent**
    - Set budget = 100, record 100 cents (agent is paused).
    - PATCH agent budget to 200.
    - Verify agent is still paused (REQ-027: manual resume required).
    - Manually resume agent → allowed because spend (100) < new budget (200).

12. **Budget re-evaluation — spend already exceeds new budget**
    - Record 150 cents, then PATCH budget to 100.
    - Verify agent is auto-paused by re-evaluation.

### E2E Tests

Run via Playwright against a live `make dev` server:

1. Create squad → set budget → create agent → record cost events via API.
2. Dashboard cost summary widget reflects updated spend.
3. Budget warning at 80% visible in UI (once inbox is wired).
4. Agent status shows `paused` after 100% breach.
5. Owner can manually resume after budget increase; non-owner cannot.

---

## System Context

- **Depends On:** PostgreSQL (cost_events table), agent status machine (`internal/domain/agent.go`),
  squad/agent budget columns (already in schema).
- **Used By:** Dashboard budget widgets, agent detail page, future billing exports.
- **Feature Dependencies:**
  - Feature 09 (Activity Log): `agent.budget_paused` log entries (marked as TODO, falls back to `slog.Info`).
  - Feature 11 (Agent Runtime): Run Token JWT for `POST /api/squads/:id/cost-events` and
    `GET /api/agent/me/budget`. Interim stub returns `501` for the `me/budget` endpoint.
- **External Dependencies:** None.

---

## Open Questions (Resolved)

| Question | Resolution |
|----------|------------|
| Calendar months or rolling 30 days? | UTC calendar months only in v1 (REQ-022). |
| Include model/token metadata? | Yes — stored in `cost_events` (REQ-001, REQ-028). |
| Automatic budget resets? | Not applicable — spend is always computed on-the-fly (REQ-015). |
| Inbox not yet implemented? | Soft-alert steps use `slog.Info` + TODO comments; hard-stop (auto-pause) proceeds without inbox (REQ-004). |
| `heartbeat_run_id` FK? | Stored as bare UUID until feature 11 migration; no FK constraint yet. |

---

## References

- Requirements: `docx/features/10-cost-events-budget/requirements.md`
- Agent domain model + status machine: `internal/domain/agent.go`
- Squad budget handler: `internal/server/handlers/squad_handler.go`
- Agent handler (update + transition patterns): `internal/server/handlers/agent_handler.go`
- Squads migration (budget column): `internal/database/migrations/20260314000003_create_squads.sql`
- Agents migration (budget column, status enum): `internal/database/migrations/20260314000005_create_agents.sql`
- sqlc config: `sqlc.yaml`
- PRD § 4, 5.2.3, 6.2, 7.3, 9: `docx/core/01-PRODUCT.md`
