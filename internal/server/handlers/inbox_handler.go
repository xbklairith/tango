package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// InboxHandler serves the inbox REST API.
type InboxHandler struct {
	queries      *db.Queries
	dbConn       *sql.DB
	inboxService *InboxService
}

// NewInboxHandler creates a new InboxHandler.
func NewInboxHandler(q *db.Queries, dbConn *sql.DB, inboxSvc *InboxService) *InboxHandler {
	return &InboxHandler{queries: q, dbConn: dbConn, inboxService: inboxSvc}
}

// RegisterRoutes registers inbox HTTP routes on the given mux.
func (h *InboxHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/squads/{id}/inbox", h.CreateInboxItem)
	mux.HandleFunc("GET /api/squads/{id}/inbox", h.ListInboxItems)
	mux.HandleFunc("GET /api/squads/{id}/inbox/count", h.GetInboxCount)
	mux.HandleFunc("GET /api/inbox/{id}", h.GetInboxItem)
	mux.HandleFunc("PATCH /api/inbox/{id}/resolve", h.ResolveInboxItem)
	mux.HandleFunc("PATCH /api/inbox/{id}/acknowledge", h.AcknowledgeInboxItem)
	mux.HandleFunc("PATCH /api/inbox/{id}/dismiss", h.DismissInboxItem)
}

// --- Response Types ---

type inboxItemResponse struct {
	ID                   uuid.UUID  `json:"id"`
	SquadID              uuid.UUID  `json:"squadId"`
	Category             string     `json:"category"`
	Type                 string     `json:"type"`
	Status               string     `json:"status"`
	Urgency              string     `json:"urgency"`
	Title                string     `json:"title"`
	Body                 *string    `json:"body"`
	Payload              any        `json:"payload"`
	RequestedByAgentID   *uuid.UUID `json:"requestedByAgentId"`
	RelatedAgentID       *uuid.UUID `json:"relatedAgentId"`
	RelatedIssueID       *uuid.UUID `json:"relatedIssueId"`
	RelatedRunID         *uuid.UUID `json:"relatedRunId"`
	Resolution           *string    `json:"resolution"`
	ResponseNote         *string    `json:"responseNote"`
	ResponsePayload      any        `json:"responsePayload"`
	ResolvedByUserID     *uuid.UUID `json:"resolvedByUserId"`
	ResolvedAt           *string    `json:"resolvedAt"`
	AcknowledgedByUserID *uuid.UUID `json:"acknowledgedByUserId"`
	AcknowledgedAt       *string    `json:"acknowledgedAt"`
	CreatedAt            string     `json:"createdAt"`
	UpdatedAt            string     `json:"updatedAt"`
}

type inboxListResponse struct {
	Data       []inboxItemResponse `json:"data"`
	Pagination paginationMeta      `json:"pagination"`
}

type inboxCountResponse struct {
	Pending      int64 `json:"pending"`
	Acknowledged int64 `json:"acknowledged"`
	Total        int64 `json:"total"`
}

func dbInboxItemToResponse(item db.InboxItem) inboxItemResponse {
	resp := inboxItemResponse{
		ID:        item.ID,
		SquadID:   item.SquadID,
		Category:  string(item.Category),
		Type:      item.Type,
		Status:    string(item.Status),
		Urgency:   string(item.Urgency),
		Title:     item.Title,
		CreatedAt: item.CreatedAt.Format(time.RFC3339),
		UpdatedAt: item.UpdatedAt.Format(time.RFC3339),
	}

	// Nullable string fields.
	if item.Body.Valid {
		resp.Body = &item.Body.String
	}
	if item.ResponseNote.Valid {
		resp.ResponseNote = &item.ResponseNote.String
	}

	// Nullable UUID fields.
	if item.RequestedByAgentID.Valid {
		resp.RequestedByAgentID = &item.RequestedByAgentID.UUID
	}
	if item.RelatedAgentID.Valid {
		resp.RelatedAgentID = &item.RelatedAgentID.UUID
	}
	if item.RelatedIssueID.Valid {
		resp.RelatedIssueID = &item.RelatedIssueID.UUID
	}
	if item.RelatedRunID.Valid {
		resp.RelatedRunID = &item.RelatedRunID.UUID
	}
	if item.ResolvedByUserID.Valid {
		resp.ResolvedByUserID = &item.ResolvedByUserID.UUID
	}
	if item.AcknowledgedByUserID.Valid {
		resp.AcknowledgedByUserID = &item.AcknowledgedByUserID.UUID
	}

	// Nullable enum fields.
	if item.Resolution.Valid {
		s := string(item.Resolution.InboxResolution)
		resp.Resolution = &s
	}

	// Nullable timestamp fields.
	if item.ResolvedAt.Valid {
		s := item.ResolvedAt.Time.Format(time.RFC3339)
		resp.ResolvedAt = &s
	}
	if item.AcknowledgedAt.Valid {
		s := item.AcknowledgedAt.Time.Format(time.RFC3339)
		resp.AcknowledgedAt = &s
	}

	// Payload (json.RawMessage → any).
	if len(item.Payload) > 0 {
		var parsed any
		if err := json.Unmarshal(item.Payload, &parsed); err == nil {
			resp.Payload = parsed
		} else {
			resp.Payload = map[string]any{}
		}
	} else {
		resp.Payload = map[string]any{}
	}

	// Response payload (pqtype.NullRawMessage → any).
	if item.ResponsePayload.Valid && len(item.ResponsePayload.RawMessage) > 0 {
		var parsed any
		if err := json.Unmarshal(item.ResponsePayload.RawMessage, &parsed); err == nil {
			resp.ResponsePayload = parsed
		}
	}

	return resp
}

// --- Request Types ---

type createInboxItemRequest struct {
	Category       string          `json:"category"`
	Type           string          `json:"type"`
	Title          string          `json:"title"`
	Body           *string         `json:"body,omitempty"`
	Urgency        *string         `json:"urgency,omitempty"`
	RelatedIssueID *string         `json:"relatedIssueId,omitempty"`
	RelatedAgentID *string         `json:"relatedAgentId,omitempty"`
	RelatedRunID   *string         `json:"relatedRunId,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
}

type resolveInboxItemRequest struct {
	Resolution      string          `json:"resolution"`
	ResponseNote    string          `json:"responseNote,omitempty"`
	ResponsePayload json.RawMessage `json:"responsePayload,omitempty"`
}

type dismissInboxItemRequest struct {
	ResponseNote string `json:"responseNote,omitempty"`
}

// --- Handlers ---

// CreateInboxItem handles POST /api/squads/{id}/inbox.
// Accepts both user session and agent Run Token authentication.
func (h *InboxHandler) CreateInboxItem(w http.ResponseWriter, r *http.Request) {
	// Parse squad ID from URL.
	squadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "NOT_FOUND"})
		return
	}

	// Parse request body.
	var req createInboxItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Build domain validation request.
	urgency := domain.InboxUrgencyNormal
	if req.Urgency != nil {
		urgency = domain.InboxUrgency(*req.Urgency)
	}

	domainReq := domain.CreateInboxItemRequest{
		Category: domain.InboxCategory(req.Category),
		Type:     req.Type,
		Title:    req.Title,
		Urgency:  &urgency,
		Payload:  req.Payload,
	}
	if req.Body != nil {
		domainReq.Body = req.Body
	}

	if err := domain.ValidateCreateInboxItemRequest(domainReq); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Build DB params.
	params := db.CreateInboxItemParams{
		SquadID:  squadID,
		Category: db.InboxCategory(req.Category),
		Type:     req.Type,
		Urgency:  db.InboxUrgency(urgency),
		Title:    req.Title,
	}

	if req.Body != nil {
		params.Body = sql.NullString{String: *req.Body, Valid: true}
	}

	if len(req.Payload) > 0 {
		params.Payload = req.Payload
	} else {
		params.Payload = json.RawMessage(`{}`)
	}

	// Parse optional related IDs.
	if req.RelatedIssueID != nil {
		id, err := uuid.Parse(*req.RelatedIssueID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid relatedIssueId", Code: "VALIDATION_ERROR"})
			return
		}
		params.RelatedIssueID = uuid.NullUUID{UUID: id, Valid: true}
	}
	if req.RelatedAgentID != nil {
		id, err := uuid.Parse(*req.RelatedAgentID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid relatedAgentId", Code: "VALIDATION_ERROR"})
			return
		}
		params.RelatedAgentID = uuid.NullUUID{UUID: id, Valid: true}
	}
	if req.RelatedRunID != nil {
		id, err := uuid.Parse(*req.RelatedRunID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid relatedRunId", Code: "VALIDATION_ERROR"})
			return
		}
		params.RelatedRunID = uuid.NullUUID{UUID: id, Valid: true}
	}

	// Determine auth type: agent or user.
	agentIdentity, isAgent := auth.AgentFromContext(r.Context())
	userIdentity, isUser := auth.UserFromContext(r.Context())

	if isAgent {
		// Agent auth: verify squad scope.
		if agentIdentity.SquadID != squadID {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Agent does not belong to this squad", Code: "FORBIDDEN"})
			return
		}
		params.RequestedByAgentID = uuid.NullUUID{UUID: agentIdentity.AgentID, Valid: true}
	} else if isUser {
		// User auth: verify squad membership.
		_, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
			UserID:  userIdentity.UserID,
			SquadID: squadID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
				return
			}
			slog.Error("inbox create: failed to check squad membership", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	} else {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Permission check: inbox.create
	if !requirePermission(w, r, squadID, auth.ResourceInbox, auth.ActionCreate, makeRoleLookup(h.queries)) {
		return
	}

	item, err := h.inboxService.Create(r.Context(), params)
	if err != nil {
		slog.Error("inbox create: service error", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusCreated, dbInboxItemToResponse(*item))
}

// ListInboxItems handles GET /api/squads/{id}/inbox.
func (h *InboxHandler) ListInboxItems(w http.ResponseWriter, r *http.Request) {
	// Parse squad ID.
	squadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "NOT_FOUND"})
		return
	}

	// Auth + membership check.
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}
	_, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return
		}
		slog.Error("inbox list: failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: inbox.read
	if !requirePermission(w, r, squadID, auth.ResourceInbox, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	// Parse pagination.
	q := r.URL.Query()
	limit, offset := 50, 0
	if v := q.Get("limit"); v != "" {
		if l, e := strconv.Atoi(v); e == nil && l >= 1 {
			if l > 200 {
				l = 200
			}
			limit = l
		}
	}
	if v := q.Get("offset"); v != "" {
		if o, e := strconv.Atoi(v); e == nil && o >= 0 {
			offset = o
		}
	}

	// Build filter params.
	listParams := db.ListInboxItemsBySquadParams{
		SquadID:    squadID,
		PageLimit:  int32(limit),
		PageOffset: int32(offset),
	}
	countParams := db.CountInboxItemsBySquadParams{
		SquadID: squadID,
	}

	// Optional category filter.
	if v := q.Get("category"); v != "" {
		cat := domain.InboxCategory(v)
		if !cat.Valid() {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid category filter", Code: "VALIDATION_ERROR"})
			return
		}
		listParams.FilterCategory = db.NullInboxCategory{InboxCategory: db.InboxCategory(v), Valid: true}
		countParams.FilterCategory = listParams.FilterCategory
	}

	// Optional urgency filter.
	if v := q.Get("urgency"); v != "" {
		urg := domain.InboxUrgency(v)
		if !urg.Valid() {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid urgency filter", Code: "VALIDATION_ERROR"})
			return
		}
		listParams.FilterUrgency = db.NullInboxUrgency{InboxUrgency: db.InboxUrgency(v), Valid: true}
		countParams.FilterUrgency = listParams.FilterUrgency
	}

	// Optional status filter.
	if v := q.Get("status"); v != "" {
		stat := domain.InboxStatus(v)
		if !stat.Valid() {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid status filter", Code: "VALIDATION_ERROR"})
			return
		}
		listParams.FilterStatus = db.NullInboxStatus{InboxStatus: db.InboxStatus(v), Valid: true}
		countParams.FilterStatus = listParams.FilterStatus
	}

	// Query items.
	rows, err := h.queries.ListInboxItemsBySquad(r.Context(), listParams)
	if err != nil {
		slog.Error("inbox list: query failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	total, err := h.queries.CountInboxItemsBySquad(r.Context(), countParams)
	if err != nil {
		slog.Error("inbox list: count failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Map to response.
	data := make([]inboxItemResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, dbInboxItemToResponse(row))
	}

	writeJSON(w, http.StatusOK, inboxListResponse{
		Data:       data,
		Pagination: paginationMeta{Limit: limit, Offset: offset, Total: total},
	})
}

// GetInboxItem handles GET /api/inbox/{id}.
func (h *InboxHandler) GetInboxItem(w http.ResponseWriter, r *http.Request) {
	// Parse item ID.
	itemID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
		return
	}

	// Auth check.
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Fetch item.
	item, err := h.queries.GetInboxItemByID(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("inbox get: query failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify squad membership.
	_, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: item.SquadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return
		}
		slog.Error("inbox get: failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, dbInboxItemToResponse(item))
}

// GetInboxCount handles GET /api/squads/{id}/inbox/count.
func (h *InboxHandler) GetInboxCount(w http.ResponseWriter, r *http.Request) {
	// Parse squad ID.
	squadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "NOT_FOUND"})
		return
	}

	// Auth + membership check.
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}
	_, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return
		}
		slog.Error("inbox count: failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	counts, err := h.queries.CountUnresolvedBySquad(r.Context(), squadID)
	if err != nil {
		slog.Error("inbox count: query failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, inboxCountResponse{
		Pending:      counts.PendingCount,
		Acknowledged: counts.AcknowledgedCount,
		Total:        counts.TotalCount,
	})
}

// ResolveInboxItem handles PATCH /api/inbox/{id}/resolve.
func (h *InboxHandler) ResolveInboxItem(w http.ResponseWriter, r *http.Request) {
	// Parse item ID.
	itemID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
		return
	}

	// Auth check.
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Fetch item to verify squad membership.
	existing, err := h.queries.GetInboxItemByID(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("inbox resolve: failed to get item", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify squad membership.
	_, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: existing.SquadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return
		}
		slog.Error("inbox resolve: failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: inbox.resolve
	if !requirePermission(w, r, existing.SquadID, auth.ResourceInbox, auth.ActionResolve, makeRoleLookup(h.queries)) {
		return
	}

	// Parse request body.
	var req resolveInboxItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if req.Resolution == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "resolution is required", Code: "VALIDATION_ERROR"})
		return
	}

	resolution := domain.InboxResolution(req.Resolution)
	if !resolution.Valid() {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid resolution value", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate resolution against category.
	if !domain.IsValidResolutionForCategory(domain.InboxCategory(existing.Category), resolution) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Resolution not valid for this item's category", Code: "INVALID_RESOLUTION"})
		return
	}

	// Validate responsePayload is valid JSON if provided.
	if len(req.ResponsePayload) > 0 {
		if !json.Valid(req.ResponsePayload) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON in responsePayload", Code: "VALIDATION_ERROR"})
			return
		}
	}

	item, err := h.inboxService.Resolve(r.Context(), itemID, identity.UserID, resolution, req.ResponseNote, req.ResponsePayload)
	if err != nil {
		if errors.Is(err, domain.ErrInboxAlreadyResolved) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "Inbox item is already resolved", Code: "ALREADY_RESOLVED"})
			return
		}
		if errors.Is(err, domain.ErrInboxInvalidTransition) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error(), Code: "INVALID_TRANSITION"})
			return
		}
		if errors.Is(err, domain.ErrInboxInvalidResolution) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "INVALID_RESOLUTION"})
			return
		}
		if errors.Is(err, domain.ErrInboxNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("inbox resolve: service error", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, dbInboxItemToResponse(*item))
}

// AcknowledgeInboxItem handles PATCH /api/inbox/{id}/acknowledge.
func (h *InboxHandler) AcknowledgeInboxItem(w http.ResponseWriter, r *http.Request) {
	// Parse item ID.
	itemID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
		return
	}

	// Auth check.
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Fetch item to verify squad membership.
	existing, err := h.queries.GetInboxItemByID(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("inbox acknowledge: failed to get item", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify squad membership.
	_, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: existing.SquadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return
		}
		slog.Error("inbox acknowledge: failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: inbox.update
	if !requirePermission(w, r, existing.SquadID, auth.ResourceInbox, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	item, err := h.inboxService.Acknowledge(r.Context(), itemID, identity.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrInboxNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
			return
		}
		if errors.Is(err, domain.ErrInboxInvalidTransition) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "Inbox item is not in pending status", Code: "ALREADY_ACKNOWLEDGED"})
			return
		}
		slog.Error("inbox acknowledge: service error", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, dbInboxItemToResponse(*item))
}

// DismissInboxItem handles PATCH /api/inbox/{id}/dismiss.
// This is a convenience endpoint for alert items only.
func (h *InboxHandler) DismissInboxItem(w http.ResponseWriter, r *http.Request) {
	// Parse item ID.
	itemID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
		return
	}

	// Auth check.
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Fetch item to verify squad membership and category.
	existing, err := h.queries.GetInboxItemByID(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("inbox dismiss: failed to get item", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Validate category is alert.
	if existing.Category != db.InboxCategoryAlert {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Only alert items can be dismissed", Code: "INVALID_RESOLUTION"})
		return
	}

	// Verify squad membership.
	_, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: existing.SquadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return
		}
		slog.Error("inbox dismiss: failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: inbox.resolve (dismiss is a form of resolution)
	if !requirePermission(w, r, existing.SquadID, auth.ResourceInbox, auth.ActionResolve, makeRoleLookup(h.queries)) {
		return
	}

	// Parse optional body for responseNote.
	var req dismissInboxItemRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Non-critical: dismiss can work without a body.
			slog.Warn("inbox dismiss: failed to parse body", "error", err)
		}
	}

	// Delegate to Resolve with dismissed resolution.
	item, err := h.inboxService.Resolve(r.Context(), itemID, identity.UserID, domain.InboxResolutionDismissed, req.ResponseNote, nil)
	if err != nil {
		if errors.Is(err, domain.ErrInboxAlreadyResolved) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "Inbox item is already resolved", Code: "ALREADY_RESOLVED"})
			return
		}
		if errors.Is(err, domain.ErrInboxInvalidTransition) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error(), Code: "INVALID_TRANSITION"})
			return
		}
		if errors.Is(err, domain.ErrInboxNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Inbox item not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("inbox dismiss: service error", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, dbInboxItemToResponse(*item))
}
