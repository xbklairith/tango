# Requirements: Issue Pipelines (Multi-Agent Workflows)

**Created:** 2026-03-15
**Status:** Draft
**Feature:** 14-issue-pipelines
**Dependencies:** 05-issue-tracking, 11-agent-runtime, 13-conversations

## Overview

Issue Pipelines enable multi-agent collaboration by defining reusable workflow templates. A pipeline is a squad-scoped sequence of stages, each optionally assigned to a specific agent. When an issue is attached to a pipeline, it flows through stages sequentially -- advancing automatically when the current stage's agent completes its work, or manually via API. Each stage transition re-assigns the issue to the next agent and auto-wakes that agent, enabling hands-off orchestration of complex multi-step workflows.

## Scope

**In Scope:**
- Pipeline entity (squad-scoped workflow template) with ordered stages
- Pipeline stage entity (name, position, optional agent assignment, description)
- CRUD API for pipelines and stages
- Attaching an issue to a pipeline (sets `pipeline_id` and `current_stage_id`)
- Stage advancement (manual via API, automatic on issue completion)
- Stage rejection (send issue back to previous stage)
- Auto-wake agent on stage transition
- Activity log entries for pipeline events
- SSE events for pipeline state changes
- React UI: pipeline builder and issue stage indicator

**Out of Scope (future):**
- Parallel pipeline stages (fan-out/fan-in)
- Conditional branching (skip stages based on rules)
- Pipeline versioning (editing a pipeline does not affect in-flight issues)
- Cross-squad pipeline sharing
- Pipeline-level analytics and metrics dashboard
- Stage-level SLA timers and escalation
- Approval gates (require human sign-off before advancing) — **v2 enhancement; hook point documented via `gate_id` column on `pipeline_stages`**

## Definitions

| Term | Definition |
|------|------------|
| Pipeline | A squad-scoped, reusable workflow template defining an ordered sequence of stages. |
| Pipeline Stage | A named step within a pipeline, with a position (order), optional assigned agent, and description. |
| Current Stage | The pipeline stage an issue is currently executing. Tracked via `current_stage_id` on the issue. |
| Stage Advancement | Moving an issue from its current stage to the next stage in pipeline order. |
| Stage Rejection | Moving an issue back to the previous stage in pipeline order. |
| Pipeline Attachment | Associating an issue with a pipeline, setting it to the pipeline's first stage. |
| Manual Stage | A pipeline stage with no assigned agent; requires user-driven advancement. |

## Requirements (EARS Format)

### Pipeline Entity

**REQ-PIP-001:** WHEN a pipeline is created, the system SHALL assign a UUID as the primary key (`id`).

**REQ-PIP-002:** The system SHALL store the following fields on each pipeline: `id` (UUID), `squad_id` (FK), `name` (string, required), `description` (text, nullable), `is_active` (boolean, default true), `created_at` (timestamp), `updated_at` (timestamp).

**REQ-PIP-003:** The system SHALL enforce that pipeline `name` is unique within a squad.

**REQ-PIP-004:** The system SHALL enforce that pipeline `name` is non-empty with a maximum length of 200 characters.

**REQ-PIP-005:** The system SHALL always scope pipelines to a single squad; cross-squad pipeline access SHALL be rejected with HTTP 403.

### Pipeline Stage Entity

**REQ-PIP-010:** WHEN a pipeline stage is created, the system SHALL assign a UUID as the primary key (`id`).

**REQ-PIP-011:** The system SHALL store the following fields on each pipeline stage: `id` (UUID), `pipeline_id` (FK), `name` (string, required), `description` (text, nullable), `position` (integer, required), `assigned_agent_id` (FK, nullable), `gate_id` (FK, nullable, reserved for v2 approval gates), `created_at` (timestamp), `updated_at` (timestamp).

**REQ-PIP-012:** The system SHALL enforce that `position` values are unique within a pipeline (no two stages share the same position).

**REQ-PIP-013:** The system SHALL enforce that stage `name` is non-empty with a maximum length of 200 characters.

**REQ-PIP-014:** IF `assigned_agent_id` is provided, THEN the system SHALL validate that the agent belongs to the same squad as the pipeline.

**REQ-PIP-015:** The system SHALL enforce that `position` is a positive integer (>= 1).

**REQ-PIP-016:** WHEN a pipeline has zero stages, the system SHALL prevent any issue from being attached to it.

### Pipeline CRUD API

**REQ-PIP-020:** The system SHALL expose `POST /api/squads/{squadId}/pipelines` to create a new pipeline within a squad.

**REQ-PIP-021:** The system SHALL expose `GET /api/squads/{squadId}/pipelines` to list all pipelines within a squad, supporting query parameters: `is_active` (boolean filter), `limit` (default 50, max 200), `offset`.

**REQ-PIP-022:** The system SHALL expose `GET /api/pipelines/{id}` to retrieve a single pipeline by UUID, including its stages ordered by position.

**REQ-PIP-023:** The system SHALL expose `PATCH /api/pipelines/{id}` to update a pipeline's `name`, `description`, and `is_active` fields.

**REQ-PIP-024:** The system SHALL expose `DELETE /api/pipelines/{id}` to delete a pipeline, subject to REQ-PIP-090.

**REQ-PIP-025:** The system SHALL expose `POST /api/pipelines/{id}/stages` to add a stage to a pipeline.

**REQ-PIP-026:** The system SHALL expose `PATCH /api/pipeline-stages/{stageId}` to update a stage's `name`, `description`, `position`, and `assigned_agent_id`.

**REQ-PIP-027:** The system SHALL expose `DELETE /api/pipeline-stages/{stageId}` to remove a stage from a pipeline, subject to REQ-PIP-091.

**REQ-PIP-028:** All pipeline and stage endpoints SHALL require authentication (valid JWT or Run Token).

**REQ-PIP-029:** All pipeline and stage endpoints SHALL enforce squad-scoped data isolation: a user can only access pipelines belonging to squads they are a member of.

### Issue-Pipeline Attachment

**REQ-PIP-030:** WHEN `PATCH /api/issues/{id}` includes a `pipelineId` field, the system SHALL attach the issue to the specified pipeline by setting `pipeline_id` and `current_stage_id` to the pipeline's first stage (lowest position).

**REQ-PIP-031:** WHEN an issue is attached to a pipeline, the system SHALL assign `assignee_agent_id` to the first stage's `assigned_agent_id` (if non-null) and auto-wake the agent.

**REQ-PIP-032:** WHEN an issue is attached to a pipeline, IF the issue status is `backlog`, THEN the system SHALL transition it to `todo`.

**REQ-PIP-033:** IF the pipeline specified in `pipelineId` belongs to a different squad than the issue, THEN the system SHALL reject the request with HTTP 422 and code `PIPELINE_SQUAD_MISMATCH`.

**REQ-PIP-034:** IF the pipeline specified in `pipelineId` has zero stages, THEN the system SHALL reject the request with HTTP 422 and code `PIPELINE_EMPTY`.

**REQ-PIP-035:** IF the pipeline specified in `pipelineId` has `is_active=false`, THEN the system SHALL reject the request with HTTP 422 and code `PIPELINE_INACTIVE`.

**REQ-PIP-036:** WHEN `PATCH /api/issues/{id}` sets `pipelineId` to `null`, the system SHALL detach the issue from the pipeline by clearing `pipeline_id` and `current_stage_id`, but SHALL NOT change `assignee_agent_id` or `status`.

### Stage Advancement

**REQ-PIP-040:** WHEN `POST /api/issues/{id}/advance` is called, the system SHALL move the issue to the next pipeline stage (next higher position) by updating `current_stage_id`, re-assigning `assignee_agent_id` to the new stage's agent (if non-null), and auto-waking the newly assigned agent. The issue status SHALL be set to `todo` for the new stage assignment.

**REQ-PIP-041:** WHEN an issue is advanced to a new stage, the system SHALL first persist the current stage as `done`, then create the new stage assignment with `status=todo`. This two-step approach ensures that the state machine transition follows the valid path: `in_progress` -> `done` (agent marks complete) -> `todo` (PipelineService reopen for next stage via `done` -> `todo`, which is a valid reopen transition).

**REQ-PIP-042:** WHEN an agent marks an issue as `done` via `PATCH /api/issues/{id}` (status change to `done`) AND the issue has a `current_stage_id`, THEN the system SHALL: (1) persist the `done` status for the current stage, then (2) automatically advance to the next stage by setting `status=todo` and updating `current_stage_id`. The `done` -> `todo` transition is valid per the state machine's reopen rule.

**REQ-PIP-043:** IF the issue is already at the pipeline's final stage (highest position) AND advancement is triggered, THEN the system SHALL set `status=done`, clear `current_stage_id`, and emit `pipeline.completed` SSE event.

**REQ-PIP-044:** IF `POST /api/issues/{id}/advance` is called on an issue that is not attached to a pipeline, THEN the system SHALL return HTTP 422 with code `NOT_IN_PIPELINE`.

**REQ-PIP-045:** IF `POST /api/issues/{id}/advance` is called on an issue at the final stage, THEN the system SHALL complete the pipeline per REQ-PIP-043.

### Stage Rejection

**REQ-PIP-050:** WHEN `POST /api/issues/{id}/reject` is called, the system SHALL move the issue back to the previous pipeline stage (next lower position) by updating `current_stage_id`, re-assigning `assignee_agent_id` to the previous stage's agent, and auto-waking the newly assigned agent.

**REQ-PIP-051:** WHEN an issue is rejected back to a previous stage, the system SHALL transition the issue status to `todo`.

**REQ-PIP-052:** The `POST /api/issues/{id}/reject` endpoint SHALL accept an optional `reason` field (string) that is recorded as a system-generated issue comment explaining the rejection.

**REQ-PIP-053:** IF `POST /api/issues/{id}/reject` is called on an issue at the first stage (lowest position), THEN the system SHALL return HTTP 422 with code `NO_PREVIOUS_STAGE`.

**REQ-PIP-054:** IF `POST /api/issues/{id}/reject` is called on an issue not attached to a pipeline, THEN the system SHALL return HTTP 422 with code `NOT_IN_PIPELINE`.

### Auto-Wake on Stage Transition

**REQ-PIP-060:** WHEN a stage transition occurs (advance or reject) AND the target stage has a non-null `assigned_agent_id`, THEN the system SHALL enqueue a wakeup request with `invocation_source=assignment` and context JSON containing `{"issue_id": "<id>", "pipeline_id": "<id>", "stage_name": "<name>", "transition": "advance|reject"}`.

**REQ-PIP-061:** WHEN a stage transition occurs AND the target stage has a null `assigned_agent_id` (manual stage), THEN the system SHALL NOT enqueue any wakeup request.

**REQ-PIP-062:** WHEN the auto-wake fires for a stage transition, the wakeup context SHALL include the pipeline and stage information so the agent knows which workflow step it is performing.

### Activity Logging

**REQ-PIP-070:** WHEN a pipeline is created, updated, or deleted, the system SHALL append an activity log entry with `entity_type=pipeline` and action `pipeline.created`, `pipeline.updated`, or `pipeline.deleted`.

**REQ-PIP-071:** WHEN an issue is attached to or detached from a pipeline, the system SHALL append an activity log entry with `entity_type=issue` and action `issue.pipeline.attached` or `issue.pipeline.detached`.

**REQ-PIP-072:** WHEN an issue advances to the next stage, the system SHALL append an activity log entry with action `issue.pipeline.advanced` including `from_stage` and `to_stage` in metadata.

**REQ-PIP-073:** WHEN an issue is rejected to a previous stage, the system SHALL append an activity log entry with action `issue.pipeline.rejected` including `from_stage`, `to_stage`, and `reason` in metadata.

**REQ-PIP-074:** WHEN a pipeline completes (final stage done), the system SHALL append an activity log entry with action `issue.pipeline.completed`.

### SSE Events

**REQ-PIP-080:** WHEN a pipeline is created, updated, or deleted, the system SHALL emit `pipeline.created`, `pipeline.updated`, or `pipeline.deleted` SSE events on the squad's event stream.

**REQ-PIP-081:** WHEN an issue's pipeline stage changes (advance, reject, attach, detach, complete), the system SHALL emit an `issue.pipeline.stage_changed` SSE event containing `issueId`, `pipelineId`, `fromStageId`, `toStageId`, `transition` (advance|reject|attach|detach|complete).

### Deletion Guards

**REQ-PIP-090:** IF a pipeline has issues currently attached to it (`pipeline_id` references this pipeline), THEN `DELETE /api/pipelines/{id}` SHALL return HTTP 422 with code `PIPELINE_IN_USE`.

**REQ-PIP-091:** IF a pipeline stage has issues currently at that stage (`current_stage_id` references this stage), THEN `DELETE /api/pipeline-stages/{stageId}` SHALL return HTTP 422 with code `STAGE_IN_USE`.

---

## Error Handling

| Scenario | HTTP Status | Error Code |
|----------|-------------|------------|
| Pipeline not found | 404 | `NOT_FOUND` |
| Pipeline stage not found | 404 | `NOT_FOUND` |
| Pipeline name already exists in squad | 409 | `PIPELINE_NAME_CONFLICT` |
| Duplicate stage position within pipeline | 409 | `POSITION_CONFLICT` |
| Issue not attached to pipeline (advance/reject) | 422 | `NOT_IN_PIPELINE` |
| No next stage (advance at final stage -- handled as completion) | -- | N/A (completes pipeline) |
| No previous stage (reject at first stage) | 422 | `NO_PREVIOUS_STAGE` |
| Pipeline belongs to different squad than issue | 422 | `PIPELINE_SQUAD_MISMATCH` |
| Pipeline has no stages | 422 | `PIPELINE_EMPTY` |
| Pipeline is inactive | 422 | `PIPELINE_INACTIVE` |
| Pipeline has attached issues (delete) | 422 | `PIPELINE_IN_USE` |
| Stage has issues at it (delete) | 422 | `STAGE_IN_USE` |
| Agent not in same squad as pipeline | 422 | `AGENT_SQUAD_MISMATCH` |
| Invalid pipeline name (empty or too long) | 400 | `VALIDATION_ERROR` |
| Invalid stage name (empty or too long) | 400 | `VALIDATION_ERROR` |
| Invalid position (not positive integer) | 400 | `VALIDATION_ERROR` |
| Unauthorized access | 403 | `FORBIDDEN` |

---

## Non-Functional Requirements

**REQ-PIP-NF-001:** Pipeline and stage CRUD operations SHALL respond within 100ms for squads with up to 100 pipelines and 20 stages per pipeline.

**REQ-PIP-NF-002:** Stage advancement and rejection SHALL complete (including auto-wake enqueue) within 200ms under normal PostgreSQL load.

**REQ-PIP-NF-003:** The database schema SHALL include indexes on `pipeline_id` and `current_stage_id` columns of the `issues` table to support efficient lookups.

**REQ-PIP-NF-004:** The system SHALL handle concurrent advance requests on the same issue safely -- only one advance SHALL succeed; the other SHALL receive HTTP 409.

---

## Acceptance Criteria

1. Pipelines can be created within a squad with a unique name
2. Pipeline stages can be added, reordered, updated, and removed
3. Issues can be attached to a pipeline and start at the first stage
4. Advancing an issue moves it to the next stage and auto-assigns + auto-wakes the next agent
5. Rejecting an issue moves it back to the previous stage with an optional reason comment
6. When the final stage completes, the issue status becomes `done`
7. Auto-advance triggers when an agent marks the issue done at a non-final stage (two-step: persist done, then reopen as todo at next stage)
8. SSE events fire for all pipeline state changes
9. Activity log captures all pipeline lifecycle events
10. Deletion is blocked when pipelines/stages are in use
11. All endpoints enforce JWT auth and squad-scoped isolation
12. React UI allows creating/editing pipelines and shows the current stage indicator on issues
13. Issue response DTOs include `pipelineId` and `currentStageId` fields

---

## References

- Issue Tracking: `docx/features/05-issue-tracking/`
- Agent Runtime: `docx/features/11-agent-runtime/`
- Conversations: `docx/features/13-conversations/`
- Issues table: `internal/database/migrations/20260314000006_create_issues.sql`
- Runtime tables: `internal/database/migrations/20260315000013_create_runtime_tables.sql`
- Issue domain model: `internal/domain/issue.go`
- Issue handler: `internal/server/handlers/issue_handler.go`
- Wakeup service: `internal/server/handlers/wakeup_handler.go`
- State machine transitions: `done` -> `todo` is a valid reopen transition in `issueValidTransitions`
