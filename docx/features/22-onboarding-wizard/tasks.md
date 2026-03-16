# Onboarding Wizard — Tasks

## Task Breakdown

### Task 1: Registry ListAvailable + AdapterInfo type
**REQs:** REQ-012, REQ-013
**Files:** `internal/adapter/registry.go`, `internal/adapter/adapter.go`

- [ ] RED: Write test for `Registry.ListAvailable()` — returns registered adapters with type, availability, and models
- [ ] RED: Write test for `ListAvailable()` — marks unavailable adapters with reason
- [ ] GREEN: Add `AdapterInfo` struct to `adapter.go`
- [ ] GREEN: Implement `ListAvailable()` on Registry
- [ ] REFACTOR: Ensure deterministic ordering (sort by type name)

---

### Task 2: AdapterHandler — GET /api/adapters
**REQs:** REQ-012, REQ-013
**Files:** `internal/server/handlers/adapter_handler.go`, `internal/server/handlers/adapter_handler_test.go`

- [ ] RED: Write integration test — `GET /api/adapters` returns list of adapters with models
- [ ] RED: Write test — endpoint requires authentication (401 without JWT)
- [ ] GREEN: Create `AdapterHandler` struct with `registry` dependency
- [ ] GREEN: Implement `List` handler
- [ ] GREEN: Register route in `routes.go` / `run.go`
- [ ] REFACTOR: Clean up response types

---

### Task 3: AdapterHandler — GET /api/adapters/{type}/models
**REQs:** REQ-012
**Files:** `internal/server/handlers/adapter_handler.go`

- [ ] RED: Write test — returns models for `claude_local`
- [ ] RED: Write test — returns 404 for unknown adapter type
- [ ] RED: Write test — returns empty array for `process` adapter (no models)
- [ ] GREEN: Implement `ListModels` handler
- [ ] GREEN: Register route

---

### Task 4: AdapterHandler — POST /api/adapters/{type}/test-environment
**REQs:** REQ-010, REQ-014
**Files:** `internal/server/handlers/adapter_handler.go`

- [ ] RED: Write test — returns `{available: true, message: "..."}` for working adapter
- [ ] RED: Write test — returns `{available: false, message: "..."}` for unavailable adapter
- [ ] RED: Write test — returns 404 for unknown adapter type
- [ ] GREEN: Implement `TestEnvironment` handler (delegates to adapter's `TestEnvironment(TestLevelBasic)`)
- [ ] GREEN: Register route

---

### Task 5: Extend POST /api/squads — captain adapter config
**REQs:** REQ-007
**Files:** `internal/server/handlers/squad_handler.go`, `internal/server/handlers/squad_integration_test.go`

- [ ] RED: Write test — create squad with `captainAdapterType: "claude_local"` + `captainAdapterConfig: {workingDir: "/tmp", model: "sonnet"}` → captain created with those values
- [ ] RED: Write test — create squad WITHOUT new fields → captain defaults to `claude_local` with empty config (backward compatibility)
- [ ] RED: Write test — create squad with invalid adapter type → 400 error
- [ ] GREEN: Add `CaptainAdapterType *string` and `CaptainAdapterConfig json.RawMessage` to `createSquadRequest`
- [ ] GREEN: Update captain creation logic to use provided values or fall back to defaults
- [ ] GREEN: Validate adapter type against known enum values
- [ ] REFACTOR: Extract captain creation into helper function

---

### Task 6: Frontend — adapters API client
**REQs:** REQ-010, REQ-012
**Files:** `web/src/api/adapters.ts`, `web/src/types/adapter.ts`

- [ ] Create `web/src/types/adapter.ts` with `AdapterInfo`, `ModelDefinition`, `TestResult` types
- [ ] Create `web/src/api/adapters.ts` with `list()`, `models(type)`, `testEnvironment(type)` methods
- [ ] Verify types match backend response contracts

---

### Task 7: OnboardingWizard — container + state management
**REQs:** REQ-001, REQ-002, REQ-003, REQ-004, REQ-018, REQ-019
**Files:** `web/src/features/onboarding/onboarding-wizard.tsx`

- [ ] Create `web/src/features/onboarding/` directory
- [ ] Implement `OnboardingWizard` component with:
  - Full-screen layout (left form panel, right branding panel)
  - Step state (1-4) with forward/back navigation
  - `WizardState` holding all form data across steps
  - Close button (X) with escape key handler
  - Cmd/Ctrl+Enter keyboard shortcut to advance
  - Step progress indicator (step dots + "Step N of 4" text)
- [ ] Export `OnboardingWizard` with `onComplete(squadId)` callback prop

---

### Task 8: Step 1 — Squad Details
**REQs:** REQ-006
**Files:** `web/src/features/onboarding/step-squad-details.tsx`

- [ ] Implement Step 1 form: squadName, issuePrefix, description
- [ ] Validation: same rules as existing Create Squad dialog (name required, prefix 2-10 uppercase alphanumeric)
- [ ] Auto-generate issue prefix from squad name (first 3 uppercase letters)
- [ ] "Next" button stores data in wizard state, advances to step 2
- [ ] No API calls in this step

---

### Task 9: Step 2 — Captain + Adapter Config
**REQs:** REQ-007, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014
**Files:** `web/src/features/onboarding/step-captain-config.tsx`, `web/src/features/onboarding/adapter-config-fields.tsx`, `web/src/features/onboarding/use-adapter-test.ts`

- [ ] Implement captain fields: captainName (default "Captain"), captainShortName (auto-derived)
- [ ] Fetch available adapters via `GET /api/adapters`
- [ ] IF only one adapter → pre-select and hide selector (REQ-013)
- [ ] IF multiple adapters → show dropdown
- [ ] Implement `AdapterConfigFields` — conditional fields per adapter type:
  - `claude_local`: workingDir (text input), model (dropdown from adapter models), collapsible advanced (timeout, skipPermissions toggle)
  - `process`: command (text input), args (text input), workingDir
- [ ] Implement `useAdapterTest` hook — calls `POST /api/adapters/{type}/test-environment`
- [ ] "Test Environment" button with pass/warn/fail inline result display
- [ ] On test fail: show error details + "Proceed anyway" option (REQ-014)
- [ ] "Next" button: calls `POST /api/squads` with all step 1 + step 2 data
- [ ] Loading state while creating (REQ-011)
- [ ] Store returned squadId in wizard state
- [ ] Fetch captain agent ID after squad creation (for task assignment in step 3)

---

### Task 10: Step 3 — First Task (skippable)
**REQs:** REQ-008, REQ-016
**Files:** `web/src/features/onboarding/step-first-task.tsx`

- [ ] Implement task form: title (pre-filled), description (textarea, pre-filled with bootstrap instructions)
- [ ] "Skip" button → advance to step 4 without API call (REQ-016)
- [ ] "Next" button → `POST /api/issues` with title, description, assigneeAgentId = captainId
- [ ] Loading state while creating
- [ ] Error handling with retry

---

### Task 11: Step 4 — Launch
**REQs:** REQ-009
**Files:** `web/src/features/onboarding/step-launch.tsx`

- [ ] Show summary: squad name, captain name, adapter type, model, task title (if created)
- [ ] "Go to Dashboard" button → set active squad, call `onComplete(squadId)`, navigate to `/`

---

### Task 12: Dashboard integration — replace empty state
**REQs:** REQ-005, REQ-015
**Files:** `web/src/features/dashboard/dashboard-page.tsx`

- [ ] Replace empty state (no active squad) with `OnboardingWizard` component
- [ ] Pass `onComplete` callback that sets active squad and navigates
- [ ] Existing Create Squad dialog remains for "New Squad" quick action (REQ-015)
- [ ] Verify users with existing squads see normal dashboard (no wizard)

---

### Task 13: E2E Tests
**REQs:** All acceptance criteria
**Files:** `web/e2e/tests/onboarding-wizard.spec.ts`

- [ ] Test: New user → sees onboarding wizard → completes all steps → lands on dashboard with squad
- [ ] Test: New user → creates squad + captain with adapter config → captain has correct adapter type and config
- [ ] Test: New user → skips task step → lands on dashboard without issues
- [ ] Test: User with existing squads → dashboard shows normal view (no wizard)
- [ ] Test: Wizard close → no orphaned resources if closed before step 2
- [ ] Test: Back navigation preserves form data across steps

---

## Execution Order

```
Tasks 1-4 (backend, parallelizable)
  → Task 5 (backend, depends on Task 1 for registry)
    → Task 6 (frontend API client)
      → Tasks 7-11 (frontend components, sequential)
        → Task 12 (dashboard integration)
          → Task 13 (E2E tests)
```

**Parallel tracks:**
- Backend tasks 1-4 can be done in parallel
- Frontend tasks 7-11 are sequential (each step builds on the wizard container)
