package handlers

import (
	"net/http"
	"sort"

	"github.com/xb/ari/internal/auth"
)

// PermissionHandler serves the permission matrix API endpoint.
type PermissionHandler struct{}

// NewPermissionHandler creates a new PermissionHandler.
func NewPermissionHandler() *PermissionHandler {
	return &PermissionHandler{}
}

// RegisterRoutes registers the permission routes on the given mux.
func (h *PermissionHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/permissions", h.GetPermissions)
}

// permissionMatrixResponse is the JSON response shape for GET /api/permissions.
type permissionMatrixResponse struct {
	UserRoles  map[string]map[string][]string `json:"userRoles"`
	AgentRoles map[string]map[string][]string `json:"agentRoles"`
}

// GetPermissions returns the full permission matrix as JSON.
func (h *PermissionHandler) GetPermissions(w http.ResponseWriter, r *http.Request) {
	// Any authenticated user or agent can view the matrix.
	_, isUser := auth.UserFromContext(r.Context())
	_, isAgent := auth.AgentFromContext(r.Context())
	if !isUser && !isAgent {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	resp := permissionMatrixResponse{
		UserRoles:  formatPermissions(auth.UserPermissions),
		AgentRoles: formatPermissions(auth.AgentPermissions),
	}

	writeJSON(w, http.StatusOK, resp)
}

// formatPermissions converts internal permission maps to a JSON-friendly format.
func formatPermissions(rp auth.RolePermissions) map[string]map[string][]string {
	result := make(map[string]map[string][]string, len(rp))
	for role, perms := range rp {
		resourceMap := make(map[string][]string, len(perms))
		for resource, actionMap := range perms {
			var actionList []string
			for action, allowed := range actionMap {
				if allowed {
					actionList = append(actionList, string(action))
				}
			}
			sort.Strings(actionList)
			resourceMap[string(resource)] = actionList
		}
		result[role] = resourceMap
	}
	return result
}
