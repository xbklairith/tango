package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// BudgetEnforcementService handles cost recording and budget threshold enforcement.
type BudgetEnforcementService struct {
	queries *db.Queries
	dbConn  *sql.DB
}

// NewBudgetEnforcementService creates a new BudgetEnforcementService.
func NewBudgetEnforcementService(q *db.Queries, dbConn *sql.DB) *BudgetEnforcementService {
	return &BudgetEnforcementService{queries: q, dbConn: dbConn}
}

// EnforcementResult contains the outcome of recording a cost event and checking thresholds.
type EnforcementResult struct {
	CostEvent      db.CostEvent
	AgentThreshold domain.ThresholdStatus
	SquadThreshold domain.ThresholdStatus
	AgentPaused    bool
}

// RecordAndEnforce inserts a cost event and checks agent + squad budget thresholds.
// If the agent exceeds its budget, the agent is auto-paused.
// If the squad exceeds its budget, all running/idle agents in the squad are auto-paused.
func (s *BudgetEnforcementService) RecordAndEnforce(ctx context.Context, params db.InsertCostEventParams) (*EnforcementResult, error) {
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	// 1. Insert the cost event
	costEvent, err := qtx.InsertCostEvent(ctx, params)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	periodStart, periodEnd := domain.BillingPeriod(now)

	result := &EnforcementResult{
		CostEvent:      costEvent,
		AgentThreshold: domain.ThresholdOK,
		SquadThreshold: domain.ThresholdOK,
	}

	// 2. Check agent budget
	agent, err := qtx.GetAgentByID(ctx, params.AgentID)
	if err != nil {
		return nil, err
	}

	if agent.BudgetMonthlyCents.Valid && agent.BudgetMonthlyCents.Int64 > 0 {
		agentSpend, err := qtx.GetAgentMonthlySpend(ctx, db.GetAgentMonthlySpendParams{
			AgentID:     params.AgentID,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
		})
		if err != nil {
			return nil, err
		}

		budgetVal := agent.BudgetMonthlyCents.Int64
		threshold, pct := domain.ComputeThreshold(&budgetVal, agentSpend)
		result.AgentThreshold = threshold

		if threshold == domain.ThresholdExceeded {
			// Auto-pause the agent
			_, err := qtx.UpdateAgent(ctx, db.UpdateAgentParams{
				ID:     params.AgentID,
				Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
			})
			if err != nil {
				slog.Error("failed to auto-pause agent on budget exceeded",
					"agent_id", params.AgentID, "error", err)
			} else {
				result.AgentPaused = true
				slog.Warn("agent auto-paused due to budget exceeded",
					"agent_id", params.AgentID, "spent", agentSpend,
					"budget", budgetVal, "percent", pct)
			}
		} else if threshold == domain.ThresholdWarning {
			slog.Warn("agent budget warning: 80% threshold reached",
				"agent_id", params.AgentID, "spent", agentSpend,
				"budget", budgetVal, "percent", pct)
		}
	}

	// 3. Check squad budget
	squad, err := qtx.GetSquadByID(ctx, params.SquadID)
	if err != nil {
		return nil, err
	}

	if squad.BudgetMonthlyCents.Valid && squad.BudgetMonthlyCents.Int64 > 0 {
		squadSpend, err := qtx.GetSquadMonthlySpend(ctx, db.GetSquadMonthlySpendParams{
			SquadID:     params.SquadID,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
		})
		if err != nil {
			return nil, err
		}

		budgetVal := squad.BudgetMonthlyCents.Int64
		threshold, pct := domain.ComputeThreshold(&budgetVal, squadSpend)
		result.SquadThreshold = threshold

		if threshold == domain.ThresholdExceeded {
			// Auto-pause all running/idle agents in the squad
			activeAgents, err := qtx.ListRunningIdleAgentsBySquad(ctx, params.SquadID)
			if err != nil {
				slog.Error("failed to list active agents for squad budget enforcement",
					"squad_id", params.SquadID, "error", err)
			} else {
				for _, a := range activeAgents {
					_, err := qtx.UpdateAgent(ctx, db.UpdateAgentParams{
						ID:     a.ID,
						Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
					})
					if err != nil {
						slog.Error("failed to auto-pause agent on squad budget exceeded",
							"agent_id", a.ID, "error", err)
					}
				}
				slog.Warn("squad budget exceeded: all active agents paused",
					"squad_id", params.SquadID, "agents_paused", len(activeAgents),
					"spent", squadSpend, "budget", budgetVal, "percent", pct)
			}
		} else if threshold == domain.ThresholdWarning {
			slog.Warn("squad budget warning: 80% threshold reached",
				"squad_id", params.SquadID, "spent", squadSpend,
				"budget", budgetVal, "percent", pct)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return result, nil
}

// ReEvaluateAgent checks an agent's budget status and pauses it if exceeded.
func (s *BudgetEnforcementService) ReEvaluateAgent(ctx context.Context, agentID uuid.UUID) (domain.ThresholdStatus, error) {
	agent, err := s.queries.GetAgentByID(ctx, agentID)
	if err != nil {
		return domain.ThresholdOK, err
	}

	if !agent.BudgetMonthlyCents.Valid || agent.BudgetMonthlyCents.Int64 <= 0 {
		return domain.ThresholdOK, nil
	}

	now := time.Now().UTC()
	periodStart, periodEnd := domain.BillingPeriod(now)

	spend, err := s.queries.GetAgentMonthlySpend(ctx, db.GetAgentMonthlySpendParams{
		AgentID:     agentID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	})
	if err != nil {
		return domain.ThresholdOK, err
	}

	budgetVal := agent.BudgetMonthlyCents.Int64
	threshold, _ := domain.ComputeThreshold(&budgetVal, spend)

	if threshold == domain.ThresholdExceeded {
		// Auto-pause if running or idle
		if domain.AgentStatus(agent.Status) == domain.AgentStatusRunning ||
			domain.AgentStatus(agent.Status) == domain.AgentStatusIdle {
			_, err := s.queries.UpdateAgent(ctx, db.UpdateAgentParams{
				ID:     agentID,
				Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
			})
			if err != nil {
				return threshold, err
			}
			slog.Warn("agent re-evaluated and auto-paused due to budget exceeded",
				"agent_id", agentID)
		}
	}

	return threshold, nil
}

// ReEvaluateSquad checks a squad's budget status and pauses active agents if exceeded.
func (s *BudgetEnforcementService) ReEvaluateSquad(ctx context.Context, squadID uuid.UUID) (domain.ThresholdStatus, error) {
	squad, err := s.queries.GetSquadByID(ctx, squadID)
	if err != nil {
		return domain.ThresholdOK, err
	}

	if !squad.BudgetMonthlyCents.Valid || squad.BudgetMonthlyCents.Int64 <= 0 {
		return domain.ThresholdOK, nil
	}

	now := time.Now().UTC()
	periodStart, periodEnd := domain.BillingPeriod(now)

	spend, err := s.queries.GetSquadMonthlySpend(ctx, db.GetSquadMonthlySpendParams{
		SquadID:     squadID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	})
	if err != nil {
		return domain.ThresholdOK, err
	}

	budgetVal := squad.BudgetMonthlyCents.Int64
	threshold, _ := domain.ComputeThreshold(&budgetVal, spend)

	if threshold == domain.ThresholdExceeded {
		activeAgents, err := s.queries.ListRunningIdleAgentsBySquad(ctx, squadID)
		if err != nil {
			return threshold, err
		}
		for _, a := range activeAgents {
			_, err := s.queries.UpdateAgent(ctx, db.UpdateAgentParams{
				ID:     a.ID,
				Status: db.NullAgentStatus{AgentStatus: db.AgentStatusPaused, Valid: true},
			})
			if err != nil {
				slog.Error("failed to auto-pause agent during squad re-evaluation",
					"agent_id", a.ID, "error", err)
			}
		}
	}

	return threshold, nil
}

// CheckResumeAllowed checks whether an agent is allowed to resume (transition from paused to active).
// Returns an error if the agent's or squad's budget is exceeded.
func (s *BudgetEnforcementService) CheckResumeAllowed(ctx context.Context, agentID uuid.UUID) error {
	agent, err := s.queries.GetAgentByID(ctx, agentID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	periodStart, periodEnd := domain.BillingPeriod(now)

	// Check agent budget
	if agent.BudgetMonthlyCents.Valid && agent.BudgetMonthlyCents.Int64 > 0 {
		spend, err := s.queries.GetAgentMonthlySpend(ctx, db.GetAgentMonthlySpendParams{
			AgentID:     agentID,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
		})
		if err != nil {
			return err
		}

		budgetVal := agent.BudgetMonthlyCents.Int64
		threshold, _ := domain.ComputeThreshold(&budgetVal, spend)
		if threshold == domain.ThresholdExceeded {
			return &BudgetExceededError{
				Entity: "agent",
				ID:     agentID,
			}
		}
	}

	// Check squad budget
	squad, err := s.queries.GetSquadByID(ctx, agent.SquadID)
	if err != nil {
		return err
	}

	if squad.BudgetMonthlyCents.Valid && squad.BudgetMonthlyCents.Int64 > 0 {
		spend, err := s.queries.GetSquadMonthlySpend(ctx, db.GetSquadMonthlySpendParams{
			SquadID:     agent.SquadID,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
		})
		if err != nil {
			return err
		}

		budgetVal := squad.BudgetMonthlyCents.Int64
		threshold, _ := domain.ComputeThreshold(&budgetVal, spend)
		if threshold == domain.ThresholdExceeded {
			return &BudgetExceededError{
				Entity: "squad",
				ID:     agent.SquadID,
			}
		}
	}

	return nil
}

// BudgetExceededError is returned when a budget threshold prevents an operation.
type BudgetExceededError struct {
	Entity string
	ID     uuid.UUID
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("%s budget exceeded; cannot resume agent until budget is increased or new billing period starts", e.Entity)
}
