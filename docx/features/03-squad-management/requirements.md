# Requirements: Squad Management

**Created:** 2026-03-14
**Status:** Draft
**Dependencies:** 01-go-scaffold, 02-user-auth

## Overview

Squad Management provides the top-level organizational unit in Ari. A Squad is the boundary for data isolation, budget enforcement, and team membership. All entities (agents, issues, projects, goals, cost events) belong to exactly one squad. This feature covers Squad CRUD, SquadMembership management, squad-scoped data isolation, issue identifier prefixes with auto-incrementing counters, squad-level settings, and budget configuration.

## Definitions

| Term | Definition |
|------|-----------|
| Squad | Top-level organizational unit (team of agents and users) |
| SquadMembership | Association between a user and a squad with a specific role |
| Issue Prefix | Short uppercase string (e.g., "ARI") used to generate human-readable issue identifiers |
| Issue Counter | Auto-incrementing integer per squad, combined with issue prefix to form identifiers like "ARI-1" |
| Squad Settings | JSONB configuration controlling squad-level governance policies |
| Budget | Monthly spend limit in cents; NULL means unlimited |

## Requirements (EARS Format)

### Squad Entity

**REQ-SM-001:** The system shall store a Squad entity with the following fields: id (UUID), name (string), slug (string), issuePrefix (string), description (text), status (enum: active, paused, archived), settings (JSONB), issueCounter (integer), budgetMonthlyCents (integer, nullable), brandColor (string, nullable), createdAt (timestamp), and updatedAt (timestamp).

**REQ-SM-002:** When a squad is created, the system shall generate a URL-safe slug from the squad name that is unique across all squads.

**REQ-SM-003:** The system shall enforce that squad name is non-empty and does not exceed 100 characters.

**REQ-SM-004:** The system shall enforce that squad slug is lowercase alphanumeric with hyphens, between 2 and 50 characters, and unique across all squads.

**REQ-SM-005:** The system shall enforce that issuePrefix is uppercase alphanumeric, between 2 and 10 characters, and unique across all squads.

**REQ-SM-006:** When a squad is created, the system shall initialize issueCounter to 0.

**REQ-SM-007:** The system shall store squad settings as a JSONB column with at minimum the following keys: requireApprovalForNewAgents (boolean, default false).

**REQ-SM-008:** When budgetMonthlyCents is NULL, the system shall treat the squad budget as unlimited.

**REQ-SM-009:** When budgetMonthlyCents is provided, the system shall enforce that it is a positive integer.

### Squad CRUD

**REQ-SM-010:** When a user sends POST /api/squads with valid name, issuePrefix, and optional description, settings, and budgetMonthlyCents, the system shall create a new squad and return it with HTTP 201.

**REQ-SM-011:** When a squad is created via POST /api/squads, the system shall automatically create a SquadMembership with role "owner" for the requesting user.

**REQ-SM-012:** When a user sends GET /api/squads, the system shall return only the squads where the user has an active SquadMembership.

**REQ-SM-013:** When a user sends GET /api/squads/:id, the system shall return the squad details if the user has a SquadMembership for that squad; otherwise, it shall return HTTP 404.

**REQ-SM-014:** When a user with role "owner" or "admin" sends PATCH /api/squads/:id with valid fields, the system shall update the squad and return the updated squad with HTTP 200.

**REQ-SM-015:** When a user with role "viewer" sends PATCH /api/squads/:id, the system shall return HTTP 403.

**REQ-SM-016:** While a squad has active agents or unresolved issues, the system shall prevent deletion and return HTTP 409 with an explanatory error.

**REQ-SM-017:** When an "owner" sends DELETE /api/squads/:id for a squad with no active agents and no unresolved issues, the system shall soft-delete the squad by setting status to "archived" and return HTTP 200.

### SquadMembership Entity

**REQ-SM-020:** The system shall store a SquadMembership entity with the following fields: id (UUID), userId (FK to User), squadId (FK to Squad), role (enum: owner, admin, viewer), createdAt (timestamp), and updatedAt (timestamp).

**REQ-SM-021:** The system shall enforce that the combination of userId and squadId is unique (a user can have only one membership per squad).

**REQ-SM-022:** The system shall enforce that every squad has at least one member with role "owner".

**REQ-SM-023:** When the last owner of a squad attempts to change their role or leave the squad, the system shall reject the operation with HTTP 409 and an explanatory error.

### SquadMembership Management

**REQ-SM-024:** When a user with role "owner" or "admin" sends POST /api/squads/:id/members with a valid userId and role, the system shall create a SquadMembership and return it with HTTP 201.

**REQ-SM-025:** When a user with role "owner" sends PATCH /api/squads/:id/members/:memberId with a new role, the system shall update the membership role and return HTTP 200.

**REQ-SM-026:** Only a user with role "owner" shall be able to grant or change a membership to the "owner" or "admin" role.

**REQ-SM-027:** When a user with role "owner" sends DELETE /api/squads/:id/members/:memberId, the system shall remove the membership and return HTTP 200, subject to REQ-SM-023.

**REQ-SM-028:** When a user sends DELETE /api/squads/:id/members/me, the system shall remove their own membership from the squad, subject to REQ-SM-023.

### Multi-Squad Support

**REQ-SM-029:** The system shall support a user being a member of multiple squads simultaneously, each with an independent role (owner, admin, or viewer).

### Squad-Scoped Data Isolation

**REQ-SM-030:** The system shall enforce that all squad-scoped entities (agents, issues, projects, goals, cost events, activity log entries, inbox items) reference exactly one squad via a non-nullable squadId foreign key.

**REQ-SM-031:** When a user requests any squad-scoped resource, the system shall verify the user has an active SquadMembership for that squad; otherwise, it shall return HTTP 404 (not 403, to avoid leaking squad existence).

**REQ-SM-032:** The system shall never return data from one squad in the context of another squad's API requests.

**REQ-SM-033:** Where the system uses database queries to list squad-scoped entities, the query shall always include a WHERE squad_id = $1 clause (no unscoped queries).

**REQ-SM-034:** The backend shall not maintain any server-side "active squad" or "current squad" state. The squad context for every request shall be determined solely by the squad ID in the URL path.

**REQ-SM-035:** The system shall support concurrent API requests targeting different squads from the same authenticated user (e.g., multiple browser tabs operating on different squads simultaneously).

### Issue Identifier Prefixes

**REQ-SM-040:** When an issue is created within a squad, the system shall atomically increment the squad's issueCounter and generate an identifier in the format "{issuePrefix}-{issueCounter}" (e.g., "ARI-1", "ACME-42").

**REQ-SM-041:** The system shall ensure issue identifiers are unique within a squad by using the atomic increment pattern (no gaps under normal operation, gaps acceptable after failed transactions).

**REQ-SM-042:** The system shall use an atomic database operation (e.g., UPDATE ... RETURNING or CAS) for issueCounter increment to prevent duplicate identifiers under concurrent issue creation.

### Squad Settings

**REQ-SM-050:** When settings.requireApprovalForNewAgents is true, the system shall place newly created agents in "pending_approval" status instead of "active".

**REQ-SM-051:** When a user with role "owner" or "admin" sends PATCH /api/squads/:id with a settings field, the system shall merge the provided settings keys with existing settings (partial update, not full replacement).

**REQ-SM-052:** The system shall validate settings keys against a known schema and reject unknown keys with HTTP 400.

### Squad-Level Budgets

**REQ-SM-060:** When a user with role "owner" sends PATCH /api/squads/:id/budgets with a budgetMonthlyCents value, the system shall update the squad's monthly budget and return HTTP 200.

**REQ-SM-061:** When a user with role "admin" or "viewer" sends PATCH /api/squads/:id/budgets, the system shall return HTTP 403.

**REQ-SM-062:** The system shall compute current monthly spend from CostEvents (SUM of costCents WHERE createdAt >= month start UTC) rather than storing a mutable spend counter.

**REQ-SM-063:** When squad monthly spend reaches 80% of budgetMonthlyCents, the system shall generate a budget warning alert (via the inbox/notification system).

**REQ-SM-064:** When squad monthly spend reaches 100% of budgetMonthlyCents, the system shall enforce a hard stop: new cost-incurring agent runs shall be blocked until the budget is increased or the month resets.

**REQ-SM-065:** The PATCH /api/squads/:id/budgets endpoint shall accept budgetMonthlyCents as either a positive integer or null (to set unlimited).

### API Response Format

**REQ-SM-070:** All squad API endpoints shall return responses with Content-Type: application/json.

**REQ-SM-071:** All error responses shall follow the format: {"error": "message", "code": "ERROR_CODE"}.

**REQ-SM-072:** The POST /api/squads endpoint shall return the created squad object including id, name, slug, issuePrefix, description, status, settings, budgetMonthlyCents, createdAt, and updatedAt.

**REQ-SM-073:** The GET /api/squads endpoint shall return an array of squad objects with the user's role included in each object.

### Validation and Error Handling

**REQ-SM-080:** When a user sends POST /api/squads with a duplicate issuePrefix, the system shall return HTTP 409 with error code "ISSUE_PREFIX_TAKEN".

**REQ-SM-081:** When a user sends POST /api/squads with a duplicate slug, the system shall return HTTP 409 with error code "SLUG_TAKEN".

**REQ-SM-082:** When a user sends a request to a squad that does not exist, the system shall return HTTP 404 with error code "SQUAD_NOT_FOUND".

**REQ-SM-083:** When a user sends PATCH /api/squads/:id with an invalid status transition (e.g., archived to active), the system shall return HTTP 400 with error code "INVALID_STATUS_TRANSITION".

**REQ-SM-084:** When any required field is missing in a create or update request, the system shall return HTTP 400 with error code "VALIDATION_ERROR" and a message indicating the missing fields.

## Non-Functional Requirements

**REQ-SM-090:** Squad creation and update operations shall complete within 200ms (p95) under normal load.

**REQ-SM-091:** The squad list endpoint shall support pagination with limit/offset parameters, defaulting to limit=50.

**REQ-SM-092:** The issueCounter increment shall be safe under concurrent access from multiple agent runs creating issues simultaneously.

**REQ-SM-093:** All squad mutations (create, update, delete, membership changes) shall be recorded in the activity log.

## Requirement Traceability

| Requirement | PRD Section | Category |
|-------------|-------------|----------|
| REQ-SM-001 to REQ-SM-009 | 3.1 Data Model — Squad | Entity Definition |
| REQ-SM-010 to REQ-SM-017 | 6.1 API — Squads | CRUD Operations |
| REQ-SM-020 to REQ-SM-028 | 3.1 Data Model — SquadMembership | Membership |
| REQ-SM-029 | BR-3.2 Multi-Squad Isolation | Multi-Squad |
| REQ-SM-030 to REQ-SM-035 | 5.1 Data Isolation | Security |
| REQ-SM-040 to REQ-SM-042 | 3.1 Data Model — Issue Identifier | Identifiers |
| REQ-SM-050 to REQ-SM-052 | 3.1 Data Model — Squad Settings | Governance |
| REQ-SM-060 to REQ-SM-065 | 5.2 Budget Enforcement | Cost Control |
| REQ-SM-070 to REQ-SM-073 | 6.1 API Format | API |
| REQ-SM-080 to REQ-SM-084 | General | Validation |
| REQ-SM-090 to REQ-SM-093 | General | Non-Functional |
