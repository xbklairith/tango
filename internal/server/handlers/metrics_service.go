package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
)

// --- Response Types ---

// DashboardMetricsResponse is the top-level metrics response returned by the endpoint.
type DashboardMetricsResponse struct {
	CostTrends    CostTrendSection    `json:"costTrends"`
	AgentHealth   []AgentHealthRecord `json:"agentHealth"`
	IssueVelocity IssueVelocitySection `json:"issueVelocity"`
}

// CostTrendSection holds aggregated cost data for the selected time range.
type CostTrendSection struct {
	TotalCents  int64             `json:"totalCents"`
	Granularity string            `json:"granularity"`
	Data        []CostTrendBucket `json:"data"`
}

// CostTrendBucket is one time bucket in the cost trend.
type CostTrendBucket struct {
	Bucket     string `json:"bucket"`
	TotalCents int64  `json:"totalCents"`
	EventCount int64  `json:"eventCount"`
}

// AgentHealthRecord holds health metrics for a single agent.
type AgentHealthRecord struct {
	AgentID       string  `json:"agentId"`
	AgentName     string  `json:"agentName"`
	Status        string  `json:"status"`
	TotalRuns     int64   `json:"totalRuns"`
	SuccessfulRuns int64  `json:"successfulRuns"`
	FailedRuns    int64   `json:"failedRuns"`
	ErrorRate     float64 `json:"errorRate"`
	LastRunAt     *string `json:"lastRunAt"`
	LastRunStatus *string `json:"lastRunStatus"`
}

// IssueVelocitySection holds daily created/closed issue counts.
type IssueVelocitySection struct {
	TotalCreated int64              `json:"totalCreated"`
	TotalClosed  int64              `json:"totalClosed"`
	Data         []IssueVelocityDay `json:"data"`
}

// IssueVelocityDay is one day's created/closed counts.
type IssueVelocityDay struct {
	Date    string `json:"date"`
	Created int64  `json:"created"`
	Closed  int64  `json:"closed"`
}

// MetricsService orchestrates concurrent metric queries.
type MetricsService struct {
	queries *db.Queries
}

// NewMetricsService creates a new MetricsService.
func NewMetricsService(q *db.Queries) *MetricsService {
	return &MetricsService{queries: q}
}

// computeTimeRange returns start and end times for the given range string.
// Accepts a `now` parameter for testability.
func computeTimeRange(now time.Time, rangeParam string) (start, end time.Time) {
	end = now.UTC()
	switch rangeParam {
	case "7d":
		start = end.AddDate(0, 0, -7)
	case "90d":
		start = end.AddDate(0, 0, -90)
	default: // "30d" and fallback
		start = end.AddDate(0, 0, -30)
	}
	return start, end
}

// granularityForRange returns the auto-selected granularity for a range.
func granularityForRange(rangeParam string) string {
	switch rangeParam {
	case "90d":
		return "week"
	default: // 7d, 30d
		return "day"
	}
}

// GetMetrics fetches all dashboard metrics concurrently.
func (s *MetricsService) GetMetrics(ctx context.Context, squadID uuid.UUID, timeRange string, now time.Time) (*DashboardMetricsResponse, error) {
	start, end := computeTimeRange(now, timeRange)
	granularity := granularityForRange(timeRange)

	var (
		costSection    CostTrendSection
		agentHealth    []AgentHealthRecord
		velocitySection IssueVelocitySection
		costErr, healthErr, velocityErr error
	)

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		costSection, costErr = s.getCostTrends(ctx, squadID, start, end, granularity)
	}()

	go func() {
		defer wg.Done()
		agentHealth, healthErr = s.getAgentHealth(ctx, squadID, start, end)
	}()

	go func() {
		defer wg.Done()
		velocitySection, velocityErr = s.getIssueVelocity(ctx, squadID, start, end)
	}()

	wg.Wait()

	for _, err := range []error{costErr, healthErr, velocityErr} {
		if err != nil {
			return nil, err
		}
	}

	return &DashboardMetricsResponse{
		CostTrends:    costSection,
		AgentHealth:   agentHealth,
		IssueVelocity: velocitySection,
	}, nil
}

// getCostTrends dispatches to the correct cost trends query based on granularity.
func (s *MetricsService) getCostTrends(ctx context.Context, squadID uuid.UUID, start, end time.Time, granularity string) (CostTrendSection, error) {
	section := CostTrendSection{
		Granularity: granularity,
		Data:        []CostTrendBucket{},
	}

	switch granularity {
	case "week":
		rows, err := s.queries.GetCostTrendsWeekly(ctx, db.GetCostTrendsWeeklyParams{
			SquadID:     squadID,
			PeriodStart: start,
			PeriodEnd:   end,
		})
		if err != nil {
			return section, err
		}
		for _, r := range rows {
			section.TotalCents += r.TotalCents
			section.Data = append(section.Data, CostTrendBucket{
				Bucket:     r.Bucket.Format(time.RFC3339),
				TotalCents: r.TotalCents,
				EventCount: r.EventCount,
			})
		}
	case "month":
		rows, err := s.queries.GetCostTrendsMonthly(ctx, db.GetCostTrendsMonthlyParams{
			SquadID:     squadID,
			PeriodStart: start,
			PeriodEnd:   end,
		})
		if err != nil {
			return section, err
		}
		for _, r := range rows {
			section.TotalCents += r.TotalCents
			section.Data = append(section.Data, CostTrendBucket{
				Bucket:     r.Bucket.Format(time.RFC3339),
				TotalCents: r.TotalCents,
				EventCount: r.EventCount,
			})
		}
	default: // "day"
		rows, err := s.queries.GetCostTrendsDaily(ctx, db.GetCostTrendsDailyParams{
			SquadID:     squadID,
			PeriodStart: start,
			PeriodEnd:   end,
		})
		if err != nil {
			return section, err
		}
		for _, r := range rows {
			section.TotalCents += r.TotalCents
			section.Data = append(section.Data, CostTrendBucket{
				Bucket:     r.Bucket.Format(time.RFC3339),
				TotalCents: r.TotalCents,
				EventCount: r.EventCount,
			})
		}
	}

	return section, nil
}

// getAgentHealth fetches agent run stats and last-run info, merging them.
func (s *MetricsService) getAgentHealth(ctx context.Context, squadID uuid.UUID, start, end time.Time) ([]AgentHealthRecord, error) {
	stats, err := s.queries.GetAgentRunStats(ctx, db.GetAgentRunStatsParams{
		SquadID:     squadID,
		PeriodStart: start,
		PeriodEnd:   end,
	})
	if err != nil {
		return nil, err
	}

	lastRuns, err := s.queries.GetAgentLastRun(ctx, squadID)
	if err != nil {
		return nil, err
	}

	// Build last-run lookup by agent ID.
	lastRunMap := make(map[uuid.UUID]db.GetAgentLastRunRow, len(lastRuns))
	for _, lr := range lastRuns {
		lastRunMap[lr.AgentID] = lr
	}

	records := make([]AgentHealthRecord, 0, len(stats))
	for _, s := range stats {
		rec := AgentHealthRecord{
			AgentID:        s.AgentID.String(),
			AgentName:      s.AgentName,
			Status:         string(s.AgentStatus),
			TotalRuns:      s.TotalRuns,
			SuccessfulRuns: s.SuccessCount,
			FailedRuns:     s.FailureCount,
		}

		if s.TotalRuns > 0 {
			rec.ErrorRate = float64(s.FailureCount) / float64(s.TotalRuns)
		}

		if lr, ok := lastRunMap[s.AgentID]; ok {
			ts := lr.CreatedAt.Format(time.RFC3339)
			st := string(lr.Status)
			rec.LastRunAt = &ts
			rec.LastRunStatus = &st
		}

		records = append(records, rec)
	}

	return records, nil
}

// getIssueVelocity fetches created/closed issue counts and produces dense daily data.
func (s *MetricsService) getIssueVelocity(ctx context.Context, squadID uuid.UUID, start, end time.Time) (IssueVelocitySection, error) {
	section := IssueVelocitySection{
		Data: []IssueVelocityDay{},
	}

	created, err := s.queries.GetIssuesCreatedPerDay(ctx, db.GetIssuesCreatedPerDayParams{
		SquadID:     squadID,
		PeriodStart: start,
		PeriodEnd:   end,
	})
	if err != nil {
		return section, err
	}

	closed, err := s.queries.GetIssuesClosedPerDay(ctx, db.GetIssuesClosedPerDayParams{
		SquadID:     squadID,
		PeriodStart: start,
		PeriodEnd:   end,
	})
	if err != nil {
		return section, err
	}

	// Build lookup maps.
	createdMap := make(map[string]int64, len(created))
	for _, c := range created {
		createdMap[c.Day.Format("2006-01-02")] = c.Count
	}

	closedMap := make(map[string]int64, len(closed))
	for _, c := range closed {
		closedMap[c.Day.Format("2006-01-02")] = c.Count
	}

	// Dense fill: iterate every day from start to end (exclusive).
	for d := truncateDay(start); d.Before(truncateDay(end)); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		c := createdMap[key]
		cl := closedMap[key]
		section.TotalCreated += c
		section.TotalClosed += cl
		section.Data = append(section.Data, IssueVelocityDay{
			Date:    key,
			Created: c,
			Closed:  cl,
		})
	}

	return section, nil
}

// truncateDay truncates a time to the start of its UTC day.
func truncateDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
