# Tasks: UI Interactivity

**Created:** 2026-03-15
**Status:** Complete

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-001 through REQ-073

## Implementation Approach

Build in 5 batches following dependency order. Each batch is independently testable and deployable.

- **Batch 0** — Active squad context + sidebar squad switcher (REQ-001, REQ-002, REQ-029, REQ-038, REQ-043, REQ-044)
- **Batch 1** — Shared primitives: `Textarea`, `FormDialog`, `PaginationControls`, `humanize` util (REQ-030, REQ-031, REQ-032, REQ-052, REQ-053, REQ-067, REQ-068, REQ-069)
- **Batch 2** — Create dialogs + mutation hooks + wire create buttons (REQ-003 through REQ-012, REQ-035, REQ-036, REQ-037, REQ-039, REQ-070, REQ-071, REQ-072, REQ-073)
- **Batch 3** — Detail page edit mode + status transitions + issue comments (REQ-013 through REQ-020, REQ-027, REQ-028, REQ-033, REQ-034, REQ-040, REQ-045 through REQ-051, REQ-058 through REQ-061)
- **Batch 4** — Issue filtering + pagination + badge colour maps (REQ-021 through REQ-026, REQ-052, REQ-053, REQ-054, REQ-056, REQ-062 through REQ-064, REQ-069)

## Progress Summary

- Total Tasks: 18
- Completed: 18/18
- In Progress: None
- Test Coverage: 0 / 18 tasks tested

---

## Batch 0: Active Squad Context + Sidebar Squad Switcher

---

### [x] Task 0.1 — `humanize` utility and `ActiveSquadProvider`

**Requirements:** REQ-001, REQ-002, REQ-038, REQ-043, REQ-044, REQ-069

**Context:**
The sidebar currently hardcodes `user.squads[0].squadId`. This task replaces that with an `ActiveSquadContext` that is the single source of truth, persists to `localStorage`, and invalidates all squad-scoped queries on change. It also adds the `humanize` string utility needed in later batches.

---

#### RED — Write failing tests first

Create `web/e2e/squad-selector.spec.ts`:

```ts
// Test 1: Single-squad user sees static label, no select control
test("single-squad user sees squad name as static label", async ({ page }) => {
  // login as user with 1 squad, navigate to dashboard
  // assert: text matching squad name visible in sidebar
  // assert: no <select> or [role="combobox"] in sidebar squad section
});

// Test 2: Multi-squad user sees a select control
test("multi-squad user sees squad selector in sidebar", async ({ page }) => {
  // login as user with 2+ squads
  // assert: [role="combobox"] visible in sidebar
  // assert: all squad names present as options
});

// Test 3: Switching squad updates nav links
test("switching squad updates all squad-scoped nav links", async ({ page }) => {
  // select second squad from selector
  // assert: Agents nav link href contains second squadId
  // assert: Issues nav link href contains second squadId
});

// Test 4: Selection persists on refresh
test("active squad persists across page refresh", async ({ page }) => {
  // select second squad
  // reload page
  // assert: second squad still shown as selected in sidebar
});
```

Create `web/src/lib/active-squad.test.ts` (unit tests with jsdom + React Testing Library):

```ts
// Test 5: Falls back to user.squads[0] when localStorage is empty
// Test 6: Reads stored squad ID from localStorage on mount (if valid)
// Test 7: Falls back to squads[0] when stored ID is not in user.squads
// Test 8: setActiveSquadId writes to localStorage
// Test 9: useActiveSquad throws when called outside ActiveSquadProvider
```

Create `web/src/lib/utils.test.ts`:
```ts
// Test 10: humanize("in_progress") === "In progress"
// Test 11: humanize("pending_approval") === "Pending approval"
// Test 12: humanize("done") === "Done"
// Test 13: humanize("") === ""
```

---

#### GREEN — Minimum implementation

**`web/src/lib/utils.ts`** — add `humanize`:
```ts
export function humanize(value: string): string {
  return value.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}
```

**`web/src/lib/active-squad.tsx`** — create new file:
- Export `ActiveSquadProvider` and `useActiveSquad()`
- On mount: read `localStorage.getItem("ari:activeSquadId")`. If stored ID exists in `user.squads`, use it. Otherwise fall back to `user.squads[0]?.squadId ?? null`.
- `setActiveSquadId(id)`: set state, write to `localStorage`, call `queryClient.invalidateQueries` for `["agents"]`, `["issues"]`, `["projects"]`, `["goals"]` with `exact: false`.
- `useActiveSquad()` throws if called outside provider.
- Dependencies: `useAuth()`, `useQueryClient()`.

**`web/src/app.tsx`** — wrap routes with `ActiveSquadProvider`:
```tsx
<AuthProvider>
  <ActiveSquadProvider>   {/* ← new, inside AuthProvider */}
    <Toaster />
    <Routes>...</Routes>
  </ActiveSquadProvider>
</AuthProvider>
```

**`web/src/components/layout/sidebar.tsx`** — wire squad selector:
1. Replace `user.squads[0].squadId` with `useActiveSquad()`.
2. Conditional render:
   - `user.squads.length > 1`: render `<Select value={activeSquadId} onValueChange={setActiveSquadId}>` with one `SelectItem` per squad.
   - `user.squads.length === 1`: render `<p className="text-sm font-medium">{squad.squadName}</p>`.
3. All squad-scoped nav `href` values use `activeSquadId`.

---

#### REFACTOR

- Extract `SquadSelector` sub-component into its own function inside `sidebar.tsx` to keep the main component readable.
- Ensure `localStorage` key `"ari:activeSquadId"` is a named constant (`ACTIVE_SQUAD_KEY`) defined once in `active-squad.tsx`.
- Remove any residual `user.squads[0]` reads from `sidebar.tsx`, `dashboard-page.tsx`, and `agent-list-page.tsx`.

---

#### Acceptance Criteria

- [ ] `humanize("in_progress")` returns `"In progress"` (REQ-069)
- [ ] Single-squad user sees squad name as static text in sidebar (REQ-044)
- [ ] Multi-squad user sees a `Select` component in sidebar listing all squads (REQ-043)
- [ ] Selecting a squad updates all squad-scoped nav links (REQ-001)
- [ ] Selected squad ID is persisted to `localStorage` under `"ari:activeSquadId"` (REQ-002)
- [ ] Page refresh restores previously selected squad (REQ-002)
- [ ] `setActiveSquadId` invalidates `agents`, `issues`, `projects`, `goals` query prefixes (REQ-001)
- [ ] `useActiveSquad()` throws a descriptive error when called outside provider (REQ-038)
- [ ] `app.tsx` nests `ActiveSquadProvider` inside `AuthProvider` and outside `AppLayout`

**Files to create:**
- `web/src/lib/active-squad.tsx`
- `web/src/lib/active-squad.test.ts`
- `web/src/lib/utils.test.ts`
- `web/e2e/squad-selector.spec.ts`

**Files to modify:**
- `web/src/lib/utils.ts`
- `web/src/app.tsx`
- `web/src/components/layout/sidebar.tsx`

---

### [x] Task 0.2 — Dashboard wired to `ActiveSquadProvider`

**Requirements:** REQ-001, REQ-038

**Context:**
`DashboardPage` currently reads from `user.squads[0]`. It must switch to `useActiveSquad()` so quick-action buttons and stat queries all reflect the active squad.

---

#### RED — Write failing test

Add to `web/e2e/squad-selector.spec.ts`:

```ts
// Test: After switching squad in sidebar, dashboard stat queries refetch for new squad
test("dashboard reflects active squad after squad switch", async ({ page }) => {
  // switch squad in sidebar
  // assert: agents count / issues count in dashboard summary cards match the new squad's data
  // (relies on test fixture with known data per squad)
});
```

---

#### GREEN — Minimum implementation

**`web/src/features/dashboard/dashboard-page.tsx`:**
1. Replace `const activeSquad = user?.squads?.[0]` with:
   ```ts
   const { activeSquadId } = useActiveSquad();
   const activeSquad = user?.squads?.find(s => s.squadId === activeSquadId);
   ```
2. Update the three `useQuery` calls to use `activeSquadId` from context.
3. Leave the quick-action button wiring (`CreateAgentDialog`, `CreateIssueDialog`, `CreateProjectDialog`) as `open` state + `TODO` comment — they will be fully wired in Batch 2.

---

#### REFACTOR

- Guard the case where `activeSquadId` is `null` (user has no squads): render an empty state or loading indicator rather than calling the API with `null`.

---

#### Acceptance Criteria

- [ ] `DashboardPage` uses `useActiveSquad()` not `user.squads[0]` (REQ-038)
- [ ] Switching squad in sidebar causes dashboard stat queries to refetch for the new squad (REQ-001)
- [ ] `activeSquad` resolves correctly when `activeSquadId` is loaded from `localStorage` (REQ-002)

**Files to modify:**
- `web/src/features/dashboard/dashboard-page.tsx`

---

## Batch 1: Shared Primitives

---

### [x] Task 1.1 — `Textarea` UI primitive

**Requirements:** REQ-067, REQ-069

**Context:**
Several create and edit forms need a multi-line text input. The existing `Input` component is single-line only. This task adds a `Textarea` primitive styled consistently with `Input`.

---

#### RED — Write failing test

Add to `web/src/components/ui/textarea.test.tsx`:

```ts
// Test 1: Renders a <textarea> element
// Test 2: Applies custom className alongside base classes
// Test 3: Forwards all native textarea props (placeholder, disabled, value, onChange)
// Test 4: Defaults to rows={3}
// Test 5: disabled state applies opacity-50 and cursor-not-allowed
```

---

#### GREEN — Minimum implementation

**`web/src/components/ui/textarea.tsx`** — create new file:
```ts
interface TextareaProps extends React.ComponentProps<"textarea"> {}

export function Textarea({ className, ...props }: TextareaProps) {
  return (
    <textarea
      rows={3}
      className={cn(
        "flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm",
        "ring-offset-background placeholder:text-muted-foreground",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}
```

---

#### REFACTOR

- Verify styles match `web/src/components/ui/input.tsx` pixel-for-pixel. Extract shared style string to a `fieldBase` constant if there is significant duplication.

---

#### Acceptance Criteria

- [ ] Renders a `<textarea>` DOM element (not `<input>`)
- [ ] Defaults to `rows={3}`
- [ ] Visual styling matches `Input` (border, ring, focus, disabled states)
- [ ] Accepts all standard `textarea` HTML attributes
- [ ] `disabled` prop applies visual disabled state

**Files to create:**
- `web/src/components/ui/textarea.tsx`
- `web/src/components/ui/textarea.test.tsx`

---

### [x] Task 1.2 — `FormDialog` shared shell

**Requirements:** REQ-030, REQ-067, REQ-068

**Context:**
All five create dialogs and all edit flows share the same dialog structure: a title, optional description, a `<form>`, Cancel + Submit buttons in the footer, pending state disabling all inputs, and spinner in the submit button. This task extracts that shell so each dialog only provides fields.

---

#### RED — Write failing test

Create `web/src/components/shared/form-dialog.test.tsx`:

```ts
// Test 1: Renders children when open=true
// Test 2: Does not render children when open=false
// Test 3: Submit button shows "Save" by default
// Test 4: submitLabel prop overrides button text
// Test 5: When isPending=true, submit button is disabled
// Test 6: When isPending=true, submit button renders <Loader2> spinner icon
// Test 7: When isPending=true, children inputs are disabled (via fieldset or disabled prop passthrough)
// Test 8: Calls onSubmit when form is submitted
// Test 9: Escape key closes dialog (fires onOpenChange(false))
// Test 10: title prop appears as DialogTitle content
```

---

#### GREEN — Minimum implementation

**`web/src/components/shared/form-dialog.tsx`** — create new file:

```tsx
interface FormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  isPending: boolean;
  onSubmit: (e: React.FormEvent) => void;
  submitLabel?: string;
  children: ReactNode;
}

export function FormDialog({ open, onOpenChange, title, description, isPending, onSubmit, submitLabel = "Save", children }: FormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        <form onSubmit={onSubmit}>
          <fieldset disabled={isPending} className="space-y-4">
            {children}
          </fieldset>
          <DialogFooter className="mt-6">
            <DialogClose asChild>
              <Button type="button" variant="outline">Cancel</Button>
            </DialogClose>
            <Button type="submit" disabled={isPending}>
              {isPending && <Loader2 className="h-4 w-4 animate-spin mr-1" />}
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
```

---

#### REFACTOR

- Confirm `<fieldset disabled>` correctly disables all child `Input`, `Select`, and `Textarea` elements across browsers.
- Ensure `DialogClose` does not prevent `onOpenChange` from firing — use `asChild` pattern correctly.
- Add `aria-label` to the Loader2 spinner: `aria-hidden="true"`.

---

#### Acceptance Criteria

- [ ] Renders `Dialog`, `DialogHeader`, `DialogTitle` from existing `dialog.tsx` (REQ-067, REQ-068)
- [ ] Wraps children in `<fieldset disabled={isPending}>` to disable all inputs when pending (REQ-030)
- [ ] Submit button shows `<Loader2 animate-spin>` when `isPending=true` (REQ-030)
- [ ] Submit button label defaults to "Save" and is overridable via `submitLabel` prop
- [ ] Cancel button fires `onOpenChange(false)` to close the dialog
- [ ] `onSubmit` is attached to the `<form>` element (not the button)
- [ ] Escape key closes dialog via Base UI's native Dialog behaviour (REQ-068)

**Files to create:**
- `web/src/components/shared/form-dialog.tsx`
- `web/src/components/shared/form-dialog.test.tsx`

---

### [x] Task 1.3 — `PaginationControls` component

**Requirements:** REQ-025, REQ-026, REQ-052, REQ-053, REQ-064

**Context:**
Issue list pagination requires a Previous/Next control bar that shows the current range ("1–20 of 47") and disables boundary buttons. This component is stateless — `IssueListPage` owns the URL offset state and passes it down.

---

#### RED — Write failing test

Create `web/src/components/shared/pagination-controls.test.tsx`:

```ts
// Test 1: Returns null (renders nothing) when total <= limit
// Test 2: Renders range text "1–20 of 47" for offset=0, limit=20, total=47
// Test 3: Renders range text "21–40 of 47" for offset=20, limit=20, total=47
// Test 4: Renders range text "41–47 of 47" for offset=40, limit=20, total=47
// Test 5: Previous button is disabled when offset=0
// Test 6: Previous button is enabled when offset > 0
// Test 7: Next button is disabled when offset + limit >= total (e.g. offset=40, limit=20, total=47)
// Test 8: Next button is enabled when offset + limit < total
// Test 9: Clicking Next calls onPageChange(offset + limit)
// Test 10: Clicking Previous calls onPageChange(offset - limit)
```

---

#### GREEN — Minimum implementation

**`web/src/components/shared/pagination-controls.tsx`** — create new file:

```tsx
interface PaginationControlsProps {
  total: number;
  offset: number;
  limit: number;
  onPageChange: (newOffset: number) => void;
}

export function PaginationControls({ total, offset, limit, onPageChange }: PaginationControlsProps) {
  if (total <= limit) return null;

  const start = offset + 1;
  const end = Math.min(offset + limit, total);
  const hasPrev = offset > 0;
  const hasNext = offset + limit < total;

  return (
    <div className="flex items-center justify-between py-3">
      <span className="text-sm text-muted-foreground">{start}–{end} of {total}</span>
      <div className="flex gap-2">
        <Button variant="outline" size="sm" disabled={!hasPrev} onClick={() => onPageChange(offset - limit)}>
          Previous
        </Button>
        <Button variant="outline" size="sm" disabled={!hasNext} onClick={() => onPageChange(offset + limit)}>
          Next
        </Button>
      </div>
    </div>
  );
}
```

---

#### REFACTOR

- Add `aria-label="Go to previous page"` and `aria-label="Go to next page"` to buttons for accessibility.
- Clamp `onPageChange(Math.max(0, offset - limit))` defensively, matching REQ-026.

---

#### Acceptance Criteria

- [ ] Returns `null` when `total <= limit` (REQ-053)
- [ ] Displays `"{start}–{end} of {total}"` range text (REQ-052)
- [ ] "Previous" disabled when `offset === 0` (REQ-025, REQ-026)
- [ ] "Next" disabled when `offset + limit >= total` (REQ-025)
- [ ] Clicking Next calls `onPageChange(offset + limit)` (REQ-025)
- [ ] Clicking Previous calls `onPageChange(offset - limit)` clamped to 0 (REQ-026)
- [ ] Buttons use `variant="outline" size="sm"` from `Button`

**Files to create:**
- `web/src/components/shared/pagination-controls.tsx`
- `web/src/components/shared/pagination-controls.test.tsx`

---

## Batch 2: Create Dialogs + Mutation Hooks

---

### [x] Task 2.1 — Squad create: `useCreateSquad` + `CreateSquadDialog`

**Requirements:** REQ-003, REQ-008, REQ-035, REQ-036, REQ-037, REQ-039, REQ-070, REQ-072, REQ-073

**Context:**
The "Create Squad" button on `SquadListPage` is currently a no-op. This task implements the mutation hook and dialog, then wires the button.

---

#### RED — Write failing tests

Create `web/src/features/squads/use-create-squad.test.ts`:

```ts
// Test 1: calls api.post("/squads", data) with the form payload
// Test 2: on success: invalidates queryKeys.squads.all
// Test 3: on success: invalidates queryKeys.auth.me (so new squad appears in selector)
// Test 4: on success: calls toast({ title: "Squad created" })
// Test 5: on error: calls toast({ title: error.message, variant: "destructive" })
// Test 6: on error with non-ApiClientError: calls toast with fallback message
```

Add to `web/e2e/squad.spec.ts`:

```ts
// Test 7: "Create Squad" button opens dialog
// Test 8: Submitting empty name shows inline validation error "Name is required"
// Test 9: Submitting empty issuePrefix shows inline validation error
// Test 10: Valid submission closes dialog and new squad appears in list
// Test 11: Success toast "Squad created" is shown
// Test 12: API error surfaces as destructive toast
```

---

#### GREEN — Minimum implementation

**`web/src/features/squads/use-create-squad.ts`** — create new file:
- `useMutation` with `mutationFn: (data: CreateSquadRequest) => api.post<Squad>("/squads", data)`.
- `onSuccess`: invalidate `queryKeys.squads.all` and `queryKeys.auth.me`; toast "Squad created".
- `onError`: toast destructive with `ApiClientError` message or fallback.

**`web/src/features/squads/create-squad-dialog.tsx`** — create new file:
- Props: `{ open: boolean; onOpenChange: (open: boolean) => void }`.
- Local state: `{ name: string; issuePrefix: string; description: string }` all empty by default.
- Validation: `errors.name` if `name.trim() === ""`, `errors.issuePrefix` if `issuePrefix.trim() === ""`.
- Render inline error `<p className="text-xs text-destructive mt-1">` below each invalid field.
- On success from `useCreateSquad`: call `onOpenChange(false)`, reset form state.
- First `Input` has `autoFocus`.

**`web/src/features/squads/squad-list-page.tsx`** — add:
```tsx
const [createOpen, setCreateOpen] = useState(false);
// Wire "Create Squad" button: onClick={() => setCreateOpen(true)}
// Add: <CreateSquadDialog open={createOpen} onOpenChange={setCreateOpen} />
```

---

#### REFACTOR

- Extract `validate` function as a pure function outside the component for testability.
- Clear individual field errors `onchange` (not on blur) per the design spec.
- Verify `issuePrefix` helper text "uppercase recommended" is shown as `<p className="text-xs text-muted-foreground">` below the input.

---

#### Acceptance Criteria

- [ ] Clicking "Create Squad" opens the dialog (REQ-003)
- [ ] Required field errors shown inline before API is called (REQ-039)
- [ ] `POST /api/squads` is called with form values (REQ-008)
- [ ] Dialog closes on success (REQ-008)
- [ ] `queryKeys.squads.all` and `queryKeys.auth.me` invalidated on success (REQ-008, REQ-037)
- [ ] Success toast "Squad created" shown (REQ-035)
- [ ] Error toast with server message shown on failure (REQ-036)
- [ ] Submit button disabled + spinner while pending (REQ-030)
- [ ] Mutation hook is co-located in `web/src/features/squads/` (REQ-072)
- [ ] `CreateSquadRequest` type used for mutation variables (REQ-073)

**Files to create:**
- `web/src/features/squads/use-create-squad.ts`
- `web/src/features/squads/use-create-squad.test.ts`
- `web/src/features/squads/create-squad-dialog.tsx`
- `web/e2e/squad.spec.ts`

**Files to modify:**
- `web/src/features/squads/squad-list-page.tsx`

---

### [x] Task 2.2 — Agent create: `useCreateAgent` + `CreateAgentDialog`

**Requirements:** REQ-004, REQ-009, REQ-035, REQ-036, REQ-037, REQ-039, REQ-072, REQ-073

**Context:**
"Create Agent" button on `AgentListPage` and "New Agent" quick-action on `DashboardPage` are no-ops. The agent form has the most fields of any create dialog, including a `reportsTo` select populated from the squad's existing agents.

---

#### RED — Write failing tests

Create `web/src/features/agents/use-create-agent.test.ts`:

```ts
// Test 1: calls api.post("/agents", { ...data, squadId })
// Test 2: on success: invalidates queryKeys.agents.list(squadId)
// Test 3: on success: calls toast({ title: "Agent created" })
// Test 4: on error: calls toast destructive with error.message
```

Add to `web/e2e/agent.spec.ts`:

```ts
// Test 5: "Create Agent" button opens dialog
// Test 6: Submitting without name shows "Name is required"
// Test 7: Submitting without urlKey shows "URL key is required"
// Test 8: Submitting without role shows "Role is required"
// Test 9: reportsTo select is populated with existing agents from squad
// Test 10: Valid submission closes dialog and new agent appears in list
// Test 11: Success toast "Agent created" shown
```

---

#### GREEN — Minimum implementation

**`web/src/features/agents/use-create-agent.ts`** — create new file:
- `mutationFn: ({ squadId, data }) => api.post<Agent>("/agents", { ...data, squadId })`.
- `onSuccess(_, { squadId })`: invalidate `queryKeys.agents.list(squadId)`; toast "Agent created".
- `onError`: toast destructive.

**`web/src/features/agents/create-agent-dialog.tsx`** — create new file:
- Props: `{ open: boolean; onOpenChange: (open: boolean) => void; squadId: string }`.
- Fields: `name` (required), `urlKey` (required), `role` (required `Select`), `title` (optional), `reportsTo` (optional `Select`), `adapterType` (optional), `capabilities` (optional `Textarea`).
- The `reportsTo` Select fetches `useQuery({ queryKey: queryKeys.agents.list(squadId), ... })` and maps agents to `SelectItem`s.
- Validation: `name`, `urlKey`, `role` all required.
- First field (`name`) has `autoFocus`.

**`web/src/features/agents/agent-list-page.tsx`** — wire:
- Replace `useActiveSquad()` for `squadId` (from Batch 0).
- Add `createOpen` state, wire "Create Agent" button, render `<CreateAgentDialog>`.

**`web/src/features/dashboard/dashboard-page.tsx`** — wire "New Agent":
- Add `agentCreateOpen` state, wire "New Agent" button, render `<CreateAgentDialog squadId={activeSquadId!}>`.

---

#### REFACTOR

- The `urlKey` input should display helper text: "Lowercase letters, numbers, and hyphens only".
- `role` Select options should render using `humanize()` — "captain" → "Captain", etc.
- `reportsTo` Select option list should include an explicit "None" option (`value=""`) as the first item.

---

#### Acceptance Criteria

- [ ] "Create Agent" on `AgentListPage` and "New Agent" on `DashboardPage` open dialog (REQ-004)
- [ ] `name`, `urlKey`, and `role` are validated client-side before API call (REQ-039)
- [ ] `reportsTo` select is populated from squad agents (REQ-004)
- [ ] `POST /api/agents` called with `squadId` merged into body (REQ-009)
- [ ] `queryKeys.agents.list(squadId)` invalidated on success (REQ-009, REQ-037)
- [ ] Success toast "Agent created" (REQ-035)
- [ ] Error toast destructive on failure (REQ-036)
- [ ] Submit disabled + spinner while pending (REQ-030)
- [ ] Hook co-located in `web/src/features/agents/` (REQ-072)

**Files to create:**
- `web/src/features/agents/use-create-agent.ts`
- `web/src/features/agents/use-create-agent.test.ts`
- `web/src/features/agents/create-agent-dialog.tsx`
- `web/e2e/agent.spec.ts`

**Files to modify:**
- `web/src/features/agents/agent-list-page.tsx`
- `web/src/features/dashboard/dashboard-page.tsx`

---

### [x] Task 2.3 — Issue create: `useCreateIssue` + `CreateIssueDialog`

**Requirements:** REQ-005, REQ-010, REQ-035, REQ-036, REQ-037, REQ-039, REQ-072, REQ-073

**Context:**
"Create Issue" on `IssueListPage` and "New Issue" on `DashboardPage` are no-ops. The issue form has the most optional selects: status, priority, type, assignee, project, goal. All are optional except `title`.

---

#### RED — Write failing tests

Create `web/src/features/issues/use-create-issue.test.ts`:

```ts
// Test 1: calls api.post("/squads/{squadId}/issues", data)
// Test 2: on success: invalidates queryKeys.issues.list(squadId)
// Test 3: on success: calls toast({ title: "Issue created" })
// Test 4: on error: toast destructive
```

Add to `web/e2e/issue.spec.ts`:

```ts
// Test 5: "Create Issue" button opens dialog
// Test 6: Submitting without title shows "Title is required"
// Test 7: assignee select populated from squad agents
// Test 8: project select populated from squad projects
// Test 9: goal select populated from squad goals
// Test 10: Valid submission adds issue to list
// Test 11: Success toast "Issue created"
```

---

#### GREEN — Minimum implementation

**`web/src/features/issues/use-create-issue.ts`** — create new file:
- `mutationFn: ({ squadId, data }) => api.post<Issue>("/squads/${squadId}/issues", data)`.
- `onSuccess(_, { squadId })`: invalidate `queryKeys.issues.list(squadId)`; toast "Issue created".

**`web/src/features/issues/create-issue-dialog.tsx`** — create new file:
- Props: `{ open: boolean; onOpenChange: (open: boolean) => void; squadId: string }`.
- Fields: `title` (required `Input`), `description` (optional `Textarea`), `type` (optional `Select`), `status` (optional `Select`), `priority` (optional `Select`), `assigneeAgentId` (optional `Select`), `projectId` (optional `Select`), `goalId` (optional `Select`).
- Each optional `Select` populates from its respective query. All optional selects have a "None" default item.
- Only `title` is validated.
- Status/priority/type values displayed via `humanize()`.

**`web/src/features/issues/issue-list-page.tsx`** — wire "Create Issue" button.

**`web/src/features/dashboard/dashboard-page.tsx`** — wire "New Issue" button.

---

#### REFACTOR

- Consolidate the three `useQuery` calls (agents, projects, goals) into named variables with meaningful loading states.
- Use `IssueStatus` and `IssuePriority` union types to generate Select option arrays at module level rather than hardcoding strings inline.

---

#### Acceptance Criteria

- [ ] "Create Issue" on `IssueListPage` and "New Issue" on `DashboardPage` open dialog (REQ-005)
- [ ] Only `title` is a required field (REQ-039)
- [ ] Assignee, project, goal selects populated from squad data (REQ-005)
- [ ] `POST /api/squads/{squadId}/issues` called on submit (REQ-010)
- [ ] `queryKeys.issues.list(squadId)` invalidated (REQ-010, REQ-037)
- [ ] Success toast "Issue created" (REQ-035)
- [ ] Error toast destructive on failure (REQ-036)
- [ ] Submit disabled + spinner while pending (REQ-030)
- [ ] Hook co-located in `web/src/features/issues/` (REQ-072)

**Files to create:**
- `web/src/features/issues/use-create-issue.ts`
- `web/src/features/issues/use-create-issue.test.ts`
- `web/src/features/issues/create-issue-dialog.tsx`
- `web/e2e/issue.spec.ts`

**Files to modify:**
- `web/src/features/issues/issue-list-page.tsx`
- `web/src/features/dashboard/dashboard-page.tsx`

---

### [x] Task 2.4 — Project + Goal create dialogs

**Requirements:** REQ-006, REQ-007, REQ-011, REQ-012, REQ-035, REQ-036, REQ-037, REQ-039, REQ-055, REQ-072, REQ-073

**Context:**
Project and goal create dialogs are simpler (2–3 fields each), but the goal dialog has a special `parentId` validation requirement (REQ-055). Both are wired in one task due to their similarity.

---

#### RED — Write failing tests

Create `web/src/features/projects/use-create-project.test.ts`:
```ts
// Test 1: calls api.post("/squads/{squadId}/projects", data)
// Test 2: invalidates queryKeys.projects.list(squadId) on success
// Test 3: toasts "Project created"
```

Create `web/src/features/goals/use-create-goal.test.ts`:
```ts
// Test 4: calls api.post("/squads/{squadId}/goals", data)
// Test 5: invalidates queryKeys.goals.list(squadId) on success
// Test 6: toasts "Goal created"
```

Add to `web/e2e/project.spec.ts`:
```ts
// Test 7: "Create Project" button opens dialog
// Test 8: Name required validation
// Test 9: Valid submission adds project to list
```

Add to `web/e2e/goal.spec.ts`:
```ts
// Test 10: "Create Goal" button opens dialog
// Test 11: Title required validation
// Test 12: parentId select populated from existing goals
// Test 13: Valid submission with parentId adds child goal to list
// Test 14: Parent breadcrumb appears on GoalDetailPage when parentId set (REQ-058)
```

---

#### GREEN — Minimum implementation

**`web/src/features/projects/use-create-project.ts`** — create:
- `mutationFn: ({ squadId, data }) => api.post<Project>("/squads/${squadId}/projects", data)`.

**`web/src/features/projects/create-project-dialog.tsx`** — create:
- Props: `{ open, onOpenChange, squadId }`. Fields: `name` (required), `description` (optional).

**`web/src/features/goals/use-create-goal.ts`** — create:
- `mutationFn: ({ squadId, data }) => api.post<Goal>("/squads/${squadId}/goals", data)`.

**`web/src/features/goals/create-goal-dialog.tsx`** — create:
- Props: `{ open, onOpenChange, squadId }`.
- Fields: `title` (required), `description` (optional), `parentId` (optional `Select` from squad goals).
- Validation (REQ-055): if `parentId` is set, confirm the selected goal exists in the goals query for this `squadId`. Since goals are fetched from the squad-scoped endpoint this always holds, but guard against stale Select state: if selected ID not found in goals list, display `errors.parentId = "Selected parent goal does not belong to this squad"`.

**Wire list pages:**
- `web/src/features/projects/project-list-page.tsx` — wire "Create Project" button.
- `web/src/features/goals/goal-list-page.tsx` — wire "Create Goal" button.

**Wire dashboard:**
- `web/src/features/dashboard/dashboard-page.tsx` — wire "New Project" button with `<CreateProjectDialog>`.

---

#### REFACTOR

- Both dialogs share the exact same two-field structure (name/title + description). Consider a generic `SimpleCreateDialog` — but only if the pattern is cleaner, not just DRY for its own sake.
- Ensure goal `parentId` Select has a "No parent" item as the default first option.

---

#### Acceptance Criteria

- [ ] "Create Project" opens dialog with `name` (required) + `description` (optional) (REQ-006)
- [ ] `POST /api/squads/{squadId}/projects` called; `queryKeys.projects.list(squadId)` invalidated (REQ-011, REQ-037)
- [ ] "Create Goal" opens dialog with `title` (required), `description` (optional), `parentId` (optional select) (REQ-007)
- [ ] `parentId` validated against loaded squad goals before submit (REQ-055)
- [ ] `POST /api/squads/{squadId}/goals` called; `queryKeys.goals.list(squadId)` invalidated (REQ-012, REQ-037)
- [ ] Success toasts "Project created" / "Goal created" (REQ-035)
- [ ] Error toasts destructive on failure (REQ-036)
- [ ] Submit disabled + spinner while pending (REQ-030)
- [ ] Hooks co-located in `web/src/features/projects/` and `web/src/features/goals/` (REQ-072)

**Files to create:**
- `web/src/features/projects/use-create-project.ts`
- `web/src/features/projects/use-create-project.test.ts`
- `web/src/features/projects/create-project-dialog.tsx`
- `web/src/features/goals/use-create-goal.ts`
- `web/src/features/goals/use-create-goal.test.ts`
- `web/src/features/goals/create-goal-dialog.tsx`
- `web/e2e/project.spec.ts`
- `web/e2e/goal.spec.ts`

**Files to modify:**
- `web/src/features/projects/project-list-page.tsx`
- `web/src/features/goals/goal-list-page.tsx`
- `web/src/features/dashboard/dashboard-page.tsx`

---

## Batch 3: Detail Page Edit Mode + Status Transitions + Comments

---

### [x] Task 3.1 — Squad + Project + Goal detail edit mode

**Requirements:** REQ-013, REQ-016, REQ-017, REQ-027, REQ-028, REQ-033, REQ-035, REQ-036, REQ-037, REQ-040, REQ-065, REQ-072, REQ-073

**Context:**
Three detail pages with similar two-to-four-field edit forms: `SquadDetailPage`, `ProjectDetailPage`, `GoalDetailPage`. Grouping them saves repetition. Each follows the same pattern: `isEditing` boolean state, `form` state seeded from the query cache at edit-click time, "Save"/"Cancel" buttons, and a mutation hook.

---

#### RED — Write failing tests

Add to `web/e2e/squad.spec.ts`:
```ts
// Test 1: Clicking "Edit" on SquadDetailPage enters edit mode
// Test 2: Edit form is pre-populated with current squad values (REQ-040)
// Test 3: Clicking "Cancel" exits edit mode without saving
// Test 4: Editing name and clicking "Save" calls PATCH /api/squads/{id}
// Test 5: Detail page exits edit mode on success
// Test 6: Success toast "Squad updated"
// Test 7: Error toast shown on API failure
```

Add to `web/e2e/project.spec.ts`:
```ts
// Test 8–14: Same pattern for ProjectDetailPage (edit, cancel, save, toast)
```

Add to `web/e2e/goal.spec.ts`:
```ts
// Test 15–21: Same pattern for GoalDetailPage (edit, cancel, save, toast)
// Test 22: Parent goal breadcrumb shown when goal has parentId (REQ-058)
```

---

#### GREEN — Minimum implementation

**`web/src/features/squads/use-update-squad.ts`** — create:
- `mutationFn: ({ id, data }) => api.patch<Squad>("/squads/${id}", data)`.
- `onSuccess(_, { id })`: invalidate `queryKeys.squads.detail(id)` and `queryKeys.squads.all`; toast "Squad updated".

**`web/src/features/squads/squad-detail-page.tsx`** — add edit mode:
- `const [isEditing, setIsEditing] = useState(false)`.
- `const [form, setForm] = useState<Partial<UpdateSquadRequest>>({})`.
- "Edit" click: `setForm({ name: squad.name, description: squad.description }); setIsEditing(true)`.
- In edit mode: render `Input` for name, `Textarea` for description, "Save" + "Cancel" buttons.
- "Cancel": `setIsEditing(false)`.
- "Save": call `useUpdateSquad().mutate({ id: squad.id, data: form })`, `onSuccess`: `setIsEditing(false)`.
- Use `useBlocker` to intercept nav while `isEditing`.

**`web/src/features/projects/use-update-project.ts`** — create:
- Mirrors `use-update-squad.ts` pattern; toast "Project updated".

**`web/src/features/projects/project-detail-page.tsx`** — add edit mode (same pattern).

**`web/src/features/goals/use-update-goal.ts`** — create:
- toast "Goal updated".

**`web/src/features/goals/goal-detail-page.tsx`** — add edit mode:
- Add parent breadcrumb: if `goal.parentId` is set, fetch `queryKeys.goals.detail(goal.parentId)` and render `<Link to="/goals/{id}">{parentGoal.title}</Link>` above the goal title (REQ-058).

---

#### REFACTOR

- Extract a reusable `useEditMode<T>` hook (optional) if the `isEditing / form / setForm` pattern is repeated identically across more than two pages. Only if it reduces net code.
- Confirm `useBlocker` import from `react-router` (not `react-router-dom`) per the project's stack.
- Ensure `UpdateSquadRequest`, `UpdateProjectRequest`, `UpdateGoalRequest` types are used for mutation variables (REQ-073).

---

#### Acceptance Criteria

- [ ] "Edit" button on each detail page enters edit mode (REQ-027)
- [ ] Edit form pre-populated from query cache (REQ-040)
- [ ] "Cancel" exits edit mode, discards changes (REQ-028)
- [ ] "Save" calls `PATCH` with correct request type (REQ-013/REQ-016/REQ-017)
- [ ] On success: exits edit mode, invalidates both detail and list query keys (REQ-037)
- [ ] Success toast "Squad/Project/Goal updated" (REQ-035)
- [ ] Error toast on failure (REQ-036)
- [ ] Navigation blocked while in edit mode (REQ-033)
- [ ] Parent goal breadcrumb shown on `GoalDetailPage` when `parentId` is set (REQ-058)
- [ ] Mutation hooks co-located in respective feature dirs (REQ-072)

**Files to create:**
- `web/src/features/squads/use-update-squad.ts`
- `web/src/features/projects/use-update-project.ts`
- `web/src/features/goals/use-update-goal.ts`

**Files to modify:**
- `web/src/features/squads/squad-detail-page.tsx`
- `web/src/features/projects/project-detail-page.tsx`
- `web/src/features/goals/goal-detail-page.tsx`

---

### [x] Task 3.2 — Agent detail: edit mode + status transitions

**Requirements:** REQ-014, REQ-018, REQ-027, REQ-028, REQ-033, REQ-034, REQ-035, REQ-036, REQ-037, REQ-040, REQ-047, REQ-048, REQ-049, REQ-056, REQ-057, REQ-066, REQ-072, REQ-073

**Context:**
`AgentDetailPage` is the most complex detail page. It combines: inline edit mode (many fields including optional `adapterConfig`/`runtimeConfig` JSON blobs that are hidden in read mode), and three conditional status transition buttons (Approve / Pause / Resume) that are mutually exclusive based on current status.

---

#### RED — Write failing tests

Add to `web/e2e/agent.spec.ts`:
```ts
// Test 1: Edit/Cancel/Save pattern for AgentDetailPage
// Test 2: Edit form pre-populated with all agent fields (REQ-040)
// Test 3: adapterConfig and runtimeConfig NOT visible in read-only view (REQ-066)
// Test 4: adapterConfig and runtimeConfig visible as Textarea fields in edit mode (REQ-066)
// Test 5: agent.status === "pending_approval" → "Approve" button visible (REQ-047)
// Test 6: agent.status === "active" → "Pause" button visible (REQ-048)
// Test 7: agent.status === "idle" → "Pause" button visible (REQ-048)
// Test 8: agent.status === "paused" → "Resume" button visible (REQ-049)
// Test 9: Clicking "Approve" calls PATCH with { status: "active" } → toast "Agent status updated"
// Test 10: All transition buttons disabled while mutation isPending (REQ-034)
// Test 11: requireApprovalForNewAgents banner shown on AgentListPage when flag is true (REQ-056)
```

---

#### GREEN — Minimum implementation

**`web/src/features/agents/use-update-agent.ts`** — create:
```ts
export function useUpdateAgent(options?: { successMessage?: string }) {
  // mutationFn: ({ id, data }) => api.patch<Agent>("/agents/${id}", data)
  // onSuccess: invalidate detail + list (using data.squadId); toast successMessage ?? "Agent updated"
}
```

**`web/src/features/agents/agent-detail-page.tsx`** — add:
1. Edit mode: same `isEditing / form` pattern. Edit form includes all `UpdateAgentRequest` fields. `adapterConfig` and `runtimeConfig` rendered as `Textarea` (JSON text) only in edit mode (REQ-066).
2. Status transition buttons below the status badge:
   ```tsx
   const updateAgent = useUpdateAgent({ successMessage: "Agent status updated" });
   // Approve: pending_approval → active
   // Pause: active | idle → paused
   // Resume: paused → active
   // All disabled when updateAgent.isPending
   ```
3. `useBlocker` while `isEditing`.

**`web/src/features/agents/agent-list-page.tsx`** — add approval banner:
- Fetch squad detail (`queryKeys.squads.detail(squadId)`). If `squad.requireApprovalForNewAgents`, render `<div className="...">New agents require approval before activation</div>` above the table (REQ-056).

---

#### REFACTOR

- The `adapterConfig` and `runtimeConfig` Textarea fields should display pretty-printed JSON (`JSON.stringify(value, null, 2)`) and parse on save. Add a try/catch for invalid JSON with an inline field error.
- Extract `AgentStatusActions` as a sub-component within the file to keep `AgentDetailPage` readable.

---

#### Acceptance Criteria

- [ ] Edit mode with all fields pre-populated from cache (REQ-040)
- [ ] `adapterConfig`/`runtimeConfig` hidden in read mode, shown as Textarea in edit mode (REQ-066)
- [ ] Cancel exits edit mode without mutation (REQ-028)
- [ ] Save calls `PATCH /api/agents/{id}`; exits edit mode on success (REQ-014)
- [ ] Correct invalidation of `detail(id)` and `list(squadId)` (REQ-037)
- [ ] Toast "Agent updated" on edit save (REQ-035)
- [ ] Approve button shown only when `status === "pending_approval"` (REQ-047)
- [ ] Pause button shown when `status === "active" || "idle"` (REQ-048)
- [ ] Resume button shown only when `status === "paused"` (REQ-049)
- [ ] Status transitions call `PATCH` with `{ status }` and toast "Agent status updated" (REQ-018)
- [ ] All transition buttons disabled while `isPending` (REQ-034)
- [ ] Approval notice banner on `AgentListPage` when `requireApprovalForNewAgents` (REQ-056)
- [ ] Hook co-located in `web/src/features/agents/` (REQ-072)

**Files to create:**
- `web/src/features/agents/use-update-agent.ts`

**Files to modify:**
- `web/src/features/agents/agent-detail-page.tsx`
- `web/src/features/agents/agent-list-page.tsx`

---

### [x] Task 3.3 — Issue detail: edit mode + status transitions + linked metadata

**Requirements:** REQ-015, REQ-019, REQ-027, REQ-028, REQ-033, REQ-034, REQ-035, REQ-036, REQ-037, REQ-040, REQ-045, REQ-046, REQ-059, REQ-060, REQ-061, REQ-072, REQ-073

**Context:**
`IssueDetailPage` combines: inline edit mode, status transition buttons (driven by `issueStatusTransitions` map), and three optional linked-entity breadcrumbs (parent issue, project, goal).

---

#### RED — Write failing tests

Add to `web/e2e/issue.spec.ts`:
```ts
// Test 1: Edit/Cancel/Save pattern for IssueDetailPage
// Test 2: Edit form pre-populated with current issue values (REQ-040)
// Test 3: Status transition buttons rendered for current status (REQ-045)
// Test 4: No transition buttons rendered when status is terminal with empty transitions (REQ-046)
// Test 5: Clicking a transition button calls PATCH with { status } and toasts "Issue status updated"
// Test 6: All transition buttons disabled while mutation isPending (REQ-034)
// Test 7: Parent issue breadcrumb shown when parentId set (REQ-059)
// Test 8: Project link shown when projectId set (REQ-060)
// Test 9: Goal link shown when goalId set (REQ-061)
```

---

#### GREEN — Minimum implementation

**`web/src/features/issues/use-update-issue.ts`** — create:
```ts
export function useUpdateIssue(options?: { successMessage?: string }) {
  // mutationFn: ({ id, data }) => api.patch<Issue>("/issues/${id}", data)
  // onSuccess(data, { id }): invalidate issues.detail(id) + issues.list(data.squadId)
  // toast options.successMessage ?? "Issue updated"
}
```

**`web/src/features/issues/issue-detail-page.tsx`** — add:
1. Edit mode using same `isEditing / form` pattern (fields match `UpdateIssueRequest`).
2. Status transition buttons:
   ```tsx
   const transitions = issueStatusTransitions[issue.status];
   if (transitions.length > 0) render transition buttons for each target
   ```
   - Each button: `onClick={() => updateIssue.mutate({ id: issue.id, data: { status: target } })}`
   - All disabled when `updateIssue.isPending` (REQ-034).
3. Linked metadata:
   - If `issue.parentId`: fetch `queryKeys.issues.detail(issue.parentId)` and render `<Link>` breadcrumb (REQ-059).
   - If `issue.projectId`: fetch `queryKeys.projects.detail(issue.projectId)` and render linked project name (REQ-060).
   - If `issue.goalId`: fetch `queryKeys.goals.detail(issue.goalId)` and render linked goal title (REQ-061).
4. Use `useBlocker` while `isEditing`.

---

#### REFACTOR

- Import and reuse `humanize()` for all status/priority/type values displayed in read mode and transition buttons.
- Extract `IssueStatusTransitions` as a named sub-component to separate the transition rendering from the main page.
- The `useUpdateIssue` `successMessage` option is passed at the call site (`"Issue status updated"` vs default `"Issue updated"`), not stored on the hook instance — this matches the agent pattern.

---

#### Acceptance Criteria

- [ ] Edit/Cancel/Save cycle works; form pre-populated from cache (REQ-027, REQ-028, REQ-040)
- [ ] `PATCH /api/issues/{id}` called with `UpdateIssueRequest` on save (REQ-015)
- [ ] Correct invalidation of detail + list keys on success (REQ-037)
- [ ] Toast "Issue updated" on save success (REQ-035)
- [ ] Transition buttons rendered per `issueStatusTransitions[issue.status]` (REQ-045)
- [ ] No transition buttons rendered when transitions array is empty (REQ-046)
- [ ] Transition calls `PATCH` with `{ status }` and toasts "Issue status updated" (REQ-019)
- [ ] All transition buttons disabled while `isPending` (REQ-034)
- [ ] Parent breadcrumb shown when `parentId` set (REQ-059)
- [ ] Project link shown when `projectId` set (REQ-060)
- [ ] Goal link shown when `goalId` set (REQ-061)
- [ ] Hook co-located in `web/src/features/issues/` (REQ-072)

**Files to create:**
- `web/src/features/issues/use-update-issue.ts`

**Files to modify:**
- `web/src/features/issues/issue-detail-page.tsx`

---

### [x] Task 3.4 — Issue comments: `useAddComment` + `IssueComments`

**Requirements:** REQ-020, REQ-035, REQ-036, REQ-037, REQ-050, REQ-051, REQ-072, REQ-073

**Context:**
`IssueDetailPage` needs a comments section at the bottom with a threaded comment list and a submit form. This is a standalone component that owns its own query and mutation.

---

#### RED — Write failing tests

Create `web/src/features/issues/use-add-comment.test.ts`:
```ts
// Test 1: calls api.post("/issues/{issueId}/comments", { body })
// Test 2: on success: invalidates queryKeys.issues.comments(issueId)
// Test 3: on success: toasts "Comment added"
// Test 4: on error: toasts destructive
```

Add to `web/e2e/issue.spec.ts`:
```ts
// Test 5: "No comments yet" shown when comment list is empty (REQ-051)
// Test 6: Comments rendered in chronological order with authorName, date, body (REQ-050)
// Test 7: "Add Comment" button disabled when textarea is empty
// Test 8: Submitting a comment clears the textarea and shows "Comment added" toast
// Test 9: New comment appears in the list after submission
```

---

#### GREEN — Minimum implementation

**`web/src/features/issues/use-add-comment.ts`** — create:
```ts
// mutationFn: ({ issueId, body }) => api.post<IssueComment>("/issues/${issueId}/comments", { body })
// onSuccess(_, { issueId }): invalidate queryKeys.issues.comments(issueId); toast "Comment added"
// onError: toast destructive
```

**`web/src/features/issues/issue-comments.tsx`** — create:
- Props: `{ issueId: string }`.
- `useQuery({ queryKey: queryKeys.issues.comments(issueId), queryFn: () => api.get<IssueComment[]>("/issues/${issueId}/comments") })`.
- If no comments: `<p className="text-sm text-muted-foreground">No comments yet.</p>` (REQ-051).
- Map comments in order: `authorName`, `createdAt` formatted, `body`.
- Local `body` state (string). `Textarea` bound to `body`.
- "Add Comment" `Button` disabled when `body.trim() === ""` or `addComment.isPending`.
- `onSuccess` from `useAddComment`: clear `body` to `""`.

**`web/src/features/issues/issue-detail-page.tsx`** — render `<IssueComments issueId={issue.id} />` at the bottom.

---

#### REFACTOR

- Use a `formatDateTime` helper from `web/src/lib/utils.ts` for `createdAt` (if one already exists) or add a simple wrapper around `Intl.DateTimeFormat`.
- Each comment card should use consistent spacing matching existing detail page card styles.

---

#### Acceptance Criteria

- [ ] Comments fetched from `GET /api/issues/{id}/comments` (REQ-050)
- [ ] Comments rendered in chronological order with `authorName`, date, `body` (REQ-050)
- [ ] "No comments yet." empty state shown when list is empty (REQ-051)
- [ ] Submit button disabled when textarea is empty or mutation is pending
- [ ] `POST /api/issues/{id}/comments` called with `{ body }` (REQ-020)
- [ ] Textarea cleared on success (REQ-020)
- [ ] `queryKeys.issues.comments(issueId)` invalidated on success (REQ-037)
- [ ] Toast "Comment added" (REQ-035)
- [ ] Error toast destructive on failure (REQ-036)
- [ ] Hook co-located in `web/src/features/issues/` (REQ-072)

**Files to create:**
- `web/src/features/issues/use-add-comment.ts`
- `web/src/features/issues/use-add-comment.test.ts`
- `web/src/features/issues/issue-comments.tsx`

**Files to modify:**
- `web/src/features/issues/issue-detail-page.tsx`

---

## Batch 4: Issue Filtering + Pagination + Badge Colours

---

### [x] Task 4.1 — `IssueFilters` component + URL-driven filter state on `IssueListPage`

**Requirements:** REQ-021, REQ-022, REQ-023, REQ-024, REQ-038, REQ-054, REQ-062, REQ-069

**Context:**
Issue filtering requires three Select controls (status, priority, assignee) whose values are mirrored to URL query params so that the browser back button and sharing links work correctly. `IssueListPage` owns the URL state and passes the derived `IssueFilters` object to both the filter component and the query key.

---

#### RED — Write failing tests

Create `web/src/features/issues/issue-filters.test.tsx`:
```ts
// Test 1: Renders three Select components (status, priority, assignee)
// Test 2: Status select options include all IssueStatus values, formatted via humanize()
// Test 3: Priority select options are critical, high, medium, low
// Test 4: Assignee select options are populated from agents prop
// Test 5: onChange called with new filter when status changes
// Test 6: onChange called with filter key omitted (undefined) when select cleared to ""
// Test 7: "Clear Filters" button not shown when no filters are active
// Test 8: "Clear Filters" button shown when any filter is active
// Test 9: Clicking "Clear Filters" calls onChange({})
```

Add to `web/e2e/issue.spec.ts`:
```ts
// Test 10: Selecting status filter updates URL query param ?status=
// Test 11: Selecting priority filter updates URL query param ?priority=
// Test 12: Selecting assignee filter updates URL query param ?assigneeAgentId=
// Test 13: Filtered query re-fetches with correct URL params
// Test 14: "Clear Filters" button removes all query params
// Test 15: Refreshing page with filter params restores filter state
```

---

#### GREEN — Minimum implementation

**`web/src/features/issues/issue-filters.tsx`** — create:
- Props: `{ filters: IssueFilters; agents: Agent[]; onChange: (filters: IssueFilters) => void }`.
- Three `Select` components. Each `onValueChange`: call `onChange({ ...filters, [field]: value || undefined })`.
- "Clear Filters" button (variant `"ghost"`) shown when any filter key is non-undefined.

**`web/src/lib/utils.ts`** — add `buildQueryString`:
```ts
export function buildQueryString(filters: IssueFilters, pagination: { offset: number; limit: number }): string {
  const params = new URLSearchParams();
  if (filters.status) params.set("status", filters.status);
  if (filters.priority) params.set("priority", filters.priority);
  if (filters.assigneeAgentId) params.set("assigneeAgentId", filters.assigneeAgentId);
  if (pagination.offset > 0) params.set("offset", String(pagination.offset));
  params.set("limit", String(pagination.limit));
  return params.toString();
}
```

**`web/src/features/issues/issue-list-page.tsx`** — refactor:
1. Add `const [searchParams, setSearchParams] = useSearchParams()`.
2. Derive filters: `const filters: IssueFilters = { status: searchParams.get("status") ?? undefined, ... }`.
3. Derive offset: `const offset = Number(searchParams.get("offset") ?? "0")`.
4. Query key: `queryKeys.issues.list(squadId!, filters)`.
5. Query URL: `/squads/${squadId}/issues?${buildQueryString(filters, { offset, limit: 20 })}`.
6. `handleFilterChange(newFilters)`: `setSearchParams` for each defined key, reset offset to "0".
7. Render `<IssueFilters filters={filters} agents={agents} onChange={handleFilterChange} />` above the table.

---

#### REFACTOR

- Extract `buildQueryString` tests to `web/src/lib/utils.test.ts` (extend Task 0.1 test file).
- Confirm all `IssueStatus` values for the status Select are derived from the `issueStatusTransitions` keys (or the type union directly) rather than a hardcoded string array.
- Status and priority option labels use `humanize()` (REQ-069).

---

#### Acceptance Criteria

- [ ] Three filter Selects rendered: status, priority, assignee (REQ-021, REQ-022, REQ-023)
- [ ] Filter changes update URL query params and trigger query refetch (REQ-021–REQ-023)
- [ ] Assignee select populated from squad agents (REQ-023)
- [ ] "Clear Filters" button shown when any filter active (REQ-054)
- [ ] "Clear Filters" removes all filter params and refetches unfiltered (REQ-024)
- [ ] Page refresh with filter params restores filter state from URL (REQ-038)
- [ ] Status/priority values displayed via `humanize()` (REQ-069)
- [ ] `staleTime: 30_000` means navigating back does not re-fetch within 30s window (REQ-062)

**Files to create:**
- `web/src/features/issues/issue-filters.tsx`
- `web/src/features/issues/issue-filters.test.tsx`

**Files to modify:**
- `web/src/features/issues/issue-list-page.tsx`
- `web/src/lib/utils.ts`

---

### [x] Task 4.2 — Issue list pagination wired end-to-end

**Requirements:** REQ-025, REQ-026, REQ-052, REQ-053, REQ-064

**Context:**
`PaginationControls` was built in Task 1.3. This task wires it into `IssueListPage` alongside the filter state, so offset is tracked in the URL and Previous/Next navigation works correctly.

---

#### RED — Write failing tests

Add to `web/e2e/issue.spec.ts`:
```ts
// Test 1: Pagination controls not shown when total <= 20 (REQ-053)
// Test 2: Pagination controls shown when total > 20 (REQ-052)
// Test 3: Range text shows "1–20 of {total}"
// Test 4: "Previous" button is disabled on first page
// Test 5: Clicking "Next" navigates to next page (offset=20), fetches correct data
// Test 6: Clicking "Previous" on page 2 returns to page 1 (offset=0)
// Test 7: "Next" disabled on last page
// Test 8: Changing a filter resets offset to 0
```

---

#### GREEN — Minimum implementation

**`web/src/features/issues/issue-list-page.tsx`** — complete pagination wiring:
1. Read `offset` from `searchParams` (already added in Task 4.1).
2. Render `<PaginationControls total={data?.pagination.total ?? 0} offset={offset} limit={20} onPageChange={(o) => setSearchParams({ ...Object.fromEntries(searchParams), offset: String(o) })} />` below the table.
3. Verify `handleFilterChange` resets offset to `"0"` whenever filters change.

The default page size is `20` (REQ-064) — this is hardcoded as a named constant `const PAGE_SIZE = 20` at the top of the file.

---

#### REFACTOR

- Verify that `PaginationControls` returns `null` for list pages with ≤ 20 results (zero DOM nodes, not a hidden element), satisfying REQ-053.
- Ensure `offset` is reset to `0` in `handleFilterChange` and not left as a stale URL param after filter changes.

---

#### Acceptance Criteria

- [ ] `PaginationControls` rendered below issue table (REQ-052)
- [ ] Controls absent when `total <= 20` (REQ-053)
- [ ] "Next" advances offset by 20 and refetches (REQ-025)
- [ ] "Previous" decrements offset by 20 (clamped to 0) and refetches (REQ-026)
- [ ] "Previous" disabled on first page (offset=0)
- [ ] "Next" disabled on last page (offset + limit >= total)
- [ ] Changing a filter resets offset to 0
- [ ] Default page size is 20 items (REQ-064)

**Files to modify:**
- `web/src/features/issues/issue-list-page.tsx`

---

### [x] Task 4.3 — Loading skeletons + status/priority badge colour maps

**Requirements:** REQ-031, REQ-032, REQ-069

**Context:**
The design specifies exact skeleton DOM structures for list pages and detail pages. This task audits all list and detail pages, adds missing skeletons, and replaces any ad-hoc status/priority rendering with `humanize()` and the `agentStatusColors` badge map.

---

#### RED — Write failing tests

Add to each E2E spec (`agent.spec.ts`, `issue.spec.ts`, `project.spec.ts`, `goal.spec.ts`, `squad.spec.ts`):
```ts
// Test (per page): while query is loading, three h-16 muted skeleton rows are shown instead of the table
// Test (per detail page): while query is loading, h-8 title skeleton and h-32 content skeleton are shown
```

Unit tests in `web/src/components/shared/`:
```ts
// Test: LoadingSkeletonList renders exactly 3 skeleton rows of class h-16
// Test: LoadingSkeletonDetail renders h-8 and h-32 divs
```

---

#### GREEN — Minimum implementation

**`web/src/lib/utils.ts`** — ensure `humanize` is exported (done in Task 0.1).

Audit each list page for loading skeleton. The design specifies:
```tsx
// List page skeleton (3 rows)
{isLoading && Array.from({ length: 3 }).map((_, i) => (
  <div key={i} className="h-16 rounded-md bg-muted animate-pulse" />
))}
```

Audit each detail page for loading skeleton:
```tsx
{isLoading && (
  <>
    <div className="h-8 w-48 bg-muted animate-pulse rounded" />
    <div className="h-32 bg-muted animate-pulse rounded mt-4" />
  </>
)}
```

Replace any inline `.replace("_", " ")` or `.charAt(0).toUpperCase()` calls throughout all list and detail pages with `humanize()`.

Apply `agentStatusColors[agent.status]` badge map in `AgentListPage` and `AgentDetailPage` wherever status `Badge` is rendered.

---

#### REFACTOR

- Extract shared skeleton structures to `web/src/components/shared/loading-skeletons.tsx` (two named exports: `ListPageSkeleton` and `DetailPageSkeleton`) if they are used in 3+ pages, to avoid copy-paste.
- Verify that the `agentStatusColors` map covers all `AgentStatus` values (it does per `agent.ts`). If any badge renders a status not in the map, add a fallback `bg-gray-100 text-gray-800`.

---

#### Acceptance Criteria

- [ ] All 5 list pages show 3 `h-16 animate-pulse` skeleton rows while loading (REQ-031)
- [ ] All 5 detail pages show title + content skeletons while loading (REQ-032)
- [ ] All status/priority/role values displayed via `humanize()` (REQ-069)
- [ ] Agent status badges use `agentStatusColors` map for colour
- [ ] No raw underscore strings visible to users in any list or detail page

**Files to modify:**
- `web/src/features/squads/squad-list-page.tsx`
- `web/src/features/squads/squad-detail-page.tsx`
- `web/src/features/agents/agent-list-page.tsx`
- `web/src/features/agents/agent-detail-page.tsx`
- `web/src/features/issues/issue-list-page.tsx`
- `web/src/features/issues/issue-detail-page.tsx`
- `web/src/features/projects/project-list-page.tsx`
- `web/src/features/projects/project-detail-page.tsx`
- `web/src/features/goals/goal-list-page.tsx`
- `web/src/features/goals/goal-detail-page.tsx`

---

## Notes

### Implementation Notes

- All new files must be TypeScript (`.ts` / `.tsx`) with strict mode compatible types.
- No new npm dependencies. All shadcn/ui and Base UI components are already installed.
- The `useToast` hook is already wired in `web/src/hooks/use-toast.ts` — never import from another toast library.
- `queryKeys` factory in `web/src/lib/query.ts` covers all required keys. Do not add new keys there unless `queryKeys.issues.comments(issueId)` is confirmed present (it is, at line 33).
- Mutation hooks all use `retry: 0` inherited from the global `QueryClient` config (REQ-070). No need to set this per hook.
- `staleTime: 30_000` is set globally — no need to repeat it in individual `useQuery` calls (REQ-062).

### Blockers

None identified.

### Future Improvements

- Real-time updates via SSE (feature 11-agent-runtime)
- Activity log feed (feature 09-activity-log)
- Budget/cost UI (feature 10-cost-events-budget)
- Drag-and-drop reordering
- Bulk operations on issue list
