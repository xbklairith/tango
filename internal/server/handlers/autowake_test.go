package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// --- Auto-Wake on Issue Assignment Behavioral Tests ---
// These tests verify that assigning an agent to an issue works correctly.
// The auto-wake side effect (wakeup enqueue) is tested via the E2E golden journey.

func TestAutoWake_AssignAgentToIssue_Succeeds(t *testing.T) {
	env := makeEnv(t, "authenticated", false)
	cookie, squadID := setupSquadAndAuth(t, env, "autowake@test.com")

	agent := getSquadCaptain(t, env, cookie, squadID)

	issue, code := createIssue(t, env, cookie, squadID, map[string]any{
		"title":  "Auto-wake test task",
		"status": "todo",
	})
	if code != http.StatusCreated {
		t.Fatalf("create issue: expected 201, got %d", code)
	}

	// Assign agent to issue via PATCH
	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+issue.ID, map[string]any{
		"assigneeAgentId": agent.ID,
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusOK {
		t.Fatalf("assign agent: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var updated issueResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.AssigneeAgentID == nil || *updated.AssigneeAgentID != agent.ID {
		t.Errorf("expected assigneeAgentId = %s, got %v", agent.ID, updated.AssigneeAgentID)
	}
}

func TestAutoWake_ClearAssignment_Succeeds(t *testing.T) {
	env := makeEnv(t, "authenticated", false)
	cookie, squadID := setupSquadAndAuth(t, env, "clear@test.com")

	agent := getSquadCaptain(t, env, cookie, squadID)

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title":           "Clear assignment task",
		"assigneeAgentId": agent.ID,
	})

	// Clear the assignment (set to null)
	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+issue.ID, map[string]any{
		"assigneeAgentId": nil,
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusOK {
		t.Fatalf("clear assignment: expected 200, got %d", rr.Code)
	}

	var updated issueResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.AssigneeAgentID != nil {
		t.Errorf("expected nil assigneeAgentId, got %v", *updated.AssigneeAgentID)
	}
}
