package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/xb/ari/internal/auth"
)

type agentResp struct {
	ID                 string          `json:"id"`
	SquadID            string          `json:"squadId"`
	Name               string          `json:"name"`
	ShortName          string          `json:"shortName"`
	Role               string          `json:"role"`
	Status             string          `json:"status"`
	ParentAgentID      *string         `json:"parentAgentId"`
	AdapterType        *string         `json:"adapterType"`
	AdapterConfig      json.RawMessage `json:"adapterConfig"`
	SystemPrompt       *string         `json:"systemPrompt"`
	Model              *string         `json:"model"`
	BudgetMonthlyCents *int64          `json:"budgetMonthlyCents"`
	CreatedAt          string          `json:"createdAt"`
	UpdatedAt          string          `json:"updatedAt"`
}

// setupSquadAndAuth creates a user, logs in, creates a squad, and returns (cookie, squadID).
func setupSquadAndAuth(t *testing.T, env *testEnv, email string) (*http.Cookie, string) {
	t.Helper()
	registerUser(t, env, email, "TestUser", strongPassword())
	loginRR, _ := loginUser(t, env, email, strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name":        "Test Squad",
		"issuePrefix": fmt.Sprintf("TS%s", strings.ToUpper(email[:2])),
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create squad: status = %d, body: %s", rr.Code, rr.Body.String())
	}
	var squad squadResp
	json.NewDecoder(rr.Body).Decode(&squad)
	return cookie, squad.ID
}

func createAgent(t *testing.T, env *testEnv, cookie *http.Cookie, body map[string]any) (*agentResp, int) {
	t.Helper()
	rr := doJSON(t, env.handler, "POST", "/api/agents", body, []*http.Cookie{cookie})
	if rr.Code == http.StatusCreated {
		var a agentResp
		json.NewDecoder(rr.Body).Decode(&a)
		return &a, rr.Code
	}
	return nil, rr.Code
}

// --- CRUD Tests ---

func TestCreateAgent_Captain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-captain@example.com")

	agent, status := createAgent(t, env, cookie, map[string]any{
		"squadId":   squadID,
		"name":      "Alice",
		"shortName": "alice",
		"role":      "captain",
	})
	if status != http.StatusCreated {
		t.Fatalf("create captain: status = %d, want 201", status)
	}
	if agent.Name != "Alice" {
		t.Errorf("name = %q, want %q", agent.Name, "Alice")
	}
	if agent.Role != "captain" {
		t.Errorf("role = %q, want %q", agent.Role, "captain")
	}
	if agent.Status != "active" {
		t.Errorf("status = %q, want %q", agent.Status, "active")
	}
	if agent.SquadID != squadID {
		t.Errorf("squadId = %q, want %q", agent.SquadID, squadID)
	}
}

func TestCreateAgent_FullHierarchy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-hier@example.com")

	// Create captain
	captain, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "Captain", "shortName": "captain", "role": "captain",
	})

	// Create lead under captain
	lead, status := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "Lead", "shortName": "lead", "role": "lead",
		"parentAgentId": captain.ID,
	})
	if status != http.StatusCreated {
		t.Fatalf("create lead: status = %d, want 201", status)
	}
	if lead.ParentAgentID == nil || *lead.ParentAgentID != captain.ID {
		t.Errorf("lead parentAgentId = %v, want %s", lead.ParentAgentID, captain.ID)
	}

	// Create member under lead
	member, status := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "Member", "shortName": "member", "role": "member",
		"parentAgentId": lead.ID,
	})
	if status != http.StatusCreated {
		t.Fatalf("create member: status = %d, want 201", status)
	}
	if member.ParentAgentID == nil || *member.ParentAgentID != lead.ID {
		t.Errorf("member parentAgentId = %v, want %s", member.ParentAgentID, lead.ID)
	}
}

func TestCreateAgent_DuplicateShortName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-dup@example.com")

	captain, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "A1", "shortName": "alice", "role": "captain",
	})

	lead, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "Lead", "shortName": "lead1", "role": "lead",
		"parentAgentId": captain.ID,
	})

	// Try to create a member with the same shortName as captain (valid hierarchy, but duplicate name)
	rr := doJSON(t, env.handler, "POST", "/api/agents", map[string]any{
		"squadId": squadID, "name": "A2", "shortName": "alice", "role": "member",
		"parentAgentId": lead.ID,
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusConflict {
		t.Errorf("duplicate shortName: status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateAgent_SecondCaptain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-2cap@example.com")

	createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "C1", "shortName": "captain-1", "role": "captain",
	})

	// REQ-AGT-071: second captain returns 409 CONFLICT
	rr := doJSON(t, env.handler, "POST", "/api/agents", map[string]any{
		"squadId": squadID, "name": "C2", "shortName": "captain-2", "role": "captain",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusConflict {
		t.Errorf("second captain: status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateAgent_InvalidHierarchy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-badhier@example.com")

	// Lead without parent
	rr := doJSON(t, env.handler, "POST", "/api/agents", map[string]any{
		"squadId": squadID, "name": "Bad Lead", "shortName": "bad-lead", "role": "lead",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("lead without parent: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateAgent_PendingApproval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-pending@example.com")

	// Enable requireApprovalForNewAgents
	doJSON(t, env.handler, "PATCH", "/api/squads/"+squadID, map[string]any{
		"settings": map[string]any{"requireApprovalForNewAgents": true},
	}, []*http.Cookie{cookie})

	agent, status := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "PendingBot", "shortName": "pending-bot", "role": "captain",
	})
	if status != http.StatusCreated {
		t.Fatalf("create with approval: status = %d, want 201", status)
	}
	if agent.Status != "pending_approval" {
		t.Errorf("status = %q, want %q", agent.Status, "pending_approval")
	}
}

func TestListAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-list@example.com")

	createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "A1", "shortName": "a1", "role": "captain",
	})

	rr := doJSON(t, env.handler, "GET", "/api/agents?squadId="+squadID, nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("list agents: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var agents []agentResp
	json.NewDecoder(rr.Body).Decode(&agents)
	if len(agents) != 1 {
		t.Errorf("got %d agents, want 1", len(agents))
	}
}

func TestListAgents_MissingSquadId(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	registerUser(t, env, "agent-nosq@example.com", "Test", strongPassword())
	loginRR, _ := loginUser(t, env, "agent-nosq@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "GET", "/api/agents", nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing squadId: status = %d, want 400", rr.Code)
	}
}

func TestGetAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-get@example.com")

	created, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "GetMe", "shortName": "get-me", "role": "captain",
	})

	rr := doJSON(t, env.handler, "GET", "/api/agents/"+created.ID, nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get agent: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var agent agentResp
	json.NewDecoder(rr.Body).Decode(&agent)
	if agent.ID != created.ID {
		t.Errorf("id = %q, want %q", agent.ID, created.ID)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	registerUser(t, env, "agent-nf@example.com", "Test", strongPassword())
	loginRR, _ := loginUser(t, env, "agent-nf@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "GET", "/api/agents/00000000-0000-0000-0000-000000000001", nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusNotFound {
		t.Errorf("not found: status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// --- Status Transition Tests ---

func TestTransitionAgentStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-trans@example.com")

	captain, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "Bot", "shortName": "bot", "role": "captain",
	})

	// active -> idle
	rr := doJSON(t, env.handler, "POST", "/api/agents/"+captain.ID+"/transition",
		map[string]any{"status": "idle"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("active->idle: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var updated agentResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Status != "idle" {
		t.Errorf("status = %q, want idle", updated.Status)
	}

	// idle -> running
	rr = doJSON(t, env.handler, "POST", "/api/agents/"+captain.ID+"/transition",
		map[string]any{"status": "running"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("idle->running: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// running -> terminated
	rr = doJSON(t, env.handler, "POST", "/api/agents/"+captain.ID+"/transition",
		map[string]any{"status": "terminated"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("running->terminated: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// terminated -> active (should fail)
	rr = doJSON(t, env.handler, "POST", "/api/agents/"+captain.ID+"/transition",
		map[string]any{"status": "active"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("terminated->active: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestTransitionAgentStatus_Invalid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-badtrans@example.com")

	captain, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "Bot2", "shortName": "bot2", "role": "captain",
	})

	// active -> running (invalid, must go through idle)
	rr := doJSON(t, env.handler, "POST", "/api/agents/"+captain.ID+"/transition",
		map[string]any{"status": "running"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("active->running: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	var errBody errResp
	json.NewDecoder(rr.Body).Decode(&errBody)
	if errBody.Code != "INVALID_STATUS_TRANSITION" {
		t.Errorf("code = %q, want INVALID_STATUS_TRANSITION", errBody.Code)
	}
}

// --- Update Tests ---

func TestUpdateAgent_Name(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-upd@example.com")

	captain, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "OldName", "shortName": "old-name", "role": "captain",
	})

	rr := doJSON(t, env.handler, "PATCH", "/api/agents/"+captain.ID,
		map[string]any{"name": "NewName"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("update name: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var updated agentResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Name != "NewName" {
		t.Errorf("name = %q, want NewName", updated.Name)
	}
	if updated.ShortName != "old-name" {
		t.Errorf("shortName changed unexpectedly: %q", updated.ShortName)
	}
}

func TestUpdateAgent_RejectSquadIdChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-sqch@example.com")

	captain, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "Bot", "shortName": "bot", "role": "captain",
	})

	rr := doJSON(t, env.handler, "PATCH", "/api/agents/"+captain.ID,
		map[string]any{"squadId": "00000000-0000-0000-0000-000000000001"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("squadId change: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateAgent_StatusTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-stup@example.com")

	captain, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "Bot", "shortName": "bot", "role": "captain",
	})

	// Valid transition via PATCH
	rr := doJSON(t, env.handler, "PATCH", "/api/agents/"+captain.ID,
		map[string]any{"status": "idle"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("status update: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Invalid transition via PATCH
	rr = doJSON(t, env.handler, "PATCH", "/api/agents/"+captain.ID,
		map[string]any{"status": "error"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid status: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateAgent_TerminatedCaptainAllowsNewCaptain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "agent-termcap@example.com")

	captain, _ := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "C1", "shortName": "c1", "role": "captain",
	})

	// Terminate the captain
	doJSON(t, env.handler, "POST", "/api/agents/"+captain.ID+"/transition",
		map[string]any{"status": "terminated"}, []*http.Cookie{cookie})

	// Create new captain should succeed
	_, status := createAgent(t, env, cookie, map[string]any{
		"squadId": squadID, "name": "C2", "shortName": "c2", "role": "captain",
	})
	if status != http.StatusCreated {
		t.Errorf("new captain after termination: status = %d, want 201", status)
	}
}
