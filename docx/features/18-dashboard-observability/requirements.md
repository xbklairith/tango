# Requirements: Dashboard Observability

**Created:** 2026-03-15
**Status:** Draft
**Feature:** 18-dashboard-observability
**Dependencies:** 09-activity-log, 10-cost-events-budget, 11-agent-runtime, 12-inbox-system

## Overview

Dashboard Observability provides a single-pane-of-glass metrics dashboard for squad operators. A single aggregated metrics endpoint returns cost trends, agent health, and issue velocity data in one call, avoiding waterfall requests. The React dashboard is enhanced with chart widgets (cost trends, issue velocity, agent health table) powered by recharts. Inbox badge counts are delivered in real-time via SSE.

## Scope

**In Scope:**
- Aggregated metrics endpoint returning all dashboard data in one call
- Cost trend aggregation by day, week, or month over configurable time ranges
- Agent health table: uptime, error rate, last run time, current status per agent
- Issue velocity widget: issues created vs issues closed per day over time
- Inbox pending count via existing endpoint, with SSE-driven real-time updates
- React UI: dashboard widgets with recharts-based line/bar charts
- Time range selector: last 7d, 30d, 90d (query parameter)

**Out of Scope (future):**
- Custom date range picker (arbitrary start/end dates)
- Per-agent drill-down pages with detailed metrics
- Exportable reports (CSV/PDF)
- Alerting rules based on metrics thresholds
- Cross-squad metrics aggregation
- Historical snapshots or metric retention policies

## Definitions

| Term | Definition |
|------|------------|
| Cost Trend | Aggregated cost_events.amount_cents grouped by time bucket (day/week/month) for a squad. |
| Agent Health | Derived metrics per agent: success rate, error rate, total runs, and last run timestamp computed from heartbeat_runs. |
| Issue Velocity | Daily count of issues created vs issues transitioned to `done` status within a squad over a time range. |
| Time Range | A lookback window from the current time: 7d, 30d, or 90d. |
| Metrics Endpoint | A single API endpoint that returns cost trends, agent health, and issue velocity in one response payload. |

## Requirements (EARS Format)

### Aggregated Metrics Endpoint

**REQ-OBS-001:** The system SHALL expose `GET /api/squads/{squadId}/metrics` to return aggregated dashboard metrics for a squad.

**REQ-OBS-002:** The `GET /api/squads/{squadId}/metrics` endpoint SHALL accept a `range` query parameter with values `7d`, `30d`, or `90d` (default `30d`).

**REQ-OBS-003:** The `GET /api/squads/{squadId}/metrics` endpoint SHALL accept a `granularity` query parameter with values `day`, `week`, or `month` (default `day`).

**REQ-OBS-004:** The `GET /api/squads/{squadId}/metrics` response SHALL include three top-level sections: `costTrends`, `agentHealth`, and `issueVelocity`.

**REQ-OBS-005:** The `GET /api/squads/{squadId}/metrics` endpoint SHALL require authentication (valid JWT).

**REQ-OBS-006:** The `GET /api/squads/{squadId}/metrics` endpoint SHALL enforce squad-scoped data isolation: only squad members can access metrics.

**REQ-OBS-007:** IF the `range` query parameter is not one of `7d`, `30d`, `90d`, THEN the system SHALL return HTTP 400 with code `VALIDATION_ERROR`.

**REQ-OBS-008:** IF the `granularity` query parameter is not one of `day`, `week`, `month`, THEN the system SHALL return HTTP 400 with code `VALIDATION_ERROR`.

### Cost Trends

**REQ-OBS-010:** The `costTrends` section SHALL contain an array of time-bucketed cost aggregations, each with `bucket` (ISO 8601 date string), `totalCents` (integer), and `eventCount` (integer).

**REQ-OBS-011:** WHEN `granularity=day`, the system SHALL aggregate cost_events by calendar day using `date_trunc('day', created_at)`.

**REQ-OBS-012:** WHEN `granularity=week`, the system SHALL aggregate cost_events by ISO week using `date_trunc('week', created_at)`.

**REQ-OBS-013:** WHEN `granularity=month`, the system SHALL aggregate cost_events by calendar month using `date_trunc('month', created_at)`.

**REQ-OBS-014:** The `costTrends` section SHALL include a `totalCents` summary field with the total cost across the entire time range.

**REQ-OBS-015:** The `costTrends` array SHALL be ordered by `bucket` ascending (oldest first).

**REQ-OBS-016:** WHEN there are no cost events in a time bucket, that bucket SHALL be omitted from the array (sparse representation).

### Agent Health

**REQ-OBS-020:** The `agentHealth` section SHALL contain an array of per-agent health records, one per agent in the squad.

**REQ-OBS-021:** Each agent health record SHALL include: `agentId` (UUID), `agentName` (string), `agentShortName` (string), `status` (current agent status), `totalRuns` (integer), `successfulRuns` (integer), `failedRuns` (integer), `errorRate` (float, 0.0-1.0), `lastRunAt` (ISO 8601 timestamp or null), `lastRunStatus` (string or null).

**REQ-OBS-022:** The `totalRuns`, `successfulRuns`, and `failedRuns` fields SHALL be computed from heartbeat_runs within the selected time range for each agent.

**REQ-OBS-023:** The `errorRate` field SHALL be computed as `failedRuns / totalRuns` (0.0 when totalRuns is 0).

**REQ-OBS-024:** The `lastRunAt` and `lastRunStatus` fields SHALL reflect the most recent heartbeat_run for that agent regardless of time range.

**REQ-OBS-025:** The `agentHealth` array SHALL be ordered by `errorRate` descending (most problematic agents first), with agents having zero runs sorted last.

### Issue Velocity

**REQ-OBS-030:** The `issueVelocity` section SHALL contain an array of daily data points, each with `date` (ISO 8601 date string), `created` (integer count of issues created that day), and `closed` (integer count of issues transitioned to `done` that day).

**REQ-OBS-031:** Issue creation date SHALL be derived from the `created_at` column of the `issues` table.

**REQ-OBS-032:** Issue closure date SHALL be derived from the `updated_at` column of issues with `status=done`, representing the day the issue was marked done.

**REQ-OBS-033:** The `issueVelocity` array SHALL cover every day in the time range, including days with zero created and zero closed (dense representation).

**REQ-OBS-034:** The `issueVelocity` section SHALL include summary fields: `totalCreated` (integer) and `totalClosed` (integer) across the entire time range.

**REQ-OBS-035:** The `issueVelocity` array SHALL be ordered by `date` ascending.

### Inbox Badge Count

**REQ-OBS-040:** The existing `GET /api/squads/{squadId}/inbox/count` endpoint SHALL continue to return the count of pending inbox items.

**REQ-OBS-041:** WHEN an `inbox.created` SSE event is received by the client, the UI SHALL increment the inbox badge count.

**REQ-OBS-042:** WHEN an `inbox.resolved` SSE event is received by the client, the UI SHALL decrement the inbox badge count.

**REQ-OBS-043:** The inbox badge count SHALL be displayed in the sidebar navigation next to the Inbox link.

### React UI Dashboard Widgets

**REQ-OBS-050:** The dashboard page SHALL display a cost trend chart widget using a line or area chart.

**REQ-OBS-051:** The dashboard page SHALL display an issue velocity chart widget using a bar chart with dual series (created vs closed).

**REQ-OBS-052:** The dashboard page SHALL display an agent health table widget showing each agent's name, status, run count, error rate, and last run time.

**REQ-OBS-053:** The dashboard page SHALL include a time range selector (7d / 30d / 90d) that controls the data shown in all widgets.

**REQ-OBS-054:** WHEN the time range is changed, the system SHALL re-fetch metrics from the API with the updated `range` parameter.

**REQ-OBS-055:** WHEN metrics are loading, the dashboard SHALL display skeleton placeholders for each widget.

**REQ-OBS-056:** WHEN the metrics endpoint returns an error, the dashboard SHALL display an error state with a retry button.

---

## Error Handling

| Scenario | HTTP Status | Error Code |
|----------|-------------|------------|
| Squad not found | 404 | `NOT_FOUND` |
| Invalid range parameter | 400 | `VALIDATION_ERROR` |
| Invalid granularity parameter | 400 | `VALIDATION_ERROR` |
| Unauthorized access | 401 | `UNAUTHORIZED` |
| Non-squad-member access | 403 | `FORBIDDEN` |

---

## Non-Functional Requirements

**REQ-OBS-NF-001:** The `GET /api/squads/{squadId}/metrics` endpoint SHALL respond within 500ms for squads with up to 10,000 cost events, 50 agents, and 5,000 issues.

**REQ-OBS-NF-002:** Cost trend and issue velocity SQL queries SHALL use indexed columns (`squad_id`, `created_at`) to avoid full table scans.

**REQ-OBS-NF-003:** The React dashboard SHALL render all chart widgets within 200ms after data is received (client-side rendering budget).

**REQ-OBS-NF-004:** The metrics endpoint SHALL execute all three data queries (cost, health, velocity) concurrently using goroutines, not sequentially.

---

## Acceptance Criteria

1. `GET /api/squads/{id}/metrics` returns cost trends, agent health, and issue velocity in a single response
2. Cost trends are correctly aggregated by day/week/month with accurate totals
3. Agent health shows correct run counts, error rates, and last run info computed from heartbeat_runs
4. Issue velocity shows daily created vs closed counts with dense date representation
5. Time range selector (7d/30d/90d) filters all three metric sections correctly
6. Dashboard displays cost trend chart, issue velocity chart, and agent health table
7. Inbox badge count updates in real-time via SSE events
8. All endpoints enforce JWT auth and squad-scoped isolation
9. Skeleton loading states and error states are handled in the UI
10. Metrics endpoint responds within 500ms under normal load

---

## References

- Activity Log: `docx/features/09-activity-log/`
- Cost Events & Budget: `docx/features/10-cost-events-budget/`
- Agent Runtime: `docx/features/11-agent-runtime/`
- Inbox System: `docx/features/12-inbox-system/`
- Cost queries: `internal/database/queries/cost_events.sql`
- Heartbeat run queries: `internal/database/queries/heartbeat_runs.sql`
- Issue queries: `internal/database/queries/issues.sql`
- Current dashboard: `web/src/features/dashboard/dashboard-page.tsx`
- Inbox count endpoint: `internal/server/handlers/inbox_handler.go`
