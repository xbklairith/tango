package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/xb/ari/internal/auth"
)

func TestAddMember_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	// Owner creates squad
	registerUser(t, env, "owner-add@example.com", "Owner", strongPassword())
	loginOwner, _ := loginUser(t, env, "owner-add@example.com", strongPassword())
	cookieOwner := sessionCookie(loginOwner)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Add Member Test", "issuePrefix": "ADDM",
		"captainName": "Captain", "captainShortName": "captain-addm",
	}, []*http.Cookie{cookieOwner})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	// Register second user
	regRR := registerUser(t, env, "member-add@example.com", "Member", strongPassword())
	var regUser userResp
	json.NewDecoder(regRR.Body).Decode(&regUser)

	// Add member
	rr := doJSON(t, env.handler, "POST", "/api/squads/"+created.ID+"/members", map[string]any{
		"userId": regUser.ID,
		"role":   "viewer",
	}, []*http.Cookie{cookieOwner})

	if rr.Code != http.StatusCreated {
		t.Fatalf("add member: status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestAddMember_AdminCannotGrantOwner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	// Owner creates squad
	registerUser(t, env, "owner-acl@example.com", "OwnerACL", strongPassword())
	loginOwner, _ := loginUser(t, env, "owner-acl@example.com", strongPassword())
	cookieOwner := sessionCookie(loginOwner)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "ACL Test", "issuePrefix": "ACLX",
		"captainName": "Captain", "captainShortName": "captain-aclx",
	}, []*http.Cookie{cookieOwner})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	// Register admin user
	regAdmin := registerUser(t, env, "admin-acl@example.com", "AdminACL", strongPassword())
	var adminUser userResp
	json.NewDecoder(regAdmin.Body).Decode(&adminUser)

	// Add as admin
	doJSON(t, env.handler, "POST", "/api/squads/"+created.ID+"/members", map[string]any{
		"userId": adminUser.ID, "role": "admin",
	}, []*http.Cookie{cookieOwner})

	// Login as admin
	loginAdmin, _ := loginUser(t, env, "admin-acl@example.com", strongPassword())
	cookieAdmin := sessionCookie(loginAdmin)

	// Register third user
	regThird := registerUser(t, env, "third-acl@example.com", "Third", strongPassword())
	var thirdUser userResp
	json.NewDecoder(regThird.Body).Decode(&thirdUser)

	// Admin tries to grant owner role
	rr := doJSON(t, env.handler, "POST", "/api/squads/"+created.ID+"/members", map[string]any{
		"userId": thirdUser.ID, "role": "owner",
	}, []*http.Cookie{cookieAdmin})

	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin grant owner: status = %d, want 403; body: %s", rr.Code, rr.Body.String())
	}
}

func TestAddMember_Duplicate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "owner-dup@example.com", "OwnerDup", strongPassword())
	loginOwner, _ := loginUser(t, env, "owner-dup@example.com", strongPassword())
	cookieOwner := sessionCookie(loginOwner)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Dup Member Test", "issuePrefix": "DUPM",
		"captainName": "Captain", "captainShortName": "captain-dupm",
	}, []*http.Cookie{cookieOwner})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	regMember := registerUser(t, env, "member-dup@example.com", "MemberDup", strongPassword())
	var memberUser userResp
	json.NewDecoder(regMember.Body).Decode(&memberUser)

	// Add once
	doJSON(t, env.handler, "POST", "/api/squads/"+created.ID+"/members", map[string]any{
		"userId": memberUser.ID, "role": "viewer",
	}, []*http.Cookie{cookieOwner})

	// Add again
	rr := doJSON(t, env.handler, "POST", "/api/squads/"+created.ID+"/members", map[string]any{
		"userId": memberUser.ID, "role": "admin",
	}, []*http.Cookie{cookieOwner})

	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate member: status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
}

func TestLeaveSquad_LastOwnerBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "lone-owner@example.com", "LoneOwner", strongPassword())
	loginOwner, _ := loginUser(t, env, "lone-owner@example.com", strongPassword())
	cookieOwner := sessionCookie(loginOwner)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Leave Test", "issuePrefix": "LEAV",
		"captainName": "Captain", "captainShortName": "captain-leav",
	}, []*http.Cookie{cookieOwner})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	// Try to leave as last owner
	rr := doJSON(t, env.handler, "DELETE", "/api/squads/"+created.ID+"/members/me", nil, []*http.Cookie{cookieOwner})
	if rr.Code != http.StatusConflict {
		t.Fatalf("last owner leave: status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
}

func TestLeaveSquad_NonOwnerSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	// Owner creates squad
	registerUser(t, env, "owner-leave@example.com", "OwnerLeave", strongPassword())
	loginOwner, _ := loginUser(t, env, "owner-leave@example.com", strongPassword())
	cookieOwner := sessionCookie(loginOwner)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Leave Success", "issuePrefix": "LVSC",
		"captainName": "Captain", "captainShortName": "captain-lvsc",
	}, []*http.Cookie{cookieOwner})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	// Add member
	regMember := registerUser(t, env, "leaver@example.com", "Leaver", strongPassword())
	var memberUser userResp
	json.NewDecoder(regMember.Body).Decode(&memberUser)

	doJSON(t, env.handler, "POST", "/api/squads/"+created.ID+"/members", map[string]any{
		"userId": memberUser.ID, "role": "viewer",
	}, []*http.Cookie{cookieOwner})

	// Member leaves
	loginMember, _ := loginUser(t, env, "leaver@example.com", strongPassword())
	cookieMember := sessionCookie(loginMember)

	rr := doJSON(t, env.handler, "DELETE", "/api/squads/"+created.ID+"/members/me", nil, []*http.Cookie{cookieMember})
	if rr.Code != http.StatusOK {
		t.Fatalf("member leave: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}

func TestListMembers_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "owner-list@example.com", "OwnerList", strongPassword())
	loginOwner, _ := loginUser(t, env, "owner-list@example.com", strongPassword())
	cookieOwner := sessionCookie(loginOwner)

	createRR := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "List Members", "issuePrefix": "LSTM",
		"captainName": "Captain", "captainShortName": "captain-lstm",
	}, []*http.Cookie{cookieOwner})
	var created squadResp
	json.NewDecoder(createRR.Body).Decode(&created)

	rr := doJSON(t, env.handler, "GET", "/api/squads/"+created.ID+"/members", nil, []*http.Cookie{cookieOwner})
	if rr.Code != http.StatusOK {
		t.Fatalf("list members: status = %d; body: %s", rr.Code, rr.Body.String())
	}

	var members []memberResp
	json.NewDecoder(rr.Body).Decode(&members)
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].Role != "owner" {
		t.Errorf("role = %q, want owner", members[0].Role)
	}
}
