# Requirements: Inbox System (Human-in-the-Loop Queue)

**Created:** 2026-03-15
**Status:** Draft

## Overview

Implement the unified inbox — the primary governance interface where agents request human attention. The inbox consolidates budget warnings, approval requests, questions, error reports, and decisions into a single prioritized queue with real-time SSE notifications.

Agents create inbox items via the Ari API during execution. The system also auto-creates inbox items from budget enforcement (80% warning, 100% auto-pause) and agent runtime errors (run failed). When a user resolves an inbox item, the originating agent is woken with `ARI_WAKE_REASON=inbox_resolved` so it can act on the human's response.

### Requirement ID Format

- Use sequential IDs: `REQ-INB-001`, `REQ-INB-002`, etc.
- Numbering is continuous across all categories.

---

## Functional Requirements

### Event-Driven Requirements (WHEN...THEN)

**Inbox Item Creation**

- [REQ-INB-001] WHEN an agent calls `POST /api/squads/{id}/inbox` with a valid payload THEN the system SHALL create an `InboxItem` record with `status=pending`, validate that `category` is one of `approval_request`, `question`, `decision`, `info`, `budget_warning`, `agent_error`, set `requestedByAgentId` from the caller's Run Token identity, and return HTTP 201 with the created item.

- [REQ-INB-002] WHEN the `BudgetEnforcementService` detects that an agent's monthly spend has reached 80% of its `budgetMonthlyCents` THEN the system SHALL auto-create an `InboxItem` with `category=budget_warning`, `urgency=high`, `type=budget_threshold_80`, link `relatedAgentId` to the agent, and include spend/budget amounts in the `payload` JSON.

- [REQ-INB-003] WHEN the `BudgetEnforcementService` detects that an agent's monthly spend has reached 100% of its `budgetMonthlyCents` and auto-pauses the agent THEN the system SHALL auto-create an `InboxItem` with `category=budget_warning`, `urgency=critical`, `type=budget_threshold_100`, link `relatedAgentId` to the agent, and include spend/budget amounts in the `payload` JSON.

- [REQ-INB-004] WHEN a `HeartbeatRun` finishes with `status=failed` THEN the system SHALL auto-create an `InboxItem` with `category=agent_error`, `urgency=high`, `type=run_failed`, link `relatedAgentId` to the agent and `relatedRunId` to the failed run, and include `exitCode` and `stderrExcerpt` in the `payload` JSON.

- [REQ-INB-005] WHEN a `HeartbeatRun` finishes with `status=timed_out` THEN the system SHALL auto-create an `InboxItem` with `category=agent_error`, `urgency=high`, `type=run_timed_out`, link `relatedAgentId` to the agent and `relatedRunId` to the run, and include the timeout duration in the `payload` JSON.

**Inbox Item Retrieval**

- [REQ-INB-006] WHEN a user calls `GET /api/squads/{id}/inbox` THEN the system SHALL return a paginated list of `InboxItem` records scoped to that squad, sorted by urgency (critical first) then by `createdAt` descending, with support for `limit` and `offset` query parameters.

- [REQ-INB-007] WHEN a user calls `GET /api/squads/{id}/inbox` with filter query parameters THEN the system SHALL support filtering by `category`, `urgency`, and `status` (each accepts a single value; multiple filters are AND-combined).

- [REQ-INB-008] WHEN a user calls `GET /api/inbox/{id}` THEN the system SHALL return the full `InboxItem` record including `payload`, `responseNote`, and `responsePayload`, after verifying the caller is a member of the item's squad.

- [REQ-INB-009] WHEN a user calls `GET /api/squads/{id}/inbox/count` THEN the system SHALL return a JSON object with `pending`, `acknowledged`, and `total` counts of unresolved inbox items for that squad.

**Inbox Item Resolution**

- [REQ-INB-010] WHEN a user calls `PATCH /api/inbox/{id}/resolve` with a valid resolution THEN the system SHALL update the `InboxItem` with `status=resolved`, set `resolution` to one of `approved`, `rejected`, `answered`, `dismissed`, record `resolvedByUserId`, `resolvedAt`, `responseNote`, and `responsePayload`, and return the updated item.

- [REQ-INB-011] WHEN an inbox item of category `approval_request`, `question`, or `decision` is resolved THEN the system SHALL create a `WakeupRequest` with `invocationSource=inbox_resolved` for the `requestedByAgentId`, injecting the resolution payload (`resolution`, `responseNote`, `responsePayload`, `inboxItemId`) as `ARI_WAKE_REASON=inbox_resolved` context so the agent wakes with the user's response.

- [REQ-INB-012] WHEN an inbox item of category `info`, `budget_warning`, or `agent_error` is resolved with `resolution=dismissed` THEN the system SHALL NOT create a `WakeupRequest` — the resolution is informational only.

- [REQ-INB-013] WHEN a user calls `PATCH /api/inbox/{id}/acknowledge` THEN the system SHALL update the `InboxItem` with `status=acknowledged` without resolving it, recording `acknowledgedByUserId` and `acknowledgedAt`.

- [REQ-INB-014] WHEN a user attempts to resolve an inbox item that is already `resolved` THEN the system SHALL return HTTP 409 with `code=ALREADY_RESOLVED`.

**SSE Events**

- [REQ-INB-015] WHEN a new `InboxItem` is created THEN the system SHALL emit an `inbox.item.created` SSE event to all subscribers on the item's squad stream with `itemId`, `category`, `urgency`, `title`, and `type`.

- [REQ-INB-016] WHEN an `InboxItem` is resolved THEN the system SHALL emit an `inbox.item.resolved` SSE event to all subscribers on the item's squad stream with `itemId`, `resolvedByUserId`, `resolution`, and `resolvedAt`.

- [REQ-INB-017] WHEN an `InboxItem` is acknowledged THEN the system SHALL emit an `inbox.item.acknowledged` SSE event to all subscribers on the item's squad stream with `itemId` and `acknowledgedByUserId`.

---

### State-Driven Requirements (WHILE...the system SHALL)

- [REQ-INB-018] WHILE there are unresolved inbox items with `urgency=critical` for a squad, the system SHALL include a `criticalCount` field in the SSE initial snapshot sent to new subscribers on `GET /api/squads/{id}/events/stream`.

- [REQ-INB-019] WHILE an `InboxItem` has `status=pending`, the system SHALL allow transitions to `acknowledged` or `resolved` but NOT back to `pending`.

---

### Ubiquitous Requirements (The system SHALL always)

- [REQ-INB-020] The system SHALL scope all inbox items to a single `squadId`; cross-squad inbox items SHALL never be created or returned.

- [REQ-INB-021] The system SHALL enforce the inbox item status machine for every status transition:
  - `pending` -> `acknowledged` | `resolved`
  - `acknowledged` -> `resolved`
  - `resolved` -> (terminal, no further transitions)

- [REQ-INB-022] The system SHALL validate that `resolution` values match the item's `category`:
  - `approval_request` -> `approved` | `rejected`
  - `question` -> `answered` | `dismissed`
  - `decision` -> `answered` | `dismissed`
  - `info` -> `dismissed`
  - `budget_warning` -> `dismissed`
  - `agent_error` -> `dismissed`

- [REQ-INB-023] The system SHALL record an `ActivityLog` entry for every inbox item creation (`inbox.created`) and resolution (`inbox.resolved`) with `entityType=inbox_item`.

- [REQ-INB-024] The system SHALL store resolution history as immutable records — resolved inbox items SHALL NOT be updated or deleted via the API.

---

### Conditional Requirements (IF...THEN)

- [REQ-INB-025] IF an agent creates an inbox item with `category=approval_request` and `urgency=critical` THEN the system SHALL emit an additional `cost.threshold.warning`-style SSE event to ensure the user is immediately notified even if they are not viewing the inbox.

- [REQ-INB-026] IF a squad has no active SSE subscribers when an inbox item is created THEN the system SHALL still persist the item; the user will see it on their next visit to the inbox.

- [REQ-INB-027] IF the `requestedByAgentId` references an agent that has been `terminated` when the user resolves the item THEN the system SHALL still resolve the item but SHALL NOT create a `WakeupRequest` (the agent cannot be woken).

- [REQ-INB-028] IF an agent creates an inbox item but the agent's Run Token is revoked (agent paused/terminated) before the request completes THEN the system SHALL reject the request with HTTP 401.

- [REQ-INB-029] IF an inbox item references a `relatedIssueId` that no longer exists THEN the system SHALL still display the inbox item with a null `relatedIssue` in the response.

---

## Non-Functional Requirements

### Performance

- [REQ-INB-030] The system SHALL return the inbox list (`GET /api/squads/{id}/inbox`) within 200ms for squads with up to 10,000 inbox items.

- [REQ-INB-031] The system SHALL deliver `inbox.item.created` SSE events to all squad subscribers within 500ms of item creation.

### Security

- [REQ-INB-032] The system SHALL verify squad membership for all inbox read endpoints — only members of the squad may view or resolve inbox items.

- [REQ-INB-033] The system SHALL accept inbox item creation from both authenticated users (via session) and agents (via Run Token JWT), validating the caller's `squadId` matches the target squad.

### Reliability

- [REQ-INB-034] The system SHALL create auto-generated inbox items (budget warnings, agent errors) within the same database transaction as the triggering event to ensure consistency.

- [REQ-INB-035] SSE event delivery for inbox events SHALL be best-effort: a slow subscriber MUST NOT block inbox item persistence or delivery to other subscribers.

---

## Constraints

- All inbox items MUST be scoped to a single squad (FK to `squads`).
- Inbox item status transitions MUST follow the state machine: `pending` -> `acknowledged` -> `resolved`.
- Resolution types MUST match the item's category (see REQ-INB-022).
- Auto-created items (budget, errors) MUST be created within the triggering transaction.
- The inbox API MUST support both user authentication (session JWT) and agent authentication (Run Token JWT).
- Resolved inbox items are immutable — no UPDATE or DELETE after resolution.

---

## Acceptance Criteria

- [ ] `POST /api/squads/{id}/inbox` creates an inbox item and returns 201.
- [ ] `GET /api/squads/{id}/inbox` returns paginated, filtered, sorted inbox items.
- [ ] `GET /api/inbox/{id}` returns full inbox item detail with squad membership check.
- [ ] `GET /api/squads/{id}/inbox/count` returns pending/acknowledged/total counts.
- [ ] `PATCH /api/inbox/{id}/resolve` resolves an item and wakes the agent (for approval/question/decision).
- [ ] `PATCH /api/inbox/{id}/acknowledge` transitions item to acknowledged.
- [ ] Resolving an already-resolved item returns 409.
- [ ] Budget enforcement at 80% auto-creates `budget_warning` inbox item.
- [ ] Budget enforcement at 100% auto-creates `budget_warning` inbox item with `urgency=critical`.
- [ ] Failed `HeartbeatRun` auto-creates `agent_error` inbox item.
- [ ] `inbox.item.created` SSE event fires on creation.
- [ ] `inbox.item.resolved` SSE event fires on resolution.
- [ ] Agent is woken with `inbox_resolved` context when approval/question/decision is resolved.
- [ ] Activity log entries are created for inbox create and resolve actions.
- [ ] Badge count endpoint returns correct unresolved counts.

---

## Out of Scope

- Inbox item expiration / TTL (all items persist indefinitely for v1).
- Email or push notifications for inbox items (SSE only for v1).
- Bulk resolve operations (one at a time for v1).
- Inbox item comments or threaded discussion (use `responseNote` for v1).
- Custom category definitions (fixed enum for v1).
- Inbox item assignment to specific users (any squad member can resolve).

---

## Dependencies

- Budget enforcement service: `internal/server/handlers/budget_service.go` (integration for auto-creating budget warning items).
- Run service: `internal/server/handlers/run_handler.go` (integration for auto-creating agent error items on run failure).
- Wakeup service: `internal/server/handlers/wakeup_handler.go` (creating wakeup on inbox resolution).
- SSE hub: `internal/server/sse/hub.go` (publishing inbox events).
- Activity log: `internal/database/queries/activity_log.sql` (recording inbox actions).
- Agent domain model: `internal/domain/agent.go` (agent status checks for wakeup eligibility).
- Auth: `internal/auth/` (Run Token validation for agent-created items, session validation for user actions).

---

## Risks & Assumptions

**Assumptions:**
- The PRD uses "Approval" as the entity name, but we use "InboxItem" as it better reflects the unified queue concept (approvals are one category among many).
- One inbox item per triggering event is sufficient — no batching or deduplication needed for v1.
- Any squad member can resolve any inbox item — no per-item assignment needed for v1.
- The `payload` and `responsePayload` JSONB fields are flexible enough to cover all category-specific data.

**Risks:**
- High-volume budget warning creation could flood the inbox if an agent makes many small cost events near the 80% threshold. Mitigation: deduplicate budget warnings — only create one per threshold per billing period per agent.
- Agent error inbox items could accumulate rapidly if an agent enters a crash loop. Mitigation: limit to one active `agent_error` item per agent (deduplicate while a pending item exists).
- The wakeup created on inbox resolution depends on the agent being in a wakeup-eligible status. If the agent was paused (e.g., budget exceeded), the wakeup will be discarded per existing REQ-044.

---

## References

- PRD: `docx/core/01-PRODUCT.md` (sections 5.2, 7, 9)
- Agent Runtime requirements: `docx/features/11-agent-runtime/requirements.md` (REQ-004, REQ-008, REQ-024, REQ-025, REQ-047)
- Budget enforcement: `internal/server/handlers/budget_service.go`
- Run service: `internal/server/handlers/run_handler.go`
- SSE hub: `internal/server/sse/hub.go`
