# Requirements: UI Interactivity

**Created:** 2026-03-15
**Status:** Draft

## Overview

Make the React frontend fully interactive. Currently all pages render read-only data — every "Create" button is a no-op, detail pages have no editing, there's no squad switching, no filtering, no pagination, and no status transitions. This feature wires all UI controls to the existing backend API to make every page functional.

### Requirement ID Format

- Use sequential IDs: `REQ-001`, `REQ-002`, etc.
- Keep numbering continuous across all requirement categories.

---

## Functional Requirements

### Event-Driven Requirements

Events that trigger system behavior. Use format: "WHEN [trigger] THEN the system SHALL [response]"

#### Squad Selector

- [REQ-001] WHEN the user selects a different squad from the squad selector in `Sidebar`, THEN the system SHALL update the active squad context, update all squad-scoped nav links (Agents, Issues, Projects, Goals) to reference the new squad ID, and invalidate all squad-scoped queries so they refetch for the newly selected squad.
- [REQ-002] WHEN the active squad context changes, THEN the system SHALL persist the selected squad ID to `localStorage` under the key `ari:activeSquadId` so the selection survives page refresh.

#### Create Button Clicks

- [REQ-003] WHEN the user clicks "Create Squad" on `SquadListPage`, THEN the system SHALL open a `Dialog` containing a form with fields for `name` (required), `issuePrefix` (required), and `description` (optional).
- [REQ-004] WHEN the user clicks "Create Agent" on `AgentListPage` or the "New Agent" quick-action button on `DashboardPage`, THEN the system SHALL open a `Dialog` containing a form with fields for `name` (required), `urlKey` (required), `role` (required, select), `title` (optional), `reportsTo` (optional, select populated from existing agents), `adapterType` (optional), and `capabilities` (optional).
- [REQ-005] WHEN the user clicks "Create Issue" on `IssueListPage` or the "New Issue" quick-action button on `DashboardPage`, THEN the system SHALL open a `Dialog` containing a form with fields for `title` (required), `description` (optional), `type` (optional, select: task/conversation), `status` (optional, select), `priority` (optional, select), `assigneeAgentId` (optional, select populated from squad agents), `projectId` (optional, select populated from squad projects), and `goalId` (optional, select populated from squad goals).
- [REQ-006] WHEN the user clicks "Create Project" on `ProjectListPage` or the "New Project" quick-action button on `DashboardPage`, THEN the system SHALL open a `Dialog` containing a form with fields for `name` (required) and `description` (optional).
- [REQ-007] WHEN the user clicks "Create Goal" on `GoalListPage`, THEN the system SHALL open a `Dialog` containing a form with fields for `title` (required), `description` (optional), and `parentId` (optional, select populated from existing squad goals).

#### Form Submissions — Create

- [REQ-008] WHEN the user submits the Create Squad form with valid data, THEN the system SHALL call `POST /api/squads` with a `CreateSquadRequest` body, close the dialog on success, invalidate the `queryKeys.squads.all` query, display a success toast with the message "Squad created", and add the new squad to the user's auth context so it appears in the squad selector immediately.
- [REQ-009] WHEN the user submits the Create Agent form with valid data, THEN the system SHALL call `POST /api/agents` with a `CreateAgentRequest` body (including `squadId` from the active squad), close the dialog on success, invalidate `queryKeys.agents.list(squadId)`, and display a success toast with the message "Agent created".
- [REQ-010] WHEN the user submits the Create Issue form with valid data, THEN the system SHALL call `POST /api/squads/{squadId}/issues` with a `CreateIssueRequest` body, close the dialog on success, invalidate `queryKeys.issues.list(squadId)`, and display a success toast with the message "Issue created".
- [REQ-011] WHEN the user submits the Create Project form with valid data, THEN the system SHALL call `POST /api/squads/{squadId}/projects` with a `CreateProjectRequest` body, close the dialog on success, invalidate `queryKeys.projects.list(squadId)`, and display a success toast with the message "Project created".
- [REQ-012] WHEN the user submits the Create Goal form with valid data, THEN the system SHALL call `POST /api/squads/{squadId}/goals` with a `CreateGoalRequest` body, close the dialog on success, invalidate `queryKeys.goals.list(squadId)`, and display a success toast with the message "Goal created".

#### Form Submissions — Edit

- [REQ-013] WHEN the user submits the edit form on `SquadDetailPage`, THEN the system SHALL call `PATCH /api/squads/{id}` with an `UpdateSquadRequest` body, exit edit mode on success, invalidate both `queryKeys.squads.detail(id)` and `queryKeys.squads.all`, and display a success toast with the message "Squad updated".
- [REQ-014] WHEN the user submits the edit form on `AgentDetailPage`, THEN the system SHALL call `PATCH /api/agents/{id}` with an `UpdateAgentRequest` body, exit edit mode on success, invalidate both `queryKeys.agents.detail(id)` and `queryKeys.agents.list(squadId)`, and display a success toast with the message "Agent updated".
- [REQ-015] WHEN the user submits the edit form on `IssueDetailPage`, THEN the system SHALL call `PATCH /api/issues/{id}` with an `UpdateIssueRequest` body, exit edit mode on success, invalidate both `queryKeys.issues.detail(id)` and `queryKeys.issues.list(squadId)`, and display a success toast with the message "Issue updated".
- [REQ-016] WHEN the user submits the edit form on `ProjectDetailPage`, THEN the system SHALL call `PATCH /api/projects/{id}` with an `UpdateProjectRequest` body, exit edit mode on success, invalidate both `queryKeys.projects.detail(id)` and `queryKeys.projects.list(squadId)`, and display a success toast with the message "Project updated".
- [REQ-017] WHEN the user submits the edit form on `GoalDetailPage`, THEN the system SHALL call `PATCH /api/goals/{id}` with an `UpdateGoalRequest` body, exit edit mode on success, invalidate both `queryKeys.goals.detail(id)` and `queryKeys.goals.list(squadId)`, and display a success toast with the message "Goal updated".

#### Status Transitions

- [REQ-018] WHEN the user clicks a status transition button on `AgentDetailPage`, THEN the system SHALL call `PATCH /api/agents/{id}` with `{ status: <target_status> }`, invalidate `queryKeys.agents.detail(id)` and `queryKeys.agents.list(squadId)`, and display a success toast with the message "Agent status updated".
- [REQ-019] WHEN the user clicks a status transition button on `IssueDetailPage`, THEN the system SHALL call `PATCH /api/issues/{id}` with `{ status: <target_status> }` where `<target_status>` is a value from `issueStatusTransitions[currentStatus]`, invalidate `queryKeys.issues.detail(id)` and `queryKeys.issues.list(squadId)`, and display a success toast with the message "Issue status updated".

#### Comment Submission

- [REQ-020] WHEN the user submits the comment form on `IssueDetailPage` with a non-empty `body`, THEN the system SHALL call `POST /api/issues/{id}/comments` with `{ body: <text> }`, clear the comment input field on success, invalidate `queryKeys.issues.comments(issueId)`, and display a success toast with the message "Comment added".

#### Filter Changes

- [REQ-021] WHEN the user changes the status filter Select on `IssueListPage`, THEN the system SHALL update the `status` query parameter in the URL, refetch issues from `GET /api/squads/{squadId}/issues?status=<value>`, and update the displayed issue list.
- [REQ-022] WHEN the user changes the priority filter Select on `IssueListPage`, THEN the system SHALL update the `priority` query parameter in the URL, refetch issues from `GET /api/squads/{squadId}/issues?priority=<value>`, and update the displayed issue list.
- [REQ-023] WHEN the user changes the assignee filter Select on `IssueListPage`, THEN the system SHALL update the `assigneeAgentId` query parameter in the URL, refetch issues from `GET /api/squads/{squadId}/issues?assigneeAgentId=<value>`, and update the displayed issue list.
- [REQ-024] WHEN the user clicks the "Clear Filters" button on `IssueListPage`, THEN the system SHALL remove all filter query parameters from the URL and refetch the unfiltered issue list.

#### Pagination

- [REQ-025] WHEN the user clicks "Next Page" on `IssueListPage`, THEN the system SHALL increment the `offset` by the current `limit`, refetch `GET /api/squads/{squadId}/issues?offset=<new_offset>&limit=<limit>`, and update the displayed issue list.
- [REQ-026] WHEN the user clicks "Previous Page" on `IssueListPage`, THEN the system SHALL decrement the `offset` by the current `limit` (clamped to 0), refetch the issue list with the updated offset, and update the displayed issue list.

#### Edit Mode Toggle

- [REQ-027] WHEN the user clicks the "Edit" button on any detail page (`SquadDetailPage`, `AgentDetailPage`, `IssueDetailPage`, `ProjectDetailPage`, `GoalDetailPage`), THEN the system SHALL enter edit mode, replace all read-only display fields with their corresponding form inputs pre-populated with the current entity values, and show "Save" and "Cancel" buttons in place of the "Edit" button.
- [REQ-028] WHEN the user clicks "Cancel" while in edit mode on any detail page, THEN the system SHALL exit edit mode, restore all fields to their original read-only display, and discard any unsaved changes.

#### Logout

- [REQ-029] WHEN the user clicks the logout icon button in the `Sidebar` footer, THEN the system SHALL call `POST /api/auth/logout`, clear the auth context (including the `AuthUser` state), clear the active squad from `localStorage`, clear all TanStack Query cache via `queryClient.clear()`, and redirect to `/login`.

---

### State-Driven Requirements

Behavior during specific system states. Use format: "WHILE [state] the system SHALL [continuous behavior]"

- [REQ-030] WHILE a create or edit form mutation is in flight (`isPending === true`), the system SHALL disable the form's submit button, disable all other form inputs, and render a loading spinner inside the submit button to prevent duplicate submissions.
- [REQ-031] WHILE any list page (`AgentListPage`, `IssueListPage`, `ProjectListPage`, `GoalListPage`, `SquadListPage`) is in a loading state (`isLoading === true`), the system SHALL render skeleton placeholder rows (three rows of muted `h-16` divs) in place of the table body.
- [REQ-032] WHILE any detail page is in a loading state (`isLoading === true`), the system SHALL render a skeleton placeholder (one `h-8 w-48` title block and one `h-32` content block) in place of the entity fields.
- [REQ-033] WHILE the user is in edit mode on a detail page, the system SHALL keep the edit form fields visible and focused, and SHALL NOT navigate away on route changes triggered by sidebar nav clicks unless the user confirms discarding unsaved changes.
- [REQ-034] WHILE a status transition mutation is in flight on `IssueDetailPage` or `AgentDetailPage`, the system SHALL disable all status transition buttons to prevent concurrent transitions.

---

### Ubiquitous Requirements

Always-true requirements. Use format: "The system SHALL [requirement]"

- [REQ-035] The system SHALL display a success toast (variant `"default"`, auto-dismissed after 5 seconds) after every successful mutation (create, update, status transition, comment submission, logout).
- [REQ-036] The system SHALL display an error toast (variant `"destructive"`, auto-dismissed after 5 seconds) after every failed mutation, using the `error` message from the `ApiClientError` or a fallback message of "An unexpected error occurred".
- [REQ-037] The system SHALL call `queryClient.invalidateQueries` with the relevant query key immediately after every successful mutation to ensure list and detail views reflect the latest server state.
- [REQ-038] The system SHALL scope all agent, issue, project, and goal queries to the active squad ID, ensuring that switching squads never surfaces data belonging to a different squad.
- [REQ-039] The system SHALL validate all required form fields (non-empty string, correct type) on the client before submitting a mutation request, and SHALL display inline field-level error messages below each invalid input rather than allowing the API request to proceed with invalid data.
- [REQ-040] The system SHALL pre-populate all edit forms with the current entity values fetched from the TanStack Query cache at the moment the user enters edit mode.
- [REQ-041] The system SHALL use the existing `useToast` hook from `web/src/hooks/use-toast.ts` for all toast notifications, and SHALL NOT introduce a separate toast library.
- [REQ-042] The system SHALL use the existing `api` client from `web/src/lib/api.ts` for all HTTP requests, relying on its built-in 401 redirect and error normalisation.

---

### Conditional Requirements

Behavior based on conditions. Use format: "IF [condition] THEN the system SHALL [requirement]"

- [REQ-043] IF the authenticated user belongs to more than one squad (`user.squads.length > 1`), THEN the system SHALL render a squad selector `Select` component in the `Sidebar` above the squad-scoped navigation items, listing all squads by name.
- [REQ-044] IF the authenticated user belongs to exactly one squad, THEN the system SHALL display the squad name as a static label in the `Sidebar` without a selector control.
- [REQ-045] IF an issue's current status has valid transitions defined in `issueStatusTransitions[issue.status]`, THEN the system SHALL render a row of transition `Button` components on `IssueDetailPage`, one per valid target status, below the current status badge.
- [REQ-046] IF an issue's current status has no valid transitions (empty array in `issueStatusTransitions`), THEN the system SHALL omit the transition button row entirely.
- [REQ-047] IF an agent's status is `"pending_approval"`, THEN the system SHALL render an "Approve" button on `AgentDetailPage` that calls `PATCH /api/agents/{id}` with `{ status: "active" }`.
- [REQ-048] IF an agent's status is `"active"` or `"idle"`, THEN the system SHALL render a "Pause" button on `AgentDetailPage` that calls `PATCH /api/agents/{id}` with `{ status: "paused" }`.
- [REQ-049] IF an agent's status is `"paused"`, THEN the system SHALL render a "Resume" button on `AgentDetailPage` that calls `PATCH /api/agents/{id}` with `{ status: "active" }`.
- [REQ-050] IF the `GET /api/issues/{id}/comments` response returns one or more `IssueComment` items, THEN the system SHALL render the comment list on `IssueDetailPage` in chronological order, displaying `authorName`, `createdAt` (formatted), and `body` for each comment.
- [REQ-051] IF the comment list for an issue is empty, THEN the system SHALL display a "No comments yet" empty state message above the comment input form.
- [REQ-052] IF `PaginatedResponse.pagination.total` exceeds `pagination.limit` on `IssueListPage`, THEN the system SHALL render pagination controls showing the current page range (e.g., "1–20 of 47"), a "Previous" button (disabled on the first page), and a "Next" button (disabled on the last page).
- [REQ-053] IF `PaginatedResponse.pagination.total` is less than or equal to `pagination.limit`, THEN the system SHALL omit pagination controls entirely.
- [REQ-054] IF any active filter is applied on `IssueListPage`, THEN the system SHALL render a "Clear Filters" button that resets all filters to their unset state.
- [REQ-055] IF a `CreateGoalRequest` includes a `parentId`, THEN the system SHALL validate that the parent goal belongs to the same squad before submitting, and SHALL display an inline error if it does not.

---

### Optional Requirements

Feature-dependent requirements. Use format: "WHERE [feature] the system SHALL [requirement]"

- [REQ-056] WHERE a squad has `requireApprovalForNewAgents: true`, the system SHALL display a notice banner on `AgentListPage` reading "New agents require approval before activation" and SHALL render newly created agents with a `pending_approval` status badge.
- [REQ-057] WHERE a squad has `requireApprovalForNewAgents: true` and an agent is in `pending_approval` status, the system SHALL render an "Approve Agent" button on `AgentDetailPage` that transitions the agent to `active` via `PATCH /api/agents/{id}` with `{ status: "active" }`.
- [REQ-058] WHERE a goal has a `parentId`, the system SHALL display the parent goal's title as a linked breadcrumb above the goal's title on `GoalDetailPage`.
- [REQ-059] WHERE an issue has a `parentId`, the system SHALL display the parent issue's identifier and title as a linked breadcrumb above the issue's title on `IssueDetailPage`.
- [REQ-060] WHERE an issue has a non-null `projectId`, the system SHALL display the linked project name with a navigation link to `ProjectDetailPage` in the issue's metadata section on `IssueDetailPage`.
- [REQ-061] WHERE an issue has a non-null `goalId`, the system SHALL display the linked goal title with a navigation link to `GoalDetailPage` in the issue's metadata section on `IssueDetailPage`.

---

## Non-Functional Requirements

### Performance

- [REQ-062] The system SHALL keep the TanStack Query `staleTime` at 30,000 ms so that navigating back to a previously loaded list page does not trigger a network request within the freshness window.
- [REQ-063] The system SHALL complete all optimistic UI updates (skeleton removal, list refresh) within 200 ms of a successful mutation response to prevent perceived lag.
- [REQ-064] The system SHALL use a default page size of 20 items for `IssueListPage` pagination, matching the API's default `limit` parameter.

### Security

- [REQ-065] The system SHALL rely on the `api` client's existing 401 redirect logic — if any mutation or query returns HTTP 401, the client SHALL redirect to `/login` and clear the auth context.
- [REQ-066] The system SHALL never expose `adapterConfig` or `runtimeConfig` JSON blobs as plaintext in list views; these fields SHALL only be visible (and editable) on the `AgentDetailPage` edit form.

### Usability

- [REQ-067] The system SHALL autofocus the first input field when any create or edit `Dialog` opens, to allow keyboard-first form entry without requiring a mouse click.
- [REQ-068] The system SHALL close any open `Dialog` when the user presses the `Escape` key, discarding unsaved form state.
- [REQ-069] The system SHALL render all status and priority values in human-readable form (replacing underscores with spaces, capitalising the first letter) throughout all list and detail pages.

### Reliability

- [REQ-070] The system SHALL set TanStack Query `retry: 0` for all mutations so that failed creates and updates are not silently retried, ensuring the user is immediately notified of failures.
- [REQ-071] The system SHALL not leave any query in a stale state after a successful mutation; every mutation handler SHALL call the minimum required set of `queryClient.invalidateQueries` calls as specified per entity in REQ-008 through REQ-017.

### Maintainability

- [REQ-072] The system SHALL encapsulate all mutation logic for each entity (agent, issue, project, goal, squad) in a dedicated TanStack Query `useMutation` hook co-located with the feature directory, rather than inlining mutation calls in component render functions.
- [REQ-073] The system SHALL use existing TypeScript types (`CreateXRequest`, `UpdateXRequest`) from `web/src/types/*.ts` as the mutation variable types, and SHALL NOT introduce ad-hoc inline type literals for form payloads.

---

## Constraints

- Must use existing shadcn/ui components (Dialog, Select, DropdownMenu, Table, Badge, Input, Button, etc.)
- Must use existing TanStack Query infrastructure (`queryKeys` factory from `web/src/lib/query.ts`, `api` client from `web/src/lib/api.ts`)
- Must use existing type definitions (`CreateXRequest`/`UpdateXRequest` for all entities from `web/src/types/*.ts`)
- Must use existing `useToast` hook from `web/src/hooks/use-toast.ts` for all notifications
- No new npm dependencies

---

## Acceptance Criteria

- [ ] Squad selector in `Sidebar` switches the active squad, updates all squad-scoped nav links, and persists the selection on page refresh
- [ ] All five create dialogs (squad, agent, issue, project, goal) open, validate, submit, and refresh the corresponding list
- [ ] All five detail pages toggle to edit mode, pre-populate form fields, save changes via PATCH, and invalidate relevant queries
- [ ] Issue status transitions render one button per valid target status from `issueStatusTransitions` and apply the transition via PATCH
- [ ] Agent status transitions render Approve / Pause / Resume contextually based on current agent status
- [ ] Issue comments load on `IssueDetailPage` and new comments can be submitted via the comment form
- [ ] `IssueListPage` filters by status, priority, and assignee via URL query params; "Clear Filters" resets all
- [ ] Pagination renders on `IssueListPage` when `total > limit` and navigates correctly between pages
- [ ] A success toast appears after every successful mutation
- [ ] A destructive error toast appears after every failed mutation, showing the server error message
- [ ] All form submit buttons are disabled and show a spinner while the mutation is in flight

---

## Out of Scope

- Real-time updates via SSE (covered by feature 11-agent-runtime)
- Activity log feed (covered by feature 09-activity-log)
- Budget/cost UI (covered by feature 10-cost-events-budget)
- Drag-and-drop reordering
- Bulk operations
- Squad member management (invite/remove)

---

## Dependencies

- All backend CRUD APIs (already implemented and returning shapes matching `web/src/types/*.ts`)
- React SPA with routing (feature 05, already implemented)
- shadcn/ui components (already installed)
- TanStack Query v5 infrastructure (already wired in `web/src/lib/query.ts`)

---

## Risks & Assumptions

**Assumptions:**
- Backend APIs return consistent response shapes matching the TypeScript types in `web/src/types/*.ts`
- The `POST /api/auth/logout` endpoint exists and clears the server-side session cookie
- All existing E2E tests continue to pass after UI changes
- The `GET /api/issues/{id}/comments` endpoint returns `IssueComment[]` (not a paginated envelope) so the full comment list is fetched in one request

**Risks:**
- Squad switching may leave stale cached data for queries keyed by the old squad ID if `invalidateQueries` is too narrow; care must be taken to invalidate all squad-scoped keys on squad change
- Form validation edge cases (e.g., duplicate `urlKey` for agents, duplicate `issuePrefix` for squads) will surface API-level `400` errors that must be mapped to readable inline messages
- The sidebar currently derives `activeSquadId` from `user.squads[0]` — adding a squad selector requires introducing a separate piece of state (context or localStorage) that is authoritative, and the sidebar must be refactored to read from that rather than `user.squads[0]`

---

## References

- Plan file: `.claude/plans/transient-sprouting-dragonfly.md` (detailed 5-batch implementation plan)
- Existing types: `web/src/types/*.ts`
- Query keys: `web/src/lib/query.ts`
- API client: `web/src/lib/api.ts`
- Toast hook: `web/src/hooks/use-toast.ts`
- Issue status transitions map: `web/src/types/issue.ts` (`issueStatusTransitions`)
- Agent status colours map: `web/src/types/agent.ts` (`agentStatusColors`)
