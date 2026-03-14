package server

import (
	"net/http"
)

// HealthResponse is returned by the health endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// HealthErrorResponse is returned when a dependency check fails.
type HealthErrorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// handleHealth checks application health including database connectivity.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, HealthErrorResponse{
			Status: "unhealthy",
			Error:  "database ping failed",
		})
		return
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "ok",
		Version: s.version,
	})
}

// handleNotFound returns a 404 for unknown API routes.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, ErrorResponse{
		Error: "Not found",
		Code:  "NOT_FOUND",
	})
}
