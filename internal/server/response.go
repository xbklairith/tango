package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorResponse is the standard API error format.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// writeJSON serializes data as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write json response", "error", err)
	}
}
