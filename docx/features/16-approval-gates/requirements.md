# Requirements: Approval Gates

**Created:** 2026-03-15
**Status:** Draft
**Depends on:** Feature 12 (Inbox System)

## Overview

Approval gates enforce human oversight before agents execute critical actions. Each squad configures a set of gate rules (e.g., "deploy", "delete", "spend over $500") in its `settings` JSONB column. When an agent encounters a gated action, it creates an inbox item with `category=approval` containing the action details. The agent is then suspended until a human approves, rejects, or the request times out.

This feature builds entirely on the existing inbox system (Feature 12). Approval gates do NOT introduce a separate database table. Instead, they add structured configuration to squad settings and structured payload conventions to `category=approval` inbox items. A background timeout checker auto-resolves expired approval requests according to the squad's configured default (reject or approve).

### Requirement ID Format

- Use sequential IDs: `REQ-APG-001`, `REQ-APG-002`, etc.
- Numbering is continuous across all categories.

---

## Functional Requirements

### Event-Driven Requirements (WHEN...THEN)

**Gate Configuration**

- [REQ-APG-001] WHEN an admin calls `PATCH /api/squads/{id}` with an `approvalGates` array in the `settings` object THEN the system SHALL validate each gate entry and persist the updated settings, returning the updated squad.

- [REQ-APG-002] WHEN a gate entry is submitted with missing or invalid fields THEN the system SHALL reject the entire update with HTTP 400 and `code=VALIDATION_ERROR`, specifying which gate entry failed validation.

- [REQ-APG-003] WHEN a gate entry is submitted without an `id` field THEN the system SHALL generate a new UUID for that gate entry before persisting.

- [REQ-APG-004] WHEN a gate entry is submitted with an `id` that matches an existing gate in the squad's settings THEN the system SHALL update that gate entry in place (upsert semantics).

**Agent Gate Discovery**

- [REQ-APG-005] WHEN an agent calls `GET /api/agent/me/gates` THEN the system SHALL return the `approvalGates` array from the agent's squad settings, allowing the agent to discover which actions require approval before attempting them.

- [REQ-APG-006] WHEN the agent's squad has no `approvalGates` configured (empty array or key absent) THEN the `GET /api/agent/me/gates` endpoint SHALL return an empty array `[]`.

**Approval Request Creation**

- [REQ-APG-007] WHEN an agent calls `POST /api/squads/{id}/inbox` with `category=approval` and a `payload` containing `gateId` and `actionDetails` THEN the system SHALL validate that the referenced `gateId` exists in the squad's `approvalGates` settings, create the inbox item with `status=pending`, compute `expires_at` from database time `now() + gate.timeoutHours` (using the dedicated `expires_at` TIMESTAMPTZ column on `inbox_items`), snapshot gate metadata into the `payload`, and return HTTP 201.

- [REQ-APG-008] WHEN an agent creates an approval request but the referenced `gateId` does not exist in the squad's settings THEN the system SHALL still create the inbox item (the gate may have been removed after the agent read it), but SHALL set a default timeout of 24 hours and log a warning.

- [REQ-APG-009] WHEN an agent creates an approval request THEN the system SHALL emit an `inbox.item.created` SSE event (per existing Feature 12 behavior) and record an activity log entry with `action=inbox.created` and `entityType=inbox_item`.

**Approval Resolution**

- [REQ-APG-010] WHEN a user resolves an approval inbox item with `resolution=approved` THEN the system SHALL wake the requesting agent with `ARI_WAKE_REASON=inbox_resolved` and include `resolution=approved`, `responseNote`, and `responsePayload` in the wakeup context (per existing Feature 12 behavior).

- [REQ-APG-011] WHEN a user resolves an approval inbox item with `resolution=rejected` THEN the system SHALL wake the requesting agent with `ARI_WAKE_REASON=inbox_resolved` and include `resolution=rejected` so the agent can abort the gated action.

- [REQ-APG-012] WHEN a user resolves an approval inbox item with `resolution=request_revision` THEN the system SHALL wake the requesting agent with `ARI_WAKE_REASON=inbox_resolved` and include `resolution=request_revision` so the agent can modify and resubmit its plan.

**Timeout Handling**

- [REQ-APG-013] WHEN an approval inbox item reaches its `expiresAt` timestamp and is still in `pending` or `acknowledged` status THEN the system SHALL auto-resolve the item using the gate's configured `autoResolution` value (either `rejected` or `approved`).

- [REQ-APG-014] WHEN an approval item is auto-resolved due to timeout THEN the system SHALL explicitly set `resolved_by_user_id` to NULL (system-resolved), set `responseNote` to a descriptive message (e.g., "Auto-resolved: approval timed out after 24 hours"), wake the requesting agent with the configured resolution, emit an `inbox.item.resolved` SSE event, and record an activity log entry with `action=inbox.auto_resolved`. The entire auto-resolve operation (DB update, wakeup, activity log) SHALL be wrapped in a database transaction.

- [REQ-APG-015] WHEN the gate's `autoResolution` is not set or the gate no longer exists at timeout THEN the system SHALL default to `rejected` as the auto-resolution.

**Background Timeout Checker**

- [REQ-APG-016] WHEN the server starts THEN the system SHALL launch a background goroutine that periodically checks for expired approval inbox items and auto-resolves them.

- [REQ-APG-017] WHEN the background checker runs THEN it SHALL query for all `category=approval` inbox items in `pending` or `acknowledged` status where the `expires_at` column is in the past, and process each one according to REQ-APG-013 and REQ-APG-014. The checker SHALL use `pg_try_advisory_lock` to ensure only one instance processes expired items at a time in multi-instance deployments.

---

### State-Driven Requirements (WHILE...the system SHALL)

- [REQ-APG-018] WHILE an agent has a pending approval request (an unresolved `category=approval` inbox item where `requestedByAgentId` matches the agent), the agent's wakeup requests from other sources SHALL still be queued normally but the agent SHOULD be aware of its pending approval via its system prompt context.

- [REQ-APG-019] WHILE the background timeout checker is running, it SHALL execute at a configurable interval (default: 60 seconds) and SHALL NOT block server shutdown for more than 5 seconds.

---

### Ubiquitous Requirements (The system SHALL always)

- [REQ-APG-020] The system SHALL scope all approval gate configurations to a single squad via the `settings` JSONB column on the `squads` table; cross-squad gate references SHALL never be created or honored.

- [REQ-APG-021] The system SHALL enforce the existing inbox status machine for approval items: `pending` -> `acknowledged` -> `resolved` | `expired`. Approval gates do not introduce new statuses.

- [REQ-APG-022] The system SHALL validate that approval inbox items can only be resolved with `approved`, `rejected`, or `request_revision` (per existing REQ-INB-022).

- [REQ-APG-023] The system SHALL record an `ActivityLog` entry for every approval gate event: creation (`inbox.created`), manual resolution (`inbox.resolved`), and auto-resolution on timeout (`inbox.auto_resolved`).

- [REQ-APG-024] The system SHALL store approval gate configuration as part of the squad `settings` JSONB; no separate database table is needed for gate definitions.

---

### Conditional Requirements (IF...THEN)

- [REQ-APG-025] IF a squad deletes or modifies a gate rule while there are pending approval items referencing that gate THEN the system SHALL still allow those items to be resolved normally; the gate configuration is captured in the inbox item's `payload` at creation time.

- [REQ-APG-026] IF the requesting agent has been `terminated` when an approval item is auto-resolved on timeout THEN the system SHALL still resolve the item but SHALL NOT create a wakeup request (per existing REQ-INB-027).

- [REQ-APG-027] IF `requiredApprovers` on a gate is greater than 1 THEN in v1 the system SHALL treat any single approval as sufficient (multi-approver enforcement is deferred to a future version); the field is stored but not enforced beyond 1.

- [REQ-APG-028] IF an agent creates multiple pending approval requests for the same gate THEN the system SHALL allow it; each request is independent and must be resolved separately.

---

## Non-Functional Requirements

### Performance

- [REQ-APG-029] The background timeout checker SHALL process up to 1,000 expired items per cycle within 10 seconds, using batch queries rather than individual row updates. If more items remain after a batch, the checker SHALL re-query until no more expired items are found.

- [REQ-APG-030] The `GET /api/agent/me/gates` endpoint SHALL respond within 50ms as it reads from the already-loaded squad settings.

### Security

- [REQ-APG-031] The `GET /api/agent/me/gates` endpoint SHALL only be accessible via agent Run Token authentication; user sessions SHALL receive HTTP 401.

- [REQ-APG-032] Squad approval gate configuration (`PATCH /api/squads/{id}`) SHALL only be modifiable by authenticated users who are members of the squad.

### Reliability

- [REQ-APG-033] The background timeout checker SHALL be resilient to transient database errors; a failed cycle SHALL log the error and retry on the next interval without crashing the server.

- [REQ-APG-034] Auto-resolution on timeout SHALL be idempotent; if the checker processes an item that was resolved between the query and the update, the CAS condition (`status IN ('pending', 'acknowledged')`) SHALL prevent double-resolution.

---

## Constraints

- Approval gates are stored in the squad `settings` JSONB column; no new database tables are introduced for gate definitions.
- Approval requests are `category=approval` inbox items with structured `payload`; no new inbox categories are introduced.
- The existing inbox resolution flow (Feature 12) handles agent wakeup; approval gates add timeout-based auto-resolution on top.
- Multi-approver enforcement (`requiredApprovers > 1`) is deferred to a future version; the field is stored but only single-approver is enforced in v1.
- The `expires_at` timestamp is stored as a dedicated nullable `TIMESTAMPTZ` column on the `inbox_items` table (added via migration). This enables proper indexing and avoids JSONB extraction for timeout queries. A partial index covers `category='approval'` items with pending/acknowledged status.

---

## Acceptance Criteria

- [ ] Squad settings accept and persist `approvalGates` array via `PATCH /api/squads/{id}`.
- [ ] Gate entries are validated: `name`, `actionPattern`, `timeoutHours` required; `autoResolution` defaults to `rejected`.
- [ ] `GET /api/agent/me/gates` returns the squad's approval gate configuration for authenticated agents.
- [ ] Agent can create `category=approval` inbox item with `gateId` and `actionDetails` in payload.
- [ ] Approval inbox item payload includes computed `expiresAt` timestamp.
- [ ] Resolving approval item (approved/rejected/request_revision) wakes the agent with correct context.
- [ ] Background checker auto-resolves expired approval items with configured `autoResolution`.
- [ ] Auto-resolved items have `resolvedByUserId=NULL` and descriptive `responseNote`.
- [ ] Auto-resolution wakes the requesting agent (if not terminated).
- [ ] SSE events fire for both manual and auto resolution.
- [ ] Activity log entries are recorded for creation, manual resolution, and auto-resolution.
- [ ] React UI shows approval items in inbox with approve/reject/request-revision buttons.
- [ ] React UI shows timeout countdown on pending approval items.
- [ ] Background checker handles transient DB errors gracefully (logs and retries).
- [ ] Removing a gate rule does not break pending approval items referencing it.

---

## Out of Scope

- Multi-approver enforcement (stored but not enforced in v1; any single approval suffices).
- Approval chains or sequential multi-step approvals.
- Automatic gate triggering by the runtime (agents must self-report gated actions via the API).
- Email or push notifications for pending approvals (SSE only for v1).
- Approval delegation (any squad member can approve; no per-gate user assignment).
- Gate rule versioning or audit trail for configuration changes.

---

## Dependencies

- **Inbox system (Feature 12):** `InboxService`, `InboxHandler`, `inbox_items` table, SSE events, wakeup integration.
- **Agent self-service (Feature 15):** `AgentSelfHandler`, Run Token auth, `GET /api/agent/me` pattern.
- **Squad settings:** `squads.settings` JSONB column, existing `PATCH /api/squads/{id}` endpoint.
- **Wakeup service:** `WakeupService.Enqueue()` with `invocationSource=inbox_resolved`.
- **SSE hub:** `internal/server/sse/hub.go` for real-time event delivery.
- **Activity log:** `internal/database/queries/activity_log.sql` for audit entries.

---

## Risks & Assumptions

**Assumptions:**
- Agents are responsible for checking their gate configuration and creating approval requests before taking gated actions. The runtime does not automatically intercept actions.
- The `expires_at` column on `inbox_items` is nullable and only populated for approval items; a partial index makes timeout queries efficient.
- Squad settings are read frequently but updated rarely; no caching layer is needed for v1.
- The background checker interval of 60 seconds provides acceptable latency for timeout enforcement (approval timeouts are measured in hours, not seconds).

**Risks:**
- The `expires_at` column adds a small migration, but enables a proper partial index and avoids JSONB extraction overhead in timeout queries.
- If the background checker goroutine panics, expired approvals will not be auto-resolved until restart. Mitigation: wrap the checker in a recover() handler and log the panic; the checker should restart automatically on the next tick.
- Clock skew between application servers (if horizontally scaled in the future) could cause premature or delayed timeout resolution. Mitigation: use database `now()` for all timestamp comparisons, not application time.

---

## References

- PRD: `docx/core/01-PRODUCT.md`
- Inbox system: `docx/features/12-inbox-system/requirements.md`, `docx/features/12-inbox-system/design.md`
- Squad schema: `internal/database/migrations/20260314000003_create_squads.sql`
- Inbox schema: `internal/database/migrations/20260315000014_create_inbox_items.sql`
- Agent self-service: `internal/server/handlers/agent_self_handler.go`
- Wakeup service: `internal/server/handlers/wakeup_handler.go`
