package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
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
	mux.HandleFunc("POST /api/agent/me/reply", h.Reply)
	mux.HandleFunc("GET /api/agent/me/conversations", h.ListMyConversations)
}

// --- Response Types ---

type agentMeResponse struct {
	Agent        agentMeAgent             `json:"agent"`
	Squad        agentMeSquad             `json:"squad"`
	Tasks        []agentMeTask            `json:"tasks"`
	Conversation *agentMeConversation     `json:"conversation,omitempty"`
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

type agentMeConversation struct {
	ID           uuid.UUID         `json:"id"`
	Identifier   string            `json:"identifier"`
	Title        string            `json:"title"`
	Messages     []commentResponse `json:"messages"`
	SessionState string            `json:"sessionState"`
}

// --- Handlers ---

// GetMe returns the agent's own context: agent info, squad info, and assigned tasks.
// When the Run Token includes a conv_id claim, the response also includes conversation context.
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

	// Filter out conversations from the tasks array
	tasks := make([]agentMeTask, 0, len(issues))
	for _, iss := range issues {
		if iss.Type == db.IssueTypeConversation {
			continue
		}
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

	resp := agentMeResponse{
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
	}

	// If the Run Token has a conversation ID, include conversation context
	if identity.ConversationID != uuid.Nil {
		convIssue, err := h.queries.GetIssueByID(r.Context(), identity.ConversationID)
		if err == nil {
			comments, err := h.queries.ListIssueComments(r.Context(), db.ListIssueCommentsParams{
				IssueID:    identity.ConversationID,
				PageLimit:  100,
				PageOffset: 0,
			})
			if err == nil {
				msgs := make([]commentResponse, 0, len(comments))
				for _, c := range comments {
					msgs = append(msgs, dbCommentToResponse(c))
				}

				var sessionState string
				ss, ssErr := h.queries.GetConversationSession(r.Context(), db.GetConversationSessionParams{
					AgentID: identity.AgentID,
					IssueID: identity.ConversationID,
				})
				if ssErr == nil {
					sessionState = ss
				}

				resp.Conversation = &agentMeConversation{
					ID:           convIssue.ID,
					Identifier:   convIssue.Identifier,
					Title:        convIssue.Title,
					Messages:     msgs,
					SessionState: sessionState,
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// replyRequest is the request body for POST /api/agent/me/reply.
type replyRequest struct {
	ConversationID string `json:"conversationId"`
	Body           string `json:"body"`
}

// Reply allows an agent to post a reply to a conversation it is assigned to.
func (h *AgentSelfHandler) Reply(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error: "Agent authentication required",
			Code:  "UNAUTHENTICATED",
		})
		return
	}

	var req replyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if strings.TrimSpace(req.Body) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "body is required", Code: "VALIDATION_ERROR"})
		return
	}

	if len(req.Body) > 50000 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "message body exceeds maximum length", Code: "VALIDATION_ERROR"})
		return
	}

	convID, err := uuid.Parse(req.ConversationID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid conversation ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch conversation issue
	issue, err := h.queries.GetIssueByID(r.Context(), convID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Conversation not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("agent/me/reply: failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify type=conversation
	if issue.Type != db.IssueTypeConversation {
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error: "Issue is not a conversation",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Verify agent is the assignee
	if !issue.AssigneeAgentID.Valid || issue.AssigneeAgentID.UUID != identity.AgentID {
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error: "Agent is not assigned to this conversation",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Verify squad scope
	if issue.SquadID != identity.SquadID {
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error: "Not a member of this squad",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Verify conversation is not closed
	if issue.Status == db.IssueStatusDone || issue.Status == db.IssueStatusCancelled {
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error: "Conversation is closed",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Create comment in transaction
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("agent/me/reply: failed to begin tx", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	comment, err := qtx.CreateIssueComment(r.Context(), db.CreateIssueCommentParams{
		IssueID:    convID,
		AuthorType: db.CommentAuthorTypeAgent,
		AuthorID:   identity.AgentID,
		Body:       req.Body,
	})
	if err != nil {
		slog.Error("agent/me/reply: failed to create comment", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Log activity
	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    issue.SquadID,
		ActorType:  domain.ActivityActorAgent,
		ActorID:    identity.AgentID,
		Action:     "conversation.agent_replied",
		EntityType: "issue",
		EntityID:   convID,
		Metadata: map[string]any{
			"commentId": comment.ID.String(),
			"runId":     identity.RunID.String(),
		},
	}); err != nil {
		slog.Error("agent/me/reply: failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("agent/me/reply: failed to commit", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit SSE event
	if h.sseHub != nil {
		h.sseHub.Publish(issue.SquadID, "conversation.agent.replied", map[string]any{
			"conversationId": convID.String(),
			"agentId":        identity.AgentID.String(),
			"commentId":      comment.ID.String(),
			"body":           req.Body,
		})
	}

	writeJSON(w, http.StatusCreated, dbCommentToResponse(comment))
}

// ListMyConversations returns conversation issues assigned to the authenticated agent.
func (h *AgentSelfHandler) ListMyConversations(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error: "Agent authentication required",
			Code:  "UNAUTHENTICATED",
		})
		return
	}

	conversations, err := h.queries.ListConversationsByAgent(r.Context(), db.ListConversationsByAgentParams{
		AgentID:    uuid.NullUUID{UUID: identity.AgentID, Valid: true},
		PageLimit:  50,
		PageOffset: 0,
	})
	if err != nil {
		slog.Error("agent/me/conversations: failed to list conversations", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	type conversationItem struct {
		ID         uuid.UUID `json:"id"`
		Identifier string    `json:"identifier"`
		Title      string    `json:"title"`
		Status     string    `json:"status"`
	}

	items := make([]conversationItem, 0, len(conversations))
	for _, c := range conversations {
		// Defensive squad scoping: skip conversations that don't belong to the agent's squad
		if c.SquadID != identity.SquadID {
			continue
		}
		items = append(items, conversationItem{
			ID:         c.ID,
			Identifier: c.Identifier,
			Title:      c.Title,
			Status:     string(c.Status),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"conversations": items,
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
