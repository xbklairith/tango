# Tasks: Inbox System (Human-in-the-Loop Queue)

**Created:** 2026-03-15
**Status:** In Progress

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-INB-001 through REQ-INB-035

## Implementation Approach

Work bottom-up through the dependency graph: domain model and validation first, then database migration and sqlc queries, then the InboxService with its side-effects (SSE, wakeup, activity log), then the HTTP handler, then integration with BudgetEnforcementService and RunService, and finally React UI components.

## Progress Summary

- Total Tasks: 10
- Completed: 9
- In Progress: None
- Remaining: Task 09 (React UI)
- Test Coverage: TBD

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 01 — Domain Model: Inbox Types and Validation

**Requirements:** REQ-INB-021, REQ-INB-022
**Estimated time:** 30 min

#### Context

Define the inbox domain types (category, urgency, status, resolution) and validation functions. This is the foundation for all inbox logic — status machine transitions and resolution-per-category rules.

#### RED — Write Failing Tests

Write `internal/domain/inbox_test.go`:

1. `TestValidateInboxStatusTransition` — verify allowed transitions: `pending->acknowledged`, `pending->resolved`, `pending->expired`, `acknowledged->resolved`, `acknowledged->expired`. Verify rejected transitions: `resolved->pending`, `resolved->acknowledged`, `acknowledged->pending`, `expired->pending`, `expired->resolved`.
2. `TestValidResolutionsForCategory` — verify each category returns the correct set of allowed resolutions per REQ-INB-022: `approval` -> `approved|rejected|request_revision`, `question|decision` -> `answered|dismissed`, `alert` -> `dismissed`.
3. `TestCategoryWakesAgent` — verify `approval`, `question`, `decision` return true; `alert` returns false.
4. `TestValidateCreateInboxItemRequest` — verify title required, type required, invalid category rejected, invalid urgency rejected.

#### GREEN — Implement

Create `internal/domain/inbox.go`:

- `InboxCategory` (`approval`, `question`, `decision`, `alert`), `InboxUrgency` (`critical`, `normal`, `low`), `InboxStatus` (`pending`, `acknowledged`, `resolved`, `expired`), `InboxResolution` (`approved`, `rejected`, `request_revision`, `answered`, `dismissed`) type constants
- `ValidateInboxStatusTransition(from, to InboxStatus) error`
- `ValidResolutionsForCategory(category InboxCategory) []InboxResolution`
- `CategoryWakesAgent(category InboxCategory) bool`
- `CreateInboxItemRequest` struct with validation
- `ValidateCreateInboxItemRequest(req CreateInboxItemRequest) error`

#### Files

- Create: `internal/domain/inbox.go`
- Create: `internal/domain/inbox_test.go`

---

### [x] Task 02 — Database Migration: Inbox Items Table

**Requirements:** REQ-INB-020, REQ-INB-024
**Estimated time:** 30 min

#### Context

Create the `inbox_items` table with all columns, indexes, FK constraints, and enum types. Follow the pattern from migration `20260315000013_create_runtime_tables.sql`. The unique partial indexes handle deduplication for auto-created items.

#### RED — Write Failing Tests

Add assertions to `internal/database/database_test.go` (or a migration smoke test):

1. After `RunMigrations()`, the table `inbox_items` exists with expected columns.
2. The enum types `inbox_category` (`approval`, `question`, `decision`, `alert`), `inbox_urgency` (`critical`, `normal`, `low`), `inbox_status` (`pending`, `acknowledged`, `resolved`, `expired`), `inbox_resolution` (`approved`, `rejected`, `request_revision`, `answered`, `dismissed`) exist.
3. The unique partial index `uq_inbox_active_alert_per_agent_type` exists. Verify `inbox_resolved` exists in `wakeup_invocation_source` enum (add `ALTER TYPE ... ADD VALUE IF NOT EXISTS` if needed).

#### GREEN — Implement

Create `internal/database/migrations/20260315000014_create_inbox_items.sql` with the schema from the design document.

#### Files

- Create: `internal/database/migrations/20260315000014_create_inbox_items.sql`
- Modify: `internal/database/database_test.go` (add migration assertions)

---

### [x] Task 03 — SQL Queries and sqlc Generation

**Requirements:** REQ-INB-001, REQ-INB-006, REQ-INB-007, REQ-INB-008, REQ-INB-009, REQ-INB-010, REQ-INB-013
**Estimated time:** 45 min

#### Context

Write sqlc query definitions for all inbox CRUD operations: create, list with filters, get by ID, count unresolved, resolve, acknowledge, and the ON CONFLICT variant for auto-creation. Run `make sqlc` to generate Go code.

#### RED — Write Failing Tests

Write `internal/database/db/inbox_items_test.go`:

1. `TestCreateInboxItem` — insert an item, verify all fields returned.
2. `TestListInboxItemsBySquad` — insert multiple items with different categories/urgencies, verify filtering and sort order (critical first, then by created_at desc).
3. `TestCountUnresolvedInboxBySquad` — insert pending and resolved items, verify counts.
4. `TestResolveInboxItem` — resolve a pending item, verify status/resolution/timestamps set. Attempt to resolve again, verify no rows returned.
5. `TestAcknowledgeInboxItem` — acknowledge a pending item, verify status change. Attempt on non-pending, verify no rows.
6. `TestCreateInboxItemOnConflictDoNothing` — insert duplicate alert for same agent+type, verify deduplication (returns nil on conflict). Insert non-duplicate alert with different type, verify it succeeds.

#### GREEN — Implement

Create `internal/database/queries/inbox_items.sql` with all queries from the design. Run `make sqlc`.

#### Files

- Create: `internal/database/queries/inbox_items.sql`
- Regenerate: `internal/database/db/` (via `make sqlc`)
- Create: `internal/database/db/inbox_items_test.go` (or integrate into existing test file)

---

### [x] Task 04 — InboxService: Core Logic

**Requirements:** REQ-INB-001, REQ-INB-010, REQ-INB-011, REQ-INB-012, REQ-INB-013, REQ-INB-015, REQ-INB-016, REQ-INB-017, REQ-INB-023
**Estimated time:** 60 min

#### Context

The `InboxService` encapsulates all inbox business logic: creating items, resolving them (with wakeup triggers), acknowledging, and emitting SSE events and activity log entries. This is the central orchestrator that both the HTTP handler and auto-creation integrations call.

#### RED — Write Failing Tests

Write `internal/server/handlers/inbox_service_test.go`:

1. `TestInboxServiceCreate` — verify item is persisted, SSE `inbox.item.created` event is published, activity log entry is created.
2. `TestInboxServiceResolve_Approval` — verify item is resolved, SSE `inbox.item.resolved` event is published, `WakeupService.Enqueue` is called with `inbox_resolved` source and resolution context. Test all three resolutions: `approved`, `rejected`, `request_revision`.
3. `TestInboxServiceResolve_Alert` — verify item is resolved with `dismissed`, NO wakeup is created.
4. `TestInboxServiceResolve_AlreadyResolved` — verify error returned.
5. `TestInboxServiceResolve_InvalidResolution` — verify error when resolution doesn't match category.
6. `TestInboxServiceResolve_TerminatedAgent` — verify item is resolved but no wakeup created when agent is terminated.
7. `TestInboxServiceAcknowledge` — verify status transitions to `acknowledged`, SSE event published.
8. `TestInboxServiceCreateBudgetWarning` — verify auto-created alert item with `type=budget_threshold_80` and correct fields. Verify deduplication returns nil (no SSE event). Verify `qtx` parameter is used (not `s.queries`).
9. `TestInboxServiceCreateAgentError` — verify auto-created alert item with `type=run_failed` and correct fields. Verify deduplication returns nil. Verify best-effort behavior (errors logged but not propagated).

#### GREEN — Implement

Create `internal/server/handlers/inbox_service.go`:

- `InboxService` struct with `queries`, `dbConn`, `sseHub`, `wakeupService` fields
- `NewInboxService(...)` constructor
- `Create(ctx, params) (*db.InboxItem, error)` — insert + SSE + activity
- `Resolve(ctx, itemID, userID, resolution, note, payload) (*db.InboxItem, error)` — update + SSE + conditional wakeup + activity
- `Acknowledge(ctx, itemID, userID) (*db.InboxItem, error)` — update + SSE
- `CreateBudgetWarning(ctx, qtx *db.Queries, params) (*db.InboxItem, error)` — auto-create alert with ON CONFLICT DO NOTHING. Accepts transactional `qtx`. Returns nil item if deduplicated.
- `CreateAgentError(ctx, qtx *db.Queries, params) (*db.InboxItem, error)` — auto-create alert with ON CONFLICT DO NOTHING. Accepts `qtx` (typically non-transactional). Returns nil item if deduplicated.

#### Files

- Create: `internal/server/handlers/inbox_service.go`
- Create: `internal/server/handlers/inbox_service_test.go`

---

### [x] Task 05 — InboxHandler: HTTP Endpoints

**Requirements:** REQ-INB-001, REQ-INB-006, REQ-INB-007, REQ-INB-008, REQ-INB-009, REQ-INB-010, REQ-INB-012, REQ-INB-013, REQ-INB-014, REQ-INB-025, REQ-INB-028, REQ-INB-032, REQ-INB-033
**Estimated time:** 60 min

#### Context

The HTTP handler exposes the inbox REST API. It handles auth (both user sessions and agent Run Tokens for creation), input validation, pagination, and delegates to `InboxService` for business logic. Follow the pattern from `activity_handler.go` and `cost_handler.go`.

#### RED — Write Failing Tests

Write `internal/server/handlers/inbox_handler_test.go`:

1. `TestCreateInboxItem_UserAuth` — POST with user session, verify 201 and response shape.
2. `TestCreateInboxItem_AgentAuth` — POST with Run Token, verify `requestedByAgentId` is set from token.
3. `TestCreateInboxItem_ValidationError` — missing title, invalid category, verify 400.
4. `TestCreateInboxItem_SquadMismatch` — agent token squad differs from URL squad, verify 403.
5. `TestListInboxItems` — GET with pagination and filters, verify response shape and sorting.
6. `TestListInboxItems_NotMember` — verify 403 for non-squad-member.
7. `TestGetInboxItem` — GET by ID, verify full response with payload fields.
8. `TestGetInboxItem_NotFound` — verify 404.
9. `TestGetInboxCount` — verify badge count response shape.
10. `TestResolveInboxItem` — PATCH resolve, verify 200 and updated fields.
11. `TestResolveInboxItem_AlreadyResolved` — verify 409.
12. `TestResolveInboxItem_InvalidResolution` — wrong resolution for category, verify 400.
13. `TestAcknowledgeInboxItem` — PATCH acknowledge, verify 200.
14. `TestDismissInboxItem_Alert` — PATCH dismiss on `alert` item, verify 200, `resolution=dismissed`, `status=resolved`.
15. `TestDismissInboxItem_NonAlert` — PATCH dismiss on `approval` item, verify 400 with `code=INVALID_RESOLUTION`.

#### GREEN — Implement

Create `internal/server/handlers/inbox_handler.go`:

- `InboxHandler` struct with `queries`, `inboxService`
- `NewInboxHandler(...)` constructor
- `RegisterRoutes(mux)` — register all 7 routes (including dismiss)
- `CreateInboxItem(w, r)` — parse body, validate, detect auth type, delegate to service
- `ListInboxItems(w, r)` — parse query params, squad membership check, paginated query
- `GetInboxItem(w, r)` — parse ID, squad membership check, return full item
- `GetInboxCount(w, r)` — squad membership check, return counts
- `ResolveInboxItem(w, r)` — parse body, validate resolution, delegate to service
- `AcknowledgeInboxItem(w, r)` — delegate to service
- `DismissInboxItem(w, r)` — validate `category=alert`, delegate to service `Resolve()` with `resolution=dismissed`
- Response type structs and DB-to-response mapper

#### Files

- Create: `internal/server/handlers/inbox_handler.go`
- Create: `internal/server/handlers/inbox_handler_test.go`

---

### [x] Task 06 — Budget Enforcement Integration

**Requirements:** REQ-INB-002, REQ-INB-003, REQ-INB-034
**Estimated time:** 45 min

#### Context

Wire `InboxService` into `BudgetEnforcementService` so that budget threshold crossings (80% warning, 100% auto-pause) automatically create inbox items. The inbox items must be created within the same database transaction as the cost event for consistency.

#### RED — Write Failing Tests

Write/extend `internal/server/handlers/budget_service_test.go`:

1. `TestRecordAndEnforce_BudgetWarning80_CreatesInboxItem` — record cost that pushes agent to 80%, verify an inbox item with `category=alert`, `type=budget_threshold_80`, `urgency=normal` is created.
2. `TestRecordAndEnforce_BudgetExceeded100_CreatesInboxItem` — record cost that pushes agent to 100%, verify an inbox item with `category=alert`, `type=budget_threshold_100`, `urgency=critical` is created.
3. `TestRecordAndEnforce_BudgetWarning_Deduplicated` — record two cost events that both cross 80%, verify only one inbox item is created (ON CONFLICT DO NOTHING).
4. `TestRecordAndEnforce_SquadBudgetExceeded_CreatesInboxItem` — verify squad-level budget exceeded creates inbox item.

#### GREEN — Implement

Modify `internal/server/handlers/budget_service.go`:

- Add `inboxService *InboxService` field to `BudgetEnforcementService`
- Update `NewBudgetEnforcementService` to accept `InboxService`
- In `RecordAndEnforce()`, after threshold detection, call `inboxService.CreateBudgetWarning(ctx, qtx, params)` passing the transactional `qtx`
- The `qtx` parameter ensures the inbox item is created atomically within the budget enforcement transaction

#### Files

- Modify: `internal/server/handlers/budget_service.go`
- Create/Modify: `internal/server/handlers/budget_service_test.go`

---

### [x] Task 07 — Run Service Integration

**Requirements:** REQ-INB-004, REQ-INB-005, REQ-INB-034
**Estimated time:** 30 min

#### Context

Wire `InboxService` into `RunService` so that failed or timed-out heartbeat runs automatically create inbox items. The inbox item creation should happen as part of the run finalization flow.

#### RED — Write Failing Tests

Write/extend `internal/server/handlers/run_handler_test.go`:

1. `TestFinalize_RunFailed_CreatesInboxItem` — finalize a run with `status=failed`, verify an inbox item with `category=alert`, `type=run_failed`, `urgency=normal` is created with `exitCode` and `stderrExcerpt` in payload. Verify best-effort (non-transactional) creation.
2. `TestFinalize_RunTimedOut_CreatesInboxItem` — finalize with `status=timed_out`, verify inbox item with `category=alert`, `type=run_timed_out`.
3. `TestFinalize_RunSucceeded_NoInboxItem` — finalize with `status=succeeded`, verify no inbox item created.
4. `TestFinalize_AgentError_Deduplicated` — two consecutive failures for the same agent, verify only one active inbox item.

#### GREEN — Implement

Modify `internal/server/handlers/run_handler.go`:

- Add `inboxService *InboxService` field to `RunService`
- Update `NewRunService` to accept `InboxService`
- In `finalize()`, after setting `nextStatus` to `error`, call `inboxService.CreateAgentError(ctx, s.queries, params)` — note: passes `s.queries` (non-transactional) since `finalize()` is non-transactional; failure is logged but does not block run finalization

#### Files

- Modify: `internal/server/handlers/run_handler.go`
- Create/Modify: `internal/server/handlers/run_handler_test.go`

---

### [x] Task 08 — Server Wiring: Register Inbox Routes

**Requirements:** All (integration)
**Estimated time:** 30 min

#### Context

Wire the `InboxService` and `InboxHandler` into the server initialization, connecting all dependencies (queries, DB, SSE hub, wakeup service). Register inbox routes on the HTTP mux. Update `BudgetEnforcementService` and `RunService` constructors to include the `InboxService`.

#### RED — Write Failing Tests

Write an integration test that:

1. Starts the full server with embedded DB.
2. Creates a squad and user.
3. `POST /api/squads/{id}/inbox` — verify 201 and item is persisted.
4. `GET /api/squads/{id}/inbox` — verify item appears in list.
5. `PATCH /api/inbox/{id}/resolve` — verify resolution completes.

#### GREEN — Implement

Modify server initialization (likely `cmd/ari/run.go` or `internal/server/server.go`):

- Create `InboxService` with all dependencies
- Create `InboxHandler` and call `RegisterRoutes(mux)`
- Pass `InboxService` to `BudgetEnforcementService` constructor
- Pass `InboxService` to `RunService` constructor

#### Files

- Modify: `cmd/ari/run.go` or `internal/server/server.go` (server initialization)
- Modify: constructor calls for `BudgetEnforcementService` and `RunService`

---

### [ ] Task 09 — React UI: Inbox Page and Badge

**Requirements:** REQ-INB-006, REQ-INB-008, REQ-INB-009, REQ-INB-010, REQ-INB-013
**Estimated time:** 90 min

#### Context

Build the inbox UI components: a badge count in the sidebar navigation, a list page with filters, and a detail view with category-specific resolve forms. Subscribe to SSE events for real-time updates.

#### RED — Write Failing Tests

(Frontend testing — verify component rendering and API integration)

1. `InboxBadge` renders count from API and updates on SSE events.
2. `InboxList` renders items sorted by urgency, supports filter changes.
3. `InboxResolveForm` renders correct form variant for each category.
4. `InboxItemDetail` shows full item data and resolve button.

#### GREEN — Implement

Create React components:

- `web/src/hooks/useInbox.ts` — API client + SSE subscription hook
- `web/src/components/inbox/InboxBadge.tsx` — badge count in nav
- `web/src/components/inbox/InboxList.tsx` — paginated list with filters
- `web/src/components/inbox/InboxItemCard.tsx` — list item card
- `web/src/components/inbox/InboxItemDetail.tsx` — full detail view
- `web/src/components/inbox/InboxResolveForm.tsx` — category-specific resolve form
- `web/src/components/inbox/InboxFilters.tsx` — filter bar
- `web/src/pages/InboxPage.tsx` — route `/inbox`
- `web/src/pages/InboxDetailPage.tsx` — route `/inbox/:id`

Add routes to the React router and add inbox link + badge to sidebar nav.

#### Files

- Create: `web/src/hooks/useInbox.ts`
- Create: `web/src/components/inbox/InboxBadge.tsx`
- Create: `web/src/components/inbox/InboxList.tsx`
- Create: `web/src/components/inbox/InboxItemCard.tsx`
- Create: `web/src/components/inbox/InboxItemDetail.tsx`
- Create: `web/src/components/inbox/InboxResolveForm.tsx`
- Create: `web/src/components/inbox/InboxFilters.tsx`
- Create: `web/src/pages/InboxPage.tsx`
- Create: `web/src/pages/InboxDetailPage.tsx`
- Modify: `web/src/App.tsx` (add routes)
- Modify: sidebar/nav component (add inbox link + badge)

---

### [x] Task 10 — Activity Log Entries and SSE Snapshot

**Requirements:** REQ-INB-018, REQ-INB-023
**Estimated time:** 30 min

#### Context

Ensure all inbox lifecycle events (create, acknowledge, resolve) append to the activity log with proper `actorType`, `action`, `entityType=inbox_item`, and `metadata`. Also update the SSE initial snapshot to include `criticalCount` for unresolved critical items.

#### RED — Write Failing Tests

1. `TestInboxCreate_ActivityLogEntry` — create an inbox item, verify activity log entry with `action=inbox.created`, `entityType=inbox_item`.
2. `TestInboxResolve_ActivityLogEntry` — resolve an item, verify activity log entry with `action=inbox.resolved`, metadata includes resolution.
3. `TestSSEInitialSnapshot_IncludesCriticalCount` — connect SSE, verify initial snapshot includes `criticalCount` field from unresolved critical inbox items.

#### GREEN — Implement

- In `InboxService.Create()` and `InboxService.Resolve()`, add `queries.InsertActivityLog()` calls
- Add `inbox_item` to the valid `entityType` list in `internal/domain/activity.go`
- Update SSE stream handler to query `CountUnresolvedInboxBySquad` and include `criticalCount` in the initial snapshot event

#### Files

- Modify: `internal/server/handlers/inbox_service.go` (add activity log calls)
- Modify: `internal/domain/activity.go` (add `inbox_item` entity type)
- Modify: `internal/server/handlers/runtime_handler.go` or SSE stream handler (add critical count to snapshot)
