package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// ConversationHandler provides endpoints for creating and managing conversations.
type ConversationHandler struct {
	queries   *db.Queries
	dbConn    *sql.DB
	wakeupSvc *WakeupService
	sseHub    *sse.Hub
}

// NewConversationHandler creates a new ConversationHandler.
func NewConversationHandler(q *db.Queries, dbConn *sql.DB, ws *WakeupService, hub *sse.Hub) *ConversationHandler {
	return &ConversationHandler{queries: q, dbConn: dbConn, wakeupSvc: ws, sseHub: hub}
}

// RegisterRoutes registers conversation routes on the given mux.
func (h *ConversationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/squads/{squadId}/conversations", h.StartConversation)
	mux.HandleFunc("GET /api/squads/{squadId}/conversations", h.ListConversations)
	mux.HandleFunc("POST /api/conversations/{id}/messages", h.SendMessage)
	mux.HandleFunc("GET /api/conversations/{id}/messages", h.ListMessages)
	mux.HandleFunc("PATCH /api/conversations/{id}/close", h.CloseConversation)
}

// --- Request/Response Types ---

type startConversationRequest struct {
	Title   string    `json:"title"`
	AgentID uuid.UUID `json:"agentId"`
	Message string    `json:"message"`
}

type startConversationResponse struct {
	Conversation issueResponse    `json:"conversation"`
	Message      *commentResponse `json:"message,omitempty"`
}

type sendMessageRequest struct {
	Body string `json:"body"`
}

// --- Handlers ---

// StartConversation creates a new conversation issue assigned to the specified agent.
func (h *ConversationHandler) StartConversation(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	userID, ok := verifySquadAccess(w, r, squadID, h.queries)
	if !ok {
		return
	}

	// Permission check: conversation.create
	if !requirePermission(w, r, squadID, auth.ResourceConversation, auth.ActionCreate, makeRoleLookup(h.queries)) {
		return
	}

	var req startConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "title is required", Code: "VALIDATION_ERROR"})
		return
	}

	if req.AgentID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agentId is required", Code: "VALIDATION_ERROR"})
		return
	}

	// Verify agent exists and belongs to the same squad
	agent, err := h.queries.GetAgentByID(r.Context(), req.AgentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	if agent.SquadID != squadID {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Agent must belong to the same squad", Code: "VALIDATION_ERROR"})
		return
	}

	// Begin transaction
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	// Atomically increment squad counter for identifier
	counterRow, err := qtx.IncrementSquadIssueCounter(r.Context(), squadID)
	if err != nil {
		slog.Error("failed to increment issue counter", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	identifier := fmt.Sprintf("%s-%d", counterRow.IssuePrefix, counterRow.IssueCounter)

	// Create conversation issue
	issue, err := qtx.CreateIssue(r.Context(), db.CreateIssueParams{
		SquadID:        squadID,
		Identifier:     identifier,
		Type:           db.IssueTypeConversation,
		Title:          req.Title,
		Status:         db.IssueStatusInProgress,
		Priority:       db.IssuePriorityMedium,
		AssigneeAgentID: uuid.NullUUID{UUID: req.AgentID, Valid: true},
	})
	if err != nil {
		slog.Error("failed to create conversation issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Create first comment if message provided
	var firstComment *commentResponse
	if req.Message != "" {
		authorType := db.CommentAuthorTypeUser
		if caller, ok := auth.CallerFromContext(r.Context()); ok && caller.IsAgent() {
			authorType = db.CommentAuthorTypeAgent
		}
		comment, err := qtx.CreateIssueComment(r.Context(), db.CreateIssueCommentParams{
			IssueID:    issue.ID,
			AuthorType: authorType,
			AuthorID:   userID,
			Body:       req.Message,
		})
		if err != nil {
			slog.Error("failed to create first comment", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		cr := dbCommentToResponse(comment)
		firstComment = &cr
	}

	// Log activity
	actorType, actorID := resolveActor(r.Context())
	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     "conversation.created",
		EntityType: "issue",
		EntityID:   issue.ID,
		Metadata: map[string]any{
			"identifier": issue.Identifier,
			"title":      issue.Title,
			"agentId":    req.AgentID.String(),
		},
	}); err != nil {
		slog.Error("failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit SSE event for initial message
	if firstComment != nil && h.sseHub != nil {
		h.sseHub.Publish(squadID, "conversation.message", map[string]any{
			"conversationId": issue.ID.String(),
			"commentId":      firstComment.ID,
			"authorType":     "user",
			"authorId":       userID.String(),
			"body":           req.Message,
		})
	}

	// If a message was created, enqueue wakeup (skip if agent is paused/error/terminated)
	if firstComment != nil && h.wakeupSvc != nil {
		if agent.Status == db.AgentStatusPaused || agent.Status == db.AgentStatusError || agent.Status == db.AgentStatusTerminated {
			slog.Warn("skipping wakeup enqueue: agent not in wakeable status", "agent_id", req.AgentID, "status", agent.Status)
		} else {
			ctxMap := map[string]any{
				"ARI_CONVERSATION_ID": issue.ID.String(),
			}
			if _, err := h.wakeupSvc.Enqueue(r.Context(), req.AgentID, squadID, "conversation_message", ctxMap); err != nil {
				slog.Warn("wakeup enqueue failed for conversation", "agent_id", req.AgentID, "error", err)
			}
		}
	}

	slog.Info("conversation created", "issue_id", issue.ID, "identifier", issue.Identifier, "squad_id", squadID)
	writeJSON(w, http.StatusCreated, startConversationResponse{
		Conversation: dbIssueToResponse(issue),
		Message:      firstComment,
	})
}

// SendMessage creates a new message (comment) on a conversation and enqueues a wakeup.
func (h *ConversationHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	convIDStr := r.PathValue("id")
	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid conversation ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch issue
	issue, err := h.queries.GetIssueByID(r.Context(), convID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Conversation not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify type=conversation
	if issue.Type != db.IssueTypeConversation {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "Issue is not a conversation", Code: "NOT_A_CONVERSATION"})
		return
	}

	// Verify not closed
	if issue.Status == db.IssueStatusDone || issue.Status == db.IssueStatusCancelled {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "Conversation is closed", Code: "CONVERSATION_CLOSED"})
		return
	}

	// Verify user is squad member
	userID, ok := verifySquadAccess(w, r, issue.SquadID, h.queries)
	if !ok {
		return
	}

	// Permission check: conversation.update
	if !requirePermission(w, r, issue.SquadID, auth.ResourceConversation, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	// Verify agent assigned
	if !issue.AssigneeAgentID.Valid {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "No agent assigned to conversation", Code: "NO_AGENT_ASSIGNED"})
		return
	}

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if req.Body == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "body is required", Code: "VALIDATION_ERROR"})
		return
	}

	if len(req.Body) > 50000 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "message body exceeds maximum length", Code: "VALIDATION_ERROR"})
		return
	}

	// Begin transaction
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	commentAuthorType := db.CommentAuthorTypeUser
	if caller, ok := auth.CallerFromContext(r.Context()); ok && caller.IsAgent() {
		commentAuthorType = db.CommentAuthorTypeAgent
	}
	comment, err := qtx.CreateIssueComment(r.Context(), db.CreateIssueCommentParams{
		IssueID:    convID,
		AuthorType: commentAuthorType,
		AuthorID:   userID,
		Body:       req.Body,
	})
	if err != nil {
		slog.Error("failed to create comment", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Log activity
	msgActorType, msgActorID := resolveActor(r.Context())
	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    issue.SquadID,
		ActorType:  msgActorType,
		ActorID:    msgActorID,
		Action:     "conversation.message_sent",
		EntityType: "comment",
		EntityID:   comment.ID,
		Metadata: map[string]any{
			"conversationId": convID.String(),
		},
	}); err != nil {
		slog.Error("failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit SSE event
	if h.sseHub != nil {
		h.sseHub.Publish(issue.SquadID, "conversation.message", map[string]any{
			"conversationId": convID.String(),
			"commentId":      comment.ID.String(),
			"authorType":     "user",
			"authorId":       userID.String(),
			"body":           req.Body,
		})
	}

	// Enqueue wakeup for assigned agent (skip if agent is paused/error/terminated)
	if h.wakeupSvc != nil {
		agentForWakeup, err := h.queries.GetAgentByID(r.Context(), issue.AssigneeAgentID.UUID)
		if err != nil {
			slog.Warn("failed to load agent for wakeup check", "agent_id", issue.AssigneeAgentID.UUID, "error", err)
		} else if agentForWakeup.Status == db.AgentStatusPaused || agentForWakeup.Status == db.AgentStatusError || agentForWakeup.Status == db.AgentStatusTerminated {
			slog.Warn("skipping wakeup enqueue: agent not in wakeable status", "agent_id", issue.AssigneeAgentID.UUID, "status", agentForWakeup.Status)
		} else {
			ctxMap := map[string]any{
				"ARI_CONVERSATION_ID": convID.String(),
			}
			if _, err := h.wakeupSvc.Enqueue(r.Context(), issue.AssigneeAgentID.UUID, issue.SquadID, "conversation_message", ctxMap); err != nil {
				slog.Warn("wakeup enqueue failed for conversation message", "agent_id", issue.AssigneeAgentID.UUID, "error", err)
			}
		}
	}

	writeJSON(w, http.StatusCreated, dbCommentToResponse(comment))
}

// ListConversations returns conversation issues for a squad with optional filtering and pagination.
func (h *ConversationHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := verifySquadAccess(w, r, squadID, h.queries); !ok {
		return
	}

	// Permission check: conversation.read
	if !requirePermission(w, r, squadID, auth.ResourceConversation, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	// Parse pagination
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l >= 1 {
			limit = l
			if limit > 200 {
				limit = 200
			}
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}

	// Build params — always filter by type=conversation
	dbParams := db.ListIssuesBySquadParams{
		SquadID:    squadID,
		FilterType: db.NullIssueType{IssueType: db.IssueTypeConversation, Valid: true},
		SortField:  "updated_at",
		PageLimit:  int32(limit),
		PageOffset: int32(offset),
	}

	// Optional status filter
	if v := r.URL.Query().Get("status"); v != "" {
		s := domain.IssueStatus(v)
		if s.Valid() {
			dbParams.FilterStatus = db.NullIssueStatus{IssueStatus: db.IssueStatus(s), Valid: true}
		}
	}

	issues, err := h.queries.ListIssuesBySquad(r.Context(), dbParams)
	if err != nil {
		slog.Error("failed to list conversations", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Count total
	countParams := db.CountIssuesBySquadParams{
		SquadID:    squadID,
		FilterType: dbParams.FilterType,
		FilterStatus: dbParams.FilterStatus,
	}
	total, err := h.queries.CountIssuesBySquad(r.Context(), countParams)
	if err != nil {
		slog.Error("failed to count conversations", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	data := make([]issueResponse, 0, len(issues))
	for _, i := range issues {
		data = append(data, dbIssueToResponse(i))
	}
	writeJSON(w, http.StatusOK, issueListResponse{
		Data: data,
		Pagination: paginationMeta{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	})
}

// ListMessages returns comments on a conversation issue with pagination.
func (h *ConversationHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	convIDStr := r.PathValue("id")
	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid conversation ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch issue to verify it exists and check squad membership
	issue, err := h.queries.GetIssueByID(r.Context(), convID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Conversation not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify type=conversation
	if issue.Type != db.IssueTypeConversation {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "Issue is not a conversation", Code: "NOT_A_CONVERSATION"})
		return
	}

	if _, ok := verifySquadAccess(w, r, issue.SquadID, h.queries); !ok {
		return
	}

	// Parse pagination
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l >= 1 {
			limit = l
			if limit > 200 {
				limit = 200
			}
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}

	comments, err := h.queries.ListIssueComments(r.Context(), db.ListIssueCommentsParams{
		IssueID:    convID,
		PageLimit:  int32(limit),
		PageOffset: int32(offset),
	})
	if err != nil {
		slog.Error("failed to list comments", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	total, err := h.queries.CountIssueComments(r.Context(), convID)
	if err != nil {
		slog.Error("failed to count comments", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	data := make([]commentResponse, 0, len(comments))
	for _, c := range comments {
		data = append(data, dbCommentToResponse(c))
	}
	writeJSON(w, http.StatusOK, commentListResponse{
		Data: data,
		Pagination: paginationMeta{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	})
}

// CloseConversation transitions a conversation's status to done.
func (h *ConversationHandler) CloseConversation(w http.ResponseWriter, r *http.Request) {
	convIDStr := r.PathValue("id")
	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid conversation ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch issue
	issue, err := h.queries.GetIssueByID(r.Context(), convID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Conversation not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify type=conversation
	if issue.Type != db.IssueTypeConversation {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "Issue is not a conversation", Code: "NOT_A_CONVERSATION"})
		return
	}

	// Verify squad membership
	_, ok := verifySquadAccess(w, r, issue.SquadID, h.queries)
	if !ok {
		return
	}

	// Permission check: conversation.update
	if !requirePermission(w, r, issue.SquadID, auth.ResourceConversation, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	// Validate status transition
	if err := domain.ValidateIssueTransition(domain.IssueStatus(issue.Status), domain.IssueStatusDone); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "INVALID_STATUS_TRANSITION"})
		return
	}

	// Update in transaction
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	updated, err := qtx.UpdateIssue(r.Context(), db.UpdateIssueParams{
		ID:     convID,
		Status: db.NullIssueStatus{IssueStatus: db.IssueStatusDone, Valid: true},
	})
	if err != nil {
		slog.Error("failed to update issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Log activity
	closeActorType, closeActorID := resolveActor(r.Context())
	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    issue.SquadID,
		ActorType:  closeActorType,
		ActorID:    closeActorID,
		Action:     "conversation.closed",
		EntityType: "issue",
		EntityID:   convID,
		Metadata: map[string]any{
			"identifier": issue.Identifier,
			"title":      issue.Title,
		},
	}); err != nil {
		slog.Error("failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit SSE event
	if h.sseHub != nil {
		h.sseHub.Publish(issue.SquadID, "issue.updated", map[string]any{
			"issueId": convID.String(),
			"status":  string(db.IssueStatusDone),
		})
	}

	writeJSON(w, http.StatusOK, dbIssueToResponse(updated))
}
