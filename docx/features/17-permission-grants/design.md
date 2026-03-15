# Design: Permission Grants (RBAC)

**Created:** 2026-03-15
**Status:** Ready for Implementation
**Feature:** 17-permission-grants
**Dependencies:** 12-inbox-system

---

## 1. Architecture Overview

Permission Grants add a static RBAC layer on top of the existing authentication and squad membership system. The core design is a compiled-in permission matrix (Go maps) checked by a `requirePermission` helper that every handler calls before performing an operation. No new database tables are needed for v1 -- the matrix is pure Go code.

### High-Level Flow

```
Request arrives
    |
    v
Auth Middleware (existing) — injects Identity or AgentIdentity into context
    |
    v
Handler extracts squad scope (from URL or entity lookup)
    |
    v
Handler calls requirePermission(ctx, resource, action)
    |
    v
requirePermission:
  1. Extract caller from ctx (Identity or AgentIdentity)
  2. Determine role:
     - Identity → lookup squad_memberships.role (already fetched for squad-scoping)
     - AgentIdentity → use AgentIdentity.Role field
     - LocalOperator → treat as "owner"
  3. Lookup (role, resource, action) in permission matrix
  4. Return nil (allowed) or error (denied)
    |
    v
Handler proceeds with operation (or returns 403)
```

### Component Relationships

```
┌─────────────────────────────────────────────────┐
│                   Handler Layer                  │
│                                                  │
│  requirePermission(ctx, "issue", "create")       │
│       │                                          │
│       ▼                                          │
│  ┌─────────────────────────────────┐             │
│  │     internal/auth/permissions   │             │
│  │                                 │             │
│  │  UserPermissions map            │             │
│  │  AgentPermissions map           │             │
│  │  RequirePermission() func       │             │
│  │  RequirePermissionWithSquad()   │             │
│  └──────────┬──────────────────────┘             │
│             │                                    │
│             ▼                                    │
│  ┌──────────────────────┐                        │
│  │  auth.UserFromContext │  (existing)            │
│  │  auth.AgentFromContext│  (existing)            │
│  │  queries.GetSquadMembership │ (existing)       │
│  └──────────────────────┘                        │
└─────────────────────────────────────────────────┘
```

## 2. Permission Matrix

### 2.1 Data Types

```go
// Resource represents a top-level entity type.
type Resource string

const (
    ResourceSquad        Resource = "squad"
    ResourceAgent        Resource = "agent"
    ResourceIssue        Resource = "issue"
    ResourceProject      Resource = "project"
    ResourceGoal         Resource = "goal"
    ResourcePipeline     Resource = "pipeline"
    ResourceInbox        Resource = "inbox"
    ResourceConversation Resource = "conversation"
)

// Action represents an operation on a resource.
type Action string

const (
    ActionCreate  Action = "create"
    ActionRead    Action = "read"
    ActionUpdate  Action = "update"
    ActionDelete  Action = "delete"
    ActionAssign  Action = "assign"
    ActionAdvance Action = "advance"
    ActionReject  Action = "reject"
    ActionResolve Action = "resolve"
)

// PermissionSet is a set of allowed actions for a resource.
type PermissionSet map[Resource]map[Action]bool

// RolePermissions maps role names to their permission sets.
type RolePermissions map[string]PermissionSet
```

### 2.2 User Role Matrix

| Resource      | owner | admin | viewer |
|---------------|-------|-------|--------|
| squad.create  | Y | Y | - |
| squad.read    | Y | Y | Y |
| squad.update  | Y | Y | - |
| squad.delete  | Y | - | - |
| agent.create  | Y | Y | - |
| agent.read    | Y | Y | Y |
| agent.update  | Y | Y | - |
| agent.delete  | Y | Y | - |
| issue.create  | Y | Y | - |
| issue.read    | Y | Y | Y |
| issue.update  | Y | Y | - |
| issue.delete  | Y | Y | - |
| issue.assign  | Y | Y | - |
| issue.advance | Y | Y | - |
| issue.reject  | Y | Y | - |
| issue.resolve | Y | Y | - |
| project.create  | Y | Y | - |
| project.read    | Y | Y | Y |
| project.update  | Y | Y | - |
| project.delete  | Y | Y | - |
| goal.create  | Y | Y | - |
| goal.read    | Y | Y | Y |
| goal.update  | Y | Y | - |
| goal.delete  | Y | Y | - |
| pipeline.create  | Y | Y | - |
| pipeline.read    | Y | Y | Y |
| pipeline.update  | Y | Y | - |
| pipeline.delete  | Y | Y | - |
| inbox.create  | Y | Y | - |
| inbox.read    | Y | Y | Y |
| inbox.update  | Y | Y | - |
| inbox.resolve | Y | Y | - |
| conversation.create  | Y | Y | - |
| conversation.read    | Y | Y | Y |
| conversation.update  | Y | Y | - |

### 2.3 Agent Role Matrix

| Resource      | captain | lead | member |
|---------------|---------|------|--------|
| issue.create  | Y | Y | - |
| issue.read    | Y | Y | Y |
| issue.update  | Y | Y | Y* |
| issue.assign  | Y | Y | - |
| issue.advance | Y | Y | - |
| issue.reject  | Y | Y | - |
| issue.resolve | Y | Y | - |
| agent.read    | Y | Y | Y |
| project.read  | Y | Y | Y |
| goal.read     | Y | Y | Y |
| pipeline.read | Y | Y | Y |
| inbox.create  | Y | Y | Y |
| inbox.read    | Y | Y | Y |
| inbox.resolve | Y | - | - |
| conversation.create | Y | Y | Y |
| conversation.read   | Y | Y | Y |
| conversation.update | Y | Y | - |

`Y*` = member can only update issues assigned to itself (enforced at handler level via `REQ-RBAC-054`).

## 3. Core Permission Package

### 3.1 File: `internal/auth/permissions.go`

```go
package auth

import (
    "context"
    "fmt"

    "github.com/google/uuid"
)

// Resource and Action type definitions (see section 2.1)
// UserPermissions — static map built at init time
// AgentPermissions — static map built at init time

// SquadRoleLookup is a function type for looking up a user's squad membership role.
// This decouples the permission package from the database package.
type SquadRoleLookup func(ctx context.Context, userID, squadID uuid.UUID) (string, error)

// RequirePermission checks whether the caller (from context) is allowed to perform
// the given action on the given resource within the specified squad scope.
//
// For human users: looks up squad membership role via roleLookup, then checks UserPermissions.
// For agents: uses AgentIdentity.Role from context, then checks AgentPermissions.
// For LocalOperator: treats as "owner".
//
// Returns nil if allowed, or a PermissionDeniedError if denied.
func RequirePermission(
    ctx context.Context,
    squadID uuid.UUID,
    resource Resource,
    action Action,
    roleLookup SquadRoleLookup,
) error {
    // 1. Check for AgentIdentity first (agents authenticate via run tokens)
    if agent, ok := AgentFromContext(ctx); ok {
        return checkAgentPermission(agent.Role, resource, action)
    }

    // 2. Check for user Identity
    user, ok := UserFromContext(ctx)
    if !ok {
        return fmt.Errorf("authentication required")
    }

    // 3. LocalOperator = owner
    if user.UserID == uuid.Nil && user.Email == "local@ari.local" {
        return nil // owner has all permissions
    }

    // 4. Look up squad membership role
    role, err := roleLookup(ctx, user.UserID, squadID)
    if err != nil {
        return fmt.Errorf("squad membership required")
    }

    return checkUserPermission(role, resource, action)
}

func checkUserPermission(role string, resource Resource, action Action) error {
    perms, ok := UserPermissions[role]
    if !ok {
        return &PermissionDeniedError{Resource: resource, Action: action}
    }
    actions, ok := perms[resource]
    if !ok {
        return &PermissionDeniedError{Resource: resource, Action: action}
    }
    if !actions[action] {
        return &PermissionDeniedError{Resource: resource, Action: action}
    }
    return nil
}

func checkAgentPermission(role string, resource Resource, action Action) error {
    perms, ok := AgentPermissions[role]
    if !ok {
        return &PermissionDeniedError{Resource: resource, Action: action}
    }
    actions, ok := perms[resource]
    if !ok {
        return &PermissionDeniedError{Resource: resource, Action: action}
    }
    if !actions[action] {
        return &PermissionDeniedError{Resource: resource, Action: action}
    }
    return nil
}

// PermissionDeniedError is returned when a permission check fails.
type PermissionDeniedError struct {
    Resource Resource
    Action   Action
}

func (e *PermissionDeniedError) Error() string {
    return fmt.Sprintf("Permission denied: %s.%s", e.Resource, e.Action)
}

// IsPermissionDenied checks if an error is a PermissionDeniedError.
func IsPermissionDenied(err error) bool {
    _, ok := err.(*PermissionDeniedError)
    return ok
}
```

### 3.2 File: `internal/auth/permission_matrix.go`

Contains the static `UserPermissions` and `AgentPermissions` maps initialized as package-level variables. Uses helper functions to build the maps concisely:

```go
package auth

// allActions is a convenience for granting all standard CRUD actions.
func allActions() map[Action]bool {
    return map[Action]bool{
        ActionCreate: true, ActionRead: true,
        ActionUpdate: true, ActionDelete: true,
        ActionAssign: true, ActionAdvance: true,
        ActionReject: true, ActionResolve: true,
    }
}

// readOnly grants only read access.
func readOnly() map[Action]bool {
    return map[Action]bool{ActionRead: true}
}

// actions builds a map from a variadic list of actions.
func actions(acts ...Action) map[Action]bool {
    m := make(map[Action]bool, len(acts))
    for _, a := range acts {
        m[a] = true
    }
    return m
}

// UserPermissions is the static user role permission matrix.
var UserPermissions = RolePermissions{
    "owner": { /* all resources: allActions() */ },
    "admin": { /* all resources: allActions() except squad has no delete */ },
    "viewer": { /* all resources: readOnly() */ },
}

// AgentPermissions is the static agent role permission matrix.
var AgentPermissions = RolePermissions{
    "captain": { /* per matrix in section 2.3 */ },
    "lead":    { /* per matrix in section 2.3 */ },
    "member":  { /* per matrix in section 2.3 */ },
}
```

## 4. Handler Integration Pattern

### 4.1 Before (Current Pattern)

```go
func (h *IssueHandler) CreateIssue(w http.ResponseWriter, r *http.Request) {
    user, ok := auth.UserFromContext(r.Context())
    // ... check squad membership ...
    _, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
        UserID: user.UserID, SquadID: squadID,
    })
    if err != nil { /* 403 */ }
    // proceed with creation
}
```

### 4.2 After (With Permission Check)

```go
func (h *IssueHandler) CreateIssue(w http.ResponseWriter, r *http.Request) {
    // Squad-scoping check (unchanged — ensures caller belongs to squad)
    // ... existing squad membership check ...

    // Permission check (new — ensures caller's role allows this action)
    if err := auth.RequirePermission(r.Context(), squadID, auth.ResourceIssue, auth.ActionCreate, h.roleLookup); err != nil {
        if auth.IsPermissionDenied(err) {
            writeJSON(w, http.StatusForbidden, errorResponse{Error: err.Error(), Code: "FORBIDDEN"})
            return
        }
        writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
        return
    }
    // proceed with creation
}
```

### 4.3 roleLookup Adapter

Each handler provides a `roleLookup` function that wraps the existing `GetSquadMembership` query:

```go
func (h *IssueHandler) roleLookup(ctx context.Context, userID, squadID uuid.UUID) (string, error) {
    m, err := h.queries.GetSquadMembership(ctx, db.GetSquadMembershipParams{
        UserID: userID, SquadID: squadID,
    })
    if err != nil {
        return "", err
    }
    return m.Role, nil
}
```

This keeps the permission package decoupled from the database package.

## 5. Agent Self-Scoping (Member Issue Guard)

For `REQ-RBAC-054`, member agents can only update issues assigned to them. This is enforced at the handler level after the permission check passes:

```go
// In agent self-service handler, after requirePermission succeeds for member+issue.update:
if agent.Role == "member" {
    issue, _ := h.queries.GetIssueByID(ctx, issueID)
    if issue.AssigneeAgentID == nil || *issue.AssigneeAgentID != agent.AgentID {
        // 403: member can only update assigned issues
    }
}
```

## 6. Permission Matrix API

### `GET /api/permissions`

Returns the full matrix as JSON. No squad scope required (the matrix is global).

```go
func (h *PermissionHandler) GetPermissions(w http.ResponseWriter, r *http.Request) {
    response := PermissionMatrixResponse{
        UserRoles:  formatPermissions(auth.UserPermissions),
        AgentRoles: formatPermissions(auth.AgentPermissions),
    }
    writeJSON(w, http.StatusOK, response)
}
```

Response shape:
```json
{
  "userRoles": {
    "owner": {
      "squad": ["create","read","update","delete"],
      "agent": ["create","read","update","delete"],
      "issue": ["create","read","update","delete","assign","advance","reject","resolve"]
    },
    "admin": {
      "squad": ["create","read","update"],
      "issue": ["create","read","update","delete","assign","advance","reject","resolve"]
    },
    "viewer": {
      "squad": ["read"],
      "issue": ["read"]
    }
  },
  "agentRoles": {
    "captain": { ... },
    "lead": { ... },
    "member": { ... }
  }
}
```

## 7. React UI: Permissions Page

### Route: `/settings/permissions`

A read-only page with two sections:

1. **User Roles** — a table with columns [Resource, owner, admin, viewer] and rows for each (resource, action). Cells show checkmark or dash.
2. **Agent Roles** — a table with columns [Resource, captain, lead, member] and rows for each (resource, action). Cells show checkmark or dash.

Data is fetched from `GET /api/permissions` on mount.

### Component Hierarchy

```
PermissionsPage
├── PermissionMatrix (section="User Roles", data=userRoles)
│   └── PermissionRow (resource, action, role → allowed?)
└── PermissionMatrix (section="Agent Roles", data=agentRoles)
    └── PermissionRow (resource, action, role → allowed?)
```

## 8. Migration Strategy

This feature modifies no database schema. The rollout is:

1. Add permission matrix and `RequirePermission` function to `internal/auth/`
2. Add `GET /api/permissions` endpoint
3. Incrementally add `RequirePermission` calls to each handler (one handler per commit)
4. Add React UI page
5. Run full test suite to ensure no regressions

### Backward Compatibility

- In `local_trusted` mode, LocalOperator is treated as owner — no behavior change
- In `authenticated` mode, existing owner/admin users see no change (they already have all permissions)
- Viewer users will experience new restrictions (read-only enforcement)
- Agent endpoints that were previously unrestricted by role will now enforce the agent matrix

## 9. Testing Strategy

### Unit Tests

- `internal/auth/permissions_test.go` — test every (role, resource, action) combination in both matrices
- `internal/auth/permission_matrix_test.go` — test helper functions and matrix completeness
- Test `RequirePermission` with mock `SquadRoleLookup` for each identity type

### Integration Tests

- Test each handler with viewer role (expect 403 on write operations)
- Test each handler with admin role (expect 403 only on squad delete)
- Test agent endpoints with member role (expect 403 on create issue, advance, reject)
- Test agent endpoints with captain role (expect success on all agent-allowed operations)
- Test `local_trusted` mode (expect owner-level access)

### Edge Cases

- Missing identity in context (expect 401)
- User who is a member of one squad but not another (expect 403 on wrong squad)
- Agent with `member` role trying to update an issue not assigned to them (expect 403)

---

## 10. Files to Create/Modify

### New Files

| File | Purpose |
|------|---------|
| `internal/auth/permissions.go` | Resource, Action types, RequirePermission function, PermissionDeniedError |
| `internal/auth/permission_matrix.go` | Static UserPermissions and AgentPermissions maps |
| `internal/auth/permissions_test.go` | Unit tests for permission checks |
| `internal/auth/permission_matrix_test.go` | Matrix completeness and helper tests |
| `internal/server/handlers/permission_handler.go` | GET /api/permissions endpoint |
| `internal/server/handlers/permission_handler_test.go` | Handler tests |
| `web/src/pages/PermissionsPage.tsx` | React permission matrix page |
| `web/src/hooks/usePermissions.ts` | API client hook for permissions |

### Modified Files

| File | Change |
|------|--------|
| `internal/server/handlers/issue_handler.go` | Add requirePermission calls |
| `internal/server/handlers/agent_handler.go` | Add requirePermission calls |
| `internal/server/handlers/squad_handler.go` | Add requirePermission calls |
| `internal/server/handlers/project_handler.go` | Add requirePermission calls |
| `internal/server/handlers/goal_handler.go` | Add requirePermission calls |
| `internal/server/handlers/pipeline_handler.go` | Add requirePermission calls |
| `internal/server/handlers/inbox_handler.go` | Add requirePermission calls |
| `internal/server/handlers/conversation_handler.go` | Add requirePermission calls |
| `internal/server/handlers/membership_handler.go` | Add requirePermission calls |
| `internal/server/handlers/runtime_handler.go` | Add requirePermission calls for agent endpoints |
| `internal/server/handlers/agent_self_handler.go` | Add agent role scoping (member self-check) |
| `cmd/ari/run.go` or `internal/server/server.go` | Wire PermissionHandler, register route |
| `web/src/App.tsx` | Add route for /settings/permissions |
| Sidebar/nav component | Add permissions link |
