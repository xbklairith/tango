package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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
	dbConn       *sql.DB
	queries      *db.Queries
	registry     *adapter.Registry
	tokenSvc     *auth.RunTokenService
	sseHub       *sse.Hub
	apiURL       string
	dataDir      string // base data directory for run log files
	inboxService *InboxService
	secretsSvc   *SecretsService

	// active tracks cancel funcs for running invocations (for graceful stop)
	mu     sync.Mutex
	active map[uuid.UUID]context.CancelFunc // runID → cancel
}

// SetInboxService sets the InboxService for error reporting on failed runs.
func (s *RunService) SetInboxService(is *InboxService) {
	s.inboxService = is
}

// SetSecretsService sets the SecretsService for injecting secrets into agent runs.
func (s *RunService) SetSecretsService(ss *SecretsService) {
	s.secretsSvc = ss
}

// NewRunService creates a new RunService.
func NewRunService(
	dbConn *sql.DB,
	queries *db.Queries,
	registry *adapter.Registry,
	tokenSvc *auth.RunTokenService,
	sseHub *sse.Hub,
	apiURL string,
	dataDir string,
) *RunService {
	return &RunService{
		dbConn:   dbConn,
		queries:  queries,
		registry: registry,
		tokenSvc: tokenSvc,
		sseHub:   sseHub,
		apiURL:   apiURL,
		dataDir:  dataDir,
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
	convID := extractConversationID(wakeup.ContextJson)

	if convID != nil {
		// Conversation session — separate table from task sessions
		ss, err := s.queries.GetConversationSession(ctx, db.GetConversationSessionParams{
			AgentID: agent.ID,
			IssueID: *convID,
		})
		if err == nil {
			// Parse structured session params and validate cwd
			params := parseSessionParams(ss)
			cwd, _ := os.Getwd()
			if canResumeSession(params, cwd) {
				sessionBefore = params.SessionID
			} else if params.SessionID != "" {
				slog.Warn("session cwd mismatch, starting fresh",
					"agent_id", agent.ID, "session_cwd", params.Cwd, "current_cwd", cwd)
			}
		}
	} else if taskID != nil {
		// Task session — existing path
		ss, err := s.queries.GetTaskSession(ctx, db.GetTaskSessionParams{
			AgentID: agent.ID,
			IssueID: *taskID,
		})
		if err == nil {
			// Parse structured session params and validate cwd
			params := parseSessionParams(ss)
			cwd, _ := os.Getwd()
			if canResumeSession(params, cwd) {
				sessionBefore = params.SessionID
			} else if params.SessionID != "" {
				slog.Warn("session cwd mismatch, starting fresh",
					"agent_id", agent.ID, "session_cwd", params.Cwd, "current_cwd", cwd)
			}
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

	// 5. Mint Run Token (with optional conversation ID)
	var mintOpts []auth.MintOption
	if convID != nil {
		mintOpts = append(mintOpts, auth.WithConversationID(*convID))
	}
	token, err := s.tokenSvc.Mint(agent.ID, wakeup.SquadID, run.ID, string(agent.Role), mintOpts...)
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

	// Emit conversation typing indicator if this is a conversation wakeup
	if convID != nil && wakeup.InvocationSource == "conversation_message" {
		s.sseHub.Publish(wakeup.SquadID, "conversation.agent.typing", map[string]any{
			"conversationId": *convID,
			"agentId":        agent.ID,
		})
	}

	// 7. Build InvokeInput
	input := s.buildInvokeInput(ctx, agent, wakeup, run, token, sessionBefore)

	// 8. Register cancel func
	runCtx, cancelRun := context.WithCancel(ctx)
	s.mu.Lock()
	s.active[run.ID] = cancelRun
	s.mu.Unlock()

	// 9. Open run log file for persistence
	var logFile *os.File
	if s.dataDir != "" {
		logDir := filepath.Join(s.dataDir, "runs")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			slog.Warn("failed to create run log directory", "error", err)
		} else {
			logFile, err = os.Create(filepath.Join(logDir, run.ID.String()+".jsonl"))
			if err != nil {
				slog.Warn("failed to create run log file", "error", err)
			}
		}
	}
	if logFile != nil {
		defer logFile.Close()
	}

	// Execute adapter (blocks)
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			payload := map[string]any{
				"runId":     run.ID,
				"agentId":   agent.ID,
				"level":     line.Level,
				"message":   line.Message,
				"timestamp": line.Timestamp.Format("2006-01-02T15:04:05Z"),
			}
			if line.Fields != nil {
				payload["fields"] = line.Fields
			}
			s.sseHub.Publish(wakeup.SquadID, "heartbeat.run.log", payload)

			// Persist log line to file
			if logFile != nil {
				logEntry, _ := json.Marshal(map[string]any{
					"ts":    line.Timestamp.Format(time.RFC3339),
					"level": line.Level,
					"msg":   line.Message,
				})
				logFile.Write(append(logEntry, '\n'))
			}
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
	return s.finalize(ctx, agent, run, wakeup, result, execErr, taskID, convID)
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

// CancelStaleRuns marks any queued/running heartbeat runs older than 2 hours as cancelled.
// Called at startup to clean up after unclean shutdown (REQ-057).
func (s *RunService) CancelStaleRuns(ctx context.Context) error {
	return s.queries.CancelAllStaleHeartbeatRuns(ctx)
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
	convID *uuid.UUID,
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

	// Persist session state as structured JSON with cwd (REQ-015)
	if result.SessionState != "" && !result.ClearSession {
		cwd, _ := os.Getwd()
		sessionJSON := marshalSessionParams(SessionParams{
			SessionID: result.SessionState,
			Cwd:       cwd,
		})
		if convID != nil {
			_ = s.queries.UpsertConversationSession(ctx, db.UpsertConversationSessionParams{
				AgentID:      agent.ID,
				IssueID:      *convID,
				SessionState: sessionJSON,
			})
		} else if taskID != nil {
			_ = s.queries.UpsertTaskSession(ctx, db.UpsertTaskSessionParams{
				AgentID:      agent.ID,
				IssueID:      *taskID,
				SessionState: sessionJSON,
			})
		}
	} else if result.ClearSession {
		slog.Info("clearing session state (max turns or explicit clear)",
			"agent_id", agent.ID, "run_id", run.ID)
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

	// Emit conversation typing stopped if this was a conversation wakeup
	if convID != nil && wakeup.InvocationSource == "conversation_message" {
		s.sseHub.Publish(wakeup.SquadID, "conversation.agent.typing.stopped", map[string]any{
			"conversationId": *convID,
			"agentId":        agent.ID,
		})
	}

	// Create inbox alert for failed/timed-out runs (best-effort)
	if (result.Status == adapter.RunStatusFailed || result.Status == adapter.RunStatusTimedOut) && s.inboxService != nil {
		alertType := "run_failed"
		if result.Status == adapter.RunStatusTimedOut {
			alertType = "run_timed_out"
		}
		if result.LoginRequired {
			alertType = "login_required"
		}
		stderrExcerpt := result.Stderr
		if result.LoginRequired && result.LoginURL != "" {
			stderrExcerpt = fmt.Sprintf("Claude CLI requires re-authentication. Login URL: %s\n\n%s", result.LoginURL, stderrExcerpt)
		}
		_, inboxErr := s.inboxService.CreateAgentError(ctx, s.queries, CreateAgentErrorParams{
			SquadID:       wakeup.SquadID,
			AgentID:       agent.ID,
			RunID:         run.ID,
			Type:          alertType,
			ExitCode:      result.ExitCode,
			StderrExcerpt: stderrExcerpt,
		})
		if inboxErr != nil {
			slog.Error("failed to create inbox alert for run error",
				"run_id", run.ID, "error", inboxErr)
		}
	}

	if execErr != nil {
		slog.Error("adapter execution error", "run_id", run.ID, "error", execErr)
	}

	return nil
}

// buildInvokeInput constructs the adapter input from the agent, wakeup, and run.
func (s *RunService) buildInvokeInput(
	ctx context.Context,
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

	// Enrich with issue details when a task is assigned (but NOT for conversation wakeups)
	if taskID != nil && convID == nil {
		issue, err := s.queries.GetIssueByID(ctx, *taskID)
		if err == nil {
			envVars["ARI_ISSUE_TITLE"] = issue.Title
			if issue.Description.Valid {
				envVars["ARI_ISSUE_DESCRIPTION"] = issue.Description.String
			}
			envVars["ARI_ISSUE_IDENTIFIER"] = issue.Identifier

			// Load squad name for prompt assembly
			squadName := wakeup.SquadID.String()
			if squad, err := s.queries.GetSquadByID(ctx, wakeup.SquadID); err == nil {
				squadName = squad.Name
			}

			// Assemble full prompt
			prompt := fmt.Sprintf(`You are %s, a %s in squad %s.

%s

Your current task: %s (%s)
Description: %s

API: %s
Auth: Use the ARI_API_KEY environment variable (already set)

IMPORTANT: You MUST post a comment on the issue describing your work BEFORE marking the task complete.
Use curl to post comments and update task status. Always include the Authorization header.

Step 1 - Do the work described in the task.

Step 2 - Post a comment summarizing what you did or your response to the task:
curl -X POST %s/api/issues/%s/comments \
  -H "Authorization: Bearer $ARI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"body": "<your detailed response>"}'

Step 3 - Mark the task complete:
curl -X PATCH %s/api/agent/me/task \
  -H "Authorization: Bearer $ARI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"issueId": "%s", "status": "done"}'

If you need human input or approval, send to inbox:
curl -X POST %s/api/agent/me/inbox \
  -H "Authorization: Bearer $ARI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"category": "<approval|question|decision>", "title": "<title>", "body": "<details>", "urgency": "<low|medium|high|critical>"}'

Do NOT mark the task done without posting a comment first.`,
				agent.Name, string(agent.Role), squadName,
				systemPrompt,
				issue.Title, issue.Identifier,
				envVars["ARI_ISSUE_DESCRIPTION"],
				s.apiURL,
				s.apiURL, taskID.String(),
				s.apiURL, taskID.String(),
				s.apiURL,
			)
			envVars["ARI_PROMPT"] = prompt
		} else {
			slog.Warn("failed to load issue for invoke input", "task_id", taskID, "error", err)
		}
	}

	// Build conversation context when ARI_CONVERSATION_ID is present
	var conversation *adapter.ConversationContext
	if convID != nil {
		convIssue, err := s.queries.GetIssueByID(ctx, *convID)
		if err == nil {
			comments, err := s.queries.ListIssueComments(ctx, db.ListIssueCommentsParams{
				IssueID:    *convID,
				PageLimit:  100,
				PageOffset: 0,
			})
			if err == nil {
				messages := make([]adapter.CommentEntry, 0, len(comments))
				for _, c := range comments {
					messages = append(messages, adapter.CommentEntry{
						ID:         c.ID,
						AuthorType: string(c.AuthorType),
						AuthorID:   c.AuthorID,
						Body:       c.Body,
						CreatedAt:  c.CreatedAt,
					})
				}

				conversation = &adapter.ConversationContext{
					IssueID:      *convID,
					Messages:     messages,
					SessionState: sessionBefore,
				}

				// Build conversation-specific prompt
				squadName := wakeup.SquadID.String()
				if squad, sqErr := s.queries.GetSquadByID(ctx, wakeup.SquadID); sqErr == nil {
					squadName = squad.Name
				}

				prompt := fmt.Sprintf(`You are %s, a %s in squad %s.

%s

You are in a conversation: %s (%s)

Message thread:
%s

API: %s
Auth: Use the ARI_API_KEY environment variable (already set). Always include "Authorization: Bearer $ARI_API_KEY" header.

== REPLY ==
Reply to the user's latest message:
POST %s/api/agent/me/reply
Body: {"conversationId": "%s", "body": "<your reply>"}

== INBOX ==
When the user asks you to send something to their inbox (e.g. a question, decision, or approval request), create an inbox item:
POST %s/api/agent/me/inbox
Body: {"category": "<approval|question|decision>", "title": "<short title>", "body": "<details>", "urgency": "<low|medium|high|critical>"}

== ISSUES ==
When the user asks you to create an issue or task, use:
POST %s/api/squads/%s/issues
Body: {"title": "<title>", "type": "<task|bug|story|epic>", "priority": "<critical|high|medium|low|none>", "description": "<optional description>"}

== COMMENTS ON ISSUES ==
To add a comment to an existing issue:
POST %s/api/issues/<issueId>/comments
Body: {"body": "<comment text>"}

IMPORTANT: Read the user's message carefully. If they ask you to send to inbox, use the inbox endpoint. If they ask to create an issue, use the issues endpoint. Do NOT just reply in the conversation when the user is asking you to perform an action.
Always reply in the conversation as well to confirm what you did.`,
					agent.Name, string(agent.Role), squadName,
					systemPrompt,
					convIssue.Title, convIssue.Identifier,
					formatMessageThread(comments),
					s.apiURL,
					s.apiURL, convID.String(),
					s.apiURL,
					s.apiURL, wakeup.SquadID.String(),
					s.apiURL,
				)
				envVars["ARI_PROMPT"] = prompt
			}
		}
	}

	if systemPrompt != "" {
		envVars["ARI_SYSTEM_PROMPT"] = systemPrompt
	}

	// Inject approval gate info into the system prompt (M-4: both task and conversation paths)
	if prompt, ok := envVars["ARI_PROMPT"]; ok {
		squad, err := s.queries.GetSquadByID(ctx, wakeup.SquadID)
		if err == nil {
			var settings domain.SquadSettings
			_ = json.Unmarshal(squad.Settings, &settings)
			if len(settings.ApprovalGates) > 0 {
				gateList := "\nActions requiring approval:\n"
				for _, g := range settings.ApprovalGates {
					gateList += fmt.Sprintf("- %s (pattern: %s, timeout: %dh)\n",
						g.Name, g.ActionPattern, g.TimeoutHours)
				}
				gateList += "\nBefore performing any of these actions, create an approval request via POST /api/squads/{squadId}/inbox with category='approval'.\n"
				gateList += "Use GET /api/agent/me/gates for the full gate configuration.\n"
				envVars["ARI_PROMPT"] = prompt + gateList
			}
		}
	}

	// Inject squad secrets as ARI_SECRET_* env vars
	if s.secretsSvc != nil {
		secretMap, err := s.secretsSvc.GetDecryptedSecrets(ctx, wakeup.SquadID)
		if err != nil {
			slog.Warn("failed to load secrets for injection", "squad_id", wakeup.SquadID, "error", err)
		} else {
			injectedNames := make([]string, 0, len(secretMap))
			for name, value := range secretMap {
				envKey := "ARI_SECRET_" + name
				// Don't override core ARI_* vars
				if _, exists := envVars[envKey]; !exists {
					envVars[envKey] = value
					injectedNames = append(injectedNames, name)
				}
			}
			if len(injectedNames) > 0 {
				sort.Strings(injectedNames)
				slog.Debug("injecting secrets into agent run",
					"squad_id", wakeup.SquadID,
					"agent_id", wakeup.AgentID,
					"secret_names", strings.Join(injectedNames, ", "),
					"count", len(injectedNames))
			}
		}
	}

	// Resolve squad name for prompt template and context
	squadName := wakeup.SquadID.String()
	if squad, sqErr := s.queries.GetSquadByID(ctx, wakeup.SquadID); sqErr == nil {
		squadName = squad.Name
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
			ID:   wakeup.SquadID,
			Name: squadName,
		},
		Run: adapter.RunContext{
			RunID:        run.ID,
			WakeReason:   string(wakeup.InvocationSource),
			TaskID:       taskID,
			SessionState: sessionBefore,
		},
		EnvVars:      envVars,
		Conversation: conversation,
	}
}

// formatMessageThread formats a list of issue comments into a human-readable message thread.
func formatMessageThread(comments []db.IssueComment) string {
	var sb strings.Builder
	for _, c := range comments {
		role := string(c.AuthorType)
		ts := c.CreatedAt.Format(time.RFC3339)
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", ts, role, c.Body))
	}
	return sb.String()
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
