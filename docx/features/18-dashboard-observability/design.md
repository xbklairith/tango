# Design: Dashboard Observability

**Created:** 2026-03-15
**Status:** Ready for Implementation
**Feature:** 18-dashboard-observability
**Dependencies:** 09-activity-log, 10-cost-events-budget, 11-agent-runtime, 12-inbox-system

---

## 1. Architecture Overview

Dashboard Observability adds a metrics aggregation layer that reads from existing tables (cost_events, heartbeat_runs, issues, agents, inbox_items) and exposes a single endpoint returning all dashboard data. No new database tables or migrations are needed — only new SQL queries and a new handler.

### High-Level Flow

```
React Dashboard
    |
    v
GET /api/squads/{id}/metrics?range=30d&granularity=day
    |
    v
MetricsHandler
    |
    v
MetricsService.GetDashboardMetrics(ctx, squadID, range, granularity)
    |
    +--→ goroutine 1: Cost Trend Query (cost_events)
    +--→ goroutine 2: Agent Health Query (heartbeat_runs + agents)
    +--→ goroutine 3: Issue Velocity Query (issues)
    |
    v
Merge results → single JSON response
```

### Component Relationships

```
MetricsHandler            ← HTTP layer: parse params, auth, squad check
       |
       v
MetricsService            ← Orchestration: concurrent queries, assembly
       |
       +--→ sqlc Queries: GetCostTrends (cost_events)
       +--→ sqlc Queries: GetAgentHealthBySquad (heartbeat_runs + agents)
       +--→ sqlc Queries: GetIssueVelocity (issues)
       |
       v
DashboardMetricsResponse  ← Single JSON payload to React
```

### Squad Isolation

All queries filter by `squad_id`. The handler validates squad membership before calling the service layer, following the same pattern as existing squad-scoped handlers.

---

## 2. Database Queries (No New Migration)

All metrics are computed from existing tables. No schema changes are required.

### 2.1 Cost Trends Query

```sql
-- name: GetCostTrends :many
SELECT
    date_trunc(@granularity::TEXT, created_at)::TIMESTAMPTZ AS bucket,
    COALESCE(SUM(amount_cents), 0)::BIGINT AS total_cents,
    COUNT(*)::BIGINT AS event_count
FROM cost_events
WHERE squad_id = @squad_id
  AND created_at >= @period_start
  AND created_at < @period_end
GROUP BY bucket
ORDER BY bucket ASC;
```

Note: `@granularity` is one of `'day'`, `'week'`, `'month'`. The Go layer validates and passes it as a literal string — not user-controlled SQL injection risk since the handler enforces the enum.

### 2.2 Agent Health Query

Two queries are needed:

```sql
-- name: GetAgentRunStatsBySquad :many
-- Aggregates heartbeat_runs per agent within the time range.
SELECT
    hr.agent_id,
    a.name AS agent_name,
    a.short_name AS agent_short_name,
    a.status AS agent_status,
    COUNT(*)::BIGINT AS total_runs,
    COUNT(*) FILTER (WHERE hr.status = 'success')::BIGINT AS successful_runs,
    COUNT(*) FILTER (WHERE hr.status = 'failed')::BIGINT AS failed_runs
FROM heartbeat_runs hr
JOIN agents a ON a.id = hr.agent_id
WHERE hr.squad_id = @squad_id
  AND hr.created_at >= @period_start
  AND hr.created_at < @period_end
GROUP BY hr.agent_id, a.name, a.short_name, a.status
ORDER BY
    CASE WHEN COUNT(*) = 0 THEN 1 ELSE 0 END,
    COUNT(*) FILTER (WHERE hr.status = 'failed')::FLOAT / GREATEST(COUNT(*), 1) DESC;
```

```sql
-- name: GetAgentLastRun :many
-- Returns the most recent heartbeat_run per agent in a squad (regardless of time range).
SELECT DISTINCT ON (agent_id)
    agent_id,
    status AS last_run_status,
    created_at AS last_run_at
FROM heartbeat_runs
WHERE squad_id = @squad_id
ORDER BY agent_id, created_at DESC;
```

The Go layer joins these two result sets in memory by `agent_id`. Agents with no runs in the time range are included from the agents table with zero counts.

### 2.3 Issue Velocity Query

Two separate queries for created and closed issues:

```sql
-- name: GetIssuesCreatedPerDay :many
SELECT
    date_trunc('day', created_at)::DATE AS date,
    COUNT(*)::BIGINT AS count
FROM issues
WHERE squad_id = @squad_id
  AND created_at >= @period_start
  AND created_at < @period_end
GROUP BY date
ORDER BY date ASC;
```

```sql
-- name: GetIssuesClosedPerDay :many
SELECT
    date_trunc('day', updated_at)::DATE AS date,
    COUNT(*)::BIGINT AS count
FROM issues
WHERE squad_id = @squad_id
  AND status = 'done'
  AND updated_at >= @period_start
  AND updated_at < @period_end
GROUP BY date
ORDER BY date ASC;
```

The Go layer merges these sparse results into a dense daily array covering every day in the range, filling missing dates with zero.

---

## 3. API Response Schema

### `GET /api/squads/{squadId}/metrics?range=30d&granularity=day`

```json
{
  "costTrends": {
    "totalCents": 145200,
    "granularity": "day",
    "data": [
      {
        "bucket": "2026-02-13T00:00:00Z",
        "totalCents": 4500,
        "eventCount": 12
      }
    ]
  },
  "agentHealth": [
    {
      "agentId": "uuid",
      "agentName": "Code Reviewer",
      "agentShortName": "CR",
      "status": "active",
      "totalRuns": 42,
      "successfulRuns": 38,
      "failedRuns": 4,
      "errorRate": 0.095,
      "lastRunAt": "2026-03-15T10:30:00Z",
      "lastRunStatus": "success"
    }
  ],
  "issueVelocity": {
    "totalCreated": 87,
    "totalClosed": 64,
    "data": [
      {
        "date": "2026-02-13",
        "created": 3,
        "closed": 2
      }
    ]
  }
}
```

### Query Parameters

| Parameter | Values | Default | Description |
|-----------|--------|---------|-------------|
| `range` | `7d`, `30d`, `90d` | `30d` | Lookback window from now |
| `granularity` | `day`, `week`, `month` | `day` | Time bucket size for cost trends |

### Time Range Computation (Go)

```go
func computeTimeRange(rangeParam string) (start, end time.Time) {
    end = time.Now().UTC()
    switch rangeParam {
    case "7d":
        start = end.AddDate(0, 0, -7)
    case "30d":
        start = end.AddDate(0, 0, -30)
    case "90d":
        start = end.AddDate(0, 0, -90)
    default:
        start = end.AddDate(0, 0, -30) // fallback
    }
    return start, end
}
```

---

## 4. Service Layer

### MetricsService

```go
type MetricsService struct {
    queries *db.Queries
}

func NewMetricsService(q *db.Queries) *MetricsService {
    return &MetricsService{queries: q}
}

func (s *MetricsService) GetDashboardMetrics(
    ctx context.Context,
    squadID uuid.UUID,
    timeRange string,
    granularity string,
) (*DashboardMetricsResponse, error) {
    start, end := computeTimeRange(timeRange)

    // Run all three queries concurrently
    var (
        costTrends    []CostTrendBucket
        agentHealth   []AgentHealthRecord
        issueVelocity []IssueVelocityDay
        costErr, healthErr, velocityErr error
    )

    var wg sync.WaitGroup
    wg.Add(3)

    go func() {
        defer wg.Done()
        costTrends, costErr = s.getCostTrends(ctx, squadID, start, end, granularity)
    }()

    go func() {
        defer wg.Done()
        agentHealth, healthErr = s.getAgentHealth(ctx, squadID, start, end)
    }()

    go func() {
        defer wg.Done()
        issueVelocity, velocityErr = s.getIssueVelocity(ctx, squadID, start, end)
    }()

    wg.Wait()

    // Return first error encountered
    for _, err := range []error{costErr, healthErr, velocityErr} {
        if err != nil {
            return nil, err
        }
    }

    return &DashboardMetricsResponse{
        CostTrends:    assembleCostTrends(costTrends, granularity),
        AgentHealth:   agentHealth,
        IssueVelocity: assembleIssueVelocity(issueVelocity, start, end),
    }, nil
}
```

### Dense Date Fill (Issue Velocity)

The `assembleIssueVelocity` function generates a day-by-day array from `start` to `end`, merging the sparse created/closed query results into a dense representation:

```go
func assembleIssueVelocity(created, closed []DayCount, start, end time.Time) IssueVelocitySection {
    createdMap := toDateMap(created)
    closedMap := toDateMap(closed)

    var data []IssueVelocityDay
    var totalCreated, totalClosed int64

    for d := truncateDay(start); !d.After(truncateDay(end)); d = d.AddDate(0, 0, 1) {
        key := d.Format("2006-01-02")
        c := createdMap[key]
        cl := closedMap[key]
        totalCreated += c
        totalClosed += cl
        data = append(data, IssueVelocityDay{Date: key, Created: c, Closed: cl})
    }

    return IssueVelocitySection{
        TotalCreated: totalCreated,
        TotalClosed:  totalClosed,
        Data:         data,
    }
}
```

---

## 5. Handler Layer

### MetricsHandler

```go
type MetricsHandler struct {
    queries    *db.Queries
    metricsSvc *MetricsService
}

func NewMetricsHandler(q *db.Queries, metricsSvc *MetricsService) *MetricsHandler {
    return &MetricsHandler{queries: q, metricsSvc: metricsSvc}
}

func (h *MetricsHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/squads/{id}/metrics", h.GetMetrics)
}
```

Handler responsibilities:
1. Extract `squadId` from URL path
2. Validate `range` and `granularity` query params
3. Verify JWT auth and squad membership
4. Delegate to `MetricsService.GetDashboardMetrics()`
5. Serialize response as JSON

---

## 6. Inbox Badge Count (SSE Integration)

The inbox badge count uses the existing infrastructure:

1. **Initial load:** `GET /api/squads/{id}/inbox/count` returns `{ "count": N }`
2. **Real-time updates:** The client listens for `inbox.created` and `inbox.resolved` SSE events
3. **Client-side logic:** On `inbox.created`, increment count. On `inbox.resolved`, decrement count (floor at 0).

No backend changes needed — the inbox count endpoint and SSE events already exist from Feature 12.

### React Hook

```typescript
function useInboxBadge(squadId: string) {
    const { data: initial } = useQuery({
        queryKey: ["inbox-count", squadId],
        queryFn: () => api.get<{ count: number }>(`/squads/${squadId}/inbox/count`),
    });

    // Subscribe to SSE events for real-time updates
    // Increment on inbox.created, decrement on inbox.resolved
    // Use queryClient.setQueryData to update cache
}
```

---

## 7. React UI Components

### Component Tree

```
DashboardPage (enhanced)
├── TimeRangeSelector          ← 7d / 30d / 90d toggle
├── CostTrendChart             ← recharts AreaChart
├── IssueVelocityChart         ← recharts BarChart (dual series)
├── AgentHealthTable           ← table with status, runs, error rate, last run
├── InboxBadge (in sidebar)    ← real-time count via SSE
├── ActivityFeed (existing)    ← already present
└── Quick Actions (existing)   ← already present
```

### CostTrendChart

- **Library:** recharts `AreaChart`
- **X-axis:** Date buckets
- **Y-axis:** Cost in dollars (totalCents / 100, formatted)
- **Tooltip:** Date, cost, event count
- **Fill:** Gradient blue

### IssueVelocityChart

- **Library:** recharts `BarChart`
- **X-axis:** Date
- **Y-axis:** Count
- **Series:** Two bars per day — green for created, blue for closed
- **Summary:** Total created vs total closed shown above chart

### AgentHealthTable

- **Columns:** Agent Name, Status (badge), Total Runs, Error Rate (progress bar), Last Run (relative time)
- **Sort:** Default by error rate descending
- **Visual:** Error rate cell uses color coding — green (<10%), yellow (10-30%), red (>30%)

### TimeRangeSelector

- **Implementation:** Button group with three options (7d, 30d, 90d)
- **State:** Controls `range` param passed to useMetrics hook
- **Default:** 30d

### Skeleton Loading

Each widget has a corresponding skeleton component:
- `CostTrendChartSkeleton` — animated rectangle matching chart dimensions
- `IssueVelocityChartSkeleton` — animated bars
- `AgentHealthTableSkeleton` — animated table rows

---

## 8. React Data Fetching

### useMetrics Hook

```typescript
function useMetrics(squadId: string, range: string = "30d", granularity: string = "day") {
    return useQuery({
        queryKey: ["metrics", squadId, range, granularity],
        queryFn: () => api.get<DashboardMetrics>(
            `/squads/${squadId}/metrics?range=${range}&granularity=${granularity}`
        ),
        refetchInterval: 60_000, // Auto-refresh every 60 seconds
        staleTime: 30_000,       // Consider data stale after 30 seconds
    });
}
```

---

## 9. Server Wiring

Wire into server initialization:

```go
// In server setup
metricsSvc := handlers.NewMetricsService(queries)
metricsHandler := handlers.NewMetricsHandler(queries, metricsSvc)
metricsHandler.RegisterRoutes(mux)
```

No changes to existing handlers or services are required. The metrics feature is purely additive.

---

## 10. Design Decisions

| Decision | Rationale |
|----------|-----------|
| Single metrics endpoint | Avoids waterfall of 3+ API calls on dashboard load. One round-trip gets all data. |
| Concurrent Go queries | Three independent SQL queries run in parallel, capped by DB connection pool. Meets the 500ms SLA. |
| Sparse cost trends, dense velocity | Cost trends can have many empty buckets (sparse is smaller). Velocity needs every day for chart rendering (dense avoids gaps in bar chart). |
| No new DB tables | All metrics are computed from existing data. Adding materialized views or pre-aggregation tables is a future optimization if needed. |
| recharts library | Already available in the React ecosystem with Tailwind-friendly styling. Lightweight and composable. |
| 60s auto-refresh | Dashboard stays reasonably current without creating excessive server load. SSE handles inbox count in real-time. |
| Error rate sorting | Surfaces problematic agents at the top of the health table for quick triage. |
| date_trunc for granularity | PostgreSQL built-in function, well-indexed, handles timezone-aware truncation correctly. |
