# Requirements: Issue Tracking

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 05-issue-tracking
**Dependencies:** 01-go-scaffold, 02-user-auth, 03-squad-management, 04-agent-management

## Overview

Issue Tracking is the core work-management feature of Ari. Every unit of work is an "issue" — a task assigned to an agent or user, tracked through a status lifecycle, and scoped to a squad. Issues support sub-task hierarchies, threaded comments, and auto-generated human-readable identifiers (e.g., "ARI-39"). This feature covers basic CRUD for issues and comments. Checkout/lock, pipeline stages, and conversation features are deferred to Phase 2.

## Scope

**In Scope (Phase 1):**
- Issue entity with full field set (excluding Phase 2 lock/pipeline execution)
- Auto-generated identifiers from squad prefix + counter
- Issue status state machine with validated transitions
- Sub-task hierarchy via parentId
- IssueComment entity for threaded discussion
- CRUD API for issues and comments
- Squad-scoped data isolation
- Filtering and listing issues by status, priority, type, assignee, project, goal

**Out of Scope (Phase 2+):**
- Checkout/lock mechanism (checkoutRunId, executionLockedAt)
- IssuePipeline, advance/reject-stage endpoints
- Conversation type behavior (real-time agent invocation on comment)
- IssueAttachments and IssueLabels
- IssueApprovals (governance links)

## Definitions

| Term | Definition |
|------|------------|
| Issue | The unit of work in Ari. Can be a task or conversation. Squad-scoped. |
| Identifier | Human-readable issue reference: `{squad.issuePrefix}-{counter}` (e.g., "ARI-39") |
| Sub-task | An issue whose parentId references another issue in the same squad |
| IssueComment | A markdown comment on an issue, authored by an agent, user, or system |
| Squad scope | All issues belong to exactly one squad; queries never cross squad boundaries |

## Requirements (EARS Format)

### Issue Entity

**REQ-ISS-001:** When an issue is created, the system shall assign a UUID as the primary key (`id`).

**REQ-ISS-002:** When an issue is created, the system shall auto-generate an `identifier` by combining the squad's `issuePrefix` with the squad's `issueCounter` incremented by one (e.g., if issuePrefix is "ARI" and issueCounter is 38, the identifier shall be "ARI-39").

**REQ-ISS-003:** When an issue identifier is generated, the system shall atomically increment the squad's `issueCounter` to prevent duplicate identifiers under concurrent creation.

**REQ-ISS-004:** The system shall store the following fields on each issue: `id` (UUID), `squadId` (FK), `identifier` (string), `type` (enum), `title` (string), `description` (text/markdown), `status` (enum), `priority` (enum), `parentId` (FK, nullable), `projectId` (FK, nullable), `goalId` (FK, nullable), `assigneeAgentId` (FK, nullable), `assigneeUserId` (FK, nullable), `billingCode` (string, nullable), `requestDepth` (int), `pipelineId` (FK, nullable, Phase 2), `currentStage` (string, nullable, Phase 2), `createdAt` (timestamp), `updatedAt` (timestamp).

**REQ-ISS-005:** The `type` field shall accept exactly the values: `task` (default), `conversation`.

**REQ-ISS-006:** The `status` field shall accept exactly the values: `backlog`, `todo`, `in_progress`, `done`, `blocked`, `cancelled`.

**REQ-ISS-007:** The `priority` field shall accept exactly the values: `critical`, `high`, `medium`, `low`.

**REQ-ISS-008:** When no `status` is provided at creation, the system shall default to `backlog`.

**REQ-ISS-009:** When no `priority` is provided at creation, the system shall default to `medium`.

**REQ-ISS-010:** When no `type` is provided at creation, the system shall default to `task`.

**REQ-ISS-011:** When no `requestDepth` is provided at creation, the system shall default to `0`.

### Issue Status State Machine

**REQ-ISS-020:** The system shall enforce the following valid status transitions:

| From | Allowed To |
|------|-----------|
| backlog | todo, in_progress, cancelled |
| todo | in_progress, backlog, blocked, cancelled |
| in_progress | done, blocked, cancelled |
| blocked | in_progress, todo, cancelled |
| done | todo (reopen) |
| cancelled | todo (reopen) |

**REQ-ISS-021:** When a status transition is attempted that is not in the allowed set, the system shall reject the request with HTTP 422 and an error message indicating the invalid transition.

**REQ-ISS-022:** When an issue is reopened (done -> todo or cancelled -> todo), the system shall record a system-generated IssueComment noting the reopen event.

### Sub-task Hierarchy

**REQ-ISS-030:** When `parentId` is provided, the system shall validate that the referenced parent issue exists and belongs to the same squad.

**REQ-ISS-031:** The system shall prevent circular parent references (an issue cannot be its own parent, directly or transitively).

**REQ-ISS-032:** When an issue has sub-tasks, the system shall not allow deletion of the parent issue unless all sub-tasks are deleted or reassigned first.

**REQ-ISS-033:** While the system supports multi-level sub-task nesting, there is no enforced depth limit in Phase 1.

### Issue Comment Entity

**REQ-ISS-040:** When an issue comment is created, the system shall assign a UUID as the primary key (`id`).

**REQ-ISS-041:** The system shall store the following fields on each comment: `id` (UUID), `issueId` (FK), `authorType` (enum), `authorId` (UUID), `body` (text/markdown), `createdAt` (timestamp), `updatedAt` (timestamp).

**REQ-ISS-042:** The `authorType` field shall accept exactly the values: `agent`, `user`, `system`.

**REQ-ISS-043:** When `authorType` is `agent`, the `authorId` shall reference a valid agent. When `authorType` is `user`, the `authorId` shall reference a valid user.

**REQ-ISS-044:** Comments shall be immutable after creation (no edit or delete in Phase 1). System-generated comments (e.g., status change notes) use `authorType=system`.

### Issue CRUD API

**REQ-ISS-050:** The system shall expose `POST /api/squads/:squadId/issues` to create a new issue within a squad.

**REQ-ISS-051:** The system shall expose `GET /api/squads/:squadId/issues` to list all issues within a squad, supporting the following query filters: `status`, `priority`, `type`, `assigneeAgentId`, `assigneeUserId`, `projectId`, `goalId`, `parentId`.

**REQ-ISS-052:** The system shall expose `GET /api/issues/:id` to retrieve a single issue by UUID or by identifier (e.g., "ARI-39").

**REQ-ISS-053:** The system shall expose `PATCH /api/issues/:id` to update an issue's mutable fields: `title`, `description`, `status`, `priority`, `parentId`, `projectId`, `goalId`, `assigneeAgentId`, `assigneeUserId`, `billingCode`, `type`.

**REQ-ISS-054:** The system shall expose `DELETE /api/issues/:id` to soft-delete or hard-delete an issue, subject to REQ-ISS-032 (no deletion with active sub-tasks).

**REQ-ISS-055:** When `GET /api/issues/:id` receives a value matching the pattern `{PREFIX}-{NUMBER}`, the system shall resolve it as an identifier lookup instead of a UUID lookup.

**REQ-ISS-056:** All issue API endpoints shall require authentication (valid JWT).

**REQ-ISS-057:** All issue API endpoints shall enforce squad-scoped data isolation: a user can only access issues belonging to squads they are a member of.

### Comment API

**REQ-ISS-060:** The system shall expose `POST /api/issues/:issueId/comments` to create a new comment on an issue.

**REQ-ISS-061:** The system shall expose `GET /api/issues/:issueId/comments` to list all comments for an issue, ordered by `createdAt` ascending.

**REQ-ISS-062:** Comment creation shall require a non-empty `body` field.

**REQ-ISS-063:** Comment endpoints shall enforce the same authentication and squad-scoped isolation as issue endpoints (REQ-ISS-056, REQ-ISS-057).

### Data Isolation and Validation

**REQ-ISS-070:** All issue queries shall be scoped to the authenticated user's accessible squads. Cross-squad issue access shall return HTTP 403.

**REQ-ISS-071:** When creating or updating an issue, if `assigneeAgentId` is provided, the system shall validate the agent belongs to the same squad.

**REQ-ISS-072:** When creating or updating an issue, if `assigneeUserId` is provided, the system shall validate the user is a member of the same squad.

**REQ-ISS-073:** When creating or updating an issue, if `projectId` is provided, the system shall validate the project belongs to the same squad.

**REQ-ISS-074:** When creating or updating an issue, if `goalId` is provided, the system shall validate the goal belongs to the same squad.

**REQ-ISS-075:** The `title` field shall be required and non-empty, with a maximum length of 500 characters.

**REQ-ISS-076:** The `identifier` field shall be unique within a squad and immutable after creation.

### Pagination and Ordering

**REQ-ISS-080:** The `GET /api/squads/:squadId/issues` endpoint shall support pagination via `limit` (default 50, max 200) and `offset` query parameters.

**REQ-ISS-081:** The `GET /api/squads/:squadId/issues` endpoint shall support ordering via `sort` query parameter with allowed values: `created_at`, `updated_at`, `priority`, `status`. Default sort shall be `created_at` descending.

**REQ-ISS-082:** The `GET /api/issues/:issueId/comments` endpoint shall support pagination via `limit` (default 50, max 200) and `offset` query parameters.

### Error Handling

**REQ-ISS-090:** When a referenced entity (squad, agent, user, project, goal, parent issue) does not exist, the system shall return HTTP 404 with a descriptive error message.

**REQ-ISS-091:** When a request contains invalid field values (unknown enum value, title too long, etc.), the system shall return HTTP 400 with a descriptive error message.

**REQ-ISS-092:** When an invalid status transition is attempted, the system shall return HTTP 422 with the error code `INVALID_STATUS_TRANSITION` and a message indicating the current status and attempted status.

**REQ-ISS-093:** All error responses shall follow the standard format: `{"error": "message", "code": "CODE"}`.

## Non-Functional Requirements

**REQ-ISS-NF-001:** Issue list queries shall respond within 200ms for squads with up to 10,000 issues.

**REQ-ISS-NF-002:** Issue creation (including identifier generation) shall be safe under concurrent requests with no duplicate identifiers.

**REQ-ISS-NF-003:** The database schema shall include indexes on: `squadId`, `status`, `priority`, `assigneeAgentId`, `assigneeUserId`, `projectId`, `goalId`, `parentId`, `identifier`.

**REQ-ISS-NF-004:** The identifier generation shall use a database-level atomic operation (e.g., UPDATE ... RETURNING or SELECT FOR UPDATE) to guarantee uniqueness.

## Acceptance Criteria Summary

1. Issues can be created with auto-generated identifiers (e.g., "ARI-1", "ARI-2")
2. Issues can be retrieved by UUID or by identifier string
3. Issue status transitions are validated against the state machine
4. Sub-tasks reference parent issues within the same squad
5. Comments can be created and listed on issues
6. All endpoints enforce JWT authentication and squad-scoped isolation
7. Concurrent issue creation produces unique, sequential identifiers
8. Invalid requests return appropriate HTTP status codes and error messages
9. List endpoints support filtering, pagination, and sorting
