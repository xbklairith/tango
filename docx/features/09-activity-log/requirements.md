# Requirements: Activity Log

**Created:** 2026-03-15
**Status:** Draft

## Overview

Add an append-only audit trail that records every mutation in the system. Each activity entry captures who did what, when, and to which entity. The activity log enables accountability, debugging, and a real-time feed on the dashboard showing recent actions across the squad.

Activity log entries are written synchronously within the same database transaction as the triggering mutation. All entries are squad-scoped and immutable — no UPDATE or DELETE routes exist for activity entries.

### Requirement ID Format

- Use sequential IDs: `REQ-001`, `REQ-002`, etc.
- Keep numbering continuous across all requirement categories.

---

## Activity Entry Schema

Every activity entry conforms to the following structure:

| Field       | Type        | Required | Description                                              |
|-------------|-------------|----------|----------------------------------------------------------|
| id          | UUID        | Yes      | Primary key, auto-generated                              |
| squadId     | UUID (FK)   | Yes      | Squad this entry belongs to — strict data isolation      |
| actorType   | enum        | Yes      | One of: `agent`, `user`, `system`                        |
| actorId     | UUID        | Yes      | ID of the actor; `uuid.Nil` for system-generated entries |
| action      | string      | Yes      | Dot-notation event name, e.g. `issue.created`            |
| entityType  | string      | Yes      | Affected entity kind, e.g. `issue`, `agent`, `squad`     |
| entityId    | UUID        | Yes      | ID of the affected entity                                |
| metadata    | JSONB       | No       | Action-specific context (e.g. `{"from": "todo", "to": "in_progress"}`) |
| createdAt   | TIMESTAMPTZ | Yes      | UTC timestamp of when the action occurred                |

### Action Name Conventions

Actions follow the pattern `{entityType}.{verb}`:

| Entity       | Actions                                                                 |
|--------------|-------------------------------------------------------------------------|
| `squad`      | `squad.created`, `squad.updated`, `squad.deleted`, `squad.budget_updated` |
| `agent`      | `agent.created`, `agent.updated`, `agent.status_changed`, `agent.deleted` |
| `issue`      | `issue.created`, `issue.updated`, `issue.status_changed`, `issue.deleted` |
| `comment`    | `comment.created`                                                       |
| `project`    | `project.created`, `project.updated`                                    |
| `goal`       | `goal.created`, `goal.updated`                                          |
| `member`     | `member.added`, `member.removed`, `member.role_changed`, `member.left`  |

---

## Functional Requirements

### Event-Driven Requirements

**Squad Mutations**

- [REQ-001] WHEN a squad is successfully created via `POST /api/squads` THEN the system SHALL append an activity entry with `action = "squad.created"`, `entityType = "squad"`, `entityId = <new squad ID>`, and `actorType = "user"` using the authenticated user's ID.

- [REQ-002] WHEN a squad's fields (name, description, status, settings, brandColor) are successfully updated via `PATCH /api/squads/{id}` THEN the system SHALL append an activity entry with `action = "squad.updated"` and include a `metadata` object containing the changed field names.

- [REQ-003] WHEN a squad is soft-deleted via `DELETE /api/squads/{id}` THEN the system SHALL append an activity entry with `action = "squad.deleted"`.

- [REQ-004] WHEN a squad's budget is successfully updated via `PATCH /api/squads/{id}/budgets` THEN the system SHALL append an activity entry with `action = "squad.budget_updated"` and include the new `budgetMonthlyCents` value in `metadata`.

**Agent Mutations**

- [REQ-005] WHEN an agent is successfully created via `POST /api/agents` THEN the system SHALL append an activity entry with `action = "agent.created"`, `entityType = "agent"`, `entityId = <new agent ID>`, `actorType = "user"`, and include `role` and `name` in `metadata`.

- [REQ-006] WHEN an agent's fields (name, shortName, role, adapterType, adapterConfig, systemPrompt, model, budgetMonthlyCents, parentAgentId) are successfully updated via `PATCH /api/agents/{id}` THEN the system SHALL append an activity entry with `action = "agent.updated"` and include a `metadata` object containing the changed field names.

- [REQ-007] WHEN an agent's status is successfully transitioned via `POST /api/agents/{id}/transition` THEN the system SHALL append an activity entry with `action = "agent.status_changed"` and include `{"from": "<old status>", "to": "<new status>"}` in `metadata`.

**Issue Mutations**

- [REQ-008] WHEN an issue is successfully created via `POST /api/squads/{squadId}/issues` THEN the system SHALL append an activity entry with `action = "issue.created"`, `entityType = "issue"`, `entityId = <new issue ID>`, and include `identifier`, `title`, and `status` in `metadata`.

- [REQ-009] WHEN an issue's fields are successfully updated via `PATCH /api/issues/{id}` and the status does not change THEN the system SHALL append an activity entry with `action = "issue.updated"` and include the changed field names in `metadata`.

- [REQ-010] WHEN an issue's status changes via `PATCH /api/issues/{id}` THEN the system SHALL append an activity entry with `action = "issue.status_changed"` and include `{"from": "<old status>", "to": "<new status>", "identifier": "<issue identifier>"}` in `metadata`.

- [REQ-011] WHEN an issue is successfully deleted via `DELETE /api/issues/{id}` THEN the system SHALL append an activity entry with `action = "issue.deleted"` and include `identifier` in `metadata`.

**Comment Mutations**

- [REQ-012] WHEN a comment is successfully created via `POST /api/issues/{issueId}/comments` THEN the system SHALL append an activity entry with `action = "comment.created"`, `entityType = "comment"`, `entityId = <new comment ID>`, and include `{"issueId": "<issue UUID>", "authorType": "<authorType>"}` in `metadata`.

**Project Mutations**

- [REQ-013] WHEN a project is successfully created via `POST /api/squads/{squadId}/projects` THEN the system SHALL append an activity entry with `action = "project.created"`, `entityType = "project"`, and include `name` in `metadata`.

- [REQ-014] WHEN a project is successfully updated via `PATCH /api/projects/{id}` THEN the system SHALL append an activity entry with `action = "project.updated"` and include the changed field names in `metadata`. IF the `status` field changes THEN the entry action SHALL be `"project.status_changed"` with `{"from": "<old>", "to": "<new>"}` in `metadata`.

**Goal Mutations**

- [REQ-015] WHEN a goal is successfully created via `POST /api/squads/{squadId}/goals` THEN the system SHALL append an activity entry with `action = "goal.created"`, `entityType = "goal"`, and include `title` in `metadata`.

- [REQ-016] WHEN a goal is successfully updated via `PATCH /api/goals/{id}` THEN the system SHALL append an activity entry with `action = "goal.updated"` and include the changed field names in `metadata`. IF the `status` field changes THEN the entry action SHALL be `"goal.status_changed"` with `{"from": "<old>", "to": "<new>"}` in `metadata`.

**Member Mutations**

- [REQ-017] WHEN a member is successfully added to a squad via `POST /api/squads/{id}/members` THEN the system SHALL append an activity entry with `action = "member.added"`, `entityType = "member"`, `entityId = <membership ID>`, and include `{"userId": "<uuid>", "role": "<role>"}` in `metadata`.

- [REQ-018] WHEN a member's role is successfully changed via `PATCH /api/squads/{id}/members/{memberId}` THEN the system SHALL append an activity entry with `action = "member.role_changed"` and include `{"userId": "<uuid>", "from": "<old role>", "to": "<new role>"}` in `metadata`.

- [REQ-019] WHEN a member is removed from a squad via `DELETE /api/squads/{id}/members/{memberId}` THEN the system SHALL append an activity entry with `action = "member.removed"` and include `{"userId": "<uuid>"}` in `metadata`.

- [REQ-020] WHEN an authenticated user leaves a squad via `DELETE /api/squads/{id}/members/me` THEN the system SHALL append an activity entry with `action = "member.left"` and include `{"userId": "<uuid>"}` in `metadata`.

**Activity Feed Endpoint**

- [REQ-021] WHEN a request is made to `GET /api/squads/{id}/activity` THEN the system SHALL return a paginated list of activity entries for the given squad, ordered by `createdAt` descending.

- [REQ-022] WHEN `GET /api/squads/{id}/activity` is called with `?limit=N&offset=M` query parameters THEN the system SHALL apply those values as page size and offset, defaulting to `limit=50` and `offset=0` if omitted.

- [REQ-023] WHEN `GET /api/squads/{id}/activity` is called with an `?actorType=<value>` query parameter THEN the system SHALL filter results to entries matching that `actorType` value.

- [REQ-024] WHEN `GET /api/squads/{id}/activity` is called with an `?entityType=<value>` query parameter THEN the system SHALL filter results to entries matching that `entityType` value.

### State-Driven Requirements

- [REQ-025] WHILE the dashboard is displaying squad activity THEN the system SHALL show the most recent N activity entries (default N = 20) for the squad, ordered by `createdAt` descending.

- [REQ-026] WHILE an activity write is being attempted THEN the system SHALL perform the write within the same database transaction as the triggering mutation so that the activity entry and the mutation either both commit or both roll back.

### Ubiquitous Requirements

- [REQ-027] The system SHALL store all activity entries as append-only records; no `UPDATE` or `DELETE` operations SHALL be permitted on activity entries.

- [REQ-028] The system SHALL scope every activity entry to a single squad via the non-nullable `squadId` foreign key.

- [REQ-029] The system SHALL include `actorType`, `actorId`, `action`, `entityType`, `entityId`, and `createdAt` on every activity entry; `metadata` is optional and defaults to an empty JSON object `{}`.

- [REQ-030] The system SHALL derive `squadId` for the activity entry from the entity being mutated — never from the request body.

- [REQ-031] The system SHALL assign `actorType = "system"` and `actorId = uuid.Nil` to activity entries generated by internal processes (e.g., system-generated reopen comments, budget enforcement).

- [REQ-032] The system SHALL enforce authentication on `GET /api/squads/{id}/activity` and return `401 UNAUTHENTICATED` if no valid session is present.

- [REQ-033] The system SHALL enforce squad membership on `GET /api/squads/{id}/activity` and return `404 SQUAD_NOT_FOUND` if the caller is not a member of the squad.

### Conditional Requirements

- [REQ-034] IF the activity feed for a squad has more entries than the requested page size THEN the system SHALL include a `pagination` envelope with `limit`, `offset`, and `total` fields in the response, matching the pattern used by `GET /api/squads/{squadId}/issues`.

- [REQ-035] IF the `actorType` filter value passed to `GET /api/squads/{id}/activity` is not one of `agent`, `user`, or `system` THEN the system SHALL return `400 VALIDATION_ERROR`.

- [REQ-036] IF the `entityType` filter value passed to `GET /api/squads/{id}/activity` is not one of `squad`, `agent`, `issue`, `comment`, `project`, `goal`, `member` THEN the system SHALL return `400 VALIDATION_ERROR`.

- [REQ-037] IF an activity write fails (e.g., database error) THEN the system SHALL roll back the entire transaction, including the triggering mutation, and return `500 INTERNAL_ERROR` to the caller.

### Optional Requirements

- [REQ-038] WHERE the `action` filter query parameter is provided on `GET /api/squads/{id}/activity` THEN the system SHALL filter results to entries whose `action` field exactly matches the provided value (e.g., `action=issue.status_changed`).

---

## Non-Functional Requirements

### Performance

- [REQ-039] The system SHALL complete each activity write (INSERT) within the latency budget of its enclosing transaction and SHALL NOT add more than 5 ms of overhead per mutation under normal load.

- [REQ-040] The system SHALL index the `activity_log` table on `(squad_id, created_at DESC)` to support efficient feed queries without full-table scans.

### Security

- [REQ-041] The system SHALL never expose activity entries belonging to a different squad; all queries MUST include a `WHERE squad_id = $1` clause bound to the verified squad ID from the request path.

- [REQ-042] The system SHALL not include sensitive field values (passwords, secret values, API keys, tokens) in the `metadata` JSONB column.

### Reliability

- [REQ-043] The system SHALL write activity entries transactionally with the mutations they describe so that no mutation can succeed without a corresponding activity entry and no orphaned activity entry can exist for a rolled-back mutation.

- [REQ-044] The system SHALL not silently swallow activity write failures; a failed activity write MUST cause the entire enclosing transaction to fail and the appropriate HTTP error to be returned.

---

## Constraints

- Activity entries are immutable (append-only; no UPDATE or DELETE routes on `activity_log`)
- Must be squad-scoped; `squadId` is a non-nullable foreign key with `ON DELETE CASCADE`
- Must not degrade write performance on existing mutations; single INSERT per mutation is acceptable
- `actorType` enum values: `agent`, `user`, `system`
- `entityType` values (controlled list): `squad`, `agent`, `issue`, `comment`, `project`, `goal`, `member`
- Action strings use dot-notation: `{entityType}.{verb}`
- `metadata` is a JSONB column, not a string; default is `'{}'`

---

## Acceptance Criteria

- [ ] Every successful mutation across squads, agents, issues, comments, projects, goals, and members creates an activity log entry
- [ ] Activity log writes occur within the same DB transaction as the triggering mutation
- [ ] `GET /api/squads/{id}/activity` returns paginated results with `data` + `pagination` envelope
- [ ] Activity feed is filtered to the authenticated user's squad (squad membership enforced)
- [ ] `actorType`, `actorId`, `action`, `entityType`, `entityId`, and `createdAt` are present on every returned entry
- [ ] `metadata` is returned as a JSON object (never `null`)
- [ ] Failed activity writes roll back the mutation and return a 500 error
- [ ] Dashboard activity feed shows the 20 most recent entries for the selected squad
- [ ] `actorType` and `entityType` query filters work correctly on the feed endpoint
- [ ] No activity entries are returned for squads the authenticated user does not belong to

---

## Out of Scope

- Activity log search / full-text search
- Activity export (CSV, JSON)
- Webhook notifications on activity events
- Cross-squad activity views
- Retention policies / automatic expiry of old entries
- Before/after field snapshots (diff-style change records) — `metadata` captures key context only
- Real-time SSE push of `activity.appended` events (tracked separately in SSE feature)

---

## Dependencies

- Database schema (features 01–03): `squads`, `users`, `squad_memberships` tables must exist
- All CRUD handlers (features 03–06) must be wrapped to emit activity entries
- Authentication middleware (feature 02) must be in place to identify `actorId`

---

## Risks & Assumptions

**Assumptions:**
- Activity writes are synchronous within the same transaction; async/fire-and-forget is explicitly rejected (see REQ-043, REQ-044)
- Activity volume is manageable with a single PostgreSQL table and a `(squad_id, created_at DESC)` index
- `metadata` does not need a strict schema per action type in v1; free-form JSONB is sufficient
- The dashboard calls the existing REST endpoint rather than a separate aggregation query

**Risks:**
- High-frequency mutations (agent heartbeats, bulk issue imports) could grow the activity table quickly; a retention/archival strategy will be needed in a later phase
- Adding an activity INSERT to every handler increases code surface and makes it easy to accidentally omit the write; a shared helper or middleware will be needed to reduce this risk
- If `metadata` content is not validated, sensitive data could leak into the audit trail (see REQ-042)

---

## References

- PRD: `docx/core/01-PRODUCT.md` — sections 3.3 (ActivityLog schema), 5.2.6 (Immutable Audit Trail), 6.3 (Activity & Access API table), 9.2 (Recent activity dashboard metric)
- Feature 03 (Squad Management), Feature 04 (Agent Management), Feature 05 (Issue Tracking), Feature 06 (Projects & Goals)
