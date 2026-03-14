package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/xb/ari/internal/auth"
)

// Squad response types for tests
type squadResp struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug"`
	IssuePrefix        string          `json:"issuePrefix"`
	Description        string          `json:"description"`
	Status             string          `json:"status"`
	Settings           json.RawMessage `json:"settings"`
	IssueCounter       int64           `json:"issueCounter"`
	BudgetMonthlyCents *int64          `json:"budgetMonthlyCents"`
	BrandColor         *string         `json:"brandColor"`
	CreatedAt          string          `json:"createdAt"`
	UpdatedAt          string          `json:"updatedAt"`
	Role               string          `json:"role,omitempty"`
}

type memberResp struct {
	ID          string `json:"id"`
	UserID      string `json:"userId"`
	SquadID     string `json:"squadId"`
	Role        string `json:"role"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

// --- Squad CRUD Tests ---

func TestCreateSquad_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	// Register and login
	registerUser(t, env, "squad-creator@example.com", "Creator", strongPassword())
	loginRR, _ := loginUser(t, env, "squad-creator@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	// Create squad
	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name":        "Acme Engineering",
		"issuePrefix": "ACME",
		"description": "The core engineering squad",
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusCreated {
		t.Fatalf("create squad: status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var squad squadResp
	if err := json.NewDecoder(rr.Body).Decode(&squad); err != nil {
		t.Fatalf("decode squad: %v", err)
	}
	if squad.Name != "Acme Engineering" {
		t.Errorf("name = %q, want %q", squad.Name, "Acme Engineering")
	}
	if squad.Slug != "acme-engineering" {
		t.Errorf("slug = %q, want %q", squad.Slug, "acme-engineering")
	}
	if squad.IssuePrefix != "ACME" {
		t.Errorf("issuePrefix = %q, want %q", squad.IssuePrefix, "ACME")
	}
	if squad.Status != "active" {
		t.Errorf("status = %q, want %q", squad.Status, "active")
	}
}

func TestCreateSquad_DuplicatePrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "dup-prefix@example.com", "DupTest", strongPassword())
	loginRR, _ := loginUser(t, env, "dup-prefix@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	// First squad
	doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Squad One", "issuePrefix": "DUPE",
	}, []*http.Cookie{cookie})

	// Second squad with same prefix
	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Squad Two", "issuePrefix": "DUPE",
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate prefix: status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	var errBody errResp
	json.NewDecoder(rr.Body).Decode(&errBody)
	if errBody.Code != "ISSUE_PREFIX_TAKEN" {
		t.Errorf("code = %q, want ISSUE_PREFIX_TAKEN", errBody.Code)
	}
}

func TestCreateSquad_InvalidInput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "invalid-squad@example.com", "Invalid", strongPassword())
	loginRR, _ := loginUser(t, env, "invalid-squad@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	// Missing name
	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"issuePrefix": "TEST",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing name: status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	// Bad prefix
	rr = doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Test Squad", "issuePrefix": "bad",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad prefix: status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListSquads_OnlyMySquads(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	// User A
	registerUser(t, env, "user-a@example.com", "UserA", strongPassword())
	loginA, _ := loginUser(t, env, "user-a@example.com", strongPassword())
	cookieA := sessionCookie(loginA)

	doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "A Squad", "issuePrefix": "ASQD",
	}, []*http.Cookie{cookieA})

	// User B
	registerUser(t, env, "user-b@example.com", "UserB", strongPassword())
	loginB, _ := loginUser(t, env, "user-b@example.com", strongPassword())
	cookieB := sessionCookie(loginB)

	doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "B Squad", "issuePrefix": "BSQD",
	}, []*http.Cookie{cookieB})

	// User A should see only their squad
	rr := doJSON(t, env.handler, "GET", "/api/squads", nil, []*http.Cookie{cookieA})
	if rr.Code != http.StatusOK {
		t.Fatalf("list: status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var squads []squadResp
	json.NewDecoder(rr.Body).Decode(&squads)
	if len(squads) != 1 {
		t.Fatalf("expected 1 squad, got %d", len(squads))
	}
	if squads[0].Name != "A Squad" {
		t.Errorf("expected A Squad, got %q", squads[0].Name)
	}
}

func TestGetSquad_NonMemberReturns404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	// User A creates squad
	registerUser(t, env, "owner-x@example.com", "OwnerX", strongPassword())
	loginA, _ := loginUser(t, env, "owner-x@example.com", strongPassword())
	cookieA := sessionCookie(loginA)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Private Squad", "issuePrefix": "PRIV",
	}, []*http.Cookie{cookieA})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	// User B tries to access
	registerUser(t, env, "intruder@example.com", "Intruder", strongPassword())
	loginB, _ := loginUser(t, env, "intruder@example.com", strongPassword())
	cookieB := sessionCookie(loginB)

	rr := doJSON(t, env.handler, "GET", "/api/squads/"+created.ID, nil, []*http.Cookie{cookieB})
	if rr.Code != http.StatusNotFound {
		t.Errorf("non-member get: status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateSquad_OwnerSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "updater@example.com", "Updater", strongPassword())
	loginRR, _ := loginUser(t, env, "updater@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Update Test", "issuePrefix": "UPDT",
	}, []*http.Cookie{cookie})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	rr := doJSON(t, env.handler, "PATCH", "/api/squads/"+created.ID, map[string]any{
		"description": "Updated description",
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusOK {
		t.Fatalf("update: status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var updated squadResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Description != "Updated description" {
		t.Errorf("description = %q, want %q", updated.Description, "Updated description")
	}
}

func TestUpdateSquad_InvalidStatusTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "transition@example.com", "Transitioner", strongPassword())
	loginRR, _ := loginUser(t, env, "transition@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Transition Test", "issuePrefix": "TRNS",
	}, []*http.Cookie{cookie})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	// Archive the squad
	doJSON(t, env.handler, "DELETE", "/api/squads/"+created.ID, nil, []*http.Cookie{cookie})

	// Try to transition from archived -> active
	rr := doJSON(t, env.handler, "PATCH", "/api/squads/"+created.ID, map[string]any{
		"status": "active",
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid transition: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteSquad_OwnerSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "deleter@example.com", "Deleter", strongPassword())
	loginRR, _ := loginUser(t, env, "deleter@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Delete Test", "issuePrefix": "DELT",
	}, []*http.Cookie{cookie})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	rr := doJSON(t, env.handler, "DELETE", "/api/squads/"+created.ID, nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var deleted squadResp
	json.NewDecoder(rr.Body).Decode(&deleted)
	if deleted.Status != "archived" {
		t.Errorf("status = %q, want archived", deleted.Status)
	}
}

func TestUpdateBudget_OwnerSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "budgeter@example.com", "Budgeter", strongPassword())
	loginRR, _ := loginUser(t, env, "budgeter@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Budget Test", "issuePrefix": "BUDG",
	}, []*http.Cookie{cookie})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	// Set budget
	budget := int64(500000)
	rr := doJSON(t, env.handler, "PATCH", "/api/squads/"+created.ID+"/budgets", map[string]any{
		"budgetMonthlyCents": budget,
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("set budget: status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var updated squadResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.BudgetMonthlyCents == nil || *updated.BudgetMonthlyCents != 500000 {
		t.Errorf("budget = %v, want 500000", updated.BudgetMonthlyCents)
	}

	// Set to unlimited (null)
	rr = doJSON(t, env.handler, "PATCH", "/api/squads/"+created.ID+"/budgets", map[string]any{
		"budgetMonthlyCents": nil,
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("clear budget: status = %d; body: %s", rr.Code, rr.Body.String())
	}
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.BudgetMonthlyCents != nil {
		t.Errorf("budget should be nil, got %v", *updated.BudgetMonthlyCents)
	}
}
