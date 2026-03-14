package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateStatusTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    AgentStatus
		to      AgentStatus
		wantErr bool
	}{
		// Valid transitions
		{"pending_approval -> active", AgentStatusPendingApproval, AgentStatusActive, false},
		{"active -> idle", AgentStatusActive, AgentStatusIdle, false},
		{"idle -> running", AgentStatusIdle, AgentStatusRunning, false},
		{"running -> idle", AgentStatusRunning, AgentStatusIdle, false},
		{"running -> error", AgentStatusRunning, AgentStatusError, false},
		{"active -> paused", AgentStatusActive, AgentStatusPaused, false},
		{"idle -> paused", AgentStatusIdle, AgentStatusPaused, false},
		{"running -> paused", AgentStatusRunning, AgentStatusPaused, false},
		{"paused -> active", AgentStatusPaused, AgentStatusActive, false},

		// Any -> terminated
		{"pending_approval -> terminated", AgentStatusPendingApproval, AgentStatusTerminated, false},
		{"active -> terminated", AgentStatusActive, AgentStatusTerminated, false},
		{"idle -> terminated", AgentStatusIdle, AgentStatusTerminated, false},
		{"running -> terminated", AgentStatusRunning, AgentStatusTerminated, false},
		{"error -> terminated", AgentStatusError, AgentStatusTerminated, false},
		{"paused -> terminated", AgentStatusPaused, AgentStatusTerminated, false},

		// Invalid transitions
		{"terminated -> active", AgentStatusTerminated, AgentStatusActive, true},
		{"terminated -> terminated", AgentStatusTerminated, AgentStatusTerminated, true},
		{"pending_approval -> idle", AgentStatusPendingApproval, AgentStatusIdle, true},
		{"pending_approval -> running", AgentStatusPendingApproval, AgentStatusRunning, true},
		{"pending_approval -> paused", AgentStatusPendingApproval, AgentStatusPaused, true},
		{"active -> running", AgentStatusActive, AgentStatusRunning, true},
		{"active -> error", AgentStatusActive, AgentStatusError, true},
		{"idle -> active", AgentStatusIdle, AgentStatusActive, true},
		{"idle -> error", AgentStatusIdle, AgentStatusError, true},
		{"running -> active", AgentStatusRunning, AgentStatusActive, true},
		{"error -> active", AgentStatusError, AgentStatusActive, true},
		{"error -> idle", AgentStatusError, AgentStatusIdle, true},
		{"error -> running", AgentStatusError, AgentStatusRunning, true},
		{"error -> paused", AgentStatusError, AgentStatusPaused, true},
		{"paused -> idle", AgentStatusPaused, AgentStatusIdle, true},
		{"paused -> running", AgentStatusPaused, AgentStatusRunning, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStatusTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStatusTransition(%q, %q) error = %v, wantErr %v",
					tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

func TestValidateHierarchy(t *testing.T) {
	squadID := uuid.New()
	otherSquadID := uuid.New()
	captainID := uuid.New()
	leadID := uuid.New()
	captainRole := AgentRoleCaptain
	leadRole := AgentRoleLead
	memberRole := AgentRoleMember

	tests := []struct {
		name    string
		ctx     HierarchyContext
		wantErr bool
	}{
		{
			name: "captain with no parent, no existing captain",
			ctx: HierarchyContext{
				Role: AgentRoleCaptain, ParentAgentID: nil,
				SquadID: squadID, ExistingCaptainID: nil,
			},
			wantErr: false,
		},
		{
			name: "captain with parent -- rejected",
			ctx: HierarchyContext{
				Role: AgentRoleCaptain, ParentAgentID: &captainID, SquadID: squadID,
			},
			wantErr: true,
		},
		{
			name: "second captain -- rejected",
			ctx: HierarchyContext{
				Role: AgentRoleCaptain, ParentAgentID: nil,
				SquadID: squadID, ExistingCaptainID: &captainID, AgentID: nil,
			},
			wantErr: true,
		},
		{
			name: "existing captain updating self -- allowed",
			ctx: HierarchyContext{
				Role: AgentRoleCaptain, ParentAgentID: nil,
				SquadID: squadID, ExistingCaptainID: &captainID, AgentID: &captainID,
			},
			wantErr: false,
		},
		{
			name: "lead with captain parent same squad",
			ctx: HierarchyContext{
				Role: AgentRoleLead, ParentAgentID: &captainID,
				ParentRole: &captainRole, ParentSquadID: &squadID, SquadID: squadID,
			},
			wantErr: false,
		},
		{
			name: "lead with no parent -- rejected",
			ctx:     HierarchyContext{Role: AgentRoleLead, ParentAgentID: nil, SquadID: squadID},
			wantErr: true,
		},
		{
			name: "lead with member parent -- rejected",
			ctx: HierarchyContext{
				Role: AgentRoleLead, ParentAgentID: &leadID,
				ParentRole: &memberRole, ParentSquadID: &squadID, SquadID: squadID,
			},
			wantErr: true,
		},
		{
			name: "lead with captain different squad -- rejected",
			ctx: HierarchyContext{
				Role: AgentRoleLead, ParentAgentID: &captainID,
				ParentRole: &captainRole, ParentSquadID: &otherSquadID, SquadID: squadID,
			},
			wantErr: true,
		},
		{
			name: "member with lead parent same squad",
			ctx: HierarchyContext{
				Role: AgentRoleMember, ParentAgentID: &leadID,
				ParentRole: &leadRole, ParentSquadID: &squadID, SquadID: squadID,
			},
			wantErr: false,
		},
		{
			name: "member with captain parent -- rejected",
			ctx: HierarchyContext{
				Role: AgentRoleMember, ParentAgentID: &captainID,
				ParentRole: &captainRole, ParentSquadID: &squadID, SquadID: squadID,
			},
			wantErr: true,
		},
		{
			name: "member with no parent -- rejected",
			ctx:     HierarchyContext{Role: AgentRoleMember, ParentAgentID: nil, SquadID: squadID},
			wantErr: true,
		},
		{
			name: "member with lead different squad -- rejected",
			ctx: HierarchyContext{
				Role: AgentRoleMember, ParentAgentID: &leadID,
				ParentRole: &leadRole, ParentSquadID: &otherSquadID, SquadID: squadID,
			},
			wantErr: true,
		},
		{
			name: "invalid role -- rejected",
			ctx:     HierarchyContext{Role: "boss", SquadID: squadID},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHierarchy(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHierarchy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateHierarchyChange(t *testing.T) {
	squadID := uuid.New()
	captainID := uuid.New()
	leadID := uuid.New()

	captain := Agent{ID: captainID, SquadID: squadID, Role: AgentRoleCaptain}
	lead := Agent{ID: leadID, SquadID: squadID, Role: AgentRoleLead}

	memberRole := AgentRoleMember
	captainRole := AgentRoleCaptain

	t.Run("role change with children -- rejected", func(t *testing.T) {
		err := ValidateHierarchyChange(captain, &memberRole, nil, nil, nil, 3)
		if err == nil {
			t.Error("expected error for role change with children")
		}
	})

	t.Run("role change without children -- checks hierarchy", func(t *testing.T) {
		parentInfo := &AgentParentInfo{Role: AgentRoleCaptain, SquadID: squadID}
		err := ValidateHierarchyChange(lead, &memberRole, &captainID, parentInfo, nil, 0)
		// Should fail because member needs lead parent, not captain
		if err == nil {
			t.Error("expected hierarchy error")
		}
	})

	t.Run("role change to captain when captain exists -- rejected", func(t *testing.T) {
		err := ValidateHierarchyChange(lead, &captainRole, nil, nil, &captainID, 0)
		if err == nil {
			t.Error("expected error for second captain")
		}
	})

	t.Run("valid lead with no role change", func(t *testing.T) {
		parentInfo := &AgentParentInfo{Role: AgentRoleCaptain, SquadID: squadID}
		err := ValidateHierarchyChange(lead, nil, &captainID, parentInfo, nil, 0)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestValidateCreateAgentInput(t *testing.T) {
	valid := CreateAgentRequest{
		SquadID:   uuid.New(),
		Name:      "Test Agent",
		ShortName: "test-agent",
		Role:      AgentRoleCaptain,
	}

	t.Run("valid input", func(t *testing.T) {
		if err := ValidateCreateAgentInput(valid); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		r := valid
		r.Name = ""
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("name too long", func(t *testing.T) {
		r := valid
		r.Name = strings.Repeat("a", 256)
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for long name")
		}
	})

	t.Run("empty shortName", func(t *testing.T) {
		r := valid
		r.ShortName = ""
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for empty shortName")
		}
	})

	t.Run("shortName too long", func(t *testing.T) {
		r := valid
		r.ShortName = strings.Repeat("a", 51)
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for long shortName")
		}
	})

	t.Run("shortName with uppercase", func(t *testing.T) {
		r := valid
		r.ShortName = "Test-Agent"
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for uppercase shortName")
		}
	})

	t.Run("shortName with spaces", func(t *testing.T) {
		r := valid
		r.ShortName = "test agent"
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for shortName with spaces")
		}
	})

	t.Run("invalid role", func(t *testing.T) {
		r := valid
		r.Role = "boss"
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for invalid role")
		}
	})

	t.Run("negative budget", func(t *testing.T) {
		r := valid
		neg := int64(-1)
		r.BudgetMonthlyCents = &neg
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for negative budget")
		}
	})

	t.Run("zero budget valid", func(t *testing.T) {
		r := valid
		zero := int64(0)
		r.BudgetMonthlyCents = &zero
		if err := ValidateCreateAgentInput(r); err != nil {
			t.Errorf("unexpected error for zero budget: %v", err)
		}
	})

	t.Run("invalid adapterConfig", func(t *testing.T) {
		r := valid
		r.AdapterConfig = json.RawMessage(`{invalid}`)
		if err := ValidateCreateAgentInput(r); err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("valid adapterConfig", func(t *testing.T) {
		r := valid
		r.AdapterConfig = json.RawMessage(`{"key":"value"}`)
		if err := ValidateCreateAgentInput(r); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestValidateUpdateAgentInput(t *testing.T) {
	t.Run("empty update is valid", func(t *testing.T) {
		if err := ValidateUpdateAgentInput(UpdateAgentRequest{}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		empty := ""
		if err := ValidateUpdateAgentInput(UpdateAgentRequest{Name: &empty}); err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("name too long", func(t *testing.T) {
		long := strings.Repeat("a", 256)
		if err := ValidateUpdateAgentInput(UpdateAgentRequest{Name: &long}); err == nil {
			t.Error("expected error for long name")
		}
	})

	t.Run("invalid shortName", func(t *testing.T) {
		bad := "Has Spaces"
		if err := ValidateUpdateAgentInput(UpdateAgentRequest{ShortName: &bad}); err == nil {
			t.Error("expected error for invalid shortName")
		}
	})

	t.Run("invalid role", func(t *testing.T) {
		bad := AgentRole("invalid")
		if err := ValidateUpdateAgentInput(UpdateAgentRequest{Role: &bad}); err == nil {
			t.Error("expected error for invalid role")
		}
	})

	t.Run("negative budget", func(t *testing.T) {
		neg := int64(-5)
		if err := ValidateUpdateAgentInput(UpdateAgentRequest{BudgetMonthlyCents: &neg}); err == nil {
			t.Error("expected error for negative budget")
		}
	})
}

func TestAgentRolesMap(t *testing.T) {
	if !ValidAgentRoles[AgentRoleCaptain] {
		t.Error("captain should be valid")
	}
	if !ValidAgentRoles[AgentRoleLead] {
		t.Error("lead should be valid")
	}
	if !ValidAgentRoles[AgentRoleMember] {
		t.Error("member should be valid")
	}
	if ValidAgentRoles["boss"] {
		t.Error("boss should not be valid")
	}
}
