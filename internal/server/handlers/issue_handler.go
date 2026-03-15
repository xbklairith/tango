package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

var identifierPattern = regexp.MustCompile(`^[A-Z]{2,10}-\d+$`)

type IssueHandler struct {
	queries   *db.Queries
	dbConn    *sql.DB
	wakeupSvc *WakeupService
}

func NewIssueHandler(q *db.Queries, dbConn *sql.DB) *IssueHandler {
	return &IssueHandler{queries: q, dbConn: dbConn}
}

// SetWakeupService sets the wakeup service for auto-wake on assignment.
func (h *IssueHandler) SetWakeupService(ws *WakeupService) {
	h.wakeupSvc = ws
}

func (h *IssueHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/squads/{squadId}/issues", h.CreateIssue)
	mux.HandleFunc("GET /api/squads/{squadId}/issues", h.ListIssues)
	mux.HandleFunc("GET /api/issues/{id}", h.GetIssue)
	mux.HandleFunc("PATCH /api/issues/{id}", h.UpdateIssue)
	mux.HandleFunc("DELETE /api/issues/{id}", h.DeleteIssue)
	mux.HandleFunc("POST /api/issues/{issueId}/comments", h.CreateComment)
	mux.HandleFunc("GET /api/issues/{issueId}/comments", h.ListComments)
}

// --- Response Types ---

type issueResponse struct {
	ID              uuid.UUID            `json:"id"`
	SquadID         uuid.UUID            `json:"squadId"`
	Identifier      string               `json:"identifier"`
	Type            domain.IssueType     `json:"type"`
	Title           string               `json:"title"`
	Description     *string              `json:"description"`
	Status          domain.IssueStatus   `json:"status"`
	Priority        domain.IssuePriority `json:"priority"`
	ParentID        *uuid.UUID           `json:"parentId"`
	ProjectID       *uuid.UUID           `json:"projectId"`
	GoalID          *uuid.UUID           `json:"goalId"`
	AssigneeAgentID *uuid.UUID           `json:"assigneeAgentId"`
	AssigneeUserID  *uuid.UUID           `json:"assigneeUserId"`
	BillingCode     *string              `json:"billingCode"`
	RequestDepth    int                  `json:"requestDepth"`
	CreatedAt       string               `json:"createdAt"`
	UpdatedAt       string               `json:"updatedAt"`
}

type paginationMeta struct {
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
	Total  int64 `json:"total"`
}

type issueListResponse struct {
	Data       []issueResponse `json:"data"`
	Pagination paginationMeta  `json:"pagination"`
}

type commentListResponse struct {
	Data       []commentResponse `json:"data"`
	Pagination paginationMeta    `json:"pagination"`
}

type commentResponse struct {
	ID         uuid.UUID                `json:"id"`
	IssueID    uuid.UUID                `json:"issueId"`
	AuthorType domain.CommentAuthorType `json:"authorType"`
	AuthorID   uuid.UUID                `json:"authorId"`
	Body       string                   `json:"body"`
	CreatedAt  string                   `json:"createdAt"`
	UpdatedAt  string                   `json:"updatedAt"`
}

func dbIssueToResponse(i db.Issue) issueResponse {
	resp := issueResponse{
		ID:           i.ID,
		SquadID:      i.SquadID,
		Identifier:   i.Identifier,
		Type:         domain.IssueType(i.Type),
		Title:        i.Title,
		Status:       domain.IssueStatus(i.Status),
		Priority:     domain.IssuePriority(i.Priority),
		RequestDepth: int(i.RequestDepth),
		CreatedAt:    i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    i.UpdatedAt.Format(time.RFC3339),
	}
	if i.Description.Valid {
		resp.Description = &i.Description.String
	}
	if i.ParentID.Valid {
		resp.ParentID = &i.ParentID.UUID
	}
	if i.ProjectID.Valid {
		resp.ProjectID = &i.ProjectID.UUID
	}
	if i.GoalID.Valid {
		resp.GoalID = &i.GoalID.UUID
	}
	if i.AssigneeAgentID.Valid {
		resp.AssigneeAgentID = &i.AssigneeAgentID.UUID
	}
	if i.AssigneeUserID.Valid {
		resp.AssigneeUserID = &i.AssigneeUserID.UUID
	}
	if i.BillingCode.Valid {
		resp.BillingCode = &i.BillingCode.String
	}
	return resp
}

func dbCommentToResponse(c db.IssueComment) commentResponse {
	return commentResponse{
		ID:         c.ID,
		IssueID:    c.IssueID,
		AuthorType: domain.CommentAuthorType(c.AuthorType),
		AuthorID:   c.AuthorID,
		Body:       c.Body,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.Format(time.RFC3339),
	}
}

// --- Squad Membership Helper ---

func (h *IssueHandler) verifySquadMembership(w http.ResponseWriter, r *http.Request, squadID uuid.UUID) (uuid.UUID, bool) {
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return uuid.Nil, false
	}
	_, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return uuid.Nil, false
		}
		slog.Error("failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return uuid.Nil, false
	}
	return identity.UserID, true
}

// --- Handlers ---

func (h *IssueHandler) CreateIssue(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, squadID); !ok {
		return
	}

	var req domain.CreateIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if err := domain.ValidateCreateIssueInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Apply defaults
	issueType := domain.IssueTypeTask
	if req.Type != nil {
		issueType = *req.Type
	}
	issueStatus := domain.IssueStatusBacklog
	if req.Status != nil {
		issueStatus = *req.Status
	}
	issuePriority := domain.IssuePriorityMedium
	if req.Priority != nil {
		issuePriority = *req.Priority
	}
	requestDepth := 0
	if req.RequestDepth != nil {
		requestDepth = *req.RequestDepth
	}

	// Validate parentId if provided
	if req.ParentID != nil {
		parent, err := h.queries.GetIssueByID(r.Context(), *req.ParentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Parent issue not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to get parent issue", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if parent.SquadID != squadID {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Parent issue must belong to the same squad", Code: "VALIDATION_ERROR"})
			return
		}
	}

	// Validate assigneeAgentId belongs to same squad
	if req.AssigneeAgentID != nil {
		agent, err := h.queries.GetAgentByID(r.Context(), *req.AssigneeAgentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Assignee agent not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to get agent", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if agent.SquadID != squadID {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Assignee agent must belong to the same squad", Code: "VALIDATION_ERROR"})
			return
		}
	}

	// Validate assigneeUserId is member of squad
	if req.AssigneeUserID != nil {
		_, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
			UserID:  *req.AssigneeUserID,
			SquadID: squadID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Assignee user is not a member of this squad", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to check assignee membership", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	}

	// Create issue in transaction (atomic identifier generation)
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	// Atomically increment squad counter
	counterRow, err := qtx.IncrementSquadIssueCounter(r.Context(), squadID)
	if err != nil {
		slog.Error("failed to increment issue counter", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	identifier := fmt.Sprintf("%s-%d", counterRow.IssuePrefix, counterRow.IssueCounter)

	// Build params
	params := db.CreateIssueParams{
		SquadID:      squadID,
		Identifier:   identifier,
		Type:         db.IssueType(issueType),
		Title:        req.Title,
		Status:       db.IssueStatus(issueStatus),
		Priority:     db.IssuePriority(issuePriority),
		RequestDepth: int32(requestDepth),
	}
	if req.Description != nil {
		params.Description = sql.NullString{String: *req.Description, Valid: true}
	}
	if req.ParentID != nil {
		params.ParentID = uuid.NullUUID{UUID: *req.ParentID, Valid: true}
	}
	if req.ProjectID != nil {
		params.ProjectID = uuid.NullUUID{UUID: *req.ProjectID, Valid: true}
	}
	if req.GoalID != nil {
		params.GoalID = uuid.NullUUID{UUID: *req.GoalID, Valid: true}
	}
	if req.AssigneeAgentID != nil {
		params.AssigneeAgentID = uuid.NullUUID{UUID: *req.AssigneeAgentID, Valid: true}
	}
	if req.AssigneeUserID != nil {
		params.AssigneeUserID = uuid.NullUUID{UUID: *req.AssigneeUserID, Valid: true}
	}
	if req.BillingCode != nil {
		params.BillingCode = sql.NullString{String: *req.BillingCode, Valid: true}
	}

	issue, err := qtx.CreateIssue(r.Context(), params)
	if err != nil {
		slog.Error("failed to create issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Get actor identity for activity logging
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    identity.UserID,
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

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Auto-wake: if issue was created with an agent assignee, enqueue wakeup
	if h.wakeupSvc != nil && req.AssigneeAgentID != nil {
		ctxMap := map[string]any{"ARI_TASK_ID": issue.ID.String()}
		if _, err := h.wakeupSvc.Enqueue(r.Context(), *req.AssigneeAgentID, squadID, "assignment", ctxMap); err != nil {
			slog.Warn("auto-wake enqueue failed on create", "agent_id", *req.AssigneeAgentID, "error", err)
		}
	}

	slog.Info("issue created", "issue_id", issue.ID, "identifier", issue.Identifier, "squad_id", squadID)
	writeJSON(w, http.StatusCreated, dbIssueToResponse(issue))
}

func (h *IssueHandler) ListIssues(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, squadID); !ok {
		return
	}

	params, err := parseIssueListParams(r, squadID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	dbParams := db.ListIssuesBySquadParams{
		SquadID:    squadID,
		SortField:  params.Sort,
		PageLimit:  int32(params.Limit),
		PageOffset: int32(params.Offset),
	}
	if params.Status != nil {
		dbParams.FilterStatus = db.NullIssueStatus{IssueStatus: db.IssueStatus(*params.Status), Valid: true}
	}
	if params.Priority != nil {
		dbParams.FilterPriority = db.NullIssuePriority{IssuePriority: db.IssuePriority(*params.Priority), Valid: true}
	}
	if params.Type != nil {
		dbParams.FilterType = db.NullIssueType{IssueType: db.IssueType(*params.Type), Valid: true}
	}
	if params.AssigneeAgentID != nil {
		dbParams.FilterAssigneeAgentID = uuid.NullUUID{UUID: *params.AssigneeAgentID, Valid: true}
	}
	if params.AssigneeUserID != nil {
		dbParams.FilterAssigneeUserID = uuid.NullUUID{UUID: *params.AssigneeUserID, Valid: true}
	}
	if params.ProjectID != nil {
		dbParams.FilterProjectID = uuid.NullUUID{UUID: *params.ProjectID, Valid: true}
	}
	if params.GoalID != nil {
		dbParams.FilterGoalID = uuid.NullUUID{UUID: *params.GoalID, Valid: true}
	}
	if params.ParentID != nil {
		dbParams.FilterParentID = uuid.NullUUID{UUID: *params.ParentID, Valid: true}
	}

	issues, err := h.queries.ListIssuesBySquad(r.Context(), dbParams)
	if err != nil {
		slog.Error("failed to list issues", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Count total matching issues for pagination
	countParams := db.CountIssuesBySquadParams{
		SquadID:               dbParams.SquadID,
		FilterStatus:          dbParams.FilterStatus,
		FilterPriority:        dbParams.FilterPriority,
		FilterType:            dbParams.FilterType,
		FilterAssigneeAgentID: dbParams.FilterAssigneeAgentID,
		FilterAssigneeUserID:  dbParams.FilterAssigneeUserID,
		FilterProjectID:       dbParams.FilterProjectID,
		FilterGoalID:          dbParams.FilterGoalID,
		FilterParentID:        dbParams.FilterParentID,
	}
	total, err := h.queries.CountIssuesBySquad(r.Context(), countParams)
	if err != nil {
		slog.Error("failed to count issues", "error", err)
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
			Limit:  params.Limit,
			Offset: params.Offset,
			Total:  total,
		},
	})
}

func (h *IssueHandler) GetIssue(w http.ResponseWriter, r *http.Request) {
	idParam := r.PathValue("id")

	var issue db.Issue
	var err error

	if identifierPattern.MatchString(idParam) {
		// Identifier lookup — search across user's squads
		identity, ok := auth.UserFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
			return
		}

		memberships, err := h.queries.ListSquadMembershipsByUser(r.Context(), identity.UserID)
		if err != nil {
			slog.Error("failed to list user memberships", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}

		found := false
		for _, m := range memberships {
			issue, err = h.queries.GetIssueByIdentifier(r.Context(), db.GetIssueByIdentifierParams{
				SquadID:    m.SquadID,
				Identifier: idParam,
			})
			if err == nil {
				found = true
				break
			}
			if !errors.Is(err, sql.ErrNoRows) {
				slog.Error("failed to get issue by identifier", "error", err)
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
				return
			}
		}
		if !found {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
	} else {
		// UUID lookup
		issueID, parseErr := uuid.Parse(idParam)
		if parseErr != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "not a valid UUID or identifier format", Code: "INVALID_ID"})
			return
		}
		issue, err = h.queries.GetIssueByID(r.Context(), issueID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to get issue", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}

		// Verify squad membership
		if _, ok := h.verifySquadMembership(w, r, issue.SquadID); !ok {
			return
		}
	}

	writeJSON(w, http.StatusOK, dbIssueToResponse(issue))
}

func (h *IssueHandler) UpdateIssue(w http.ResponseWriter, r *http.Request) {
	issueID, ok := parseIssueID(w, r)
	if !ok {
		return
	}

	// Parse raw JSON to detect sentinel keys for nullable field clearing
	var rawBody map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	bodyBytes, err := json.Marshal(rawBody)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}
	var req domain.UpdateIssueRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Detect sentinel fields (present in JSON means "set this field", even if null)
	if _, has := rawBody["description"]; has {
		req.SetDescription = true
	}
	if _, has := rawBody["parentId"]; has {
		req.SetParent = true
	}
	if _, has := rawBody["projectId"]; has {
		req.SetProject = true
	}
	if _, has := rawBody["goalId"]; has {
		req.SetGoal = true
	}
	if _, has := rawBody["assigneeAgentId"]; has {
		req.SetAssigneeAgent = true
	}
	if _, has := rawBody["assigneeUserId"]; has {
		req.SetAssigneeUser = true
	}
	if _, has := rawBody["billingCode"]; has {
		req.SetBillingCode = true
	}

	if err := domain.ValidateUpdateIssueInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch existing issue
	existing, err := h.queries.GetIssueByID(r.Context(), issueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, existing.SquadID); !ok {
		return
	}

	// Status transition validation
	if req.Status != nil {
		if err := domain.ValidateIssueTransition(domain.IssueStatus(existing.Status), *req.Status); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "INVALID_STATUS_TRANSITION"})
			return
		}
	}

	// Validate parentId if changing
	if req.SetParent && req.ParentID != nil {
		if *req.ParentID == issueID {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "An issue cannot be its own parent", Code: "VALIDATION_ERROR"})
			return
		}
		parent, err := h.queries.GetIssueByID(r.Context(), *req.ParentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Parent issue not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to get parent issue", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if parent.SquadID != existing.SquadID {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Parent issue must belong to the same squad", Code: "VALIDATION_ERROR"})
			return
		}
		// Cycle detection: walk up from parent to check if issueID appears
		if wouldCycle, err := h.checkIssueCycle(r.Context(), *req.ParentID, issueID); err != nil {
			slog.Error("cycle check failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		} else if wouldCycle {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Circular parent reference detected", Code: "VALIDATION_ERROR"})
			return
		}
	}

	// Validate assigneeAgentId
	if req.SetAssigneeAgent && req.AssigneeAgentID != nil {
		agent, err := h.queries.GetAgentByID(r.Context(), *req.AssigneeAgentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Assignee agent not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to get agent", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if agent.SquadID != existing.SquadID {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Assignee agent must belong to the same squad", Code: "VALIDATION_ERROR"})
			return
		}
	}

	// Validate assigneeUserId
	if req.SetAssigneeUser && req.AssigneeUserID != nil {
		_, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
			UserID:  *req.AssigneeUserID,
			SquadID: existing.SquadID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Assignee user is not a member of this squad", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to check assignee membership", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	}

	// Build update params
	params := db.UpdateIssueParams{ID: issueID}
	if req.Title != nil {
		params.Title = sql.NullString{String: *req.Title, Valid: true}
	}
	params.SetDescription = req.SetDescription
	if req.SetDescription && req.Description != nil {
		params.Description = sql.NullString{String: *req.Description, Valid: true}
	}
	if req.Type != nil {
		params.Type = db.NullIssueType{IssueType: db.IssueType(*req.Type), Valid: true}
	}
	if req.Status != nil {
		params.Status = db.NullIssueStatus{IssueStatus: db.IssueStatus(*req.Status), Valid: true}
	}
	if req.Priority != nil {
		params.Priority = db.NullIssuePriority{IssuePriority: db.IssuePriority(*req.Priority), Valid: true}
	}
	params.SetParent = req.SetParent
	if req.SetParent && req.ParentID != nil {
		params.ParentID = uuid.NullUUID{UUID: *req.ParentID, Valid: true}
	}
	params.SetProject = req.SetProject
	if req.SetProject && req.ProjectID != nil {
		params.ProjectID = uuid.NullUUID{UUID: *req.ProjectID, Valid: true}
	}
	params.SetGoal = req.SetGoal
	if req.SetGoal && req.GoalID != nil {
		params.GoalID = uuid.NullUUID{UUID: *req.GoalID, Valid: true}
	}
	params.SetAssigneeAgent = req.SetAssigneeAgent
	if req.SetAssigneeAgent && req.AssigneeAgentID != nil {
		params.AssigneeAgentID = uuid.NullUUID{UUID: *req.AssigneeAgentID, Valid: true}
	}
	params.SetAssigneeUser = req.SetAssigneeUser
	if req.SetAssigneeUser && req.AssigneeUserID != nil {
		params.AssigneeUserID = uuid.NullUUID{UUID: *req.AssigneeUserID, Valid: true}
	}
	params.SetBillingCode = req.SetBillingCode
	if req.SetBillingCode && req.BillingCode != nil {
		params.BillingCode = sql.NullString{String: *req.BillingCode, Valid: true}
	}

	// Get actor identity for activity logging
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Use transaction if we need to insert a reopen comment
	isReopen := req.Status != nil && domain.IsReopen(domain.IssueStatus(existing.Status), *req.Status)
	isStatusChange := req.Status != nil && domain.IssueStatus(existing.Status) != *req.Status

	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	updated, err := qtx.UpdateIssue(r.Context(), params)
	if err != nil {
		slog.Error("failed to update issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if isReopen {
		// Insert system comment for reopen
		_, err = qtx.CreateIssueComment(r.Context(), db.CreateIssueCommentParams{
			IssueID:    issueID,
			AuthorType: db.CommentAuthorTypeSystem,
			AuthorID:   uuid.Nil,
			Body:       fmt.Sprintf("Issue reopened: status changed from %s to %s", existing.Status, *req.Status),
		})
		if err != nil {
			slog.Error("failed to create reopen comment", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	}

	// Log activity: status change gets a separate action
	if isStatusChange {
		if err := logActivity(r.Context(), qtx, ActivityParams{
			SquadID:    existing.SquadID,
			ActorType:  domain.ActivityActorUser,
			ActorID:    identity.UserID,
			Action:     "issue.status_changed",
			EntityType: "issue",
			EntityID:   issueID,
			Metadata: map[string]any{
				"from": string(existing.Status),
				"to":   string(*req.Status),
			},
		}); err != nil {
			slog.Error("failed to log activity", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	}

	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    existing.SquadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    identity.UserID,
		Action:     "issue.updated",
		EntityType: "issue",
		EntityID:   issueID,
		Metadata: map[string]any{
			"changedFields": changedFieldNames(rawBody),
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

	// Auto-wake: if assignee agent was set (not cleared), enqueue wakeup
	if h.wakeupSvc != nil && req.SetAssigneeAgent && req.AssigneeAgentID != nil {
		ctxMap := map[string]any{"ARI_TASK_ID": issueID.String()}
		if _, err := h.wakeupSvc.Enqueue(r.Context(), *req.AssigneeAgentID, existing.SquadID, "assignment", ctxMap); err != nil {
			slog.Warn("auto-wake enqueue failed", "agent_id", *req.AssigneeAgentID, "error", err)
		}
	}

	slog.Info("issue updated", "issue_id", issueID)
	writeJSON(w, http.StatusOK, dbIssueToResponse(updated))
}

func (h *IssueHandler) DeleteIssue(w http.ResponseWriter, r *http.Request) {
	issueID, ok := parseIssueID(w, r)
	if !ok {
		return
	}

	existing, err := h.queries.GetIssueByID(r.Context(), issueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, existing.SquadID); !ok {
		return
	}

	// Check for sub-tasks
	subTaskCount, err := h.queries.CountSubTasks(r.Context(), uuid.NullUUID{UUID: issueID, Valid: true})
	if err != nil {
		slog.Error("failed to count sub-tasks", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	if subTaskCount > 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "Cannot delete issue with active sub-tasks", Code: "CONFLICT"})
		return
	}

	if err := h.queries.DeleteIssue(r.Context(), issueID); err != nil {
		slog.Error("failed to delete issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("issue deleted", "issue_id", issueID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "issue deleted"})
}

func (h *IssueHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	issueIDStr := r.PathValue("issueId")
	issueID, err := uuid.Parse(issueIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid issue ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch issue to verify squad membership
	issue, err := h.queries.GetIssueByID(r.Context(), issueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, issue.SquadID); !ok {
		return
	}

	var req domain.CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if req.Body == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "body is required", Code: "VALIDATION_ERROR"})
		return
	}
	if !req.AuthorType.Valid() {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "authorType must be one of: agent, user, system", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate authorId references
	switch req.AuthorType {
	case domain.CommentAuthorAgent:
		agent, err := h.queries.GetAgentByID(r.Context(), req.AuthorID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Referenced agent not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to get agent for comment", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if agent.SquadID != issue.SquadID {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Agent must belong to the same squad", Code: "VALIDATION_ERROR"})
			return
		}
	case domain.CommentAuthorUser:
		_, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
			UserID:  req.AuthorID,
			SquadID: issue.SquadID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Referenced user is not a member of this squad", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to check comment author membership", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	case domain.CommentAuthorSystem:
		// System comments are allowed with any authorId (typically uuid.Nil)
	}

	// Get actor identity for activity logging
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	comment, err := qtx.CreateIssueComment(r.Context(), db.CreateIssueCommentParams{
		IssueID:    issueID,
		AuthorType: db.CommentAuthorType(req.AuthorType),
		AuthorID:   req.AuthorID,
		Body:       req.Body,
	})
	if err != nil {
		slog.Error("failed to create comment", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    issue.SquadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    identity.UserID,
		Action:     "comment.created",
		EntityType: "comment",
		EntityID:   comment.ID,
		Metadata: map[string]any{
			"issueId": issueID.String(),
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

	writeJSON(w, http.StatusCreated, dbCommentToResponse(comment))
}

func (h *IssueHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	issueIDStr := r.PathValue("issueId")
	issueID, err := uuid.Parse(issueIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid issue ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch issue to verify squad membership
	issue, err := h.queries.GetIssueByID(r.Context(), issueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, issue.SquadID); !ok {
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
		IssueID:    issueID,
		PageLimit:  int32(limit),
		PageOffset: int32(offset),
	})
	if err != nil {
		slog.Error("failed to list comments", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	total, err := h.queries.CountIssueComments(r.Context(), issueID)
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

// --- Helpers ---

func parseIssueID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid issue ID", Code: "VALIDATION_ERROR"})
		return uuid.Nil, false
	}
	return id, true
}

func parseIssueListParams(r *http.Request, squadID uuid.UUID) (domain.IssueListParams, error) {
	q := r.URL.Query()
	params := domain.IssueListParams{
		SquadID: squadID,
		Limit:   50,
		Offset:  0,
		Sort:    "created_at",
	}

	if v := q.Get("status"); v != "" {
		s := domain.IssueStatus(v)
		if !s.Valid() {
			return params, fmt.Errorf("invalid status: %s", v)
		}
		params.Status = &s
	}
	if v := q.Get("priority"); v != "" {
		p := domain.IssuePriority(v)
		if !p.Valid() {
			return params, fmt.Errorf("invalid priority: %s", v)
		}
		params.Priority = &p
	}
	if v := q.Get("type"); v != "" {
		t := domain.IssueType(v)
		if !t.Valid() {
			return params, fmt.Errorf("invalid type: %s", v)
		}
		params.Type = &t
	}
	if v := q.Get("assigneeAgentId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid assigneeAgentId")
		}
		params.AssigneeAgentID = &id
	}
	if v := q.Get("assigneeUserId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid assigneeUserId")
		}
		params.AssigneeUserID = &id
	}
	if v := q.Get("projectId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid projectId")
		}
		params.ProjectID = &id
	}
	if v := q.Get("goalId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid goalId")
		}
		params.GoalID = &id
	}
	if v := q.Get("parentId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return params, fmt.Errorf("invalid parentId")
		}
		params.ParentID = &id
	}
	if v := q.Get("sort"); v != "" {
		switch v {
		case "created_at", "updated_at", "priority", "status":
			params.Sort = v
		default:
			return params, fmt.Errorf("invalid sort field: %s", v)
		}
	}
	if v := q.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 {
			return params, fmt.Errorf("invalid limit")
		}
		if limit > 200 {
			limit = 200
		}
		params.Limit = limit
	}
	if v := q.Get("offset"); v != "" {
		offset, err := strconv.Atoi(v)
		if err != nil || offset < 0 {
			return params, fmt.Errorf("invalid offset")
		}
		params.Offset = offset
	}

	return params, nil
}

// checkIssueCycle walks up the ancestor chain from startID to see if targetID appears.
func (h *IssueHandler) checkIssueCycle(ctx context.Context, startID, targetID uuid.UUID) (bool, error) {
	const query = `
WITH RECURSIVE ancestors AS (
    SELECT id, parent_id, 1 AS depth
    FROM issues
    WHERE id = $1

    UNION ALL

    SELECT i.id, i.parent_id, a.depth + 1
    FROM issues i
    JOIN ancestors a ON a.parent_id = i.id
    WHERE a.depth < 100
)
SELECT EXISTS (
    SELECT 1 FROM ancestors WHERE id = $2
) AS would_cycle`

	var wouldCycle bool
	err := h.dbConn.QueryRowContext(ctx, query, startID, targetID).Scan(&wouldCycle)
	return wouldCycle, err
}
