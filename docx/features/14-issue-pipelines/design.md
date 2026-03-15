# Design: Issue Pipelines (Multi-Agent Workflows)

**Created:** 2026-03-15
**Status:** Ready for Implementation
**Feature:** 14-issue-pipelines
**Dependencies:** 05-issue-tracking, 11-agent-runtime, 13-conversations

---

## 1. Architecture Overview

Issue Pipelines add a workflow orchestration layer on top of the existing issue and agent runtime systems. A pipeline is a reusable template defining a sequence of stages. When an issue is attached to a pipeline, the system tracks which stage the issue is at and orchestrates agent hand-offs as the issue progresses.

### High-Level Flow

```
User creates Pipeline + Stages
        |
        v
User attaches Issue to Pipeline
        |
        v
Issue.current_stage_id = Stage[1]
Issue.assignee_agent_id = Stage[1].assigned_agent_id
        |
        v
WakeupService.Enqueue(agentId, "assignment")
        |
        v
Agent wakes, works on issue, marks done
        |
        v
IssueHandler.UpdateIssue detects status=done + pipeline
        |
        v
PipelineService.AutoAdvanceOnDone():
  Step 1: Persist status=done for current stage
  Step 2: Advance to next stage:
    - current_stage_id = Stage[2]
    - assignee_agent_id = Stage[2].assigned_agent_id
    - status = todo  (done→todo is valid reopen transition)
    - WakeupService.Enqueue(nextAgentId, "assignment")
        |
        v
[Repeat until final stage completes → issue.status = done]
```

### Component Relationships

```
PipelineHandler          ← CRUD for pipelines and stages
       |
       v
PipelineService          ← Business logic: attach, advance, reject
       |
       +--→ sqlc Queries (pipelines, pipeline_stages, issues)
       +--→ WakeupService.Enqueue() (auto-wake on transition)
       +--→ SSE Hub (broadcast events)
       +--→ logActivity() (audit trail — same pattern as InboxService)
       |
       v
IssueHandler (modified)  ← Intercepts status=done for auto-advance
       |
       +--→ pipelineSvc (constructor-injected via NewIssueHandler)
```

### Squad Isolation

All pipelines and pipeline stages inherit squad scope from the pipeline's `squad_id`. The `issues` table gains `pipeline_id` and `current_stage_id` foreign keys but remains squad-scoped through its own `squad_id`. Cross-squad attachment is blocked at the service layer (REQ-PIP-033).

---

## 2. Database Schema

### Migration: `20260316000016_create_pipelines.sql`

```sql
-- +goose Up

-- Pipeline definitions (workflow templates)
CREATE TABLE pipelines (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id    UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    name        TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    description TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_pipelines_squad_name UNIQUE (squad_id, name)
);

-- [M-2] Removed redundant idx_pipelines_squad_id — uq_pipelines_squad_name already covers squad_id lookups.
CREATE INDEX idx_pipelines_squad_active ON pipelines (squad_id) WHERE is_active = true;

-- Pipeline stages (ordered steps within a pipeline)
CREATE TABLE pipeline_stages (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id       UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    name              TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    description       TEXT,
    position          INTEGER NOT NULL CHECK (position >= 1),
    assigned_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    gate_id           UUID DEFAULT NULL,  -- [XC-1] v2: FK to approval_gates table; NULL = no gate
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_pipeline_stages_position UNIQUE (pipeline_id, position)
);

CREATE INDEX idx_pipeline_stages_pipeline_id ON pipeline_stages (pipeline_id);
CREATE INDEX idx_pipeline_stages_agent ON pipeline_stages (assigned_agent_id)
    WHERE assigned_agent_id IS NOT NULL;

-- Add pipeline tracking columns to issues
ALTER TABLE issues
    ADD COLUMN pipeline_id       UUID REFERENCES pipelines(id) ON DELETE SET NULL,
    ADD COLUMN current_stage_id  UUID REFERENCES pipeline_stages(id) ON DELETE SET NULL;

-- [DB] CHECK constraint: pipeline_id and current_stage_id must be consistent.
-- current_stage_id can only be set when pipeline_id is set.
ALTER TABLE issues
    ADD CONSTRAINT chk_issues_pipeline_stage_consistency
    CHECK ((pipeline_id IS NULL AND current_stage_id IS NULL) OR (pipeline_id IS NOT NULL));

CREATE INDEX idx_issues_pipeline_id ON issues (pipeline_id)
    WHERE pipeline_id IS NOT NULL;
CREATE INDEX idx_issues_current_stage_id ON issues (current_stage_id)
    WHERE current_stage_id IS NOT NULL;

-- Auto-update updated_at triggers
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_pipelines_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_pipelines_updated_at
    BEFORE UPDATE ON pipelines
    FOR EACH ROW
    EXECUTE FUNCTION update_pipelines_updated_at();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_pipeline_stages_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_pipeline_stages_updated_at
    BEFORE UPDATE ON pipeline_stages
    FOR EACH ROW
    EXECUTE FUNCTION update_pipeline_stages_updated_at();

-- +goose Down
-- [H-4] Drop indexes first, then columns, then tables.
DROP INDEX IF EXISTS idx_issues_current_stage_id;
DROP INDEX IF EXISTS idx_issues_pipeline_id;
ALTER TABLE issues DROP CONSTRAINT IF EXISTS chk_issues_pipeline_stage_consistency;
ALTER TABLE issues DROP COLUMN IF EXISTS current_stage_id;
ALTER TABLE issues DROP COLUMN IF EXISTS pipeline_id;
DROP TRIGGER IF EXISTS trg_pipeline_stages_updated_at ON pipeline_stages;
DROP FUNCTION IF EXISTS update_pipeline_stages_updated_at;
DROP TRIGGER IF EXISTS trg_pipelines_updated_at ON pipelines;
DROP FUNCTION IF EXISTS update_pipelines_updated_at;
DROP TABLE IF EXISTS pipeline_stages;
DROP TABLE IF EXISTS pipelines;
```

---

## 3. SQL Queries (sqlc)

### File: `internal/database/queries/pipelines.sql`

```sql
-- name: CreatePipeline :one
INSERT INTO pipelines (squad_id, name, description, is_active)
VALUES (@squad_id, @name, @description, @is_active)
RETURNING *;

-- name: GetPipelineByID :one
SELECT * FROM pipelines WHERE id = @id;

-- name: ListPipelinesBySquad :many
SELECT * FROM pipelines
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_is_active')::BOOLEAN IS NULL OR is_active = sqlc.narg('filter_is_active'))
ORDER BY name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPipelinesBySquad :one
SELECT count(*) FROM pipelines
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_is_active')::BOOLEAN IS NULL OR is_active = sqlc.narg('filter_is_active'));

-- name: UpdatePipeline :one
UPDATE pipelines
SET
    name        = COALESCE(sqlc.narg('name'), name),
    description = CASE WHEN sqlc.arg('set_description')::boolean THEN sqlc.narg('description') ELSE description END,
    is_active   = COALESCE(sqlc.narg('is_active'), is_active)
WHERE id = @id
RETURNING *;

-- name: DeletePipeline :exec
DELETE FROM pipelines WHERE id = @id;

-- name: CountIssuesInPipeline :one
SELECT count(*) FROM issues WHERE pipeline_id = @pipeline_id;
```

### File: `internal/database/queries/pipeline_stages.sql`

```sql
-- name: CreatePipelineStage :one
INSERT INTO pipeline_stages (pipeline_id, name, description, position, assigned_agent_id)
VALUES (@pipeline_id, @name, @description, @position, @assigned_agent_id)
RETURNING *;

-- name: GetPipelineStageByID :one
SELECT * FROM pipeline_stages WHERE id = @id;

-- name: ListStagesByPipeline :many
SELECT * FROM pipeline_stages
WHERE pipeline_id = @pipeline_id
ORDER BY position ASC;

-- name: UpdatePipelineStage :one
UPDATE pipeline_stages
SET
    name              = COALESCE(sqlc.narg('name'), name),
    description       = CASE WHEN sqlc.arg('set_description')::boolean THEN sqlc.narg('description') ELSE description END,
    position          = COALESCE(sqlc.narg('position'), position),
    assigned_agent_id = CASE WHEN sqlc.arg('set_assigned_agent')::boolean THEN sqlc.narg('assigned_agent_id') ELSE assigned_agent_id END
WHERE id = @id
RETURNING *;

-- name: DeletePipelineStage :exec
DELETE FROM pipeline_stages WHERE id = @id;

-- name: CountIssuesAtStage :one
SELECT count(*) FROM issues WHERE current_stage_id = @stage_id;

-- name: GetFirstStage :one
SELECT * FROM pipeline_stages
WHERE pipeline_id = @pipeline_id
ORDER BY position ASC
LIMIT 1;

-- name: GetNextStage :one
SELECT * FROM pipeline_stages
WHERE pipeline_id = @pipeline_id
  AND position > @current_position
ORDER BY position ASC
LIMIT 1;

-- name: GetPreviousStage :one
SELECT * FROM pipeline_stages
WHERE pipeline_id = @pipeline_id
  AND position < @current_position
ORDER BY position DESC
LIMIT 1;

-- name: CountStagesByPipeline :one
SELECT count(*) FROM pipeline_stages WHERE pipeline_id = @pipeline_id;

-- [M-1] Batch reorder stages: update positions for all stages in a pipeline.
-- name: ReorderPipelineStages :exec
UPDATE pipeline_stages
SET position = new_positions.position
FROM (
    SELECT unnest(@ids::UUID[]) AS id,
           unnest(@positions::INTEGER[]) AS position
) AS new_positions
WHERE pipeline_stages.id = new_positions.id
  AND pipeline_stages.pipeline_id = @pipeline_id;
```

### Additional Issue Queries (append to `internal/database/queries/issues.sql`)

```sql
-- name: UpdateIssuePipeline :one
-- [M-6] Note: For pipeline attachment, prefer using this within a transaction
-- or use AdvanceIssuePipelineStage CAS query to avoid race conditions.
UPDATE issues
SET
    pipeline_id       = sqlc.narg('pipeline_id'),
    current_stage_id  = sqlc.narg('current_stage_id'),
    assignee_agent_id = CASE WHEN sqlc.arg('set_assignee_agent')::boolean THEN sqlc.narg('assignee_agent_id') ELSE assignee_agent_id END,
    status            = COALESCE(sqlc.narg('status'), status)
WHERE id = @id
RETURNING *;

-- [C-3] CAS query for concurrent-safe stage advancement.
-- name: AdvanceIssuePipelineStage :one
UPDATE issues
SET
    current_stage_id  = @next_stage_id,
    assignee_agent_id = @next_agent_id,
    status            = 'todo'
WHERE id = @id
  AND current_stage_id = @expected_stage_id  -- CAS guard
RETURNING *;
```

### [H-5] Additional issue query filters for pipeline_id

```sql
-- Add to ListIssuesBySquad query: optional pipeline_id filter
-- AND (sqlc.narg('filter_pipeline_id')::UUID IS NULL OR pipeline_id = sqlc.narg('filter_pipeline_id'))

-- Add to CountIssuesBySquad query: optional pipeline_id filter
-- AND (sqlc.narg('filter_pipeline_id')::UUID IS NULL OR pipeline_id = sqlc.narg('filter_pipeline_id'))
```

> **Implementation note:** The `filter_pipeline_id` parameter must be added to both `ListIssuesBySquad` and `CountIssuesBySquad` in `internal/database/queries/issues.sql`, following the same pattern as existing optional filters.

---

## 4. Domain Model

### File: `internal/domain/pipeline.go`

```go
package domain

import (
    "fmt"
    "time"

    "github.com/google/uuid"
)

// -------- Domain Models --------

type Pipeline struct {
    ID          uuid.UUID  `json:"id"`
    SquadID     uuid.UUID  `json:"squadId"`
    Name        string     `json:"name"`
    Description *string    `json:"description,omitempty"`
    IsActive    bool       `json:"isActive"`
    CreatedAt   time.Time  `json:"createdAt"`
    UpdatedAt   time.Time  `json:"updatedAt"`
}

type PipelineStage struct {
    ID              uuid.UUID  `json:"id"`
    PipelineID      uuid.UUID  `json:"pipelineId"`
    Name            string     `json:"name"`
    Description     *string    `json:"description,omitempty"`
    Position        int        `json:"position"`
    AssignedAgentID *uuid.UUID `json:"assignedAgentId,omitempty"`
    GateID          *uuid.UUID `json:"gateId,omitempty"` // v2: approval gate reference
    CreatedAt       time.Time  `json:"createdAt"`
    UpdatedAt       time.Time  `json:"updatedAt"`
}

// PipelineWithStages is a convenience type for API responses that
// include the full pipeline definition with all stages.
type PipelineWithStages struct {
    Pipeline
    Stages []PipelineStage `json:"stages"`
}

// -------- Request / Response DTOs --------

type CreatePipelineRequest struct {
    Name        string  `json:"name"`
    Description *string `json:"description,omitempty"`
    IsActive    *bool   `json:"isActive,omitempty"`
}

type UpdatePipelineRequest struct {
    Name           *string `json:"name,omitempty"`
    Description    *string `json:"description,omitempty"`
    SetDescription bool    `json:"-"`
    IsActive       *bool   `json:"isActive,omitempty"`
}

type CreatePipelineStageRequest struct {
    Name            string     `json:"name"`
    Description     *string    `json:"description,omitempty"`
    Position        int        `json:"position"`
    AssignedAgentID *uuid.UUID `json:"assignedAgentId,omitempty"`
}

type UpdatePipelineStageRequest struct {
    Name             *string    `json:"name,omitempty"`
    Description      *string    `json:"description,omitempty"`
    SetDescription   bool       `json:"-"`
    Position         *int       `json:"position,omitempty"`
    AssignedAgentID  *uuid.UUID `json:"assignedAgentId,omitempty"`
    SetAssignedAgent bool       `json:"-"`
}

type AdvanceIssueRequest struct {
    // Empty body -- advancement uses the issue's current pipeline state.
}

type RejectIssueRequest struct {
    Reason *string `json:"reason,omitempty"`
}

// -------- Validation --------

func ValidateCreatePipelineInput(input CreatePipelineRequest) error {
    if input.Name == "" {
        return fmt.Errorf("name is required")
    }
    if len(input.Name) > 200 {
        return fmt.Errorf("name must not exceed 200 characters")
    }
    return nil
}

func ValidateUpdatePipelineInput(input UpdatePipelineRequest) error {
    if input.Name != nil {
        if *input.Name == "" {
            return fmt.Errorf("name must not be empty")
        }
        if len(*input.Name) > 200 {
            return fmt.Errorf("name must not exceed 200 characters")
        }
    }
    return nil
}

func ValidateCreateStageInput(input CreatePipelineStageRequest) error {
    if input.Name == "" {
        return fmt.Errorf("name is required")
    }
    if len(input.Name) > 200 {
        return fmt.Errorf("name must not exceed 200 characters")
    }
    if input.Position < 1 {
        return fmt.Errorf("position must be a positive integer (>= 1)")
    }
    return nil
}

func ValidateUpdateStageInput(input UpdatePipelineStageRequest) error {
    if input.Name != nil {
        if *input.Name == "" {
            return fmt.Errorf("name must not be empty")
        }
        if len(*input.Name) > 200 {
            return fmt.Errorf("name must not exceed 200 characters")
        }
    }
    if input.Position != nil && *input.Position < 1 {
        return fmt.Errorf("position must be a positive integer (>= 1)")
    }
    return nil
}
```

---

## 5. Service Layer

### File: `internal/server/handlers/pipeline_service.go`

The `PipelineService` encapsulates pipeline business logic, separating it from HTTP concerns. It uses `logActivity(ctx, qtx, ActivityParams{...})` directly, matching the pattern used by `InboxService` and `IssueHandler`.

```go
package handlers

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/google/uuid"

    "github.com/xb/ari/internal/database/db"
    "github.com/xb/ari/internal/domain"
    "github.com/xb/ari/internal/server/sse"
)

// [H-1] PipelineService uses logActivity() directly (same pattern as InboxService),
// not an ActivityLogFunc type which does not exist.
type PipelineService struct {
    queries   *db.Queries
    dbConn    *sql.DB
    wakeupSvc *WakeupService
    sseHub    *sse.Hub
}

func NewPipelineService(
    q *db.Queries,
    dbConn *sql.DB,
    wakeupSvc *WakeupService,
    sseHub *sse.Hub,
) *PipelineService {
    return &PipelineService{
        queries:   q,
        dbConn:    dbConn,
        wakeupSvc: wakeupSvc,
        sseHub:    sseHub,
    }
}
```

### Key Service Methods

#### `AttachIssueToPipeline`

```go
// AttachIssueToPipeline sets the issue's pipeline_id and current_stage_id
// to the first stage, assigns the first stage's agent, and auto-wakes.
// [M-6] Runs within a transaction for atomicity.
func (s *PipelineService) AttachIssueToPipeline(
    ctx context.Context, issueID, pipelineID uuid.UUID,
) (db.Issue, error) {
    // 1. Load pipeline, verify squad match, active, has stages
    // 2. Get first stage (lowest position)
    // 3. Update issue: pipeline_id, current_stage_id, assignee_agent_id, status
    //    (use UpdateIssuePipeline within a transaction)
    // 4. If first stage has assigned_agent_id, enqueue wakeup
    // 5. Log activity: logActivity(ctx, qtx, ActivityParams{Action: "issue.pipeline.attached", ...})
    // 6. Broadcast SSE: issue.pipeline.stage_changed
    // 7. Return updated issue
}
```

#### `AdvanceStage`

```go
// AdvanceStage moves the issue to the next pipeline stage.
// If at the final stage, completes the pipeline.
//
// [C-2] Two-step status transition:
// The agent marks the issue as done (in_progress→done). Then AdvanceStage
// sets status=todo for the next stage assignment. The done→todo transition
// is a valid "reopen" in the state machine (see issueValidTransitions in
// internal/domain/issue.go).
//
// [XC-1] Approval gate hook: if the target stage has a non-null gate_id,
// the system should create an approval inbox item instead of auto-advancing.
// This is a v2 enhancement — for now, gate_id is ignored and advancement
// proceeds normally. The hook point is documented here for future implementation.
func (s *PipelineService) AdvanceStage(
    ctx context.Context, issueID uuid.UUID,
) (db.Issue, error) {
    // 1. Load issue, verify pipeline_id and current_stage_id are set
    // 2. Load current stage to get position
    // 3. Query next stage (position > current, ORDER BY position ASC LIMIT 1)
    // 4a. If next stage exists:
    //     - Use AdvanceIssuePipelineStage CAS query:
    //       UPDATE issues SET current_stage_id=@next, assignee_agent_id=@agent, status='todo'
    //       WHERE id=@id AND current_stage_id=@expected
    //     - If CAS returns 0 rows, return HTTP 409 (concurrent advance)
    //     - Enqueue wakeup if agent assigned (and gate_id is NULL)
    //     - Log activity: logActivity(ctx, qtx, ActivityParams{Action: "issue.pipeline.advanced", ...})
    //     - Broadcast SSE: issue.pipeline.stage_changed
    // 4b. If no next stage (final):
    //     - Update issue: current_stage_id = NULL, status = done
    //     - Log activity: logActivity(ctx, qtx, ActivityParams{Action: "issue.pipeline.completed", ...})
    //     - Broadcast SSE: issue.pipeline.stage_changed (transition=complete)
    // 5. Return updated issue
}
```

#### `RejectStage`

```go
// RejectStage moves the issue back to the previous pipeline stage.
func (s *PipelineService) RejectStage(
    ctx context.Context, issueID uuid.UUID, reason *string,
) (db.Issue, error) {
    // 1. Load issue, verify pipeline_id and current_stage_id are set
    // 2. Load current stage to get position
    // 3. Query previous stage (position < current, ORDER BY position DESC LIMIT 1)
    // 4. If no previous stage, return error NO_PREVIOUS_STAGE
    // 5. Update issue: current_stage_id = prev, assignee_agent_id = prev.assigned_agent_id, status = todo
    // 6. If reason provided, create system comment on issue
    // 7. Enqueue wakeup if agent assigned
    // 8. Log activity: logActivity(ctx, qtx, ActivityParams{Action: "issue.pipeline.rejected", ...})
    // 9. Broadcast SSE: issue.pipeline.stage_changed (transition=reject)
    // 10. Return updated issue
}
```

#### `AutoAdvanceOnDone` (called from IssueHandler)

```go
// AutoAdvanceOnDone checks if an issue has a pipeline and auto-advances
// instead of setting status=done. Returns true if auto-advance was triggered.
//
// [C-2] Two-step process:
// 1. The caller (IssueHandler) has already persisted status=done for the current stage.
// 2. This method then calls AdvanceStage, which sets status=todo for the next
//    stage assignment (done→todo is a valid reopen transition).
func (s *PipelineService) AutoAdvanceOnDone(
    ctx context.Context, issueID uuid.UUID,
) (handled bool, updatedIssue db.Issue, err error) {
    // 1. Load issue
    // 2. If pipeline_id is NULL, return handled=false
    // 3. Call AdvanceStage (which handles final-stage completion)
    // 4. Return handled=true, updated issue
}
```

### Integration with IssueHandler

The existing `IssueHandler.UpdateIssue` method must be modified to intercept `status=done` transitions for pipeline-attached issues:

```go
// [H-3] PipelineService is constructor-injected into IssueHandler via NewIssueHandler,
// not setter-injected. See section 6 for the updated constructor signature.

// In IssueHandler.UpdateIssue, after status validation passes and the
// done status has been persisted in the transaction:
if newStatus == domain.IssueStatusDone && existingIssue.PipelineID.Valid {
    // [C-2] The done status is already committed to DB at this point.
    // Now auto-advance: sets status back to todo for the next stage.
    handled, updatedIssue, err := h.pipelineSvc.AutoAdvanceOnDone(ctx, issueID)
    if err != nil {
        // handle error
    }
    if handled {
        // Return the updated issue (now at next stage with status=todo, or completed with status=done)
        return
    }
}
```

---

## 6. Handler Layer

### File: `internal/server/handlers/pipeline_handler.go`

```go
package handlers

type PipelineHandler struct {
    queries     *db.Queries
    dbConn      *sql.DB
    pipelineSvc *PipelineService
}

func NewPipelineHandler(
    q *db.Queries,
    dbConn *sql.DB,
    pipelineSvc *PipelineService,
) *PipelineHandler {
    return &PipelineHandler{
        queries:     q,
        dbConn:      dbConn,
        pipelineSvc: pipelineSvc,
    }
}

func (h *PipelineHandler) RegisterRoutes(mux *http.ServeMux) {
    // Pipeline CRUD
    mux.HandleFunc("POST /api/squads/{squadId}/pipelines", h.CreatePipeline)
    mux.HandleFunc("GET /api/squads/{squadId}/pipelines", h.ListPipelines)
    mux.HandleFunc("GET /api/pipelines/{id}", h.GetPipeline)
    mux.HandleFunc("PATCH /api/pipelines/{id}", h.UpdatePipeline)
    mux.HandleFunc("DELETE /api/pipelines/{id}", h.DeletePipeline)

    // Stage CRUD
    mux.HandleFunc("POST /api/pipelines/{id}/stages", h.CreateStage)
    mux.HandleFunc("PATCH /api/pipeline-stages/{stageId}", h.UpdateStage)
    mux.HandleFunc("DELETE /api/pipeline-stages/{stageId}", h.DeleteStage)

    // Issue pipeline operations
    mux.HandleFunc("POST /api/issues/{id}/advance", h.AdvanceIssue)
    mux.HandleFunc("POST /api/issues/{id}/reject", h.RejectIssue)
}
```

### [H-3] Updated IssueHandler Constructor

The `PipelineService` must be constructor-injected into `IssueHandler`, not setter-injected:

```go
// Updated NewIssueHandler signature — replaces the current one.
func NewIssueHandler(q *db.Queries, dbConn *sql.DB, pipelineSvc *PipelineService) *IssueHandler {
    return &IssueHandler{queries: q, dbConn: dbConn, pipelineSvc: pipelineSvc}
}
```

> **Note:** The existing `SetWakeupService` setter pattern remains for `wakeupSvc` since it has a circular dependency. `PipelineService` does not have this issue and should use constructor injection.

### [H-2] Updated issueResponse and dbIssueToResponse

The `issueResponse` struct and `dbIssueToResponse` function in `issue_handler.go` must be updated to include pipeline fields:

```go
type issueResponse struct {
    // ... existing fields ...
    PipelineID     *uuid.UUID `json:"pipelineId"`      // [H-2] Added
    CurrentStageID *uuid.UUID `json:"currentStageId"`   // [H-2] Added
}

func dbIssueToResponse(i db.Issue) issueResponse {
    resp := issueResponse{
        // ... existing field mappings ...
    }
    // ... existing nullable field mappings ...
    if i.PipelineID.Valid {        // [H-2] Added
        resp.PipelineID = &i.PipelineID.UUID
    }
    if i.CurrentStageID.Valid {    // [H-2] Added
        resp.CurrentStageID = &i.CurrentStageID.UUID
    }
    return resp
}
```

> **[XH-1]:** This also ensures that assignment-related responses (e.g., from agent runtime task checkout) include `pipelineId` and `currentStageId` in the issue DTO.

### Handler Method Signatures

```go
// CreatePipeline handles POST /api/squads/{squadId}/pipelines
// Request: { "name": "Code Review Pipeline", "description": "...", "isActive": true }
// Response: 201 { pipeline object }
func (h *PipelineHandler) CreatePipeline(w http.ResponseWriter, r *http.Request)

// ListPipelines handles GET /api/squads/{squadId}/pipelines?is_active=true&limit=50&offset=0
// Response: 200 { "data": [...], "total": N }
func (h *PipelineHandler) ListPipelines(w http.ResponseWriter, r *http.Request)

// GetPipeline handles GET /api/pipelines/{id}
// Response: 200 { pipeline object with stages[] }
func (h *PipelineHandler) GetPipeline(w http.ResponseWriter, r *http.Request)

// UpdatePipeline handles PATCH /api/pipelines/{id}
// Request: { "name": "Updated Name", "isActive": false }
// Response: 200 { pipeline object }
func (h *PipelineHandler) UpdatePipeline(w http.ResponseWriter, r *http.Request)

// DeletePipeline handles DELETE /api/pipelines/{id}
// Response: 204 (or 422 if in use per REQ-PIP-090)
func (h *PipelineHandler) DeletePipeline(w http.ResponseWriter, r *http.Request)

// CreateStage handles POST /api/pipelines/{id}/stages
// Request: { "name": "Code Review", "position": 2, "assignedAgentId": "...", "description": "..." }
// Response: 201 { stage object }
func (h *PipelineHandler) CreateStage(w http.ResponseWriter, r *http.Request)

// UpdateStage handles PATCH /api/pipeline-stages/{stageId}
// Request: { "name": "Updated", "position": 3, "assignedAgentId": null }
// Response: 200 { stage object }
func (h *PipelineHandler) UpdateStage(w http.ResponseWriter, r *http.Request)

// DeleteStage handles DELETE /api/pipeline-stages/{stageId}
// Response: 204 (or 422 if in use per REQ-PIP-091)
func (h *PipelineHandler) DeleteStage(w http.ResponseWriter, r *http.Request)

// AdvanceIssue handles POST /api/issues/{id}/advance
// Request: {} (empty body)
// Response: 200 { updated issue }
func (h *PipelineHandler) AdvanceIssue(w http.ResponseWriter, r *http.Request)

// RejectIssue handles POST /api/issues/{id}/reject
// Request: { "reason": "Code quality issues found" } (reason optional)
// Response: 200 { updated issue }
func (h *PipelineHandler) RejectIssue(w http.ResponseWriter, r *http.Request)
```

---

## 7. API Endpoints

### Pipeline CRUD

#### `POST /api/squads/{squadId}/pipelines`

**Request:**
```json
{
  "name": "Code Review Pipeline",
  "description": "Three-stage code review workflow",
  "isActive": true
}
```

**Response (201):**
```json
{
  "id": "a1b2c3d4-...",
  "squadId": "s1q2u3a4-...",
  "name": "Code Review Pipeline",
  "description": "Three-stage code review workflow",
  "isActive": true,
  "createdAt": "2026-03-15T10:00:00Z",
  "updatedAt": "2026-03-15T10:00:00Z"
}
```

#### `GET /api/squads/{squadId}/pipelines`

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `is_active` | boolean | -- | Filter by active/inactive |
| `limit` | int | 50 | Max results (max 200) |
| `offset` | int | 0 | Pagination offset |

**Response (200):**
```json
{
  "data": [
    {
      "id": "a1b2c3d4-...",
      "squadId": "s1q2u3a4-...",
      "name": "Code Review Pipeline",
      "description": "...",
      "isActive": true,
      "createdAt": "2026-03-15T10:00:00Z",
      "updatedAt": "2026-03-15T10:00:00Z"
    }
  ],
  "total": 1
}
```

#### `GET /api/pipelines/{id}`

**Response (200):**
```json
{
  "id": "a1b2c3d4-...",
  "squadId": "s1q2u3a4-...",
  "name": "Code Review Pipeline",
  "description": "Three-stage code review workflow",
  "isActive": true,
  "createdAt": "2026-03-15T10:00:00Z",
  "updatedAt": "2026-03-15T10:00:00Z",
  "stages": [
    {
      "id": "st1-...",
      "pipelineId": "a1b2c3d4-...",
      "name": "Implementation",
      "description": "Agent implements the feature",
      "position": 1,
      "assignedAgentId": "ag1-...",
      "createdAt": "2026-03-15T10:00:00Z",
      "updatedAt": "2026-03-15T10:00:00Z"
    },
    {
      "id": "st2-...",
      "pipelineId": "a1b2c3d4-...",
      "name": "Code Review",
      "description": "Reviewer agent checks code quality",
      "position": 2,
      "assignedAgentId": "ag2-...",
      "createdAt": "2026-03-15T10:01:00Z",
      "updatedAt": "2026-03-15T10:01:00Z"
    },
    {
      "id": "st3-...",
      "pipelineId": "a1b2c3d4-...",
      "name": "QA Verification",
      "description": "QA agent runs tests",
      "position": 3,
      "assignedAgentId": "ag3-...",
      "createdAt": "2026-03-15T10:02:00Z",
      "updatedAt": "2026-03-15T10:02:00Z"
    }
  ]
}
```

#### `PATCH /api/pipelines/{id}`

**Request:**
```json
{
  "name": "Updated Pipeline Name",
  "isActive": false
}
```

**Response (200):** Updated pipeline object (same shape as create response).

#### `DELETE /api/pipelines/{id}`

**Response (204):** No content.

**Error (422):**
```json
{
  "error": "Cannot delete pipeline: 3 issues are currently attached",
  "code": "PIPELINE_IN_USE"
}
```

### Stage CRUD

#### `POST /api/pipelines/{id}/stages`

**Request:**
```json
{
  "name": "Code Review",
  "description": "Reviewer agent checks code quality",
  "position": 2,
  "assignedAgentId": "ag2-..."
}
```

**Response (201):** Stage object.

#### `PATCH /api/pipeline-stages/{stageId}`

**Request:**
```json
{
  "name": "Updated Stage Name",
  "position": 3,
  "assignedAgentId": null
}
```

**Response (200):** Updated stage object.

#### `DELETE /api/pipeline-stages/{stageId}`

**Response (204):** No content.

**Error (422):**
```json
{
  "error": "Cannot delete stage: 1 issue is currently at this stage",
  "code": "STAGE_IN_USE"
}
```

### Issue Pipeline Operations

#### `POST /api/issues/{id}/advance`

**Request:** Empty body `{}` or no body.

**Response (200):** Updated issue object with new `currentStageId` and `assigneeAgentId`.

Example response when advancing from stage 1 to stage 2:
```json
{
  "id": "iss-...",
  "squadId": "sq-...",
  "identifier": "ARI-42",
  "title": "Implement login feature",
  "status": "todo",
  "pipelineId": "pip-...",
  "currentStageId": "st2-...",
  "assigneeAgentId": "ag2-...",
  "updatedAt": "2026-03-15T11:00:00Z"
}
```

Example response when advancing from final stage (pipeline complete):
```json
{
  "id": "iss-...",
  "squadId": "sq-...",
  "identifier": "ARI-42",
  "title": "Implement login feature",
  "status": "done",
  "pipelineId": "pip-...",
  "currentStageId": null,
  "assigneeAgentId": "ag3-...",
  "updatedAt": "2026-03-15T12:00:00Z"
}
```

#### `POST /api/issues/{id}/reject`

**Request:**
```json
{
  "reason": "Unit tests are failing, needs fixes before review can proceed"
}
```

**Response (200):** Updated issue object with previous stage's `currentStageId` and `assigneeAgentId`.

---

## 8. SSE Events

New events added to the squad SSE stream (`GET /api/squads/{id}/events/stream`):

| Event Type | Trigger | Payload |
|---|---|---|
| `pipeline.created` | New pipeline created | `{ "pipelineId", "squadId", "name" }` |
| `pipeline.updated` | Pipeline fields changed | `{ "pipelineId", "squadId", "changes" }` |
| `pipeline.deleted` | Pipeline deleted | `{ "pipelineId", "squadId" }` |
| `issue.pipeline.stage_changed` | Issue stage transition | `{ "issueId", "identifier", "pipelineId", "fromStageId", "toStageId", "fromStageName", "toStageName", "transition": "advance\|reject\|attach\|detach\|complete" }` |

Example SSE frame for stage advancement:
```
event: issue.pipeline.stage_changed
data: {"issueId":"iss-...","identifier":"ARI-42","pipelineId":"pip-...","fromStageId":"st1-...","toStageId":"st2-...","fromStageName":"Implementation","toStageName":"Code Review","transition":"advance"}
id: 147

```

---

## 9. Domain Model Updates

### Issue struct additions

Add to `internal/domain/issue.go`:

```go
// Add to Issue struct:
type Issue struct {
    // ... existing fields ...
    PipelineID     *uuid.UUID `json:"pipelineId,omitempty"`
    CurrentStageID *uuid.UUID `json:"currentStageId,omitempty"`
}

// Add to CreateIssueRequest:
type CreateIssueRequest struct {
    // ... existing fields ...
    PipelineID *uuid.UUID `json:"pipelineId,omitempty"`
}

// Add to UpdateIssueRequest:
type UpdateIssueRequest struct {
    // ... existing fields ...
    PipelineID    *uuid.UUID `json:"pipelineId,omitempty"`
    SetPipeline   bool       `json:"-"`
}
```

---

## 10. React UI Components

### Pipeline Builder Page

**Route:** `/squads/{squadId}/pipelines`

```
src/
  pages/
    PipelinesPage.tsx          # List view with create button
    PipelineBuilderPage.tsx    # Create/edit pipeline with drag-and-drop stages
  components/
    pipelines/
      PipelineCard.tsx         # Pipeline summary card for list view
      PipelineStageList.tsx    # Ordered list of stages with drag handles
      PipelineStageForm.tsx    # Add/edit stage form (name, agent, description)
      AgentSelector.tsx        # Dropdown to pick agent for a stage
      IssuePipelineIndicator.tsx  # Badge/progress bar showing current stage
      PipelineAttachDialog.tsx # Dialog to attach an issue to a pipeline
```

### PipelinesPage (`/squads/{squadId}/pipelines`)

- Lists all pipelines for the squad in card format
- Filter toggle: Active / Inactive / All
- "Create Pipeline" button opens builder
- Each card shows: name, description, stage count, active issues count

### PipelineBuilderPage (`/squads/{squadId}/pipelines/{id}`)

- Pipeline name and description fields at top
- Ordered list of stages, each showing:
  - Stage name
  - Assigned agent (with avatar + name, or "Manual" if unassigned)
  - Description
  - Drag handle for reordering (updates position values)
  - Edit/delete buttons
- "Add Stage" button at bottom of list
- Save button persists all changes

### IssuePipelineIndicator (used in issue detail view)

- Horizontal step indicator showing all pipeline stages
- Current stage highlighted with a distinct color
- Completed stages shown with checkmark
- Future stages shown as gray circles
- Clicking "Advance" or "Reject" buttons triggers the corresponding API calls
- If issue is not in a pipeline, shows nothing

### PipelineAttachDialog (used from issue detail)

- Dropdown to select an active pipeline from the squad
- Preview of pipeline stages
- "Attach" button calls `PATCH /api/issues/{id}` with `pipelineId`
- "Detach" button to remove pipeline (sets `pipelineId: null`)

---

## 11. Error Handling

| Scenario | HTTP | Code | Handler Action |
|----------|------|------|----------------|
| Pipeline not found | 404 | `NOT_FOUND` | Return error JSON |
| Stage not found | 404 | `NOT_FOUND` | Return error JSON |
| Pipeline name conflict | 409 | `PIPELINE_NAME_CONFLICT` | Check unique constraint violation |
| Position conflict | 409 | `POSITION_CONFLICT` | Check unique constraint violation |
| Issue not in pipeline | 422 | `NOT_IN_PIPELINE` | Check `pipeline_id IS NULL` |
| No previous stage | 422 | `NO_PREVIOUS_STAGE` | GetPreviousStage returns no rows |
| Pipeline-squad mismatch | 422 | `PIPELINE_SQUAD_MISMATCH` | Compare `pipeline.squad_id != issue.squad_id` |
| Pipeline empty (no stages) | 422 | `PIPELINE_EMPTY` | CountStagesByPipeline == 0 |
| Pipeline inactive | 422 | `PIPELINE_INACTIVE` | Check `pipeline.is_active == false` |
| Pipeline in use (delete) | 422 | `PIPELINE_IN_USE` | CountIssuesInPipeline > 0 |
| Stage in use (delete) | 422 | `STAGE_IN_USE` | CountIssuesAtStage > 0 |
| Agent not in squad | 422 | `AGENT_SQUAD_MISMATCH` | Load agent, compare squad_id |
| Concurrent advance race | 409 | `CONFLICT` | AdvanceIssuePipelineStage CAS returns 0 rows |
| Validation error | 400 | `VALIDATION_ERROR` | Domain validation functions |

---

## 12. Concurrency and Atomicity

### Concurrent Advance Protection

Two users or agents could attempt to advance the same issue simultaneously. To prevent double-advancement:

```sql
-- [C-3] This query is in the sqlc queries section (section 3) and also here for reference.
-- name: AdvanceIssuePipelineStage :one
UPDATE issues
SET
    current_stage_id  = @next_stage_id,
    assignee_agent_id = @next_agent_id,
    status            = 'todo'
WHERE id = @id
  AND current_stage_id = @expected_stage_id  -- CAS guard
RETURNING *;
```

If the `WHERE` clause matches 0 rows (because another request already advanced), the handler returns HTTP 409. This follows the same Compare-And-Swap pattern used for task checkout.

### Transaction Boundary

Stage advancement must be atomic:
1. Read current stage and compute next stage
2. Update issue (CAS on `current_stage_id`)
3. Enqueue wakeup request
4. Insert activity log entry (via `logActivity(ctx, qtx, ActivityParams{...})`)

Steps 1-4 run inside a single `sql.Tx`. If any step fails, the entire transaction rolls back.

---

## 13. Testing Strategy

### Unit Tests

| Test | File | Description |
|------|------|-------------|
| Pipeline validation | `internal/domain/pipeline_test.go` | CreatePipeline/UpdatePipeline/CreateStage/UpdateStage validation |
| Stage ordering | `internal/domain/pipeline_test.go` | Position must be >= 1, unique within pipeline |

### Integration Tests

| Test | File | Description |
|------|------|-------------|
| Pipeline CRUD | `internal/server/handlers/pipeline_integration_test.go` | Create, read, update, delete pipelines |
| Stage CRUD | `internal/server/handlers/pipeline_integration_test.go` | Add, update, reorder, delete stages |
| Pipeline attachment | `internal/server/handlers/pipeline_integration_test.go` | Attach issue to pipeline, verify first stage assignment |
| Stage advance | `internal/server/handlers/pipeline_integration_test.go` | Advance through all stages, verify agent assignment |
| Stage reject | `internal/server/handlers/pipeline_integration_test.go` | Reject to previous stage, verify reason comment |
| Auto-advance on done | `internal/server/handlers/pipeline_integration_test.go` | Mark issue done mid-pipeline, verify two-step auto-advance (done then todo) |
| Final stage completion | `internal/server/handlers/pipeline_integration_test.go` | Complete final stage, verify issue status=done |
| Deletion guards | `internal/server/handlers/pipeline_integration_test.go` | Delete pipeline/stage with active issues, expect 422 |
| Squad isolation | `internal/server/handlers/pipeline_integration_test.go` | Cross-squad access returns 403 |
| Concurrent advance | `internal/server/handlers/pipeline_integration_test.go` | Two simultaneous advances, one succeeds, one gets 409 |
| Auto-wake on transition | `internal/server/handlers/pipeline_integration_test.go` | Verify wakeup enqueued with correct context |
| Name uniqueness | `internal/server/handlers/pipeline_integration_test.go` | Duplicate pipeline name in same squad returns 409 |
| Pipeline fields in response | `internal/server/handlers/pipeline_integration_test.go` | Verify issueResponse includes pipelineId and currentStageId |

### Test Scenarios (End-to-End)

```
Scenario: Full pipeline execution
  Given a squad with agents A1, A2, A3
  And a pipeline "Build Pipeline" with stages:
    | Position | Name           | Agent |
    | 1        | Implementation | A1    |
    | 2        | Code Review    | A2    |
    | 3        | QA Testing     | A3    |
  When I create issue "ARI-1" and attach it to "Build Pipeline"
  Then issue status is "todo"
  And assignee is A1
  And current stage is "Implementation"
  And wakeup enqueued for A1

  When A1 marks issue "ARI-1" as done (in_progress→done)
  Then issue status is first persisted as "done"
  Then issue auto-advances to stage 2 "Code Review"
  And status transitions done→todo (valid reopen)
  And assignee is A2
  And status is "todo"
  And wakeup enqueued for A2

  When A2 rejects issue "ARI-1" with reason "Tests failing"
  Then issue moves back to stage 1 "Implementation"
  And assignee is A1
  And status is "todo"
  And a system comment is created with the rejection reason
  And wakeup enqueued for A1

  When A1 marks issue "ARI-1" as done
  Then issue auto-advances to stage 2 "Code Review"

  When A2 marks issue "ARI-1" as done
  Then issue auto-advances to stage 3 "QA Testing"
  And assignee is A3

  When A3 marks issue "ARI-1" as done
  Then issue status is "done"
  And current_stage_id is NULL
  And pipeline.completed event is emitted
```

---

## 14. File Inventory

| File | Action | Description |
|------|--------|-------------|
| `internal/database/migrations/20260316000016_create_pipelines.sql` | CREATE | Migration for pipelines + pipeline_stages tables + issue columns |
| `internal/database/queries/pipelines.sql` | CREATE | sqlc queries for pipeline CRUD |
| `internal/database/queries/pipeline_stages.sql` | CREATE | sqlc queries for stage CRUD + navigation + batch reorder |
| `internal/database/queries/issues.sql` | MODIFY | Add `UpdateIssuePipeline`, `AdvanceIssuePipelineStage` queries, and `pipeline_id` filter |
| `internal/domain/pipeline.go` | CREATE | Pipeline + PipelineStage domain types, DTOs, validation |
| `internal/domain/pipeline_test.go` | CREATE | Validation unit tests |
| `internal/domain/issue.go` | MODIFY | Add PipelineID, CurrentStageID to Issue struct and DTOs |
| `internal/server/handlers/pipeline_service.go` | CREATE | Business logic for attach, advance, reject |
| `internal/server/handlers/pipeline_handler.go` | CREATE | HTTP handlers for pipeline/stage CRUD + advance/reject |
| `internal/server/handlers/pipeline_integration_test.go` | CREATE | Integration tests |
| `internal/server/handlers/issue_handler.go` | MODIFY | Add pipelineSvc (constructor-injected), pipeline auto-advance intercept, issueResponse + dbIssueToResponse pipeline fields |
| `internal/server/router.go` (or equivalent) | MODIFY | Register PipelineHandler routes |
| `web/src/pages/PipelinesPage.tsx` | CREATE | Pipeline list page |
| `web/src/pages/PipelineBuilderPage.tsx` | CREATE | Pipeline editor with stage management |
| `web/src/components/pipelines/PipelineCard.tsx` | CREATE | Pipeline summary card |
| `web/src/components/pipelines/PipelineStageList.tsx` | CREATE | Stage list with ordering |
| `web/src/components/pipelines/PipelineStageForm.tsx` | CREATE | Stage add/edit form |
| `web/src/components/pipelines/IssuePipelineIndicator.tsx` | CREATE | Issue stage progress indicator |
| `web/src/components/pipelines/PipelineAttachDialog.tsx` | CREATE | Dialog to attach issue to pipeline |

---

## 15. Dependencies

- **Issue Tracking (05):** `issues` table, `IssueHandler`, `domain.Issue` -- modified with new columns
- **Agent Runtime (11):** `WakeupService.Enqueue()` for auto-wake on stage transitions
- **SSE (11):** `sse.Hub` for broadcasting pipeline events
- **Activity Log (09):** `logActivity(ctx, qtx, ActivityParams{...})` for audit trail entries (called directly, no wrapper type)
- **Conversations (13):** Pipeline stages work with both `task` and `conversation` issue types

---

## 16. Migration Path

1. **Task 14.1:** Run migration to create `pipelines`, `pipeline_stages` tables and add `pipeline_id`/`current_stage_id` to `issues`. Regenerate sqlc.
2. **Task 14.2:** Implement pipeline CRUD handler + service (no stage logic yet).
3. **Task 14.3:** Implement stage CRUD handler + service.
4. **Task 14.4:** Implement issue-pipeline attachment via `PATCH /api/issues/{id}`.
5. **Task 14.5:** Implement `POST /api/issues/{id}/advance` with CAS guard.
6. **Task 14.6:** Wire auto-wake into stage transitions.
7. **Task 14.7:** Implement `POST /api/issues/{id}/reject` with reason comment.
8. **Task 14.8:** Build React UI — pipeline CRUD pages.
9. **Task 14.9:** Build React UI — pipeline builder + stage indicator.
10. **Task 14.10:** Integration tests for all scenarios.
