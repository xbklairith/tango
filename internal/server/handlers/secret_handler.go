package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
)

// SecretHandler handles HTTP requests for secret CRUD and master key rotation.
type SecretHandler struct {
	queries    *db.Queries
	secretsSvc *SecretsService
}

// NewSecretHandler creates a new SecretHandler.
func NewSecretHandler(q *db.Queries, svc *SecretsService) *SecretHandler {
	return &SecretHandler{queries: q, secretsSvc: svc}
}

// RegisterRoutes registers secret routes on the mux.
func (h *SecretHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/squads/{squadId}/secrets", h.CreateSecret)
	mux.HandleFunc("GET /api/squads/{squadId}/secrets", h.ListSecrets)
	mux.HandleFunc("PUT /api/squads/{squadId}/secrets/{name}", h.UpdateSecret)
	mux.HandleFunc("DELETE /api/squads/{squadId}/secrets/{name}", h.DeleteSecret)
	mux.HandleFunc("POST /api/secrets/rotate-master-key", h.RotateMasterKey)
}

// --- Request types ---

type createSecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type updateSecretRequest struct {
	Value string `json:"value"`
}

// CreateSecret handles POST /api/squads/{squadId}/secrets.
func (h *SecretHandler) CreateSecret(w http.ResponseWriter, r *http.Request) {
	squadID, err := uuid.Parse(r.PathValue("squadId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if !requirePermission(w, r, squadID, auth.ResourceSecret, auth.ActionCreate, makeRoleLookup(h.queries)) {
		return
	}

	var req createSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	resp, err := h.secretsSvc.Create(r.Context(), squadID, req.Name, req.Value)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// ListSecrets handles GET /api/squads/{squadId}/secrets.
func (h *SecretHandler) ListSecrets(w http.ResponseWriter, r *http.Request) {
	squadID, err := uuid.Parse(r.PathValue("squadId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if !requirePermission(w, r, squadID, auth.ResourceSecret, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	list, err := h.secretsSvc.List(r.Context(), squadID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

// UpdateSecret handles PUT /api/squads/{squadId}/secrets/{name}.
func (h *SecretHandler) UpdateSecret(w http.ResponseWriter, r *http.Request) {
	squadID, err := uuid.Parse(r.PathValue("squadId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if !requirePermission(w, r, squadID, auth.ResourceSecret, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Secret name is required", Code: "VALIDATION_ERROR"})
		return
	}

	var req updateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	resp, err := h.secretsSvc.Update(r.Context(), squadID, name, req.Value)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// DeleteSecret handles DELETE /api/squads/{squadId}/secrets/{name}.
func (h *SecretHandler) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	squadID, err := uuid.Parse(r.PathValue("squadId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if !requirePermission(w, r, squadID, auth.ResourceSecret, auth.ActionDelete, makeRoleLookup(h.queries)) {
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Secret name is required", Code: "VALIDATION_ERROR"})
		return
	}

	err = h.secretsSvc.Delete(r.Context(), squadID, name)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RotateMasterKey handles POST /api/secrets/rotate-master-key.
func (h *SecretHandler) RotateMasterKey(w http.ResponseWriter, r *http.Request) {
	// System-level operation: requires is_admin
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// LocalOperator is always admin
	isAdmin := false
	if caller.ID == uuid.Nil && caller.Email == "local@ari.local" {
		isAdmin = true
	} else {
		// Look up user in DB to check is_admin
		dbUser, err := h.queries.GetUserByID(r.Context(), caller.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		isAdmin = dbUser.IsAdmin
	}

	if !isAdmin {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Admin access required", Code: "FORBIDDEN"})
		return
	}

	count, err := h.secretsSvc.RotateMasterKey(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Rotation failed: " + err.Error(), Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"rotatedCount": count})
}

// handleServiceError writes the appropriate HTTP response for a service error.
func (h *SecretHandler) handleServiceError(w http.ResponseWriter, err error) {
	if svcErr, ok := err.(*ServiceError); ok {
		writeJSON(w, svcErr.Code, errorResponse{Error: svcErr.Message, Code: svcErr.ErrorCode})
		return
	}
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
}
