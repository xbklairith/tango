package handlers

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
)

// --- buildInvokeInput Behavioral Tests ---
// These tests verify that buildInvokeInput enriches env vars with issue details.

func TestBuildInvokeInput_IncludesIssueDetails(t *testing.T) {
	t.Skip("buildInvokeInput issue enrichment not yet implemented")

	// Given: a wakeup context with ARI_TASK_ID pointing to an issue
	// When: buildInvokeInput is called
	// Then: the result includes ARI_ISSUE_TITLE, ARI_ISSUE_DESCRIPTION, ARI_PROMPT, ARI_SYSTEM_PROMPT

	squadID := uuid.New()
	agentID := uuid.New()
	runID := uuid.New()
	issueID := uuid.New()

	ctxJSON, _ := json.Marshal(map[string]string{
		"ARI_TASK_ID": issueID.String(),
	})

	agent := db.Agent{
		ID:        agentID,
		SquadID:   squadID,
		Name:      "test-agent",
		ShortName: "TA",
		Role:      "member",
	}

	wakeup := db.WakeupRequest{
		ID:               uuid.New(),
		SquadID:          squadID,
		AgentID:          agentID,
		InvocationSource: "assignment",
		ContextJson:      ctxJSON,
	}

	run := db.HeartbeatRun{
		ID:      runID,
		SquadID: squadID,
		AgentID: agentID,
	}

	// TODO: This test needs RunService with a real/mock queries that can load the issue.
	// For now, verify the contract we expect:
	// - EnvVars must include ARI_ISSUE_TITLE
	// - EnvVars must include ARI_ISSUE_DESCRIPTION
	// - EnvVars must include ARI_PROMPT (assembled prompt with instructions)
	// - EnvVars must include ARI_SYSTEM_PROMPT if agent has one

	_ = agent
	_ = wakeup
	_ = run
}

func TestBuildInvokeInput_WithoutTaskID_NoIssueEnvVars(t *testing.T) {
	t.Skip("buildInvokeInput issue enrichment not yet implemented")

	// Given: a wakeup context without ARI_TASK_ID
	// When: buildInvokeInput is called
	// Then: ARI_ISSUE_TITLE is absent

	// Verify that when there's no task ID, the issue-related env vars are not set
}

func TestBuildInvokeInput_PromptFormat(t *testing.T) {
	t.Skip("buildInvokeInput issue enrichment not yet implemented")

	// Given: an agent with a system prompt and an assigned issue
	// When: buildInvokeInput is called
	// Then: ARI_PROMPT contains the agent name, role, system prompt,
	//       issue title, description, and API callback instructions

	// Expected format:
	// You are {agent.Name}, a {agent.Role} in squad {squad.Name}.
	// {systemPrompt}
	// Your current task: {issue.Title} ({issue.Identifier})
	// Description: {issue.Description}
	// API: {ARI_API_URL}
	// Auth: Bearer {ARI_API_KEY}
	// When done, mark the task complete:
	// PATCH {ARI_API_URL}/api/agent/me/task
	// Body: {"issueId": "{issueID}", "status": "done"}
}
