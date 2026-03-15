# Tasks: Permission Grants (RBAC)

**Feature:** 17-permission-grants
**Created:** 2026-03-15
**Status:** Pending

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-RBAC-001 through REQ-RBAC-073, REQ-RBAC-NF-001 through REQ-RBAC-NF-004

## Implementation Approach

Work bottom-up: define the permission types and static matrix first, then the `RequirePermission` enforcement function, then integrate into each handler group incrementally, then add the API endpoint and React UI. Each task follows the Red-Green-Refactor TDD cycle. No database migrations are needed — this is pure Go code and handler changes.

## Progress Summary

- Total Tasks: 10
- Completed: 0/10
- In Progress: None

---

## Tasks (TDD: Red-Green-Refactor)

---

### [ ] Task 01 — Permission Types and Static Matrix Definition

**Requirements:** REQ-RBAC-001, REQ-RBAC-002, REQ-RBAC-003, REQ-RBAC-004, REQ-RBAC-005, REQ-RBAC-010, REQ-RBAC-011, REQ-RBAC-012, REQ-RBAC-013, REQ-RBAC-020 through REQ-RBAC-029, REQ-RBAC-NF-002, REQ-RBAC-NF-004
**Estimated time:** 45 min

#### Context

Define the `Resource`, `Action`, `PermissionSet`, and `RolePermissions` types. Build the static `UserPermissions` and `AgentPermissions` maps with all (role, resource, action) tuples from the design matrix. Use helper functions (`allActions`, `readOnly`, `actions`) for concise map construction. This is the foundation all enforcement relies on.

#### RED — Write Failing Tests

Write `internal/auth/permission_matrix_test.go`:

1. `TestUserPermissions_OwnerHasAllActions` — verify owner has all actions on all resources.
2. `TestUserPermissions_AdminHasAllExceptSquadDelete` — verify admin has all actions on all resources except `squad.delete`.
3. `TestUserPermissions_ViewerReadOnly` — verify viewer has `read` on all resources, denied on all other actions.
4. `TestAgentPermissions_CaptainMatrix` — verify captain permissions match design matrix section 2.3.
5. `TestAgentPermissions_LeadMatrix` — verify lead permissions match design matrix section 2.3.
6. `TestAgentPermissions_MemberMatrix` — verify member permissions match design matrix section 2.3 (read on most, limited create/update).
7. `TestPermissionMatrix_DefaultDeny` — verify unknown role returns deny, unknown resource returns deny, unknown action returns deny.
8. `TestPermissionMatrix_AllResourcesCovered` — verify every resource in the `Resource` constants list appears in every role's permission set.
9. `TestHelpers_AllActions` — verify `allActions()` returns all 8 action types.
10. `TestHelpers_ReadOnly` — verify `readOnly()` returns only `ActionRead`.
11. `TestHelpers_Actions` — verify `actions(ActionCreate, ActionRead)` returns exactly those two.

#### GREEN — Implement

Create `internal/auth/permission_matrix.go`:

- `Resource` type and constants: `ResourceSquad`, `ResourceAgent`, `ResourceIssue`, `ResourceProject`, `ResourceGoal`, `ResourcePipeline`, `ResourceInbox`, `ResourceConversation`
- `Action` type and constants: `ActionCreate`, `ActionRead`, `ActionUpdate`, `ActionDelete`, `ActionAssign`, `ActionAdvance`, `ActionReject`, `ActionResolve`
- `PermissionSet` type: `map[Resource]map[Action]bool`
- `RolePermissions` type: `map[string]PermissionSet`
- Helper functions: `allActions()`, `readOnly()`, `actions(...Action)`
- `UserPermissions` variable with owner/admin/viewer maps per design section 2.2
- `AgentPermissions` variable with captain/lead/member maps per design section 2.3
- `AllResources` slice and `AllActions` slice for iteration/validation

#### REFACTOR

Ensure constants are exported, maps are immutable (populated at init, never modified), and the package has no external dependencies beyond stdlib.

#### Files

- Create: `internal/auth/permission_matrix.go`
- Create: `internal/auth/permission_matrix_test.go`

---

### [ ] Task 02 — RequirePermission Enforcement Function

**Requirements:** REQ-RBAC-030, REQ-RBAC-031, REQ-RBAC-032, REQ-RBAC-033, REQ-RBAC-034, REQ-RBAC-035, REQ-RBAC-NF-001, REQ-RBAC-NF-003
**Estimated time:** 45 min

#### Context

Implement the `RequirePermission` function that extracts the caller from context, determines their role, and checks the permission matrix. This function is the single enforcement point called by all handlers. It uses a `SquadRoleLookup` function type to decouple from the database package. Also implement `PermissionDeniedError` for structured error reporting.

#### RED — Write Failing Tests

Write `internal/auth/permissions_test.go`:

1. `TestRequirePermission_UserOwner_Allowed` — mock roleLookup returning "owner", verify nil error for any (resource, action).
2. `TestRequirePermission_UserAdmin_AllowedExceptSquadDelete` — verify nil for most operations, error for squad.delete.
3. `TestRequirePermission_UserViewer_ReadOnly` — verify nil for read, error for create/update/delete.
4. `TestRequirePermission_AgentCaptain_Allowed` — inject AgentIdentity with role "captain", verify allowed operations.
5. `TestRequirePermission_AgentMember_LimitedAccess` — inject AgentIdentity with role "member", verify denied for create issue, advance, reject.
6. `TestRequirePermission_LocalOperator_TreatedAsOwner` — inject LocalOperatorIdentity, verify nil for all operations (no roleLookup called).
7. `TestRequirePermission_NoIdentity_AuthError` — empty context, verify authentication error.
8. `TestRequirePermission_UserNotInSquad_Error` — roleLookup returns error, verify squad membership error.
9. `TestRequirePermission_AgentTakesPrecedence` — context with both Identity and AgentIdentity, verify agent path taken.
10. `TestPermissionDeniedError_Message` — verify error message format "Permission denied: issue.create".
11. `TestIsPermissionDenied_True` — verify `IsPermissionDenied` returns true for `PermissionDeniedError`.
12. `TestIsPermissionDenied_False` — verify `IsPermissionDenied` returns false for other errors.
13. `TestRequirePermission_ConcurrentSafe` — run 100 goroutines calling RequirePermission simultaneously, verify no races.

#### GREEN — Implement

Create `internal/auth/permissions.go`:

- `SquadRoleLookup` function type: `func(ctx context.Context, userID, squadID uuid.UUID) (string, error)`
- `PermissionDeniedError` struct with `Resource` and `Action` fields, implementing `error` interface
- `IsPermissionDenied(err error) bool` helper
- `RequirePermission(ctx, squadID, resource, action, roleLookup) error` — the main enforcement function per design section 3.1
- `checkUserPermission(role, resource, action) error` — internal helper
- `checkAgentPermission(role, resource, action) error` — internal helper

#### REFACTOR

Ensure `RequirePermission` has no allocations in the happy path (map lookups only). Add godoc comments.

#### Files

- Create: `internal/auth/permissions.go`
- Create: `internal/auth/permissions_test.go`

---

### [ ] Task 03 — Handler Integration: Squad and Membership Handlers

**Requirements:** REQ-RBAC-040, REQ-RBAC-041, REQ-RBAC-042, REQ-RBAC-043, REQ-RBAC-010, REQ-RBAC-011, REQ-RBAC-012
**Estimated time:** 45 min

#### Context

Add `requirePermission` calls to `SquadHandler` and `MembershipHandler`. These are the highest-risk handlers because squad.delete is the one action admin cannot perform. The existing squad membership check remains for isolation; the permission check is layered on top.

Add a `roleLookup` method to each handler that wraps `GetSquadMembership`.

#### RED — Write Failing Tests

Extend handler tests:

1. `TestSquadHandler_UpdateSquad_ViewerDenied` — viewer tries PATCH, expect 403.
2. `TestSquadHandler_DeleteSquad_AdminDenied` — admin tries DELETE, expect 403.
3. `TestSquadHandler_DeleteSquad_OwnerAllowed` — owner tries DELETE, expect 204.
4. `TestSquadHandler_ReadSquad_ViewerAllowed` — viewer tries GET, expect 200.
5. `TestMembershipHandler_UpdateRole_ViewerDenied` — viewer tries PATCH membership, expect 403.
6. `TestMembershipHandler_RemoveMember_ViewerDenied` — viewer tries DELETE membership, expect 403.
7. `TestMembershipHandler_ListMembers_ViewerAllowed` — viewer tries GET members, expect 200.

#### GREEN — Implement

Modify `internal/server/handlers/squad_handler.go`:
- Add `roleLookup` method
- Add `auth.RequirePermission(ctx, squadID, auth.ResourceSquad, auth.ActionUpdate, h.roleLookup)` to UpdateSquad
- Add `auth.RequirePermission(ctx, squadID, auth.ResourceSquad, auth.ActionDelete, h.roleLookup)` to DeleteSquad
- Add `auth.RequirePermission(ctx, squadID, auth.ResourceSquad, auth.ActionRead, h.roleLookup)` to GetSquad/ListSquads

Modify `internal/server/handlers/membership_handler.go`:
- Add `roleLookup` method
- Add permission checks to UpdateRole, RemoveMember, InviteMember (squad.update action)
- Add permission check to ListMembers (squad.read action)

#### Files

- Modify: `internal/server/handlers/squad_handler.go`
- Modify: `internal/server/handlers/membership_handler.go`
- Modify: `internal/server/handlers/squad_handler_test.go`
- Modify: `internal/server/handlers/membership_handler_test.go`

---

### [ ] Task 04 — Handler Integration: Issue and Pipeline Handlers

**Requirements:** REQ-RBAC-040, REQ-RBAC-041, REQ-RBAC-042, REQ-RBAC-050, REQ-RBAC-052, REQ-RBAC-053
**Estimated time:** 45 min

#### Context

Add `requirePermission` calls to `IssueHandler` and `PipelineHandler`. Issue handlers have the most diverse action set (create, read, update, delete, assign, advance, reject). Pipeline handlers map to create/read/update/delete plus advance/reject for stage transitions.

#### RED — Write Failing Tests

Extend handler tests:

1. `TestIssueHandler_CreateIssue_ViewerDenied` — viewer tries POST, expect 403.
2. `TestIssueHandler_UpdateIssue_ViewerDenied` — viewer tries PATCH, expect 403.
3. `TestIssueHandler_ReadIssue_ViewerAllowed` — viewer tries GET, expect 200.
4. `TestIssueHandler_AdvanceIssue_ViewerDenied` — viewer tries POST advance, expect 403.
5. `TestIssueHandler_RejectIssue_ViewerDenied` — viewer tries POST reject, expect 403.
6. `TestPipelineHandler_CreatePipeline_ViewerDenied` — viewer tries POST, expect 403.
7. `TestPipelineHandler_DeletePipeline_ViewerDenied` — viewer tries DELETE, expect 403.
8. `TestPipelineHandler_ListPipelines_ViewerAllowed` — viewer tries GET, expect 200.

#### GREEN — Implement

Modify `internal/server/handlers/issue_handler.go`:
- Add `roleLookup` method
- Add permission checks: CreateIssue → `issue.create`, GetIssue/ListIssues → `issue.read`, UpdateIssue → `issue.update`, DeleteIssue → `issue.delete`, AdvanceIssue → `issue.advance`, RejectIssue → `issue.reject`

Modify `internal/server/handlers/pipeline_handler.go`:
- Add `roleLookup` method
- Add permission checks: CreatePipeline → `pipeline.create`, ListPipelines/GetPipeline → `pipeline.read`, UpdatePipeline → `pipeline.update`, DeletePipeline → `pipeline.delete`, AdvanceIssue → `issue.advance`, RejectIssue → `issue.reject`

#### Files

- Modify: `internal/server/handlers/issue_handler.go`
- Modify: `internal/server/handlers/pipeline_handler.go`
- Modify: `internal/server/handlers/issue_handler_test.go`
- Modify: `internal/server/handlers/pipeline_handler_test.go`

---

### [ ] Task 05 — Handler Integration: Agent, Project, Goal, Inbox, Conversation Handlers

**Requirements:** REQ-RBAC-040, REQ-RBAC-041, REQ-RBAC-042
**Estimated time:** 60 min

#### Context

Add `requirePermission` calls to all remaining handlers. This is the broadest task but each handler follows the same pattern: add `roleLookup` method, add permission check before each operation.

#### RED — Write Failing Tests

Extend handler tests (one or two per handler for coverage):

1. `TestAgentHandler_CreateAgent_ViewerDenied` — viewer tries POST, expect 403.
2. `TestAgentHandler_ListAgents_ViewerAllowed` — viewer tries GET, expect 200.
3. `TestProjectHandler_CreateProject_ViewerDenied` — viewer tries POST, expect 403.
4. `TestProjectHandler_ListProjects_ViewerAllowed` — viewer tries GET, expect 200.
5. `TestGoalHandler_CreateGoal_ViewerDenied` — viewer tries POST, expect 403.
6. `TestGoalHandler_ListGoals_ViewerAllowed` — viewer tries GET, expect 200.
7. `TestInboxHandler_ResolveItem_ViewerDenied` — viewer tries PATCH resolve, expect 403.
8. `TestInboxHandler_ListItems_ViewerAllowed` — viewer tries GET, expect 200.
9. `TestConversationHandler_CreateConversation_ViewerDenied` — viewer tries POST, expect 403.
10. `TestConversationHandler_ListConversations_ViewerAllowed` — viewer tries GET, expect 200.

#### GREEN — Implement

Modify each handler file:
- `internal/server/handlers/agent_handler.go` — agent.create/read/update/delete
- `internal/server/handlers/project_handler.go` — project.create/read/update/delete
- `internal/server/handlers/goal_handler.go` — goal.create/read/update/delete
- `internal/server/handlers/inbox_handler.go` — inbox.create/read/update/resolve
- `internal/server/handlers/conversation_handler.go` — conversation.create/read/update

Each handler: add `roleLookup` method, add `auth.RequirePermission(...)` call before each operation.

#### Files

- Modify: `internal/server/handlers/agent_handler.go`
- Modify: `internal/server/handlers/project_handler.go`
- Modify: `internal/server/handlers/goal_handler.go`
- Modify: `internal/server/handlers/inbox_handler.go`
- Modify: `internal/server/handlers/conversation_handler.go`
- Modify: corresponding `*_test.go` files

---

### [ ] Task 06 — Agent Role Scoping: Self-Service Endpoint Enforcement

**Requirements:** REQ-RBAC-050, REQ-RBAC-051, REQ-RBAC-052, REQ-RBAC-053, REQ-RBAC-054
**Estimated time:** 45 min

#### Context

Agent self-service endpoints (`/api/agent/me/*`) are called by agents authenticated via run tokens. Apply the agent permission matrix to these endpoints. Additionally, enforce `REQ-RBAC-054`: member agents can only update issues assigned to themselves.

#### RED — Write Failing Tests

Write/extend `internal/server/handlers/agent_self_handler_test.go` (or runtime_handler_test.go):

1. `TestAgentSelf_CaptainCreateIssue_Allowed` — captain calls POST create issue, expect 201.
2. `TestAgentSelf_MemberCreateIssue_Denied` — member calls POST create issue, expect 403.
3. `TestAgentSelf_LeadCreateIssue_Allowed` — lead calls POST create issue, expect 201.
4. `TestAgentSelf_MemberUpdateAssignedIssue_Allowed` — member updates issue assigned to itself, expect 200.
5. `TestAgentSelf_MemberUpdateUnassignedIssue_Denied` — member updates issue NOT assigned to itself, expect 403.
6. `TestAgentSelf_CaptainAdvanceIssue_Allowed` — captain calls advance, expect 200.
7. `TestAgentSelf_MemberAdvanceIssue_Denied` — member calls advance, expect 403.
8. `TestAgentSelf_MemberReadIssue_Allowed` — member calls GET issue, expect 200.
9. `TestAgentSelf_MemberCreateInboxItem_Allowed` — member calls POST inbox (ask human), expect 201.
10. `TestAgentSelf_MemberResolveInboxItem_Denied` — member calls resolve inbox, expect 403.

#### GREEN — Implement

Modify `internal/server/handlers/runtime_handler.go` (or agent self-service handler):
- Add `auth.RequirePermission(ctx, agent.SquadID, resource, action, nil)` calls — for agents, `roleLookup` is nil since role comes from `AgentIdentity`
- For member issue.update: after permission check passes, verify `issue.AssigneeAgentID == agent.AgentID`
- Return 403 with `FORBIDDEN` code and descriptive message for denied operations

#### Files

- Modify: `internal/server/handlers/runtime_handler.go` (or agent self-service handler)
- Modify: corresponding test file

---

### [ ] Task 07 — Permission Matrix API Endpoint

**Requirements:** REQ-RBAC-060, REQ-RBAC-061, REQ-RBAC-062
**Estimated time:** 30 min

#### Context

Add a `GET /api/permissions` endpoint that returns the full permission matrix as JSON. Any authenticated user or agent can call it. The handler reads the static Go maps and formats them into the response shape defined in the requirements.

#### RED — Write Failing Tests

Write `internal/server/handlers/permission_handler_test.go`:

1. `TestGetPermissions_Authenticated` — authenticated user calls GET, verify 200 and response shape matches REQ-RBAC-062.
2. `TestGetPermissions_Unauthenticated` — no auth, verify 401.
3. `TestGetPermissions_ResponseShape` — verify response contains `userRoles` and `agentRoles` keys, each containing role maps with resource arrays.
4. `TestGetPermissions_OwnerHasAllActions` — verify owner entry in response contains all actions for all resources.
5. `TestGetPermissions_ViewerReadOnly` — verify viewer entry contains only "read" for each resource.
6. `TestGetPermissions_AgentRolesPresent` — verify captain, lead, member entries exist in agentRoles.

#### GREEN — Implement

Create `internal/server/handlers/permission_handler.go`:

- `PermissionHandler` struct (no dependencies needed — reads static maps)
- `NewPermissionHandler()` constructor
- `RegisterRoutes(mux)` — register `GET /api/permissions`
- `GetPermissions(w, r)` — format `auth.UserPermissions` and `auth.AgentPermissions` into JSON response
- `PermissionMatrixResponse` struct with `UserRoles` and `AgentRoles` fields
- `formatPermissions(RolePermissions) map[string]map[string][]string` — converts internal maps to JSON-friendly format (resource → sorted action list)

#### Files

- Create: `internal/server/handlers/permission_handler.go`
- Create: `internal/server/handlers/permission_handler_test.go`

---

### [ ] Task 08 — Server Wiring: Register Permission Handler

**Requirements:** All (integration)
**Estimated time:** 15 min

#### Context

Wire the `PermissionHandler` into server initialization. Register the `/api/permissions` route on the HTTP mux.

#### RED — Write Failing Tests

Write a smoke test:

1. Start the full server, call `GET /api/permissions` with valid auth, verify 200 response.

#### GREEN — Implement

Modify server initialization (`cmd/ari/run.go` or `internal/server/server.go`):

- Create `PermissionHandler` and call `RegisterRoutes(mux)`

#### Files

- Modify: `cmd/ari/run.go` or `internal/server/server.go`

---

### [ ] Task 09 — React UI: Permissions Page

**Requirements:** REQ-RBAC-070, REQ-RBAC-071, REQ-RBAC-072, REQ-RBAC-073
**Estimated time:** 60 min

#### Context

Build the React permissions page that displays the permission matrix as a read-only table. Fetch data from `GET /api/permissions`. The page has two sections: User Roles and Agent Roles. Each section is a grid with resources/actions as rows and roles as columns. Cells show a checkmark (allowed) or dash (denied).

#### RED — Write Failing Tests

(Frontend testing — verify component rendering):

1. `PermissionsPage` renders loading state, then two matrix sections after fetch.
2. `PermissionMatrix` renders correct number of rows (resources x actions) and columns (roles).
3. Cells show checkmark for allowed, dash for denied.
4. Page is accessible from settings navigation.

#### GREEN — Implement

Create React components:

- `web/src/pages/PermissionsPage.tsx` — main page with two `PermissionMatrix` sections
- `web/src/components/permissions/PermissionMatrix.tsx` — reusable table component accepting role permission data
- `web/src/hooks/usePermissions.ts` — `useSWR` or `useEffect` hook fetching `GET /api/permissions`

Add route to React router and navigation:
- Add `/settings/permissions` route in `App.tsx`
- Add "Permissions" link in settings/sidebar navigation

#### Files

- Create: `web/src/pages/PermissionsPage.tsx`
- Create: `web/src/components/permissions/PermissionMatrix.tsx`
- Create: `web/src/hooks/usePermissions.ts`
- Modify: `web/src/App.tsx` (add route)
- Modify: sidebar/nav component (add permissions link)

---

### [ ] Task 10 — Integration Tests: Full RBAC Flow

**Requirements:** All requirements (end-to-end coverage)
**Estimated time:** 60 min

#### Context

Write comprehensive integration tests covering the full RBAC lifecycle: user role enforcement across all handlers, agent role scoping, member self-restriction, local_trusted mode, and the permissions API. These tests run against the full server with embedded DB.

#### RED — Write Failing Tests

Create `internal/server/handlers/permission_integration_test.go`:

1. `TestRBAC_ViewerCannotCreateIssue` — create squad with viewer member, attempt POST issue, expect 403.
2. `TestRBAC_ViewerCanReadIssue` — viewer calls GET issue, expect 200.
3. `TestRBAC_AdminCannotDeleteSquad` — admin calls DELETE squad, expect 403.
4. `TestRBAC_AdminCanDeleteAgent` — admin calls DELETE agent, expect 204.
5. `TestRBAC_OwnerCanDeleteSquad` — owner calls DELETE squad, expect 204.
6. `TestRBAC_AgentCaptainCanCreateIssue` — captain agent calls create issue, expect 201.
7. `TestRBAC_AgentMemberCannotCreateIssue` — member agent calls create issue, expect 403.
8. `TestRBAC_AgentMemberCanUpdateAssignedIssue` — member updates own assigned issue, expect 200.
9. `TestRBAC_AgentMemberCannotUpdateOtherIssue` — member updates unassigned issue, expect 403.
10. `TestRBAC_AgentMemberCannotAdvanceIssue` — member calls advance, expect 403.
11. `TestRBAC_AgentLeadCanAdvanceIssue` — lead calls advance, expect 200.
12. `TestRBAC_LocalTrustedMode_OwnerAccess` — LocalOperator identity has full access.
13. `TestRBAC_PermissionsEndpoint_ReturnsMatrix` — GET /api/permissions returns valid matrix JSON.
14. `TestRBAC_CrossSquadDenied` — user in squad A cannot access squad B resources even as owner of squad A.

#### GREEN — Implement

Run all tests against implementations from Tasks 01-09 and verify they pass.

#### REFACTOR

Review test coverage, add edge cases if needed, ensure all requirements are exercised.

#### Files

- Create: `internal/server/handlers/permission_integration_test.go`

---

## Requirement Coverage Matrix

| Requirement | Task(s) |
|-------------|---------|
| REQ-RBAC-001 | Task 01 |
| REQ-RBAC-002 | Task 01 |
| REQ-RBAC-003 | Task 01 |
| REQ-RBAC-004 | Task 01 |
| REQ-RBAC-005 | Task 01, Task 02 |
| REQ-RBAC-010 | Task 01, Task 03 |
| REQ-RBAC-011 | Task 01, Task 03 |
| REQ-RBAC-012 | Task 01, Task 03 |
| REQ-RBAC-013 | Task 01, Task 03 |
| REQ-RBAC-020 | Task 01, Task 06 |
| REQ-RBAC-021 | Task 01, Task 06 |
| REQ-RBAC-022 | Task 01, Task 06 |
| REQ-RBAC-023 | Task 01, Task 06 |
| REQ-RBAC-024 | Task 01, Task 06 |
| REQ-RBAC-025 | Task 01, Task 06 |
| REQ-RBAC-026 | Task 01, Task 06 |
| REQ-RBAC-027 | Task 01, Task 06 |
| REQ-RBAC-028 | Task 01, Task 06 |
| REQ-RBAC-029 | Task 01, Task 06 |
| REQ-RBAC-030 | Task 02 |
| REQ-RBAC-031 | Task 02 |
| REQ-RBAC-032 | Task 02 |
| REQ-RBAC-033 | Task 02 |
| REQ-RBAC-034 | Task 02 |
| REQ-RBAC-035 | Task 02 |
| REQ-RBAC-040 | Task 03, Task 04, Task 05 |
| REQ-RBAC-041 | Task 03, Task 04, Task 05 |
| REQ-RBAC-042 | Task 03, Task 04, Task 05 |
| REQ-RBAC-043 | Task 03, Task 04, Task 05 |
| REQ-RBAC-050 | Task 06 |
| REQ-RBAC-051 | Task 06 |
| REQ-RBAC-052 | Task 04, Task 06 |
| REQ-RBAC-053 | Task 04, Task 06 |
| REQ-RBAC-054 | Task 06 |
| REQ-RBAC-060 | Task 07 |
| REQ-RBAC-061 | Task 07 |
| REQ-RBAC-062 | Task 07 |
| REQ-RBAC-070 | Task 09 |
| REQ-RBAC-071 | Task 09 |
| REQ-RBAC-072 | Task 09 |
| REQ-RBAC-073 | Task 09 |
| REQ-RBAC-NF-001 | Task 02 |
| REQ-RBAC-NF-002 | Task 01 |
| REQ-RBAC-NF-003 | Task 02 |
| REQ-RBAC-NF-004 | Task 01 |
