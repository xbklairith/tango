# Design: Approval Gates

**Created:** 2026-03-15
**Status:** Ready for Implementation

---

## Architecture Overview

Approval gates add a governance layer on top of the existing inbox system. Agents discover gated actions via a self-service endpoint, create approval requests as inbox items, and are suspended until a human resolves the request or it times out. A background goroutine handles timeout-based auto-resolution.

No new database tables are introduced. Gate rules are stored in the squad `settings` JSONB column. Approval requests are standard `category=approval` inbox items with a structured `payload` containing the gate reference, action details, and computed expiration timestamp.

### High-Level Component Relationships

```
Squad Settings (JSONB)
  └── approvalGates[] ── gate rules (action patterns, timeouts, auto-resolution)

Agent ──> GET /api/agent/me/gates ──> reads squad.settings.approvalGates
  │
  └──> POST /api/squads/{id}/inbox (category=approval)
           │
           ├──> InboxService.Create() ──> DB insert (with expiresAt in payload)
           ├──> SSE Hub.Publish("inbox.item.created")
           └──> ActivityLog append

User ──> PATCH /api/inbox/{id}/resolve ──> InboxService.Resolve()
           │
           ├──> DB update (status=resolved)
           ├──> WakeupService.Enqueue(inbox_resolved)
           ├──> SSE Hub.Publish("inbox.item.resolved")
           └──> ActivityLog append

Background Goroutine (ApprovalTimeoutChecker)
  │  runs every 60s
  └──> Query expired approval items
       └──> For each: auto-resolve with gate's autoResolution
            ├──> DB update (status=resolved)
            ├──> WakeupService.Enqueue(inbox_resolved)
            ├──> SSE Hub.Publish("inbox.item.resolved")
            └──> ActivityLog append (action=inbox.auto_resolved)
```

### Squad Isolation

All gate rules are stored per-squad in `settings`. Approval inbox items are squad-scoped via `squad_id`. The agent self-service endpoint returns gates only for the agent's own squad. The timeout checker processes items across all squads but each resolution is scoped to the item's squad for SSE and activity logging.

---

## Database Schema

### Migration: Add `expires_at` Column to `inbox_items`

A small migration adds a nullable `expires_at` column to the `inbox_items` table, used only for approval items:

```sql
-- +goose Up
ALTER TABLE inbox_items ADD COLUMN expires_at TIMESTAMPTZ;

CREATE INDEX idx_inbox_items_approval_expires
    ON inbox_items(expires_at)
    WHERE category = 'approval'
      AND status IN ('pending', 'acknowledged')
      AND expires_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_inbox_items_approval_expires;
ALTER TABLE inbox_items DROP COLUMN IF EXISTS expires_at;
```

This column is only set for `category=approval` items. The partial index enables efficient timeout queries without JSONB extraction. Gate rules are stored in the `squads.settings` JSONB column.

### Squad Settings Schema Extension

The `settings` JSONB on the `squads` table is extended to include an `approvalGates` array:

```json
{
  "requireApprovalForNewAgents": false,
  "approvalGates": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Production Deploy",
      "actionPattern": "deploy",
      "requiredApprovers": 1,
      "timeoutHours": 24,
      "autoResolution": "rejected"
    },
    {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "name": "Delete Resource",
      "actionPattern": "delete",
      "requiredApprovers": 1,
      "timeoutHours": 12,
      "autoResolution": "rejected"
    },
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "name": "High-Cost Action",
      "actionPattern": "spend_over_500",
      "requiredApprovers": 1,
      "timeoutHours": 48,
      "autoResolution": "rejected"
    }
  ]
}
```

### Gate Rule Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `id` | UUID | No (auto-generated) | `gen_random_uuid()` | Unique identifier for the gate rule |
| `name` | string | Yes | — | Human-readable name for display |
| `actionPattern` | string | Yes | — | Action identifier label (e.g., `deploy`, `delete`, `spend_over_500`). **Note:** patterns are opaque labels in v1, not glob-matched. Agents must use exact action names when referencing gates. |
| `requiredApprovers` | int | No | 1 | Number of approvals required (v1: only 1 enforced) |
| `timeoutHours` | int | Yes | — | Hours before auto-resolution; minimum 1, maximum 168 (7 days) |
| `autoResolution` | string | No | `"rejected"` | Resolution on timeout: `"rejected"` or `"approved"` |

### Approval Inbox Item Payload Convention

When an agent creates an approval request, the `payload` JSONB follows this structure:

```json
{
  "gateId": "550e8400-e29b-41d4-a716-446655440000",
  "gateName": "Production Deploy",
  "actionPattern": "deploy",
  "autoResolution": "rejected",
  "timeoutHours": 24,
  "actionDetails": {
    "target": "production",
    "version": "v2.3.1",
    "description": "Deploy new authentication module to production"
  }
}
```

The `expires_at` timestamp is stored in the dedicated `inbox_items.expires_at` column (not in payload), computed using database time: `now() + interval 'N hours'`.

| Payload Field | Type | Description |
|--------------|------|-------------|
| `gateId` | UUID | Reference to the gate rule in squad settings |
| `gateName` | string | Snapshot of gate name at creation time |
| `actionPattern` | string | Snapshot of the action pattern |
| `autoResolution` | string | Snapshot of the gate's auto-resolution setting |
| `timeoutHours` | int | Snapshot of the gate's timeout hours |
| `actionDetails` | object | Agent-provided details about the specific action |

**Note:** The `expires_at` is NOT stored in the payload. It is computed by the SQL INSERT query as `now() + interval 'N hours'` using database time, avoiding clock skew issues. The column is on the `inbox_items` table.

The gate configuration is snapshotted into the payload at creation time so that subsequent gate edits or deletions do not affect pending approval items.

---

## SQL Queries (sqlc)

### File: `internal/database/queries/inbox_items.sql` (additions)

```sql
-- name: ListExpiredApprovalItems :many
-- Returns approval inbox items that have passed their expires_at timestamp
-- and are still pending or acknowledged. Used by the background timeout checker.
-- Uses the expires_at column (not JSONB extraction) for efficient indexed queries.
SELECT * FROM inbox_items
WHERE category = 'approval'
  AND status IN ('pending', 'acknowledged')
  AND expires_at IS NOT NULL
  AND expires_at <= now()
ORDER BY created_at ASC
LIMIT $1;

-- name: AutoResolveInboxItem :one
-- System-initiated resolution (no user). Uses CAS on status for idempotency.
-- Explicitly sets resolved_by_user_id = NULL to indicate system-resolved.
UPDATE inbox_items SET
    status = 'resolved',
    resolution = @resolution,
    response_note = @response_note,
    resolved_by_user_id = NULL,
    resolved_at = now(),
    updated_at = now()
WHERE id = @id AND status IN ('pending', 'acknowledged')
RETURNING *;
```

**Notes:**
- `ListExpiredApprovalItems` uses the `expires_at` column with a partial index for efficient queries. The LIMIT enables batch processing (default 1000, per REQ-APG-029). If items remain, the caller re-queries.
- `AutoResolveInboxItem` **explicitly** sets `resolved_by_user_id = NULL` to indicate system-initiated resolution (H-1 fix).
- The CAS condition `status IN ('pending', 'acknowledged')` ensures idempotency if a user resolves the item between the query and the update.

### Existing Queries Used

The following existing queries from Feature 12 are reused without modification:
- `CreateInboxItem` — for creating approval requests
- `GetInboxItemByID` — for fetching item details
- `ListInboxItemsBySquad` — for listing with `category=approval` filter
- `ResolveInboxItem` — for user-initiated resolution
- `ExpireInboxItem` — available but not used (auto-resolve uses `AutoResolveInboxItem` with a resolution value instead)

---

## API Endpoints

### GET /api/agent/me/gates -- Agent Gate Discovery

**Auth:** Agent Run Token only

**Response (200):**
```json
{
  "gates": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Production Deploy",
      "actionPattern": "deploy",
      "requiredApprovers": 1,
      "timeoutHours": 24,
      "autoResolution": "rejected"
    },
    {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "name": "Delete Resource",
      "actionPattern": "delete",
      "requiredApprovers": 1,
      "timeoutHours": 12,
      "autoResolution": "rejected"
    }
  ]
}
```

**Response (200, no gates):**
```json
{
  "gates": []
}
```

This endpoint reads from the agent's squad `settings` JSONB. It does not query the `inbox_items` table.

### POST /api/squads/{id}/inbox -- Create Approval Request

Uses the existing inbox creation endpoint (Feature 12). The approval gate logic adds validation and payload enrichment:

**Request (agent creates approval):**
```json
{
  "category": "approval",
  "type": "gate_approval",
  "title": "Approval required: Deploy v2.3.1 to production",
  "body": "Agent requests approval to deploy the new authentication module to production.",
  "urgency": "normal",
  "payload": {
    "gateId": "550e8400-e29b-41d4-a716-446655440000",
    "actionDetails": {
      "target": "production",
      "version": "v2.3.1",
      "description": "Deploy new authentication module to production"
    }
  }
}
```

**Enrichment by InboxService:**
When the inbox service detects `category=approval` with a `gateId` in the payload, it:
1. Unmarshals `params.Payload` (json.RawMessage) into `map[string]any`.
2. Loads the squad's `approvalGates` settings.
3. Finds the matching gate by `gateId`.
4. Snapshots `gateName`, `actionPattern`, `autoResolution`, `timeoutHours` into the map.
5. Marshals the map back to `json.RawMessage` for the DB insert.
6. Sets the `expires_at` column via the SQL query using `now() + interval 'N hours'` (database time).
7. If the gate is not found, defaults to `timeoutHours=24`, `autoResolution=rejected`, and logs a warning.

**Canonical path for agents (XH-4):** `POST /api/agent/me/inbox` (from Feature 15) is the canonical path for agents to create inbox items, but `POST /api/squads/{id}/inbox` also accepts agent Run Token auth. The enrichment in `InboxService.Create()` triggers regardless of which handler calls it, so both paths produce identical results.

**Response (201):** Standard inbox item response with enriched payload.

### PATCH /api/squads/{id} -- Update Gate Configuration

Uses the existing squad update endpoint. The settings validation is extended to validate the `approvalGates` array.

**Request (partial, settings only):**
```json
{
  "settings": {
    "approvalGates": [
      {
        "name": "Production Deploy",
        "actionPattern": "deploy",
        "requiredApprovers": 1,
        "timeoutHours": 24,
        "autoResolution": "rejected"
      }
    ]
  }
}
```

**Validation rules for each gate entry:**
- `name` required, max 100 characters
- `actionPattern` required, max 100 characters, alphanumeric + underscores + asterisks
- `timeoutHours` required, integer, minimum 1, maximum 168
- `autoResolution` optional, must be `"rejected"` or `"approved"`, defaults to `"rejected"`
- `requiredApprovers` optional, integer >= 1, defaults to 1
- `id` optional; generated if absent, preserved if present
- Maximum 50 gate entries per squad

---

## Domain Model

### File: `internal/domain/approval_gate.go`

```go
package domain

import (
    "fmt"
    "time"

    "github.com/google/uuid"
)

// ApprovalGate represents a single gate rule configured in squad settings.
type ApprovalGate struct {
    ID                uuid.UUID `json:"id"`
    Name              string    `json:"name"`
    ActionPattern     string    `json:"actionPattern"`
    RequiredApprovers int       `json:"requiredApprovers"`
    TimeoutHours      int       `json:"timeoutHours"`
    AutoResolution    string    `json:"autoResolution"`
}

// DefaultAutoResolution is the fallback when autoResolution is not specified.
const DefaultAutoResolution = "rejected"

// DefaultTimeoutHours is used when a gate is not found for a pending approval.
const DefaultTimeoutHours = 24

// MaxGatesPerSquad is the maximum number of gate rules per squad.
const MaxGatesPerSquad = 50

// ValidateApprovalGate validates a single gate entry.
func ValidateApprovalGate(g *ApprovalGate) error {
    if g.Name == "" {
        return fmt.Errorf("gate name is required")
    }
    if len(g.Name) > 100 {
        return fmt.Errorf("gate name must be 100 characters or fewer")
    }
    if g.ActionPattern == "" {
        return fmt.Errorf("gate actionPattern is required")
    }
    if len(g.ActionPattern) > 100 {
        return fmt.Errorf("gate actionPattern must be 100 characters or fewer")
    }
    if g.TimeoutHours < 1 || g.TimeoutHours > 168 {
        return fmt.Errorf("gate timeoutHours must be between 1 and 168")
    }
    if g.AutoResolution != "" && g.AutoResolution != "rejected" && g.AutoResolution != "approved" {
        return fmt.Errorf("gate autoResolution must be 'rejected' or 'approved'")
    }
    if g.RequiredApprovers < 0 {
        return fmt.Errorf("gate requiredApprovers must be >= 0")
    }
    return nil
}

// NormalizeApprovalGate fills in defaults for optional fields.
func NormalizeApprovalGate(g *ApprovalGate) {
    if g.ID == uuid.Nil {
        g.ID = uuid.New()
    }
    if g.AutoResolution == "" {
        g.AutoResolution = DefaultAutoResolution
    }
    if g.RequiredApprovers < 1 {
        g.RequiredApprovers = 1
    }
}

// ApprovalPayload is the structured payload for category=approval inbox items.
// Note: expires_at is stored in the inbox_items.expires_at column, NOT in payload.
type ApprovalPayload struct {
    GateID         uuid.UUID      `json:"gateId"`
    GateName       string         `json:"gateName"`
    ActionPattern  string         `json:"actionPattern"`
    AutoResolution string         `json:"autoResolution"`
    TimeoutHours   int            `json:"timeoutHours"`
    ActionDetails  map[string]any `json:"actionDetails"`
}

// FindGateByID searches the gates slice for a gate with the given ID.
func FindGateByID(gates []ApprovalGate, id uuid.UUID) *ApprovalGate {
    for i := range gates {
        if gates[i].ID == id {
            return &gates[i]
        }
    }
    return nil
}
```

### Squad Settings Type Extension

### File: `internal/domain/squad.go` (additions)

```go
// SquadSettings represents the JSONB settings for a squad.
type SquadSettings struct {
    RequireApprovalForNewAgents bool           `json:"requireApprovalForNewAgents"`
    ApprovalGates              []ApprovalGate `json:"approvalGates,omitempty"`
}
```

**Important implementation notes for `internal/domain/squad.go`:**

1. **`knownSettingsKeys` whitelist:** The `knownSettingsKeys` map must be updated to include `"approvalGates"`. Additionally, `ValidateSettingsKeys()` must validate that `approvalGates` is an array type (not a string or number):
   ```go
   var knownSettingsKeys = map[string]bool{
       "requireApprovalForNewAgents": true,
       "approvalGates":              true,
   }
   // In ValidateSettingsKeys, add type validation:
   if v, ok := raw["approvalGates"]; ok {
       if _, isSlice := v.([]any); !isSlice {
           return ValidationError{Field: "settings.approvalGates", Message: "must be an array"}
       }
   }
   ```

2. **`SquadSettings.Merge()` must handle `ApprovalGates`:** Since `SquadSettings` uses pointer types for optional fields, use `[]ApprovalGate` with `omitempty` for the slice. In `Merge()`, if the incoming patch has a non-nil `ApprovalGates` field, replace the entire slice (not field-by-field merge):
   ```go
   func (s *SquadSettings) Merge(patch SquadSettings) {
       if patch.RequireApprovalForNewAgents != nil {
           s.RequireApprovalForNewAgents = patch.RequireApprovalForNewAgents
       }
       if patch.ApprovalGates != nil {
           s.ApprovalGates = patch.ApprovalGates
       }
   }
   ```

3. The `ApprovalGates` field uses `[]ApprovalGate` with `json:"approvalGates,omitempty"` (not a pointer type). A nil slice means "not provided in the patch" and an empty slice `[]` means "clear all gates". This follows the standard Go pattern for JSON slices with omitempty.

---

## Service Layer

### File: `internal/server/handlers/inbox_service.go` (modifications)

The `InboxService` is extended with approval-gate-specific logic:

```go
// enrichApprovalResult holds the enrichment output: the updated payload
// and the timeoutHours to use for the expires_at column computation.
type enrichApprovalResult struct {
    Payload      map[string]any
    TimeoutHours int
}

// enrichApprovalPayload enriches an approval inbox item with gate metadata.
// Called from InboxService.Create() when category=approval.
// Does NOT compute expiresAt — that is done in the SQL INSERT using DB time.
// Returns the enriched payload map and the timeoutHours for the DB column.
//
// json.RawMessage ↔ map[string]any pattern:
//   1. Unmarshal params.Payload (json.RawMessage) into map[string]any
//   2. Add gate fields to the map
//   3. Marshal back to json.RawMessage for the DB insert
func (s *InboxService) enrichApprovalPayload(
    ctx context.Context,
    squadID uuid.UUID,
    rawPayload json.RawMessage,
) (*enrichApprovalResult, error) {
    // Unmarshal the raw JSON payload into a map for enrichment
    var payload map[string]any
    if err := json.Unmarshal(rawPayload, &payload); err != nil {
        payload = make(map[string]any)
    }

    // 1. Extract gateId from payload
    gateIDStr, _ := payload["gateId"].(string)
    gateID, err := uuid.Parse(gateIDStr)
    if err != nil {
        // No valid gateId — use defaults
        payload["autoResolution"] = domain.DefaultAutoResolution
        payload["timeoutHours"] = domain.DefaultTimeoutHours
        return &enrichApprovalResult{Payload: payload, TimeoutHours: domain.DefaultTimeoutHours}, nil
    }

    // 2. Load squad settings
    squad, err := s.queries.GetSquadByID(ctx, squadID)
    if err != nil {
        return nil, fmt.Errorf("load squad: %w", err)
    }

    var settings domain.SquadSettings
    _ = json.Unmarshal(squad.Settings, &settings)

    // 3. Find matching gate
    gate := domain.FindGateByID(settings.ApprovalGates, gateID)
    if gate == nil {
        slog.Warn("approval gate not found in squad settings, using defaults",
            "gateId", gateID, "squadId", squadID)
        payload["autoResolution"] = domain.DefaultAutoResolution
        payload["timeoutHours"] = domain.DefaultTimeoutHours
        return &enrichApprovalResult{Payload: payload, TimeoutHours: domain.DefaultTimeoutHours}, nil
    }

    // 4. Enrich payload with gate snapshot (no expiresAt — that goes in the column)
    payload["gateName"] = gate.Name
    payload["actionPattern"] = gate.ActionPattern
    payload["autoResolution"] = gate.AutoResolution
    payload["timeoutHours"] = gate.TimeoutHours

    return &enrichApprovalResult{Payload: payload, TimeoutHours: gate.TimeoutHours}, nil
}
```

### Modification to `InboxService.Create()`

Add a hook at the top of the existing `Create()` method:

```go
func (s *InboxService) Create(ctx context.Context, params db.CreateInboxItemParams) (*db.InboxItem, error) {
    // Enrich approval payloads with gate metadata
    if params.Category == db.InboxCategoryApproval {
        result, err := s.enrichApprovalPayload(ctx, params.SquadID, params.Payload)
        if err != nil {
            return nil, fmt.Errorf("enrich approval payload: %w", err)
        }
        // Marshal enriched map back to json.RawMessage for DB insert
        enrichedJSON, err := json.Marshal(result.Payload)
        if err != nil {
            return nil, fmt.Errorf("marshal enriched payload: %w", err)
        }
        params.Payload = enrichedJSON
        // Set expires_at column using DB time (computed in SQL as now() + interval)
        // This requires the CreateInboxItem query to accept an optional expires_at param,
        // or a separate UPDATE after insert, or a new CreateInboxItemWithExpiry query:
        //   INSERT INTO inbox_items (..., expires_at) VALUES (..., now() + @timeout_hours * interval '1 hour')
        params.ExpiresAt = sql.NullInt32{Int32: int32(result.TimeoutHours), Valid: true}
    }

    // ... existing creation logic (DB insert, SSE, activity log) ...
}
```

**Note on `expires_at` computation:** The cleanest approach is to add an optional `timeout_hours` parameter to the `CreateInboxItem` SQL query, which computes `expires_at` as `now() + @timeout_hours * interval '1 hour'` in the database. This avoids any clock skew between app and DB. When `timeout_hours` is NULL (non-approval items), `expires_at` remains NULL.

---

## Background Timeout Checker

### File: `internal/server/handlers/approval_timeout.go`

```go
package handlers

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log/slog"
    "time"

    "github.com/google/uuid"
    "github.com/xb/ari/internal/database/db"
    "github.com/xb/ari/internal/domain"
    "github.com/xb/ari/internal/server/sse"
)

// ApprovalTimeoutChecker periodically auto-resolves expired approval items.
type ApprovalTimeoutChecker struct {
    queries       *db.Queries
    dbConn        *sql.DB
    wakeupService *WakeupService
    sseHub        *sse.Hub
    interval      time.Duration
    batchSize     int
}

// NewApprovalTimeoutChecker creates a new checker.
func NewApprovalTimeoutChecker(
    q *db.Queries,
    dbConn *sql.DB,
    wakeupSvc *WakeupService,
    sseHub *sse.Hub,
) *ApprovalTimeoutChecker {
    return &ApprovalTimeoutChecker{
        queries:       q,
        dbConn:        dbConn,
        wakeupService: wakeupSvc,
        sseHub:        sseHub,
        interval:      60 * time.Second,
        batchSize:     1000, // per REQ-APG-029
    }
}

// Start launches the background loop. Blocks until ctx is cancelled.
func (c *ApprovalTimeoutChecker) Start(ctx context.Context) {
    ticker := time.NewTicker(c.interval)
    defer ticker.Stop()

    slog.Info("approval timeout checker started", "interval", c.interval)

    for {
        select {
        case <-ctx.Done():
            slog.Info("approval timeout checker stopped")
            return
        case <-ticker.C:
            c.processExpired(ctx)
        }
    }
}

func (c *ApprovalTimeoutChecker) processExpired(ctx context.Context) {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("approval timeout checker panic recovered", "panic", r)
        }
    }()

    // XH-2: Use pg_try_advisory_lock to ensure only one instance processes
    // expired items at a time in multi-instance deployments.
    // Lock ID is a fixed constant; if another instance holds the lock, skip this cycle.
    const advisoryLockID = 16_000_001 // unique ID for approval timeout checker
    var locked bool
    err := c.dbConn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", advisoryLockID).Scan(&locked)
    if err != nil || !locked {
        return // another instance is handling it, or DB error
    }
    defer c.dbConn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockID) //nolint:errcheck

    // Re-query loop: process batches until no more expired items remain (H-2 fix)
    for {
        items, err := c.queries.ListExpiredApprovalItems(ctx, int32(c.batchSize))
        if err != nil {
            slog.Error("approval timeout checker: query failed", "error", err)
            return
        }

        if len(items) == 0 {
            return
        }

        slog.Info("auto-resolving expired approval items", "count", len(items))

        for _, item := range items {
            c.autoResolveItem(ctx, item)
        }

        // If we got fewer items than batchSize, we've processed them all
        if len(items) < c.batchSize {
            return
        }
    }
}

func (c *ApprovalTimeoutChecker) autoResolveItem(ctx context.Context, item db.InboxItem) {
    // Extract autoResolution from payload
    var payload map[string]any
    _ = json.Unmarshal(item.Payload, &payload)

    resolution := domain.DefaultAutoResolution
    if ar, ok := payload["autoResolution"].(string); ok && (ar == "rejected" || ar == "approved") {
        resolution = ar
    }

    timeoutHours := domain.DefaultTimeoutHours
    if th, ok := payload["timeoutHours"].(float64); ok && th > 0 {
        timeoutHours = int(th)
    }

    responseNote := fmt.Sprintf(
        "Auto-resolved: approval timed out after %d hours (policy: %s)",
        timeoutHours, resolution,
    )

    // H-5: Wrap resolve + wakeup + activity log in a transaction
    // This matches the transactional pattern in InboxService.Resolve().
    tx, err := c.dbConn.BeginTx(ctx, nil)
    if err != nil {
        slog.Error("auto-resolve: begin tx failed", "itemId", item.ID, "error", err)
        return
    }
    defer tx.Rollback() //nolint:errcheck

    qtx := c.queries.WithTx(tx)

    // Auto-resolve via CAS query (idempotent)
    resolved, err := qtx.AutoResolveInboxItem(ctx, db.AutoResolveInboxItemParams{
        ID:           item.ID,
        Resolution:   db.InboxResolution(resolution),
        ResponseNote: sql.NullString{String: responseNote, Valid: true},
    })
    if err != nil {
        slog.Error("auto-resolve failed", "itemId", item.ID, "error", err)
        return
    }

    // H-6: logActivity is a package-level function, not a method.
    // Call it directly with the transactional qtx.
    if err := logActivity(ctx, qtx, ActivityParams{
        SquadID:    resolved.SquadID,
        ActorType:  domain.ActivityActorSystem,
        ActorID:    uuid.Nil,
        Action:     "inbox.auto_resolved",
        EntityType: "inbox_item",
        EntityID:   resolved.ID,
        Metadata: map[string]any{
            "resolution":    resolution,
            "response_note": responseNote,
        },
    }); err != nil {
        slog.Error("auto-resolve: activity log failed", "itemId", item.ID, "error", err)
        return
    }

    if err := tx.Commit(); err != nil {
        slog.Error("auto-resolve: commit failed", "itemId", item.ID, "error", err)
        return
    }

    // After commit: best-effort side effects (SSE, wakeup)

    // Wake the requesting agent (if applicable)
    if resolved.RequestedByAgentID.Valid {
        agent, err := c.queries.GetAgentByID(ctx, resolved.RequestedByAgentID.UUID)
        if err == nil && agent.Status != db.AgentStatusTerminated {
            _, _ = c.wakeupService.Enqueue(ctx,
                resolved.RequestedByAgentID.UUID,
                resolved.SquadID,
                "inbox_resolved",
                map[string]any{
                    "inbox_item_id": resolved.ID,
                    "resolution":    resolution,
                    "response_note": responseNote,
                    "auto_resolved": true,
                },
            )
        }
    }

    // Emit SSE event
    c.sseHub.Publish(resolved.SquadID, "inbox.item.resolved", map[string]any{
        "itemId":       resolved.ID,
        "resolution":   resolution,
        "resolvedAt":   resolved.ResolvedAt.Time,
        "autoResolved": true,
    })

    slog.Info("auto-resolved approval item",
        "itemId", resolved.ID,
        "resolution", resolution,
        "squadId", resolved.SquadID,
    )
}
```

### Server Startup Integration

### File: `internal/server/server.go` (modification)

```go
// In server initialization, after InboxService and WakeupService are created:
approvalChecker := handlers.NewApprovalTimeoutChecker(
    queries, dbConn, wakeupService, sseHub,
)
go approvalChecker.Start(ctx)
```

The checker goroutine is launched with the server's root context, so it shuts down when the server shuts down.

---

## Handler Layer

### File: `internal/server/handlers/agent_self_handler.go` (additions)

```go
// RegisterRoutes — add to existing route registration:
mux.HandleFunc("GET /api/agent/me/gates", h.GetGates)

// GetGates returns the approval gate configuration for the agent's squad.
func (h *AgentSelfHandler) GetGates(w http.ResponseWriter, r *http.Request) {
    identity, ok := auth.AgentFromContext(r.Context())
    if !ok {
        writeJSON(w, http.StatusUnauthorized, errorResponse{
            Error: "Agent authentication required",
            Code:  "UNAUTHENTICATED",
        })
        return
    }

    squad, err := h.queries.GetSquadByID(r.Context(), identity.SquadID)
    if err != nil {
        slog.Error("agent/me/gates: failed to get squad", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{
            Error: "Failed to load squad",
            Code:  "INTERNAL_ERROR",
        })
        return
    }

    var settings domain.SquadSettings
    if err := json.Unmarshal(squad.Settings, &settings); err != nil {
        slog.Error("agent/me/gates: failed to parse settings", "error", err)
        settings.ApprovalGates = nil
    }

    gates := settings.ApprovalGates
    if gates == nil {
        gates = []domain.ApprovalGate{}
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "gates": gates,
    })
}
```

### File: `internal/server/handlers/squad_handler.go` (modifications)

Extend the existing squad update handler to validate `approvalGates` in settings:

```go
// In the squad update handler, when processing settings changes:
func validateSquadSettings(settings domain.SquadSettings) error {
    if len(settings.ApprovalGates) > domain.MaxGatesPerSquad {
        return fmt.Errorf("maximum %d approval gates allowed per squad", domain.MaxGatesPerSquad)
    }
    seen := make(map[string]bool)
    for i := range settings.ApprovalGates {
        gate := &settings.ApprovalGates[i]
        domain.NormalizeApprovalGate(gate)
        if err := domain.ValidateApprovalGate(gate); err != nil {
            return fmt.Errorf("gate[%d] (%s): %w", i, gate.Name, err)
        }
        if seen[gate.ActionPattern] {
            return fmt.Errorf("duplicate actionPattern: %s", gate.ActionPattern)
        }
        seen[gate.ActionPattern] = true
    }
    return nil
}
```

---

## React Component Structure

### New Components

```
web/src/
  components/
    inbox/
      ApprovalActions.tsx      # Approve/Reject/Request Revision buttons
      ApprovalTimeoutBadge.tsx # Countdown timer showing time until auto-resolution
  pages/
    InboxPage.tsx              # Extended: approval filter + approval-specific actions
```

### ApprovalActions Component

Renders contextual action buttons for `category=approval` inbox items in the inbox detail view:

```tsx
// ApprovalActions.tsx
interface ApprovalActionsProps {
  itemId: string;
  onResolve: (resolution: "approved" | "rejected" | "request_revision", note?: string) => void;
  isLoading: boolean;
}

function ApprovalActions({ itemId, onResolve, isLoading }: ApprovalActionsProps) {
  const [note, setNote] = useState("");

  return (
    <div className="flex flex-col gap-3">
      <Textarea
        placeholder="Optional note for the agent..."
        value={note}
        onChange={(e) => setNote(e.target.value)}
      />
      <div className="flex gap-2">
        <Button
          variant="default"
          onClick={() => onResolve("approved", note)}
          disabled={isLoading}
        >
          Approve
        </Button>
        <Button
          variant="destructive"
          onClick={() => onResolve("rejected", note)}
          disabled={isLoading}
        >
          Reject
        </Button>
        <Button
          variant="outline"
          onClick={() => onResolve("request_revision", note)}
          disabled={isLoading}
        >
          Request Revision
        </Button>
      </div>
    </div>
  );
}
```

### ApprovalTimeoutBadge Component

Shows a countdown to the approval deadline:

```tsx
// ApprovalTimeoutBadge.tsx
interface ApprovalTimeoutBadgeProps {
  expiresAt: string;       // ISO 8601
  autoResolution: string;  // "rejected" | "approved"
}

function ApprovalTimeoutBadge({ expiresAt, autoResolution }: ApprovalTimeoutBadgeProps) {
  const [timeLeft, setTimeLeft] = useState("");

  useEffect(() => {
    const update = () => {
      const diff = new Date(expiresAt).getTime() - Date.now();
      if (diff <= 0) {
        setTimeLeft("Expired");
        return;
      }
      const hours = Math.floor(diff / 3600000);
      const minutes = Math.floor((diff % 3600000) / 60000);
      setTimeLeft(`${hours}h ${minutes}m remaining`);
    };
    update();
    const interval = setInterval(update, 60000);
    return () => clearInterval(interval);
  }, [expiresAt]);

  return (
    <Badge variant={timeLeft === "Expired" ? "destructive" : "secondary"}>
      {timeLeft} (auto-{autoResolution} on timeout)
    </Badge>
  );
}
```

### InboxItemDetail Integration

The existing `InboxItemDetail` / `InboxResolveForm` components (from Feature 12) already render category-specific actions. For `category=approval`, the form renders `ApprovalActions` and `ApprovalTimeoutBadge`. The approval payload fields (`gateName`, `actionPattern`, `actionDetails`) are displayed in an information panel above the actions.

### Inbox List Filtering

The existing `InboxFilters` component already supports `category` filtering. Users can filter by `category=approval` to see only approval requests. No new filter UI is needed.

---

## Integration with Agent Runtime

### System Prompt Injection

When the `RunService` prepares the system prompt for an agent run, it should include the squad's gate configuration so agents know which actions require approval. This is injected via the existing `ARI_SYSTEM_PROMPT` or environment variable mechanism.

**Important (M-4):** Gate info must be injected in BOTH the task wakeup path and the conversation prompt path in `buildInvokeInput`. Both paths should check for approval gates and append the gate list to the system prompt context.

```go
// In RunService, when building the agent's system prompt:
var settings domain.SquadSettings
_ = json.Unmarshal(squad.Settings, &settings)

if len(settings.ApprovalGates) > 0 {
    gateList := "Actions requiring approval:\n"
    for _, g := range settings.ApprovalGates {
        gateList += fmt.Sprintf("- %s (pattern: %s, timeout: %dh)\n",
            g.Name, g.ActionPattern, g.TimeoutHours)
    }
    gateList += "\nBefore performing any of these actions, create an approval request via POST /api/squads/{squadId}/inbox with category='approval'.\n"
    gateList += "Use GET /api/agent/me/gates for the full gate configuration.\n"
    systemPrompt += "\n" + gateList
}
```

---

## Error Handling

| Scenario | HTTP Code | Error Code |
|----------|-----------|------------|
| Invalid gate entry in settings update | 400 | `VALIDATION_ERROR` |
| Too many gates (> 50) | 400 | `VALIDATION_ERROR` |
| Duplicate actionPattern in gates | 400 | `VALIDATION_ERROR` |
| Agent calls /gates without Run Token | 401 | `UNAUTHENTICATED` |
| Gate not found for gateId in approval payload | — | Warning logged; defaults applied; item created normally |
| Auto-resolve CAS failure (already resolved) | — | No-op; logged at debug level |
| Background checker DB error | — | Logged; retry on next cycle |

---

## Testing Strategy

### Unit Tests

1. **`domain/approval_gate_test.go`**
   - `ValidateApprovalGate`: valid gate, missing name, missing actionPattern, timeoutHours out of range, invalid autoResolution
   - `NormalizeApprovalGate`: fills defaults for ID, autoResolution, requiredApprovers
   - `FindGateByID`: found, not found

2. **`domain/squad_settings_test.go`**
   - Unmarshal `SquadSettings` with and without `approvalGates`
   - Marshal and round-trip

### Integration Tests

3. **`handlers/agent_self_handler_test.go`** (additions)
   - `GET /api/agent/me/gates` with gates configured: returns gate array
   - `GET /api/agent/me/gates` with no gates: returns empty array
   - `GET /api/agent/me/gates` without Run Token: returns 401

4. **`handlers/inbox_service_test.go`** (additions)
   - Create approval item with valid gateId: payload enriched with expiresAt, gateName, actionPattern
   - Create approval item with unknown gateId: defaults applied, warning logged
   - Create approval item without gateId: defaults applied

5. **`handlers/approval_timeout_test.go`**
   - Expired item with autoResolution=rejected: resolved with rejected
   - Expired item with autoResolution=approved: resolved with approved
   - Item already resolved before checker runs: no-op (CAS)
   - Requesting agent terminated: resolved but no wakeup
   - Multiple expired items processed in batch
   - DB error during query: logged, no panic

6. **`handlers/squad_handler_test.go`** (additions)
   - Update settings with valid gates: persisted
   - Update settings with invalid gate (missing name): 400
   - Update settings with > 50 gates: 400
   - Update settings with duplicate actionPattern: 400

### End-to-End Test

7. **Full approval flow**
   - Configure gate on squad
   - Agent discovers gates via `GET /api/agent/me/gates`
   - Agent creates approval request
   - User resolves with `approved`
   - Verify agent wakeup with correct context

8. **Timeout flow**
   - Create approval item with short timeout
   - Verify background checker auto-resolves
   - Verify agent wakeup with auto-resolved context
   - Verify SSE event and activity log
