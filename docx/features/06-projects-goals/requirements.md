# Requirements: Projects & Goals

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 06-projects-goals
**Dependencies:** 01-go-scaffold, 02-user-auth, 03-squad-management

## Overview

Projects and Goals provide the strategic and organizational layer for work management in Ari. Projects group related issues under a shared scope, while Goals define strategic objectives that issues and projects align to. Goals support a self-referential hierarchy (sub-goals via parentId). Both entities are strictly squad-scoped for data isolation.

This feature is part of Phase 1 (v0.1) per the PRD.

## Definitions

- **Project**: A grouping of related issues under a shared scope within a squad.
- **Goal**: A strategic objective that issues and projects align to. Goals can form a hierarchy via self-referential parentId.
- **Squad-scoped**: All projects and goals belong to exactly one squad; cross-squad access is forbidden.
- **Status lifecycle**: The progression of an entity through its allowed statuses (active, completed, archived).

## Requirements (EARS Format)

### Project Entity

**REQ-PG-001**: The system shall store projects with the following attributes: id (UUID, primary key), squadId (FK to squad), name (string, required), description (text, optional), status (enum: active, completed, archived), createdAt (timestamp), and updatedAt (timestamp).

**REQ-PG-002**: When a project is created, the system shall set the default status to "active".

**REQ-PG-003**: The system shall enforce that every project belongs to exactly one squad (squadId is required and immutable after creation).

**REQ-PG-004**: The system shall enforce that project name is non-empty and does not exceed 255 characters.

**REQ-PG-005**: Where the project name already exists within the same squad, the system shall reject the creation with a 409 Conflict error.

### Goal Entity

**REQ-PG-010**: The system shall store goals with the following attributes: id (UUID, primary key), squadId (FK to squad), parentId (FK to goal, nullable for top-level goals), title (string, required), description (text, optional), status (enum: active, completed, archived), createdAt (timestamp), and updatedAt (timestamp).

**REQ-PG-011**: When a goal is created, the system shall set the default status to "active".

**REQ-PG-012**: The system shall enforce that every goal belongs to exactly one squad (squadId is required and immutable after creation).

**REQ-PG-013**: The system shall enforce that goal title is non-empty and does not exceed 255 characters.

**REQ-PG-014**: Where a parentId is provided, the system shall validate that the parent goal exists and belongs to the same squad.

**REQ-PG-015**: Where a parentId would create a circular reference (goal directly or indirectly referencing itself), the system shall reject the operation with a 422 Unprocessable Entity error.

**REQ-PG-016**: The system shall support a maximum goal nesting depth of 5 levels.

### Project API

**REQ-PG-020**: The system shall expose `GET /api/squads/:id/projects` to list all projects for a squad, returning an array of project objects.

**REQ-PG-021**: The system shall expose `POST /api/squads/:id/projects` to create a new project within a squad, accepting name (required), description (optional), and status (optional, defaults to "active").

**REQ-PG-022**: The system shall expose `PATCH /api/projects/:id` to update an existing project, accepting name (optional), description (optional), and status (optional).

**REQ-PG-023**: When the user is not authenticated, the system shall reject project API requests with 401 Unauthorized.

**REQ-PG-024**: When the user does not have membership in the target squad, the system shall reject project API requests with 403 Forbidden.

**REQ-PG-025**: When the referenced squad does not exist, the system shall return 404 Not Found.

**REQ-PG-026**: When the referenced project does not exist, the system shall return 404 Not Found.

**REQ-PG-027**: The system shall return project list results ordered by createdAt descending (newest first).

### Goal API

**REQ-PG-030**: The system shall expose `GET /api/squads/:id/goals` to list all goals for a squad, returning an array of goal objects.

**REQ-PG-031**: The system shall expose `POST /api/squads/:id/goals` to create a new goal within a squad, accepting title (required), description (optional), parentId (optional), and status (optional, defaults to "active").

**REQ-PG-032**: The system shall expose `PATCH /api/goals/:id` to update an existing goal, accepting title (optional), description (optional), parentId (optional), and status (optional).

**REQ-PG-033**: When the user is not authenticated, the system shall reject goal API requests with 401 Unauthorized.

**REQ-PG-034**: When the user does not have membership in the target squad, the system shall reject goal API requests with 403 Forbidden.

**REQ-PG-035**: When the referenced squad does not exist, the system shall return 404 Not Found.

**REQ-PG-036**: When the referenced goal does not exist, the system shall return 404 Not Found.

**REQ-PG-037**: The system shall return goal list results ordered by createdAt descending (newest first).

**REQ-PG-038**: The system shall support filtering goals by parentId query parameter (including `null` for top-level goals only).

### Issue Linkage

**REQ-PG-040**: The system shall allow issues to reference a project via an optional projectId foreign key.

**REQ-PG-041**: The system shall allow issues to reference a goal via an optional goalId foreign key.

**REQ-PG-042**: Where a projectId is provided on an issue, the system shall validate that the project exists and belongs to the same squad as the issue.

**REQ-PG-043**: Where a goalId is provided on an issue, the system shall validate that the goal exists and belongs to the same squad as the issue.

**REQ-PG-044**: When a project or goal is referenced by one or more issues, the system shall still allow status changes on the project or goal (no cascade blocking).

### Status Transitions

**REQ-PG-050**: The system shall enforce the following valid status transitions for projects: active -> completed, active -> archived, completed -> active, completed -> archived, archived -> active.

**REQ-PG-051**: The system shall enforce the following valid status transitions for goals: active -> completed, active -> archived, completed -> active, completed -> archived, archived -> active.

**REQ-PG-052**: When an invalid status transition is attempted, the system shall reject the request with a 422 Unprocessable Entity error and include the current status and attempted status in the error message.

### Squad-Scoped Data Isolation

**REQ-PG-060**: The system shall ensure that project API endpoints only return projects belonging to the specified squad.

**REQ-PG-061**: The system shall ensure that goal API endpoints only return goals belonging to the specified squad.

**REQ-PG-062**: The system shall prevent updating a project that belongs to a different squad than the authenticated user has access to.

**REQ-PG-063**: The system shall prevent updating a goal that belongs to a different squad than the authenticated user has access to.

### Cost Attribution (Forward-Looking)

**REQ-PG-070**: The system shall ensure that the project id and goal id fields are available for cost event attribution (projectId, goalId on CostEvent entity), enabling cost tracking per project and per goal in Phase 2.

## Non-Functional Requirements

**REQ-PG-NF-001**: The system shall respond to project and goal list endpoints within 200ms for squads with up to 1,000 projects or goals.

**REQ-PG-NF-002**: The system shall use database indexes on squadId for both projects and goals tables to ensure performant lookups.

**REQ-PG-NF-003**: The system shall use database indexes on parentId for the goals table to support efficient hierarchy queries.

## Out of Scope

- Project deletion (soft-delete via archived status is sufficient for Phase 1)
- Goal deletion (soft-delete via archived status is sufficient for Phase 1)
- ProjectWorkspaces (repo/directory configs) -- deferred to a later feature
- Goal progress tracking / percentage completion
- Bulk operations on projects or goals
- Project or goal assignment to agents
- Cross-squad project or goal references
