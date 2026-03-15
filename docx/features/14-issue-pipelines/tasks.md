# Tasks: Issue Pipelines (Multi-Agent Workflows)

**Feature:** 14-issue-pipelines
**Created:** 2026-03-15
**Status:** In Progress

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-PIP-001 through REQ-PIP-081, REQ-PIP-090, REQ-PIP-091, REQ-PIP-NF-001 through REQ-PIP-NF-004

## Implementation Approach

Work bottom-up through the dependency graph: domain model and validation first, then database migration and sqlc queries, then PipelineService with side-effects (SSE, wakeup, activity log), then the HTTP handler for pipeline and stage CRUD, then issue-pipeline attachment and stage advancement/rejection, then IssueHandler integration for auto-advance, and finally React UI components. Each task follows the Red-Green-Refactor TDD cycle.

## Progress Summary

- Total Tasks: 13
- Completed: 2/13
- In Progress: Task 03 — Pipeline sqlc, Task 04 — Stage sqlc

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 01 — Domain Model: Pipeline Types and Validation

**Requirements:** REQ-PIP-001, REQ-PIP-002, REQ-PIP-004, REQ-PIP-010, REQ-PIP-011, REQ-PIP-013, REQ-PIP-015
**Estimated time:** 30 min

#### Context

Define the pipeline and pipeline stage domain types, request/response DTOs, and validation functions. This is the foundation for all pipeline logic — field constraints and input sanitization. The `PipelineStage` struct includes a `GateID` field (nullable, reserved for v2 approval gates per XC-1).

#### RED — Write Failing Tests

Write `internal/domain/pipeline_test.go`:

1. `TestValidateCreatePipelineInput` — verify name required, name max 200 chars, empty name rejected, valid input accepted.
2. `TestValidateUpdatePipelineInput` — verify empty name rejected when provided, name max 200 chars when provided, nil name accepted (no-op).
3. `TestValidateCreateStageInput` — verify name required, name max 200 chars, position >= 1, position 0 rejected, negative position rejected, valid input accepted.
4. `TestValidateUpdateStageInput` — verify empty name rejected when provided, name max 200 chars when provided, position >= 1 when provided, nil fields accepted.

#### GREEN — Implement

Create `internal/domain/pipeline.go`:

- `Pipeline` struct with all fields (ID, SquadID, Name, Description, IsActive, CreatedAt, UpdatedAt)
- `PipelineStage` struct with all fields (ID, PipelineID, Name, Description, Position, AssignedAgentID, GateID, CreatedAt, UpdatedAt)
- `PipelineWithStages` struct embedding Pipeline with Stages slice
- `CreatePipelineRequest`, `UpdatePipelineRequest`, `CreatePipelineStageRequest`, `UpdatePipelineStageRequest` DTOs
- `AdvanceIssueRequest`, `RejectIssueRequest` DTOs
- `ValidateCreatePipelineInput()`, `ValidateUpdatePipelineInput()`, `ValidateCreateStageInput()`, `ValidateUpdateStageInput()` functions

#### REFACTOR

Ensure JSON tags use camelCase, pointer types for optional fields, and `omitempty` where appropriate.

#### Files

- Create: `internal/domain/pipeline.go`
- Create: `internal/domain/pipeline_test.go`

---

### [x] Task 02 — Database Migration: Pipelines and Pipeline Stages Tables

**Requirements:** REQ-PIP-001, REQ-PIP-002, REQ-PIP-003, REQ-PIP-005, REQ-PIP-010, REQ-PIP-011, REQ-PIP-012, REQ-PIP-015, REQ-PIP-NF-003
**Estimated time:** 30 min

#### Context

Create the `pipelines` and `pipeline_stages` tables with all columns, constraints, indexes, and triggers. Add `pipeline_id` and `current_stage_id` columns to the `issues` table. Follow the pattern from existing migrations.

Key schema notes:
- [M-2] No `idx_pipelines_squad_id` — the `uq_pipelines_squad_name` unique constraint already covers `squad_id` lookups.
- [DB] `chk_issues_pipeline_stage_consistency` CHECK constraint ensures `current_stage_id` is only set when `pipeline_id` is set.
- [XC-1] `gate_id` nullable column on `pipeline_stages` for v2 approval gates.
- [H-4] Down migration drops indexes before columns before tables.

#### RED — Write Failing Tests

Add assertions to migration smoke tests:

1. After `RunMigrations()`, the table `pipelines` exists with expected columns and constraints.
2. The table `pipeline_stages` exists with expected columns, unique constraint on `(pipeline_id, position)`, and `gate_id` column.
3. The `issues` table has `pipeline_id` and `current_stage_id` columns with foreign keys.
4. Indexes `idx_pipelines_squad_active`, `idx_pipeline_stages_pipeline_id`, `idx_issues_pipeline_id`, `idx_issues_current_stage_id` exist.
5. Unique constraint `uq_pipelines_squad_name` on `(squad_id, name)` exists.
6. CHECK constraint `chk_issues_pipeline_stage_consistency` exists.

#### GREEN — Implement

Create `internal/database/migrations/20260316000016_create_pipelines.sql` with the schema from design.md section 2.

#### Files

- Create: `internal/database/migrations/20260316000016_create_pipelines.sql`
- Modify: `internal/database/database_test.go` (add migration assertions)

---

### [ ] Task 03 — SQL Queries and sqlc Generation: Pipeline CRUD

**Requirements:** REQ-PIP-020, REQ-PIP-021, REQ-PIP-022, REQ-PIP-023, REQ-PIP-024, REQ-PIP-090 (deletion guard)
**Estimated time:** 45 min

#### Context

Write sqlc query definitions for pipeline CRUD operations: create, list with filters, get by ID, update, delete, and count issues in pipeline (for deletion guard per REQ-PIP-090). Run `make sqlc` to generate Go code.

#### RED — Write Failing Tests

Write `internal/database/db/pipelines_test.go`:

1. `TestCreatePipeline` — insert a pipeline, verify all fields returned including generated UUID and timestamps.
2. `TestListPipelinesBySquad` — insert multiple pipelines, verify filtering by `is_active`, pagination with limit/offset, and ordering by name.
3. `TestGetPipelineByID` — insert and retrieve, verify all fields match.
4. `TestUpdatePipeline` — update name, description, is_active fields, verify COALESCE behavior (unchanged fields preserved).
5. `TestDeletePipeline` — insert and delete, verify gone. Verify cascade deletes stages.
6. `TestCountPipelinesBySquad` — insert pipelines with mixed active status, verify count with and without filter.
7. `TestCountIssuesInPipeline` — verify count returns 0 for empty pipeline.

#### GREEN — Implement

Create `internal/database/queries/pipelines.sql` with queries from design.md section 3 (CreatePipeline, GetPipelineByID, ListPipelinesBySquad, CountPipelinesBySquad, UpdatePipeline, DeletePipeline, CountIssuesInPipeline). Run `make sqlc`.

#### Files

- Create: `internal/database/queries/pipelines.sql`
- Regenerate: `internal/database/db/` (via `make sqlc`)
- Create: `internal/database/db/pipelines_test.go`

---

### [ ] Task 04 — SQL Queries and sqlc Generation: Stage CRUD and Navigation

**Requirements:** REQ-PIP-025, REQ-PIP-026, REQ-PIP-027, REQ-PIP-091 (deletion guard), REQ-PIP-016, REQ-PIP-012
**Estimated time:** 45 min

#### Context

Write sqlc query definitions for stage CRUD and navigation queries (first stage, next stage, previous stage) used by the advancement and rejection logic. Also add the `UpdateIssuePipeline`, `AdvanceIssuePipelineStage` (CAS query per C-3), and `ReorderPipelineStages` (batch query per M-1) queries. Add `pipeline_id` filter to `ListIssuesBySquad` and `CountIssuesBySquad` (H-5).

#### RED — Write Failing Tests

Write `internal/database/db/pipeline_stages_test.go`:

1. `TestCreatePipelineStage` — insert a stage, verify all fields including position and assigned_agent_id.
2. `TestListStagesByPipeline` — insert stages with different positions, verify ordered by position ASC.
3. `TestUpdatePipelineStage` — update name, position, assigned_agent_id, verify COALESCE behavior.
4. `TestDeletePipelineStage` — insert and delete, verify gone.
5. `TestCountIssuesAtStage` — verify count returns 0 for stage with no issues.
6. `TestGetFirstStage` — insert stages at positions 1, 2, 3, verify first stage returned is position 1.
7. `TestGetNextStage` — from position 1, verify next is position 2. From position 2, verify next is position 3. From position 3, verify no rows.
8. `TestGetPreviousStage` — from position 3, verify previous is position 2. From position 1, verify no rows.
9. `TestCountStagesByPipeline` — verify correct count.
10. `TestUpdateIssuePipeline` — update issue's pipeline_id, current_stage_id, assignee, status.
11. `TestAdvanceIssuePipelineStage` — verify CAS guard on current_stage_id; mismatched expected_stage_id returns no rows.
12. `TestReorderPipelineStages` — verify batch position update works correctly.
13. `TestListIssuesBySquad_PipelineFilter` — verify pipeline_id filter on issue list query.

#### GREEN — Implement

Create `internal/database/queries/pipeline_stages.sql` with queries from design.md (CreatePipelineStage, GetPipelineStageByID, ListStagesByPipeline, UpdatePipelineStage, DeletePipelineStage, CountIssuesAtStage, GetFirstStage, GetNextStage, GetPreviousStage, CountStagesByPipeline, ReorderPipelineStages).

Modify `internal/database/queries/issues.sql` to add UpdateIssuePipeline and AdvanceIssuePipelineStage queries from design.md sections 3 and 12. Also add `filter_pipeline_id` optional parameter to ListIssuesBySquad and CountIssuesBySquad (H-5). Run `make sqlc`.

#### Files

- Create: `internal/database/queries/pipeline_stages.sql`
- Modify: `internal/database/queries/issues.sql`
- Regenerate: `internal/database/db/` (via `make sqlc`)
- Create: `internal/database/db/pipeline_stages_test.go`

---

### [ ] Task 05 — PipelineService: Pipeline and Stage CRUD Logic

**Requirements:** REQ-PIP-003, REQ-PIP-005, REQ-PIP-014, REQ-PIP-028, REQ-PIP-029, REQ-PIP-090 (deletion guard), REQ-PIP-091 (deletion guard), REQ-PIP-070, REQ-PIP-080
**Estimated time:** 60 min

#### Context

The `PipelineService` encapsulates pipeline and stage business logic: creation with squad-scoped name uniqueness, agent-squad validation for stage assignment, deletion guards (in-use checks), activity logging, and SSE event broadcasting. Uses `logActivity(ctx, qtx, ActivityParams{...})` directly (H-1), matching the InboxService pattern.

#### RED — Write Failing Tests

Write `internal/server/handlers/pipeline_service_test.go`:

1. `TestPipelineService_CreatePipeline` — verify pipeline is persisted, activity log `pipeline.created` entry created, SSE `pipeline.created` event published.
2. `TestPipelineService_CreatePipeline_DuplicateName` — verify 409 error with `PIPELINE_NAME_CONFLICT` code.
3. `TestPipelineService_UpdatePipeline` — verify fields updated, activity log `pipeline.updated` entry created, SSE `pipeline.updated` event published.
4. `TestPipelineService_DeletePipeline_Success` — verify pipeline deleted when no issues attached, activity log `pipeline.deleted`, SSE `pipeline.deleted`.
5. `TestPipelineService_DeletePipeline_InUse` — verify 422 error with `PIPELINE_IN_USE` when issues are attached.
6. `TestPipelineService_CreateStage` — verify stage persisted with correct position.
7. `TestPipelineService_CreateStage_DuplicatePosition` — verify 409 error with `POSITION_CONFLICT`.
8. `TestPipelineService_CreateStage_AgentSquadMismatch` — verify 422 error when assigned agent belongs to different squad.
9. `TestPipelineService_DeleteStage_Success` — verify stage deleted when no issues at that stage.
10. `TestPipelineService_DeleteStage_InUse` — verify 422 error with `STAGE_IN_USE` when issues are at that stage.

#### GREEN — Implement

Create `internal/server/handlers/pipeline_service.go`:

- `PipelineService` struct with `queries`, `dbConn`, `wakeupSvc`, `sseHub` fields (no `activityFn` — uses `logActivity` directly per H-1)
- `NewPipelineService(q, dbConn, wakeupSvc, sseHub)` constructor
- `CreatePipeline(ctx, squadID, req)` — validate, insert, activity log via `logActivity(ctx, qtx, ActivityParams{...})`, SSE
- `UpdatePipeline(ctx, pipelineID, req)` — validate, update, activity log, SSE
- `DeletePipeline(ctx, pipelineID)` — count issues check (REQ-PIP-090), delete, activity log, SSE
- `CreateStage(ctx, pipelineID, req)` — validate agent squad match, insert
- `UpdateStage(ctx, stageID, req)` — validate agent squad match, update
- `DeleteStage(ctx, stageID)` — count issues check (REQ-PIP-091), delete

#### Files

- Create: `internal/server/handlers/pipeline_service.go`
- Create: `internal/server/handlers/pipeline_service_test.go`

---

### [ ] Task 06 — PipelineService: Attach, Advance, and Reject Logic

**Requirements:** REQ-PIP-030, REQ-PIP-031, REQ-PIP-032, REQ-PIP-033, REQ-PIP-034, REQ-PIP-035, REQ-PIP-036, REQ-PIP-040, REQ-PIP-041, REQ-PIP-042, REQ-PIP-043, REQ-PIP-044, REQ-PIP-045, REQ-PIP-050, REQ-PIP-051, REQ-PIP-052, REQ-PIP-053, REQ-PIP-054, REQ-PIP-060, REQ-PIP-061, REQ-PIP-062, REQ-PIP-071, REQ-PIP-072, REQ-PIP-073, REQ-PIP-074, REQ-PIP-081, REQ-PIP-NF-002, REQ-PIP-NF-004
**Estimated time:** 60 min

#### Context

Implement the core workflow logic: attaching issues to pipelines, advancing through stages, rejecting back to previous stages, and auto-advancing when an agent marks an issue done. This is the most complex task in the feature — it orchestrates issue updates, agent wakeups, activity logging, and SSE events within database transactions.

**[C-2] Two-step status transition:** Auto-advance first persists `done` status (agent marking complete), then sets `todo` for the next stage assignment. The `done` -> `todo` transition is a valid reopen per `issueValidTransitions` in `internal/domain/issue.go`.

**[XC-1] Approval gate hook:** In `AdvanceStage`, if the target stage has a non-null `gate_id`, the system should (in v2) create an approval inbox item instead of auto-advancing. For now, document this hook point but proceed with normal advancement.

#### RED — Write Failing Tests

Extend `internal/server/handlers/pipeline_service_test.go`:

1. `TestPipelineService_AttachIssueToPipeline` — verify issue's pipeline_id, current_stage_id set to first stage, assignee set to first stage's agent, status changed from `backlog` to `todo`, wakeup enqueued with `invocation_source=assignment`, activity log `issue.pipeline.attached`, SSE `issue.pipeline.stage_changed` with `transition=attach`.
2. `TestPipelineService_AttachIssue_PipelineSquadMismatch` — verify 422 with `PIPELINE_SQUAD_MISMATCH`.
3. `TestPipelineService_AttachIssue_PipelineEmpty` — verify 422 with `PIPELINE_EMPTY` when pipeline has no stages.
4. `TestPipelineService_AttachIssue_PipelineInactive` — verify 422 with `PIPELINE_INACTIVE`.
5. `TestPipelineService_DetachIssue` — verify pipeline_id and current_stage_id cleared, assignee and status unchanged, activity log `issue.pipeline.detached`.
6. `TestPipelineService_AdvanceStage` — verify current_stage_id updated to next stage, assignee updated, status set to `todo` (done->todo reopen), wakeup enqueued, activity log `issue.pipeline.advanced` with `from_stage` and `to_stage`, SSE `issue.pipeline.stage_changed` with `transition=advance`.
7. `TestPipelineService_AdvanceStage_FinalStage` — verify status set to `done`, current_stage_id cleared, activity log `issue.pipeline.completed`, SSE event with `transition=complete`.
8. `TestPipelineService_AdvanceStage_NotInPipeline` — verify 422 with `NOT_IN_PIPELINE`.
9. `TestPipelineService_AdvanceStage_ManualStage` — verify no wakeup enqueued when target stage has null assigned_agent_id.
10. `TestPipelineService_AdvanceStage_ConcurrentRace` — verify CAS guard: two simultaneous advances, one succeeds, other gets 409.
11. `TestPipelineService_RejectStage` — verify current_stage_id updated to previous stage, assignee updated, status set to `todo`, wakeup enqueued, activity log `issue.pipeline.rejected` with `from_stage`, `to_stage`, `reason`.
12. `TestPipelineService_RejectStage_WithReason` — verify system comment created with rejection reason.
13. `TestPipelineService_RejectStage_FirstStage` — verify 422 with `NO_PREVIOUS_STAGE`.
14. `TestPipelineService_RejectStage_NotInPipeline` — verify 422 with `NOT_IN_PIPELINE`.
15. `TestPipelineService_AutoAdvanceOnDone` — verify when issue status changes to `done` and has pipeline, auto-advance triggers (done persisted first, then todo at next stage). Returns `handled=true`.
16. `TestPipelineService_AutoAdvanceOnDone_NoPipeline` — verify returns `handled=false` when issue has no pipeline.

#### GREEN — Implement

Add to `internal/server/handlers/pipeline_service.go`:

- `AttachIssueToPipeline(ctx, issueID, pipelineID)` — validate squad match, active, has stages, get first stage, update issue (within transaction), wakeup, activity via `logActivity()`, SSE
- `DetachIssueFromPipeline(ctx, issueID)` — clear pipeline_id and current_stage_id, activity, SSE
- `AdvanceStage(ctx, issueID)` — load issue, get current stage, get next stage, CAS update via `AdvanceIssuePipelineStage`, wakeup if agent assigned, activity, SSE. Handle final stage completion. (v2 hook: check gate_id on target stage)
- `RejectStage(ctx, issueID, reason)` — load issue, get current stage, get previous stage, update, create comment if reason, wakeup, activity, SSE
- `AutoAdvanceOnDone(ctx, issueID)` — check pipeline, call AdvanceStage (done already persisted by caller)

All stage transitions run inside `sql.Tx` for atomicity.

#### Files

- Modify: `internal/server/handlers/pipeline_service.go`
- Modify: `internal/server/handlers/pipeline_service_test.go`

---

### [ ] Task 07 — PipelineHandler: Pipeline and Stage CRUD Endpoints

**Requirements:** REQ-PIP-020, REQ-PIP-021, REQ-PIP-022, REQ-PIP-023, REQ-PIP-024, REQ-PIP-025, REQ-PIP-026, REQ-PIP-027, REQ-PIP-028, REQ-PIP-029
**Estimated time:** 60 min

#### Context

The HTTP handler exposes the pipeline and stage REST API. It handles auth (JWT), input validation, pagination, squad-scoped isolation, and delegates to `PipelineService` for business logic. Follow the pattern from existing handlers like `issue_handler.go`.

#### RED — Write Failing Tests

Write `internal/server/handlers/pipeline_handler_test.go`:

1. `TestCreatePipeline` — POST with valid body, verify 201 and response shape.
2. `TestCreatePipeline_ValidationError` — missing name, verify 400.
3. `TestCreatePipeline_DuplicateName` — verify 409 with `PIPELINE_NAME_CONFLICT`.
4. `TestListPipelines` — GET with pagination and is_active filter, verify response shape with `data` and `total`.
5. `TestListPipelines_SquadIsolation` — verify 403 for non-squad-member.
6. `TestGetPipeline` — GET by ID, verify response includes stages ordered by position.
7. `TestGetPipeline_NotFound` — verify 404.
8. `TestUpdatePipeline` — PATCH with partial update, verify 200.
9. `TestDeletePipeline` — DELETE, verify 204.
10. `TestDeletePipeline_InUse` — verify 422 with `PIPELINE_IN_USE`.
11. `TestCreateStage` — POST stage to pipeline, verify 201.
12. `TestCreateStage_DuplicatePosition` — verify 409 with `POSITION_CONFLICT`.
13. `TestCreateStage_AgentSquadMismatch` — verify 422 with `AGENT_SQUAD_MISMATCH`.
14. `TestUpdateStage` — PATCH stage, verify 200.
15. `TestDeleteStage` — DELETE stage, verify 204.
16. `TestDeleteStage_InUse` — verify 422 with `STAGE_IN_USE`.
17. `TestAllEndpoints_RequireAuth` — verify 401 for unauthenticated requests.

#### GREEN — Implement

Create `internal/server/handlers/pipeline_handler.go`:

- `PipelineHandler` struct with `queries`, `dbConn`, `pipelineSvc`
- `NewPipelineHandler(...)` constructor
- `RegisterRoutes(mux)` — register all 8 CRUD routes
- `CreatePipeline(w, r)` — parse squadId from URL, parse body, validate, delegate to service
- `ListPipelines(w, r)` — parse query params (is_active, limit, offset), squad membership check
- `GetPipeline(w, r)` — parse ID, load pipeline + stages, squad membership check
- `UpdatePipeline(w, r)` — parse body with explicit null handling for description
- `DeletePipeline(w, r)` — delegate to service
- `CreateStage(w, r)` — parse pipelineId from URL, parse body, validate, delegate
- `UpdateStage(w, r)` — parse stageId from URL, parse body with explicit null handling
- `DeleteStage(w, r)` — delegate to service
- Response type structs and DB-to-response mappers

#### Files

- Create: `internal/server/handlers/pipeline_handler.go`
- Create: `internal/server/handlers/pipeline_handler_test.go`

---

### [ ] Task 08 — PipelineHandler: Advance and Reject Endpoints

**Requirements:** REQ-PIP-040, REQ-PIP-041, REQ-PIP-042, REQ-PIP-043, REQ-PIP-044, REQ-PIP-045, REQ-PIP-050, REQ-PIP-051, REQ-PIP-052, REQ-PIP-053, REQ-PIP-054, REQ-PIP-028
**Estimated time:** 45 min

#### Context

Add the advance and reject HTTP endpoints to `PipelineHandler`. These endpoints call into the `PipelineService` methods implemented in Task 06. Also wire pipeline attachment into the existing `IssueHandler.UpdateIssue` for the `pipelineId` field.

#### RED — Write Failing Tests

Extend `internal/server/handlers/pipeline_handler_test.go`:

1. `TestAdvanceIssue` — POST advance, verify 200 and issue moved to next stage.
2. `TestAdvanceIssue_FinalStage` — verify pipeline completion, status=done.
3. `TestAdvanceIssue_NotInPipeline` — verify 422 with `NOT_IN_PIPELINE`.
4. `TestAdvanceIssue_ConcurrentRace` — two simultaneous requests, one gets 409.
5. `TestRejectIssue` — POST reject, verify 200 and issue moved to previous stage.
6. `TestRejectIssue_WithReason` — verify reason recorded as system comment.
7. `TestRejectIssue_FirstStage` — verify 422 with `NO_PREVIOUS_STAGE`.
8. `TestRejectIssue_NotInPipeline` — verify 422 with `NOT_IN_PIPELINE`.
9. `TestAdvanceReject_RequireAuth` — verify 401 for unauthenticated requests.

#### GREEN — Implement

Add to `internal/server/handlers/pipeline_handler.go`:

- Register `POST /api/issues/{id}/advance` and `POST /api/issues/{id}/reject` routes
- `AdvanceIssue(w, r)` — parse issue ID, delegate to `pipelineSvc.AdvanceStage()`
- `RejectIssue(w, r)` — parse body for optional reason, delegate to `pipelineSvc.RejectStage()`

#### Files

- Modify: `internal/server/handlers/pipeline_handler.go`
- Modify: `internal/server/handlers/pipeline_handler_test.go`

---

### [ ] Task 09 — IssueHandler Integration: Auto-Advance and Pipeline Attachment

**Requirements:** REQ-PIP-030, REQ-PIP-031, REQ-PIP-032, REQ-PIP-033, REQ-PIP-034, REQ-PIP-035, REQ-PIP-036, REQ-PIP-042
**Estimated time:** 45 min

#### Context

Modify the existing `IssueHandler.UpdateIssue` method to: (1) handle `pipelineId` field in PATCH requests for attaching/detaching pipelines, and (2) intercept `status=done` transitions for pipeline-attached issues to trigger auto-advance instead of directly completing the issue. Also update the Issue domain model to include pipeline fields.

**[H-2]** Update `issueResponse` struct and `dbIssueToResponse` function to include `pipelineId` and `currentStageId` fields. This also satisfies **[XH-1]** (assignment responses include pipeline fields).

**[H-3]** Change `IssueHandler` to accept `PipelineService` via constructor injection (not setter).

#### Acceptance Criteria

- `issueResponse` includes `pipelineId` and `currentStageId` fields
- `dbIssueToResponse` maps the new nullable DB columns to response fields
- `NewIssueHandler` accepts `*PipelineService` as a parameter
- PATCH with `pipelineId` delegates to `PipelineService.AttachIssueToPipeline`
- PATCH with `pipelineId: null` delegates to `PipelineService.DetachIssueFromPipeline`
- PATCH with `status=done` on pipeline-attached issue triggers `AutoAdvanceOnDone`
- [C-2] The `done` status is persisted first, then `AutoAdvanceOnDone` sets `todo` for the next stage

#### RED — Write Failing Tests

Extend `internal/server/handlers/issue_handler_test.go`:

1. `TestUpdateIssue_AttachPipeline` — PATCH with `pipelineId`, verify issue attached to pipeline at first stage with correct assignee.
2. `TestUpdateIssue_AttachPipeline_SquadMismatch` — verify 422 with `PIPELINE_SQUAD_MISMATCH`.
3. `TestUpdateIssue_AttachPipeline_Empty` — verify 422 with `PIPELINE_EMPTY`.
4. `TestUpdateIssue_AttachPipeline_Inactive` — verify 422 with `PIPELINE_INACTIVE`.
5. `TestUpdateIssue_DetachPipeline` — PATCH with `pipelineId: null`, verify pipeline_id and current_stage_id cleared.
6. `TestUpdateIssue_StatusDone_AutoAdvance` — PATCH status=done on pipeline-attached issue, verify done persisted first then auto-advance to next stage with status=todo.
7. `TestUpdateIssue_StatusDone_FinalStage` — PATCH status=done on final stage, verify pipeline completes and status becomes done.
8. `TestUpdateIssue_StatusDone_NoPipeline` — PATCH status=done on non-pipeline issue, verify normal behavior (status=done).
9. `TestIssueResponse_PipelineFields` — verify issue response includes `pipelineId` and `currentStageId` when set.

#### GREEN — Implement

Modify `internal/domain/issue.go`:

- Add `PipelineID *uuid.UUID` and `CurrentStageID *uuid.UUID` to `Issue` struct
- Add `PipelineID *uuid.UUID` and `SetPipeline bool` to `UpdateIssueRequest`

Modify `internal/server/handlers/issue_handler.go`:

- [H-3] Add `pipelineSvc *PipelineService` field to `IssueHandler`
- [H-3] Update `NewIssueHandler` to accept `*PipelineService` parameter (constructor injection)
- [H-2] Add `PipelineID *uuid.UUID` and `CurrentStageID *uuid.UUID` to `issueResponse`
- [H-2] Update `dbIssueToResponse` to map `PipelineID` and `CurrentStageID`
- In `UpdateIssue`, handle `pipelineId` field: if set, call `pipelineSvc.AttachIssueToPipeline()`; if null, call `pipelineSvc.DetachIssueFromPipeline()`
- In `UpdateIssue`, intercept `status=done`: if issue has `pipeline_id`, persist done first then call `pipelineSvc.AutoAdvanceOnDone()` (C-2 two-step)

#### Files

- Modify: `internal/domain/issue.go`
- Modify: `internal/server/handlers/issue_handler.go`
- Modify: `internal/server/handlers/issue_handler_test.go`

---

### [ ] Task 10 — Server Wiring: Register Pipeline Routes and Dependencies

**Requirements:** All (integration)
**Estimated time:** 30 min

#### Context

Wire `PipelineService` and `PipelineHandler` into server initialization, connecting all dependencies (queries, DB, SSE hub, wakeup service). Register pipeline routes on the HTTP mux. Pass `PipelineService` to `IssueHandler` via constructor injection (H-3).

Note: `PipelineService` constructor no longer takes an `ActivityLogFunc` — it calls `logActivity()` directly (H-1).

#### RED — Write Failing Tests

Write an integration test that:

1. Starts the full server with embedded DB.
2. Creates a squad and user.
3. `POST /api/squads/{id}/pipelines` — verify 201 and pipeline is persisted.
4. `POST /api/pipelines/{id}/stages` — verify 201 and stage is persisted.
5. `GET /api/pipelines/{id}` — verify pipeline returned with stages.
6. `DELETE /api/pipelines/{id}` — verify 204.

#### GREEN — Implement

Modify server initialization (likely `cmd/ari/run.go` or `internal/server/server.go`):

- Create `PipelineService` with dependencies: `NewPipelineService(queries, dbConn, wakeupSvc, sseHub)`
- Create `PipelineHandler` and call `RegisterRoutes(mux)`
- Pass `PipelineService` to `NewIssueHandler(queries, dbConn, pipelineSvc)` — constructor injection (H-3)

#### Files

- Modify: `cmd/ari/run.go` or `internal/server/server.go` (server initialization)
- Modify: constructor call for `IssueHandler` (add pipelineSvc parameter)

---

### [ ] Task 11a — React UI: Pipeline CRUD Pages

**Requirements:** REQ-PIP-020, REQ-PIP-021, REQ-PIP-022, REQ-PIP-080
**Estimated time:** 60 min

#### Context

[M-4] Split from original Task 11. Build the pipeline list page and individual pipeline CRUD pages with API integration and SSE subscriptions.

#### RED — Write Failing Tests

(Frontend testing — verify component rendering and API integration)

1. `PipelinesPage` renders pipeline cards from API and create button.
2. `PipelinesPage` filter toggle works (active/inactive/all).
3. Pipeline create/edit forms validate input.

#### GREEN — Implement

Create React components:

- `web/src/pages/PipelinesPage.tsx` — list view with create button, filter toggle (active/inactive/all)
- `web/src/components/pipelines/PipelineCard.tsx` — pipeline summary card
- `web/src/components/pipelines/AgentSelector.tsx` — dropdown to pick agent for a stage
- `web/src/hooks/usePipelines.ts` — API client + SSE subscription hook

Add routes to the React router and navigation sidebar.

#### Files

- Create: `web/src/pages/PipelinesPage.tsx`
- Create: `web/src/components/pipelines/PipelineCard.tsx`
- Create: `web/src/components/pipelines/AgentSelector.tsx`
- Create: `web/src/hooks/usePipelines.ts`
- Modify: `web/src/App.tsx` (add routes)
- Modify: sidebar/nav component (add pipelines link)

---

### [ ] Task 11b — React UI: Pipeline Builder and Stage Indicator

**Requirements:** REQ-PIP-025, REQ-PIP-026, REQ-PIP-027, REQ-PIP-081
**Estimated time:** 60 min

#### Context

[M-4] Split from original Task 11. Build the pipeline builder/editor with stage management (drag-and-drop reordering), issue stage indicator (progress bar), and pipeline attach dialog. Subscribe to SSE events for real-time updates.

#### RED — Write Failing Tests

(Frontend testing — verify component rendering and API integration)

1. `PipelineBuilderPage` renders pipeline form with editable stage list.
2. `IssuePipelineIndicator` renders stage progress bar with current stage highlighted.
3. `PipelineAttachDialog` renders pipeline dropdown and attach/detach buttons.

#### GREEN — Implement

Create React components:

- `web/src/pages/PipelineBuilderPage.tsx` — create/edit pipeline with stage management
- `web/src/components/pipelines/PipelineStageList.tsx` — ordered list of stages with drag handles
- `web/src/components/pipelines/PipelineStageForm.tsx` — add/edit stage form
- `web/src/components/pipelines/IssuePipelineIndicator.tsx` — horizontal step indicator on issue detail
- `web/src/components/pipelines/PipelineAttachDialog.tsx` — dialog to attach/detach issue from pipeline

#### Files

- Create: `web/src/pages/PipelineBuilderPage.tsx`
- Create: `web/src/components/pipelines/PipelineStageList.tsx`
- Create: `web/src/components/pipelines/PipelineStageForm.tsx`
- Create: `web/src/components/pipelines/IssuePipelineIndicator.tsx`
- Create: `web/src/components/pipelines/PipelineAttachDialog.tsx`

---

### [ ] Task 12 — Integration Tests: Full Pipeline Flow

**Requirements:** All requirements (end-to-end coverage)
**Estimated time:** 60 min

#### Context

Write comprehensive integration tests covering the full pipeline lifecycle: creation, stage management, issue attachment, multi-stage advancement with auto-wake, rejection with reason comments, auto-advance on done (two-step: done then todo), final-stage completion, deletion guards, squad isolation, and concurrent advance protection.

#### RED — Write Failing Tests

Create `internal/server/handlers/pipeline_integration_test.go`:

1. `TestFullPipelineExecution` — end-to-end scenario from design.md section 13:
   - Create squad with agents A1, A2, A3
   - Create pipeline with 3 stages (Implementation, Code Review, QA Testing)
   - Create issue, attach to pipeline, verify first stage assignment and wakeup
   - Mark issue done (agent A1), verify done persisted first then auto-advance to stage 2 with status=todo and wakeup for A2
   - Reject issue (agent A2) with reason, verify return to stage 1 and reason comment
   - Mark issue done (agent A1), verify auto-advance to stage 2
   - Mark issue done (agent A2), verify auto-advance to stage 3
   - Mark issue done (agent A3), verify pipeline completion (status=done, current_stage_id=null)
2. `TestPipelineDeletionGuards` — verify cannot delete pipeline with attached issues (REQ-PIP-090), cannot delete stage with issues at it (REQ-PIP-091).
3. `TestSquadIsolation` — verify cross-squad pipeline access returns 403, cross-squad attachment returns 422.
4. `TestConcurrentAdvance` — two goroutines advancing same issue, verify one succeeds and one gets 409.
5. `TestAutoWakeOnTransition` — verify wakeup enqueued with correct `invocation_source=assignment` and context JSON containing `issue_id`, `pipeline_id`, `stage_name`, `transition`.
6. `TestManualStageNoWakeup` — verify no wakeup enqueued when stage has null assigned_agent_id.
7. `TestPipelineNameUniqueness` — verify duplicate name in same squad returns 409, same name in different squad succeeds.
8. `TestStagePositionUniqueness` — verify duplicate position in same pipeline returns 409.
9. `TestActivityLogEntries` — verify all pipeline lifecycle events produce correct activity log entries.
10. `TestSSEEvents` — verify pipeline and stage-change SSE events are broadcast.
11. `TestIssueResponsePipelineFields` — verify issue response DTOs include pipelineId and currentStageId (H-2, XH-1).

#### GREEN — Implement

Run all tests and verify they pass against the implementations from Tasks 01-10.

#### REFACTOR

Review test coverage, add edge cases if needed, ensure all requirements are exercised.

#### Files

- Create: `internal/server/handlers/pipeline_integration_test.go`

---

## Requirement Coverage Matrix

| Requirement | Task(s) |
|-------------|---------|
| REQ-PIP-001 | Task 01, Task 02 |
| REQ-PIP-002 | Task 01, Task 02 |
| REQ-PIP-003 | Task 02, Task 05 |
| REQ-PIP-004 | Task 01 |
| REQ-PIP-005 | Task 02, Task 05 |
| REQ-PIP-010 | Task 01, Task 02 |
| REQ-PIP-011 | Task 01, Task 02 |
| REQ-PIP-012 | Task 02, Task 04 |
| REQ-PIP-013 | Task 01 |
| REQ-PIP-014 | Task 05 |
| REQ-PIP-015 | Task 01, Task 02 |
| REQ-PIP-016 | Task 04, Task 06 |
| REQ-PIP-020 | Task 03, Task 07 |
| REQ-PIP-021 | Task 03, Task 07 |
| REQ-PIP-022 | Task 03, Task 07 |
| REQ-PIP-023 | Task 03, Task 07 |
| REQ-PIP-024 | Task 03, Task 07 |
| REQ-PIP-025 | Task 04, Task 07 |
| REQ-PIP-026 | Task 04, Task 07 |
| REQ-PIP-027 | Task 04, Task 07 |
| REQ-PIP-028 | Task 07, Task 08 |
| REQ-PIP-029 | Task 05, Task 07 |
| REQ-PIP-030 | Task 06, Task 09 |
| REQ-PIP-031 | Task 06, Task 09 |
| REQ-PIP-032 | Task 06, Task 09 |
| REQ-PIP-033 | Task 06, Task 09 |
| REQ-PIP-034 | Task 06, Task 09 |
| REQ-PIP-035 | Task 06, Task 09 |
| REQ-PIP-036 | Task 06, Task 09 |
| REQ-PIP-040 | Task 06, Task 08 |
| REQ-PIP-041 | Task 06, Task 08 |
| REQ-PIP-042 | Task 06, Task 08, Task 09 |
| REQ-PIP-043 | Task 06, Task 08 |
| REQ-PIP-044 | Task 06, Task 08 |
| REQ-PIP-045 | Task 06, Task 08 |
| REQ-PIP-050 | Task 06, Task 08 |
| REQ-PIP-051 | Task 06, Task 08 |
| REQ-PIP-052 | Task 06, Task 08 |
| REQ-PIP-053 | Task 06, Task 08 |
| REQ-PIP-054 | Task 06, Task 08 |
| REQ-PIP-060 | Task 06, Task 12 |
| REQ-PIP-061 | Task 06, Task 12 |
| REQ-PIP-062 | Task 06, Task 12 |
| REQ-PIP-070 | Task 05, Task 12 |
| REQ-PIP-071 | Task 06, Task 12 |
| REQ-PIP-072 | Task 06, Task 12 |
| REQ-PIP-073 | Task 06, Task 12 |
| REQ-PIP-074 | Task 06, Task 12 |
| REQ-PIP-080 | Task 05, Task 11a |
| REQ-PIP-081 | Task 06, Task 11b |
| REQ-PIP-090 | Task 03, Task 05, Task 12 |
| REQ-PIP-091 | Task 04, Task 05, Task 12 |
| REQ-PIP-NF-001 | Task 02 (indexes), Task 12 |
| REQ-PIP-NF-002 | Task 06, Task 12 |
| REQ-PIP-NF-003 | Task 02 |
| REQ-PIP-NF-004 | Task 06, Task 12 |
