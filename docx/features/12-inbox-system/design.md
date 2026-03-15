# Design: Inbox System (Human-in-the-Loop Queue)

**Created:** 2026-03-15
**Status:** Ready for Implementation

---

## Architecture Overview

The inbox is the unified governance interface in Ari. It consolidates all events requiring human attention — approvals, questions, decisions, and alerts (budget warnings, agent errors, etc.) — into a single prioritized queue per squad. The PRD defines four categories: `approval`, `question`, `decision`, `alert`. The `type` TEXT field differentiates within each category (e.g., `budget_threshold_80`, `run_failed`). The inbox integrates with the existing `BudgetEnforcementService`, `RunService`, and `WakeupService` to auto-create items and trigger agent wakeups on resolution.

### High-Level Component Relationships

```
Budget Enforcement ──┐
                     ├──> InboxService.Create() ──> DB insert
Run Finalization ────┘           │                     │
                                 ├──> SSE Hub.Publish("inbox.item.created")
Agent API Call ──> InboxHandler ──┘    │
                                       ├──> ActivityLog append
                                       │
User resolves ──> InboxHandler ──> InboxService.Resolve()
                                       │
                                       ├──> DB update (status=resolved)
                                       ├──> SSE Hub.Publish("inbox.item.resolved")
                                       ├──> WakeupService.Enqueue(inbox_resolved)
                                       └──> ActivityLog append
```

### Squad Isolation

Every `InboxItem` is keyed by `squad_id`. The list, count, and detail endpoints verify squad membership before returning data. Auto-created items inherit the squad from the triggering agent.

---

## Database Schema

### Migration: `20260315000014_create_inbox_items.sql`

```sql
-- +goose Up

CREATE TYPE inbox_category AS ENUM (
    'approval', 'question', 'decision', 'alert'
);

CREATE TYPE inbox_urgency AS ENUM (
    'critical', 'normal', 'low'
);

CREATE TYPE inbox_status AS ENUM (
    'pending', 'acknowledged', 'resolved', 'expired'
);

CREATE TYPE inbox_resolution AS ENUM (
    'approved', 'rejected', 'request_revision', 'answered', 'dismissed'
);

-- Ensure 'inbox_resolved' exists in wakeup_invocation_source enum.
-- If not already present from a prior migration, uncomment:
-- ALTER TYPE wakeup_invocation_source ADD VALUE IF NOT EXISTS 'inbox_resolved';

CREATE TABLE inbox_items (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id              UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    category              inbox_category NOT NULL,
    type                  TEXT NOT NULL,
    status                inbox_status NOT NULL DEFAULT 'pending',
    urgency               inbox_urgency NOT NULL DEFAULT 'normal',
    title                 TEXT NOT NULL,
    body                  TEXT,
    payload               JSONB NOT NULL DEFAULT '{}',

    -- Source references
    requested_by_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    related_agent_id      UUID REFERENCES agents(id) ON DELETE SET NULL,
    related_issue_id      UUID REFERENCES issues(id) ON DELETE SET NULL,
    related_run_id        UUID REFERENCES heartbeat_runs(id) ON DELETE SET NULL,

    -- Resolution
    resolution            inbox_resolution,
    response_note         TEXT,
    response_payload      JSONB,
    resolved_by_user_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    resolved_at           TIMESTAMPTZ,

    -- Acknowledgment
    acknowledged_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    acknowledged_at         TIMESTAMPTZ,

    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Primary listing query: squad-scoped, sorted by urgency then recency
CREATE INDEX idx_inbox_items_squad_list ON inbox_items(squad_id, urgency, created_at DESC);

-- Filter by status (for badge counts and filtered views)
CREATE INDEX idx_inbox_items_squad_status ON inbox_items(squad_id, status)
    WHERE status NOT IN ('resolved', 'expired');

-- Deduplication for auto-created alert items (one active alert per agent per type)
CREATE UNIQUE INDEX uq_inbox_active_alert_per_agent_type ON inbox_items(squad_id, related_agent_id, type)
    WHERE category = 'alert' AND status IN ('pending', 'acknowledged');

-- +goose Down
DROP TABLE IF EXISTS inbox_items;
DROP TYPE IF EXISTS inbox_resolution;
DROP TYPE IF EXISTS inbox_status;
DROP TYPE IF EXISTS inbox_urgency;
DROP TYPE IF EXISTS inbox_category;
```

### Column Rationale

| Column | Purpose |
|--------|---------|
| `category` | Determines resolution options and wakeup behavior |
| `type` | Specific action string (e.g., `budget_threshold_80`, `run_failed`, `clarify_requirement`) |
| `urgency` | Sorting priority in the inbox list |
| `requested_by_agent_id` | Agent that created the item (for agent-initiated items) |
| `related_agent_id` | Agent this item is about (for system-generated items like budget/error) |
| `related_issue_id` | Optional link to a relevant issue |
| `related_run_id` | Optional link to a heartbeat run (for error items) |
| `payload` | Structured data specific to the category (options list, budget amounts, error details) |
| `response_note` | Free-text user response (answer to a question, rejection reason) |
| `response_payload` | Structured response (selected option ID, approval conditions) |

---

## API Endpoints

### POST /api/squads/{id}/inbox — Create Inbox Item

**Auth:** User session OR Agent Run Token (squad must match)

**Request:**
```json
{
  "category": "question",
  "type": "clarify_requirement",
  "title": "Need clarification on auth flow",
  "body": "Should login support SSO or just email/password for v1?",
  "urgency": "normal",
  "relatedIssueId": "uuid-optional",
  "payload": {
    "options": ["SSO + email/password", "Email/password only"],
    "context": "Working on ARI-42"
  }
}
```

**Response (201):**
```json
{
  "id": "uuid",
  "squadId": "uuid",
  "category": "question",
  "type": "clarify_requirement",
  "status": "pending",
  "urgency": "normal",
  "title": "Need clarification on auth flow",
  "body": "Should login support SSO or just email/password for v1?",
  "requestedByAgentId": "uuid",
  "relatedIssueId": "uuid",
  "payload": { ... },
  "createdAt": "2026-03-15T10:00:00Z"
}
```

**Validation:**
- `category` must be a valid enum value
- `title` is required, max 500 characters
- `urgency` defaults to `normal` if not provided
- `type` is required, max 100 characters
- Agent callers: `requestedByAgentId` is set from Run Token; squad must match

### GET /api/squads/{id}/inbox — List Inbox Items

**Auth:** User session (squad membership required)

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `limit` | int | 50 | Max items per page (max 200) |
| `offset` | int | 0 | Pagination offset |
| `category` | string | — | Filter by category |
| `urgency` | string | — | Filter by urgency |
| `status` | string | — | Filter by status |

**Response (200):**
```json
{
  "data": [
    {
      "id": "uuid",
      "squadId": "uuid",
      "category": "alert",
      "type": "budget_threshold_80",
      "status": "pending",
      "urgency": "normal",
      "title": "Agent alice is at 80% budget",
      "requestedByAgentId": null,
      "relatedAgentId": "uuid",
      "createdAt": "2026-03-15T10:00:00Z"
    }
  ],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total": 12
  }
}
```

**Sorting:** urgency (critical > normal > low), then `created_at DESC`.

### GET /api/inbox/{id} — Get Inbox Item Detail

**Auth:** User session (squad membership required)

**Response (200):**
```json
{
  "id": "uuid",
  "squadId": "uuid",
  "category": "question",
  "type": "clarify_requirement",
  "status": "resolved",
  "urgency": "normal",
  "title": "Need clarification on auth flow",
  "body": "Should login support SSO or just email/password for v1?",
  "requestedByAgentId": "uuid",
  "relatedIssueId": "uuid",
  "payload": { "options": ["SSO + email/password", "Email/password only"] },
  "resolution": "answered",
  "responseNote": "Email/password only for v1. SSO in Phase 3.",
  "responsePayload": { "selectedOption": 1 },
  "resolvedByUserId": "uuid",
  "resolvedAt": "2026-03-15T10:30:00Z",
  "acknowledgedByUserId": null,
  "acknowledgedAt": null,
  "createdAt": "2026-03-15T10:00:00Z",
  "updatedAt": "2026-03-15T10:30:00Z"
}
```

### GET /api/squads/{id}/inbox/count — Badge Count

**Auth:** User session (squad membership required)

**Response (200):**
```json
{
  "pending": 5,
  "acknowledged": 2,
  "total": 7
}
```

### PATCH /api/inbox/{id}/resolve — Resolve Inbox Item

**Auth:** User session (squad membership required)

**Request:**
```json
{
  "resolution": "answered",
  "responseNote": "Email/password only for v1.",
  "responsePayload": { "selectedOption": 1 }
}
```

**Response (200):** Full updated inbox item (same shape as GET detail).

**Error (409):**
```json
{
  "error": "Inbox item is already resolved",
  "code": "ALREADY_RESOLVED"
}
```

**Validation:**
- `resolution` must be valid for the item's category (see REQ-INB-022)
- Item must be in `pending` or `acknowledged` status

**Side effects:**
1. Sets `resolvedByUserId`, `resolvedAt`, `responseNote`, `responsePayload`
2. For categories `approval`, `question`, `decision`: creates a `WakeupRequest` with `invocationSource=inbox_resolved` for the `requestedByAgentId`
3. Emits `inbox.item.resolved` SSE event
4. Appends `ActivityLog` entry

### PATCH /api/inbox/{id}/acknowledge — Acknowledge Inbox Item

**Auth:** User session (squad membership required)

**Request:** Empty body (no payload needed).

**Response (200):** Full updated inbox item.

**Validation:**
- Item must be in `pending` status

**Side effects:**
1. Sets `acknowledgedByUserId`, `acknowledgedAt`, `status=acknowledged`
2. Emits `inbox.item.acknowledged` SSE event

### PATCH /api/inbox/{id}/dismiss — Dismiss Inbox Item (Alerts Only)

**Auth:** User session (squad membership required)

**Request:**
```json
{
  "responseNote": "Noted, will monitor."
}
```

**Response (200):** Full updated inbox item (same shape as GET detail, with `resolution=dismissed`).

**Validation:**
- Item must have `category=alert`; returns 400 with `code=INVALID_RESOLUTION` for non-alert categories
- Item must be in `pending` or `acknowledged` status

**Side effects:**
This is a convenience shortcut equivalent to `PATCH /api/inbox/{id}/resolve` with `resolution=dismissed`. It:
1. Sets `status=resolved`, `resolution=dismissed`, `resolvedByUserId`, `resolvedAt`, and optional `responseNote`
2. Does NOT create a `WakeupRequest` (alerts are informational)
3. Emits `inbox.item.resolved` SSE event
4. Appends `ActivityLog` entry

---

## Domain Model

### File: `internal/domain/inbox.go`

```go
package domain

// InboxCategory represents the type of attention an inbox item requires.
// Aligned with PRD: approval, question, decision, alert.
type InboxCategory string

const (
    InboxCategoryApproval InboxCategory = "approval"
    InboxCategoryQuestion InboxCategory = "question"
    InboxCategoryDecision InboxCategory = "decision"
    InboxCategoryAlert    InboxCategory = "alert"
)

// InboxUrgency represents the priority level of an inbox item.
// Aligned with PRD: critical, normal, low.
type InboxUrgency string

const (
    InboxUrgencyCritical InboxUrgency = "critical"
    InboxUrgencyNormal   InboxUrgency = "normal"
    InboxUrgencyLow      InboxUrgency = "low"
)

// InboxStatus represents the lifecycle state of an inbox item.
type InboxStatus string

const (
    InboxStatusPending      InboxStatus = "pending"
    InboxStatusAcknowledged InboxStatus = "acknowledged"
    InboxStatusResolved     InboxStatus = "resolved"
    InboxStatusExpired      InboxStatus = "expired"
)

// InboxResolution represents how an inbox item was resolved.
type InboxResolution string

const (
    InboxResolutionApproved        InboxResolution = "approved"
    InboxResolutionRejected        InboxResolution = "rejected"
    InboxResolutionRequestRevision InboxResolution = "request_revision"
    InboxResolutionAnswered        InboxResolution = "answered"
    InboxResolutionDismissed       InboxResolution = "dismissed"
)

// ValidateInboxStatusTransition checks if a status transition is allowed.
func ValidateInboxStatusTransition(from, to InboxStatus) error {
    // pending -> acknowledged | resolved | expired
    // acknowledged -> resolved | expired
    // resolved -> (terminal)
    // expired -> (terminal)
    ...
}

// ValidResolutionsForCategory returns the allowed resolution types for a given category.
func ValidResolutionsForCategory(category InboxCategory) []InboxResolution {
    switch category {
    case InboxCategoryApproval:
        return []InboxResolution{InboxResolutionApproved, InboxResolutionRejected, InboxResolutionRequestRevision}
    case InboxCategoryQuestion, InboxCategoryDecision:
        return []InboxResolution{InboxResolutionAnswered, InboxResolutionDismissed}
    case InboxCategoryAlert:
        return []InboxResolution{InboxResolutionDismissed}
    default:
        return nil
    }
}

// CategoryWakesAgent returns true if resolving items of this category should wake the requesting agent.
func CategoryWakesAgent(category InboxCategory) bool {
    switch category {
    case InboxCategoryApproval, InboxCategoryQuestion, InboxCategoryDecision:
        return true
    default:
        return false
    }
}
```

### Status Machine

```
pending ──────> acknowledged ──────> resolved
   │                │                    ^
   │                └─────> expired      │
   │                          ^          │
   ├──────────────────────────┘          │
   └─────────────────────────────────────┘
```

- `pending`: Item created, awaiting human attention.
- `acknowledged`: User has seen the item but not yet resolved it.
- `resolved`: User has taken action; terminal state.
- `expired`: Item is no longer actionable; terminal state. (Auto-expiry not implemented in v1; status available for manual use.)

---

## Integration Points

### 1. BudgetEnforcementService Integration

**File:** `internal/server/handlers/budget_service.go`

Modify `RecordAndEnforce()` to accept an `InboxService` dependency and call it when thresholds are crossed:

```go
// In RecordAndEnforce, after threshold detection:
// Pass the transactional qtx so the inbox item is created atomically
// within the same transaction as the cost event.
if threshold == domain.ThresholdWarning {
    _ = s.inboxService.CreateBudgetWarning(ctx, qtx, CreateBudgetWarningParams{
        SquadID:    params.SquadID,
        AgentID:    params.AgentID,
        Type:       "budget_threshold_80",
        Urgency:    domain.InboxUrgencyNormal,
        SpentCents: agentSpend,
        BudgetCents: budgetVal,
    })
}
if threshold == domain.ThresholdExceeded {
    _ = s.inboxService.CreateBudgetWarning(ctx, qtx, CreateBudgetWarningParams{
        SquadID:    params.SquadID,
        AgentID:    params.AgentID,
        Type:       "budget_threshold_100",
        Urgency:    domain.InboxUrgencyCritical,
        SpentCents: agentSpend,
        BudgetCents: budgetVal,
    })
}
```

The `InboxService.CreateBudgetWarning(ctx, qtx, params)` method accepts a `*db.Queries` parameter (the transactional `qtx`) instead of using the service's own `s.queries`. This ensures the inbox item is created within the same database transaction as the cost event. It uses `INSERT ... ON CONFLICT DO NOTHING` on the unique partial index to deduplicate within a billing period.

### 2. RunService Integration

**File:** `internal/server/handlers/run_handler.go`

Modify `finalize()` to create an inbox item when a run fails:

```go
// In finalize(), after marking HeartbeatRun as failed:
// Note: finalize() is non-transactional, so agent error inbox items are
// best-effort. Failure to create the inbox item must NOT prevent run
// finalization. Pass s.queries (non-transactional) directly.
if result.Status == adapter.RunStatusFailed || result.Status == adapter.RunStatusTimedOut {
    _ = s.inboxService.CreateAgentError(ctx, s.queries, CreateAgentErrorParams{
        SquadID:       wakeup.SquadID,
        AgentID:       agent.ID,
        RunID:         run.ID,
        Type:          "run_failed", // or "run_timed_out"
        ExitCode:      result.ExitCode,
        StderrExcerpt: result.Stderr,
    })
}
```

### 3. WakeupService Integration

**File:** `internal/server/handlers/wakeup_handler.go`

On inbox resolution, the `InboxHandler` calls the existing `WakeupService.Enqueue()` with `invocationSource=inbox_resolved`:

```go
// In InboxHandler.Resolve(), after DB update:
if domain.CategoryWakesAgent(item.Category) && item.RequestedByAgentID.Valid {
    _, _ = s.wakeupService.Enqueue(ctx, item.RequestedByAgentID.UUID, item.SquadID, "inbox_resolved", map[string]any{
        "inbox_item_id":    item.ID,
        "resolution":       resolution,
        "response_note":    responseNote,
        "response_payload": responsePayload,
    })
}
```

### 4. SSE Event Shapes

**`inbox.item.created`:**
```json
{
  "itemId": "uuid",
  "category": "alert",
  "type": "budget_threshold_80",
  "urgency": "normal",
  "title": "Agent alice is at 80% budget"
}
```

**`inbox.item.resolved`:**
```json
{
  "itemId": "uuid",
  "resolvedByUserId": "uuid",
  "resolution": "dismissed",
  "resolvedAt": "2026-03-15T10:30:00Z"
}
```

**`inbox.item.acknowledged`:**
```json
{
  "itemId": "uuid",
  "acknowledgedByUserId": "uuid"
}
```

---

## SQL Queries (sqlc)

### File: `internal/database/queries/inbox_items.sql`

```sql
-- name: CreateInboxItem :one
INSERT INTO inbox_items (
    squad_id, category, type, urgency, title, body, payload,
    requested_by_agent_id, related_agent_id, related_issue_id, related_run_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetInboxItemByID :one
SELECT * FROM inbox_items WHERE id = $1;

-- name: ListInboxItemsBySquad :many
SELECT * FROM inbox_items
WHERE squad_id = @squad_id
  AND (CASE WHEN @filter_category::inbox_category IS NOT NULL
       THEN category = @filter_category ELSE TRUE END)
  AND (CASE WHEN @filter_urgency::inbox_urgency IS NOT NULL
       THEN urgency = @filter_urgency ELSE TRUE END)
  AND (CASE WHEN @filter_status::inbox_status IS NOT NULL
       THEN status = @filter_status ELSE TRUE END)
ORDER BY
    CASE urgency
        WHEN 'critical' THEN 0
        WHEN 'normal' THEN 1
        WHEN 'low' THEN 2
    END,
    created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountInboxItemsBySquad :one
SELECT COUNT(*) FROM inbox_items
WHERE squad_id = @squad_id
  AND (CASE WHEN @filter_category::inbox_category IS NOT NULL
       THEN category = @filter_category ELSE TRUE END)
  AND (CASE WHEN @filter_urgency::inbox_urgency IS NOT NULL
       THEN urgency = @filter_urgency ELSE TRUE END)
  AND (CASE WHEN @filter_status::inbox_status IS NOT NULL
       THEN status = @filter_status ELSE TRUE END);

-- name: CountUnresolvedInboxBySquad :one
SELECT
    COUNT(*) FILTER (WHERE status = 'pending') AS pending_count,
    COUNT(*) FILTER (WHERE status = 'acknowledged') AS acknowledged_count,
    COUNT(*) FILTER (WHERE status IN ('pending', 'acknowledged')) AS total_count
FROM inbox_items
WHERE squad_id = $1;

-- name: ResolveInboxItem :one
UPDATE inbox_items SET
    status = 'resolved',
    resolution = $2,
    response_note = $3,
    response_payload = $4,
    resolved_by_user_id = $5,
    resolved_at = now(),
    updated_at = now()
WHERE id = $1 AND status != 'resolved'
RETURNING *;

-- name: AcknowledgeInboxItem :one
UPDATE inbox_items SET
    status = 'acknowledged',
    acknowledged_by_user_id = $2,
    acknowledged_at = now(),
    updated_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: CreateInboxItemOnConflictDoNothing :one
INSERT INTO inbox_items (
    squad_id, category, type, urgency, title, body, payload,
    related_agent_id, related_run_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT DO NOTHING
RETURNING *;
-- Note: Returns nil when the row was deduplicated (ON CONFLICT).
-- The service layer must handle a nil return gracefully (item already exists, no SSE event needed).
```

---

## InboxService

### File: `internal/server/handlers/inbox_service.go`

```go
// InboxService handles inbox item creation, resolution, and side-effects.
type InboxService struct {
    queries       *db.Queries
    dbConn        *sql.DB
    sseHub        *sse.Hub
    wakeupService *WakeupService
}
```

**Key methods:**

| Method | Purpose |
|--------|---------|
| `Create(ctx, params)` | Insert item, emit SSE, log activity |
| `Resolve(ctx, itemID, userID, resolution, note, payload)` | Update status, emit SSE, optionally enqueue wakeup, log activity |
| `Acknowledge(ctx, itemID, userID)` | Update status, emit SSE |
| `CreateBudgetWarning(ctx, qtx, params)` | Auto-create budget alert (ON CONFLICT DO NOTHING). Accepts transactional `*db.Queries` (`qtx`) for atomic creation within budget enforcement transaction. Returns nil item if deduplicated. |
| `CreateAgentError(ctx, qtx, params)` | Auto-create agent error alert (ON CONFLICT DO NOTHING). Accepts `*db.Queries` — typically the service's own non-transactional queries since `finalize()` is non-transactional. Returns nil item if deduplicated. |

---

## InboxHandler

### File: `internal/server/handlers/inbox_handler.go`

```go
type InboxHandler struct {
    queries      *db.Queries
    inboxService *InboxService
}

func (h *InboxHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /api/squads/{id}/inbox", h.CreateInboxItem)
    mux.HandleFunc("GET /api/squads/{id}/inbox", h.ListInboxItems)
    mux.HandleFunc("GET /api/squads/{id}/inbox/count", h.GetInboxCount)
    mux.HandleFunc("GET /api/inbox/{id}", h.GetInboxItem)
    mux.HandleFunc("PATCH /api/inbox/{id}/resolve", h.ResolveInboxItem)
    mux.HandleFunc("PATCH /api/inbox/{id}/acknowledge", h.AcknowledgeInboxItem)
    mux.HandleFunc("PATCH /api/inbox/{id}/dismiss", h.DismissInboxItem)
}
```

**Auth patterns:**
- `POST /api/squads/{id}/inbox` — accepts both user sessions (via `auth.UserFromContext`) and agent Run Tokens (via `auth.AgentFromContext`). If agent, `requestedByAgentId` is set from the token.
- All other endpoints — user session only, with squad membership verification.

---

## React Component Structure

### New Components

```
web/src/
  components/
    inbox/
      InboxList.tsx          # Main inbox page with filters + list
      InboxItemCard.tsx      # Individual item card in list view
      InboxItemDetail.tsx    # Full detail view with resolve form
      InboxResolveForm.tsx   # Resolution form (varies by category)
      InboxBadge.tsx         # Badge count for nav/header
      InboxFilters.tsx       # Filter bar (category, urgency, status)
  hooks/
    useInbox.ts              # API calls + SSE subscription for inbox
  pages/
    InboxPage.tsx            # Route: /inbox (list view)
    InboxDetailPage.tsx      # Route: /inbox/:id (detail view)
```

### InboxBadge Integration

The `InboxBadge` component calls `GET /api/squads/{id}/inbox/count` on mount and subscribes to `inbox.item.created` / `inbox.item.resolved` SSE events to update the count in real time. It renders a numeric badge in the sidebar navigation.

### InboxResolveForm Variants

| Category | Form Elements |
|----------|--------------|
| `approval` | Approve / Reject / Request Revision buttons + optional note |
| `question` | Text area for answer |
| `decision` | Radio/select from `payload.options` + optional note |
| `alert` | Dismiss button + optional note |

---

## Error Handling

| Scenario | HTTP Code | Error Code |
|----------|-----------|------------|
| Invalid category/urgency | 400 | `VALIDATION_ERROR` |
| Missing required field (title, type) | 400 | `VALIDATION_ERROR` |
| Invalid resolution for category | 400 | `INVALID_RESOLUTION` |
| Squad not found | 404 | `NOT_FOUND` |
| Inbox item not found | 404 | `NOT_FOUND` |
| Not a squad member | 403 | `FORBIDDEN` |
| Already resolved | 409 | `ALREADY_RESOLVED` |
| Already acknowledged (for acknowledge) | 409 | `ALREADY_ACKNOWLEDGED` |
| Unauthenticated | 401 | `UNAUTHENTICATED` |

---

## Testing Strategy

1. **Unit tests** for domain validation (`ValidateInboxStatusTransition`, `ValidResolutionsForCategory`)
2. **Integration tests** for sqlc queries (embedded DB)
3. **Handler tests** for HTTP endpoints (mock queries)
4. **Integration tests** for budget/run auto-creation (verify inbox items are created alongside budget/run events)
5. **SSE tests** for event emission (verify events are published to the hub)
