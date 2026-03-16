# Onboarding Wizard — Requirements

## Overview

Replace the current 2-step Create Squad dialog with a guided multi-step onboarding wizard for first-time squad creation. The wizard walks users through creating a squad, configuring the captain agent (including adapter and runtime settings), assigning a bootstrap task, and launching.

**Problem:** Today, the captain is auto-created with hardcoded `claude_local` adapter and zero configuration. Users must navigate to the agent detail page afterward to configure working directory, model, and other adapter settings — a disconnected experience.

**Solution:** A full-screen, step-by-step wizard that treats captain creation as a deliberate, user-driven action with adapter configuration built in.

## Research Summary

- Current Ari flow: Dashboard empty state → Create Squad dialog (2 steps: squad info + captain name) → captain auto-created with `claude_local`, no config
- Adapter config fields currently live on the Agent Detail page only (adapterType text input + adapterConfig JSON editor)
- The backend already supports `adapter_type` and `adapter_config` on agent creation (JSONB), but the squad creation endpoint hardcodes `claude_local` with empty config
- Adapter registry auto-detects available adapters at startup (claude_local, process)
- Claude adapter config supports: working directory, model, timeout, env vars, skip permissions, max budget

## Functional Requirements

### Ubiquitous

- **REQ-001:** The system SHALL present the onboarding wizard as a full-screen experience (not a modal dialog)
- **REQ-002:** The system SHALL show step progress indicators (e.g., "Step 2 of 4") throughout the wizard
- **REQ-003:** The system SHALL allow the user to navigate back to previous steps without losing entered data
- **REQ-004:** The system SHALL allow the user to close/dismiss the wizard at any point

### Event-Driven

- **REQ-005:** WHEN the user has no squads and lands on the dashboard THEN the system SHALL display a "Get Started" CTA that opens the onboarding wizard
- **REQ-006:** WHEN the user completes Step 1 (squad details) THEN the system SHALL create the squad in the database and advance to Step 2
- **REQ-007:** WHEN the user completes Step 2 (captain config) THEN the system SHALL create the captain agent with the specified adapter type, adapter config, and model, then advance to Step 3
- **REQ-008:** WHEN the user completes Step 3 (first task) THEN the system SHALL create an issue assigned to the captain and advance to Step 4
- **REQ-009:** WHEN the user clicks "Launch" on Step 4 THEN the system SHALL navigate to the squad dashboard
- **REQ-010:** WHEN the user clicks "Test Environment" on Step 2 THEN the system SHALL invoke the adapter's environment test and display pass/warn/fail results inline

### State-Driven

- **REQ-011:** WHILE the wizard is creating resources (squad, agent, issue) the system SHALL show a loading state and disable the "Next" button
- **REQ-012:** WHILE on Step 2, the system SHALL show adapter-specific configuration fields based on the selected adapter type

### Conditional

- **REQ-013:** IF only one adapter type is available in the registry THEN the system SHALL pre-select it and hide the adapter type selector
- **REQ-014:** IF the adapter environment test fails THEN the system SHALL display the error details and allow the user to retry or proceed anyway
- **REQ-015:** IF the user already has squads THEN the system SHALL use the existing Create Squad dialog (not the wizard) for subsequent squad creation
- **REQ-016:** IF Step 3 (first task) is skipped THEN the system SHALL proceed to Step 4 without creating an issue

## Wizard Steps

### Step 1: Squad Details
- Squad Name (required)
- Issue Prefix (required, 2-10 uppercase alphanumeric)
- Description (optional)

### Step 2: Captain Agent + Adapter Config
- Captain Name (required, pre-filled "Captain")
- Captain Short Name (required, auto-derived from name)
- Adapter Type (dropdown, pre-selected if only one available)
- Working Directory (path input)
- Model (dropdown from adapter's available models)
- "Test Environment" button
- Collapsible advanced section: timeout, env vars, skip permissions

### Step 3: First Task (skippable)
- Task Title (pre-filled with a bootstrap task, e.g., "Set up your workspace")
- Task Description (textarea, pre-filled with starter instructions)
- Skip button

### Step 4: Launch
- Summary of what was created (squad name, captain name, adapter, task)
- "Go to Dashboard" button

## Non-Functional Requirements

- **REQ-017:** The wizard SHALL render within 200ms of opening (no lazy-loaded heavy dependencies)
- **REQ-018:** The system SHALL preserve wizard state in component state only (no persistence to localStorage or server between steps — closing the wizard discards progress)
- **REQ-019:** The wizard SHALL be keyboard-navigable (Cmd/Ctrl+Enter to advance, Escape to close)

## Constraints

- Backend squad creation endpoint must be updated to accept captain adapter config (currently hardcoded)
- Adapter environment test endpoint must be exposed (new API: `POST /api/adapters/{type}/test-environment`)
- Available models endpoint must be exposed (new API: `GET /api/adapters/{type}/models`)
- Only `claude_local` and `process` adapters exist today — wizard must handle both but optimize UX for `claude_local` as the primary adapter

## Acceptance Criteria

- [ ] New user sees full-screen onboarding wizard when they have no squads
- [ ] User can create a squad + captain with adapter config in a single guided flow
- [ ] Captain is created with user-specified adapter type, working directory, and model
- [ ] "Test Environment" button validates the adapter config before proceeding
- [ ] User can optionally create a bootstrap task assigned to the captain
- [ ] Existing users with squads still use the current Create Squad dialog
- [ ] Keyboard shortcuts (Cmd+Enter, Escape) work throughout the wizard
- [ ] Back navigation preserves form state
- [ ] Error states are shown inline with retry options

## Out of Scope

- Onboarding wizard for subsequent squad creation (keep existing dialog)
- Squad-level adapter defaults (config lives on agent only)
- Secret management UI (future feature 19)
- Agent hiring beyond the captain (user creates additional agents via existing flow)
- Adapter config revision/audit trail
