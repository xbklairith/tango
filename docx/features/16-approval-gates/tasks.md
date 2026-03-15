# Tasks: Approval Gates

**Created:** 2026-03-15
**Status:** Complete

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-APG-001 through REQ-APG-034

## Implementation Approach

Work bottom-up through the dependency graph: domain model and validation first, then squad settings schema validation, then SQL queries for timeout handling, then the agent gate discovery endpoint, then InboxService enrichment for approval payloads, then the background timeout checker goroutine, then system prompt injection, then React UI components, and finally end-to-end integration tests. This feature builds entirely on the existing inbox system (Feature 12) — no new tables, just structured payload conventions, settings validation, and a background checker.

## Progress Summary

- Total Tasks: 9
- Completed: 9/9
- In Progress: None
- Test Coverage: Domain tests pass; integration tests pending

---

## Tasks (TDD: Red-Green-Refactor)

---

### [x] Task 01 — Domain Model: ApprovalGate Type, Validation, and Helpers

**Requirements:** REQ-APG-020, REQ-APG-024, REQ-APG-025, REQ-APG-027
**Estimated time:** 30 min

#### Context

Define the `ApprovalGate` struct, `ApprovalPayload` struct, validation and normalization functions, and the `FindGateByID` helper. These are the foundation types used by all subsequent tasks. Also extend `SquadSettings` with the `ApprovalGates` field.

#### RED — Write Failing Tests

Write `internal/domain/approval_gate_test.go`:

1. `TestValidateApprovalGate_Valid` — valid gate passes validation.
2. `TestValidateApprovalGate_MissingName` — empty name returns error.
3. `TestValidateApprovalGate_NameTooLong` — name > 100 chars returns error.
4. `TestValidateApprovalGate_MissingActionPattern` — empty actionPattern returns error.
5. `TestValidateApprovalGate_ActionPatternTooLong` — actionPattern > 100 chars returns error.
6. `TestValidateApprovalGate_TimeoutHoursOutOfRange` — timeoutHours < 1 or > 168 returns error.
7. `TestValidateApprovalGate_InvalidAutoResolution` — autoResolution not in {"rejected", "approved", ""} returns error.
8. `TestValidateApprovalGate_NegativeRequiredApprovers` — requiredApprovers < 0 returns error.
9. `TestNormalizeApprovalGate_FillsDefaults` — nil ID gets generated, empty autoResolution defaults to "rejected", requiredApprovers < 1 defaults to 1.
10. `TestFindGateByID_Found` — returns pointer to matching gate.
11. `TestFindGateByID_NotFound` — returns nil for missing ID.
12. `TestSquadSettings_UnmarshalWithGates` — JSON with approvalGates array unmarshals correctly.
13. `TestSquadSettings_UnmarshalWithoutGates` — JSON without approvalGates key yields empty slice.

#### GREEN — Implement

Create `internal/domain/approval_gate.go`:

- `ApprovalGate` struct with JSON tags
- Constants: `DefaultAutoResolution`, `DefaultTimeoutHours`, `MaxGatesPerSquad`
- `ValidateApprovalGate(g *ApprovalGate) error`
- `NormalizeApprovalGate(g *ApprovalGate)`
- `ApprovalPayload` struct (gateId, gateName, actionPattern, expiresAt, autoResolution, actionDetails)
- `FindGateByID(gates []ApprovalGate, id uuid.UUID) *ApprovalGate`

Modify `internal/domain/squad.go`:

- Add `ApprovalGates []ApprovalGate `json:"approvalGates,omitempty"`` field to `SquadSettings` struct
- **[C-2]** Add `"approvalGates": true` to the `knownSettingsKeys` map
- **[C-2]** In `ValidateSettingsKeys()`, add type validation: if `approvalGates` key is present, verify it is an array type (`[]any`), not a scalar
- **[C-3]** Extend `SquadSettings.Merge()` to handle `ApprovalGates`: if the incoming patch has a non-nil `ApprovalGates` slice, replace the entire field (not field-by-field merge). A nil slice means "not provided", an empty slice means "clear all gates"
- **[M-3]** Use `[]ApprovalGate` with `omitempty` (not a pointer type) — this is the standard Go pattern for JSON slices

#### REFACTOR

- Ensure constants are exported and documented
- Verify JSON round-trip for all structs
- Verify that `Merge()` correctly distinguishes nil (not provided) from empty slice (clear all)

#### Files

- Create: `internal/domain/approval_gate.go`
- Create: `internal/domain/approval_gate_test.go`
- Modify: `internal/domain/squad.go` (add ApprovalGates to SquadSettings)

---

### [x] Task 02 — Squad Settings Schema Validation: approvalGates Array

**Requirements:** REQ-APG-001, REQ-APG-002, REQ-APG-003, REQ-APG-004, REQ-APG-024, REQ-APG-032
**Estimated time:** 45 min

#### Context

Extend the existing squad update handler (`PATCH /api/squads/{id}`) to validate the `approvalGates` array in the `settings` JSONB. Each gate entry is validated and normalized (ID generation, default filling). Duplicate actionPatterns and exceeding the 50-gate limit are rejected.

#### RED — Write Failing Tests

Write/extend `internal/server/handlers/squad_handler_test.go`:

1. `TestUpdateSquadSettings_ValidGates` — PATCH with valid approvalGates array, verify 200 and gates persisted with generated IDs.
2. `TestUpdateSquadSettings_GateMissingName` — gate entry without name, verify 400 with `VALIDATION_ERROR`.
3. `TestUpdateSquadSettings_GateMissingActionPattern` — verify 400.
4. `TestUpdateSquadSettings_GateTimeoutOutOfRange` — timeoutHours=0 and timeoutHours=200, verify 400.
5. `TestUpdateSquadSettings_GateInvalidAutoResolution` — autoResolution="ignored", verify 400.
6. `TestUpdateSquadSettings_TooManyGates` — 51 gates, verify 400 with max-gates error.
7. `TestUpdateSquadSettings_DuplicateActionPattern` — two gates with same actionPattern, verify 400.
8. `TestUpdateSquadSettings_GateIdPreserved` — submit gate with existing ID, verify ID preserved (upsert semantics).
9. `TestUpdateSquadSettings_GateDefaults` — submit gate without autoResolution and requiredApprovers, verify defaults applied.

#### GREEN — Implement

Modify `internal/server/handlers/squad_handler.go`:

- Add `validateSquadSettings(settings domain.SquadSettings) error` function
- In the squad update handler, unmarshal settings into `domain.SquadSettings`, call `validateSquadSettings`, normalize each gate, then persist
- Enforce max 50 gates, unique actionPatterns, and per-gate validation
- **[M-2]** Note: `actionPattern` values are opaque labels in v1, not glob-matched. Agents must use exact action names. No wildcard matching logic needed.

Modify `internal/domain/squad.go` (if not already done in Task 01):

- **[C-2]** Ensure `knownSettingsKeys` includes `"approvalGates": true`
- **[C-2]** Ensure `ValidateSettingsKeys()` validates that `approvalGates` is an array type
- **[C-3]** Ensure `SquadSettings.Merge()` handles `ApprovalGates` field (replace entirely if non-nil)

#### REFACTOR

- Extract validation into reusable helper if needed by other handlers
- Ensure error messages specify which gate index failed

#### Files

- Modify: `internal/server/handlers/squad_handler.go`
- Modify: `internal/server/handlers/squad_handler_test.go`
- Modify: `internal/domain/squad.go` (knownSettingsKeys, ValidateSettingsKeys, Merge)

---

### [x] Task 03 — Migration + SQL Queries: expires_at Column, ListExpiredApprovalItems, AutoResolveInboxItem

**Requirements:** REQ-APG-013, REQ-APG-014, REQ-APG-017, REQ-APG-029, REQ-APG-034
**Estimated time:** 45 min

#### Context

Add a migration to add the `expires_at TIMESTAMPTZ` column to `inbox_items` with a partial index. Then add two new sqlc queries for the background timeout checker: one to list expired approval items (filtered by category, status, and `expires_at` column), and one to auto-resolve an item with CAS for idempotency. Also update `CreateInboxItem` to accept an optional timeout_hours parameter that computes `expires_at` using database time. Run `make sqlc` to regenerate.

#### RED — Write Failing Tests

Write/extend `internal/database/db/inbox_items_test.go`:

1. `TestListExpiredApprovalItems_ReturnsExpired` — insert approval item with past `expires_at`, verify it appears in results.
2. `TestListExpiredApprovalItems_IgnoresNonApproval` — insert non-approval item with past `expires_at`, verify it is excluded.
3. `TestListExpiredApprovalItems_IgnoresResolved` — insert resolved approval item with past `expires_at`, verify excluded.
4. `TestListExpiredApprovalItems_IgnoresFutureExpiry` — insert approval item with future `expires_at`, verify excluded.
5. `TestListExpiredApprovalItems_IgnoresNullExpiry` — insert approval item with NULL `expires_at`, verify excluded.
6. `TestListExpiredApprovalItems_RespectsLimit` — insert 5 expired items, query with limit=3, verify only 3 returned.
7. `TestAutoResolveInboxItem_Success` — auto-resolve a pending item, verify status=resolved, resolution set, `resolved_by_user_id` is NULL.
8. `TestAutoResolveInboxItem_AlreadyResolved` — attempt to auto-resolve already-resolved item, verify no rows returned (CAS failure).
9. `TestAutoResolveInboxItem_AcknowledgedStatus` — auto-resolve an acknowledged item, verify success.
10. `TestAutoResolveInboxItem_ExplicitNullResolvedByUserId` — verify the resolved item has `resolved_by_user_id = NULL` (H-1).

#### GREEN — Implement

1. Create migration `internal/database/migrations/YYYYMMDDHHMMSS_add_inbox_expires_at.sql`:
   - Add `expires_at TIMESTAMPTZ` nullable column to `inbox_items`
   - **[M-1]** Add partial index: `CREATE INDEX idx_inbox_items_approval_expires ON inbox_items(expires_at) WHERE category = 'approval' AND status IN ('pending', 'acknowledged') AND expires_at IS NOT NULL;`

2. Add to `internal/database/queries/inbox_items.sql`:
   - `ListExpiredApprovalItems` — SELECT with `category='approval'`, `status IN ('pending','acknowledged')`, `expires_at IS NOT NULL AND expires_at <= now()`, ORDER BY created_at ASC, LIMIT $1
   - **[H-1]** `AutoResolveInboxItem` — UPDATE with CAS on status, sets resolution, response_note, **explicitly sets `resolved_by_user_id = NULL`**, resolved_at=now(), RETURNING *

3. Update `CreateInboxItem` query (or add `CreateInboxItemWithExpiry` variant) to accept an optional timeout_hours parameter and compute `expires_at = now() + @timeout_hours * interval '1 hour'` using database time (**C-4 fix**).

Run `make sqlc` to regenerate Go code.

#### REFACTOR

- Verify query execution plans use the partial index (EXPLAIN ANALYZE)
- Confirm the migration runs cleanly on existing data (all existing rows get NULL `expires_at`)

#### Files

- Create: `internal/database/migrations/YYYYMMDDHHMMSS_add_inbox_expires_at.sql`
- Modify: `internal/database/queries/inbox_items.sql`
- Regenerate: `internal/database/db/` (via `make sqlc`)
- Modify: `internal/database/db/inbox_items_test.go`

---

### [x] Task 04 — Agent Endpoint: GET /api/agent/me/gates

**Requirements:** REQ-APG-005, REQ-APG-006, REQ-APG-030, REQ-APG-031
**Estimated time:** 30 min

#### Context

Add a new endpoint to `AgentSelfHandler` that returns the approval gate configuration for the agent's squad. This endpoint reads from the squad's `settings` JSONB and is only accessible via agent Run Token authentication.

#### RED — Write Failing Tests

Write/extend `internal/server/handlers/agent_self_handler_test.go`:

1. `TestGetGates_WithGatesConfigured` — squad has 2 gates in settings, verify response contains `gates` array with 2 entries and correct fields.
2. `TestGetGates_NoGatesConfigured` — squad has no approvalGates key in settings, verify response returns `{"gates": []}`.
3. `TestGetGates_EmptyGatesArray` — squad has `approvalGates: []`, verify response returns `{"gates": []}`.
4. `TestGetGates_WithoutRunToken` — call without agent auth, verify 401 with `UNAUTHENTICATED`.
5. `TestGetGates_UserSessionDenied` — call with user session (not Run Token), verify 401.
6. `TestGetGates_ResponseTime` — verify response within 50ms (performance sanity check).

#### GREEN — Implement

Modify `internal/server/handlers/agent_self_handler.go`:

- Add `GetGates(w, r)` handler method
- Register route: `mux.HandleFunc("GET /api/agent/me/gates", h.GetGates)`
- **[C-1]** Read agent identity from context using `identity, ok := auth.AgentFromContext(r.Context())` with `!ok` check (returns value + bool, not pointer). Match the pattern in existing handlers like `GetMe`.

#### REFACTOR

- Ensure consistent error response format with other agent self-service endpoints

#### Files

- Modify: `internal/server/handlers/agent_self_handler.go`
- Modify: `internal/server/handlers/agent_self_handler_test.go`

---

### [x] Task 05 — InboxService Enrichment: Approval Payload with Gate Snapshot

**Requirements:** REQ-APG-007, REQ-APG-008, REQ-APG-009, REQ-APG-021, REQ-APG-022, REQ-APG-023, REQ-APG-025, REQ-APG-028
**Estimated time:** 45 min

#### Context

Extend `InboxService.Create()` to detect `category=approval` items and enrich the payload with gate metadata (gateName, actionPattern, expiresAt, autoResolution). The gate configuration is snapshotted at creation time so future gate edits don't affect pending items. If the referenced gateId is not found, defaults are applied and a warning is logged.

#### RED — Write Failing Tests

Write/extend `internal/server/handlers/inbox_service_test.go`:

1. `TestCreateApprovalItem_ValidGateId` — create with category=approval and valid gateId, verify payload enriched with gateName, actionPattern, autoResolution, timeoutHours from gate config. Verify `expires_at` column is set (non-NULL) using DB time.
2. `TestCreateApprovalItem_UnknownGateId` — create with gateId not in squad settings, verify defaults applied (24h timeout, "rejected" autoResolution), warning logged, `expires_at` column set.
3. `TestCreateApprovalItem_NoGateId` — create with category=approval but no gateId in payload, verify defaults applied.
4. `TestCreateApprovalItem_ExpiresAtColumn` — gate with timeoutHours=48, verify `expires_at` column is approximately now+48h (computed by DB, not app).
5. `TestCreateApprovalItem_GateSnapshotImmutable` — create item, then modify gate in squad settings, verify the item's payload still has original gate values.
6. `TestCreateApprovalItem_SSEAndActivityLog` — verify SSE event `inbox.item.created` and activity log entry with `action=inbox.created` are emitted.
7. `TestCreateApprovalItem_MultipleForSameGate` — create two approval items for same gateId, verify both created independently.

#### GREEN — Implement

Modify `internal/server/handlers/inbox_service.go`:

- Add `enrichApprovalPayload(ctx, squadID, rawPayload json.RawMessage) (*enrichApprovalResult, error)` private method
- **[H-3]** Handle `json.RawMessage` to `map[string]any` impedance mismatch: unmarshal the raw payload to a map, add gate fields to the map, then marshal back to `json.RawMessage` for the DB insert
- **[C-4]** Do NOT compute `expiresAt` with `time.Now()` — instead, return `timeoutHours` in the result and set the `expires_at` column via the SQL INSERT using database time (`now() + interval`)
- In `Create()`, add hook at top: if `category == "approval"`, call `enrichApprovalPayload`, marshal result payload back to JSON, and set the expires_at parameter for the SQL query
- Enrichment logic: extract gateId, load squad settings, find gate, snapshot gateName/actionPattern/autoResolution/timeoutHours into payload map
- Fallback to defaults if gate not found (with slog.Warn)
- **[XH-4]** The enrichment triggers in `InboxService.Create()` regardless of whether the call comes from `InboxHandler` or `AgentSelfHandler`, ensuring both paths produce identical results

#### REFACTOR

- Ensure enrichment does not modify the original payload map (create a copy if needed)
- The `enrichApprovalResult` struct cleanly separates the enriched payload from the timeout metadata

#### Files

- Modify: `internal/server/handlers/inbox_service.go`
- Modify: `internal/server/handlers/inbox_service_test.go`

---

### [x] Task 06 — ApprovalTimeoutChecker: Background Goroutine

**Requirements:** REQ-APG-013, REQ-APG-014, REQ-APG-015, REQ-APG-016, REQ-APG-017, REQ-APG-019, REQ-APG-026, REQ-APG-029, REQ-APG-033, REQ-APG-034
**Estimated time:** 60 min

#### Context

Build the `ApprovalTimeoutChecker` background goroutine that periodically queries for expired approval inbox items and auto-resolves them. It uses the `AutoResolveInboxItem` CAS query for idempotency, wakes the requesting agent (if not terminated), emits SSE events, and logs activity. The checker is resilient to DB errors and panics.

#### RED — Write Failing Tests

Write `internal/server/handlers/approval_timeout_test.go`:

1. `TestProcessExpired_AutoRejectsExpiredItem` — expired item with autoResolution="rejected", verify resolved with "rejected", responseNote contains timeout info.
2. `TestProcessExpired_AutoApprovesExpiredItem` — expired item with autoResolution="approved", verify resolved with "approved".
3. `TestProcessExpired_DefaultsToRejected` — expired item with no autoResolution in payload, verify defaults to "rejected".
4. `TestProcessExpired_AlreadyResolved_NoOp` — item resolved between query and update, verify CAS prevents double-resolution.
5. `TestProcessExpired_WakesRequestingAgent` — verify WakeupService.Enqueue called with inbox_resolved source, auto_resolved=true in context.
6. `TestProcessExpired_TerminatedAgent_NoWakeup` — requesting agent is terminated, verify item resolved but no wakeup created.
7. `TestProcessExpired_NullRequestingAgent_NoWakeup` — item has no requestedByAgentId, verify resolved but no wakeup.
8. `TestProcessExpired_EmitsSSEEvent` — verify SSE `inbox.item.resolved` event with autoResolved=true published.
9. `TestProcessExpired_ActivityLog` — verify activity log entry with `action=inbox.auto_resolved`.
10. `TestProcessExpired_BatchProcessing` — 5 expired items, verify all processed in single cycle.
11. `TestProcessExpired_DBError_LogsAndContinues` — query returns error, verify error logged and checker does not panic.
12. `TestProcessExpired_PanicRecovery` — simulate panic in processing, verify recovered and logged.
13. `TestStart_RespectsContextCancellation` — cancel context, verify goroutine exits cleanly within 5 seconds.

#### GREEN — Implement

Create `internal/server/handlers/approval_timeout.go`:

- `ApprovalTimeoutChecker` struct with queries, dbConn, wakeupService, sseHub, interval, batchSize
- `NewApprovalTimeoutChecker(...)` constructor (default interval=60s, **batchSize=1000** per REQ-APG-029)
- `Start(ctx)` — ticker loop, blocks until context cancelled
- `processExpired(ctx)` — wrapped in defer/recover:
  - **[XH-2]** Acquire `pg_try_advisory_lock` at start of each cycle. If lock not acquired, skip (another instance is handling it).
  - **[H-2]** Re-query loop: process batches of 1000 until no more expired items found.
  - Queries expired items using the `expires_at` column (not JSONB extraction).
- `autoResolveItem(ctx, item)` — extract autoResolution from payload, build responseNote:
  - **[H-5]** Wrap resolve + activity log in a transaction (matching InboxService.Resolve() pattern). Wakeup and SSE are best-effort after commit.
  - **[H-6]** Call `logActivity(ctx, qtx, ActivityParams{...})` directly as a package-level function (not `c.inboxService.logActivity(...)`).
  - **[H-1]** The `AutoResolveInboxItem` SQL query explicitly sets `resolved_by_user_id = NULL`.

#### REFACTOR

- Make interval and batchSize configurable via constructor options
- Ensure clean shutdown (ticker stopped, goroutine returns promptly)
- Advisory lock is released automatically on disconnect, but also explicitly released via defer

#### Files

- Create: `internal/server/handlers/approval_timeout.go`
- Create: `internal/server/handlers/approval_timeout_test.go`

---

### [x] Task 07 — System Prompt Injection: Gate List in Agent Prompt

**Requirements:** REQ-APG-018
**Estimated time:** 30 min

#### Context

When the RunService prepares the system prompt for an agent run, include the squad's gate configuration so agents are aware of which actions require approval. This is appended to the existing system prompt via the agent environment or prompt injection mechanism.

#### RED — Write Failing Tests

Write/extend `internal/server/handlers/run_handler_test.go` or appropriate test file:

1. `TestSystemPrompt_IncludesGateList` — squad has 2 gates configured, verify system prompt contains gate names and action patterns.
2. `TestSystemPrompt_NoGates_NoInjection` — squad has no gates, verify system prompt does not contain approval gate section.
3. `TestSystemPrompt_GateInstructionFormat` — verify prompt includes instructions to use `POST /api/squads/{squadId}/inbox` and `GET /api/agent/me/gates`.

#### GREEN — Implement

Modify the RunService or system prompt builder:

- After loading squad settings, check if `ApprovalGates` is non-empty
- If so, append a formatted section listing each gate (name, pattern, timeout) and instructions for the agent to create approval requests
- Include the `GET /api/agent/me/gates` reference for full configuration
- **[M-4]** Gate info must be injected in BOTH the task wakeup path AND the conversation prompt path in `buildInvokeInput`. Ensure both code paths check for approval gates and include the gate list.

#### REFACTOR

- Ensure gate list formatting is clean and readable in the agent's context window
- Extract prompt section builder to a helper function for testability

#### Files

- Modify: `internal/server/handlers/run_handler.go` or system prompt builder
- Modify: corresponding test file

---

### [x] Task 08 — React UI: ApprovalActions and ApprovalTimeoutBadge Components

**Requirements:** REQ-APG-010, REQ-APG-011, REQ-APG-012, REQ-APG-022
**Estimated time:** 60 min

#### Context

Build two React components for the approval UI: `ApprovalActions` (approve/reject/request-revision buttons with optional note) and `ApprovalTimeoutBadge` (countdown timer showing time until auto-resolution). Integrate them into the existing inbox detail view for `category=approval` items. Also display the approval payload fields (gateName, actionPattern, actionDetails) in an information panel.

#### RED — Write Failing Tests

(Frontend tests — component rendering and interaction)

1. `ApprovalActions` renders three buttons: Approve, Reject, Request Revision.
2. `ApprovalActions` calls `onResolve` with correct resolution string and note on button click.
3. `ApprovalActions` disables buttons when `isLoading=true`.
4. `ApprovalActions` includes an optional note textarea.
5. `ApprovalTimeoutBadge` displays remaining time in "Xh Ym remaining" format.
6. `ApprovalTimeoutBadge` shows "Expired" when expiresAt is in the past.
7. `ApprovalTimeoutBadge` shows auto-resolution policy text (e.g., "auto-rejected on timeout").
8. `ApprovalTimeoutBadge` updates countdown every minute.
9. Inbox detail view renders `ApprovalActions` and `ApprovalTimeoutBadge` for `category=approval` items.
10. Inbox detail view displays gateName, actionPattern, and actionDetails from payload.

#### GREEN — Implement

Create React components:

- `web/src/components/inbox/ApprovalActions.tsx` — Approve/Reject/Request Revision buttons with note textarea
- `web/src/components/inbox/ApprovalTimeoutBadge.tsx` — countdown timer badge with auto-resolution policy
- Modify `web/src/components/inbox/InboxItemDetail.tsx` (or equivalent) — render approval-specific components for `category=approval` items, display payload fields

#### REFACTOR

- Ensure consistent styling with existing inbox components
- Extract countdown logic to a reusable hook if applicable

#### Files

- Create: `web/src/components/inbox/ApprovalActions.tsx`
- Create: `web/src/components/inbox/ApprovalTimeoutBadge.tsx`
- Modify: `web/src/components/inbox/InboxItemDetail.tsx` (approval-specific rendering)

---

### [x] Task 09 — Integration Tests: Full Approval Flow and Timeout Flow

**Requirements:** REQ-APG-001 through REQ-APG-034 (all — end-to-end validation)
**Estimated time:** 60 min

#### Context

Write end-to-end integration tests that exercise the complete approval gate lifecycle: configuring gates, agent discovery, creating approval requests, human resolution, and timeout-based auto-resolution. Also wire the `ApprovalTimeoutChecker` into server startup.

#### RED — Write Failing Tests

Write `internal/server/handlers/approval_integration_test.go`:

1. **Full Approval Flow (Happy Path)**
   - Configure 2 gates on a squad via `PATCH /api/squads/{id}`
   - Agent discovers gates via `GET /api/agent/me/gates`, verify 2 gates returned
   - Agent creates approval request via `POST /api/squads/{id}/inbox` with category=approval, gateId, actionDetails
   - Verify inbox item created with enriched payload (expiresAt, gateName, actionPattern, autoResolution)
   - User resolves with `resolution=approved` and a responseNote
   - Verify agent wakeup created with `inbox_resolved` source, `resolution=approved`, and responseNote
   - Verify SSE events fired for creation and resolution
   - Verify activity log entries for `inbox.created` and `inbox.resolved`

2. **Rejection Flow**
   - Agent creates approval request
   - User resolves with `resolution=rejected`
   - Verify agent wakeup with `resolution=rejected`

3. **Request Revision Flow**
   - Agent creates approval request
   - User resolves with `resolution=request_revision`
   - Verify agent wakeup with `resolution=request_revision`

4. **Timeout Auto-Resolution Flow**
   - Create approval item with a short expiresAt (set in the past or use a test helper)
   - Trigger the timeout checker's `processExpired` method directly
   - Verify item auto-resolved with gate's configured autoResolution
   - Verify resolvedByUserId is NULL
   - Verify responseNote contains timeout description
   - Verify agent wakeup created with auto_resolved=true
   - Verify SSE event with autoResolved=true
   - Verify activity log with `action=inbox.auto_resolved`

5. **Timeout with Terminated Agent**
   - Create approval item, set requesting agent to terminated
   - Trigger timeout checker
   - Verify item resolved but no wakeup created

6. **Gate Deleted While Pending**
   - Configure gate, create approval request referencing it
   - Remove gate from squad settings
   - Resolve the approval item normally
   - Verify resolution succeeds (gate config was snapshotted in payload)

7. **Server Startup Wiring**
   - Verify `ApprovalTimeoutChecker` is created and started during server initialization
   - Verify it shuts down cleanly when server context is cancelled

#### GREEN — Implement

- Create the integration test file with all scenarios
- Wire `ApprovalTimeoutChecker` into `internal/server/server.go` (or `cmd/ari/run.go`):
  - Create checker after InboxService and WakeupService are initialized
  - Launch via `go approvalChecker.Start(ctx)` with server's root context

#### REFACTOR

- Ensure test helpers are shared (squad creation, agent creation, gate configuration)
- Clean up any test database state between scenarios

#### Files

- Create: `internal/server/handlers/approval_integration_test.go`
- Modify: `internal/server/server.go` or `cmd/ari/run.go` (wire ApprovalTimeoutChecker)
