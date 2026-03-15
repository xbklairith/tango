package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/sqlc-dev/pqtype"

	"github.com/xb/ari/internal/adapter"
	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// RunService manages the HeartbeatRun lifecycle: creating runs,
// invoking adapters, and finalizing results.
type RunService struct {
	dbConn   *sql.DB
	queries  *db.Queries
	registry *adapter.Registry
	tokenSvc *auth.RunTokenService
	sseHub   *sse.Hub
	apiURL   string

	// active tracks cancel funcs for running invocations (for graceful stop)
	mu     sync.Mutex
	active map[uuid.UUID]context.CancelFunc // runID → cancel
}

// NewRunService creates a new RunService.
func NewRunService(
	dbConn *sql.DB,
	queries *db.Queries,
	registry *adapter.Registry,
	tokenSvc *auth.RunTokenService,
	sseHub *sse.Hub,
	apiURL string,
) *RunService {
	return &RunService{
		dbConn:   dbConn,
		queries:  queries,
		registry: registry,
		tokenSvc: tokenSvc,
		sseHub:   sseHub,
		apiURL:   apiURL,
		active:   make(map[uuid.UUID]context.CancelFunc),
	}
}

// Invoke creates a HeartbeatRun record, transitions the agent to running,
// mints a Run Token, calls adapter.Execute(), then finalizes the result.
func (s *RunService) Invoke(ctx context.Context, wakeup db.WakeupRequest) error {
	// 1. Load agent
	agent, err := s.queries.GetAgentByID(ctx, wakeup.AgentID)
	if err != nil {
		return err
	}

	// 2. Resolve adapter
	adapterType := "process" // default
	if agent.AdapterType.Valid {
		adapterType = string(agent.AdapterType.AdapterType)
	}
	a, err := s.registry.Resolve(adapterType)
	if err != nil {
		slog.Error("adapter not found", "adapter_type", adapterType, "error", err)
		return err
	}

	// 3. Load session state (if applicable)
	var sessionBefore string
	taskID := extractTaskID(wakeup.ContextJson)
	if taskID != nil {
		ss, err := s.queries.GetTaskSession(ctx, db.GetTaskSessionParams{
			AgentID: agent.ID,
			IssueID: *taskID,
		})
		if err == nil {
			sessionBefore = ss
		}
	}

	// 4. Create HeartbeatRun (status=queued)
	run, err := s.queries.CreateHeartbeatRun(ctx, db.CreateHeartbeatRunParams{
		SquadID:          wakeup.SquadID,
		AgentID:          wakeup.AgentID,
		WakeupRequestID:  uuid.NullUUID{UUID: wakeup.ID, Valid: true},
		InvocationSource: wakeup.InvocationSource,
		SessionIDBefore:  sql.NullString{String: sessionBefore, Valid: sessionBefore != ""},
	})
	if err != nil {
		return err
	}

	// Emit heartbeat.run.queued SSE
	s.sseHub.Publish(wakeup.SquadID, "heartbeat.run.queued", map[string]any{
		"runId":            run.ID,
		"agentId":          agent.ID,
		"invocationSource": string(wakeup.InvocationSource),
	})

	// 5. Mint Run Token
	token, err := s.tokenSvc.Mint(agent.ID, wakeup.SquadID, run.ID, string(agent.Role))
	if err != nil {
		return err
	}

	// 6. Transition to running
	run, err = s.queries.UpdateHeartbeatRunStarted(ctx, run.ID)
	if err != nil {
		return err
	}

	// Update agent status to running
	_, err = s.queries.UpdateAgent(ctx, db.UpdateAgentParams{
		ID:     agent.ID,
		Status: db.NullAgentStatus{AgentStatus: db.AgentStatusRunning, Valid: true},
	})
	if err != nil {
		slog.Error("failed to transition agent to running", "error", err)
	}

	// Emit SSE events
	s.sseHub.Publish(wakeup.SquadID, "heartbeat.run.started", map[string]any{
		"runId":     run.ID,
		"agentId":   agent.ID,
		"startedAt": run.StartedAt.Time.Format("2006-01-02T15:04:05Z"),
	})
	s.sseHub.Publish(wakeup.SquadID, "agent.status.changed", map[string]any{
		"agentId": agent.ID,
		"from":    string(agent.Status),
		"to":      "running",
		"runId":   run.ID,
	})

	// 7. Build InvokeInput
	input := s.buildInvokeInput(agent, wakeup, run, token, sessionBefore)

	// 8. Register cancel func
	runCtx, cancelRun := context.WithCancel(ctx)
	s.mu.Lock()
	s.active[run.ID] = cancelRun
	s.mu.Unlock()

	// 9. Execute adapter (blocks)
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			s.sseHub.Publish(wakeup.SquadID, "heartbeat.run.log", map[string]any{
				"runId":     run.ID,
				"agentId":   agent.ID,
				"level":     line.Level,
				"message":   line.Message,
				"timestamp": line.Timestamp.Format("2006-01-02T15:04:05Z"),
			})
		},
		OnStatusChange: func(detail string) {
			slog.Debug("adapter status change", "run_id", run.ID, "detail", detail)
		},
	}

	result, execErr := a.Execute(runCtx, input, hooks)
	cancelRun()

	// 10. Clean up active map
	s.mu.Lock()
	delete(s.active, run.ID)
	s.mu.Unlock()

	// 11. Finalize
	return s.finalize(ctx, agent, run, wakeup, result, execErr, taskID)
}

// Stop cancels the active invocation for the agent's current run.
func (s *RunService) Stop(ctx context.Context, agentID uuid.UUID) error {
	// Find active run for this agent
	activeRun, err := s.queries.GetActiveRunByAgent(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("no active run for agent")
		}
		return err
	}

	// Cancel the run context
	s.mu.Lock()
	cancelFn, ok := s.active[activeRun.ID]
	s.mu.Unlock()

	if ok {
		cancelFn()
	}

	// Transition agent to paused
	_, err = s.queries.UpdateAgent(ctx, db.UpdateAgentParams{
		ID:     agentID,
		Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
	})
	if err != nil {
		slog.Error("failed to transition agent to paused", "error", err)
	}

	// Revoke the run token
	s.tokenSvc.Revoke(activeRun.ID)

	// Discard any pending wakeups
	if err := s.queries.DiscardPendingWakeupsByAgent(ctx, agentID); err != nil {
		slog.Error("failed to discard pending wakeups", "error", err)
	}

	return nil
}

// CancelStaleRuns marks any queued/running heartbeat runs as cancelled.
// Called at startup to clean up after unclean shutdown (REQ-057).
func (s *RunService) CancelStaleRuns(ctx context.Context) error {
	return s.queries.CancelStaleHeartbeatRuns(ctx)
}

// finalize writes the run result, persists session state, emits SSE events,
// and transitions the agent status.
func (s *RunService) finalize(
	ctx context.Context,
	agent db.Agent,
	run db.HeartbeatRun,
	wakeup db.WakeupRequest,
	result adapter.InvokeResult,
	execErr error,
	taskID *uuid.UUID,
) error {
	// Map adapter RunStatus to DB HeartbeatRunStatus
	dbStatus := mapRunStatus(result.Status)

	usageJSON, _ := json.Marshal(result.Usage)

	// Update HeartbeatRun with final state
	_, err := s.queries.UpdateHeartbeatRunFinished(ctx, db.UpdateHeartbeatRunFinishedParams{
		ID:             run.ID,
		Status:         dbStatus,
		ExitCode:       sql.NullInt32{Int32: int32(result.ExitCode), Valid: true},
		UsageJson:      pqtype.NullRawMessage{RawMessage: usageJSON, Valid: true},
		SessionIDAfter: sql.NullString{String: result.SessionState, Valid: result.SessionState != ""},
		StdoutExcerpt:  sql.NullString{String: result.Stdout, Valid: result.Stdout != ""},
		StderrExcerpt:  sql.NullString{String: result.Stderr, Valid: result.Stderr != ""},
	})
	if err != nil {
		slog.Error("failed to finalize heartbeat run", "run_id", run.ID, "error", err)
		return err
	}

	// Persist session state (REQ-026)
	if result.SessionState != "" && taskID != nil {
		convID := extractConversationID(wakeup.ContextJson)
		if convID != nil {
			_ = s.queries.UpsertConversationSession(ctx, db.UpsertConversationSessionParams{
				AgentID:      agent.ID,
				IssueID:      *convID,
				SessionState: result.SessionState,
			})
		} else {
			_ = s.queries.UpsertTaskSession(ctx, db.UpsertTaskSessionParams{
				AgentID:      agent.ID,
				IssueID:      *taskID,
				SessionState: result.SessionState,
			})
		}
	}

	// Determine next agent status
	var nextStatus db.AgentStatus
	switch result.Status {
	case adapter.RunStatusSucceeded:
		nextStatus = db.AgentStatusIdle
	case adapter.RunStatusFailed, adapter.RunStatusTimedOut:
		nextStatus = db.AgentStatusError
	case adapter.RunStatusCancelled:
		nextStatus = db.AgentStatusPaused
	default:
		nextStatus = db.AgentStatusIdle
	}

	// Validate state transition before applying
	if err := domain.ValidateStatusTransition(domain.AgentStatus(agent.Status), domain.AgentStatus(nextStatus)); err != nil {
		slog.Warn("skipping invalid agent status transition after run",
			"agent_id", agent.ID,
			"from", agent.Status,
			"to", nextStatus,
			"error", err)
	} else {
		_, updateErr := s.queries.UpdateAgent(ctx, db.UpdateAgentParams{
			ID:     agent.ID,
			Status: db.NullAgentStatus{AgentStatus: nextStatus, Valid: true},
		})
		if updateErr != nil {
			slog.Error("failed to update agent status after run", "error", updateErr)
		}
	}

	// Emit SSE events
	s.sseHub.Publish(wakeup.SquadID, "heartbeat.run.finished", map[string]any{
		"runId":      run.ID,
		"agentId":    agent.ID,
		"status":     string(dbStatus),
		"exitCode":   result.ExitCode,
		"finishedAt": run.FinishedAt.Time.Format("2006-01-02T15:04:05Z"),
	})
	s.sseHub.Publish(wakeup.SquadID, "agent.status.changed", map[string]any{
		"agentId": agent.ID,
		"from":    "running",
		"to":      string(nextStatus),
	})

	if execErr != nil {
		slog.Error("adapter execution error", "run_id", run.ID, "error", execErr)
	}

	return nil
}

// buildInvokeInput constructs the adapter input from the agent, wakeup, and run.
func (s *RunService) buildInvokeInput(
	agent db.Agent,
	wakeup db.WakeupRequest,
	run db.HeartbeatRun,
	token string,
	sessionBefore string,
) adapter.InvokeInput {
	envVars := map[string]string{
		"ARI_API_URL":     s.apiURL,
		"ARI_API_KEY":     token,
		"ARI_AGENT_ID":    agent.ID.String(),
		"ARI_SQUAD_ID":    wakeup.SquadID.String(),
		"ARI_RUN_ID":      run.ID.String(),
		"ARI_WAKE_REASON": string(wakeup.InvocationSource),
	}

	// Add task/conversation IDs from wakeup context
	taskID := extractTaskID(wakeup.ContextJson)
	if taskID != nil {
		envVars["ARI_TASK_ID"] = taskID.String()
	}
	convID := extractConversationID(wakeup.ContextJson)
	if convID != nil {
		envVars["ARI_CONVERSATION_ID"] = convID.String()
	}

	var adapterConfig json.RawMessage
	if agent.AdapterConfig.Valid {
		adapterConfig = agent.AdapterConfig.RawMessage
	}
	var systemPrompt string
	if agent.SystemPrompt.Valid {
		systemPrompt = agent.SystemPrompt.String
	}
	var model string
	if agent.Model.Valid {
		model = agent.Model.String
	}

	// Enrich with issue details when a task is assigned
	if taskID != nil {
		issue, err := s.queries.GetIssueByID(context.Background(), *taskID)
		if err == nil {
			envVars["ARI_ISSUE_TITLE"] = issue.Title
			if issue.Description.Valid {
				envVars["ARI_ISSUE_DESCRIPTION"] = issue.Description.String
			}
			envVars["ARI_ISSUE_IDENTIFIER"] = issue.Identifier

			// Load squad name for prompt assembly
			squadName := wakeup.SquadID.String()
			if squad, err := s.queries.GetSquadByID(context.Background(), wakeup.SquadID); err == nil {
				squadName = squad.Name
			}

			// Assemble full prompt
			prompt := fmt.Sprintf(`You are %s, a %s in squad %s.

%s

Your current task: %s (%s)
Description: %s

API: %s
Auth: Bearer %s

When done, mark the task complete:
PATCH %s/api/agent/me/task
Body: {"issueId": "%s", "status": "done"}`,
				agent.Name, string(agent.Role), squadName,
				systemPrompt,
				issue.Title, issue.Identifier,
				envVars["ARI_ISSUE_DESCRIPTION"],
				s.apiURL, token,
				s.apiURL, taskID.String(),
			)
			envVars["ARI_PROMPT"] = prompt
		} else {
			slog.Warn("failed to load issue for invoke input", "task_id", taskID, "error", err)
		}
	}

	if systemPrompt != "" {
		envVars["ARI_SYSTEM_PROMPT"] = systemPrompt
	}

	return adapter.InvokeInput{
		Agent: adapter.AgentContext{
			ID:            agent.ID,
			Name:          agent.Name,
			ShortName:     agent.ShortName,
			Role:          string(agent.Role),
			AdapterConfig: adapterConfig,
			SystemPrompt:  systemPrompt,
			Model:         model,
		},
		Squad: adapter.SquadContext{
			ID: wakeup.SquadID,
		},
		Run: adapter.RunContext{
			RunID:        run.ID,
			WakeReason:   string(wakeup.InvocationSource),
			TaskID:       taskID,
			SessionState: sessionBefore,
		},
		EnvVars: envVars,
	}
}

// mapRunStatus converts adapter.RunStatus to db.HeartbeatRunStatus.
func mapRunStatus(status adapter.RunStatus) db.HeartbeatRunStatus {
	switch status {
	case adapter.RunStatusSucceeded:
		return db.HeartbeatRunStatusSucceeded
	case adapter.RunStatusFailed:
		return db.HeartbeatRunStatusFailed
	case adapter.RunStatusCancelled:
		return db.HeartbeatRunStatusCancelled
	case adapter.RunStatusTimedOut:
		return db.HeartbeatRunStatusTimedOut
	default:
		return db.HeartbeatRunStatusFailed
	}
}

// extractTaskID extracts ARI_TASK_ID from wakeup context JSON.
func extractTaskID(contextJSON json.RawMessage) *uuid.UUID {
	var ctx map[string]string
	if err := json.Unmarshal(contextJSON, &ctx); err != nil {
		return nil
	}
	if idStr, ok := ctx["ARI_TASK_ID"]; ok {
		id, err := uuid.Parse(idStr)
		if err == nil {
			return &id
		}
	}
	return nil
}

// extractConversationID extracts ARI_CONVERSATION_ID from wakeup context JSON.
func extractConversationID(contextJSON json.RawMessage) *uuid.UUID {
	var ctx map[string]string
	if err := json.Unmarshal(contextJSON, &ctx); err != nil {
		return nil
	}
	if idStr, ok := ctx["ARI_CONVERSATION_ID"]; ok {
		id, err := uuid.Parse(idStr)
		if err == nil {
			return &id
		}
	}
	return nil
}
