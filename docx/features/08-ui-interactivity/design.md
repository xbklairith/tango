# Design: UI Interactivity

**Created:** 2026-03-15
**Status:** Final

## System Context

- **Depends On:** Backend CRUD APIs, React SPA (feature 05), shadcn/ui + Base UI components, TanStack Query v5
- **Used By:** All authenticated user-facing pages
- **External Dependencies:** None — no new npm packages

---

## Architecture Overview

### Active Squad Context Flow

The current `Sidebar` derives `activeSquadId` from `user.squads[0].squadId`, hardcoding the first squad. This must be replaced with an `ActiveSquadContext` that is the single authoritative source for which squad is currently in view.

The context is introduced as a new provider (`web/src/lib/active-squad.tsx`) that wraps the app inside `AuthProvider`. It reads from `localStorage` on mount, falls back to `user.squads[0].squadId` if nothing is stored, and exposes a setter that both updates state and writes to `localStorage`.

Provider nesting order in `web/src/app.tsx`:

```
QueryClientProvider
  BrowserRouter
    AuthProvider
      ActiveSquadProvider      ← new, reads from AuthProvider
        Toaster
        Routes
          AuthGuard
            AppLayout
              Sidebar          ← reads from ActiveSquadContext
              <Outlet />       ← page components
```

`ActiveSquadProvider` must be inside `AuthProvider` because it needs `user.squads` to compute the fallback. It must be outside `AppLayout` so both `Sidebar` and page components share the same context instance.

### Squad-Scoped Navigation

`Sidebar` currently builds `squadNavItems` using `user.squads[0].squadId`. After this feature it reads `activeSquadId` from `useActiveSquad()`. When the squad selector fires `onValueChange`, the context setter is called, the new squad ID is written to `localStorage`, and `queryClient.invalidateQueries` is called with `{ queryKey: ["agents"], exact: false }`, `{ queryKey: ["issues"], exact: false }`, `{ queryKey: ["projects"], exact: false }`, and `{ queryKey: ["goals"], exact: false }`. This forces all squad-scoped list queries to refetch for the newly selected squad.

### Mutation Data Flow

Every create and edit operation follows a consistent pattern:

```
User submits form
  → validate() returns errors? → show inline field errors, stop
  → useMutation.mutate(payload)
    → api.post / api.patch
      → onSuccess(data):
          → close dialog / exit edit mode
          → queryClient.invalidateQueries(relevantKeys)
          → toast({ title: "Entity created", variant: "default" })
      → onError(error):
          → toast({ title: error.message ?? "An unexpected error occurred", variant: "destructive" })
```

The `isPending` flag from `useMutation` propagates into `FormDialog` to disable all inputs and show a spinner inside the submit button.

### Edit Mode Flow on Detail Pages

Each detail page holds a boolean `isEditing` state and a `FormData` state initialised from the TanStack Query cache at the moment the user clicks "Edit". While `isEditing` is `true` the page renders form inputs pre-populated from that snapshot. "Cancel" resets `isEditing` to `false` and discards local state. "Save" calls the entity's mutation hook.

`useBlocker` from `react-router` is used to intercept navigation while `isEditing && isDirty` (where `isDirty` is computed by comparing current form values to the original snapshot). If navigation is blocked the user sees a browser-native confirm dialog.

---

## Component Structure

### 1. `ActiveSquadProvider` — `web/src/lib/active-squad.tsx`

**Purpose:** Single authoritative source of the active squad ID. Persists to `localStorage`.

**Props / exports:**
```ts
interface ActiveSquadContextValue {
  activeSquadId: string | null;
  setActiveSquadId: (id: string) => void;
}

export function ActiveSquadProvider({ children }: { children: ReactNode }): JSX.Element
export function useActiveSquad(): ActiveSquadContextValue
```

**Behaviour:**
- On mount: read `localStorage.getItem("ari:activeSquadId")`. If the stored ID exists in `user.squads`, use it. Otherwise fall back to `user.squads[0]?.squadId ?? null` and write that fallback to `localStorage`.
- `setActiveSquadId(id)`: sets state, writes `localStorage.setItem("ari:activeSquadId", id)`, calls `queryClient.invalidateQueries` for all squad-scoped key prefixes (`["agents"]`, `["issues"]`, `["projects"]`, `["goals"]`).
- Exports `useActiveSquad()` which throws if called outside the provider.

**Dependencies:** `useAuth()`, `useQueryClient()`.

---

### 2. `FormDialog` — `web/src/components/shared/form-dialog.tsx`

**Purpose:** Reusable dialog shell for create and edit forms. Handles loading state, keyboard dismissal, and footer layout.

**Props:**
```ts
interface FormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  isPending: boolean;
  onSubmit: (e: React.FormEvent) => void;
  submitLabel?: string;       // default: "Save"
  children: ReactNode;
}
```

**Behaviour:**
- Renders `Dialog`, `DialogContent`, `DialogHeader`, `DialogTitle`, `DialogDescription`, `DialogFooter` from `web/src/components/ui/dialog.tsx`.
- `DialogContent` wraps a `<form onSubmit={onSubmit}>`.
- Footer contains a Cancel `DialogClose` button and a Submit `Button` (type="submit"). When `isPending` is `true` the submit button renders `<Loader2 className="h-4 w-4 animate-spin mr-1" />` and is `disabled`. All `children` form inputs are `disabled` when `isPending`.
- Autofocuses first input via `autoFocus` on the first field (each consumer sets `autoFocus` on their first `Input`).
- Escape key dismissal is handled natively by Base UI's `Dialog.Root`.

---

### 3. `PaginationControls` — `web/src/components/shared/pagination-controls.tsx`

**Purpose:** Render pagination range text and Previous/Next buttons for `IssueListPage`.

**Props:**
```ts
interface PaginationControlsProps {
  total: number;
  offset: number;
  limit: number;
  onPageChange: (newOffset: number) => void;
}
```

**Behaviour:**
- Renders nothing if `total <= limit` (REQ-053).
- Displays `"{offset + 1}–{Math.min(offset + limit, total)} of {total}"`.
- "Previous" button: disabled when `offset === 0`; onClick calls `onPageChange(offset - limit)`.
- "Next" button: disabled when `offset + limit >= total`; onClick calls `onPageChange(offset + limit)`.
- Uses `Button` variant `"outline"` size `"sm"` from `web/src/components/ui/button.tsx`.

---

### 4. `Textarea` — `web/src/components/ui/textarea.tsx`

**Purpose:** Styled textarea matching the existing Input design.

**Implementation:**
```ts
interface TextareaProps extends React.ComponentProps<"textarea"> {}

export function Textarea({ className, ...props }: TextareaProps)
```

Apply the same border/ring/focus-visible styles as `web/src/components/ui/input.tsx`. Minimum `rows={3}`.

---

### 5. Create Dialogs

All five dialogs follow the same structure: local controlled-form state, client-side validation, `FormDialog` wrapper, entity-specific `useMutation` hook. Each is co-located with its feature directory.

#### 5a. `CreateSquadDialog` — `web/src/features/squads/create-squad-dialog.tsx`

**Props:** `{ open: boolean; onOpenChange: (open: boolean) => void }`

**Fields:**
| Field | Type | Validation |
|---|---|---|
| `name` | `Input` | Required, non-empty |
| `issuePrefix` | `Input` | Required, non-empty, uppercase hint |
| `description` | `Textarea` | Optional |

**Mutation hook:** `useCreateSquad` (see Hooks section).

**On success:** close dialog, invalidate `queryKeys.squads.all`, toast "Squad created".

#### 5b. `CreateAgentDialog` — `web/src/features/agents/create-agent-dialog.tsx`

**Props:** `{ open: boolean; onOpenChange: (open: boolean) => void; squadId: string }`

**Fields:**
| Field | Type | Validation |
|---|---|---|
| `name` | `Input` | Required |
| `urlKey` | `Input` | Required, lowercase alphanumeric + hyphens hint |
| `role` | `Select` (captain/lead/member) | Required |
| `title` | `Input` | Optional |
| `reportsTo` | `Select` (populated from `queryKeys.agents.list(squadId)`) | Optional |
| `adapterType` | `Input` | Optional |
| `capabilities` | `Textarea` | Optional |

**Mutation hook:** `useCreateAgent`.

**On success:** close dialog, invalidate `queryKeys.agents.list(squadId)`, toast "Agent created".

**Note:** The `reportsTo` select fetches agents with `useQuery({ queryKey: queryKeys.agents.list(squadId), queryFn: ... })`. The query is already enabled because `squadId` is known when the dialog opens.

#### 5c. `CreateIssueDialog` — `web/src/features/issues/create-issue-dialog.tsx`

**Props:** `{ open: boolean; onOpenChange: (open: boolean) => void; squadId: string }`

**Fields:**
| Field | Type | Validation |
|---|---|---|
| `title` | `Input` | Required |
| `description` | `Textarea` | Optional |
| `type` | `Select` (task/conversation) | Optional |
| `status` | `Select` (IssueStatus values) | Optional |
| `priority` | `Select` (critical/high/medium/low) | Optional |
| `assigneeAgentId` | `Select` (agents from squad) | Optional |
| `projectId` | `Select` (projects from squad) | Optional |
| `goalId` | `Select` (goals from squad) | Optional |

**Mutation hook:** `useCreateIssue`.

**On success:** close dialog, invalidate `queryKeys.issues.list(squadId)`, toast "Issue created".

#### 5d. `CreateProjectDialog` — `web/src/features/projects/create-project-dialog.tsx`

**Props:** `{ open: boolean; onOpenChange: (open: boolean) => void; squadId: string }`

**Fields:**
| Field | Type | Validation |
|---|---|---|
| `name` | `Input` | Required |
| `description` | `Textarea` | Optional |

**Mutation hook:** `useCreateProject`.

**On success:** close dialog, invalidate `queryKeys.projects.list(squadId)`, toast "Project created".

#### 5e. `CreateGoalDialog` — `web/src/features/goals/create-goal-dialog.tsx`

**Props:** `{ open: boolean; onOpenChange: (open: boolean) => void; squadId: string }`

**Fields:**
| Field | Type | Validation |
|---|---|---|
| `title` | `Input` | Required |
| `description` | `Textarea` | Optional |
| `parentId` | `Select` (goals from squad) | Optional |

**Validation:** If `parentId` is selected, verify the chosen goal's `squadId` matches `squadId` before submitting (REQ-055). Since goals are already fetched from `GET /api/squads/{squadId}/goals` this is always true — the validation guards against stale Select state. Display inline error under the `parentId` field if violated.

**Mutation hook:** `useCreateGoal`.

**On success:** close dialog, invalidate `queryKeys.goals.list(squadId)`, toast "Goal created".

---

### 6. `IssueFilters` — `web/src/features/issues/issue-filters.tsx`

**Purpose:** Filter bar displayed above the issues table on `IssueListPage`.

**Props:**
```ts
interface IssueFiltersProps {
  filters: IssueFilters;
  agents: Agent[];
  onChange: (filters: IssueFilters) => void;
}
```

**Behaviour:**
- Renders three `Select` components side by side: Status, Priority, Assignee.
- Each `onValueChange` calls `onChange({ ...filters, [field]: value })`. When a Select is cleared (value `""`) the key is omitted from the filters object via `undefined`.
- When any filter is non-null, renders a "Clear Filters" `Button` variant `"ghost"` that calls `onChange({})`.
- Selecting a filter updates the URL via `useSearchParams` from `react-router` (set in `IssueListPage`, not inside this component — `IssueListPage` owns URL state and passes derived `filters` down as props).

**Status options:** all `IssueStatus` values formatted with `humanize()` (replace `_` with space, capitalise first letter).

**Priority options:** critical, high, medium, low.

**Assignee options:** `agents` prop mapped to `{ value: agent.id, label: agent.name }`.

---

### 7. `IssueComments` — `web/src/features/issues/issue-comments.tsx`

**Purpose:** Comment thread display and new-comment form on `IssueDetailPage`.

**Props:**
```ts
interface IssueCommentsProps {
  issueId: string;
}
```

**Behaviour:**
- Fetches `GET /api/issues/{issueId}/comments` via:
  ```ts
  useQuery({
    queryKey: queryKeys.issues.comments(issueId),
    queryFn: () => api.get<IssueComment[]>(`/issues/${issueId}/comments`),
  })
  ```
- Renders comments in chronological order. Each comment shows `authorName`, `formatDateTime(createdAt)`, and `body`.
- Empty state: `<p className="text-sm text-muted-foreground">No comments yet.</p>` (REQ-051).
- Comment form: a `Textarea` + "Add Comment" `Button`. Local `body` state. Submit calls `useAddComment` mutation hook. `onSuccess`: clear textarea, invalidate `queryKeys.issues.comments(issueId)`, toast "Comment added". Submit button disabled when `body.trim() === ""` or mutation `isPending`.

---

## Mutation Hooks

All mutation hooks live co-located with their feature directory, satisfying REQ-072.

### `useCreateSquad` — `web/src/features/squads/use-create-squad.ts`

```ts
export function useCreateSquad() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  return useMutation({
    mutationFn: (data: CreateSquadRequest) => api.post<Squad>("/squads", data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.squads.all });
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.me });
      toast({ title: "Squad created" });
    },
    onError: (err) => {
      toast({ title: err instanceof ApiClientError ? err.message : "An unexpected error occurred", variant: "destructive" });
    },
  });
}
```

Note: `queryKeys.auth.me` is invalidated so the `AuthProvider` refetches `user.squads`, making the new squad appear in the sidebar selector immediately (REQ-008).

### `useUpdateSquad` — `web/src/features/squads/use-update-squad.ts`

```ts
mutationFn: ({ id, data }: { id: string; data: UpdateSquadRequest }) =>
  api.patch<Squad>(`/squads/${id}`, data)
onSuccess: (_, { id }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.squads.detail(id) });
  queryClient.invalidateQueries({ queryKey: queryKeys.squads.all });
  toast({ title: "Squad updated" });
}
```

### `useCreateAgent` — `web/src/features/agents/use-create-agent.ts`

```ts
mutationFn: ({ squadId, data }: { squadId: string; data: CreateAgentRequest }) =>
  api.post<Agent>(`/agents`, { ...data, squadId })
onSuccess: (_, { squadId }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.agents.list(squadId) });
  toast({ title: "Agent created" });
}
```

### `useUpdateAgent` — `web/src/features/agents/use-update-agent.ts`

```ts
mutationFn: ({ id, data }: { id: string; data: UpdateAgentRequest }) =>
  api.patch<Agent>(`/agents/${id}`, data)
onSuccess: (data, { id }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.agents.detail(id) });
  queryClient.invalidateQueries({ queryKey: queryKeys.agents.list(data.squadId) });
  toast({ title: "Agent updated" });  // or "Agent status updated" when caller passes status-only payload
}
```

The toast message is caller-controlled via an optional `successMessage` parameter:
```ts
export function useUpdateAgent(options?: { successMessage?: string })
```

### `useCreateIssue` — `web/src/features/issues/use-create-issue.ts`

```ts
mutationFn: ({ squadId, data }: { squadId: string; data: CreateIssueRequest }) =>
  api.post<Issue>(`/squads/${squadId}/issues`, data)
onSuccess: (_, { squadId }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.issues.list(squadId) });
  toast({ title: "Issue created" });
}
```

### `useUpdateIssue` — `web/src/features/issues/use-update-issue.ts`

```ts
mutationFn: ({ id, data }: { id: string; data: UpdateIssueRequest }) =>
  api.patch<Issue>(`/issues/${id}`, data)
onSuccess: (data, { id }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.issues.detail(id) });
  queryClient.invalidateQueries({ queryKey: queryKeys.issues.list(data.squadId) });
  toast({ title: "Issue updated" });  // caller overrides to "Issue status updated" for transitions
}
```

### `useAddComment` — `web/src/features/issues/use-add-comment.ts`

```ts
mutationFn: ({ issueId, body }: { issueId: string; body: string }) =>
  api.post<IssueComment>(`/issues/${issueId}/comments`, { body })
onSuccess: (_, { issueId }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.issues.comments(issueId) });
  toast({ title: "Comment added" });
}
```

### `useCreateProject` — `web/src/features/projects/use-create-project.ts`

```ts
mutationFn: ({ squadId, data }: { squadId: string; data: CreateProjectRequest }) =>
  api.post<Project>(`/squads/${squadId}/projects`, data)
onSuccess: (_, { squadId }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.projects.list(squadId) });
  toast({ title: "Project created" });
}
```

### `useUpdateProject` — `web/src/features/projects/use-update-project.ts`

```ts
mutationFn: ({ id, data }: { id: string; data: UpdateProjectRequest }) =>
  api.patch<Project>(`/projects/${id}`, data)
onSuccess: (data, { id }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.projects.detail(id) });
  queryClient.invalidateQueries({ queryKey: queryKeys.projects.list(data.squadId) });
  toast({ title: "Project updated" });
}
```

### `useCreateGoal` — `web/src/features/goals/use-create-goal.ts`

```ts
mutationFn: ({ squadId, data }: { squadId: string; data: CreateGoalRequest }) =>
  api.post<Goal>(`/squads/${squadId}/goals`, data)
onSuccess: (_, { squadId }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.goals.list(squadId) });
  toast({ title: "Goal created" });
}
```

### `useUpdateGoal` — `web/src/features/goals/use-update-goal.ts`

```ts
mutationFn: ({ id, data }: { id: string; data: UpdateGoalRequest }) =>
  api.patch<Goal>(`/goals/${id}`, data)
onSuccess: (data, { id }) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.goals.detail(id) });
  queryClient.invalidateQueries({ queryKey: queryKeys.goals.list(data.squadId) });
  toast({ title: "Goal updated" });
}
```

---

## Page-by-Page Changes

### `Sidebar` — `web/src/components/layout/sidebar.tsx`

**Changes:**
1. Replace `const activeSquadId = user?.squads?.[0]?.squadId` with `const { activeSquadId, setActiveSquadId } = useActiveSquad()`.
2. Wrap the "Squad" section header with a conditional:
   - If `user.squads.length > 1`: render a `Select` with `value={activeSquadId ?? ""}` and `onValueChange={setActiveSquadId}`. Each `SelectItem` has `value={squad.squadId}` and label `squad.squadName`.
   - If `user.squads.length === 1`: render the squad name as a static `<p>` label.
3. The logout `<button>` calls `logout` from `useAuth()`. The existing `auth.tsx` `logout()` already calls `api.post("/auth/logout")`, `queryClient.clear()`, and `window.location.href = "/login"`. No additional change needed for logout (REQ-029).

### `DashboardPage` — `web/src/features/dashboard/dashboard-page.tsx`

**Changes:**
1. Replace `const activeSquad = user?.squads?.[0]` with `const { activeSquadId } = useActiveSquad()` and resolve the full squad membership from `user.squads.find(s => s.squadId === activeSquadId)`.
2. Wire "New Agent" button to open `CreateAgentDialog`.
3. Wire "New Issue" button to open `CreateIssueDialog`.
4. Wire "New Project" button to open `CreateProjectDialog`.
5. Update all three `useQuery` calls to use `activeSquadId` from context instead of `user.squads[0]`.

### `SquadListPage` — `web/src/features/squads/squad-list-page.tsx`

**Changes:**
1. Add `const [createOpen, setCreateOpen] = useState(false)`.
2. Wire "Create Squad" button to `setCreateOpen(true)`.
3. Render `<CreateSquadDialog open={createOpen} onOpenChange={setCreateOpen} />`.

### `SquadDetailPage` — `web/src/features/squads/squad-detail-page.tsx`

**Changes:**
1. Add `const [isEditing, setIsEditing] = useState(false)` and `const [form, setForm] = useState<UpdateSquadRequest>({})`.
2. When "Edit" is clicked: populate `form` from the cached `squad` object, set `isEditing = true`.
3. In edit mode: replace static field displays with `Input` and `Textarea` bound to `form`. Show "Save" and "Cancel" buttons.
4. "Cancel": `setIsEditing(false)`, reset `form` to `{}`.
5. "Save": call `useUpdateSquad().mutate({ id: squad.id, data: form })`. `onSuccess` calls `setIsEditing(false)`.
6. Use `useBlocker` to prevent navigation while `isEditing` is true.

### `AgentListPage` — `web/src/features/agents/agent-list-page.tsx`

**Changes:**
1. Add `const [createOpen, setCreateOpen] = useState(false)`.
2. Wire "Create Agent" button to `setCreateOpen(true)`.
3. Render `<CreateAgentDialog open={createOpen} onOpenChange={setCreateOpen} squadId={squadId!} />`.
4. If `squad.requireApprovalForNewAgents` is true (fetched from `queryKeys.squads.detail(squadId)`), render the approval notice banner above the table.

### `AgentDetailPage` — `web/src/features/agents/agent-detail-page.tsx`

**Changes:**
1. Add edit mode state, form state, `useUpdateAgent` hook.
2. Status transition buttons (REQ-047–REQ-049):
   ```tsx
   const updateAgent = useUpdateAgent({ successMessage: "Agent status updated" });

   // Render conditionally below the status badge:
   {agent.status === "pending_approval" && (
     <Button disabled={updateAgent.isPending} onClick={() => updateAgent.mutate({ id, data: { status: "active" } })}>
       Approve
     </Button>
   )}
   {(agent.status === "active" || agent.status === "idle") && (
     <Button disabled={updateAgent.isPending} onClick={() => updateAgent.mutate({ id, data: { status: "paused" } })}>
       Pause
     </Button>
   )}
   {agent.status === "paused" && (
     <Button disabled={updateAgent.isPending} onClick={() => updateAgent.mutate({ id, data: { status: "active" } })}>
       Resume
     </Button>
   )}
   ```
3. All status transition buttons are `disabled` when `updateAgent.isPending` (REQ-034).
4. `adapterConfig` and `runtimeConfig` are only rendered in edit mode as `Textarea` fields (REQ-066), not in the read-only view.

### `IssueListPage` — `web/src/features/issues/issue-list-page.tsx`

**Changes:**
1. Use `useSearchParams` to read/write `status`, `priority`, `assigneeAgentId`, and `offset` from the URL.
2. Derive `filters: IssueFilters` from search params. Build `offset` as `Number(searchParams.get("offset") ?? "0")`.
3. Query key: `queryKeys.issues.list(squadId!, filters)`. Query URL: `/squads/${squadId}/issues?${buildQueryString(filters, { offset, limit: 20 })}`.
4. Add `<IssueFilters filters={filters} agents={agents} onChange={handleFilterChange} />` above the table.
5. `handleFilterChange(newFilters)`: set search params for each key, also reset `offset` to `0`.
6. Add `<PaginationControls total={data?.pagination.total ?? 0} offset={offset} limit={20} onPageChange={(o) => setSearchParams({ ...currentParams, offset: String(o) })} />` below the table.
7. Add `const [createOpen, setCreateOpen] = useState(false)` and wire "Create Issue" button.

**Helper `buildQueryString`** (can be a small module-level function in the file or in `web/src/lib/utils.ts`):
```ts
function buildQueryString(filters: IssueFilters, pagination: { offset: number; limit: number }): string {
  const params = new URLSearchParams();
  if (filters.status) params.set("status", filters.status);
  if (filters.priority) params.set("priority", filters.priority);
  if (filters.assigneeAgentId) params.set("assigneeAgentId", filters.assigneeAgentId);
  if (pagination.offset > 0) params.set("offset", String(pagination.offset));
  params.set("limit", String(pagination.limit));
  return params.toString();
}
```

### `IssueDetailPage` — `web/src/features/issues/issue-detail-page.tsx`

**Changes:**
1. Add edit mode state and `useUpdateIssue` hook.
2. Status transition buttons (REQ-045–REQ-046):
   ```tsx
   const transitions = issueStatusTransitions[issue.status];
   {transitions.length > 0 && (
     <div className="flex gap-2 flex-wrap">
       {transitions.map((target) => (
         <Button key={target} size="sm" variant="outline"
           disabled={updateIssue.isPending}
           onClick={() => updateIssue.mutate({ id: issue.id, data: { status: target } }, { successMessage: "Issue status updated" })}>
           {humanize(target)}
         </Button>
       ))}
     </div>
   )}
   ```
3. Render `<IssueComments issueId={issue.id} />` at the bottom.
4. If `issue.parentId` is non-null, render linked breadcrumb fetched from `queryKeys.issues.detail(issue.parentId)` (REQ-059).
5. If `issue.projectId` is non-null, render linked project name (REQ-060).
6. If `issue.goalId` is non-null, render linked goal title (REQ-061).

### `ProjectListPage` — `web/src/features/projects/project-list-page.tsx`

**Changes:** Wire "Create Project" button to `<CreateProjectDialog>`.

### `ProjectDetailPage` — `web/src/features/projects/project-detail-page.tsx`

**Changes:** Add edit mode with `useUpdateProject`.

### `GoalListPage` — `web/src/features/goals/goal-list-page.tsx`

**Changes:** Wire "Create Goal" button to `<CreateGoalDialog>`.

### `GoalDetailPage` — `web/src/features/goals/goal-detail-page.tsx`

**Changes:**
1. Add edit mode with `useUpdateGoal`.
2. If `goal.parentId` is non-null, fetch the parent goal and render it as a linked breadcrumb (REQ-058).

---

## Data Flow

### Squad Switching

```
User selects squad from Sidebar Select
  → setActiveSquadId(newSquadId)          [ActiveSquadContext]
    → localStorage.setItem("ari:activeSquadId", newSquadId)
    → queryClient.invalidateQueries({ queryKey: ["agents"], exact: false })
    → queryClient.invalidateQueries({ queryKey: ["issues"], exact: false })
    → queryClient.invalidateQueries({ queryKey: ["projects"], exact: false })
    → queryClient.invalidateQueries({ queryKey: ["goals"], exact: false })
  → React re-renders Sidebar with new activeSquadId
    → squadNavItems links update to /squads/{newId}/agents etc.
  → DashboardPage re-renders with new activeSquadId
    → useQuery re-fetches with new key (cache is invalidated)
```

### Create Flow

```
User clicks "Create Agent"
  → setCreateOpen(true) in AgentListPage
  → CreateAgentDialog renders with open=true
    → User fills form fields
    → User clicks "Save"
      → validate() → if errors: set fieldErrors state, return
      → useCreateAgent.mutate({ squadId, data: formValues })
        → api.post("/agents", { ...data, squadId })
          → 201 Created → Agent
            → queryClient.invalidateQueries({ queryKey: ["agents", { squadId }] })
            → toast({ title: "Agent created" })
            → onOpenChange(false) → dialog closes
          → 4xx/5xx
            → toast({ title: error.message, variant: "destructive" })
            → dialog stays open
```

### Edit Flow

```
User clicks "Edit" on AgentDetailPage
  → setIsEditing(true)
  → form state initialised from queryClient.getQueryData(queryKeys.agents.detail(id))
  → Page renders Input/Select fields pre-populated

User edits fields
  → form state updates via onChange handlers

User clicks "Save"
  → validate() → if errors: show inline errors
  → useUpdateAgent.mutate({ id, data: dirtyFields })
    → api.patch("/agents/{id}", data)
      → 200 OK → Agent
        → queryClient.invalidateQueries(queryKeys.agents.detail(id))
        → queryClient.invalidateQueries(queryKeys.agents.list(squadId))
        → toast({ title: "Agent updated" })
        → setIsEditing(false)
      → error → toast destructive, stay in edit mode

User clicks "Cancel"
  → setIsEditing(false)
  → form state discarded
```

---

## API Contracts

All endpoints are on the existing backend. Types reference `web/src/types/*.ts`.

| Method | Path | Request Type | Response Type | Used By |
|--------|------|--------------|---------------|---------|
| `POST` | `/api/squads` | `CreateSquadRequest` | `Squad` | `useCreateSquad` |
| `PATCH` | `/api/squads/{id}` | `UpdateSquadRequest` | `Squad` | `useUpdateSquad` |
| `POST` | `/api/agents` | `CreateAgentRequest & { squadId }` | `Agent` | `useCreateAgent` |
| `PATCH` | `/api/agents/{id}` | `UpdateAgentRequest` | `Agent` | `useUpdateAgent` |
| `POST` | `/api/squads/{squadId}/issues` | `CreateIssueRequest` | `Issue` | `useCreateIssue` |
| `PATCH` | `/api/issues/{id}` | `UpdateIssueRequest` | `Issue` | `useUpdateIssue` |
| `POST` | `/api/issues/{id}/comments` | `{ body: string }` | `IssueComment` | `useAddComment` |
| `GET` | `/api/issues/{id}/comments` | — | `IssueComment[]` | `IssueComments` |
| `POST` | `/api/squads/{squadId}/projects` | `CreateProjectRequest` | `Project` | `useCreateProject` |
| `PATCH` | `/api/projects/{id}` | `UpdateProjectRequest` | `Project` | `useUpdateProject` |
| `POST` | `/api/squads/{squadId}/goals` | `CreateGoalRequest` | `Goal` | `useCreateGoal` |
| `PATCH` | `/api/goals/{id}` | `UpdateGoalRequest` | `Goal` | `useUpdateGoal` |
| `GET` | `/api/squads/{id}/issues?status=&priority=&assigneeAgentId=&offset=&limit=` | — | `PaginatedResponse<Issue>` | `IssueListPage` |
| `POST` | `/api/auth/logout` | — | `204` | `auth.tsx` (existing) |

**Query String Construction for `IssueListPage`:**

URL filter params map 1:1 to `IssueFilters` fields. All are optional. Pagination: `offset` defaults to `0`, `limit` is always `20` (REQ-064).

---

## Form Validation

Client-side validation runs synchronously before the mutation is called. Each dialog maintains a `Record<string, string>` error map keyed by field name.

```ts
// Example for CreateSquadDialog
function validate(values: Partial<CreateSquadRequest>): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!values.name?.trim()) errors.name = "Name is required";
  if (!values.issuePrefix?.trim()) errors.issuePrefix = "Issue prefix is required";
  return errors;
}
```

Errors are displayed below the relevant `Input` or `Select` as `<p className="text-xs text-destructive mt-1">{errors.fieldName}</p>`. The submit button is NOT disabled by validation errors — errors become visible only after the user clicks Save (standard pattern). Fields clear their error on change.

API-level errors (e.g., duplicate `urlKey`, 422 Unprocessable Entity) surface via the `onError` toast, not inline. The toast message comes from `ApiClientError.message` which carries the server's `error` string.

---

## Loading States

| Location | `isLoading` skeleton | REQ |
|---|---|---|
| All list pages | Three `<div className="h-16 rounded-md bg-muted animate-pulse" />` rows | REQ-031 |
| All detail pages | `<div className="h-8 w-48 bg-muted animate-pulse rounded" />` + `<div className="h-32 bg-muted animate-pulse rounded" />` | REQ-032 |
| Form submit button | `<Loader2 className="h-4 w-4 animate-spin mr-1" />` inside button, all inputs disabled | REQ-030 |
| Status transition buttons | All disabled while `isPending` | REQ-034 |

The existing list pages already implement the skeleton pattern (e.g., `AgentListPage` line 19). Ensure all pages follow the same structure consistently.

---

## Human-Readable Display Utility

A shared `humanize` utility should be added to `web/src/lib/utils.ts`:

```ts
export function humanize(value: string): string {
  return value.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}
```

Used wherever status/priority/role values are displayed (REQ-069). Replace existing inline `.replace("_", " ")` calls with `humanize()`.

---

## Error Handling

| Error Scenario | Handling |
|---|---|
| Network failure (`NETWORK_ERROR`) | Destructive toast: "Network connection failed" |
| `401 Unauthenticated` | `api.ts` redirects to `/login` before `onError` fires |
| `400 Bad Request` (e.g., duplicate prefix) | Destructive toast with server `error` message |
| `404 Not Found` on detail page | Render `<p>Entity not found</p>` (existing pattern) |
| `500 Server Error` | Destructive toast: server `error` or fallback "An unexpected error occurred" |

The `ApiClientError` class (`web/src/lib/api.ts`) carries `.message` from the server's `{ "error": "..." }` body. Every `onError` handler uses:
```ts
onError: (err: unknown) => {
  toast({
    title: err instanceof ApiClientError ? err.message : "An unexpected error occurred",
    variant: "destructive",
  });
}
```

The `useToast` hook from `web/src/hooks/use-toast.ts` is used exclusively (REQ-041). All toasts auto-dismiss after 5000 ms (already wired in the hook's `setTimeout`).

---

## TanStack Query Configuration

The existing `queryClient` in `web/src/lib/query.ts` already sets:
- `staleTime: 30_000` — satisfies REQ-062
- `mutations.retry: 0` — satisfies REQ-070

No changes needed to query client configuration.

The `queryKeys` factory already covers all entity types needed. No new keys are required except confirming `queryKeys.issues.comments(issueId)` exists (it does, line 33).

---

## File Structure Summary

New files to create:

```
web/src/lib/
  active-squad.tsx                        ← ActiveSquadProvider, useActiveSquad

web/src/components/shared/
  form-dialog.tsx                         ← FormDialog wrapper
  pagination-controls.tsx                 ← PaginationControls

web/src/components/ui/
  textarea.tsx                            ← Textarea primitive

web/src/features/squads/
  create-squad-dialog.tsx
  use-create-squad.ts
  use-update-squad.ts

web/src/features/agents/
  create-agent-dialog.tsx
  use-create-agent.ts
  use-update-agent.ts

web/src/features/issues/
  create-issue-dialog.tsx
  issue-filters.tsx
  issue-comments.tsx
  use-create-issue.ts
  use-update-issue.ts
  use-add-comment.ts

web/src/features/projects/
  create-project-dialog.tsx
  use-create-project.ts
  use-update-project.ts

web/src/features/goals/
  create-goal-dialog.tsx
  use-create-goal.ts
  use-update-goal.ts
```

Files modified:

```
web/src/app.tsx                           ← add ActiveSquadProvider
web/src/lib/utils.ts                      ← add humanize(), buildQueryString()
web/src/components/layout/sidebar.tsx     ← squad selector, useActiveSquad
web/src/features/dashboard/dashboard-page.tsx
web/src/features/squads/squad-list-page.tsx
web/src/features/squads/squad-detail-page.tsx
web/src/features/agents/agent-list-page.tsx
web/src/features/agents/agent-detail-page.tsx
web/src/features/issues/issue-list-page.tsx
web/src/features/issues/issue-detail-page.tsx
web/src/features/projects/project-list-page.tsx
web/src/features/projects/project-detail-page.tsx
web/src/features/goals/goal-list-page.tsx
web/src/features/goals/goal-detail-page.tsx
```

---

## Testing Strategy

### Existing E2E Tests

The `.playwright-mcp/` directory contains console logs but no test spec files. There are no existing Playwright test specs in the repo. `@playwright/test` is installed as a dev dependency but no test directory exists yet.

### E2E Tests to Create (`web/e2e/`)

Each spec should start from a logged-in state (fixture or `storageState`).

| Spec File | Scenarios |
|---|---|
| `squad.spec.ts` | Create squad → appears in list and squad selector; Edit squad → changes reflected in detail page |
| `agent.spec.ts` | Create agent → appears in list; Edit agent; Approve / Pause / Resume status transitions |
| `issue.spec.ts` | Create issue → appears in list; Edit issue; Status transitions; Add comment; Filter by status/priority; Pagination with > 20 items; Clear filters |
| `project.spec.ts` | Create project; Edit project |
| `goal.spec.ts` | Create goal with parent; Edit goal; Breadcrumb shown |
| `squad-selector.spec.ts` | User with 2 squads: switch squad → nav links update → list pages show different data; Refresh → same squad selected |
| `sidebar-logout.spec.ts` | Logout → redirected to `/login`; Cannot access protected route after logout |

### Component / Unit Tests

No Jest / Vitest is currently installed. If unit tests are added in a future feature, the mutation hooks (`useCreateAgent`, etc.) are the highest-value targets because they encapsulate all the invalidation logic. Each hook can be tested with `renderHook` + `@tanstack/react-query`'s `QueryClientProvider` test wrapper and a mocked `api`.

---

## Open Questions (Resolved)

- **Squad switching cache strategy:** Selectively invalidate by key prefix (`["agents"]`, `["issues"]`, `["projects"]`, `["goals"]`) rather than `queryClient.clear()` to preserve squad-agnostic cache entries like `queryKeys.squads.all` and `queryKeys.auth.me`.
- **Create dialogs pre-populating from URL params:** No. Fields start empty except where entity-specific defaults are defined (e.g., issue `status` defaults to `"backlog"` per API). No URL param pre-population.

---

## References

- Requirements: `docx/features/08-ui-interactivity/requirements.md`
- API client: `web/src/lib/api.ts`
- Query keys: `web/src/lib/query.ts`
- Toast hook: `web/src/hooks/use-toast.ts`
- Auth context: `web/src/lib/auth.tsx`
- Issue status transitions: `web/src/types/issue.ts` (`issueStatusTransitions`)
- Agent status colours: `web/src/types/agent.ts` (`agentStatusColors`)
- Dialog component: `web/src/components/ui/dialog.tsx` (Base UI `@base-ui/react/dialog`)
- Select component: `web/src/components/ui/select.tsx` (Base UI `@base-ui/react/select`)
