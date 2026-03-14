package server

import (
	"net/http"
)

// registerRoutes configures all API routes on the given mux.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /api/health", s.handleHealth)

	// Catch-all for unknown API routes
	mux.HandleFunc("/api/", s.handleNotFound)
}
