package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/xb/ari/internal/auth"
)

// --- Response Types ---

type projectResp struct {
	ID          string  `json:"id"`
	SquadID     string  `json:"squadId"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

type goalResp struct {
	ID          string  `json:"id"`
	SquadID     string  `json:"squadId"`
	ParentID    *string `json:"parentId"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// --- Helpers ---

func createProject(t *testing.T, env *testEnv, cookie *http.Cookie, squadID string, body map[string]any) (*projectResp, int) {
	t.Helper()
	rr := doJSON(t, env.handler, "POST", fmt.Sprintf("/api/squads/%s/projects", squadID), body, []*http.Cookie{cookie})
	if rr.Code == http.StatusCreated {
		var p projectResp
		json.NewDecoder(rr.Body).Decode(&p)
		return &p, rr.Code
	}
	return nil, rr.Code
}

func createGoal(t *testing.T, env *testEnv, cookie *http.Cookie, squadID string, body map[string]any) (*goalResp, int) {
	t.Helper()
	rr := doJSON(t, env.handler, "POST", fmt.Sprintf("/api/squads/%s/goals", squadID), body, []*http.Cookie{cookie})
	if rr.Code == http.StatusCreated {
		var g goalResp
		json.NewDecoder(rr.Body).Decode(&g)
		return &g, rr.Code
	}
	return nil, rr.Code
}

// ==================== Project Tests ====================

func TestCreateProject_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "proj-create@example.com")

	desc := "A test project"
	p, code := createProject(t, env, cookie, squadID, map[string]any{
		"name":        "My Project",
		"description": desc,
	})
	if code != http.StatusCreated {
		t.Fatalf("create project: status = %d, want %d", code, http.StatusCreated)
	}
	if p.Name != "My Project" {
		t.Errorf("name = %q, want %q", p.Name, "My Project")
	}
	if p.Description == nil || *p.Description != desc {
		t.Errorf("description = %v, want %q", p.Description, desc)
	}
	if p.Status != "active" {
		t.Errorf("status = %q, want %q", p.Status, "active")
	}
	if p.SquadID != squadID {
		t.Errorf("squadId = %q, want %q", p.SquadID, squadID)
	}
	if p.ID == "" {
		t.Error("id should not be empty")
	}
	if p.CreatedAt == "" {
		t.Error("createdAt should not be empty")
	}
	if p.UpdatedAt == "" {
		t.Error("updatedAt should not be empty")
	}
}

func TestCreateProject_DuplicateName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "proj-dup@example.com")

	_, code := createProject(t, env, cookie, squadID, map[string]any{"name": "Duplicate"})
	if code != http.StatusCreated {
		t.Fatalf("first create: status = %d", code)
	}

	rr := doJSON(t, env.handler, "POST", fmt.Sprintf("/api/squads/%s/projects", squadID),
		map[string]any{"name": "Duplicate"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate create: status = %d, want %d", rr.Code, http.StatusConflict)
	}
	var e errResp
	json.NewDecoder(rr.Body).Decode(&e)
	if e.Code != "PROJECT_NAME_TAKEN" {
		t.Errorf("error code = %q, want %q", e.Code, "PROJECT_NAME_TAKEN")
	}
}

func TestCreateProject_MissingName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "proj-noname@example.com")

	rr := doJSON(t, env.handler, "POST", fmt.Sprintf("/api/squads/%s/projects", squadID),
		map[string]any{"name": ""}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty name: status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListProjects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "proj-list@example.com")

	for i := 1; i <= 3; i++ {
		_, code := createProject(t, env, cookie, squadID, map[string]any{
			"name": fmt.Sprintf("Project %d", i),
		})
		if code != http.StatusCreated {
			t.Fatalf("create project %d: status = %d", i, code)
		}
	}

	rr := doJSON(t, env.handler, "GET", fmt.Sprintf("/api/squads/%s/projects", squadID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("list projects: status = %d, want %d", rr.Code, http.StatusOK)
	}
	var projects []projectResp
	json.NewDecoder(rr.Body).Decode(&projects)
	if len(projects) != 3 {
		t.Errorf("len(projects) = %d, want 3", len(projects))
	}
}

func TestGetProject_ByID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "proj-get@example.com")

	p, _ := createProject(t, env, cookie, squadID, map[string]any{"name": "GetMe"})

	rr := doJSON(t, env.handler, "GET", fmt.Sprintf("/api/projects/%s", p.ID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get project: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got projectResp
	json.NewDecoder(rr.Body).Decode(&got)
	if got.ID != p.ID {
		t.Errorf("id = %q, want %q", got.ID, p.ID)
	}
	if got.Name != "GetMe" {
		t.Errorf("name = %q, want %q", got.Name, "GetMe")
	}
}

func TestGetProject_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, _ := setupSquadAndAuth(t, env, "proj-notfound@example.com")

	rr := doJSON(t, env.handler, "GET", "/api/projects/00000000-0000-0000-0000-000000000000", nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusNotFound {
		t.Errorf("get non-existent project: status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestUpdateProject_Name(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "proj-upname@example.com")

	p, _ := createProject(t, env, cookie, squadID, map[string]any{"name": "OldName"})

	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/projects/%s", p.ID),
		map[string]any{"name": "NewName"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("update project name: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var updated projectResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Name != "NewName" {
		t.Errorf("name = %q, want %q", updated.Name, "NewName")
	}
}

func TestUpdateProject_StatusTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "proj-status@example.com")

	p, _ := createProject(t, env, cookie, squadID, map[string]any{"name": "StatusProj"})

	// active -> completed (valid)
	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/projects/%s", p.ID),
		map[string]any{"status": "completed"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("active->completed: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// completed -> archived (valid)
	rr = doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/projects/%s", p.ID),
		map[string]any{"status": "archived"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("completed->archived: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// archived -> completed (invalid)
	rr = doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/projects/%s", p.ID),
		map[string]any{"status": "completed"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("archived->completed: status = %d, want %d", rr.Code, http.StatusUnprocessableEntity)
	}
	var e errResp
	json.NewDecoder(rr.Body).Decode(&e)
	if e.Code != "INVALID_STATUS_TRANSITION" {
		t.Errorf("error code = %q, want %q", e.Code, "INVALID_STATUS_TRANSITION")
	}
}

func TestUpdateProject_DuplicateNameOnUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "proj-dupup@example.com")

	createProject(t, env, cookie, squadID, map[string]any{"name": "Existing"})
	p2, _ := createProject(t, env, cookie, squadID, map[string]any{"name": "Other"})

	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/projects/%s", p2.ID),
		map[string]any{"name": "Existing"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusConflict {
		t.Fatalf("dup name on update: status = %d, want %d", rr.Code, http.StatusConflict)
	}
	var e errResp
	json.NewDecoder(rr.Body).Decode(&e)
	if e.Code != "PROJECT_NAME_TAKEN" {
		t.Errorf("error code = %q, want %q", e.Code, "PROJECT_NAME_TAKEN")
	}
}

// ==================== Goal Tests ====================

func TestCreateGoal_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-create@example.com")

	desc := "A test goal"
	g, code := createGoal(t, env, cookie, squadID, map[string]any{
		"title":       "My Goal",
		"description": desc,
	})
	if code != http.StatusCreated {
		t.Fatalf("create goal: status = %d, want %d", code, http.StatusCreated)
	}
	if g.Title != "My Goal" {
		t.Errorf("title = %q, want %q", g.Title, "My Goal")
	}
	if g.Description == nil || *g.Description != desc {
		t.Errorf("description = %v, want %q", g.Description, desc)
	}
	if g.Status != "active" {
		t.Errorf("status = %q, want %q", g.Status, "active")
	}
	if g.ParentID != nil {
		t.Errorf("parentId = %v, want nil", g.ParentID)
	}
	if g.SquadID != squadID {
		t.Errorf("squadId = %q, want %q", g.SquadID, squadID)
	}
	if g.ID == "" {
		t.Error("id should not be empty")
	}
}

func TestCreateGoal_WithParent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-parent@example.com")

	parent, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Parent Goal"})

	child, code := createGoal(t, env, cookie, squadID, map[string]any{
		"title":    "Child Goal",
		"parentId": parent.ID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create child goal: status = %d, want %d", code, http.StatusCreated)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Errorf("parentId = %v, want %q", child.ParentID, parent.ID)
	}
}

func TestCreateGoal_MaxDepthExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-depth@example.com")

	// MaxGoalDepth is 5. Create a chain of 5 goals (depth 1..5).
	// The 1st goal is at depth 1 (no parent).
	// Each subsequent goal has the previous as parent.
	var parentID string
	for i := 1; i <= 5; i++ {
		body := map[string]any{"title": fmt.Sprintf("Goal Depth %d", i)}
		if parentID != "" {
			body["parentId"] = parentID
		}
		g, code := createGoal(t, env, cookie, squadID, body)
		if i <= 5 && code != http.StatusCreated {
			// Depth 1 has 0 ancestors -> depth = 0+2=2 when checking? No.
			// For the first child (i=2): ancestors of parent (i=1) = [] -> len=0, 0+2=2 <= 5 OK
			// For i=3: ancestors of parent (i=2) = [i=1] -> len=1, 1+2=3 <= 5 OK
			// For i=4: ancestors of parent (i=3) = [i=2, i=1] -> len=2, 2+2=4 <= 5 OK
			// For i=5: ancestors of parent (i=4) = [i=3,i=2,i=1] -> len=3, 3+2=5 <= 5 OK
			t.Fatalf("create goal depth %d: status = %d, want %d", i, code, http.StatusCreated)
		}
		if g != nil {
			parentID = g.ID
		}
	}

	// 6th level should fail: ancestors of parent (i=5) = [i=4,i=3,i=2,i=1] -> len=4, 4+2=6 > 5
	rr := doJSON(t, env.handler, "POST", fmt.Sprintf("/api/squads/%s/goals", squadID),
		map[string]any{"title": "Too Deep", "parentId": parentID}, []*http.Cookie{cookie})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("max depth exceeded: status = %d, want %d; body: %s", rr.Code, http.StatusUnprocessableEntity, rr.Body.String())
	}
	var e errResp
	json.NewDecoder(rr.Body).Decode(&e)
	if e.Code != "MAX_DEPTH_EXCEEDED" {
		t.Errorf("error code = %q, want %q", e.Code, "MAX_DEPTH_EXCEEDED")
	}
}

func TestListGoals_All(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-listall@example.com")

	for i := 1; i <= 3; i++ {
		_, code := createGoal(t, env, cookie, squadID, map[string]any{
			"title": fmt.Sprintf("Goal %d", i),
		})
		if code != http.StatusCreated {
			t.Fatalf("create goal %d: status = %d", i, code)
		}
	}

	rr := doJSON(t, env.handler, "GET", fmt.Sprintf("/api/squads/%s/goals", squadID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("list goals: status = %d, want %d", rr.Code, http.StatusOK)
	}
	var goals []goalResp
	json.NewDecoder(rr.Body).Decode(&goals)
	if len(goals) != 3 {
		t.Errorf("len(goals) = %d, want 3", len(goals))
	}
}

func TestListGoals_TopLevel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-toplevel@example.com")

	parent, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Top Level"})
	createGoal(t, env, cookie, squadID, map[string]any{"title": "Top Level 2"})
	createGoal(t, env, cookie, squadID, map[string]any{"title": "Child", "parentId": parent.ID})

	rr := doJSON(t, env.handler, "GET", fmt.Sprintf("/api/squads/%s/goals?parentId=null", squadID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("list top-level goals: status = %d, want %d", rr.Code, http.StatusOK)
	}
	var goals []goalResp
	json.NewDecoder(rr.Body).Decode(&goals)
	if len(goals) != 2 {
		t.Errorf("len(goals) = %d, want 2", len(goals))
	}
	for _, g := range goals {
		if g.ParentID != nil {
			t.Errorf("goal %s has parentId = %v, want nil", g.ID, g.ParentID)
		}
	}
}

func TestListGoals_ByParent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-byparent@example.com")

	parent, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Parent"})
	createGoal(t, env, cookie, squadID, map[string]any{"title": "Child 1", "parentId": parent.ID})
	createGoal(t, env, cookie, squadID, map[string]any{"title": "Child 2", "parentId": parent.ID})
	createGoal(t, env, cookie, squadID, map[string]any{"title": "Unrelated"})

	rr := doJSON(t, env.handler, "GET", fmt.Sprintf("/api/squads/%s/goals?parentId=%s", squadID, parent.ID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("list goals by parent: status = %d, want %d", rr.Code, http.StatusOK)
	}
	var goals []goalResp
	json.NewDecoder(rr.Body).Decode(&goals)
	if len(goals) != 2 {
		t.Errorf("len(goals) = %d, want 2", len(goals))
	}
	for _, g := range goals {
		if g.ParentID == nil || *g.ParentID != parent.ID {
			t.Errorf("goal %s parentId = %v, want %q", g.ID, g.ParentID, parent.ID)
		}
	}
}

func TestGetGoal_ByID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-getid@example.com")

	g, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Fetch Me"})

	rr := doJSON(t, env.handler, "GET", fmt.Sprintf("/api/goals/%s", g.ID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get goal: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got goalResp
	json.NewDecoder(rr.Body).Decode(&got)
	if got.ID != g.ID {
		t.Errorf("id = %q, want %q", got.ID, g.ID)
	}
	if got.Title != "Fetch Me" {
		t.Errorf("title = %q, want %q", got.Title, "Fetch Me")
	}
}

func TestUpdateGoal_Title(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-uptitle@example.com")

	g, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Old Title"})

	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/goals/%s", g.ID),
		map[string]any{"title": "New Title"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("update goal title: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var updated goalResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Title != "New Title" {
		t.Errorf("title = %q, want %q", updated.Title, "New Title")
	}
}

func TestUpdateGoal_StatusTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-status@example.com")

	g, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "StatusGoal"})

	// active -> completed (valid)
	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/goals/%s", g.ID),
		map[string]any{"status": "completed"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("active->completed: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// completed -> archived (valid)
	rr = doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/goals/%s", g.ID),
		map[string]any{"status": "archived"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("completed->archived: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// archived -> completed (invalid)
	rr = doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/goals/%s", g.ID),
		map[string]any{"status": "completed"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("archived->completed: status = %d, want %d", rr.Code, http.StatusUnprocessableEntity)
	}
	var e errResp
	json.NewDecoder(rr.Body).Decode(&e)
	if e.Code != "INVALID_STATUS_TRANSITION" {
		t.Errorf("error code = %q, want %q", e.Code, "INVALID_STATUS_TRANSITION")
	}
}

func TestUpdateGoal_CycleDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-cycle@example.com")

	// Create chain: A -> B -> C
	a, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Goal A"})
	b, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Goal B", "parentId": a.ID})
	c, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Goal C", "parentId": b.ID})

	// Try to set A.parent = C (would create cycle: A -> C -> B -> A)
	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/goals/%s", a.ID),
		map[string]any{"parentId": c.ID}, []*http.Cookie{cookie})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cycle detection: status = %d, want %d; body: %s", rr.Code, http.StatusUnprocessableEntity, rr.Body.String())
	}
	var e errResp
	json.NewDecoder(rr.Body).Decode(&e)
	if e.Code != "CIRCULAR_REFERENCE" {
		t.Errorf("error code = %q, want %q", e.Code, "CIRCULAR_REFERENCE")
	}
}

func TestUpdateGoal_SelfParent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-self@example.com")

	g, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Self Ref"})

	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/goals/%s", g.ID),
		map[string]any{"parentId": g.ID}, []*http.Cookie{cookie})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("self-parent: status = %d, want %d; body: %s", rr.Code, http.StatusUnprocessableEntity, rr.Body.String())
	}
	var e errResp
	json.NewDecoder(rr.Body).Decode(&e)
	if e.Code != "CIRCULAR_REFERENCE" {
		t.Errorf("error code = %q, want %q", e.Code, "CIRCULAR_REFERENCE")
	}
}

func TestUpdateGoal_ClearParent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "goal-clear@example.com")

	parent, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Parent"})
	child, _ := createGoal(t, env, cookie, squadID, map[string]any{"title": "Child", "parentId": parent.ID})

	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("child parentId = %v, want %q", child.ParentID, parent.ID)
	}

	// Clear parent by setting parentId to null
	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/goals/%s", child.ID),
		map[string]any{"parentId": nil}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("clear parent: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var updated goalResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.ParentID != nil {
		t.Errorf("parentId = %v, want nil", updated.ParentID)
	}
}
