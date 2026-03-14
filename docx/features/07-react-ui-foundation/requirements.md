# Requirements: React UI Foundation

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 07-react-ui-foundation
**Dependencies:** 01-go-scaffold, 02-user-auth, 03-squad-management, 04-agent-management, 05-issue-tracking, 06-projects-goals

## Overview

This feature establishes the frontend foundation for Ari's React SPA. The UI is built with React 19 + Vite + Tailwind CSS + shadcn/ui and served as embedded static files from the Go binary. It provides the shell layout, routing, API client, auth integration, and all Phase 1 pages (dashboard, squads, agents, issues, projects, goals, login).

## EARS Requirements

### Toolchain and Build

**REQ-UI-001** [Ubiquitous]
The frontend SHALL be built with React 19, Vite, Tailwind CSS 4, and TypeScript.

**REQ-UI-002** [Ubiquitous]
The frontend SHALL use shadcn/ui as the component library foundation.

**REQ-UI-003** [Ubiquitous]
The Vite build output SHALL be embeddable into the Go binary via `go:embed` so that `ari run` serves the SPA without requiring a separate frontend process.

**REQ-UI-004** [Ubiquitous]
The `make ui-build` command SHALL produce a production-optimized build in the `web/dist/` directory.

**REQ-UI-005** [Ubiquitous]
The `make ui-dev` command SHALL start a Vite dev server with hot module replacement proxied to the Go API server at `http://localhost:3100`.

### SPA Routing

**REQ-UI-010** [Ubiquitous]
The application SHALL use React Router v7 for client-side routing.

**REQ-UI-011** [Ubiquitous]
The Go server SHALL serve the SPA's `index.html` for all non-API routes so that client-side routing works on page refresh and direct navigation.

**REQ-UI-012** [Ubiquitous]
The application SHALL define the following routes:

| Route | Page |
|-------|------|
| `/login` | Login page |
| `/` | Dashboard |
| `/squads` | Squad list |
| `/squads/:id` | Squad detail / settings |
| `/squads/:id/agents` | Agent list (filtered by squad) |
| `/agents/:id` | Agent detail |
| `/squads/:id/issues` | Issue list (filtered by squad) |
| `/issues/:id` | Issue detail |
| `/squads/:id/projects` | Project list (filtered by squad) |
| `/projects/:id` | Project detail |
| `/squads/:id/goals` | Goal list (filtered by squad) |
| `/goals/:id` | Goal detail |

**REQ-UI-013** [Ubiquitous]
Route transitions SHALL NOT trigger full-page reloads.

### API Client

**REQ-UI-020** [Ubiquitous]
The application SHALL provide a centralized API client module for all communication with the Go backend (`/api/*` endpoints).

**REQ-UI-021** [Ubiquitous]
The API client SHALL send the session cookie automatically with every request.

**REQ-UI-022** [Ubiquitous]
The API client SHALL deserialize error responses in the format `{"error": "message", "code": "CODE"}` and surface them to the calling code.

**REQ-UI-023** [Event-driven]
When the API client receives a `401 Unauthorized` response, the application SHALL redirect the user to the `/login` page and clear any cached auth state.

**REQ-UI-024** [Ubiquitous]
The API client SHALL set `Content-Type: application/json` on all requests with a body.

### Authentication Integration

**REQ-UI-030** [Ubiquitous]
The application SHALL provide a login page with email and password fields that authenticates against `POST /api/auth/login`.

**REQ-UI-031** [Ubiquitous]
Upon successful login, the application SHALL redirect the user to the dashboard (`/`).

**REQ-UI-032** [Ubiquitous]
The application SHALL provide an auth guard that redirects unauthenticated users to `/login` for all routes except `/login` itself.

**REQ-UI-033** [Ubiquitous]
The application SHALL maintain an auth context (React Context) that exposes the current user profile and squad memberships.

**REQ-UI-034** [Ubiquitous]
The application SHALL provide a logout action that calls `POST /api/auth/logout`, clears auth state, and redirects to `/login`.

**REQ-UI-035** [State-driven]
While the user's session is being validated on initial load, the application SHALL display a loading indicator instead of flashing the login page.

### Layout

**REQ-UI-040** [Ubiquitous]
The authenticated layout SHALL consist of a fixed sidebar navigation on the left and a scrollable main content area on the right.

**REQ-UI-041** [Ubiquitous]
The sidebar SHALL display navigation links for: Dashboard, Squads, Agents, Issues, Projects, and Goals.

**REQ-UI-042** [Ubiquitous]
The sidebar SHALL visually indicate the currently active navigation item.

**REQ-UI-043** [Ubiquitous]
The sidebar SHALL display the current user's display name and a logout button at the bottom.

**REQ-UI-044** [Ubiquitous]
The layout SHALL include a top header bar in the main content area showing the current page title and breadcrumb trail.

**REQ-UI-045** [State-driven]
When the viewport width is less than 1024px, the sidebar SHALL collapse into a hamburger menu overlay.

### Dashboard Page

**REQ-UI-050** [Ubiquitous]
The dashboard page SHALL display an overview of the user's primary squad.

**REQ-UI-051** [Ubiquitous]
The dashboard SHALL display the following metrics: active agent count, total agent count, issues by status (backlog, todo, in_progress, done, blocked, cancelled), and project count.

**REQ-UI-052** [Ubiquitous]
The dashboard SHALL display a list of recent activity (last 10 items from activity log or recent issues/comments).

**REQ-UI-053** [Ubiquitous]
The dashboard SHALL provide quick-action buttons for: create agent, create issue, and create project.

**REQ-UI-054** [State-driven]
When the user belongs to multiple squads, the application SHALL provide a squad selector (in the sidebar or header) to switch the active squad context. Switching squads SHALL be instant (client-side state change only, no server round-trip). The active squad SHALL be persisted in localStorage so it survives page refreshes and new sessions.

**REQ-UI-054a** [State-driven]
When the user logs in and has no previously active squad in localStorage, the application SHALL default to the first squad returned by `GET /api/squads`.

**REQ-UI-054b** [State-driven]
When the user has no squads (empty list from `GET /api/squads`), the application SHALL display a "Create your first squad" onboarding prompt instead of the dashboard.

**REQ-UI-054c** [State-driven]
When the user's active squad is deleted/archived or they are removed from it, the application SHALL automatically switch to the next available squad, or show the onboarding prompt if none remain.

**REQ-UI-054d** [Ubiquitous]
All squad-scoped navigation links (Agents, Issues, Projects, Goals) SHALL use the active squad's ID in their URL paths (e.g., `/squads/{activeSquadId}/agents`).

### Squad Pages

**REQ-UI-060** [Ubiquitous]
The squad list page SHALL display all squads the user is a member of, showing name, description, status, and agent count.

**REQ-UI-061** [Ubiquitous]
The squad list page SHALL provide a button to create a new squad.

**REQ-UI-062** [Ubiquitous]
The squad detail page SHALL display squad name, description, status, issue prefix, brand color, and budget settings.

**REQ-UI-063** [Ubiquitous]
The squad detail page SHALL allow editing of squad name, description, status, issue prefix, brand color, and budget via inline forms or a settings panel.

**REQ-UI-064** [Ubiquitous]
The squad detail page SHALL display a member list with roles (owner, admin, viewer).

### Agent Pages

**REQ-UI-070** [Ubiquitous]
The agent list page SHALL display all agents in the selected squad, showing name, role, title, status, and reports-to hierarchy.

**REQ-UI-071** [Ubiquitous]
Each agent in the list SHALL display a status badge using distinct colors for each status: active (green), idle (gray), running (blue), error (red), paused (yellow), terminated (dark gray), pending_approval (orange).

**REQ-UI-072** [Ubiquitous]
The agent list page SHALL provide a button to create a new agent.

**REQ-UI-073** [Ubiquitous]
The agent detail page SHALL display the agent's full profile: name, URL key, role, title, status, system prompt (truncated), model, reports-to agent, and creation date.

**REQ-UI-074** [Ubiquitous]
The agent detail page SHALL display the agent's hierarchy showing its direct reports (agents that report to it) and its own reporting chain.

**REQ-UI-075** [Ubiquitous]
The agent detail page SHALL allow editing of agent name, role, title, status, system prompt, model, and reports-to fields.

**REQ-UI-076** [Ubiquitous]
The agent list page SHALL support a tree view mode that renders the full squad hierarchy (captain at root, leads as branches, members as leaves).

### Issue Pages

**REQ-UI-080** [Ubiquitous]
The issue list page SHALL display all issues in the selected squad, showing identifier (e.g., "ARI-42"), title, status, priority, assignee, and creation date.

**REQ-UI-081** [Ubiquitous]
The issue list page SHALL support filtering by status, assignee, priority, and project.

**REQ-UI-082** [Ubiquitous]
The issue list page SHALL provide a button to create a new issue.

**REQ-UI-083** [Ubiquitous]
The issue detail page SHALL display the full issue: identifier, title, description (rendered Markdown), status, priority, assignee, reporter, project, parent issue, labels, and timestamps.

**REQ-UI-084** [Ubiquitous]
The issue detail page SHALL display a comments thread in chronological order, showing author (user or agent), content, and timestamp.

**REQ-UI-085** [Ubiquitous]
The issue detail page SHALL provide a comment input form for adding new comments.

**REQ-UI-086** [Ubiquitous]
The issue detail page SHALL allow editing of title, description, status, priority, assignee, and project via inline editing or an edit form.

**REQ-UI-087** [Ubiquitous]
Issue status transitions SHALL be represented as a selectable dropdown or button group reflecting the valid state machine transitions.

### Project Pages

**REQ-UI-090** [Ubiquitous]
The project list page SHALL display all projects in the selected squad, showing name, description, status, and linked issue count.

**REQ-UI-091** [Ubiquitous]
The project list page SHALL provide a button to create a new project.

**REQ-UI-092** [Ubiquitous]
The project detail page SHALL display project name, description, status, and a list of issues linked to the project.

**REQ-UI-093** [Ubiquitous]
The project detail page SHALL allow editing of project name, description, and status.

### Goal Pages

**REQ-UI-100** [Ubiquitous]
The goal list page SHALL display all goals in the selected squad, showing title, description, status, target date, and linked project count.

**REQ-UI-101** [Ubiquitous]
The goal list page SHALL provide a button to create a new goal.

**REQ-UI-102** [Ubiquitous]
The goal detail page SHALL display goal title, description, status, target date, and a list of linked projects.

**REQ-UI-103** [Ubiquitous]
The goal detail page SHALL allow editing of goal title, description, status, and target date.

### Component Library

**REQ-UI-110** [Ubiquitous]
The application SHALL use shadcn/ui components configured with the project's Tailwind theme.

**REQ-UI-111** [Ubiquitous]
The following shadcn/ui components SHALL be installed and available: Button, Input, Label, Card, Table, Dialog, DropdownMenu, Select, Badge, Tabs, Toast, Skeleton, Avatar, Separator, Sheet, Command, Popover, and Form.

**REQ-UI-112** [Ubiquitous]
The application SHALL define a consistent color theme with CSS variables for light mode, following the shadcn/ui theming conventions.

### Responsive Design

**REQ-UI-120** [Ubiquitous]
The application SHALL be designed desktop-first with a minimum supported viewport width of 1024px.

**REQ-UI-121** [Ubiquitous]
The application SHALL remain usable on tablet viewports (768px-1023px) with the sidebar collapsed into an overlay menu.

**REQ-UI-122** [Ubiquitous]
Tables with many columns SHALL provide horizontal scrolling on narrow viewports rather than breaking layout.

### Error Handling

**REQ-UI-130** [Ubiquitous]
The application SHALL display toast notifications for transient success and error messages (e.g., "Squad created", "Failed to save").

**REQ-UI-131** [Ubiquitous]
The application SHALL implement React error boundaries that catch rendering errors and display a fallback UI with a "Retry" option instead of a blank screen.

**REQ-UI-132** [Event-driven]
When an API request fails with a network error, the application SHALL display a toast notification indicating the connection issue.

**REQ-UI-133** [Event-driven]
When an API request returns a 4xx or 5xx error, the application SHALL display the error message from the response body in a toast notification.

**REQ-UI-134** [Ubiquitous]
The application SHALL display a 404 page for unrecognized routes.

### Loading States

**REQ-UI-140** [State-driven]
While data is being fetched for a page, the application SHALL display skeleton placeholders that approximate the shape of the expected content.

**REQ-UI-141** [State-driven]
While a form submission is in progress, the submit button SHALL display a spinner and be disabled to prevent duplicate submissions.

**REQ-UI-142** [State-driven]
While navigating between pages, the application SHALL display a loading indicator if the target page's data is not yet available.

### Data Fetching

**REQ-UI-150** [Ubiquitous]
The application SHALL use React Query (TanStack Query) for server state management, caching, and background refetching.

**REQ-UI-151** [Ubiquitous]
List pages SHALL implement pagination using the API's offset/limit parameters.

**REQ-UI-152** [Ubiquitous]
Mutations (create, update, delete) SHALL invalidate the relevant query cache entries to ensure the UI reflects the latest state.

### Accessibility

**REQ-UI-160** [Ubiquitous]
All interactive elements SHALL be keyboard-navigable using Tab, Enter, and Escape keys.

**REQ-UI-161** [Ubiquitous]
All form inputs SHALL have associated `<label>` elements or `aria-label` attributes.

**REQ-UI-162** [Ubiquitous]
Color alone SHALL NOT be the only means of conveying status information; text labels or icons SHALL accompany color-coded badges.

### Performance

**REQ-UI-170** [Ubiquitous]
The initial page load (first contentful paint) SHALL complete in under 2 seconds on a broadband connection.

**REQ-UI-171** [Ubiquitous]
The production build SHALL use code splitting so that page-specific code is loaded on demand via lazy imports.

**REQ-UI-172** [Ubiquitous]
Static assets (JS, CSS, images) SHALL include content hashes in filenames for cache busting.

## Non-Functional Requirements

**REQ-UI-NF-001** [Ubiquitous]
All TypeScript code SHALL pass strict type checking with no `any` types except where explicitly justified and documented.

**REQ-UI-NF-002** [Ubiquitous]
The frontend codebase SHALL pass ESLint checks with zero errors.

**REQ-UI-NF-003** [Ubiquitous]
The frontend codebase SHALL be tested with Vitest, targeting at least 60% code coverage for utility functions and hooks.

**REQ-UI-NF-004** [Ubiquitous]
Component file organization SHALL follow a feature-based structure (e.g., `web/src/features/squads/`, `web/src/features/agents/`).

## Glossary

| Term | Definition |
|------|-----------|
| SPA | Single Page Application |
| SSE | Server-Sent Events |
| EARS | Easy Approach to Requirements Syntax |
| shadcn/ui | A collection of reusable UI components built with Radix UI and Tailwind CSS |
| TanStack Query | Data fetching and caching library for React (formerly React Query) |
| Squad | Top-level organizational unit in Ari; all entities are squad-scoped |
| Issue Identifier | Human-readable issue ID in format `{prefix}-{counter}` (e.g., ARI-42) |
