package domain

import (
	"strings"
	"testing"
)

func TestSquadStatus_Valid(t *testing.T) {
	tests := []struct {
		status SquadStatus
		want   bool
	}{
		{SquadStatusActive, true},
		{SquadStatusPaused, true},
		{SquadStatusArchived, true},
		{"invalid", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := tc.status.Valid(); got != tc.want {
			t.Errorf("SquadStatus(%q).Valid() = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestSquadStatus_ValidTransition(t *testing.T) {
	tests := []struct {
		from, to SquadStatus
		want     bool
	}{
		{SquadStatusActive, SquadStatusPaused, true},
		{SquadStatusActive, SquadStatusArchived, true},
		{SquadStatusActive, SquadStatusActive, true},
		{SquadStatusPaused, SquadStatusActive, true},
		{SquadStatusPaused, SquadStatusArchived, true},
		{SquadStatusPaused, SquadStatusPaused, true},
		{SquadStatusArchived, SquadStatusActive, false},
		{SquadStatusArchived, SquadStatusPaused, false},
		{SquadStatusArchived, SquadStatusArchived, true},
	}
	for _, tc := range tests {
		if got := tc.from.ValidTransition(tc.to); got != tc.want {
			t.Errorf("%s -> %s: got %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestMemberRole_Valid(t *testing.T) {
	tests := []struct {
		role MemberRole
		want bool
	}{
		{MemberRoleOwner, true},
		{MemberRoleAdmin, true},
		{MemberRoleViewer, true},
		{"invalid", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := tc.role.Valid(); got != tc.want {
			t.Errorf("MemberRole(%q).Valid() = %v, want %v", tc.role, got, tc.want)
		}
	}
}

func TestMemberRole_CanGrantRole(t *testing.T) {
	tests := []struct {
		actor  MemberRole
		target MemberRole
		want   bool
	}{
		{MemberRoleOwner, MemberRoleOwner, true},
		{MemberRoleOwner, MemberRoleAdmin, true},
		{MemberRoleOwner, MemberRoleViewer, true},
		{MemberRoleAdmin, MemberRoleOwner, false},
		{MemberRoleAdmin, MemberRoleAdmin, false},
		{MemberRoleAdmin, MemberRoleViewer, true},
		{MemberRoleViewer, MemberRoleOwner, false},
		{MemberRoleViewer, MemberRoleViewer, false},
		{MemberRoleViewer, MemberRoleAdmin, false},
	}
	for _, tc := range tests {
		if got := tc.actor.CanGrantRole(tc.target); got != tc.want {
			t.Errorf("%s.CanGrantRole(%s) = %v, want %v", tc.actor, tc.target, got, tc.want)
		}
	}
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Acme Engineering", "acme-engineering"},
		{"My Squad Name!", "my-squad-name"},
		{"  hello  world  ", "hello-world"},
		{"UPPER CASE", "upper-case"},
		{"a", "a-squad"},
		{"test123", "test123"},
		{"a--b", "a-b"},
		{"", "unnamed-squad"},
		{"!!!", "unnamed-squad"},
		{"Ärbeits Gruppe", "ärbeits-gruppe"},
	}
	for _, tc := range tests {
		if got := GenerateSlug(tc.name); got != tc.want {
			t.Errorf("GenerateSlug(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestValidateSquadName(t *testing.T) {
	if err := ValidateSquadName("valid"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := ValidateSquadName(""); err == nil {
		t.Error("expected error for empty name")
	}
	if err := ValidateSquadName("   "); err == nil {
		t.Error("expected error for whitespace-only name")
	}
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateSquadName(string(long)); err == nil {
		t.Error("expected error for name > 100 chars")
	}
}

func TestValidateIssuePrefix(t *testing.T) {
	valid := []string{"AB", "ACME", "PROJ123", "ABCDEFGHIJ"}
	for _, v := range valid {
		if err := ValidateIssuePrefix(v); err != nil {
			t.Errorf("ValidateIssuePrefix(%q) unexpected error: %v", v, err)
		}
	}
	invalid := []string{"a", "ab", "acme", "A", "TOOLONGPREFIXX", "AB-CD"}
	for _, v := range invalid {
		if err := ValidateIssuePrefix(v); err == nil {
			t.Errorf("ValidateIssuePrefix(%q) expected error", v)
		}
	}
}

func TestValidateBudget(t *testing.T) {
	if err := ValidateBudget(nil); err != nil {
		t.Errorf("nil budget should be valid: %v", err)
	}
	pos := int64(100)
	if err := ValidateBudget(&pos); err != nil {
		t.Errorf("positive budget should be valid: %v", err)
	}
	zero := int64(0)
	if err := ValidateBudget(&zero); err == nil {
		t.Error("zero budget should be invalid")
	}
	neg := int64(-1)
	if err := ValidateBudget(&neg); err == nil {
		t.Error("negative budget should be invalid")
	}
}

func TestValidateBrandColor(t *testing.T) {
	tests := []struct {
		color   *string
		wantErr bool
	}{
		{nil, false},
		{strPtr("#1a2b3c"), false},
		{strPtr("#1A2B3C"), false},  // uppercase valid
		{strPtr("#aAbBcC"), false},  // mixed case valid
		{strPtr("not-a-color"), true},
		{strPtr("#abc"), true},      // too short
		{strPtr("#1a2b3cff"), true}, // too long
		{strPtr("1a2b3c"), true},    // missing #
	}
	for _, tc := range tests {
		err := ValidateBrandColor(tc.color)
		if (err != nil) != tc.wantErr {
			label := "nil"
			if tc.color != nil {
				label = *tc.color
			}
			t.Errorf("ValidateBrandColor(%q) error = %v, wantErr %v", label, err, tc.wantErr)
		}
	}
}

func strPtr(s string) *string { return &s }

func TestSquadSettings_Merge(t *testing.T) {
	f, tr := false, true
	s := SquadSettings{RequireApprovalForNewAgents: &f}
	s.Merge(SquadSettings{RequireApprovalForNewAgents: &tr})
	if s.RequireApprovalForNewAgents == nil || !*s.RequireApprovalForNewAgents {
		t.Error("expected RequireApprovalForNewAgents to be true after merge")
	}

	// Nil patch should not change value
	s.Merge(SquadSettings{})
	if s.RequireApprovalForNewAgents == nil || !*s.RequireApprovalForNewAgents {
		t.Error("nil patch should preserve existing value")
	}
}

func TestValidateSettingsKeys(t *testing.T) {
	valid := map[string]any{"requireApprovalForNewAgents": true}
	if err := ValidateSettingsKeys(valid); err != nil {
		t.Errorf("valid keys should pass: %v", err)
	}
	invalid := map[string]any{"unknownKey": true}
	if err := ValidateSettingsKeys(invalid); err == nil {
		t.Error("unknown key should fail")
	}
}

func TestValidateSquadSlug(t *testing.T) {
	tests := []struct {
		slug    string
		wantErr bool
	}{
		{"my-squad", false},
		{"ab", false},
		{strings.Repeat("a", 50), false},
		{"a", true},                      // too short
		{strings.Repeat("a", 51), true},  // too long
		{"-squad", true},                 // leading hyphen
		{"squad-", true},                 // trailing hyphen
		{"My-Squad", true},               // uppercase
		{"a1", false},                    // 2-char valid
		{"hello-world-123", false},
	}
	for _, tc := range tests {
		err := ValidateSquadSlug(tc.slug)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateSquadSlug(%q) error = %v, wantErr %v", tc.slug, err, tc.wantErr)
		}
	}
}

func TestFormatIssueIdentifier(t *testing.T) {
	tests := []struct {
		prefix  string
		counter int64
		want    string
	}{
		{"ARI", 1, "ARI-1"},
		{"ACME", 42, "ACME-42"},
	}
	for _, tc := range tests {
		if got := FormatIssueIdentifier(tc.prefix, tc.counter); got != tc.want {
			t.Errorf("FormatIssueIdentifier(%q, %d) = %q, want %q", tc.prefix, tc.counter, got, tc.want)
		}
	}
}
