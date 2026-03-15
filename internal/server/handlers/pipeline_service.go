package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// PipelineService handles pipeline lifecycle, stage transitions, and side-effects.
type PipelineService struct {
	queries   *db.Queries
	dbConn    *sql.DB
	sseHub    *sse.Hub
	wakeupSvc *WakeupService
}

// NewPipelineService creates a new PipelineService.
func NewPipelineService(q *db.Queries, dbConn *sql.DB, sseHub *sse.Hub, wakeupSvc *WakeupService) *PipelineService {
	return &PipelineService{
		queries:   q,
		dbConn:    dbConn,
		sseHub:    sseHub,
		wakeupSvc: wakeupSvc,
	}
}

// ---------- Pipeline CRUD ----------

// CreatePipeline creates a pipeline within a transaction, logs activity, and emits SSE.
func (s *PipelineService) CreatePipeline(ctx context.Context, squadID uuid.UUID, userID uuid.UUID, req domain.CreatePipelineRequest) (*db.Pipeline, error) {
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("pipeline create: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	p, err := qtx.CreatePipeline(ctx, db.CreatePipelineParams{
		SquadID:     squadID,
		Name:        req.Name,
		Description: nullString(req.Description),
	})
	if err != nil {
		return nil, fmt.Errorf("pipeline create: insert: %w", err)
	}

	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    userID,
		Action:     "pipeline.created",
		EntityType: "pipeline",
		EntityID:   p.ID,
		Metadata:   map[string]any{"name": p.Name},
	}); err != nil {
		return nil, fmt.Errorf("pipeline create: log activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("pipeline create: commit: %w", err)
	}

	if s.sseHub != nil {
		s.sseHub.Publish(squadID, "pipeline.created", map[string]any{
			"pipelineId": p.ID, "name": p.Name,
		})
	}

	return &p, nil
}

// UpdatePipeline updates a pipeline.
func (s *PipelineService) UpdatePipeline(ctx context.Context, pipelineID uuid.UUID, userID uuid.UUID, req domain.UpdatePipelineRequest) (*db.Pipeline, error) {
	params := db.UpdatePipelineParams{
		ID:             pipelineID,
		SetDescription: req.SetDescription,
	}
	if req.Name != nil {
		params.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	if req.Description != nil {
		params.Description = sql.NullString{String: *req.Description, Valid: true}
	}
	if req.IsActive != nil {
		params.IsActive = sql.NullBool{Bool: *req.IsActive, Valid: true}
	}

	p, err := s.queries.UpdatePipeline(ctx, params)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("pipeline not found")
		}
		return nil, fmt.Errorf("pipeline update: %w", err)
	}

	if s.sseHub != nil {
		s.sseHub.Publish(p.SquadID, "pipeline.updated", map[string]any{
			"pipelineId": p.ID, "name": p.Name,
		})
	}

	return &p, nil
}

// DeletePipeline deletes a pipeline if no issues are attached.
func (s *PipelineService) DeletePipeline(ctx context.Context, pipelineID uuid.UUID, userID uuid.UUID) error {
	p, err := s.queries.GetPipelineByID(ctx, pipelineID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("pipeline not found")
		}
		return fmt.Errorf("pipeline delete: get: %w", err)
	}

	count, err := s.queries.CountIssuesInPipeline(ctx, uuid.NullUUID{UUID: pipelineID, Valid: true})
	if err != nil {
		return fmt.Errorf("pipeline delete: count issues: %w", err)
	}
	if count > 0 {
		return &PipelineInUseError{PipelineID: pipelineID, IssueCount: count}
	}

	if err := s.queries.DeletePipeline(ctx, pipelineID); err != nil {
		return fmt.Errorf("pipeline delete: %w", err)
	}

	if s.sseHub != nil {
		s.sseHub.Publish(p.SquadID, "pipeline.deleted", map[string]any{
			"pipelineId": pipelineID,
		})
	}

	return nil
}

// ---------- Stage CRUD ----------

// CreateStage creates a stage in a pipeline, validating agent squad membership.
func (s *PipelineService) CreateStage(ctx context.Context, pipelineID uuid.UUID, req domain.CreateStageRequest) (*db.PipelineStage, error) {
	pipeline, err := s.queries.GetPipelineByID(ctx, pipelineID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("pipeline not found")
		}
		return nil, fmt.Errorf("stage create: get pipeline: %w", err)
	}

	// Validate agent is in the same squad if assigned.
	if req.AssignedAgentID != nil {
		agent, err := s.queries.GetAgentByID(ctx, *req.AssignedAgentID)
		if err != nil {
			return nil, fmt.Errorf("stage create: get agent: %w", err)
		}
		if agent.SquadID != pipeline.SquadID {
			return nil, &AgentSquadMismatchError{AgentID: *req.AssignedAgentID, SquadID: pipeline.SquadID}
		}
	}

	stage, err := s.queries.CreatePipelineStage(ctx, db.CreatePipelineStageParams{
		PipelineID:      pipelineID,
		Name:            req.Name,
		Description:     nullString(req.Description),
		Position:        int32(req.Position),
		AssignedAgentID: nullUUID(req.AssignedAgentID),
	})
	if err != nil {
		return nil, fmt.Errorf("stage create: insert: %w", err)
	}

	return &stage, nil
}

// UpdateStage updates a stage, validating agent squad membership.
func (s *PipelineService) UpdateStage(ctx context.Context, stageID uuid.UUID, req domain.UpdateStageRequest) (*db.PipelineStage, error) {
	existing, err := s.queries.GetPipelineStageByID(ctx, stageID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("stage not found")
		}
		return nil, fmt.Errorf("stage update: get: %w", err)
	}

	// Validate agent squad match if changing assigned agent.
	if req.SetAgent && req.AssignedAgentID != nil {
		pipeline, err := s.queries.GetPipelineByID(ctx, existing.PipelineID)
		if err != nil {
			return nil, fmt.Errorf("stage update: get pipeline: %w", err)
		}
		agent, err := s.queries.GetAgentByID(ctx, *req.AssignedAgentID)
		if err != nil {
			return nil, fmt.Errorf("stage update: get agent: %w", err)
		}
		if agent.SquadID != pipeline.SquadID {
			return nil, &AgentSquadMismatchError{AgentID: *req.AssignedAgentID, SquadID: pipeline.SquadID}
		}
	}

	params := db.UpdatePipelineStageParams{
		ID:             stageID,
		SetDescription: req.SetDescription,
		SetAgent:       req.SetAgent,
	}
	if req.Name != nil {
		params.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	if req.Position != nil {
		params.Position = sql.NullInt32{Int32: int32(*req.Position), Valid: true}
	}
	if req.AssignedAgentID != nil {
		params.AssignedAgentID = uuid.NullUUID{UUID: *req.AssignedAgentID, Valid: true}
	}

	stage, err := s.queries.UpdatePipelineStage(ctx, params)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("stage not found")
		}
		return nil, fmt.Errorf("stage update: %w", err)
	}

	return &stage, nil
}

// DeleteStage deletes a stage if no issues are at it.
func (s *PipelineService) DeleteStage(ctx context.Context, stageID uuid.UUID) error {
	count, err := s.queries.CountIssuesAtStage(ctx, uuid.NullUUID{UUID: stageID, Valid: true})
	if err != nil {
		return fmt.Errorf("stage delete: count issues: %w", err)
	}
	if count > 0 {
		return &StageInUseError{StageID: stageID, IssueCount: count}
	}

	if err := s.queries.DeletePipelineStage(ctx, stageID); err != nil {
		return fmt.Errorf("stage delete: %w", err)
	}

	return nil
}

// ---------- Issue Pipeline Workflow ----------

// AttachIssueToPipeline attaches an issue to a pipeline at its first stage.
func (s *PipelineService) AttachIssueToPipeline(ctx context.Context, issueID, pipelineID uuid.UUID, userID uuid.UUID) (*db.Issue, error) {
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("attach: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	issue, err := qtx.GetIssueByID(ctx, issueID)
	if err != nil {
		return nil, fmt.Errorf("attach: get issue: %w", err)
	}

	pipeline, err := qtx.GetPipelineByID(ctx, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("attach: get pipeline: %w", err)
	}

	if issue.SquadID != pipeline.SquadID {
		return nil, &PipelineSquadMismatchError{}
	}

	if !pipeline.IsActive {
		return nil, &PipelineInactiveError{PipelineID: pipelineID}
	}

	firstStage, err := qtx.GetFirstStage(ctx, pipelineID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &PipelineEmptyError{PipelineID: pipelineID}
		}
		return nil, fmt.Errorf("attach: get first stage: %w", err)
	}

	// Update issue: set pipeline, stage, assignee, status=todo
	updated, err := qtx.UpdateIssuePipeline(ctx, db.UpdateIssuePipelineParams{
		ID:              issueID,
		PipelineID:      uuid.NullUUID{UUID: pipelineID, Valid: true},
		CurrentStageID:  uuid.NullUUID{UUID: firstStage.ID, Valid: true},
		SetAssignee:     firstStage.AssignedAgentID.Valid,
		AssigneeAgentID: firstStage.AssignedAgentID,
		SetStatus:       true,
		Status:          db.NullIssueStatus{IssueStatus: db.IssueStatusTodo, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("attach: update issue: %w", err)
	}

	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    issue.SquadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    userID,
		Action:     "issue.pipeline.attached",
		EntityType: "issue",
		EntityID:   issueID,
		Metadata: map[string]any{
			"pipelineId": pipelineID,
			"stageId":    firstStage.ID,
			"stageName":  firstStage.Name,
		},
	}); err != nil {
		return nil, fmt.Errorf("attach: log activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("attach: commit: %w", err)
	}

	// SSE + wakeup (after commit)
	if s.sseHub != nil {
		s.sseHub.Publish(issue.SquadID, "issue.pipeline.stage_changed", map[string]any{
			"issueId":    issueID,
			"pipelineId": pipelineID,
			"stageId":    firstStage.ID,
			"stageName":  firstStage.Name,
			"transition": "attach",
		})
	}

	if firstStage.AssignedAgentID.Valid && s.wakeupSvc != nil {
		_, _ = s.wakeupSvc.Enqueue(ctx, firstStage.AssignedAgentID.UUID, issue.SquadID, "assignment", map[string]any{
			"issue_id":    issueID.String(),
			"pipeline_id": pipelineID.String(),
			"stage_name":  firstStage.Name,
			"transition":  "attach",
		})
	}

	return &updated, nil
}

// DetachIssueFromPipeline removes an issue from its pipeline.
func (s *PipelineService) DetachIssueFromPipeline(ctx context.Context, issueID uuid.UUID, userID uuid.UUID) (*db.Issue, error) {
	updated, err := s.queries.UpdateIssuePipeline(ctx, db.UpdateIssuePipelineParams{
		ID:             issueID,
		PipelineID:     uuid.NullUUID{},
		CurrentStageID: uuid.NullUUID{},
	})
	if err != nil {
		return nil, fmt.Errorf("detach: update issue: %w", err)
	}

	return &updated, nil
}

// AdvanceStage advances an issue to the next pipeline stage.
// Uses a CAS guard on current_stage_id to prevent concurrent double-advancement.
func (s *PipelineService) AdvanceStage(ctx context.Context, issueID uuid.UUID, userID uuid.UUID) (*db.Issue, error) {
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("advance: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	issue, err := qtx.GetIssueByID(ctx, issueID)
	if err != nil {
		return nil, fmt.Errorf("advance: get issue: %w", err)
	}

	if !issue.PipelineID.Valid || !issue.CurrentStageID.Valid {
		return nil, &NotInPipelineError{IssueID: issueID}
	}

	currentStage, err := qtx.GetPipelineStageByID(ctx, issue.CurrentStageID.UUID)
	if err != nil {
		return nil, fmt.Errorf("advance: get current stage: %w", err)
	}

	// Try to get the next stage
	nextStage, err := qtx.GetNextStage(ctx, db.GetNextStageParams{
		PipelineID:      issue.PipelineID.UUID,
		CurrentPosition: currentStage.Position,
	})

	if err == sql.ErrNoRows {
		// Final stage — mark pipeline complete
		updated, err := qtx.AdvanceIssuePipelineStage(ctx, db.AdvanceIssuePipelineStageParams{
			ID:              issueID,
			ExpectedStageID: issue.CurrentStageID,
			NextStageID:     uuid.NullUUID{}, // clear
			SetStatus:       true,
			Status:          db.NullIssueStatus{IssueStatus: db.IssueStatusDone, Valid: true},
		})
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, &ConcurrentAdvanceError{IssueID: issueID}
			}
			return nil, fmt.Errorf("advance: complete: %w", err)
		}

		if err := logActivity(ctx, qtx, ActivityParams{
			SquadID:    issue.SquadID,
			ActorType:  domain.ActivityActorUser,
			ActorID:    userID,
			Action:     "issue.pipeline.completed",
			EntityType: "issue",
			EntityID:   issueID,
			Metadata: map[string]any{
				"pipelineId": issue.PipelineID.UUID,
				"lastStage":  currentStage.Name,
			},
		}); err != nil {
			return nil, fmt.Errorf("advance: log activity: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("advance: commit: %w", err)
		}

		if s.sseHub != nil {
			s.sseHub.Publish(issue.SquadID, "issue.pipeline.stage_changed", map[string]any{
				"issueId":    issueID,
				"pipelineId": issue.PipelineID.UUID,
				"transition": "complete",
			})
		}

		return &updated, nil
	}
	if err != nil {
		return nil, fmt.Errorf("advance: get next stage: %w", err)
	}

	// Advance to next stage — CAS on current_stage_id
	updated, err := qtx.AdvanceIssuePipelineStage(ctx, db.AdvanceIssuePipelineStageParams{
		ID:              issueID,
		ExpectedStageID: issue.CurrentStageID,
		NextStageID:     uuid.NullUUID{UUID: nextStage.ID, Valid: true},
		SetAssignee:     nextStage.AssignedAgentID.Valid,
		AssigneeAgentID: nextStage.AssignedAgentID,
		SetStatus:       true,
		Status:          db.NullIssueStatus{IssueStatus: db.IssueStatusTodo, Valid: true},
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &ConcurrentAdvanceError{IssueID: issueID}
		}
		return nil, fmt.Errorf("advance: update: %w", err)
	}

	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    issue.SquadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    userID,
		Action:     "issue.pipeline.advanced",
		EntityType: "issue",
		EntityID:   issueID,
		Metadata: map[string]any{
			"pipelineId": issue.PipelineID.UUID,
			"fromStage":  currentStage.Name,
			"toStage":    nextStage.Name,
		},
	}); err != nil {
		return nil, fmt.Errorf("advance: log activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("advance: commit: %w", err)
	}

	if s.sseHub != nil {
		s.sseHub.Publish(issue.SquadID, "issue.pipeline.stage_changed", map[string]any{
			"issueId":    issueID,
			"pipelineId": issue.PipelineID.UUID,
			"fromStage":  currentStage.Name,
			"toStage":    nextStage.Name,
			"transition": "advance",
		})
	}

	if nextStage.AssignedAgentID.Valid && s.wakeupSvc != nil {
		_, _ = s.wakeupSvc.Enqueue(ctx, nextStage.AssignedAgentID.UUID, issue.SquadID, "assignment", map[string]any{
			"issue_id":    issueID.String(),
			"pipeline_id": issue.PipelineID.UUID.String(),
			"stage_name":  nextStage.Name,
			"transition":  "advance",
		})
	}

	return &updated, nil
}

// RejectStage rejects an issue back to the previous pipeline stage.
func (s *PipelineService) RejectStage(ctx context.Context, issueID uuid.UUID, userID uuid.UUID, reason *string) (*db.Issue, error) {
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("reject: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	issue, err := qtx.GetIssueByID(ctx, issueID)
	if err != nil {
		return nil, fmt.Errorf("reject: get issue: %w", err)
	}

	if !issue.PipelineID.Valid || !issue.CurrentStageID.Valid {
		return nil, &NotInPipelineError{IssueID: issueID}
	}

	currentStage, err := qtx.GetPipelineStageByID(ctx, issue.CurrentStageID.UUID)
	if err != nil {
		return nil, fmt.Errorf("reject: get current stage: %w", err)
	}

	prevStage, err := qtx.GetPreviousStage(ctx, db.GetPreviousStageParams{
		PipelineID:      issue.PipelineID.UUID,
		CurrentPosition: currentStage.Position,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &NoPreviousStageError{IssueID: issueID}
		}
		return nil, fmt.Errorf("reject: get previous stage: %w", err)
	}

	// CAS update to previous stage
	updated, err := qtx.AdvanceIssuePipelineStage(ctx, db.AdvanceIssuePipelineStageParams{
		ID:              issueID,
		ExpectedStageID: issue.CurrentStageID,
		NextStageID:     uuid.NullUUID{UUID: prevStage.ID, Valid: true},
		SetAssignee:     prevStage.AssignedAgentID.Valid,
		AssigneeAgentID: prevStage.AssignedAgentID,
		SetStatus:       true,
		Status:          db.NullIssueStatus{IssueStatus: db.IssueStatusTodo, Valid: true},
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &ConcurrentAdvanceError{IssueID: issueID}
		}
		return nil, fmt.Errorf("reject: update: %w", err)
	}

	// Add rejection reason as system comment if provided
	if reason != nil && *reason != "" {
		_, err := qtx.CreateIssueComment(ctx, db.CreateIssueCommentParams{
			IssueID:    issueID,
			AuthorType: db.CommentAuthorTypeSystem,
			AuthorID:   userID,
			Body:       fmt.Sprintf("Rejected from %q: %s", currentStage.Name, *reason),
		})
		if err != nil {
			slog.Error("reject: failed to create comment", "error", err)
		}
	}

	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    issue.SquadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    userID,
		Action:     "issue.pipeline.rejected",
		EntityType: "issue",
		EntityID:   issueID,
		Metadata: map[string]any{
			"pipelineId": issue.PipelineID.UUID,
			"fromStage":  currentStage.Name,
			"toStage":    prevStage.Name,
			"reason":     reason,
		},
	}); err != nil {
		return nil, fmt.Errorf("reject: log activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("reject: commit: %w", err)
	}

	if s.sseHub != nil {
		s.sseHub.Publish(issue.SquadID, "issue.pipeline.stage_changed", map[string]any{
			"issueId":    issueID,
			"pipelineId": issue.PipelineID.UUID,
			"fromStage":  currentStage.Name,
			"toStage":    prevStage.Name,
			"transition": "reject",
		})
	}

	if prevStage.AssignedAgentID.Valid && s.wakeupSvc != nil {
		_, _ = s.wakeupSvc.Enqueue(ctx, prevStage.AssignedAgentID.UUID, issue.SquadID, "assignment", map[string]any{
			"issue_id":    issueID.String(),
			"pipeline_id": issue.PipelineID.UUID.String(),
			"stage_name":  prevStage.Name,
			"transition":  "reject",
		})
	}

	return &updated, nil
}

// AutoAdvanceOnDone is called when an issue's status transitions to done.
// If the issue is in a pipeline, it advances to the next stage.
// Returns (true, issue, nil) if handled, (false, nil, nil) if not in a pipeline.
func (s *PipelineService) AutoAdvanceOnDone(ctx context.Context, issueID uuid.UUID, userID uuid.UUID) (bool, *db.Issue, error) {
	issue, err := s.queries.GetIssueByID(ctx, issueID)
	if err != nil {
		return false, nil, fmt.Errorf("auto-advance: get issue: %w", err)
	}

	if !issue.PipelineID.Valid {
		return false, nil, nil
	}

	updated, err := s.AdvanceStage(ctx, issueID, userID)
	if err != nil {
		return true, nil, err
	}

	return true, updated, nil
}

// ---------- Error Types ----------

// PipelineInUseError indicates a pipeline cannot be deleted because issues are attached.
type PipelineInUseError struct {
	PipelineID uuid.UUID
	IssueCount int64
}

func (e *PipelineInUseError) Error() string {
	return fmt.Sprintf("pipeline %s has %d attached issues", e.PipelineID, e.IssueCount)
}

// StageInUseError indicates a stage cannot be deleted because issues are at it.
type StageInUseError struct {
	StageID    uuid.UUID
	IssueCount int64
}

func (e *StageInUseError) Error() string {
	return fmt.Sprintf("stage %s has %d issues", e.StageID, e.IssueCount)
}

// AgentSquadMismatchError indicates the agent is not in the pipeline's squad.
type AgentSquadMismatchError struct {
	AgentID uuid.UUID
	SquadID uuid.UUID
}

func (e *AgentSquadMismatchError) Error() string {
	return fmt.Sprintf("agent %s is not in squad %s", e.AgentID, e.SquadID)
}

// PipelineSquadMismatchError indicates the issue and pipeline are in different squads.
type PipelineSquadMismatchError struct{}

func (e *PipelineSquadMismatchError) Error() string {
	return "issue and pipeline are in different squads"
}

// PipelineInactiveError indicates the pipeline is not active.
type PipelineInactiveError struct {
	PipelineID uuid.UUID
}

func (e *PipelineInactiveError) Error() string {
	return fmt.Sprintf("pipeline %s is not active", e.PipelineID)
}

// PipelineEmptyError indicates the pipeline has no stages.
type PipelineEmptyError struct {
	PipelineID uuid.UUID
}

func (e *PipelineEmptyError) Error() string {
	return fmt.Sprintf("pipeline %s has no stages", e.PipelineID)
}

// NotInPipelineError indicates the issue is not attached to a pipeline.
type NotInPipelineError struct {
	IssueID uuid.UUID
}

func (e *NotInPipelineError) Error() string {
	return fmt.Sprintf("issue %s is not in a pipeline", e.IssueID)
}

// NoPreviousStageError indicates there is no previous stage to reject to.
type NoPreviousStageError struct {
	IssueID uuid.UUID
}

func (e *NoPreviousStageError) Error() string {
	return fmt.Sprintf("issue %s is at the first stage, cannot reject", e.IssueID)
}

// ConcurrentAdvanceError indicates a concurrent stage transition was detected.
type ConcurrentAdvanceError struct {
	IssueID uuid.UUID
}

func (e *ConcurrentAdvanceError) Error() string {
	return fmt.Sprintf("concurrent stage transition detected for issue %s", e.IssueID)
}

// ---------- Helpers ----------

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullUUID(u *uuid.UUID) uuid.NullUUID {
	if u == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *u, Valid: true}
}
