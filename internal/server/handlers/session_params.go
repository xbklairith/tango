package handlers

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// SessionParams holds structured session state with cwd for validation on resume.
type SessionParams struct {
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd,omitempty"`
}

// parseSessionParams handles both new JSON format and legacy bare strings.
// Legacy format: the entire string is a bare session ID (no JSON structure).
// New format: JSON object with sessionId and cwd fields.
func parseSessionParams(raw string) SessionParams {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SessionParams{}
	}
	var params SessionParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		// Legacy bare string — treat entire value as session ID
		return SessionParams{SessionID: raw}
	}
	return params
}

// marshalSessionParams encodes session params as JSON for storage.
func marshalSessionParams(params SessionParams) string {
	data, err := json.Marshal(params)
	if err != nil {
		return params.SessionID // fallback
	}
	return string(data)
}

// canResumeSession checks if a session can be resumed based on cwd match.
func canResumeSession(params SessionParams, currentCwd string) bool {
	if params.SessionID == "" {
		return false
	}
	if params.Cwd == "" {
		return true // no cwd recorded, allow resume
	}
	return filepath.Clean(params.Cwd) == filepath.Clean(currentCwd)
}
