package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// AgentSelfHandler provides endpoints for agents to query their own context
// and update task status using run-token authentication.
type AgentSelfHandler struct {
	queries       *db.Queries
	dbConn        *sql.DB
	sseHub        *sse.Hub
	budgetService *BudgetEnforcementService
	inboxService  *InboxService
	pipelineSvc   *PipelineService
}

// NewAgentSelfHandler creates a new AgentSelfHandler.
func NewAgentSelfHandler(q *db.Queries, dbConn *sql.DB, sseHub *sse.Hub, budgetSvc *BudgetEnforcementService, inboxSvc *InboxService) *AgentSelfHandler {
	return &AgentSelfHandler{
		queries:       q,
		dbConn:        dbConn,
		sseHub:        sseHub,
		budgetService: budgetSvc,
		inboxService:  inboxSvc,
	}
}

// SetPipelineService sets the pipeline service for auto-advance on done.
func (h *AgentSelfHandler) SetPipelineService(ps *PipelineService) {
	h.pipelineSvc = ps
}

// RegisterRoutes registers agent self-service routes on the given mux.
func (h *AgentSelfHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent/me", h.GetMe)
	mux.HandleFunc("PATCH /api/agent/me/task", h.UpdateTask)
	mux.HandleFunc("POST /api/agent/me/reply", h.Reply)
	mux.HandleFunc("GET /api/agent/me/conversations", h.ListMyConversations)
	mux.HandleFunc("GET /api/agent/me/assignments", h.GetAssignments)
	mux.HandleFunc("GET /api/agent/me/team", h.GetTeam)
	mux.HandleFunc("GET /api/agent/me/budget", h.GetBudget)
	mux.HandleFunc("GET /api/agent/me/goals", h.GetGoals)
	mux.HandleFunc("POST /api/agent/me/inbox", h.CreateInboxItem)
	mux.HandleFunc("POST /api/agent/me/cost", h.ReportCost)
	mux.HandleFunc("GET /api/agent/me/gates", h.GetGates)
}

// --- Response Types ---

type agentMeResponse struct {
	Agent        agentMeAgent         `json:"agent"`
	Squad        agentMeSquad         `json:"squad"`
	Tasks        []agentMeTask        `json:"tasks"`
	Conversation *agentMeConversation `json:"conversation,omitempty"`
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

// --- Feature 15 Response Types ---

type assignmentItem struct {
	ID              uuid.UUID  `json:"id"`
	Identifier      string     `json:"identifier"`
	Type            string     `json:"type"`
	Title           string     `json:"title"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	Description     *string    `json:"description"`
	ProjectID       *uuid.UUID `json:"projectId"`
	GoalID          *uuid.UUID `json:"goalId"`
	AssigneeAgentID *uuid.UUID `json:"assigneeAgentId"`
	PipelineID      *uuid.UUID `json:"pipelineId"`
	CurrentStageID  *uuid.UUID `json:"currentStageId"`
	CreatedAt       string     `json:"createdAt"`
	UpdatedAt       string     `json:"updatedAt"`
}

type assignmentsResponse struct {
	Assignments []assignmentItem `json:"assignments"`
	Total       int64            `json:"total"`
}

type teamAgent struct {
	ID            uuid.UUID  `json:"id"`
	Name          string     `json:"name"`
	ShortName     string     `json:"shortName"`
	Role          string     `json:"role"`
	Status        string     `json:"status"`
	ParentAgentID *uuid.UUID `json:"parentAgentId"`
}

type teamResponse struct {
	Self      teamAgent   `json:"self"`
	Parent    *teamAgent  `json:"parent,omitempty"`
	Siblings  []teamAgent `json:"siblings,omitempty"`
	Children  []teamAgent `json:"children,omitempty"`
	AllAgents []teamAgent `json:"allAgents,omitempty"`
}

type budgetResponse struct {
	SpentCents      int64  `json:"spentCents"`
	BudgetCents     *int64 `json:"budgetCents"`
	RemainingCents  *int64 `json:"remainingCents"`
	ThresholdStatus string `json:"thresholdStatus"`
	PeriodStart     string `json:"periodStart"`
	PeriodEnd       string `json:"periodEnd"`
}

type goalsResponse struct {
	Goals []goalItem `json:"goals"`
}

type goalItem struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	Description   *string   `json:"description"`
	Status        string    `json:"status"`
	RelatedIssues []string  `json:"relatedIssues"`
}

// --- Helpers ---

// requireActiveAgent fetches the agent and returns 403 if terminated, 404 if not found.
// Returns the agent and true if active, or writes an error and returns false.
func (h *AgentSelfHandler) requireActiveAgent(ctx context.Context, w http.ResponseWriter, agentID uuid.UUID) (db.Agent, bool) {
	agent, err := h.queries.GetAgentByID(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return db.Agent{}, false
		}
		slog.Error("agent/me: failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return db.Agent{}, false
	}
	if agent.Status == db.AgentStatusTerminated {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Agent terminated", Code: "FORBIDDEN"})
		return db.Agent{}, false
	}
	return agent, true
}

// resolveAgentIDs returns the agent IDs to scope queries by role.
// nil means "no agent filter — use squad-level queries" (captain).
// A non-nil slice means "use agent ID array queries" (lead: self+children, member: self).
func (h *AgentSelfHandler) resolveAgentIDs(ctx context.Context, identity auth.AgentIdentity) ([]uuid.UUID, error) {
	switch identity.Role {
	case "captain":
		return nil, nil
	case "lead":
		children, err := h.queries.ListAgentChildrenBySquad(ctx, db.ListAgentChildrenBySquadParams{
			SquadID:       identity.SquadID,
			ParentAgentID: uuid.NullUUID{UUID: identity.AgentID, Valid: true},
		})
		if err != nil {
			return nil, err
		}
		ids := []uuid.UUID{identity.AgentID}
		for _, c := range children {
			ids = append(ids, c.ID)
		}
		return ids, nil
	default: // member
		return []uuid.UUID{identity.AgentID}, nil
	}
}

func dbAgentToTeamAgent(a db.Agent) teamAgent {
	ta := teamAgent{
		ID:        a.ID,
		Name:      a.Name,
		ShortName: a.ShortName,
		Role:      string(a.Role),
		Status:    string(a.Status),
	}
	if a.ParentAgentID.Valid {
		ta.ParentAgentID = &a.ParentAgentID.UUID
	}
	return ta
}

func issueToAssignmentItem(i db.Issue) assignmentItem {
	item := assignmentItem{
		ID:         i.ID,
		Identifier: i.Identifier,
		Type:       string(i.Type),
		Title:      i.Title,
		Status:     string(i.Status),
		Priority:   string(i.Priority),
		CreatedAt:  i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  i.UpdatedAt.Format(time.RFC3339),
	}
	if i.Description.Valid {
		item.Description = &i.Description.String
	}
	if i.ProjectID.Valid {
		item.ProjectID = &i.ProjectID.UUID
	}
	if i.GoalID.Valid {
		item.GoalID = &i.GoalID.UUID
	}
	if i.AssigneeAgentID.Valid {
		item.AssigneeAgentID = &i.AssigneeAgentID.UUID
	}
	if i.PipelineID.Valid {
		item.PipelineID = &i.PipelineID.UUID
	}
	if i.CurrentStageID.Valid {
		item.CurrentStageID = &i.CurrentStageID.UUID
	}
	return item
}

func parseIntQueryParam(r *http.Request, name string, defaultVal, maxVal int) (int, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, errors.New("must be non-negative")
	}
	if maxVal > 0 && v > maxVal {
		return 0, errors.New("exceeds maximum")
	}
	return v, nil
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

	// Auto-advance: if status changed to done and issue is in a pipeline, advance to next stage
	if newStatus == domain.IssueStatusDone && h.pipelineSvc != nil {
		if handled, _, advErr := h.pipelineSvc.AutoAdvanceOnDone(r.Context(), issueID, identity.AgentID); handled && advErr != nil {
			slog.Warn("auto-advance on done failed", "issue_id", issueID, "error", advErr)
		}
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

// --- Feature 15: Extended Self-Service Endpoints ---

// GetAssignments returns issues assigned to the agent (or subtree for lead/captain).
func (h *AgentSelfHandler) GetAssignments(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Agent authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	if _, active := h.requireActiveAgent(r.Context(), w, identity.AgentID); !active {
		return
	}

	// Parse query params
	limit, err := parseIntQueryParam(r, "limit", 50, 100)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid limit parameter", Code: "VALIDATION_ERROR"})
		return
	}
	offset, err := parseIntQueryParam(r, "offset", 0, 0)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid offset parameter", Code: "VALIDATION_ERROR"})
		return
	}

	// Build optional filters with validation
	var filterStatus db.NullIssueStatus
	if s := r.URL.Query().Get("status"); s != "" {
		status := domain.IssueStatus(s)
		if !status.Valid() {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid status filter", Code: "VALIDATION_ERROR"})
			return
		}
		filterStatus = db.NullIssueStatus{IssueStatus: db.IssueStatus(s), Valid: true}
	}
	var filterType db.NullIssueType
	if t := r.URL.Query().Get("type"); t != "" {
		issueType := domain.IssueType(t)
		if !issueType.Valid() {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid type filter", Code: "VALIDATION_ERROR"})
			return
		}
		filterType = db.NullIssueType{IssueType: db.IssueType(t), Valid: true}
	}

	agentIDs, err := h.resolveAgentIDs(r.Context(), identity)
	if err != nil {
		slog.Error("agent/me/assignments: failed to resolve agent IDs", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	var issues []db.Issue
	var total int64

	if agentIDs == nil {
		// Captain: squad-wide query using existing ListIssuesBySquad/CountIssuesBySquad
		issues, err = h.queries.ListIssuesBySquad(r.Context(), db.ListIssuesBySquadParams{
			SquadID:      identity.SquadID,
			FilterStatus: filterStatus,
			FilterType:   filterType,
			SortField:    "created_at",
			PageLimit:    int32(limit),
			PageOffset:   int32(offset),
		})
		if err != nil {
			slog.Error("agent/me/assignments: failed to list issues", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		total, err = h.queries.CountIssuesBySquad(r.Context(), db.CountIssuesBySquadParams{
			SquadID:      identity.SquadID,
			FilterStatus: filterStatus,
			FilterType:   filterType,
		})
		if err != nil {
			slog.Error("agent/me/assignments: failed to count issues", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	} else if len(agentIDs) == 1 {
		// Member: single agent
		issues, err = h.queries.ListAssignmentsByAgent(r.Context(), db.ListAssignmentsByAgentParams{
			SquadID:      identity.SquadID,
			AgentID:      uuid.NullUUID{UUID: agentIDs[0], Valid: true},
			FilterStatus: filterStatus,
			FilterType:   filterType,
			PageLimit:    int32(limit),
			PageOffset:   int32(offset),
		})
		if err != nil {
			slog.Error("agent/me/assignments: failed to list assignments", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		total, err = h.queries.CountAssignmentsByAgent(r.Context(), db.CountAssignmentsByAgentParams{
			SquadID:      identity.SquadID,
			AgentID:      uuid.NullUUID{UUID: agentIDs[0], Valid: true},
			FilterStatus: filterStatus,
			FilterType:   filterType,
		})
		if err != nil {
			slog.Error("agent/me/assignments: failed to count assignments", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	} else {
		// Lead: multi-agent
		issues, err = h.queries.ListAssignmentsByAgentIDs(r.Context(), db.ListAssignmentsByAgentIDsParams{
			SquadID:      identity.SquadID,
			AgentIds:     agentIDs,
			FilterStatus: filterStatus,
			FilterType:   filterType,
			PageLimit:    int32(limit),
			PageOffset:   int32(offset),
		})
		if err != nil {
			slog.Error("agent/me/assignments: failed to list assignments by IDs", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		total, err = h.queries.CountAssignmentsByAgentIDs(r.Context(), db.CountAssignmentsByAgentIDsParams{
			SquadID:      identity.SquadID,
			AgentIds:     agentIDs,
			FilterStatus: filterStatus,
			FilterType:   filterType,
		})
		if err != nil {
			slog.Error("agent/me/assignments: failed to count assignments by IDs", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	}

	items := make([]assignmentItem, 0, len(issues))
	for _, i := range issues {
		items = append(items, issueToAssignmentItem(i))
	}

	writeJSON(w, http.StatusOK, assignmentsResponse{
		Assignments: items,
		Total:       total,
	})
}

// GetTeam returns the agent's team context based on hierarchy and role.
func (h *AgentSelfHandler) GetTeam(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Agent authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	agent, active := h.requireActiveAgent(r.Context(), w, identity.AgentID)
	if !active {
		return
	}

	selfAgent := dbAgentToTeamAgent(agent)
	resp := teamResponse{Self: selfAgent}

	if identity.Role == "captain" {
		// Captain: return self + allAgents
		allAgents, err := h.queries.ListAgentsBySquad(r.Context(), identity.SquadID)
		if err != nil {
			slog.Error("agent/me/team: failed to list agents", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		agents := make([]teamAgent, 0, len(allAgents))
		for _, a := range allAgents {
			agents = append(agents, dbAgentToTeamAgent(a))
		}
		resp.AllAgents = agents
	} else {
		// Member or lead: get parent + siblings
		if agent.ParentAgentID.Valid {
			parent, err := h.queries.GetAgentByID(r.Context(), agent.ParentAgentID.UUID)
			if err == nil {
				pa := dbAgentToTeamAgent(parent)
				resp.Parent = &pa
			}

			siblings, err := h.queries.ListAgentChildrenBySquad(r.Context(), db.ListAgentChildrenBySquadParams{
				SquadID:       identity.SquadID,
				ParentAgentID: uuid.NullUUID{UUID: agent.ParentAgentID.UUID, Valid: true},
			})
			if err == nil {
				sibs := make([]teamAgent, 0, len(siblings))
				for _, s := range siblings {
					if s.ID == identity.AgentID {
						continue // exclude self
					}
					sibs = append(sibs, dbAgentToTeamAgent(s))
				}
				if len(sibs) > 0 {
					resp.Siblings = sibs
				}
			}
		}

		// Lead also gets children
		if identity.Role == "lead" {
			children, err := h.queries.ListAgentChildrenBySquad(r.Context(), db.ListAgentChildrenBySquadParams{
				SquadID:       identity.SquadID,
				ParentAgentID: uuid.NullUUID{UUID: identity.AgentID, Valid: true},
			})
			if err == nil {
				kids := make([]teamAgent, 0, len(children))
				for _, c := range children {
					kids = append(kids, dbAgentToTeamAgent(c))
				}
				if len(kids) > 0 {
					resp.Children = kids
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetBudget returns the agent's spend and budget for the current billing period.
func (h *AgentSelfHandler) GetBudget(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Agent authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	agent, active := h.requireActiveAgent(r.Context(), w, identity.AgentID)
	if !active {
		return
	}

	periodStart, periodEnd := domain.BillingPeriod(time.Now())

	spentCents, err := h.queries.GetAgentMonthlySpend(r.Context(), db.GetAgentMonthlySpendParams{
		AgentID:     identity.AgentID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	})
	if err != nil {
		slog.Error("agent/me/budget: failed to get agent spend", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	resp := budgetResponse{
		SpentCents:  spentCents,
		PeriodStart: periodStart.Format(time.RFC3339),
		PeriodEnd:   periodEnd.Format(time.RFC3339),
	}

	if agent.BudgetMonthlyCents.Valid && agent.BudgetMonthlyCents.Int64 > 0 {
		budgetVal := agent.BudgetMonthlyCents.Int64
		resp.BudgetCents = &budgetVal
		remaining := budgetVal - spentCents
		resp.RemainingCents = &remaining
	}

	threshold, _ := domain.ComputeThreshold(resp.BudgetCents, spentCents)
	resp.ThresholdStatus = string(threshold)

	writeJSON(w, http.StatusOK, resp)
}

// GetGoals returns goals linked to the agent's assigned issues.
func (h *AgentSelfHandler) GetGoals(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Agent authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	if _, active := h.requireActiveAgent(r.Context(), w, identity.AgentID); !active {
		return
	}

	agentIDs, err := h.resolveAgentIDs(r.Context(), identity)
	if err != nil {
		slog.Error("agent/me/goals: failed to resolve agent IDs", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// goalID -> []identifier mapping
	type linkedIssue struct {
		GoalID     uuid.UUID
		Identifier string
	}

	var linkedIssues []linkedIssue

	if agentIDs == nil {
		// Captain: all goal-linked issues in squad
		rows, err := h.queries.ListGoalLinkedIssuesBySquad(r.Context(), identity.SquadID)
		if err != nil {
			slog.Error("agent/me/goals: failed to list goal-linked issues", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		for _, row := range rows {
			if row.GoalID.Valid {
				linkedIssues = append(linkedIssues, linkedIssue{GoalID: row.GoalID.UUID, Identifier: row.Identifier})
			}
		}
	} else if len(agentIDs) == 1 {
		rows, err := h.queries.ListGoalLinkedIssuesByAgent(r.Context(), db.ListGoalLinkedIssuesByAgentParams{
			SquadID: identity.SquadID,
			AgentID: uuid.NullUUID{UUID: agentIDs[0], Valid: true},
		})
		if err != nil {
			slog.Error("agent/me/goals: failed to list goal-linked issues by agent", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		for _, row := range rows {
			if row.GoalID.Valid {
				linkedIssues = append(linkedIssues, linkedIssue{GoalID: row.GoalID.UUID, Identifier: row.Identifier})
			}
		}
	} else {
		rows, err := h.queries.ListGoalLinkedIssuesByAgentIDs(r.Context(), db.ListGoalLinkedIssuesByAgentIDsParams{
			SquadID:  identity.SquadID,
			AgentIds: agentIDs,
		})
		if err != nil {
			slog.Error("agent/me/goals: failed to list goal-linked issues by agent IDs", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		for _, row := range rows {
			if row.GoalID.Valid {
				linkedIssues = append(linkedIssues, linkedIssue{GoalID: row.GoalID.UUID, Identifier: row.Identifier})
			}
		}
	}

	// Collect unique goal IDs
	goalIDSet := make(map[uuid.UUID]struct{})
	goalIssueMap := make(map[uuid.UUID][]string)
	for _, li := range linkedIssues {
		goalIDSet[li.GoalID] = struct{}{}
		goalIssueMap[li.GoalID] = append(goalIssueMap[li.GoalID], li.Identifier)
	}

	if len(goalIDSet) == 0 {
		writeJSON(w, http.StatusOK, goalsResponse{Goals: []goalItem{}})
		return
	}

	goalIDs := make([]uuid.UUID, 0, len(goalIDSet))
	for id := range goalIDSet {
		goalIDs = append(goalIDs, id)
	}

	goals, err := h.queries.GetGoalsByIDs(r.Context(), goalIDs)
	if err != nil {
		slog.Error("agent/me/goals: failed to batch fetch goals", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]goalItem, 0, len(goals))
	for _, g := range goals {
		gi := goalItem{
			ID:            g.ID,
			Title:         g.Title,
			Status:        g.Status,
			RelatedIssues: goalIssueMap[g.ID],
		}
		if g.Description.Valid {
			gi.Description = &g.Description.String
		}
		if gi.RelatedIssues == nil {
			gi.RelatedIssues = []string{}
		}
		items = append(items, gi)
	}

	writeJSON(w, http.StatusOK, goalsResponse{Goals: items})
}

// createInboxRequest is the request body for POST /api/agent/me/inbox.
type createInboxRequest struct {
	Category       string          `json:"category"`
	Title          string          `json:"title"`
	Body           string          `json:"body"`
	Urgency        string          `json:"urgency"`
	RelatedIssueID string          `json:"relatedIssueId"`
	Payload        json.RawMessage `json:"payload"`
}

// CreateInboxItem creates an inbox item on behalf of the agent.
func (h *AgentSelfHandler) CreateInboxItem(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Agent authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	if _, active := h.requireActiveAgent(r.Context(), w, identity.AgentID); !active {
		return
	}

	var req createInboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate category
	switch req.Category {
	case "approval", "question", "decision":
		// valid
	case "alert":
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agents cannot create alert inbox items", Code: "VALIDATION_ERROR"})
		return
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "category must be one of: approval, question, decision", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate title
	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) > 500 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "title is required and must be 1-500 characters", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate urgency
	urgency := req.Urgency
	if urgency == "" {
		urgency = "normal"
	}
	switch urgency {
	case "critical", "normal", "low":
		// valid
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "urgency must be one of: critical, normal, low", Code: "VALIDATION_ERROR"})
		return
	}

	// Build params
	params := db.CreateInboxItemParams{
		SquadID:            identity.SquadID,
		Category:           db.InboxCategory(req.Category),
		Type:               "agent_request",
		Urgency:            db.InboxUrgency(urgency),
		Title:              title,
		RequestedByAgentID: uuid.NullUUID{UUID: identity.AgentID, Valid: true},
		RelatedAgentID:     uuid.NullUUID{UUID: identity.AgentID, Valid: true},
	}

	if req.Body != "" {
		params.Body = sql.NullString{String: req.Body, Valid: true}
	}

	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		params.Payload = req.Payload
	} else {
		params.Payload = json.RawMessage(`{}`)
	}

	// Validate relatedIssueId if provided
	if req.RelatedIssueID != "" {
		issueID, err := uuid.Parse(req.RelatedIssueID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid relatedIssueId", Code: "VALIDATION_ERROR"})
			return
		}
		issue, err := h.queries.GetIssueByID(r.Context(), issueID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Related issue not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("agent/me/inbox: failed to get related issue", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if issue.SquadID != identity.SquadID {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Related issue does not belong to agent's squad", Code: "FORBIDDEN"})
			return
		}
		params.RelatedIssueID = uuid.NullUUID{UUID: issueID, Valid: true}
	}

	// Delegate to InboxService.Create() which handles tx, activity log, and SSE
	item, err := h.inboxService.Create(r.Context(), params)
	if err != nil {
		slog.Error("agent/me/inbox: failed to create inbox item", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusCreated, dbInboxItemToResponse(*item))
}

// reportCostRequest is the request body for POST /api/agent/me/cost.
type reportCostRequest struct {
	AmountCents  int64           `json:"amountCents"`
	EventType    string          `json:"eventType"`
	Model        string          `json:"model"`
	InputTokens  *int64          `json:"inputTokens"`
	OutputTokens *int64          `json:"outputTokens"`
	Metadata     json.RawMessage `json:"metadata"`
}

// ReportCost allows an agent to self-report a cost event from external tool usage.
func (h *AgentSelfHandler) ReportCost(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.AgentFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Agent authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	if _, active := h.requireActiveAgent(r.Context(), w, identity.AgentID); !active {
		return
	}

	var req reportCostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate
	if req.AmountCents <= 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "amountCents must be a positive integer", Code: "VALIDATION_ERROR"})
		return
	}
	if req.AmountCents > 100000 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "amountCents must not exceed 100000", Code: "VALIDATION_ERROR"})
		return
	}

	eventType := strings.TrimSpace(req.EventType)
	if eventType == "" || len(eventType) > 50 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "eventType is required and must be 1-50 characters", Code: "VALIDATION_ERROR"})
		return
	}

	model := strings.TrimSpace(req.Model)
	if model == "" || len(model) > 100 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "model is required and must be 1-100 characters", Code: "VALIDATION_ERROR"})
		return
	}

	if req.InputTokens != nil && *req.InputTokens < 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "inputTokens must be non-negative", Code: "VALIDATION_ERROR"})
		return
	}
	if req.OutputTokens != nil && *req.OutputTokens < 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "outputTokens must be non-negative", Code: "VALIDATION_ERROR"})
		return
	}

	// Build InsertCostEventParams
	params := db.InsertCostEventParams{
		SquadID:     identity.SquadID,
		AgentID:     identity.AgentID,
		AmountCents: req.AmountCents,
		EventType:   eventType,
		Model:       sql.NullString{String: model, Valid: true},
	}

	if req.InputTokens != nil {
		params.InputTokens = sql.NullInt64{Int64: *req.InputTokens, Valid: true}
	}
	if req.OutputTokens != nil {
		params.OutputTokens = sql.NullInt64{Int64: *req.OutputTokens, Valid: true}
	}
	if len(req.Metadata) > 0 && string(req.Metadata) != "null" {
		params.Metadata = pqtype.NullRawMessage{RawMessage: req.Metadata, Valid: true}
	}

	// Delegate to BudgetEnforcementService
	result, err := h.budgetService.RecordAndEnforce(r.Context(), params)
	if err != nil {
		slog.Error("agent/me/cost: failed to record cost", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit SSE event
	if h.sseHub != nil {
		h.sseHub.Publish(identity.SquadID, "cost.event.created", map[string]any{
			"costEventId": result.CostEvent.ID.String(),
			"agentId":     identity.AgentID.String(),
			"amountCents": result.CostEvent.AmountCents,
		})
	}

	// Build response from created cost event
	costResp := map[string]any{
		"id":          result.CostEvent.ID,
		"squadId":     result.CostEvent.SquadID,
		"agentId":     result.CostEvent.AgentID,
		"amountCents": result.CostEvent.AmountCents,
		"eventType":   result.CostEvent.EventType,
		"createdAt":   result.CostEvent.CreatedAt.Format(time.RFC3339),
	}
	if result.CostEvent.Model.Valid {
		costResp["model"] = result.CostEvent.Model.String
	}
	if result.CostEvent.InputTokens.Valid {
		costResp["inputTokens"] = result.CostEvent.InputTokens.Int64
	}
	if result.CostEvent.OutputTokens.Valid {
		costResp["outputTokens"] = result.CostEvent.OutputTokens.Int64
	}
	if result.CostEvent.Metadata.Valid {
		costResp["metadata"] = result.CostEvent.Metadata.RawMessage
	}

	writeJSON(w, http.StatusCreated, costResp)
}

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
