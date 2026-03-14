# Design: Activity Log

**Created:** 2026-03-15
**Status:** Ready for Implementation

---

## System Context

- **Depends On:** PostgreSQL, all CRUD handlers (squads, agents, issues, comments, projects, goals, members), authentication middleware (`internal/auth`)
- **Used By:** Dashboard activity feed (`GET /api/squads/{id}/activity`), future SSE push, audit/compliance
- **External Dependencies:** None — single INSERT per mutation, no external services

---

## Architecture Overview

Activity logging plugs directly into the existing handler → DB flow. Every mutation handler that already uses `h.dbConn.BeginTx` gets the activity INSERT added inside that same transaction via `qtx.InsertActivityEntry(...)`. Handlers that currently do not use a transaction (e.g., `ProjectHandler`, `MembershipHandler`) must be upgraded to open a transaction so the activity write and the mutation are atomic.

The central integration point is a thin helper function `logActivity` (defined in `internal/server/handlers/activity.go`) that accepts a `*db.Queries` scoped to the open transaction and the entry parameters. Every handler calls this helper before `tx.Commit()`.

```
Client Request
     │
     ▼
Handler (e.g., IssueHandler.CreateIssue)
     │
     ├─ Validate input
     ├─ h.dbConn.BeginTx(ctx, nil)  ──────────────────┐
     │  qtx := h.queries.WithTx(tx)                    │  same tx
     │                                                  │
     ├─ qtx.CreateIssue(...)   ◄── mutation query      │
     │                                                  │
     ├─ logActivity(ctx, qtx, ActivityParams{...})      │
     │    └─ qtx.InsertActivityEntry(...)               │
     │                                                  │
     ├─ tx.Commit()  ──────────────────────────────────┘
     │
     ▼
Response (201 / 200)
```

**Key decisions:**

1. Activity writes are **synchronous within the same transaction** — REQ-026, REQ-043, REQ-044.
2. `logActivity` returns an error; if it fails the handler returns 500 and the deferred `tx.Rollback()` fires — no silent swallowing (REQ-044).
3. Handlers that mutate without a transaction today (`ProjectHandler`, `MembershipHandler`, `GoalHandler`) gain `dbConn *sql.DB` and open a transaction for their mutating methods.
4. The `ActivityHandler` for the feed endpoint uses `h.queries` directly (no write, no transaction needed).

---

## Data Model

### Migration

File: `internal/database/migrations/20260315000011_create_activity_log.sql`

```sql
-- +goose Up

CREATE TYPE activity_actor_type AS ENUM ('agent', 'user', 'system');

CREATE TABLE activity_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id    UUID        NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    actor_type  activity_actor_type NOT NULL,
    actor_id    UUID        NOT NULL,
    action      TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   UUID        NOT NULL,
    metadata    JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Primary feed index: all queries are squad-scoped and ordered by recency (REQ-040)
CREATE INDEX idx_activity_log_squad_created_at
    ON activity_log (squad_id, created_at DESC);

-- Secondary indexes for optional filters (REQ-023, REQ-024, REQ-038)
CREATE INDEX idx_activity_log_squad_actor_type
    ON activity_log (squad_id, actor_type);

CREATE INDEX idx_activity_log_squad_entity_type
    ON activity_log (squad_id, entity_type);

-- +goose Down
DROP TABLE IF EXISTS activity_log;
DROP TYPE  IF EXISTS activity_actor_type;
```

Notes:
- No `updated_at` — entries are immutable; no UPDATE trigger needed (REQ-027).
- `metadata` defaults to `'{}'` not `NULL` so callers always receive a JSON object (REQ-029).
- `ON DELETE CASCADE` on `squad_id` keeps the table consistent when a squad is hard-deleted.

### Domain Model

File: `internal/domain/activity.go`

```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

// ActivityActorType identifies who triggered the mutation.
type ActivityActorType string

const (
    ActivityActorUser   ActivityActorType = "user"
    ActivityActorAgent  ActivityActorType = "agent"
    ActivityActorSystem ActivityActorType = "system"
)

func (a ActivityActorType) Valid() bool {
    switch a {
    case ActivityActorUser, ActivityActorAgent, ActivityActorSystem:
        return true
    }
    return false
}

// ValidActivityEntityTypes is the controlled list for the entityType field (REQ-036).
var ValidActivityEntityTypes = map[string]bool{
    "squad": true, "agent": true, "issue": true,
    "comment": true, "project": true, "goal": true, "member": true,
}

// ActivityEntry is the domain model returned from the feed endpoint.
type ActivityEntry struct {
    ID         uuid.UUID         `json:"id"`
    SquadID    uuid.UUID         `json:"squadId"`
    ActorType  ActivityActorType `json:"actorType"`
    ActorID    uuid.UUID         `json:"actorId"`
    Action     string            `json:"action"`
    EntityType string            `json:"entityType"`
    EntityID   uuid.UUID         `json:"entityId"`
    Metadata   any               `json:"metadata"` // always a JSON object, never null
    CreatedAt  string            `json:"createdAt"`
}
```

### Generated DB Model (after `make sqlc`)

sqlc will generate a struct like:

```go
// internal/database/db/models.go  (generated)
type ActivityLog struct {
    ID         uuid.UUID            `json:"id"`
    SquadID    uuid.UUID            `json:"squad_id"`
    ActorType  ActivityActorType    `json:"actor_type"`
    ActorID    uuid.UUID            `json:"actor_id"`
    Action     string               `json:"action"`
    EntityType string               `json:"entity_type"`
    EntityID   uuid.UUID            `json:"entity_id"`
    Metadata   pqtype.NullRawMessage `json:"metadata"`
    CreatedAt  time.Time            `json:"created_at"`
}
```

---

## sqlc Queries

File: `internal/database/queries/activity_log.sql`

```sql
-- name: InsertActivityEntry :one
INSERT INTO activity_log (
    squad_id, actor_type, actor_id, action, entity_type, entity_id, metadata
) VALUES (
    @squad_id,
    @actor_type,
    @actor_id,
    @action,
    @entity_type,
    @entity_id,
    @metadata
)
RETURNING *;

-- name: ListActivityBySquad :many
SELECT * FROM activity_log
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_actor_type')::activity_actor_type IS NULL
       OR actor_type = sqlc.narg('filter_actor_type'))
  AND (sqlc.narg('filter_entity_type')::TEXT IS NULL
       OR entity_type = sqlc.narg('filter_entity_type'))
  AND (sqlc.narg('filter_action')::TEXT IS NULL
       OR action = sqlc.narg('filter_action'))
ORDER BY created_at DESC
LIMIT  @page_limit
OFFSET @page_offset;

-- name: CountActivityBySquad :one
SELECT count(*) FROM activity_log
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_actor_type')::activity_actor_type IS NULL
       OR actor_type = sqlc.narg('filter_actor_type'))
  AND (sqlc.narg('filter_entity_type')::TEXT IS NULL
       OR entity_type = sqlc.narg('filter_entity_type'))
  AND (sqlc.narg('filter_action')::TEXT IS NULL
       OR action = sqlc.narg('filter_action'));
```

Run `make sqlc` after adding this file to regenerate `internal/database/db/activity_log.sql.go` and update `querier.go`.

---

## Activity Helper

File: `internal/server/handlers/activity.go`

```go
package handlers

import (
    "context"
    "encoding/json"

    "github.com/google/uuid"
    "github.com/sqlc-dev/pqtype"

    "github.com/xb/ari/internal/database/db"
    "github.com/xb/ari/internal/domain"
)

// ActivityParams carries the fields needed for a single activity entry.
// Metadata is any JSON-serialisable value; pass nil for an empty object.
type ActivityParams struct {
    SquadID    uuid.UUID
    ActorType  domain.ActivityActorType
    ActorID    uuid.UUID
    Action     string
    EntityType string
    EntityID   uuid.UUID
    Metadata   any // will be marshalled to JSONB; nil → '{}'
}

// logActivity inserts a single activity entry using qtx (a transaction-scoped Queries).
// It MUST be called before tx.Commit(). Any error must be handled by the caller —
// the deferred tx.Rollback() will undo the enclosing mutation (REQ-043, REQ-044).
func logActivity(ctx context.Context, qtx *db.Queries, p ActivityParams) error {
    raw := json.RawMessage(`{}`)
    if p.Metadata != nil {
        b, err := json.Marshal(p.Metadata)
        if err != nil {
            return err
        }
        raw = b
    }

    _, err := qtx.InsertActivityEntry(ctx, db.InsertActivityEntryParams{
        SquadID:    p.SquadID,
        ActorType:  db.ActivityActorType(p.ActorType),
        ActorID:    p.ActorID,
        Action:     p.Action,
        EntityType: p.EntityType,
        EntityID:   p.EntityID,
        Metadata:   pqtype.NullRawMessage{RawMessage: raw, Valid: true},
    })
    return err
}
```

### Why a helper instead of middleware

Middleware would need to inspect the response body after the fact to know which entity was mutated and what the new state is. The required metadata (e.g., `{"from": "todo", "to": "in_progress"}`) is only available mid-handler. A helper called at the exact point the metadata is known is simpler, more explicit, and easier to test.

---

## Handler Integration

### Handlers that already use transactions

These handlers already call `h.dbConn.BeginTx` and use `qtx := h.queries.WithTx(tx)`. They need only a call to `logActivity` before `tx.Commit()`.

#### `IssueHandler.CreateIssue` (REQ-008)

After `qtx.CreateIssue(...)` succeeds, before `tx.Commit()`:

```go
if err := logActivity(r.Context(), qtx, ActivityParams{
    SquadID:    squadID,
    ActorType:  domain.ActivityActorUser,
    ActorID:    userID,   // from h.verifySquadMembership return value
    Action:     "issue.created",
    EntityType: "issue",
    EntityID:   issue.ID,
    Metadata: map[string]any{
        "identifier": issue.Identifier,
        "title":      issue.Title,
        "status":     string(issue.Status),
    },
}); err != nil {
    slog.Error("failed to log activity", "error", err)
    writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
    return
}
```

Note: `verifySquadMembership` currently returns only `(uuid.UUID, bool)` — the returned UUID is the `userID`. The existing call already captures this; update call sites to name the variable `userID`.

#### `IssueHandler.UpdateIssue` — status changed path (REQ-009, REQ-010)

In the `isReopen` branch, after `qtx.UpdateIssue` and `qtx.CreateIssueComment`, before `tx.Commit()`:

```go
action := "issue.updated"
var meta map[string]any
if req.Status != nil && string(*req.Status) != string(existing.Status) {
    action = "issue.status_changed"
    meta = map[string]any{
        "from":       string(existing.Status),
        "to":         string(*req.Status),
        "identifier": existing.Identifier,
    }
} else {
    meta = map[string]any{"changedFields": changedFieldNames(rawBody)}
}
if err := logActivity(r.Context(), qtx, ActivityParams{...}); err != nil { ... }
```

For the non-reopen `UpdateIssue` path (which currently does not use a transaction), wrap the `UpdateIssue` query in a transaction so the activity write can be included.

#### `IssueHandler.DeleteIssue` (REQ-011)

`DeleteIssue` currently calls `h.queries.DeleteIssue` directly without a transaction. Upgrade to use `BeginTx` → `qtx.DeleteIssue` → `logActivity` → `tx.Commit()`.

#### `IssueHandler.CreateComment` (REQ-012)

Upgrade `CreateComment` to use a transaction. After `qtx.CreateIssueComment`:

```go
logActivity(r.Context(), qtx, ActivityParams{
    SquadID:    issue.SquadID,
    ActorType:  domain.ActivityActorType(req.AuthorType),
    ActorID:    req.AuthorID,
    Action:     "comment.created",
    EntityType: "comment",
    EntityID:   comment.ID,
    Metadata: map[string]any{
        "issueId":    issue.ID.String(),
        "authorType": string(req.AuthorType),
    },
})
```

#### `SquadHandler.Create` (REQ-001)

Already transactional (creates squad + membership). After `qtx.CreateSquadMembership`, before commit:

```go
logActivity(r.Context(), qtx, ActivityParams{
    SquadID:    squad.ID,
    ActorType:  domain.ActivityActorUser,
    ActorID:    identity.UserID,
    Action:     "squad.created",
    EntityType: "squad",
    EntityID:   squad.ID,
    Metadata:   nil,
})
```

### Handlers that need transaction upgrade

The following handlers currently call `h.queries.<Method>` directly without a transaction. Each must be upgraded:

1. **`SquadHandler.Update`** — needs `dbConn *sql.DB`; wrap `UpdateSquad` in a transaction. (REQ-002)
2. **`SquadHandler.Delete`** — wrap `SoftDeleteSquad` in a transaction. (REQ-003)
3. **`SquadHandler.UpdateBudget`** — wrap `UpdateSquad` in a transaction. (REQ-004)
4. **`AgentHandler.CreateAgent`** — already uses `dbConn`; check if transactional; if not, wrap. (REQ-005)
5. **`AgentHandler.UpdateAgent`** — wrap `UpdateAgent`. (REQ-006)
6. **`AgentHandler.TransitionAgentStatus`** — wrap `UpdateAgent`. (REQ-007)
7. **`ProjectHandler.CreateProject`** — add `dbConn *sql.DB` to `ProjectHandler`; wrap `CreateProject`. (REQ-013)
8. **`ProjectHandler.UpdateProject`** — wrap `UpdateProject`. (REQ-014)
9. **`GoalHandler.CreateGoal`** — add `dbConn *sql.DB` to `GoalHandler`; wrap `CreateGoal`. (REQ-015)
10. **`GoalHandler.UpdateGoal`** — wrap `UpdateGoal`. (REQ-016)
11. **`MembershipHandler.Add`** — add `dbConn *sql.DB` to `MembershipHandler`; wrap `CreateSquadMembership`. (REQ-017)
12. **`MembershipHandler.UpdateRole`** — wrap `UpdateSquadMembershipRole`. (REQ-018)
13. **`MembershipHandler.Remove`** — wrap `DeleteSquadMembership`. (REQ-019)
14. **`MembershipHandler.Leave`** — wrap `DeleteSquadMembershipByUserIfNotLastOwner`. (REQ-020)

Pattern for upgrading a handler that previously used `h.queries` directly:

```go
// Before
result, err := h.queries.CreateProject(r.Context(), params)

// After
tx, err := h.dbConn.BeginTx(r.Context(), nil)
if err != nil {
    slog.Error("failed to begin transaction", "error", err)
    writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
    return
}
defer tx.Rollback() //nolint:errcheck

qtx := h.queries.WithTx(tx)
result, err := qtx.CreateProject(r.Context(), params)
if err != nil { /* handle */ }

if err := logActivity(r.Context(), qtx, ActivityParams{...}); err != nil {
    slog.Error("failed to log activity", "error", err)
    writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
    return
}

if err := tx.Commit(); err != nil { /* handle */ }
```

### Metadata values per action

| Action | Metadata keys |
|---|---|
| `squad.created` | `{}` |
| `squad.updated` | `{"changedFields": ["name", "description"]}` |
| `squad.deleted` | `{}` |
| `squad.budget_updated` | `{"budgetMonthlyCents": 50000}` |
| `agent.created` | `{"role": "member", "name": "Aria"}` |
| `agent.updated` | `{"changedFields": ["model", "systemPrompt"]}` |
| `agent.status_changed` | `{"from": "active", "to": "paused"}` |
| `issue.created` | `{"identifier": "ARI-7", "title": "Fix login", "status": "backlog"}` |
| `issue.updated` | `{"changedFields": ["priority", "assigneeAgentId"]}` |
| `issue.status_changed` | `{"from": "todo", "to": "in_progress", "identifier": "ARI-7"}` |
| `issue.deleted` | `{"identifier": "ARI-7"}` |
| `comment.created` | `{"issueId": "<uuid>", "authorType": "user"}` |
| `project.created` | `{"name": "Q2 Roadmap"}` |
| `project.updated` | `{"changedFields": ["description"]}` |
| `project.status_changed` | `{"from": "active", "to": "completed"}` |
| `goal.created` | `{"title": "Hit 100 customers"}` |
| `goal.updated` | `{"changedFields": ["description"]}` |
| `goal.status_changed` | `{"from": "active", "to": "completed"}` |
| `member.added` | `{"userId": "<uuid>", "role": "member"}` |
| `member.role_changed` | `{"userId": "<uuid>", "from": "member", "to": "admin"}` |
| `member.removed` | `{"userId": "<uuid>"}` |
| `member.left` | `{"userId": "<uuid>"}` |

Helper for collecting changed field names from the raw JSON body (used by `squad.updated`, `agent.updated`, etc.):

```go
// changedFieldNames returns a slice of JSON key names present in rawBody,
// excluding sentinel-only keys that don't represent user intent changes.
func changedFieldNames(rawBody map[string]json.RawMessage, exclude ...string) []string {
    skip := make(map[string]bool, len(exclude))
    for _, k := range exclude {
        skip[k] = true
    }
    names := make([]string, 0, len(rawBody))
    for k := range rawBody {
        if !skip[k] {
            names = append(names, k)
        }
    }
    sort.Strings(names)
    return names
}
```

---

## New Handler: Activity Feed

File: `internal/server/handlers/activity_handler.go`

```go
package handlers

import (
    "database/sql"
    "encoding/json"
    "errors"
    "log/slog"
    "net/http"
    "strconv"

    "github.com/google/uuid"
    "github.com/sqlc-dev/pqtype"

    "github.com/xb/ari/internal/auth"
    "github.com/xb/ari/internal/database/db"
    "github.com/xb/ari/internal/domain"
)

// ActivityHandler serves the activity feed for a squad.
type ActivityHandler struct {
    queries *db.Queries
}

// NewActivityHandler creates an ActivityHandler.
func NewActivityHandler(q *db.Queries) *ActivityHandler {
    return &ActivityHandler{queries: q}
}

// RegisterRoutes registers activity feed routes.
func (h *ActivityHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/squads/{id}/activity", h.ListActivity)
}
```

### `ListActivity` implementation sketch

```go
func (h *ActivityHandler) ListActivity(w http.ResponseWriter, r *http.Request) {
    // 1. Parse and validate squad ID
    idStr := r.PathValue("id")
    squadID, err := uuid.Parse(idStr)
    if err != nil {
        writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
        return
    }

    // 2. Auth + squad membership (REQ-032, REQ-033)
    identity, ok := auth.UserFromContext(r.Context())
    if !ok {
        writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
        return
    }
    _, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
        UserID: identity.UserID, SquadID: squadID,
    })
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
            return
        }
        slog.Error("failed to check squad membership", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    // 3. Parse query params with defaults (REQ-022)
    q := r.URL.Query()
    limit, offset := 50, 0
    if v := q.Get("limit"); v != "" {
        if l, e := strconv.Atoi(v); e == nil && l >= 1 {
            if l > 200 { l = 200 }
            limit = l
        }
    }
    if v := q.Get("offset"); v != "" {
        if o, e := strconv.Atoi(v); e == nil && o >= 0 {
            offset = o
        }
    }

    // 4. Validate optional enum filters (REQ-035, REQ-036)
    listParams := db.ListActivityBySquadParams{
        SquadID:    squadID,
        PageLimit:  int32(limit),
        PageOffset: int32(offset),
    }
    countParams := db.CountActivityBySquadParams{SquadID: squadID}

    if v := q.Get("actorType"); v != "" {
        at := domain.ActivityActorType(v)
        if !at.Valid() {
            writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid actorType", Code: "VALIDATION_ERROR"})
            return
        }
        listParams.FilterActorType  = db.NullActivityActorType{ActivityActorType: db.ActivityActorType(at), Valid: true}
        countParams.FilterActorType = listParams.FilterActorType
    }
    if v := q.Get("entityType"); v != "" {
        if !domain.ValidActivityEntityTypes[v] {
            writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid entityType", Code: "VALIDATION_ERROR"})
            return
        }
        listParams.FilterEntityType  = sql.NullString{String: v, Valid: true}
        countParams.FilterEntityType = listParams.FilterEntityType
    }
    if v := q.Get("action"); v != "" {   // REQ-038
        listParams.FilterAction  = sql.NullString{String: v, Valid: true}
        countParams.FilterAction = listParams.FilterAction
    }

    // 5. Query
    rows, err := h.queries.ListActivityBySquad(r.Context(), listParams)
    if err != nil {
        slog.Error("failed to list activity", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }
    total, err := h.queries.CountActivityBySquad(r.Context(), countParams)
    if err != nil {
        slog.Error("failed to count activity", "error", err)
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
        return
    }

    // 6. Map to response
    data := make([]activityEntryResponse, 0, len(rows))
    for _, row := range rows {
        data = append(data, dbActivityToResponse(row))
    }
    writeJSON(w, http.StatusOK, activityListResponse{
        Data: data,
        Pagination: paginationMeta{Limit: limit, Offset: offset, Total: total},
    })
}
```

---

## API Contracts

### `GET /api/squads/{id}/activity`

**Auth:** Session cookie required (REQ-032). Caller must be a member of the squad (REQ-033).

**Query Parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | int | 50 | Page size; max 200 |
| `offset` | int | 0 | Page offset |
| `actorType` | string | — | Filter: `agent`, `user`, `system` (REQ-023, REQ-035) |
| `entityType` | string | — | Filter: `squad`, `agent`, `issue`, `comment`, `project`, `goal`, `member` (REQ-024, REQ-036) |
| `action` | string | — | Exact match on action string, e.g. `issue.status_changed` (REQ-038) |

**Success Response — 200 OK:**

```json
{
  "data": [
    {
      "id": "018f2d3a-...",
      "squadId": "018f2d3b-...",
      "actorType": "user",
      "actorId": "018f2d3c-...",
      "action": "issue.status_changed",
      "entityType": "issue",
      "entityId": "018f2d3d-...",
      "metadata": { "from": "todo", "to": "in_progress", "identifier": "ARI-7" },
      "createdAt": "2026-03-15T10:00:00Z"
    }
  ],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total": 142
  }
}
```

The `pagination` envelope always appears, matching `GET /api/squads/{squadId}/issues` (REQ-034). `metadata` is always a JSON object, never `null` (REQ-029).

**Response types (Go):**

```go
type activityEntryResponse struct {
    ID         uuid.UUID `json:"id"`
    SquadID    uuid.UUID `json:"squadId"`
    ActorType  string    `json:"actorType"`
    ActorID    uuid.UUID `json:"actorId"`
    Action     string    `json:"action"`
    EntityType string    `json:"entityType"`
    EntityID   uuid.UUID `json:"entityId"`
    Metadata   any       `json:"metadata"`
    CreatedAt  string    `json:"createdAt"`
}

type activityListResponse struct {
    Data       []activityEntryResponse `json:"data"`
    Pagination paginationMeta          `json:"pagination"`
}

func dbActivityToResponse(a db.ActivityLog) activityEntryResponse {
    var meta any = map[string]any{}
    if a.Metadata.Valid {
        json.Unmarshal(a.Metadata.RawMessage, &meta)
    }
    return activityEntryResponse{
        ID:         a.ID,
        SquadID:    a.SquadID,
        ActorType:  string(a.ActorType),
        ActorID:    a.ActorID,
        Action:     a.Action,
        EntityType: a.EntityType,
        EntityID:   a.EntityID,
        Metadata:   meta,
        CreatedAt:  a.CreatedAt.Format(time.RFC3339),
    }
}
```

**Error Responses:**

| Status | Code | Condition |
|---|---|---|
| 401 | `UNAUTHENTICATED` | No valid session |
| 404 | `SQUAD_NOT_FOUND` | Squad not found or caller not a member |
| 400 | `VALIDATION_ERROR` | Invalid `actorType` or `entityType` filter value |
| 500 | `INTERNAL_ERROR` | Database error |

---

## Route Registration

In `internal/server/router.go` (or wherever handlers are wired up), add:

```go
activityHandler := handlers.NewActivityHandler(queries)
activityHandler.RegisterRoutes(mux)
```

`MembershipHandler`, `ProjectHandler`, and `GoalHandler` constructors need to accept `*sql.DB` as an additional parameter so they can open transactions:

```go
// Before
handlers.NewProjectHandler(queries)

// After
handlers.NewProjectHandler(queries, dbConn)
```

---

## Error Handling

### Transaction rollback on activity failure (REQ-037, REQ-044)

Every handler that calls `logActivity` must check the returned error and return `500 INTERNAL_ERROR` before calling `tx.Commit()`. The deferred `tx.Rollback()` fires automatically:

```go
defer tx.Rollback() //nolint:errcheck  ← rolls back if Commit is never called

// ... mutation query ...

if err := logActivity(ctx, qtx, params); err != nil {
    slog.Error("failed to log activity", "error", err)
    writeJSON(w, http.StatusInternalServerError, errorResponse{
        Error: "Internal server error", Code: "INTERNAL_ERROR",
    })
    return   // ← Commit never called → deferred Rollback fires
}

if err := tx.Commit(); err != nil { ... }
```

### Filter validation (REQ-035, REQ-036)

`actorType` is validated against `domain.ActivityActorType.Valid()`. `entityType` is validated against `domain.ValidActivityEntityTypes`. Both checks happen before any DB query. Invalid values return `400 VALIDATION_ERROR`.

### Metadata safety (REQ-042)

`changedFieldNames` only records key names, never values. Metadata for status-change events records only the status string (an enum value), never a field that could contain credentials. The `agent.updated` metadata records field names only (not the new `adapterConfig` or `systemPrompt` content).

---

## Data Flow Diagram

```
Client
  │  POST /api/squads/{squadId}/issues
  ▼
IssueHandler.CreateIssue
  │
  ├── validate input
  ├── verify squad membership  ──► returns userID
  │
  ├── tx, _ = h.dbConn.BeginTx(ctx, nil)
  │   defer tx.Rollback()
  │   qtx = h.queries.WithTx(tx)
  │
  ├── qtx.IncrementSquadIssueCounter(ctx, squadID)
  │                                   │
  │                              [DB: UPDATE squads SET issue_counter+1]
  │
  ├── qtx.CreateIssue(ctx, params)
  │                    │
  │               [DB: INSERT INTO issues ...]  ──► issue (with ID)
  │
  ├── logActivity(ctx, qtx, ActivityParams{
  │       SquadID:    squadID,
  │       ActorType:  "user",
  │       ActorID:    userID,
  │       Action:     "issue.created",
  │       EntityType: "issue",
  │       EntityID:   issue.ID,
  │       Metadata:   {"identifier":..., "title":..., "status":...},
  │   })
  │       │
  │       └── qtx.InsertActivityEntry(ctx, params)
  │                                    │
  │                              [DB: INSERT INTO activity_log ...]
  │
  ├── tx.Commit()  ◄── both INSERTs committed atomically
  │
  └── writeJSON(w, 201, issueResponse{...})
```

If `logActivity` returns an error:
```
  ├── logActivity returns err
  ├── writeJSON(w, 500, ...)
  ├── return
  └── deferred tx.Rollback() fires  ◄── IncrementCounter + CreateIssue both rolled back
```

---

## Testing Strategy

### Unit tests — `internal/server/handlers/activity_test.go`

Test the `logActivity` helper in isolation using a test database or table-driven mocks:

```go
// TestLogActivity_InsertEntry: verify InsertActivityEntry is called with correct params
// TestLogActivity_MarshalMetadata: verify metadata is marshalled to JSON correctly
// TestLogActivity_NilMetadata: verify nil metadata produces '{}'
// TestChangedFieldNames: verify helper extracts key names from raw JSON map
```

### Integration tests — `internal/server/handlers/activity_integration_test.go`

Each test uses the `testEnv` helper pattern from existing integration tests (in-process server + real embedded Postgres):

```go
// TestActivityLog_IssueCreated
//   - Create issue via POST /api/squads/{id}/issues
//   - GET /api/squads/{id}/activity
//   - Assert entry with action="issue.created", entityType="issue", entityId=<issue.ID>

// TestActivityLog_IssueStatusChanged
//   - Create issue, then PATCH to change status
//   - Assert two entries: issue.created and issue.status_changed with from/to metadata

// TestActivityLog_FilterByActorType
//   - Create issue (user actor), GET with ?actorType=user → returns entry
//   - GET with ?actorType=agent → returns empty

// TestActivityLog_FilterByEntityType
//   - Create issue and a project
//   - GET with ?entityType=issue → only issue entry returned

// TestActivityLog_FilterByAction
//   - Create issue and update it
//   - GET with ?action=issue.created → only created entry

// TestActivityLog_Pagination
//   - Create 5 issues
//   - GET with ?limit=2&offset=0 → 2 entries, total=5 (plus squad.created = 6 total)

// TestActivityLog_RollbackOnActivityFailure
//   - Inject a failing qtx (mock or DB constraint violation)
//   - Assert the mutation is also rolled back (entity not present after error)

// TestActivityLog_InvalidActorTypeFilter
//   - GET /api/squads/{id}/activity?actorType=robot → 400 VALIDATION_ERROR

// TestActivityLog_InvalidEntityTypeFilter
//   - GET /api/squads/{id}/activity?entityType=widget → 400 VALIDATION_ERROR

// TestActivityLog_Unauthorized
//   - GET without session cookie → 401 UNAUTHENTICATED

// TestActivityLog_NonMember
//   - GET as user who is not a member of the squad → 404 SQUAD_NOT_FOUND
```

### Transactional write correctness

The rollback test is the most critical: it verifies REQ-043 and REQ-044. The approach is to cause the activity INSERT to fail (e.g., by passing an invalid actor_type value that passes Go validation but fails the DB enum constraint) and then assert the entity that triggered the mutation is also absent from the DB.

### Test helper additions

Add `getActivity` and `assertActivityEntry` helpers to the test package following the pattern of `createIssue` / `issueResp`:

```go
type activityEnvelope struct {
    Data       []activityEntryResp `json:"data"`
    Pagination paginationResp      `json:"pagination"`
}

type activityEntryResp struct {
    ID         string         `json:"id"`
    SquadID    string         `json:"squadId"`
    ActorType  string         `json:"actorType"`
    ActorID    string         `json:"actorId"`
    Action     string         `json:"action"`
    EntityType string         `json:"entityType"`
    EntityID   string         `json:"entityId"`
    Metadata   map[string]any `json:"metadata"`
    CreatedAt  string         `json:"createdAt"`
}

func getActivity(t *testing.T, env *testEnv, cookie *http.Cookie, squadID string, query string) (*activityEnvelope, int) {
    t.Helper()
    path := "/api/squads/" + squadID + "/activity"
    if query != "" {
        path += "?" + query
    }
    rr := doRequest(t, env.handler, "GET", path, nil, []*http.Cookie{cookie})
    if rr.Code != http.StatusOK {
        return nil, rr.Code
    }
    var env activityEnvelope
    json.NewDecoder(rr.Body).Decode(&env)
    return &env, rr.Code
}
```

---

## Implementation Checklist

- [ ] Add migration `20260315000011_create_activity_log.sql`
- [ ] Add sqlc queries `internal/database/queries/activity_log.sql`
- [ ] Run `make sqlc` to regenerate `internal/database/db/`
- [ ] Add domain types `internal/domain/activity.go`
- [ ] Add `logActivity` helper `internal/server/handlers/activity.go`
- [ ] Add `ActivityHandler` `internal/server/handlers/activity_handler.go`
- [ ] Register `ActivityHandler` routes in router
- [ ] Upgrade `SquadHandler` (Update, Delete, UpdateBudget) — add activity calls
- [ ] Upgrade `AgentHandler` (CreateAgent, UpdateAgent, TransitionAgentStatus)
- [ ] Upgrade `IssueHandler` (CreateIssue, UpdateIssue, DeleteIssue, CreateComment) — all paths including non-reopen UpdateIssue
- [ ] Upgrade `ProjectHandler` — add `dbConn`, wrap mutations in transactions
- [ ] Upgrade `GoalHandler` — add `dbConn`, wrap mutations in transactions
- [ ] Upgrade `MembershipHandler` — add `dbConn`, wrap mutations in transactions
- [ ] Write unit tests for `logActivity` and `changedFieldNames`
- [ ] Write integration tests covering all action types, filters, and rollback behaviour
- [ ] Run `make test` — all tests pass

---

## Open Questions (Resolved)

- **Should activity writes be async?** No — synchronous within the same transaction (REQ-026, REQ-043).
- **Before/after field snapshots?** No — metadata carries key context only, not full diff (out of scope per requirements).
- **Retention policy?** Out of scope for v0.1; tracked as a future risk.

---

## References

- Requirements: `docx/features/09-activity-log/requirements.md`
- PRD: `docx/core/01-PRODUCT.md` — sections 3.3, 5.2.6, 6.3, 9.2
- Existing handler patterns: `internal/server/handlers/issue_handler.go`, `internal/server/handlers/squad_handler.go`
- sqlc config: `sqlc.yaml`
- Migration naming: `internal/database/migrations/` (format: `YYYYMMDDNNNNNN_<name>.sql`)
