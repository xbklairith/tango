# Tasks: Dashboard Observability

**Feature:** 18-dashboard-observability
**Created:** 2026-03-15
**Status:** Pending

## Requirement Traceability

- Source requirements: [requirements.md](./requirements.md)
- Design: [design.md](./design.md)
- Requirement coverage: REQ-OBS-001 through REQ-OBS-056, REQ-OBS-NF-001 through REQ-OBS-NF-004

## Implementation Approach

Work bottom-up: SQL queries first, then service layer with concurrent execution, then HTTP handler, then React UI widgets. No new database migration is needed — all metrics are computed from existing tables (cost_events, heartbeat_runs, issues, agents). Each task follows the Red-Green-Refactor TDD cycle.

## Progress Summary

- Total Tasks: 6
- Completed: 0/6
- In Progress: None

---

## Tasks (TDD: Red-Green-Refactor)

---

### [ ] Task 01 — SQL Queries: Cost Trends, Agent Health, Issue Velocity

**Requirements:** REQ-OBS-010 through REQ-OBS-016, REQ-OBS-020 through REQ-OBS-025, REQ-OBS-030 through REQ-OBS-035, REQ-OBS-NF-002
**Estimated time:** 45 min

#### Context

Write sqlc query definitions for the three metric categories. These queries read from existing tables (cost_events, heartbeat_runs, agents, issues) and produce aggregated results. No new migration is needed. Run `make sqlc` to generate Go code.

#### RED — Write Failing Tests

Write `internal/database/db/metrics_test.go`:

1. `TestGetCostTrends_Daily` — insert cost events across 3 days, verify daily bucketing returns correct totals and event counts per day.
2. `TestGetCostTrends_Weekly` — insert cost events across 3 weeks, verify weekly bucketing aggregates correctly.
3. `TestGetCostTrends_Monthly` — insert cost events across 2 months, verify monthly bucketing.
4. `TestGetCostTrends_Empty` — no cost events in range, verify empty result set.
5. `TestGetCostTrends_SquadIsolation` — insert events for two squads, verify only target squad's events returned.
6. `TestGetAgentRunStatsBySquad` — insert heartbeat_runs with mixed statuses (success, failed, cancelled) for 3 agents, verify correct total/success/failed counts per agent.
7. `TestGetAgentRunStatsBySquad_OrderByErrorRate` — verify agents with highest error rate appear first, agents with zero runs appear last.
8. `TestGetAgentLastRun` — insert multiple runs per agent, verify only the most recent run per agent is returned.
9. `TestGetIssuesCreatedPerDay` — insert issues across 5 days, verify daily creation counts.
10. `TestGetIssuesClosedPerDay` — insert issues with status=done and varying updated_at dates, verify daily closure counts.
11. `TestGetIssuesClosedPerDay_OnlyDone` — insert issues with various statuses, verify only `done` issues are counted.

#### GREEN — Implement

Create `internal/database/queries/metrics.sql` with these queries from design.md section 2:

- `GetCostTrends` — aggregate cost_events by date_trunc granularity
- `GetAgentRunStatsBySquad` — aggregate heartbeat_runs per agent with success/failed counts
- `GetAgentLastRun` — DISTINCT ON to get most recent run per agent
- `GetIssuesCreatedPerDay` — count issues by created_at day
- `GetIssuesClosedPerDay` — count done issues by updated_at day

Run `make sqlc`.

#### REFACTOR

Verify query plans use indexes on `squad_id` and `created_at` columns. Add index hints in comments if needed.

#### Files

- Create: `internal/database/queries/metrics.sql`
- Regenerate: `internal/database/db/` (via `make sqlc`)
- Create: `internal/database/db/metrics_test.go`

---

### [ ] Task 02 — MetricsService: Concurrent Query Orchestration

**Requirements:** REQ-OBS-001, REQ-OBS-002, REQ-OBS-003, REQ-OBS-004, REQ-OBS-014, REQ-OBS-015, REQ-OBS-016, REQ-OBS-023, REQ-OBS-024, REQ-OBS-025, REQ-OBS-033, REQ-OBS-034, REQ-OBS-035, REQ-OBS-NF-001, REQ-OBS-NF-004
**Estimated time:** 60 min

#### Context

The `MetricsService` orchestrates the three metric queries concurrently using goroutines and `sync.WaitGroup`. It assembles the results into a single `DashboardMetricsResponse` struct. The service handles time range computation, dense date filling for issue velocity, error rate calculation, and merging agent run stats with last-run data.

#### RED — Write Failing Tests

Write `internal/server/handlers/metrics_service_test.go`:

1. `TestMetricsService_GetDashboardMetrics` — with seeded data, verify response contains all three sections (costTrends, agentHealth, issueVelocity) with correct values.
2. `TestMetricsService_CostTrends_Aggregation` — verify cost cents and event counts are summed correctly per bucket.
3. `TestMetricsService_CostTrends_TotalCents` — verify the totalCents summary field sums all buckets.
4. `TestMetricsService_AgentHealth_ErrorRate` — insert runs with known success/failure ratios, verify errorRate calculation (failedRuns / totalRuns).
5. `TestMetricsService_AgentHealth_ZeroRuns` — agent with no runs in range has totalRuns=0, errorRate=0.0, but still appears with agent info.
6. `TestMetricsService_AgentHealth_LastRun` — verify lastRunAt and lastRunStatus reflect most recent run regardless of time range.
7. `TestMetricsService_AgentHealth_Ordering` — verify agents sorted by errorRate descending, zero-run agents last.
8. `TestMetricsService_IssueVelocity_DenseFill` — verify every day in range has an entry, including days with zero created/closed.
9. `TestMetricsService_IssueVelocity_Totals` — verify totalCreated and totalClosed summary fields.
10. `TestMetricsService_TimeRange_7d` — verify 7-day range computes correct start/end dates.
11. `TestMetricsService_TimeRange_90d` — verify 90-day range computes correct start/end dates.
12. `TestMetricsService_ConcurrentExecution` — verify all three queries execute (no panics, no data races). Run with `-race` flag.

#### GREEN — Implement

Create `internal/server/handlers/metrics_service.go`:

- `DashboardMetricsResponse` struct with `CostTrends`, `AgentHealth`, `IssueVelocity` sections
- `CostTrendSection` with `TotalCents`, `Granularity`, `Data []CostTrendBucket`
- `CostTrendBucket` with `Bucket`, `TotalCents`, `EventCount`
- `AgentHealthRecord` with all fields from REQ-OBS-021
- `IssueVelocitySection` with `TotalCreated`, `TotalClosed`, `Data []IssueVelocityDay`
- `IssueVelocityDay` with `Date`, `Created`, `Closed`
- `MetricsService` struct with `queries *db.Queries`
- `NewMetricsService(q)` constructor
- `GetDashboardMetrics(ctx, squadID, timeRange, granularity)` — concurrent execution with WaitGroup
- `computeTimeRange(rangeParam)` — returns start, end times
- `getCostTrends(ctx, squadID, start, end, granularity)` — query + assemble
- `getAgentHealth(ctx, squadID, start, end)` — run stats query + last run query + merge
- `getIssueVelocity(ctx, squadID, start, end)` — created + closed queries + dense fill
- `assembleIssueVelocity(created, closed, start, end)` — dense date array generation

#### REFACTOR

Ensure JSON tags use camelCase. Verify that context cancellation propagates to all goroutines.

#### Files

- Create: `internal/server/handlers/metrics_service.go`
- Create: `internal/server/handlers/metrics_service_test.go`

---

### [ ] Task 03 — MetricsHandler: HTTP Endpoint and Validation

**Requirements:** REQ-OBS-001, REQ-OBS-002, REQ-OBS-003, REQ-OBS-005, REQ-OBS-006, REQ-OBS-007, REQ-OBS-008
**Estimated time:** 30 min

#### Context

The HTTP handler exposes `GET /api/squads/{id}/metrics` with query parameter validation, JWT auth, squad membership check, and delegation to `MetricsService`. Follow the pattern from existing handlers like `inbox_handler.go`.

#### RED — Write Failing Tests

Write `internal/server/handlers/metrics_handler_test.go`:

1. `TestGetMetrics_Success` — GET with valid params, verify 200 and response shape with all three sections.
2. `TestGetMetrics_DefaultParams` — GET without range/granularity params, verify defaults to range=30d, granularity=day.
3. `TestGetMetrics_InvalidRange` — GET with range=60d, verify 400 with `VALIDATION_ERROR`.
4. `TestGetMetrics_InvalidGranularity` — GET with granularity=hour, verify 400 with `VALIDATION_ERROR`.
5. `TestGetMetrics_SquadNotFound` — GET with non-existent squad ID, verify 404.
6. `TestGetMetrics_SquadIsolation` — GET by non-member user, verify 403.
7. `TestGetMetrics_RequireAuth` — GET without JWT, verify 401.
8. `TestGetMetrics_AllRanges` — GET with range=7d, verify correct time window. Repeat for 30d, 90d.
9. `TestGetMetrics_AllGranularities` — GET with granularity=week, verify cost trends use weekly bucketing. Repeat for month.

#### GREEN — Implement

Create `internal/server/handlers/metrics_handler.go`:

- `MetricsHandler` struct with `queries`, `metricsSvc`
- `NewMetricsHandler(q, metricsSvc)` constructor
- `RegisterRoutes(mux)` — register `GET /api/squads/{id}/metrics`
- `GetMetrics(w, r)` — parse squad ID from URL, parse and validate query params, verify auth and squad membership, delegate to service, serialize JSON response

#### Files

- Create: `internal/server/handlers/metrics_handler.go`
- Create: `internal/server/handlers/metrics_handler_test.go`

---

### [ ] Task 04 — Server Wiring: Register Metrics Routes

**Requirements:** All (integration)
**Estimated time:** 15 min

#### Context

Wire `MetricsService` and `MetricsHandler` into server initialization. This is a small task — just constructor calls and route registration. Follow the pattern from how `PipelineHandler` was wired in Feature 14 Task 10.

#### RED — Write Failing Tests

Write an integration test that:

1. Starts the full server with embedded DB.
2. Creates a squad, agents, issues, cost events, and heartbeat runs.
3. `GET /api/squads/{id}/metrics` — verify 200 and response contains all three metric sections with correct data.
4. `GET /api/squads/{id}/metrics?range=7d` — verify time range filter works end-to-end.

#### GREEN — Implement

Modify server initialization (likely `cmd/ari/run.go` or `internal/server/server.go`):

- Create `MetricsService`: `NewMetricsService(queries)`
- Create `MetricsHandler`: `NewMetricsHandler(queries, metricsSvc)`
- Call `metricsHandler.RegisterRoutes(mux)`

#### Files

- Modify: `cmd/ari/run.go` or `internal/server/server.go` (server initialization)
- Create: `internal/server/handlers/metrics_integration_test.go`

---

### [ ] Task 05 — React UI: Dashboard Widgets with Charts

**Requirements:** REQ-OBS-050, REQ-OBS-051, REQ-OBS-052, REQ-OBS-053, REQ-OBS-054, REQ-OBS-055, REQ-OBS-056, REQ-OBS-NF-003
**Estimated time:** 90 min

#### Context

Enhance the existing dashboard page with three new chart/table widgets and a time range selector. Install recharts as a dependency. Create reusable chart components that consume the metrics API response. Add skeleton loading states and error handling.

#### RED — Write Failing Tests

(Frontend testing — verify component rendering and data binding)

1. `DashboardPage` renders time range selector with 7d/30d/90d buttons, 30d selected by default.
2. `CostTrendChart` renders an area chart with correct number of data points.
3. `IssueVelocityChart` renders a bar chart with created and closed series.
4. `AgentHealthTable` renders a row per agent with name, status badge, run count, error rate, last run.
5. `DashboardPage` shows skeleton state when metrics are loading.
6. `DashboardPage` shows error state with retry button when metrics fail.
7. Changing time range triggers re-fetch of metrics data.

#### GREEN — Implement

Install recharts:
```bash
cd web && npm install recharts
```

Create React components:

- `web/src/features/dashboard/time-range-selector.tsx` — button group (7d / 30d / 90d)
- `web/src/features/dashboard/cost-trend-chart.tsx` — recharts AreaChart with gradient fill, tooltip, formatted Y-axis (dollars)
- `web/src/features/dashboard/issue-velocity-chart.tsx` — recharts BarChart with dual series (created=green, closed=blue), tooltip, legend
- `web/src/features/dashboard/agent-health-table.tsx` — table with status badge, run counts, error rate bar (color-coded), relative last-run time
- `web/src/features/dashboard/metrics-skeleton.tsx` — skeleton loading placeholders for all three widgets
- `web/src/hooks/use-metrics.ts` — `useMetrics(squadId, range, granularity)` hook with react-query, 60s auto-refresh, 30s stale time
- `web/src/types/metrics.ts` — TypeScript types for DashboardMetrics response

Modify existing:

- `web/src/features/dashboard/dashboard-page.tsx` — integrate TimeRangeSelector, CostTrendChart, IssueVelocityChart, AgentHealthTable below existing stats cards. Add loading/error states.

#### REFACTOR

Ensure charts are responsive (use recharts ResponsiveContainer). Verify color palette is consistent with existing Tailwind theme.

#### Files

- Create: `web/src/features/dashboard/time-range-selector.tsx`
- Create: `web/src/features/dashboard/cost-trend-chart.tsx`
- Create: `web/src/features/dashboard/issue-velocity-chart.tsx`
- Create: `web/src/features/dashboard/agent-health-table.tsx`
- Create: `web/src/features/dashboard/metrics-skeleton.tsx`
- Create: `web/src/hooks/use-metrics.ts`
- Create: `web/src/types/metrics.ts`
- Modify: `web/src/features/dashboard/dashboard-page.tsx`

---

### [ ] Task 06 — React UI: Inbox Badge Count via SSE

**Requirements:** REQ-OBS-040, REQ-OBS-041, REQ-OBS-042, REQ-OBS-043
**Estimated time:** 30 min

#### Context

Add a real-time inbox badge count to the sidebar navigation. The badge fetches the initial count from the existing `/inbox/count` endpoint and then updates in real-time by listening to `inbox.created` and `inbox.resolved` SSE events. No backend changes needed.

#### RED — Write Failing Tests

(Frontend testing — verify component rendering and SSE integration)

1. `InboxBadge` renders initial count from API.
2. `InboxBadge` increments count when `inbox.created` SSE event is received.
3. `InboxBadge` decrements count when `inbox.resolved` SSE event is received.
4. `InboxBadge` does not go below zero on decrement.
5. `InboxBadge` is displayed next to the Inbox link in the sidebar.

#### GREEN — Implement

Create React components:

- `web/src/features/inbox/inbox-badge.tsx` — badge component that displays pending count
- `web/src/hooks/use-inbox-badge.ts` — hook that fetches initial count and subscribes to SSE events for real-time updates. Uses `queryClient.setQueryData` to update the cached count.

Modify existing:

- Sidebar/navigation component — add `<InboxBadge>` next to the Inbox navigation link

#### Files

- Create: `web/src/features/inbox/inbox-badge.tsx`
- Create: `web/src/hooks/use-inbox-badge.ts`
- Modify: sidebar/navigation component (add inbox badge)

---

## Requirement Coverage Matrix

| Requirement | Task(s) |
|-------------|---------|
| REQ-OBS-001 | Task 02, Task 03 |
| REQ-OBS-002 | Task 02, Task 03 |
| REQ-OBS-003 | Task 02, Task 03 |
| REQ-OBS-004 | Task 02, Task 03 |
| REQ-OBS-005 | Task 03 |
| REQ-OBS-006 | Task 03 |
| REQ-OBS-007 | Task 03 |
| REQ-OBS-008 | Task 03 |
| REQ-OBS-010 | Task 01, Task 02 |
| REQ-OBS-011 | Task 01 |
| REQ-OBS-012 | Task 01 |
| REQ-OBS-013 | Task 01 |
| REQ-OBS-014 | Task 02 |
| REQ-OBS-015 | Task 01, Task 02 |
| REQ-OBS-016 | Task 02 |
| REQ-OBS-020 | Task 01, Task 02 |
| REQ-OBS-021 | Task 01, Task 02 |
| REQ-OBS-022 | Task 01 |
| REQ-OBS-023 | Task 02 |
| REQ-OBS-024 | Task 01, Task 02 |
| REQ-OBS-025 | Task 01, Task 02 |
| REQ-OBS-030 | Task 01, Task 02 |
| REQ-OBS-031 | Task 01 |
| REQ-OBS-032 | Task 01 |
| REQ-OBS-033 | Task 02 |
| REQ-OBS-034 | Task 02 |
| REQ-OBS-035 | Task 01, Task 02 |
| REQ-OBS-040 | Task 06 |
| REQ-OBS-041 | Task 06 |
| REQ-OBS-042 | Task 06 |
| REQ-OBS-043 | Task 06 |
| REQ-OBS-050 | Task 05 |
| REQ-OBS-051 | Task 05 |
| REQ-OBS-052 | Task 05 |
| REQ-OBS-053 | Task 05 |
| REQ-OBS-054 | Task 05 |
| REQ-OBS-055 | Task 05 |
| REQ-OBS-056 | Task 05 |
| REQ-OBS-NF-001 | Task 02, Task 04 |
| REQ-OBS-NF-002 | Task 01 |
| REQ-OBS-NF-003 | Task 05 |
| REQ-OBS-NF-004 | Task 02 |
