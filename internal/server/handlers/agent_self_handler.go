package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// AgentSelfHandler provides endpoints for agents to query their own context
// and update task status using run-token authentication.
type AgentSelfHandler struct {
	queries *db.Queries
	dbConn  *sql.DB
	sseHub  *sse.Hub
}

// NewAgentSelfHandler creates a new AgentSelfHandler.
func NewAgentSelfHandler(q *db.Queries, dbConn *sql.DB, sseHub *sse.Hub) *AgentSelfHandler {
	return &AgentSelfHandler{queries: q, dbConn: dbConn, sseHub: sseHub}
}

// RegisterRoutes registers agent self-service routes on the given mux.
func (h *AgentSelfHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent/me", h.GetMe)
	mux.HandleFunc("PATCH /api/agent/me/task", h.UpdateTask)
}

// --- Response Types ---

type agentMeResponse struct {
	Agent agentMeAgent  `json:"agent"`
	Squad agentMeSquad  `json:"squad"`
	Tasks []agentMeTask `json:"tasks"`
}

type agentMeAgent struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	ShortName string    `json:"shortName"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
}

type agentMeSquad struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type agentMeTask struct {
	ID          uuid.UUID `json:"id"`
	Identifier  string    `json:"identifier"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Description *string   `json:"description"`
}

// --- Handlers ---

// GetMe returns the agent's own context: agent info, squad info, and assigned tasks.
func (h *AgentSelfHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error: "Agent authentication required",
			Code:  "UNAUTHENTICATED",
		})
		return
	}

	agent, err := h.queries.GetAgentByID(r.Context(), identity.AgentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("agent/me: failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	squad, err := h.queries.GetSquadByID(r.Context(), identity.SquadID)
	if err != nil {
		slog.Error("agent/me: failed to get squad", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	issues, err := h.queries.ListIssuesByAssigneeAgent(r.Context(), uuid.NullUUID{UUID: identity.AgentID, Valid: true})
	if err != nil {
		slog.Error("agent/me: failed to list issues", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	tasks := make([]agentMeTask, 0, len(issues))
	for _, iss := range issues {
		task := agentMeTask{
			ID:         iss.ID,
			Identifier: iss.Identifier,
			Title:      iss.Title,
			Status:     string(iss.Status),
		}
		if iss.Description.Valid {
			task.Description = &iss.Description.String
		}
		tasks = append(tasks, task)
	}

	writeJSON(w, http.StatusOK, agentMeResponse{
		Agent: agentMeAgent{
			ID:        agent.ID,
			Name:      agent.Name,
			ShortName: agent.ShortName,
			Role:      string(agent.Role),
			Status:    string(agent.Status),
		},
		Squad: agentMeSquad{
			ID:   squad.ID,
			Name: squad.Name,
			Slug: squad.Slug,
		},
		Tasks: tasks,
	})
}

// updateTaskRequest is the request body for PATCH /api/agent/me/task.
type updateTaskRequest struct {
	IssueID string `json:"issueId"`
	Status  string `json:"status"`
}

// UpdateTask allows an agent to update the status of an issue it is assigned to.
func (h *AgentSelfHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error: "Agent authentication required",
			Code:  "UNAUTHENTICATED",
		})
		return
	}

	var req updateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	issueID, err := uuid.Parse(req.IssueID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid issue ID", Code: "VALIDATION_ERROR"})
		return
	}

	newStatus := domain.IssueStatus(req.Status)
	if !newStatus.Valid() {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid status", Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch issue
	issue, err := h.queries.GetIssueByID(r.Context(), issueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("agent/me/task: failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify agent is the assignee
	if !issue.AssigneeAgentID.Valid || issue.AssigneeAgentID.UUID != identity.AgentID {
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error: "Agent is not the assignee of this issue",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Verify squad scope
	if issue.SquadID != identity.SquadID {
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error: "Issue does not belong to agent's squad",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Validate status transition
	if err := domain.ValidateIssueTransition(domain.IssueStatus(issue.Status), newStatus); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "INVALID_STATUS_TRANSITION"})
		return
	}

	// Update in transaction
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("agent/me/task: failed to begin tx", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	updated, err := qtx.UpdateIssue(r.Context(), db.UpdateIssueParams{
		ID:     issueID,
		Status: db.NullIssueStatus{IssueStatus: db.IssueStatus(newStatus), Valid: true},
	})
	if err != nil {
		slog.Error("agent/me/task: failed to update issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Log activity
	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    issue.SquadID,
		ActorType:  domain.ActivityActorAgent,
		ActorID:    identity.AgentID,
		Action:     "issue.status_changed",
		EntityType: "issue",
		EntityID:   issueID,
		Metadata: map[string]any{
			"from":  string(issue.Status),
			"to":    req.Status,
			"runId": identity.RunID.String(),
		},
	}); err != nil {
		slog.Error("agent/me/task: failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("agent/me/task: failed to commit", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit SSE event
	if h.sseHub != nil {
		h.sseHub.Publish(issue.SquadID, "issue.updated", map[string]any{
			"issueId": issueID,
			"status":  req.Status,
			"agentId": identity.AgentID,
		})
	}

	writeJSON(w, http.StatusOK, issueResponse{
		ID:           updated.ID,
		SquadID:      updated.SquadID,
		Identifier:   updated.Identifier,
		Type:         domain.IssueType(updated.Type),
		Title:        updated.Title,
		Status:       domain.IssueStatus(updated.Status),
		Priority:     domain.IssuePriority(updated.Priority),
		RequestDepth: int(updated.RequestDepth),
		CreatedAt:    updated.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    updated.UpdatedAt.Format(time.RFC3339),
	})
}
