package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestE2E_GoldenAgentJourney tests the full agent lifecycle:
// create squad → add agent → create issue → assign to agent →
// system spawns agent → agent resolves issue → marks done via API.
//
// This is the core value proposition of Ari — the end-to-end flow.
func TestE2E_GoldenAgentJourney(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Find the mock-agent.sh script relative to this test file
	_, thisFile, _, _ := runtime.Caller(0)
	mockAgentScript := filepath.Join(filepath.Dir(thisFile), "testdata", "mock-agent.sh")
	if _, err := os.Stat(mockAgentScript); err != nil {
		t.Fatalf("mock-agent.sh not found at %s: %v", mockAgentScript, err)
	}

	baseURL, cancel := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	defer cancel()

	c := newE2EClient(t, baseURL)

	// 1. Register + login
	if status := c.register(t, "journey@test.com", "Journey User"); status != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", status)
	}
	loginStatus, _ := c.login(t, "journey@test.com")
	if loginStatus != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", loginStatus)
	}

	// 2. Create squad
	var squad e2eSquadResp
	status := c.doJSON(t, "POST", "/api/squads", map[string]any{
		"name":        "Journey Squad",
		"slug":        "journey-squad",
		"issuePrefix": "JS",
	}, &squad)
	if status != http.StatusCreated {
		t.Fatalf("create squad: expected 201, got %d", status)
	}

	// 3. Create agent with process adapter pointing to mock-agent.sh
	var agent e2eAgentResp
	agentStatus, agentBody := c.do(t, "POST", "/api/agents", map[string]any{
		"name":      "mock-agent",
		"shortName": "ma",
		"role":      "captain",
		"squadId":   squad.ID,
		"adapterType": "process",
		"adapterConfig": map[string]any{
			"command": "bash",
			"args":    []string{mockAgentScript},
		},
	})
	if agentStatus != http.StatusCreated {
		t.Fatalf("create agent: expected 201, got %d; body: %s", agentStatus, string(agentBody))
	}
	json.Unmarshal(agentBody, &agent)

	// 4. Ensure agent is active (may already be active if squad doesn't require approval)
	if agent.Status != "active" {
		status = c.doJSON(t, "POST", "/api/agents/"+agent.ID+"/transition", map[string]any{
			"status": "active",
		}, nil)
		if status != http.StatusOK {
			t.Fatalf("transition to active: expected 200, got %d", status)
		}
	}

	// 5. Create issue and assign to agent (should trigger auto-wake)
	var issue e2eIssueResp
	status = c.doJSON(t, "POST", "/api/squads/"+squad.ID+"/issues", map[string]any{
		"title":           "Resolve this bug",
		"description":     "There's a critical bug in the login flow",
		"status":          "in_progress",
		"assigneeAgentId": agent.ID,
	}, &issue)
	if status != http.StatusCreated {
		t.Fatalf("create issue: expected 201, got %d", status)
	}

	// 6. Poll: wait for issue status to become "done" (agent should auto-wake and mark it)
	deadline := time.Now().Add(30 * time.Second)
	var finalIssue e2eIssueResp
	for time.Now().Before(deadline) {
		status = c.doJSON(t, "GET", "/api/issues/"+issue.ID, nil, &finalIssue)
		if status != http.StatusOK {
			t.Fatalf("get issue: expected 200, got %d", status)
		}
		if finalIssue.Status == "done" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if finalIssue.Status != "done" {
		t.Fatalf("issue status: expected 'done', got %q after 30s", finalIssue.Status)
	}

	// 7. Verify agent status is back to idle
	var finalAgent e2eAgentResp
	status = c.doJSON(t, "GET", "/api/agents/"+agent.ID, nil, &finalAgent)
	if status != http.StatusOK {
		t.Fatalf("get agent: expected 200, got %d", status)
	}
	if finalAgent.Status != "idle" {
		t.Errorf("agent status: expected 'idle', got %q", finalAgent.Status)
	}

	// 8. Verify activity log has entries
	_, activityData := c.do(t, "GET", "/api/squads/"+squad.ID+"/activity?limit=50", nil)
	var activityResp struct {
		Data []json.RawMessage `json:"data"`
	}
	json.Unmarshal(activityData, &activityResp)
	if len(activityResp.Data) == 0 {
		t.Error("expected activity log entries")
	}

	t.Logf("GoldenAgentJourney completed: issue %s marked done by agent %s", issue.Identifier, agent.Name)
}

// TestE2E_AgentGetMe_ReturnsContext tests that an agent can call GET /api/agent/me
// using its run token and get back its context.
func TestE2E_AgentGetMe_ReturnsContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	t.Skip("Agent self-service endpoints not yet implemented")

	// This test would:
	// 1. Start server
	// 2. Create squad + agent
	// 3. Wake agent manually
	// 4. While agent is running, have it call GET /api/agent/me
	// 5. Verify it returns agent info, squad info, and tasks
}
