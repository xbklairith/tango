package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// TaskHandler handles task checkout/release operations.
type TaskHandler struct {
	queries *db.Queries
	dbConn  *sql.DB
	sseHub  *sse.Hub
}

// NewTaskHandler creates a new TaskHandler.
func NewTaskHandler(q *db.Queries, dbConn *sql.DB, sseHub *sse.Hub) *TaskHandler {
	return &TaskHandler{queries: q, dbConn: dbConn, sseHub: sseHub}
}

// RegisterRoutes registers task checkout/release routes on the given mux.
func (h *TaskHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/issues/{id}/checkout", h.CheckoutIssue)
	mux.HandleFunc("POST /api/issues/{id}/release", h.ReleaseIssue)
}

// --- Request Types ---

type checkoutRequest struct {
	AgentID          uuid.UUID `json:"agentId"`
	RunID            uuid.UUID `json:"runId"`
	ExpectedStatuses []string  `json:"expectedStatuses"`
}

type releaseRequest struct {
	RunID        uuid.UUID `json:"runId"`
	TargetStatus string    `json:"targetStatus"`
}

// --- Handlers ---

// CheckoutIssue atomically acquires an execution lock on an issue (REQ-013).
func (h *TaskHandler) CheckoutIssue(w http.ResponseWriter, r *http.Request) {
	issueID, ok := parseIssueID(w, r)
	if !ok {
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if req.AgentID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agentId is required", Code: "VALIDATION_ERROR"})
		return
	}
	if req.RunID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "runId is required", Code: "VALIDATION_ERROR"})
		return
	}
	if len(req.ExpectedStatuses) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "expectedStatuses is required", Code: "VALIDATION_ERROR"})
		return
	}

	// Verify caller identity matches agentId (REQ-056)
	if agentIdent, ok := auth.AgentFromContext(r.Context()); ok {
		if agentIdent.AgentID != req.AgentID {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Agent ID does not match authenticated identity", Code: "FORBIDDEN"})
			return
		}
	}

	// Execute CAS transaction
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin checkout transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback() //nolint:errcheck

	// SELECT ... FOR UPDATE to lock the row
	var issue struct {
		ID            uuid.UUID
		SquadID       uuid.UUID
		Status        string
		CheckoutRunID *uuid.UUID
	}
	err = tx.QueryRowContext(r.Context(),
		`SELECT id, squad_id, status, checkout_run_id FROM issues WHERE id = $1 FOR UPDATE`,
		issueID,
	).Scan(&issue.ID, &issue.SquadID, &issue.Status, &issue.CheckoutRunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Issue not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to lock issue for checkout", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// REQ-016: Idempotent — if same run already holds the lock, return 200
	if issue.CheckoutRunID != nil && *issue.CheckoutRunID == req.RunID {
		// Fetch the full issue to return
		var fullIssue db.Issue
		err = tx.QueryRowContext(r.Context(),
			`SELECT * FROM issues WHERE id = $1`, issueID,
		).Scan(issueFields(&fullIssue)...)
		if err != nil {
			// Fall through to a simple response
			writeJSON(w, http.StatusOK, map[string]string{"message": "checkout already held"})
			return
		}
		if err := tx.Commit(); err != nil {
			slog.Error("failed to commit idempotent checkout", "error", err)
		}
		writeJSON(w, http.StatusOK, dbIssueToResponse(fullIssue))
		return
	}

	// Check for conflicts
	if issue.CheckoutRunID != nil {
		// REQ-015: Another agent holds the lock
		writeJSON(w, http.StatusConflict, errorResponse{Error: "Issue is already checked out by another run", Code: "CHECKOUT_CONFLICT"})
		return
	}

	// Verify status is in expectedStatuses
	statusMatch := false
	for _, s := range req.ExpectedStatuses {
		if issue.Status == s {
			statusMatch = true
			break
		}
	}
	if !statusMatch {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "Issue status does not match expected statuses",
			Code:  "STATUS_MISMATCH",
		})
		return
	}

	// Perform the checkout UPDATE
	row := tx.QueryRowContext(r.Context(),
		`UPDATE issues
		 SET status = 'in_progress',
		     checkout_run_id = $2,
		     execution_locked_at = now()
		 WHERE id = $1
		 RETURNING id, squad_id, identifier, type, title, description, status, priority,
		           parent_id, project_id, goal_id, assignee_agent_id, assignee_user_id,
		           billing_code, request_depth, created_at, updated_at, checkout_run_id, execution_locked_at`,
		issueID, req.RunID,
	)

	var updated db.Issue
	var checkoutRunID uuid.NullUUID
	var executionLockedAt sql.NullTime
	err = row.Scan(
		&updated.ID, &updated.SquadID, &updated.Identifier, &updated.Type,
		&updated.Title, &updated.Description, &updated.Status, &updated.Priority,
		&updated.ParentID, &updated.ProjectID, &updated.GoalID,
		&updated.AssigneeAgentID, &updated.AssigneeUserID,
		&updated.BillingCode, &updated.RequestDepth,
		&updated.CreatedAt, &updated.UpdatedAt,
		&checkoutRunID, &executionLockedAt,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) {
			slog.Error("checkout update failed", "pq_error", pqErr.Message, "detail", pqErr.Detail)
		}
		slog.Error("failed to update issue for checkout", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit checkout transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit issue.updated SSE event (REQ-014)
	h.sseHub.Publish(updated.SquadID, "issue.updated", map[string]any{
		"issueId":    updated.ID,
		"identifier": updated.Identifier,
		"changes": map[string]any{
			"status":        "in_progress",
			"checkoutRunId": req.RunID,
		},
	})

	slog.Info("issue checked out", "issue_id", issueID, "run_id", req.RunID)
	writeJSON(w, http.StatusOK, dbIssueToResponse(updated))
}

// ReleaseIssue atomically releases an execution lock on an issue (REQ-017).
func (h *TaskHandler) ReleaseIssue(w http.ResponseWriter, r *http.Request) {
	issueID, ok := parseIssueID(w, r)
	if !ok {
		return
	}

	var req releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if req.RunID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "runId is required", Code: "VALIDATION_ERROR"})
		return
	}

	targetStatus := "todo"
	if req.TargetStatus != "" {
		targetStatus = req.TargetStatus
	}

	// Validate target status is a valid issue status
	if !domain.IssueStatus(targetStatus).Valid() {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid target status", Code: "VALIDATION_ERROR"})
		return
	}

	// Atomic release: only the lock owner can release
	row := h.dbConn.QueryRowContext(r.Context(),
		`UPDATE issues
		 SET checkout_run_id = NULL,
		     execution_locked_at = NULL,
		     status = $2
		 WHERE id = $1 AND checkout_run_id = $3
		 RETURNING id, squad_id, identifier, type, title, description, status, priority,
		           parent_id, project_id, goal_id, assignee_agent_id, assignee_user_id,
		           billing_code, request_depth, created_at, updated_at, checkout_run_id, execution_locked_at`,
		issueID, targetStatus, req.RunID,
	)

	var updated db.Issue
	var checkoutRunID uuid.NullUUID
	var executionLockedAt sql.NullTime
	err := row.Scan(
		&updated.ID, &updated.SquadID, &updated.Identifier, &updated.Type,
		&updated.Title, &updated.Description, &updated.Status, &updated.Priority,
		&updated.ParentID, &updated.ProjectID, &updated.GoalID,
		&updated.AssigneeAgentID, &updated.AssigneeUserID,
		&updated.BillingCode, &updated.RequestDepth,
		&updated.CreatedAt, &updated.UpdatedAt,
		&checkoutRunID, &executionLockedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "Caller does not hold the checkout lock", Code: "CHECKOUT_CONFLICT"})
			return
		}
		slog.Error("failed to release issue", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Emit issue.updated SSE event (REQ-017)
	h.sseHub.Publish(updated.SquadID, "issue.updated", map[string]any{
		"issueId":    updated.ID,
		"identifier": updated.Identifier,
		"changes": map[string]any{
			"status":        targetStatus,
			"checkoutRunId": nil,
		},
	})

	slog.Info("issue released", "issue_id", issueID, "run_id", req.RunID, "target_status", targetStatus)
	writeJSON(w, http.StatusOK, dbIssueToResponse(updated))
}

// issueFields returns a slice of scan destinations for all columns in the issues table.
// This helper is used for raw SQL queries that return SELECT * from issues.
func issueFields(i *db.Issue) []any {
	var checkoutRunID uuid.NullUUID
	var executionLockedAt sql.NullTime
	return []any{
		&i.ID, &i.SquadID, &i.Identifier, &i.Type, &i.Title, &i.Description,
		&i.Status, &i.Priority, &i.ParentID, &i.ProjectID, &i.GoalID,
		&i.AssigneeAgentID, &i.AssigneeUserID, &i.BillingCode, &i.RequestDepth,
		&i.CreatedAt, &i.UpdatedAt, &checkoutRunID, &executionLockedAt,
	}
}

// issueResponseWithCheckout extends the standard issue response with checkout fields.
type issueWithCheckoutResponse struct {
	issueResponse
	CheckoutRunID    *uuid.UUID `json:"checkoutRunId,omitempty"`
	ExecutionLockedAt *time.Time `json:"executionLockedAt,omitempty"`
}
