package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// PipelineHandler handles HTTP requests for pipeline and stage CRUD, plus advance/reject.
type PipelineHandler struct {
	queries     *db.Queries
	pipelineSvc *PipelineService
}

// NewPipelineHandler creates a new PipelineHandler.
func NewPipelineHandler(q *db.Queries, pipelineSvc *PipelineService) *PipelineHandler {
	return &PipelineHandler{queries: q, pipelineSvc: pipelineSvc}
}

// RegisterRoutes registers pipeline routes on the mux.
func (h *PipelineHandler) RegisterRoutes(mux *http.ServeMux) {
	// Pipeline CRUD
	mux.HandleFunc("POST /api/squads/{squadId}/pipelines", h.CreatePipeline)
	mux.HandleFunc("GET /api/squads/{squadId}/pipelines", h.ListPipelines)
	mux.HandleFunc("GET /api/pipelines/{id}", h.GetPipeline)
	mux.HandleFunc("PATCH /api/pipelines/{id}", h.UpdatePipeline)
	mux.HandleFunc("DELETE /api/pipelines/{id}", h.DeletePipeline)

	// Stage CRUD
	mux.HandleFunc("POST /api/pipelines/{pipelineId}/stages", h.CreateStage)
	mux.HandleFunc("PATCH /api/pipeline-stages/{id}", h.UpdateStage)
	mux.HandleFunc("DELETE /api/pipeline-stages/{id}", h.DeleteStage)

	// Issue pipeline workflow
	mux.HandleFunc("POST /api/issues/{id}/advance", h.AdvanceIssue)
	mux.HandleFunc("POST /api/issues/{id}/reject", h.RejectIssue)
}

// --- Response Types ---

type pipelineResponse struct {
	ID          uuid.UUID  `json:"id"`
	SquadID     uuid.UUID  `json:"squadId"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	IsActive    bool       `json:"isActive"`
	CreatedAt   string     `json:"createdAt"`
	UpdatedAt   string     `json:"updatedAt"`
}

type stageResponse struct {
	ID              uuid.UUID  `json:"id"`
	PipelineID      uuid.UUID  `json:"pipelineId"`
	Name            string     `json:"name"`
	Description     *string    `json:"description"`
	Position        int        `json:"position"`
	AssignedAgentID *uuid.UUID `json:"assignedAgentId"`
	GateID          *uuid.UUID `json:"gateId"`
	CreatedAt       string     `json:"createdAt"`
	UpdatedAt       string     `json:"updatedAt"`
}

type pipelineWithStagesResponse struct {
	pipelineResponse
	Stages []stageResponse `json:"stages"`
}

type pipelineListResponse struct {
	Data       []pipelineResponse `json:"data"`
	Pagination paginationMeta     `json:"pagination"`
}

func dbPipelineToResponse(p db.Pipeline) pipelineResponse {
	resp := pipelineResponse{
		ID:        p.ID,
		SquadID:   p.SquadID,
		Name:      p.Name,
		IsActive:  p.IsActive,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	}
	if p.Description.Valid {
		resp.Description = &p.Description.String
	}
	return resp
}

func dbStageToResponse(s db.PipelineStage) stageResponse {
	resp := stageResponse{
		ID:         s.ID,
		PipelineID: s.PipelineID,
		Name:       s.Name,
		Position:   int(s.Position),
		CreatedAt:  s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  s.UpdatedAt.Format(time.RFC3339),
	}
	if s.Description.Valid {
		resp.Description = &s.Description.String
	}
	if s.AssignedAgentID.Valid {
		resp.AssignedAgentID = &s.AssignedAgentID.UUID
	}
	if s.GateID.Valid {
		resp.GateID = &s.GateID.UUID
	}
	return resp
}

// --- Pipeline CRUD Endpoints ---

func (h *PipelineHandler) CreatePipeline(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	squadID, err := uuid.Parse(r.PathValue("squadId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	var req domain.CreatePipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if err := domain.ValidateCreatePipelineInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Permission check: pipeline.create
	if !requirePermission(w, r, squadID, auth.ResourcePipeline, auth.ActionCreate, makeRoleLookup(h.queries)) {
		return
	}

	p, err := h.pipelineSvc.CreatePipeline(r.Context(), squadID, caller.ID, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusCreated, dbPipelineToResponse(*p))
}

func (h *PipelineHandler) ListPipelines(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	squadID, err := uuid.Parse(r.PathValue("squadId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Permission check: pipeline.read
	if !requirePermission(w, r, squadID, auth.ResourcePipeline, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	listParams := db.ListPipelinesBySquadParams{
		SquadID:   squadID,
		PageLimit: int32(limit),
		PageOffset: int32(offset),
	}
	countParams := db.CountPipelinesBySquadParams{
		SquadID: squadID,
	}

	if v := r.URL.Query().Get("isActive"); v != "" {
		b := v == "true"
		listParams.FilterIsActive = sql.NullBool{Bool: b, Valid: true}
		countParams.FilterIsActive = sql.NullBool{Bool: b, Valid: true}
	}

	pipelines, err := h.queries.ListPipelinesBySquad(r.Context(), listParams)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	total, err := h.queries.CountPipelinesBySquad(r.Context(), countParams)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	data := make([]pipelineResponse, len(pipelines))
	for i, p := range pipelines {
		data[i] = dbPipelineToResponse(p)
	}

	writeJSON(w, http.StatusOK, pipelineListResponse{
		Data:       data,
		Pagination: paginationMeta{Limit: limit, Offset: offset, Total: total},
	})
}

func (h *PipelineHandler) GetPipeline(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid pipeline ID", Code: "VALIDATION_ERROR"})
		return
	}

	p, err := h.queries.GetPipelineByID(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Pipeline not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: pipeline.read
	if !requirePermission(w, r, p.SquadID, auth.ResourcePipeline, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	stages, err := h.queries.ListStagesByPipeline(r.Context(), p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	stageResps := make([]stageResponse, len(stages))
	for i, s := range stages {
		stageResps[i] = dbStageToResponse(s)
	}

	resp := pipelineWithStagesResponse{
		pipelineResponse: dbPipelineToResponse(p),
		Stages:           stageResps,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *PipelineHandler) UpdatePipeline(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid pipeline ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Parse with raw JSON to detect explicit null for description
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	var req domain.UpdatePipelineRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Detect explicit null for description
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err == nil {
		if _, exists := fields["description"]; exists {
			req.SetDescription = true
		}
	}

	if err := domain.ValidateUpdatePipelineInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Look up pipeline to get squadID for permission check
	existingPipeline, lookupErr := h.queries.GetPipelineByID(r.Context(), id)
	if lookupErr != nil {
		if lookupErr == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Pipeline not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: pipeline.update
	if !requirePermission(w, r, existingPipeline.SquadID, auth.ResourcePipeline, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	p, err := h.pipelineSvc.UpdatePipeline(r.Context(), id, caller.ID, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, dbPipelineToResponse(*p))
}

func (h *PipelineHandler) DeletePipeline(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid pipeline ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Look up pipeline for squadID
	delPipeline, delLookupErr := h.queries.GetPipelineByID(r.Context(), id)
	if delLookupErr != nil {
		if delLookupErr == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Pipeline not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: pipeline.delete
	if !requirePermission(w, r, delPipeline.SquadID, auth.ResourcePipeline, auth.ActionDelete, makeRoleLookup(h.queries)) {
		return
	}

	err = h.pipelineSvc.DeletePipeline(r.Context(), id, caller.ID)
	if err != nil {
		var inUseErr *PipelineInUseError
		if errors.As(err, &inUseErr) {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "PIPELINE_IN_USE"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Stage CRUD Endpoints ---

func (h *PipelineHandler) CreateStage(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	pipelineID, err := uuid.Parse(r.PathValue("pipelineId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid pipeline ID", Code: "VALIDATION_ERROR"})
		return
	}

	var req domain.CreateStageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if err := domain.ValidateCreateStageInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Look up pipeline to get squadID for permission check
	cstagePipeline, cstageLookupErr := h.queries.GetPipelineByID(r.Context(), pipelineID)
	if cstageLookupErr != nil {
		if cstageLookupErr == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Pipeline not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: pipeline.create
	if !requirePermission(w, r, cstagePipeline.SquadID, auth.ResourcePipeline, auth.ActionCreate, makeRoleLookup(h.queries)) {
		return
	}

	stage, err := h.pipelineSvc.CreateStage(r.Context(), pipelineID, req)
	if err != nil {
		var mismatchErr *AgentSquadMismatchError
		if errors.As(err, &mismatchErr) {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "AGENT_SQUAD_MISMATCH"})
			return
		}
		if isUniqueConstraintViolation(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "Stage position already exists in this pipeline", Code: "POSITION_CONFLICT"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusCreated, dbStageToResponse(*stage))
}

func (h *PipelineHandler) UpdateStage(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid stage ID", Code: "VALIDATION_ERROR"})
		return
	}

	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	var req domain.UpdateStageRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Detect explicit null fields
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err == nil {
		if _, exists := fields["description"]; exists {
			req.SetDescription = true
		}
		if _, exists := fields["assignedAgentId"]; exists {
			req.SetAgent = true
		}
	}

	if err := domain.ValidateUpdateStageInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Look up stage → pipeline to get squadID for permission check
	ustageExisting, ustageErr := h.queries.GetPipelineStageByID(r.Context(), id)
	if ustageErr != nil {
		if ustageErr == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Stage not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	ustagePipeline, ustagePipelineErr := h.queries.GetPipelineByID(r.Context(), ustageExisting.PipelineID)
	if ustagePipelineErr != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: pipeline.update
	if !requirePermission(w, r, ustagePipeline.SquadID, auth.ResourcePipeline, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	stage, err := h.pipelineSvc.UpdateStage(r.Context(), id, req)
	if err != nil {
		var mismatchErr *AgentSquadMismatchError
		if errors.As(err, &mismatchErr) {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "AGENT_SQUAD_MISMATCH"})
			return
		}
		if isUniqueConstraintViolation(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "Stage position already exists in this pipeline", Code: "POSITION_CONFLICT"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, dbStageToResponse(*stage))
}

func (h *PipelineHandler) DeleteStage(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid stage ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Look up stage → pipeline to get squadID for permission check
	dstageExisting, dstageErr := h.queries.GetPipelineStageByID(r.Context(), id)
	if dstageErr != nil {
		if dstageErr == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Stage not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	dstagePipeline, dstagePipelineErr := h.queries.GetPipelineByID(r.Context(), dstageExisting.PipelineID)
	if dstagePipelineErr != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: pipeline.delete
	if !requirePermission(w, r, dstagePipeline.SquadID, auth.ResourcePipeline, auth.ActionDelete, makeRoleLookup(h.queries)) {
		return
	}

	err = h.pipelineSvc.DeleteStage(r.Context(), id)
	if err != nil {
		var inUseErr *StageInUseError
		if errors.As(err, &inUseErr) {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "STAGE_IN_USE"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Advance / Reject Endpoints ---

func (h *PipelineHandler) AdvanceIssue(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	issueID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid issue ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Look up issue to get squadID for permission check
	advIssue, advLookupErr := h.queries.GetIssueByID(r.Context(), issueID)
	if advLookupErr != nil {
		if advLookupErr == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: issue.advance
	if !requirePermission(w, r, advIssue.SquadID, auth.ResourceIssue, auth.ActionAdvance, makeRoleLookup(h.queries)) {
		return
	}

	updated, err := h.pipelineSvc.AdvanceStage(r.Context(), issueID, caller.ID)
	if err != nil {
		h.handlePipelineError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dbIssueToResponse(*updated))
}

func (h *PipelineHandler) RejectIssue(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	issueID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid issue ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Look up issue to get squadID for permission check
	rejIssue, rejLookupErr := h.queries.GetIssueByID(r.Context(), issueID)
	if rejLookupErr != nil {
		if rejLookupErr == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: issue.reject
	if !requirePermission(w, r, rejIssue.SquadID, auth.ResourceIssue, auth.ActionReject, makeRoleLookup(h.queries)) {
		return
	}

	var req domain.RejectIssueRequest
	// Body is optional for reject
	_ = json.NewDecoder(r.Body).Decode(&req)

	updated, err := h.pipelineSvc.RejectStage(r.Context(), issueID, caller.ID, req.Reason)
	if err != nil {
		h.handlePipelineError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dbIssueToResponse(*updated))
}

// isUniqueConstraintViolation checks if an error is a PostgreSQL unique constraint violation.
func isUniqueConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "SQLSTATE 23505")
}

// handlePipelineError maps pipeline domain errors to HTTP responses.
func (h *PipelineHandler) handlePipelineError(w http.ResponseWriter, err error) {
	var notInPipeline *NotInPipelineError
	var noPrevStage *NoPreviousStageError
	var concurrentAdv *ConcurrentAdvanceError

	switch {
	case errors.As(err, &notInPipeline):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "NOT_IN_PIPELINE"})
	case errors.As(err, &noPrevStage):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "NO_PREVIOUS_STAGE"})
	case errors.As(err, &concurrentAdv):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error(), Code: "CONCURRENT_ADVANCE"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error(), Code: "INTERNAL_ERROR"})
	}
}
