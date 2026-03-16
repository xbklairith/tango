package auth

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// --- Permission Matrix Tests ---

func TestUserPermissions_OwnerHasAllActions(t *testing.T) {
	ownerPerms, ok := UserPermissions["owner"]
	if !ok {
		t.Fatal("owner role not found in UserPermissions")
	}
	for _, res := range AllResources {
		acts, ok := ownerPerms[res]
		if !ok {
			// activity and cost only have read for some roles, but owner should have all
			if res == ResourceActivity {
				continue // activity only has read
			}
			t.Errorf("owner missing resource %s", res)
			continue
		}
		for _, act := range AllActions {
			if res == ResourceActivity && act != ActionRead {
				continue // activity only has read
			}
			if !acts[act] {
				t.Errorf("owner should have %s.%s", res, act)
			}
		}
	}
}

func TestUserPermissions_AdminHasAllExceptSquadDelete(t *testing.T) {
	adminPerms, ok := UserPermissions["admin"]
	if !ok {
		t.Fatal("admin role not found in UserPermissions")
	}

	// admin should NOT have squad.delete
	if adminPerms[ResourceSquad][ActionDelete] {
		t.Error("admin should NOT have squad.delete")
	}

	// admin should have squad.create, squad.read, squad.update
	for _, act := range []Action{ActionCreate, ActionRead, ActionUpdate} {
		if !adminPerms[ResourceSquad][act] {
			t.Errorf("admin should have squad.%s", act)
		}
	}

	// admin should have all actions on non-squad, non-activity resources
	for _, res := range AllResources {
		if res == ResourceSquad || res == ResourceActivity {
			continue
		}
		acts, ok := adminPerms[res]
		if !ok {
			t.Errorf("admin missing resource %s", res)
			continue
		}
		for _, act := range AllActions {
			if !acts[act] {
				t.Errorf("admin should have %s.%s", res, act)
			}
		}
	}
}

func TestUserPermissions_ViewerReadOnly(t *testing.T) {
	viewerPerms, ok := UserPermissions["viewer"]
	if !ok {
		t.Fatal("viewer role not found in UserPermissions")
	}
	for _, res := range AllResources {
		acts, ok := viewerPerms[res]
		if !ok {
			continue
		}
		// viewer should have read
		if !acts[ActionRead] {
			t.Errorf("viewer should have %s.read", res)
		}
		// viewer should NOT have write actions
		for _, act := range []Action{ActionCreate, ActionUpdate, ActionDelete, ActionAssign, ActionAdvance, ActionReject, ActionResolve} {
			if acts[act] {
				t.Errorf("viewer should NOT have %s.%s", res, act)
			}
		}
	}
}

func TestAgentPermissions_CaptainMatrix(t *testing.T) {
	captainPerms, ok := AgentPermissions["captain"]
	if !ok {
		t.Fatal("captain role not found in AgentPermissions")
	}

	// captain should have issue create/read/update/assign/advance/reject/resolve
	for _, act := range []Action{ActionCreate, ActionRead, ActionUpdate, ActionAssign, ActionAdvance, ActionReject, ActionResolve} {
		if !captainPerms[ResourceIssue][act] {
			t.Errorf("captain should have issue.%s", act)
		}
	}

	// captain should have inbox.create, inbox.read, inbox.resolve
	for _, act := range []Action{ActionCreate, ActionRead, ActionResolve} {
		if !captainPerms[ResourceInbox][act] {
			t.Errorf("captain should have inbox.%s", act)
		}
	}

	// captain should have full agent management (CEO role)
	for _, act := range []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionAssign} {
		if !captainPerms[ResourceAgent][act] {
			t.Errorf("captain should have agent.%s", act)
		}
	}
}

func TestAgentPermissions_LeadMatrix(t *testing.T) {
	leadPerms, ok := AgentPermissions["lead"]
	if !ok {
		t.Fatal("lead role not found in AgentPermissions")
	}

	// lead should have issue create/read/update/assign/advance/reject/resolve
	for _, act := range []Action{ActionCreate, ActionRead, ActionUpdate, ActionAssign, ActionAdvance, ActionReject, ActionResolve} {
		if !leadPerms[ResourceIssue][act] {
			t.Errorf("lead should have issue.%s", act)
		}
	}

	// lead should NOT have inbox.resolve
	if leadPerms[ResourceInbox][ActionResolve] {
		t.Error("lead should NOT have inbox.resolve")
	}
}

func TestAgentPermissions_MemberMatrix(t *testing.T) {
	memberPerms, ok := AgentPermissions["member"]
	if !ok {
		t.Fatal("member role not found in AgentPermissions")
	}

	// member should have issue.read and issue.update only
	if !memberPerms[ResourceIssue][ActionRead] {
		t.Error("member should have issue.read")
	}
	if !memberPerms[ResourceIssue][ActionUpdate] {
		t.Error("member should have issue.update")
	}
	if memberPerms[ResourceIssue][ActionCreate] {
		t.Error("member should NOT have issue.create")
	}
	if memberPerms[ResourceIssue][ActionAssign] {
		t.Error("member should NOT have issue.assign")
	}
	if memberPerms[ResourceIssue][ActionAdvance] {
		t.Error("member should NOT have issue.advance")
	}

	// member should have inbox.create and inbox.read
	if !memberPerms[ResourceInbox][ActionCreate] {
		t.Error("member should have inbox.create")
	}
	if !memberPerms[ResourceInbox][ActionRead] {
		t.Error("member should have inbox.read")
	}
	// member should NOT have inbox.resolve
	if memberPerms[ResourceInbox][ActionResolve] {
		t.Error("member should NOT have inbox.resolve")
	}

	// member should NOT have wakeup.create
	if memberPerms[ResourceWakeup] != nil && memberPerms[ResourceWakeup][ActionCreate] {
		t.Error("member should NOT have wakeup.create")
	}
}

func TestPermissionMatrix_DefaultDeny(t *testing.T) {
	// Unknown role
	err := checkUserPermission("unknown_role", ResourceIssue, ActionCreate)
	if !IsPermissionDenied(err) {
		t.Error("unknown role should be denied")
	}

	// Unknown resource on valid role
	err = checkUserPermission("owner", Resource("bogus"), ActionRead)
	if !IsPermissionDenied(err) {
		t.Error("unknown resource should be denied")
	}

	// Unknown action on valid role+resource
	err = checkAgentPermission("member", ResourceIssue, Action("explode"))
	if !IsPermissionDenied(err) {
		t.Error("unknown action should be denied")
	}
}

func TestHelpers_AllActions(t *testing.T) {
	aa := allActions()
	if len(aa) != 8 {
		t.Errorf("allActions() should return 8 actions, got %d", len(aa))
	}
	for _, act := range AllActions {
		if !aa[act] {
			t.Errorf("allActions() missing %s", act)
		}
	}
}

func TestHelpers_ReadOnly(t *testing.T) {
	ro := readOnly()
	if len(ro) != 1 {
		t.Errorf("readOnly() should return 1 action, got %d", len(ro))
	}
	if !ro[ActionRead] {
		t.Error("readOnly() should include ActionRead")
	}
}

func TestHelpers_Actions(t *testing.T) {
	a := actions(ActionCreate, ActionRead)
	if len(a) != 2 {
		t.Errorf("actions() should return 2 actions, got %d", len(a))
	}
	if !a[ActionCreate] || !a[ActionRead] {
		t.Error("actions() missing expected action")
	}
	if a[ActionDelete] {
		t.Error("actions() should not include ActionDelete")
	}
}

// --- RequirePermission Tests ---

func TestRequirePermission_UserOwner_Allowed(t *testing.T) {
	ctx := withUser(context.Background(), Identity{UserID: uuid.New(), Email: "owner@test.com"})
	roleLookup := func(_ context.Context, _, _ uuid.UUID) (string, error) { return "owner", nil }

	err := RequirePermission(ctx, uuid.New(), ResourceIssue, ActionCreate, roleLookup)
	if err != nil {
		t.Errorf("owner should be allowed: %v", err)
	}
}

func TestRequirePermission_UserAdmin_AllowedExceptSquadDelete(t *testing.T) {
	ctx := withUser(context.Background(), Identity{UserID: uuid.New(), Email: "admin@test.com"})
	roleLookup := func(_ context.Context, _, _ uuid.UUID) (string, error) { return "admin", nil }

	// admin should be allowed on most things
	err := RequirePermission(ctx, uuid.New(), ResourceIssue, ActionCreate, roleLookup)
	if err != nil {
		t.Errorf("admin should be allowed to create issues: %v", err)
	}

	// admin should be denied squad.delete
	err = RequirePermission(ctx, uuid.New(), ResourceSquad, ActionDelete, roleLookup)
	if !IsPermissionDenied(err) {
		t.Error("admin should be denied squad.delete")
	}
}

func TestRequirePermission_UserViewer_ReadOnly(t *testing.T) {
	ctx := withUser(context.Background(), Identity{UserID: uuid.New(), Email: "viewer@test.com"})
	roleLookup := func(_ context.Context, _, _ uuid.UUID) (string, error) { return "viewer", nil }

	err := RequirePermission(ctx, uuid.New(), ResourceIssue, ActionRead, roleLookup)
	if err != nil {
		t.Errorf("viewer should be allowed to read: %v", err)
	}

	err = RequirePermission(ctx, uuid.New(), ResourceIssue, ActionCreate, roleLookup)
	if !IsPermissionDenied(err) {
		t.Error("viewer should be denied issue.create")
	}
}

func TestRequirePermission_AgentCaptain_Allowed(t *testing.T) {
	ctx := WithAgent(context.Background(), AgentIdentity{
		AgentID: uuid.New(),
		SquadID: uuid.New(),
		RunID:   uuid.New(),
		Role:    "captain",
	})

	err := RequirePermission(ctx, uuid.New(), ResourceIssue, ActionCreate, nil)
	if err != nil {
		t.Errorf("captain agent should be allowed to create issues: %v", err)
	}
}

func TestRequirePermission_AgentMember_LimitedAccess(t *testing.T) {
	ctx := WithAgent(context.Background(), AgentIdentity{
		AgentID: uuid.New(),
		SquadID: uuid.New(),
		RunID:   uuid.New(),
		Role:    "member",
	})

	// member should NOT be able to create issues
	err := RequirePermission(ctx, uuid.New(), ResourceIssue, ActionCreate, nil)
	if !IsPermissionDenied(err) {
		t.Error("member agent should be denied issue.create")
	}

	// member should be able to read
	err = RequirePermission(ctx, uuid.New(), ResourceIssue, ActionRead, nil)
	if err != nil {
		t.Errorf("member agent should be allowed issue.read: %v", err)
	}

	// member should NOT be able to advance
	err = RequirePermission(ctx, uuid.New(), ResourceIssue, ActionAdvance, nil)
	if !IsPermissionDenied(err) {
		t.Error("member agent should be denied issue.advance")
	}
}

func TestRequirePermission_LocalOperator_TreatedAsOwner(t *testing.T) {
	ctx := withUser(context.Background(), LocalOperatorIdentity)

	// Should always be allowed — no roleLookup needed
	err := RequirePermission(ctx, uuid.New(), ResourceSquad, ActionDelete, nil)
	if err != nil {
		t.Errorf("local operator should be allowed: %v", err)
	}
}

func TestRequirePermission_NoIdentity_AuthError(t *testing.T) {
	err := RequirePermission(context.Background(), uuid.New(), ResourceIssue, ActionRead, nil)
	if err == nil {
		t.Error("empty context should return error")
	}
	if IsPermissionDenied(err) {
		t.Error("empty context should return auth error, not permission denied")
	}
}

func TestRequirePermission_UserNotInSquad_Error(t *testing.T) {
	ctx := withUser(context.Background(), Identity{UserID: uuid.New(), Email: "test@test.com"})
	roleLookup := func(_ context.Context, _, _ uuid.UUID) (string, error) {
		return "", errors.New("not found")
	}

	err := RequirePermission(ctx, uuid.New(), ResourceIssue, ActionRead, roleLookup)
	if err == nil {
		t.Error("should return error when user not in squad")
	}
}

func TestRequirePermission_AgentTakesPrecedence(t *testing.T) {
	// Context with both user and agent — agent should take precedence
	ctx := withUser(context.Background(), Identity{UserID: uuid.New(), Email: "test@test.com"})
	ctx = WithAgent(ctx, AgentIdentity{
		AgentID: uuid.New(),
		SquadID: uuid.New(),
		RunID:   uuid.New(),
		Role:    "member",
	})

	// member agent cannot create issues
	err := RequirePermission(ctx, uuid.New(), ResourceIssue, ActionCreate, nil)
	if !IsPermissionDenied(err) {
		t.Error("agent identity should take precedence; member cannot create issues")
	}
}

func TestPermissionDeniedError_Message(t *testing.T) {
	err := &PermissionDeniedError{Resource: ResourceIssue, Action: ActionCreate, Role: "viewer"}
	expected := "Permission denied: issue.create"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestIsPermissionDenied_True(t *testing.T) {
	err := &PermissionDeniedError{Resource: ResourceIssue, Action: ActionCreate}
	if !IsPermissionDenied(err) {
		t.Error("should return true for PermissionDeniedError")
	}
}

func TestIsPermissionDenied_False(t *testing.T) {
	if IsPermissionDenied(errors.New("some other error")) {
		t.Error("should return false for non-PermissionDeniedError")
	}
	if IsPermissionDenied(nil) {
		t.Error("should return false for nil")
	}
}

func TestRequirePermission_ConcurrentSafe(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := WithAgent(context.Background(), AgentIdentity{
				AgentID: uuid.New(),
				SquadID: uuid.New(),
				RunID:   uuid.New(),
				Role:    "captain",
			})
			_ = RequirePermission(ctx, uuid.New(), ResourceIssue, ActionCreate, nil)
		}()
	}
	wg.Wait()
}

func TestRequirePermissionWithRole_User(t *testing.T) {
	err := RequirePermissionWithRole("owner", ResourceSquad, ActionDelete, false)
	if err != nil {
		t.Errorf("owner should be allowed squad.delete: %v", err)
	}

	err = RequirePermissionWithRole("admin", ResourceSquad, ActionDelete, false)
	if !IsPermissionDenied(err) {
		t.Error("admin should be denied squad.delete")
	}
}

func TestRequirePermissionWithRole_Agent(t *testing.T) {
	err := RequirePermissionWithRole("captain", ResourceIssue, ActionCreate, true)
	if err != nil {
		t.Errorf("captain should be allowed issue.create: %v", err)
	}

	err = RequirePermissionWithRole("member", ResourceIssue, ActionCreate, true)
	if !IsPermissionDenied(err) {
		t.Error("member agent should be denied issue.create")
	}
}

func TestRequirePermission_NilRoleLookupForUser(t *testing.T) {
	ctx := withUser(context.Background(), Identity{UserID: uuid.New(), Email: "test@test.com"})

	err := RequirePermission(ctx, uuid.New(), ResourceIssue, ActionRead, nil)
	if err == nil {
		t.Error("should return error when roleLookup is nil for non-local user")
	}
}
