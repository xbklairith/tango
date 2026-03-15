# Requirements: Permission Grants (RBAC)

**Created:** 2026-03-15
**Status:** Draft
**Feature:** 17-permission-grants
**Dependencies:** 12-inbox-system (squad memberships established)

## Overview

Permission Grants add fine-grained role-based access control (RBAC) to Ari. Currently, squad membership roles (owner, admin, viewer) control access at the squad level, but all authenticated members can perform any action within their squad. This feature introduces a static permission matrix that maps (role, resource, action) tuples to allow/deny decisions for both human users (via squad membership role) and AI agents (via agent hierarchy role: captain, lead, member). A `requirePermission` helper enforces checks in every handler, replacing the current "membership exists = full access" pattern.

## Scope

**In Scope:**
- Static permission matrix in Go code mapping (role, resource, action) to allow/deny
- Permission resources: squad, agent, issue, project, goal, pipeline, inbox, conversation
- Permission actions: create, read, update, delete, assign, advance, reject, resolve
- User role enforcement: owner (all), admin (all except squad delete), viewer (read only)
- Agent role scoping: captain (broad), lead (team-scoped), member (self-scoped)
- `requirePermission(ctx, resource, action) error` helper function
- Middleware/handler integration for all existing endpoints
- React UI: Role management page showing permission matrix per role
- API endpoint to retrieve the permission matrix (read-only)

**Out of Scope (future):**
- Custom per-user or per-agent permission overrides stored in DB
- Dynamic permission rules (e.g., "allow if budget < $X")
- Resource-instance-level permissions (e.g., "user X can edit issue Y but not issue Z")
- Permission audit log (separate from activity log)
- OAuth scope mapping
- Cross-squad permission delegation

## Definitions

| Term | Definition |
|------|------------|
| Permission | A (resource, action) tuple representing a specific capability. |
| Permission Matrix | A static map of (role, resource, action) to boolean allow/deny. |
| Resource | A top-level entity type in Ari: squad, agent, issue, project, goal, pipeline, inbox, conversation. |
| Action | An operation on a resource: create, read, update, delete, assign, advance, reject, resolve. |
| User Role | The squad membership role: owner, admin, or viewer. Stored in `squad_memberships.role`. |
| Agent Role | The agent hierarchy role: captain, lead, or member. Stored in `agents.role` and in `AgentIdentity.Role`. |
| Permission Check | The act of verifying that the caller's role allows the requested (resource, action). |
| Caller | Either a human user (Identity) or an AI agent (AgentIdentity) making an API request. |

## Requirements (EARS Format)

### Permission Matrix Definition

**REQ-RBAC-001:** The system SHALL define a static permission matrix in Go code that maps each combination of (user role, resource, action) to an allow or deny decision.

**REQ-RBAC-002:** The system SHALL define a static permission matrix in Go code that maps each combination of (agent role, resource, action) to an allow or deny decision.

**REQ-RBAC-003:** The permission matrix SHALL cover the following resources: `squad`, `agent`, `issue`, `project`, `goal`, `pipeline`, `inbox`, `conversation`.

**REQ-RBAC-004:** The permission matrix SHALL cover the following actions: `create`, `read`, `update`, `delete`, `assign`, `advance`, `reject`, `resolve`.

**REQ-RBAC-005:** IF a (role, resource, action) tuple is not present in the matrix, THEN the system SHALL default to deny.

### User Role Permissions (Default Matrix)

**REQ-RBAC-010:** The `owner` role SHALL be allowed all actions on all resources.

**REQ-RBAC-011:** The `admin` role SHALL be allowed all actions on all resources EXCEPT `delete` on the `squad` resource.

**REQ-RBAC-012:** The `viewer` role SHALL be allowed only the `read` action on all resources.

**REQ-RBAC-013:** The `viewer` role SHALL be denied `create`, `update`, `delete`, `assign`, `advance`, `reject`, and `resolve` actions on all resources.

### Agent Role Permissions (Default Matrix)

**REQ-RBAC-020:** The `captain` agent role SHALL be allowed `create`, `read`, `update`, `assign`, `advance`, `reject`, and `resolve` actions on `issue`, `inbox`, and `conversation` resources.

**REQ-RBAC-021:** The `captain` agent role SHALL be allowed `read` on `agent`, `project`, `goal`, and `pipeline` resources.

**REQ-RBAC-022:** The `captain` agent role SHALL be denied `delete` on all resources and any action on the `squad` resource.

**REQ-RBAC-023:** The `lead` agent role SHALL be allowed `create`, `read`, `update`, `assign`, and `resolve` actions on `issue` and `inbox` resources.

**REQ-RBAC-024:** The `lead` agent role SHALL be allowed `read` on `agent`, `project`, `goal`, `pipeline`, and `conversation` resources.

**REQ-RBAC-025:** The `lead` agent role SHALL be denied `advance`, `reject`, and `delete` actions on `pipeline` resources, and any action on the `squad` resource.

**REQ-RBAC-026:** The `member` agent role SHALL be allowed `read` and `update` on `issue` resources (only issues assigned to itself).

**REQ-RBAC-027:** The `member` agent role SHALL be allowed `read` on `agent`, `project`, `goal`, `pipeline`, `inbox`, and `conversation` resources.

**REQ-RBAC-028:** The `member` agent role SHALL be denied `create`, `delete`, `assign`, `advance`, and `reject` actions on `issue` resources.

**REQ-RBAC-029:** The `member` agent role SHALL be allowed `create` and `read` on `inbox` resources (to ask humans for help) and `read` and `create` on `conversation` resources (to reply).

### Permission Enforcement Helper

**REQ-RBAC-030:** The system SHALL provide a `requirePermission(ctx context.Context, resource string, action string) error` function that extracts the caller identity from context, determines the caller's role, checks the permission matrix, and returns nil (allowed) or an error (denied).

**REQ-RBAC-031:** WHEN `requirePermission` returns an error, the error SHALL be suitable for conversion to an HTTP 403 response with error code `FORBIDDEN` and a message indicating which (resource, action) was denied.

**REQ-RBAC-032:** WHEN the context contains an `Identity` (human user), `requirePermission` SHALL determine the user's role by looking up the squad membership for the current squad scope.

**REQ-RBAC-033:** WHEN the context contains an `AgentIdentity` (AI agent), `requirePermission` SHALL use the `Role` field from the agent identity to check the agent permission matrix.

**REQ-RBAC-034:** WHEN the context contains neither `Identity` nor `AgentIdentity`, `requirePermission` SHALL return an authentication error (HTTP 401).

**REQ-RBAC-035:** WHEN the deployment mode is `local_trusted` and the caller is the `LocalOperatorIdentity`, `requirePermission` SHALL treat the caller as `owner` role.

### API Enforcement Integration

**REQ-RBAC-040:** All existing handler methods that perform create, update, delete, assign, advance, reject, or resolve operations SHALL call `requirePermission` before executing the operation.

**REQ-RBAC-041:** All existing handler methods that perform read operations SHALL call `requirePermission` with action `read` to enforce viewer-level access.

**REQ-RBAC-042:** WHEN `requirePermission` denies an operation, the handler SHALL return HTTP 403 with `{"error": "Permission denied: <resource>.<action>", "code": "FORBIDDEN"}`.

**REQ-RBAC-043:** The existing squad membership check (`GetSquadMembership`) SHALL remain as squad-scoping isolation. `requirePermission` is an additional check layered on top — the caller must be both a squad member AND have the required permission.

### Agent Role Scoping (Behavioral)

**REQ-RBAC-050:** WHEN an agent with role `captain` calls `POST /api/agent/me/issues` (create sub-issue), the system SHALL allow the operation per the captain's permission matrix.

**REQ-RBAC-051:** WHEN an agent with role `member` calls `POST /api/agent/me/issues` (create sub-issue), the system SHALL deny the operation with HTTP 403 and code `FORBIDDEN`.

**REQ-RBAC-052:** WHEN an agent with role `captain` or `lead` calls `POST /api/issues/{id}/advance`, the system SHALL allow the operation per their permission matrix.

**REQ-RBAC-053:** WHEN an agent with role `member` calls `POST /api/issues/{id}/advance`, the system SHALL deny the operation with HTTP 403 and code `FORBIDDEN`.

**REQ-RBAC-054:** WHEN an agent with role `member` updates an issue, the system SHALL verify the issue is assigned to that agent. IF not assigned, the system SHALL deny with HTTP 403.

### Permission Matrix API

**REQ-RBAC-060:** The system SHALL expose `GET /api/permissions` to return the full permission matrix as JSON, containing both user role permissions and agent role permissions.

**REQ-RBAC-061:** The `GET /api/permissions` endpoint SHALL require authentication but no specific role (any authenticated user or agent can view the matrix).

**REQ-RBAC-062:** The response format SHALL be:
```json
{
  "userRoles": {
    "owner": { "squad": ["create","read","update","delete"], ... },
    "admin": { "squad": ["create","read","update"], ... },
    "viewer": { "squad": ["read"], ... }
  },
  "agentRoles": {
    "captain": { "issue": ["create","read","update","assign","advance","reject","resolve"], ... },
    "lead": { ... },
    "member": { ... }
  }
}
```

### React UI: Role Management Page

**REQ-RBAC-070:** The system SHALL provide a React page at `/settings/permissions` displaying the permission matrix as a table (roles as columns, resources/actions as rows).

**REQ-RBAC-071:** The permission matrix page SHALL display both user roles and agent roles in separate sections.

**REQ-RBAC-072:** The permission matrix page SHALL be read-only in v1 (no editing). Each cell SHALL show a checkmark (allowed) or dash (denied).

**REQ-RBAC-073:** The permission matrix page SHALL be accessible from the squad settings navigation.

---

## Error Handling

| Scenario | HTTP Status | Error Code |
|----------|-------------|------------|
| Permission denied (user lacks role for action) | 403 | `FORBIDDEN` |
| Permission denied (agent lacks role for action) | 403 | `FORBIDDEN` |
| Permission denied (member updating unassigned issue) | 403 | `FORBIDDEN` |
| Not authenticated (no identity in context) | 401 | `UNAUTHENTICATED` |
| Squad membership not found (not a member) | 403 | `FORBIDDEN` |

---

## Non-Functional Requirements

**REQ-RBAC-NF-001:** Permission checks SHALL add no more than 1ms overhead per request (static map lookup, no DB query for the matrix itself).

**REQ-RBAC-NF-002:** The permission matrix SHALL be defined as Go constants/variables so it is compiled into the binary with zero runtime allocation for lookups.

**REQ-RBAC-NF-003:** The `requirePermission` function SHALL be safe for concurrent use from multiple goroutines.

**REQ-RBAC-NF-004:** Adding a new resource or action to the matrix SHALL require only adding entries to the Go map — no schema changes or migrations.

---

## Acceptance Criteria

1. A static permission matrix is defined in Go code covering all (role, resource, action) combinations
2. `requirePermission` correctly allows owner/admin/viewer based on the matrix
3. `requirePermission` correctly allows captain/lead/member based on the agent matrix
4. Viewer users can only read resources, not modify them
5. Admin users can do everything except delete squads
6. Captain agents can create sub-issues; member agents cannot
7. Member agents can only update issues assigned to them
8. All existing handlers call `requirePermission` before performing operations
9. `GET /api/permissions` returns the full matrix as JSON
10. React UI displays the permission matrix in a readable table
11. `local_trusted` mode treats the operator as owner
12. Permission checks add negligible latency (static map, no DB query)
13. Denied operations return HTTP 403 with descriptive error messages

---

## References

- Squad Memberships: `internal/database/migrations/20260314000004_create_squad_memberships.sql`
- Auth Types: `internal/auth/types.go` (Identity, DeploymentMode)
- Agent Identity: `internal/auth/run_token.go` (AgentIdentity, AgentFromContext)
- Auth Middleware: `internal/auth/middleware.go`
- Agent Domain: `internal/domain/agent.go` (AgentRole: captain, lead, member)
- Membership Queries: `internal/database/queries/squad_memberships.sql`
- Existing membership check pattern: `h.queries.GetSquadMembership(ctx, ...)` in all handlers
