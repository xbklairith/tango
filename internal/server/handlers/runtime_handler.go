package handlers

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// RuntimeHandler handles agent runtime operations (wake, stop, SSE streaming).
type RuntimeHandler struct {
	queries   *db.Queries
	dbConn    *sql.DB
	sseHub    *sse.Hub
	wakeupSvc *WakeupService
	runSvc    *RunService
	dataDir   string
}

// NewRuntimeHandler creates a new RuntimeHandler.
func NewRuntimeHandler(
	q *db.Queries,
	dbConn *sql.DB,
	sseHub *sse.Hub,
	wakeupSvc *WakeupService,
	runSvc *RunService,
	dataDir string,
) *RuntimeHandler {
	return &RuntimeHandler{
		queries:   q,
		dbConn:    dbConn,
		sseHub:    sseHub,
		wakeupSvc: wakeupSvc,
		runSvc:    runSvc,
		dataDir:   dataDir,
	}
}

// RegisterRoutes registers runtime routes on the given mux.
func (h *RuntimeHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/agents/{id}/wake", h.WakeAgent)
	mux.HandleFunc("POST /api/agents/{id}/stop", h.StopAgent)
	mux.HandleFunc("GET /api/squads/{id}/events/stream", h.StreamEvents)
	mux.HandleFunc("GET /api/agents/{id}/runs", h.ListAgentRuns)
	mux.HandleFunc("GET /api/runs/{id}/logs", h.GetRunLogs)
}

// --- Request/Response Types ---

type wakeRequest struct {
	Reason     string         `json:"reason"`     // optional override for invocation source
	ContextMap map[string]any `json:"contextMap"` // optional context JSON
}

// --- Handlers ---

// WakeAgent creates a wakeup request for the specified agent (REQ-001).
func (h *RuntimeHandler) WakeAgent(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	// Fetch agent to verify existence and get squad
	agent, err := h.queries.GetAgentByID(r.Context(), agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify squad membership (user must be a member)
	if _, ok := verifySquadAccess(w, r, agent.SquadID, h.queries); !ok {
		return
	}

	// Permission check: run.create
	if !requirePermission(w, r, agent.SquadID, auth.ResourceRun, auth.ActionCreate, makeRoleLookup(h.queries)) {
		return
	}

	// Validate agent status (must be active, idle, or running)
	switch domain.AgentStatus(agent.Status) {
	case domain.AgentStatusActive, domain.AgentStatusIdle, domain.AgentStatusRunning:
		// OK — running agents queue the wakeup (REQ-043)
	case domain.AgentStatusPaused, domain.AgentStatusTerminated:
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: fmt.Sprintf("Cannot wake agent in %s status", agent.Status),
			Code:  "INVALID_STATUS",
		})
		return
	case domain.AgentStatusPendingApproval:
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "Agent requires approval before it can be woken",
			Code:  "INVALID_STATUS",
		})
		return
	case domain.AgentStatusError:
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "Agent is in error state; transition to active before waking",
			Code:  "INVALID_STATUS",
		})
		return
	}

	// Check adapter type is set
	if !agent.AdapterType.Valid {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "Agent has no adapter type configured",
			Code:  "ADAPTER_NOT_FOUND",
		})
		return
	}

	// Parse optional request body
	var req wakeRequest
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			_ = json.Unmarshal(body, &req)
		}
	}

	source := "on_demand"
	if req.Reason != "" {
		source = req.Reason
	}

	contextMap := req.ContextMap
	if contextMap == nil {
		contextMap = make(map[string]any)
	}

	wakeup, err := h.wakeupSvc.Enqueue(r.Context(), agentID, agent.SquadID, source, contextMap)
	if err != nil {
		slog.Error("failed to enqueue wakeup", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Failed to enqueue wakeup", Code: "INTERNAL_ERROR"})
		return
	}

	if wakeup == nil {
		// Deduplicated — a pending wakeup already exists
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "Wakeup already pending for this agent",
		})
		return
	}

	slog.Info("agent wake requested", "agent_id", agentID, "wakeup_id", wakeup.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"wakeupId": wakeup.ID,
		"agentId":  agentID,
		"status":   "pending",
	})
}

// StopAgent gracefully stops a running agent (REQ-011).
func (h *RuntimeHandler) StopAgent(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	agent, err := h.queries.GetAgentByID(r.Context(), agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := verifySquadAccess(w, r, agent.SquadID, h.queries); !ok {
		return
	}

	// Permission check: run.update
	if !requirePermission(w, r, agent.SquadID, auth.ResourceRun, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	if domain.AgentStatus(agent.Status) != domain.AgentStatusRunning {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: fmt.Sprintf("Agent is not running (current status: %s)", agent.Status),
			Code:  "INVALID_STATUS",
		})
		return
	}

	if err := h.runSvc.Stop(r.Context(), agentID); err != nil {
		slog.Error("failed to stop agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Failed to stop agent", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit SSE event
	h.sseHub.Publish(agent.SquadID, "agent.status.changed", map[string]any{
		"agentId": agentID,
		"from":    "running",
		"to":      "paused",
	})

	slog.Info("agent stop requested", "agent_id", agentID)
	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Agent stop initiated",
		"status":  "paused",
	})
}

// StreamEvents serves SSE events for a squad (REQ-020, REQ-031, REQ-046).
func (h *RuntimeHandler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("id")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Verify access
	if _, ok := verifySquadAccess(w, r, squadID, h.queries); !ok {
		return
	}

	// Permission check: run.read
	if !requirePermission(w, r, squadID, auth.ResourceRun, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Streaming not supported", Code: "INTERNAL_ERROR"})
		return
	}

	// Disable write timeout for SSE connections
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("failed to disable write deadline for SSE", "error", err)
	}

	sub := h.sseHub.Subscribe(squadID)
	defer h.sseHub.Unsubscribe(sub)

	// REQ-046: Send initial snapshot of all agent statuses
	h.sendAgentSnapshot(w, flusher, squadID, r)

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
			if !ok {
				return
			}
			writeSSEEvent(w, flusher, evt)
		}
	}
}

// ListAgentRuns returns heartbeat runs for an agent.
func (h *RuntimeHandler) ListAgentRuns(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	agent, err := h.queries.GetAgentByID(r.Context(), agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := verifySquadAccess(w, r, agent.SquadID, h.queries); !ok {
		return
	}

	// Permission check: run.read
	if !requirePermission(w, r, agent.SquadID, auth.ResourceRun, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	runs, err := h.queries.ListAgentRunsWithContext(r.Context(), db.ListAgentRunsWithContextParams{
		AgentID:    agentID,
		PageLimit:  50,
		PageOffset: 0,
	})
	if err != nil {
		slog.Error("failed to list agent runs", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	result := make([]map[string]any, 0, len(runs))
	for _, r := range runs {
		entry := map[string]any{
			"id":               r.ID,
			"squadId":          r.SquadID,
			"agentId":          r.AgentID,
			"invocationSource": string(r.InvocationSource),
			"status":           string(r.Status),
			"createdAt":        r.CreatedAt.Format(time.RFC3339),
		}
		if r.ExitCode.Valid {
			entry["exitCode"] = r.ExitCode.Int32
		}
		if r.StartedAt.Valid {
			entry["startedAt"] = r.StartedAt.Time.Format(time.RFC3339)
		}
		if r.FinishedAt.Valid {
			entry["finishedAt"] = r.FinishedAt.Time.Format(time.RFC3339)
		}
		if r.StdoutExcerpt.Valid {
			entry["stdoutExcerpt"] = r.StdoutExcerpt.String
		}
		if r.StderrExcerpt.Valid {
			entry["stderrExcerpt"] = r.StderrExcerpt.String
		}
		if r.IssueIdentifier != "" {
			entry["issueIdentifier"] = r.IssueIdentifier
			entry["issueTitle"] = r.IssueTitle
			entry["issueId"] = r.IssueID
		}
		result = append(result, entry)
	}

	writeJSON(w, http.StatusOK, result)
}

// --- Helpers ---

func (h *RuntimeHandler) sendAgentSnapshot(w http.ResponseWriter, flusher http.Flusher, squadID uuid.UUID, r *http.Request) {
	agents, err := h.queries.ListAgentsBySquad(r.Context(), squadID)
	if err != nil {
		slog.Error("failed to list agents for SSE snapshot", "error", err)
		return
	}
	for _, agent := range agents {
		evt := sse.Event{
			Type: "agent.status.changed",
			Data: map[string]any{
				"agentId": agent.ID,
				"from":    string(agent.Status),
				"to":      string(agent.Status),
			},
		}
		writeSSEEvent(w, flusher, evt)
	}

	// Include unresolved inbox count in the initial snapshot.
	counts, err := h.queries.CountUnresolvedBySquad(r.Context(), squadID)
	if err != nil {
		slog.Error("failed to count unresolved inbox items for SSE snapshot", "error", err)
		return
	}
	writeSSEEvent(w, flusher, sse.Event{
		Type: "inbox.count",
		Data: map[string]any{
			"pendingCount":      counts.PendingCount,
			"acknowledgedCount": counts.AcknowledgedCount,
			"totalCount":        counts.TotalCount,
		},
	})
}

func writeSSEEvent(w io.Writer, f http.Flusher, evt sse.Event) {
	data, _ := json.Marshal(evt.Data)
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", evt.ID, evt.Type, data)
	f.Flush()
}

// GetRunLogs serves the JSONL log file for a run.
func (h *RuntimeHandler) GetRunLogs(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("id")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid run ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Verify the run exists and get squad for access check
	run, err := h.queries.GetHeartbeatRunByID(r.Context(), runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Run not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := verifySquadAccess(w, r, run.SquadID, h.queries); !ok {
		return
	}

	if h.dataDir == "" {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Log storage not configured", Code: "NOT_FOUND"})
		return
	}

	logPath := filepath.Join(h.dataDir, "runs", runID.String()+".jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "No log file for this run", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Failed to read log file", Code: "INTERNAL_ERROR"})
		return
	}
	defer f.Close()

	// Optional ?tail=N for last N lines
	tailParam := r.URL.Query().Get("tail")
	if tailParam != "" {
		tailN, err := strconv.Atoi(tailParam)
		if err != nil || tailN <= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid tail parameter", Code: "VALIDATION_ERROR"})
			return
		}

		// Read all lines, return last N
		var lines []string
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 256*1024), 256*1024)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if tailN > len(lines) {
			tailN = len(lines)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, line := range lines[len(lines)-tailN:] {
			fmt.Fprintln(w, line)
		}
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	io.Copy(w, f)
}
