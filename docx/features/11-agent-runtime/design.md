# Design: Agent Runtime

**Created:** 2026-03-15
**Status:** Ready for Implementation

---

## Architecture Overview

Ari is a **control plane**, not an execution engine. It orchestrates when agents run and what context they receive; agents themselves execute externally (subprocesses, HTTP endpoints, containers) and call back into the Ari API to report progress using a short-lived Run Token JWT.

### High-Level Component Relationships

```
HTTP Request / Timer
        |
        v
  WakeupService          ← enqueues WakeupRequest to DB
        |
        v
  WakeupProcessor        ← background goroutine per squad
  (concurrency guard)
        |
        v
  AdapterRegistry        ← resolves adapterType → Adapter
        |
        v
  Adapter.Execute()      ← blocks until run completes/cancelled
  (process, http, etc.)
        |
        v
  RunFinisher            ← writes HeartbeatRun, CostEvent,
                            session state, transitions agent
        |
        v
  SSE Hub (per squad)    ← broadcasts events to all subscribers
```

### Squad Isolation

Every runtime object — `WakeupRequest`, `HeartbeatRun`, `AgentTaskSession`, `AgentConversationSession`, SSE subscriber channel — is keyed by `squad_id`. The SSE hub holds a map of `squadID → set of subscriber channels`; events are only written to the matching squad's set.

---

## Adapter Interface (Expanded)

Replace `internal/adapter/adapter.go` entirely with the following contract. All agent execution in the system goes through this interface (REQ-035).

```go
// Package adapter defines the agent execution interface and supporting types.
package adapter

import (
    "context"
    "time"

    "github.com/google/uuid"
)

// TestLevel controls how thorough the environment check is.
type TestLevel int

const (
    TestLevelBasic   TestLevel = iota // fast: check binary/URL exists
    TestLevelFull                     // slow: attempt a no-op invocation
)

// TestResult reports the outcome of TestEnvironment.
type TestResult struct {
    Available bool
    Message   string
}

// RunStatus mirrors domain.RunStatus — kept here to avoid import cycles.
type RunStatus string

const (
    RunStatusSucceeded RunStatus = "succeeded"
    RunStatusFailed    RunStatus = "failed"
    RunStatusCancelled RunStatus = "cancelled"
    RunStatusTimedOut  RunStatus = "timed_out"
)

// ModelDefinition describes one AI model the adapter can use.
type ModelDefinition struct {
    ID       string // e.g., "claude-3-5-sonnet-20241022"
    Provider string // e.g., "anthropic"
    MaxTokens int
}

// AgentContext carries the agent's identity and configuration.
type AgentContext struct {
    ID           uuid.UUID
    SquadID      uuid.UUID
    Name         string
    Role         string // "captain" | "lead" | "member"
    AdapterType  string
    AdapterConfig []byte // raw JSON from agents.adapter_config
    SystemPrompt string
    Model        string
}

// SquadContext carries squad-level metadata.
type SquadContext struct {
    ID                 uuid.UUID
    Name               string
    BudgetMonthlyCents *int64
    SpentThisMonthCents int64
}

// RunContext carries per-invocation context.
type RunContext struct {
    RunID        uuid.UUID
    WakeReason   string // "on_demand" | "timer" | "assignment" | "inbox_resolved" | "conversation_message"
    TaskID       *uuid.UUID // set when WakeReason == "assignment"
    SessionState string     // sessionIdBefore — opaque blob from previous run
}

// CommentEntry is a single message in a conversation thread.
type CommentEntry struct {
    ID         uuid.UUID
    AuthorType string // "agent" | "user" | "system"
    AuthorID   uuid.UUID
    Body       string
    CreatedAt  time.Time
}

// ConversationContext is non-nil when WakeReason == "conversation_message".
type ConversationContext struct {
    IssueID      uuid.UUID
    Messages     []CommentEntry
    SessionState string // previous conversation session state
}

// InvokeInput is the full context passed to Execute().
type InvokeInput struct {
    Agent        AgentContext
    Squad        SquadContext
    Run          RunContext
    EnvVars      map[string]string      // ARI_* vars to inject
    Prompt       string                 // system-generated initial prompt
    Conversation *ConversationContext   // non-nil for conversation_message wakeups
}

// TokenUsage captures LLM usage for cost accounting.
type TokenUsage struct {
    InputTokens  int
    OutputTokens int
    Model        string
    Provider     string
}

// InvokeResult is returned by Execute() when the run ends.
type InvokeResult struct {
    Status       RunStatus
    ExitCode     int
    Usage        TokenUsage
    SessionState string // sessionIdAfter — opaque state blob, empty if not applicable
    Stdout       string // excerpt (up to MaxExcerptBytes)
    Stderr       string // excerpt (up to MaxExcerptBytes)
}

// LogLine represents a single structured log line from an adapter.
type LogLine struct {
    Level     string    // "debug" | "info" | "warn" | "error"
    Message   string
    Timestamp time.Time
    Fields    map[string]any
}

// Hooks are callbacks called by the adapter during execution.
// All callbacks must be safe to call from a background goroutine.
type Hooks struct {
    // OnLogLine forwards a real-time log line for SSE streaming.
    // MUST NOT block; implementations may drop lines if the channel is full.
    OnLogLine func(line LogLine)

    // OnStatusChange signals a sub-status change (e.g., "awaiting_tool_result").
    OnStatusChange func(detail string)
}

// Adapter is the interface all agent runtime adapters must implement.
type Adapter interface {
    // Type returns the unique identifier for this adapter (matches adapter_type column).
    Type() string

    // Execute spawns/invokes the agent and blocks until the run completes
    // or the context is cancelled. The context is cancelled on graceful stop.
    // Implementations must handle context cancellation and return
    // InvokeResult{Status: RunStatusCancelled} in that case.
    Execute(ctx context.Context, input InvokeInput, hooks Hooks) (InvokeResult, error)

    // TestEnvironment checks runtime prerequisites.
    // Called at startup; failures mark the adapter unavailable (REQ-049).
    TestEnvironment(level TestLevel) (TestResult, error)

    // Models returns the AI models this adapter supports.
    Models() []ModelDefinition
}
```

### Adapter Registry

```go
// Package adapter (continued)

// Registry maps adapterType strings to Adapter implementations.
// Safe for concurrent reads after initialization.
type Registry struct {
    mu       sync.RWMutex
    adapters map[string]Adapter
    unavail  map[string]string // adapterType → reason (from failed TestEnvironment)
}

func NewRegistry() *Registry {
    return &Registry{
        adapters: make(map[string]Adapter),
        unavail:  make(map[string]string),
    }
}

// Register adds an adapter. Called once at startup, not concurrently.
func (r *Registry) Register(a Adapter) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.adapters[a.Type()] = a
}

// Resolve returns the adapter for the given type, or an error if not found
// or marked unavailable. Callers get ADAPTER_NOT_FOUND (REQ-042) on error.
func (r *Registry) Resolve(adapterType string) (Adapter, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    if reason, bad := r.unavail[adapterType]; bad {
        return nil, fmt.Errorf("adapter %q is unavailable: %s", adapterType, reason)
    }
    a, ok := r.adapters[adapterType]
    if !ok {
        return nil, fmt.Errorf("no adapter registered for type %q", adapterType)
    }
    return a, nil
}

// MarkUnavailable records a startup failure for an adapter (REQ-049).
func (r *Registry) MarkUnavailable(adapterType, reason string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.unavail[adapterType] = reason
}
```

Startup sequence in `runServer()`:

```go
registry := adapter.NewRegistry()
processAdapter := process.New(cfg)
registry.Register(processAdapter)

// Run environment checks; mark unavailable but don't abort startup (REQ-049)
for _, a := range registeredAdapters {
    result, err := a.TestEnvironment(adapter.TestLevelBasic)
    if err != nil || !result.Available {
        reason := result.Message
        if err != nil { reason = err.Error() }
        slog.Warn("adapter environment check failed", "adapter", a.Type(), "reason", reason)
        registry.MarkUnavailable(a.Type(), reason)
    }
}
```

---

## Process Adapter

**Package:** `internal/adapter/process/`
**adapterType:** `"process"`

The process adapter spawns a shell command as a subprocess, injects all `ARI_*` environment variables, captures stdout/stderr, and enforces the agent's run timeout.

### AdapterConfig Schema (JSON)

```json
{
  "command": "/usr/local/bin/my-agent",
  "args": ["--mode", "task"],
  "workingDir": "/opt/agents/my-agent",
  "timeoutSeconds": 3600,
  "maxExcerptBytes": 65536
}
```

All fields are optional. `command` defaults to the agent's `short_name`. `timeoutSeconds` defaults to 3600 (1 hour). `maxExcerptBytes` defaults to 65536 (64 KB per REQ-037).

### Implementation Sketch

```go
// internal/adapter/process/process.go
package process

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "os"
    "os/exec"
    "syscall"
    "time"

    "github.com/xb/ari/internal/adapter"
)

const DefaultTimeoutSeconds = 3600
const DefaultMaxExcerptBytes = 65536

type Config struct {
    Command         string
    Args            []string
    WorkingDir      string
    TimeoutSeconds  int
    MaxExcerptBytes int
}

type ProcessAdapter struct{}

func New() *ProcessAdapter { return &ProcessAdapter{} }

func (p *ProcessAdapter) Type() string { return "process" }

func (p *ProcessAdapter) Execute(ctx context.Context, input adapter.InvokeInput, hooks adapter.Hooks) (adapter.InvokeResult, error) {
    var cfg Config
    if err := json.Unmarshal(input.Agent.AdapterConfig, &cfg); err != nil || cfg.Command == "" {
        cfg.Command = input.Agent.Name
    }
    if cfg.TimeoutSeconds == 0 {
        cfg.TimeoutSeconds = DefaultTimeoutSeconds
    }
    if cfg.MaxExcerptBytes == 0 {
        cfg.MaxExcerptBytes = DefaultMaxExcerptBytes
    }

    // Enforce run timeout
    runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
    defer cancel()

    cmd := exec.CommandContext(runCtx, cfg.Command, cfg.Args...)
    if cfg.WorkingDir != "" {
        cmd.Dir = cfg.WorkingDir
    }

    // Inject environment: inherit current env + ARI_* overrides
    cmd.Env = append(os.Environ(), envMapToSlice(input.EnvVars)...)

    // Process group: allows killing all children on cancel
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

    var stdoutBuf, stderrBuf bytes.Buffer
    stdoutPipe, _ := cmd.StdoutPipe()
    stderrPipe, _ := cmd.StderrPipe()

    if err := cmd.Start(); err != nil {
        return adapter.InvokeResult{Status: adapter.RunStatusFailed}, fmt.Errorf("start: %w", err)
    }

    // Stream stdout for real-time SSE log lines
    go streamLines(stdoutPipe, &stdoutBuf, cfg.MaxExcerptBytes, hooks.OnLogLine, "info")
    go streamLines(stderrPipe, &stderrBuf, cfg.MaxExcerptBytes, nil, "error")

    err := cmd.Wait()

    // Determine status
    var status adapter.RunStatus
    exitCode := 0
    switch {
    case runCtx.Err() == context.DeadlineExceeded:
        // Kill the process group so no orphaned children remain
        _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
        status = adapter.RunStatusTimedOut
    case ctx.Err() != nil:
        // External cancellation (graceful stop, REQ-011)
        _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
        status = adapter.RunStatusCancelled
    case err != nil:
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
        }
        status = adapter.RunStatusFailed
    default:
        status = adapter.RunStatusSucceeded
    }

    stdout := truncate(stdoutBuf.String(), cfg.MaxExcerptBytes)
    stderr := truncate(stderrBuf.String(), cfg.MaxExcerptBytes)

    // The agent writes its session state to stdout as the last JSON line:
    // {"ari_session_state": "<opaque blob>"}
    sessionState := extractSessionState(stdout)

    return adapter.InvokeResult{
        Status:       status,
        ExitCode:     exitCode,
        Stdout:       stdout,
        Stderr:       stderr,
        SessionState: sessionState,
    }, nil
}

func (p *ProcessAdapter) TestEnvironment(level adapter.TestLevel) (adapter.TestResult, error) {
    // Basic: no-op; the command is agent-specific, not global
    return adapter.TestResult{Available: true, Message: "process adapter always available"}, nil
}

func (p *ProcessAdapter) Models() []adapter.ModelDefinition { return nil }
```

**Key security note:** The process adapter executes arbitrary commands. In production, callers MUST ensure `adapterConfig.command` is an absolute path to a trusted binary. Sandboxing (namespaces, seccomp) is out of scope for v1 but MUST be documented.

---

## Wakeup Queue

### Purpose

Decouple wakeup triggers (HTTP, timer, assignment) from adapter dispatch. Persists requests to the database before dispatching (REQ-058), enabling replay on restart and enforcing per-squad concurrency limits (REQ-032).

### Database Table

```sql
-- Migration: 20260315000011_create_runtime_tables.sql (partial)

CREATE TYPE wakeup_invocation_source AS ENUM (
    'on_demand',
    'timer',
    'assignment',
    'inbox_resolved',
    'conversation_message'
);

CREATE TYPE wakeup_request_status AS ENUM (
    'pending',
    'dispatched',
    'discarded'
);

CREATE TABLE wakeup_requests (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id            UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id            UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    invocation_source   wakeup_invocation_source NOT NULL,
    status              wakeup_request_status NOT NULL DEFAULT 'pending',
    context_json        JSONB NOT NULL DEFAULT '{}', -- ARI_TASK_ID, ARI_CONVERSATION_ID, etc.
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at       TIMESTAMPTZ,
    discarded_at        TIMESTAMPTZ,
    CONSTRAINT uq_wakeup_pending_per_agent
        UNIQUE NULLS NOT DISTINCT (agent_id, status)
        WHERE status = 'pending'          -- deduplication: at most one pending per agent
);

CREATE INDEX idx_wakeup_squad_pending ON wakeup_requests(squad_id, created_at)
    WHERE status = 'pending';
```

**Deduplication logic (REQ-002, REQ-043):** The partial unique index `uq_wakeup_pending_per_agent` ensures at most one `pending` record per agent. When a new trigger fires for an agent that already has a pending wakeup, `INSERT ... ON CONFLICT DO NOTHING` leaves the existing entry. If the agent is already `running`, the wakeup is still enqueued (pending) and dispatched after the current run completes.

### Priority Mapping

```go
var wakeupPriority = map[string]int{
    "assignment":            0, // highest
    "inbox_resolved":        1,
    "conversation_message":  2,
    "timer":                 3,
    "on_demand":             4,
}
```

The queue processor pulls the next `pending` request with the lowest priority value for a given squad, subject to the squad concurrency limit.

### WakeupProcessor

**Package:** `internal/runtime/wakeup/`

```go
type Processor struct {
    db          *sql.DB
    queries     *db.Queries
    registry    *adapter.Registry
    sseHub      *sse.Hub
    runSvc      *RunService
    maxPerSquad int           // default 3 (REQ-032)
    pollInterval time.Duration // default 500ms
}

// Start launches one goroutine per squad and a global polling loop.
// Called from runServer() after migrations are complete.
func (p *Processor) Start(ctx context.Context) {
    go p.pollLoop(ctx)
}

func (p *Processor) pollLoop(ctx context.Context) {
    ticker := time.NewTicker(p.pollInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            p.dispatch(ctx)
        }
    }
}

func (p *Processor) dispatch(ctx context.Context) {
    // 1. Count running HeartbeatRuns per squad
    // 2. For each squad with capacity, pull next pending WakeupRequest in priority order
    // 3. Validate agent is not paused/terminated (REQ-044)
    // 4. Mark WakeupRequest as 'dispatched'
    // 5. Call runSvc.Invoke() in a goroutine
}
```

The `pollInterval` trades latency against DB load. Wakeup dispatch latency target is ≤2s (REQ-051); 500ms poll plus minimal processing overhead keeps this well within budget.

### WakeupService

**Package:** `internal/runtime/wakeup/`
Called by HTTP handlers, timer goroutines, and assignment hooks.

```go
type Service struct {
    queries *db.Queries
    dbConn  *sql.DB
}

// Enqueue persists a WakeupRequest and returns immediately.
// Returns nil if an identical pending request already exists (deduplication).
func (s *Service) Enqueue(ctx context.Context, agentID, squadID uuid.UUID, source string, ctxJSON map[string]any) error
```

---

## HeartbeatRun Lifecycle

### Database Table

```sql
CREATE TYPE heartbeat_run_status AS ENUM (
    'queued',
    'running',
    'succeeded',
    'failed',
    'cancelled',
    'timed_out'
);

CREATE TABLE heartbeat_runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id            UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id            UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    wakeup_request_id   UUID REFERENCES wakeup_requests(id),
    invocation_source   wakeup_invocation_source NOT NULL,
    status              heartbeat_run_status NOT NULL DEFAULT 'queued',
    session_id_before   TEXT,
    session_id_after    TEXT,
    exit_code           INTEGER,
    usage_json          JSONB,           -- TokenUsage
    stdout_excerpt      TEXT,
    stderr_excerpt      TEXT,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_heartbeat_runs_agent   ON heartbeat_runs(agent_id, created_at DESC);
CREATE INDEX idx_heartbeat_runs_squad   ON heartbeat_runs(squad_id, created_at DESC);
CREATE INDEX idx_heartbeat_runs_active  ON heartbeat_runs(squad_id)
    WHERE status IN ('queued', 'running');  -- startup audit query
```

### Status Flow

```
queued → running → succeeded
                 → failed
                 → cancelled   (user stop or server drain)
                 → timed_out   (exceeded adapter timeout)
```

### RunService

**Package:** `internal/runtime/`

```go
type RunService struct {
    db       *sql.DB
    queries  *db.Queries
    registry *adapter.Registry
    tokenSvc *RunTokenService
    sseHub   *sse.Hub
    // active tracks cancel funcs for running invocations (for graceful stop)
    mu     sync.Mutex
    active map[uuid.UUID]context.CancelFunc // runID → cancel
}

// Invoke creates a HeartbeatRun record (status=queued), transitions the agent
// to running, mints a Run Token, calls adapter.Execute(), then finalizes.
func (s *RunService) Invoke(ctx context.Context, req WakeupRequest) error

// Stop cancels the active invocation for the agent's current run.
// Transitions agent to paused (REQ-011).
func (s *RunService) Stop(ctx context.Context, agentID uuid.UUID) error
```

**Invoke sequence:**

1. Resolve adapter from registry; return `ADAPTER_NOT_FOUND` if missing (REQ-042).
2. Load agent, squad, session state from DB.
3. Build `InvokeInput` — populate all `ARI_*` env vars (REQ-010).
4. `INSERT INTO heartbeat_runs (status='queued') RETURNING id` → emit `heartbeat.run.queued` SSE.
5. Mint Run Token JWT (`RunTokenService.Mint`).
6. `UPDATE heartbeat_runs SET status='running', started_at=now()` → emit `heartbeat.run.started` SSE.
7. `UPDATE agents SET status='running'` → emit `agent.status.changed` SSE.
8. Register cancel func in `active` map.
9. Call `adapter.Execute(runCtx, input, hooks)` — hooks forward log lines as `heartbeat.run.log` SSE events.
10. On return: finalize (write result, persist session state, emit `heartbeat.run.finished`, transition agent status, record CostEvent if usage is non-zero).

---

## Run Token JWT

### Purpose

Agents authenticate callbacks to the Ari API using a short-lived HS256 JWT minted per invocation. Tokens are never reused across runs (REQ-041).

### Claims

```go
// internal/runtime/token/token.go
package token

type RunTokenClaims struct {
    jwt.RegisteredClaims             // sub=agentId, exp=48h
    SquadID  string `json:"squad_id"`
    RunID    string `json:"run_id"`
    Role     string `json:"role"`     // agent role
    TokenType string `json:"typ"`     // "run_token" — distinguishes from user session JWTs
}
```

- **Algorithm:** HS256 (same `signingKey` as user JWTs, `TokenType` claim differentiates them).
- **TTL:** 48 hours from issuance (REQ-053).
- **Injected as:** `ARI_API_KEY` environment variable.

### RunTokenService

```go
type RunTokenService struct {
    signingKey []byte
    revoked    sync.Map // runID(string) → struct{}{}
}

func (s *RunTokenService) Mint(agentID, squadID, runID uuid.UUID, role string) (string, error)

func (s *RunTokenService) Validate(tokenString string) (*RunTokenClaims, error)

// Revoke adds the run ID to the in-memory revocation list (REQ-054).
// Called when an agent is paused or terminated (REQ-012).
func (s *RunTokenService) Revoke(runID uuid.UUID)

// IsRevoked checks the revocation list.
func (s *RunTokenService) IsRevoked(runID uuid.UUID) bool
```

**Revocation list cleanup:** The `sync.Map` grows as tokens are revoked. A background goroutine sweeps expired entries every hour by checking `claims.ExpiresAt < now()`. This bounds the list to tokens still within their 48h window.

### Run Token Middleware

The auth middleware is extended to accept `Authorization: Bearer <run_token>` with `TokenType == "run_token"`:

```go
// In auth.Middleware(), after extracting the token string:
// Try to parse as a run token first (typ claim check).
// If it is a run token:
//   1. Validate signature + expiry.
//   2. Check RunTokenService.IsRevoked(runID).
//   3. Check agent status is not paused/terminated (REQ-034).
//   4. Inject AgentIdentity into context (separate context key from user Identity).
```

The agent identity context key allows handlers like checkout and release to verify `agentID` matches the token's `sub` claim (REQ-056).

---

## SSE Infrastructure

**Package:** `internal/server/sse/`

### Hub

```go
// Hub manages per-squad SSE subscriber channels.
// All methods are safe for concurrent use.
type Hub struct {
    mu          sync.RWMutex
    subscribers map[uuid.UUID]map[*Subscriber]struct{} // squadID → set
    counter     atomic.Int64 // monotonic event ID
}

type Subscriber struct {
    SquadID uuid.UUID
    Ch      chan Event // buffered, size 64
}

type Event struct {
    ID    int64
    Type  string
    Data  any
}

func NewHub() *Hub

// Subscribe registers a subscriber and returns it.
// The caller must call Unsubscribe when the client disconnects.
func (h *Hub) Subscribe(squadID uuid.UUID) *Subscriber

// Unsubscribe removes the subscriber and closes its channel.
func (h *Hub) Unsubscribe(s *Subscriber)

// Publish emits an event to all subscribers on the given squad.
// Non-blocking: if a subscriber's channel is full, the event is dropped
// for that subscriber (REQ-059 — slow subscriber must not block others).
func (h *Hub) Publish(squadID uuid.UUID, eventType string, data any)
```

### SSE Handler

```go
// internal/server/handlers/sse_handler.go

func (h *SSEHandler) Stream(w http.ResponseWriter, r *http.Request) {
    squadID, ok := parseSquadID(w, r)
    if !ok { return }

    // Auth: verify user/agent is member of this squad
    if _, ok := h.verifyAccess(w, r, squadID); !ok { return }

    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

    flusher, ok := w.(http.Flusher)
    if !ok {
        writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "streaming not supported"})
        return
    }

    sub := h.hub.Subscribe(squadID)
    defer h.hub.Unsubscribe(sub)

    // REQ-046: Send initial snapshot of all agent statuses
    h.sendAgentSnapshot(w, flusher, squadID)

    // Keep-alive ticker (REQ-031)
    keepAlive := time.NewTicker(15 * time.Second)
    defer keepAlive.Stop()

    for {
        select {
        case <-r.Context().Done():
            // REQ-045: normal disconnect, no error logged
            return
        case <-keepAlive.C:
            fmt.Fprintf(w, ": ping\n\n")
            flusher.Flush()
        case evt, ok := <-sub.Ch:
            if !ok { return }
            writeSSEEvent(w, flusher, evt)
        }
    }
}

func writeSSEEvent(w io.Writer, f http.Flusher, evt sse.Event) {
    data, _ := json.Marshal(evt.Data)
    fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", evt.ID, evt.Type, data)
    f.Flush()
}
```

**WriteTimeout consideration:** The standard `http.Server.WriteTimeout` of 30s will cut SSE connections. The SSE endpoint must use a custom `http.ResponseController` or the server must exclude SSE paths from the write timeout. Preferred approach: detect SSE paths in middleware and use `rc.SetWriteDeadline(time.Time{})` to disable the deadline for that connection.

### Route Registration

```
GET /api/squads/{id}/events/stream → SSEHandler.Stream
```

---

## Task Checkout (CAS)

### New Columns on `issues`

```sql
-- Migration: 20260315000011_create_runtime_tables.sql (continued)

ALTER TABLE issues
    ADD COLUMN checkout_run_id      UUID REFERENCES heartbeat_runs(id) ON DELETE SET NULL,
    ADD COLUMN execution_locked_at  TIMESTAMPTZ;

CREATE INDEX idx_issues_checkout_run ON issues(checkout_run_id) WHERE checkout_run_id IS NOT NULL;
```

### Checkout Endpoint

`POST /api/issues/{id}/checkout`

**Request body:**
```json
{
  "agentId": "<uuid>",
  "runId": "<uuid>",
  "expectedStatuses": ["todo", "backlog"]
}
```

**Validation:**
- Caller's Run Token `sub` must equal `agentId` (REQ-056).
- `runId` must match an active `HeartbeatRun` owned by `agentId`.

**CAS Transaction (REQ-013):**

```sql
-- name: CheckoutIssue :one
-- (implemented as raw SQL in the handler, not sqlc, due to dynamic expectedStatuses list)
BEGIN;

SELECT id, status, checkout_run_id, squad_id
FROM issues
WHERE id = $1
FOR UPDATE;

-- Application-layer checks inside transaction:
-- IF status NOT IN (expectedStatuses) OR checkout_run_id IS NOT NULL
--    AND checkout_run_id != $runId  → ROLLBACK, return 409
-- IF checkout_run_id == $runId → COMMIT, return 200 idempotent (REQ-016)

UPDATE issues
SET status              = 'in_progress',
    checkout_run_id     = $2,  -- runId
    execution_locked_at = now()
WHERE id = $1
RETURNING *;

COMMIT;
```

**Response codes:**
- `200 OK` — lock acquired (or idempotent re-acquire by same run).
- `409 Conflict` with `code=CHECKOUT_CONFLICT` — another run holds the lock (REQ-015).
- `422 Unprocessable Entity` — issue status not in `expectedStatuses`.

After success: emit `issue.updated` SSE event to squad (REQ-014).

### Release Endpoint

`POST /api/issues/{id}/release`

**Request body:**
```json
{
  "runId": "<uuid>",
  "targetStatus": "done"
}
```

```sql
UPDATE issues
SET checkout_run_id     = NULL,
    execution_locked_at = NULL,
    status              = $2   -- targetStatus (validated against issue state machine)
WHERE id = $1
  AND checkout_run_id = $3     -- runId; ensures only lock owner can release
RETURNING *;
```

If `0 rows updated` → return `409` (caller does not hold the lock).

After success: emit `issue.updated` SSE event (REQ-017).

### Stale Lock Sweep (REQ-033)

Background goroutine in `RunService`, runs every 5 minutes:

```sql
UPDATE issues
SET checkout_run_id = NULL,
    execution_locked_at = NULL,
    status = 'todo'
WHERE checkout_run_id IS NOT NULL
  AND execution_locked_at < now() - INTERVAL '2 hours'
  -- 2 hours = conservative max run duration; configurable via cfg.StaleCheckoutAge
RETURNING id, squad_id;
```

For each released row, emit `issue.updated` SSE event.

---

## Session Persistence

### Tables

```sql
-- Task sessions: one record per (agent, issue)
CREATE TABLE agent_task_sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    issue_id      UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    session_state TEXT NOT NULL,   -- opaque blob from adapter
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_task_session UNIQUE (agent_id, issue_id)
);

-- Conversation sessions: one record per (agent, conversation issue)
CREATE TABLE agent_conversation_sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    issue_id      UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    session_state TEXT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_conversation_session UNIQUE (agent_id, issue_id)
);
```

### Session Queries

```sql
-- name: UpsertTaskSession :exec
INSERT INTO agent_task_sessions (agent_id, issue_id, session_state)
VALUES (@agent_id, @issue_id, @session_state)
ON CONFLICT (agent_id, issue_id) DO UPDATE
    SET session_state = EXCLUDED.session_state,
        updated_at    = now();

-- name: GetTaskSession :one
SELECT session_state FROM agent_task_sessions
WHERE agent_id = @agent_id AND issue_id = @issue_id;

-- name: UpsertConversationSession :exec / GetConversationSession :one
-- (mirror of above for agent_conversation_sessions)
```

### Lifecycle

On each `Invoke()` call:
1. **Before Execute:** load `session_state` from the matching session table → populate `Run.SessionState` in `InvokeInput` and `session_id_before` in `heartbeat_runs`.
2. **After Execute:** if `InvokeResult.SessionState != ""`, upsert into the session table and write `session_id_after` to `heartbeat_runs` (REQ-026, REQ-027).

---

## New Database Migrations

All new tables go in one new migration file: **`20260315000011_create_runtime_tables.sql`**

```sql
-- +goose Up

-- Wakeup requests queue
CREATE TYPE wakeup_invocation_source AS ENUM (
    'on_demand', 'timer', 'assignment', 'inbox_resolved', 'conversation_message'
);
CREATE TYPE wakeup_request_status AS ENUM ('pending', 'dispatched', 'discarded');

CREATE TABLE wakeup_requests (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id          UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id          UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    invocation_source wakeup_invocation_source NOT NULL,
    status            wakeup_request_status NOT NULL DEFAULT 'pending',
    context_json      JSONB NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at     TIMESTAMPTZ,
    discarded_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_wakeup_pending_per_agent ON wakeup_requests(agent_id)
    WHERE status = 'pending';
CREATE INDEX idx_wakeup_squad_pending ON wakeup_requests(squad_id, created_at)
    WHERE status = 'pending';

-- HeartbeatRun records
CREATE TYPE heartbeat_run_status AS ENUM (
    'queued', 'running', 'succeeded', 'failed', 'cancelled', 'timed_out'
);

CREATE TABLE heartbeat_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id          UUID NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    agent_id          UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    wakeup_request_id UUID REFERENCES wakeup_requests(id),
    invocation_source wakeup_invocation_source NOT NULL,
    status            heartbeat_run_status NOT NULL DEFAULT 'queued',
    session_id_before TEXT,
    session_id_after  TEXT,
    exit_code         INTEGER,
    usage_json        JSONB,
    stdout_excerpt    TEXT,
    stderr_excerpt    TEXT,
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_heartbeat_runs_agent  ON heartbeat_runs(agent_id, created_at DESC);
CREATE INDEX idx_heartbeat_runs_squad  ON heartbeat_runs(squad_id, created_at DESC);
CREATE INDEX idx_heartbeat_runs_active ON heartbeat_runs(squad_id)
    WHERE status IN ('queued', 'running');

-- Task checkout columns on issues
ALTER TABLE issues
    ADD COLUMN checkout_run_id     UUID REFERENCES heartbeat_runs(id) ON DELETE SET NULL,
    ADD COLUMN execution_locked_at TIMESTAMPTZ;
CREATE INDEX idx_issues_checkout_run ON issues(checkout_run_id)
    WHERE checkout_run_id IS NOT NULL;

-- Session persistence
CREATE TABLE agent_task_sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    issue_id      UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    session_state TEXT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_task_session UNIQUE (agent_id, issue_id)
);

CREATE TABLE agent_conversation_sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    issue_id      UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    session_state TEXT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_conversation_session UNIQUE (agent_id, issue_id)
);

-- +goose Down
ALTER TABLE issues DROP COLUMN IF EXISTS checkout_run_id;
ALTER TABLE issues DROP COLUMN IF EXISTS execution_locked_at;
DROP TABLE IF EXISTS agent_conversation_sessions;
DROP TABLE IF EXISTS agent_task_sessions;
DROP TABLE IF EXISTS heartbeat_runs;
DROP TABLE IF EXISTS wakeup_requests;
DROP TYPE IF EXISTS heartbeat_run_status;
DROP TYPE IF EXISTS wakeup_request_status;
DROP TYPE IF EXISTS wakeup_invocation_source;
```

Config additions needed in `internal/config/config.go`:

```go
// New runtime config fields
MaxRunsPerSquad    int           // default 3 (REQ-032)
StaleCheckoutAge   time.Duration // default 2h (REQ-033)
AgentDrainTimeout  time.Duration // default 30s (REQ-029)
```

---

## New API Endpoints

### Handler Registration

`AgentHandler.RegisterRoutes` gains two new routes:

```go
mux.HandleFunc("POST /api/agents/{id}/wake", h.WakeAgent)
mux.HandleFunc("POST /api/agents/{id}/stop", h.StopAgent)
```

`IssueHandler.RegisterRoutes` gains:

```go
mux.HandleFunc("POST /api/issues/{id}/checkout", h.CheckoutIssue)
mux.HandleFunc("POST /api/issues/{id}/release",  h.ReleaseIssue)
```

A new `SSEHandler` is registered in `server.New()`:

```go
mux.HandleFunc("GET /api/squads/{id}/events/stream", sseHandler.Stream)
```

### Endpoint Contracts

#### `POST /api/agents/{id}/wake`

- **Auth:** user session or Run Token (squad member).
- **Body:** `{}` (no required fields; optional `{ "reason": "on_demand" }`).
- **Success `200`:** `{ "runId": "<uuid>", "agentId": "<uuid>", "status": "queued" }`.
- **`422`:** `{ "error": "...", "code": "ADAPTER_NOT_FOUND" }` — no adapter registered.
- **`409`:** agent is `paused` or `terminated` (REQ-044 — discards wakeup, returns error to caller).
- **Side effects:** calls `WakeupService.Enqueue(agentID, squadID, "on_demand", ctx)`.

#### `POST /api/agents/{id}/stop`

- **Auth:** user session (squad member).
- **Body:** `{}`.
- **Success `200`:** updated agent response with `status: "paused"`.
- **Side effects:** calls `RunService.Stop(agentID)`, which cancels the run context and transitions agent → `paused`. Run Token is revoked (REQ-012).

#### `GET /api/squads/{id}/events/stream`

- **Auth:** user session or Run Token for the same squad.
- **Headers:** `Content-Type: text/event-stream`, keep-alive ping every 15s.
- **On connect:** synthetic `agent.status.changed` snapshot (REQ-046).
- **Events:** all types listed in the SSE event table.

#### `POST /api/issues/{id}/checkout`

- **Auth:** Run Token (agents only; user callers rejected with `403`).
- **Body:** `{ "agentId": "<uuid>", "runId": "<uuid>", "expectedStatuses": ["todo"] }`.
- **Success `200`:** updated issue JSON.
- **`409`:** `{ "code": "CHECKOUT_CONFLICT" }`.
- **`422`:** status precondition not met.

#### `POST /api/issues/{id}/release`

- **Auth:** Run Token.
- **Body:** `{ "runId": "<uuid>", "targetStatus": "done" }`.
- **Success `200`:** updated issue JSON.
- **`409`:** caller does not hold the lock.

---

## Data Flow Diagrams

### Wakeup Dispatch Flow

```
User / Timer / Assignment Hook
        |
        | calls WakeupService.Enqueue()
        v
  wakeup_requests (status=pending)
        |
        | WakeupProcessor.pollLoop() [every 500ms]
        | checks squad concurrency < maxPerSquad
        v
  WakeupProcessor.dispatch()
        |
        | marks WakeupRequest dispatched
        | calls RunService.Invoke() in goroutine
        v
  RunService.Invoke()
    1. INSERT heartbeat_runs (status=queued)
    2. Emit heartbeat.run.queued SSE
    3. Mint Run Token JWT
    4. UPDATE heartbeat_runs (status=running)
    5. UPDATE agents SET status=running
    6. Emit heartbeat.run.started + agent.status.changed SSE
    7. adapter.Execute(runCtx, input, hooks)  ← blocks
         |   OnLogLine → Emit heartbeat.run.log SSE
         v
    8. InvokeResult returned
    9. UPDATE heartbeat_runs (status=succeeded|failed|cancelled|timed_out)
   10. UPDATE agents SET status=idle|error|paused
   11. Emit heartbeat.run.finished + agent.status.changed SSE
   12. Upsert session state (if non-empty)
   13. INSERT cost_events (if usage > 0)
```

### Task Checkout Flow (from Requirements)

```
Agent                           Ari Server                    Database
  |                                 |                              |
  |  POST /issues/{id}/checkout     |                              |
  |  { agentId, expectedStatuses,   |                              |
  |    runId }                      |                              |
  |-------------------------------->|                              |
  |                                 |  BEGIN TRANSACTION           |
  |                                 |  SELECT ... FOR UPDATE       |
  |                                 |  WHERE id = {id}             |
  |                                 |------------------------------>|
  |                                 |  <- row locked               |
  |                                 |                              |
  |                                 |  IF status NOT IN expected   |
  |                                 |     OR checkout_run_id != NULL|
  |                                 |  THEN ROLLBACK → 409         |
  |                                 |                              |
  |                                 |  UPDATE issues SET           |
  |                                 |    status = 'in_progress',   |
  |                                 |    checkout_run_id = {runId},|
  |                                 |    execution_locked_at = NOW()|
  |                                 |  RETURNING *                 |
  |                                 |------------------------------>|
  |                                 |  COMMIT                      |
  |                                 |                              |
  |  200 OK { issue }               |  EMIT issue.updated SSE      |
  |<--------------------------------|                              |
```

### SSE Fan-out Flow

```
RunService / Handler
        |
        | hub.Publish(squadID, eventType, data)
        v
  sse.Hub
        |  for each Subscriber in squad's set:
        |    select { case sub.Ch <- event: default: /* drop */ }
        v
  Subscriber goroutines (one per connected SSE client)
        |
        | reads from sub.Ch
        | writes SSE event to http.ResponseWriter
        | flusher.Flush()
        v
  Client browser / agent process
```

---

## Error Handling

### Adapter Crash Recovery (REQ-028)

`adapter.Execute()` is called inside a `recover()` wrapper in `RunService.Invoke()`:

```go
func (s *RunService) safeExecute(ctx context.Context, a adapter.Adapter, input adapter.InvokeInput, hooks adapter.Hooks) (result adapter.InvokeResult, err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("adapter panic: %v", r)
            result = adapter.InvokeResult{Status: adapter.RunStatusFailed}
        }
    }()
    return a.Execute(ctx, input, hooks)
}
```

On any crash/panic: `HeartbeatRun` is marked `failed`, agent transitions to `error`, inbox alert is created (REQ-008).

### Server Shutdown Drain (REQ-029)

In `runServer()`, before the HTTP server shuts down:

```go
// Drain: signal all active adapters to cancel
drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.AgentDrainTimeout)
defer drainCancel()
runService.DrainAll(drainCtx)
```

`DrainAll()` calls `cancel()` for every entry in `active` map, then waits for all goroutines to exit (via a `sync.WaitGroup`).

### Startup Audit of Stale Runs (REQ-057)

Called in `runServer()` immediately after migrations, before starting the wakeup processor:

```go
func auditStaleRuns(ctx context.Context, queries *db.Queries) {
    // Mark any HeartbeatRun in 'queued' or 'running' as 'cancelled'
    // (server was not cleanly shut down; drain was incomplete)
    n, err := queries.CancelStaleRuns(ctx)
    if err != nil {
        slog.Error("stale run audit failed", "error", err)
        return
    }
    if n > 0 {
        slog.Warn("marked stale runs as cancelled", "count", n)
        // Also reset any agent stuck in 'running' back to 'error'
        _ = queries.ResetStaleRunningAgents(ctx)
    }
}
```

```sql
-- name: CancelStaleRuns :execrows
UPDATE heartbeat_runs
SET status = 'cancelled', finished_at = now()
WHERE status IN ('queued', 'running');

-- name: ResetStaleRunningAgents :exec
UPDATE agents
SET status = 'error'
WHERE status = 'running';
```

### Budget Enforcement (REQ-047)

After each `CostEvent` insert, `RunService` calls `checkBudget()`:

```go
func (s *RunService) checkBudget(ctx context.Context, agentID uuid.UUID) {
    agent, _ := s.queries.GetAgentByID(ctx, agentID)
    if agent.BudgetMonthlyCents == nil { return }

    spent, _ := s.queries.GetAgentMonthlySpend(ctx, agentID)
    pct := float64(spent) / float64(*agent.BudgetMonthlyCents) * 100

    if pct >= 100 {
        // Hard stop: auto-pause (REQ-047)
        s.queries.UpdateAgentStatus(ctx, agentID, "paused")
        s.cancelPendingWakeups(ctx, agentID)
        s.sseHub.Publish(agent.SquadID, "agent.status.changed", ...)
        s.sseHub.Publish(agent.SquadID, "cost.threshold.warning", ...)
        s.createInboxAlert(ctx, agentID, "budget_warning", ...)
    } else if pct >= 80 {
        // Soft warning (REQ-025)
        s.sseHub.Publish(agent.SquadID, "cost.threshold.warning", ...)
        s.createInboxAlert(ctx, agentID, "budget_warning", ...)
    }
}
```

---

## Package Layout

```
internal/
  adapter/
    adapter.go          ← Adapter interface, all types (expanded)
    registry.go         ← Registry
    process/
      process.go        ← ProcessAdapter implementation
      process_test.go
  runtime/
    run_service.go      ← RunService (Invoke, Stop, DrainAll)
    run_service_test.go
    token/
      token.go          ← RunTokenService (Mint, Validate, Revoke)
      token_test.go
    wakeup/
      service.go        ← WakeupService (Enqueue)
      processor.go      ← WakeupProcessor (pollLoop, dispatch)
      processor_test.go
  server/
    sse/
      hub.go            ← Hub, Subscriber, Event
      hub_test.go
    handlers/
      sse_handler.go    ← SSEHandler.Stream
      runtime_handler.go  ← WakeAgent, StopAgent (extend AgentHandler or new handler)
      checkout_handler.go ← CheckoutIssue, ReleaseIssue (extend IssueHandler)
  database/
    migrations/
      20260315000011_create_runtime_tables.sql
    queries/
      heartbeat_runs.sql
      wakeup_requests.sql
      sessions.sql      ← agent_task_sessions + agent_conversation_sessions
```

---

## Server Initialization Changes (`cmd/ari/run.go`)

```go
// After auth initialization, before ListenAndServe:

// 1. Startup audit
auditStaleRuns(ctx, queries)

// 2. Build runtime dependencies
registry := adapter.NewRegistry()
registry.Register(process.New())
// (check environments, mark unavailable on failure)

sseHub := sse.NewHub()

runTokenSvc := token.NewRunTokenService(signingKey) // same key as JWTService

runSvc := runtime.NewRunService(db, queries, registry, runTokenSvc, sseHub, cfg)

wakeupSvc := wakeup.NewService(db, queries)
wakeupProc := wakeup.NewProcessor(db, queries, registry, sseHub, runSvc, cfg)
wakeupProc.Start(ctx)

// 3. Pass to handlers
runtimeHandler := handlers.NewRuntimeHandler(queries, db, wakeupSvc, runSvc)
sseHandler := handlers.NewSSEHandler(queries, sseHub)

// 4. Register in server.New()
srv := server.New(..., runtimeHandler, sseHandler, ...)
```

---

## Testing Strategy

### Unit Tests

**`internal/adapter/process/process_test.go`**
- Execute a real shell command (e.g., `echo hello`), verify stdout captured.
- Execute a command that exits non-zero, verify `RunStatusFailed` and `ExitCode`.
- Cancel context mid-run, verify `RunStatusCancelled` and process group killed.
- Timeout fires before command completes, verify `RunStatusTimedOut`.
- Session state extraction from last JSON line in stdout.

**`internal/runtime/token/token_test.go`**
- Mint and validate a Run Token, verify all claims present.
- Revoke a run ID, verify `IsRevoked` returns true.
- Expired token rejected by Validate.
- Wrong `TokenType` claim rejected.

**`internal/server/sse/hub_test.go`**
- Subscribe, publish, receive event on channel.
- Slow subscriber (full channel) does not block faster subscriber.
- Unsubscribe closes channel.
- Events are scoped to correct squad (cross-squad isolation).

**`internal/runtime/wakeup/processor_test.go`**
- Priority ordering: `assignment` dispatched before `on_demand` when both pending.
- Concurrency limit: 3rd pending request not dispatched while 3 runs active.
- Paused agent wakeup is discarded, not dispatched.

### Integration Tests

**`internal/server/handlers/checkout_integration_test.go`**
(follows pattern of existing `agent_integration_test.go`)

- Happy path: checkout succeeds, issue status becomes `in_progress`, checkout_run_id set.
- Conflict: second checkout by different run returns 409 `CHECKOUT_CONFLICT`.
- Idempotent: same run re-checks out, returns 200.
- Release: clears lock, status transitions to `targetStatus`, SSE emitted.
- Stale lock sweep: `execution_locked_at` in the past cleared on sweep run.

**`internal/runtime/run_service_test.go`** (uses embedded test DB)
- Full invoke cycle with a mock adapter: queued → running → succeeded.
- Stop during run: HeartbeatRun → cancelled, agent → paused.
- Startup audit: pre-existing running HeartbeatRun marked cancelled.
- Session state roundtrip: session_id_after written, loaded as session_id_before on next run.

### End-to-End Tests

Using the existing integration test harness (`handlers/*_integration_test.go` pattern):

1. **Wake/Stop lifecycle:**
   POST `/wake` → verify SSE event `agent.status.changed` with `to: "running"` → POST `/stop` → verify `to: "paused"`.

2. **Checkout/Release cycle:**
   Agent Run Token POST `/checkout` → verify `200` → second agent POST `/checkout` → verify `409` → POST `/release` → verify lock cleared.

3. **SSE snapshot on connect:**
   Connect SSE client → verify first event is `agent.status.changed` for each agent in squad.

4. **Budget auto-pause:**
   Set `budget_monthly_cents = 100`, simulate spend > 100 → verify agent transitions to `paused`.

---

## Open Questions Resolved

| Question | Decision |
|---|---|
| Which adapter first? | `process` (subprocess) for v1. |
| How is agent output persisted? | DB text excerpts (up to 64 KB per stream) stored in `heartbeat_runs`. |
| SSE reconnection? | Client-side: browser `EventSource` reconnects automatically; server sends current state snapshot on connect (REQ-046). |
| Sandbox for subprocess? | Out of scope for v1; documented as operator responsibility. Absolute command paths required. |
| SSE WriteTimeout? | Disable per-connection write deadline for SSE paths using `http.ResponseController.SetWriteDeadline(time.Time{})`. |
| Token revocation list memory growth? | Background goroutine sweeps entries with `exp < now()` hourly. |

---

## References

- Requirements: `docx/features/11-agent-runtime/requirements.md`
- Agent status machine: `internal/domain/agent.go` (`ValidateStatusTransition`)
- Issue status machine: `internal/domain/issue.go` (`ValidateIssueTransition`)
- Existing JWT pattern: `internal/auth/jwt.go` (`JWTService`)
- Auth middleware: `internal/auth/middleware.go`
- Handler pattern: `internal/server/handlers/agent_handler.go`
- DB migration pattern: `internal/database/migrations/20260314000005_create_agents.sql`
- Server init: `cmd/ari/run.go`
