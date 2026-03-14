package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// ProjectHandler handles project CRUD operations.
type ProjectHandler struct {
	queries *db.Queries
}

// NewProjectHandler creates a new ProjectHandler.
func NewProjectHandler(q *db.Queries) *ProjectHandler {
	return &ProjectHandler{queries: q}
}

// RegisterRoutes registers project routes on the given mux.
func (h *ProjectHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/squads/{squadId}/projects", h.CreateProject)
	mux.HandleFunc("GET /api/squads/{squadId}/projects", h.ListProjects)
	mux.HandleFunc("GET /api/squads/{squadId}/projects/{id}", h.GetProject)
	mux.HandleFunc("PATCH /api/projects/{id}", h.UpdateProject)
}

// --- Response Types ---

type projectResponse struct {
	ID          uuid.UUID            `json:"id"`
	SquadID     uuid.UUID            `json:"squadId"`
	Name        string               `json:"name"`
	Description *string              `json:"description"`
	Status      domain.ProjectStatus `json:"status"`
	CreatedAt   string               `json:"createdAt"`
	UpdatedAt   string               `json:"updatedAt"`
}

func dbProjectToResponse(p db.Project) projectResponse {
	resp := projectResponse{
		ID:        p.ID,
		SquadID:   p.SquadID,
		Name:      p.Name,
		Status:    domain.ProjectStatus(p.Status),
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	}
	if p.Description.Valid {
		resp.Description = &p.Description.String
	}
	return resp
}

// --- Squad Membership Helper ---

func (h *ProjectHandler) verifySquadMembership(w http.ResponseWriter, r *http.Request, squadID uuid.UUID) (uuid.UUID, bool) {
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

func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, squadID); !ok {
		return
	}

	var req domain.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)

	if err := domain.ValidateCreateProjectInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Check duplicate name
	exists, err := h.queries.ProjectExistsByName(r.Context(), db.ProjectExistsByNameParams{
		SquadID: squadID,
		Name:    req.Name,
	})
	if err != nil {
		slog.Error("failed to check project name", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	if exists {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "A project with this name already exists in the squad", Code: "PROJECT_NAME_TAKEN"})
		return
	}

	params := db.CreateProjectParams{
		SquadID: squadID,
		Name:    req.Name,
	}
	if req.Description != nil {
		params.Description = sql.NullString{String: *req.Description, Valid: true}
	}

	project, err := h.queries.CreateProject(r.Context(), params)
	if err != nil {
		slog.Error("failed to create project", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("project created", "project_id", project.ID, "squad_id", squadID)
	writeJSON(w, http.StatusCreated, dbProjectToResponse(project))
}

func (h *ProjectHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, squadID); !ok {
		return
	}

	projects, err := h.queries.ListProjectsBySquad(r.Context(), squadID)
	if err != nil {
		slog.Error("failed to list projects", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	result := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		result = append(result, dbProjectToResponse(p))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	idStr := r.PathValue("id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid project ID", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, squadID); !ok {
		return
	}

	project, err := h.queries.GetProjectByID(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Project not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get project", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if project.SquadID != squadID {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Project not found", Code: "NOT_FOUND"})
		return
	}

	writeJSON(w, http.StatusOK, dbProjectToResponse(project))
}

func (h *ProjectHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid project ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Read body once, unmarshal twice: once for sentinel detection, once for struct
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	var rawBody map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &rawBody); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	var req domain.UpdateProjectRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Detect sentinel fields
	if _, has := rawBody["description"]; has {
		req.SetDescription = true
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
	}

	if err := domain.ValidateUpdateProjectInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch existing project
	existing, err := h.queries.GetProjectByID(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Project not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get project", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, existing.SquadID); !ok {
		return
	}

	// Status transition validation
	if req.Status != nil {
		if err := domain.ValidateProjectTransition(domain.ProjectStatus(existing.Status), *req.Status); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "INVALID_STATUS_TRANSITION"})
			return
		}
	}

	// Duplicate name check
	if req.Name != nil {
		exists, err := h.queries.ProjectExistsByNameExcluding(r.Context(), db.ProjectExistsByNameExcludingParams{
			SquadID: existing.SquadID,
			Name:    *req.Name,
			ID:      projectID,
		})
		if err != nil {
			slog.Error("failed to check project name", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if exists {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "A project with this name already exists in the squad", Code: "PROJECT_NAME_TAKEN"})
			return
		}
	}

	// Build update params
	params := db.UpdateProjectParams{ID: projectID}
	if req.Name != nil {
		params.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	params.SetDescription = req.SetDescription
	if req.SetDescription && req.Description != nil {
		params.Description = sql.NullString{String: *req.Description, Valid: true}
	}
	if req.Status != nil {
		params.Status = sql.NullString{String: string(*req.Status), Valid: true}
	}

	updated, err := h.queries.UpdateProject(r.Context(), params)
	if err != nil {
		slog.Error("failed to update project", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("project updated", "project_id", updated.ID)
	writeJSON(w, http.StatusOK, dbProjectToResponse(updated))
}
