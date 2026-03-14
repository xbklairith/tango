package domain

import "testing"

func TestGoalStatusValid(t *testing.T) {
	tests := []struct {
		s    GoalStatus
		want bool
	}{
		{GoalStatusActive, true},
		{GoalStatusCompleted, true},
		{GoalStatusArchived, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.s.Valid(); got != tt.want {
			t.Errorf("GoalStatus(%q).Valid() = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestValidateGoalTransition(t *testing.T) {
	tests := []struct {
		name    string
		from, to GoalStatus
		wantErr bool
	}{
		{"active->completed", GoalStatusActive, GoalStatusCompleted, false},
		{"active->archived", GoalStatusActive, GoalStatusArchived, false},
		{"completed->active", GoalStatusCompleted, GoalStatusActive, false},
		{"completed->archived", GoalStatusCompleted, GoalStatusArchived, false},
		{"archived->active", GoalStatusArchived, GoalStatusActive, false},
		{"active->active", GoalStatusActive, GoalStatusActive, false},
		{"archived->completed", GoalStatusArchived, GoalStatusCompleted, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGoalTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGoalTransition(%q, %q) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCreateGoalInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateGoalRequest
		wantErr bool
	}{
		{"valid", CreateGoalRequest{Title: "My Goal"}, false},
		{"empty title", CreateGoalRequest{Title: ""}, true},
		{"title too long", CreateGoalRequest{Title: string(make([]byte, 256))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateGoalInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateGoalInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUpdateGoalInput(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdateGoalRequest
		wantErr bool
	}{
		{"empty (noop)", UpdateGoalRequest{}, false},
		{"valid title", UpdateGoalRequest{Title: strPtr("New goal")}, false},
		{"empty title", UpdateGoalRequest{Title: strPtr("")}, true},
		{"title too long", UpdateGoalRequest{Title: strPtr(string(make([]byte, 256)))}, true},
		{"invalid status", UpdateGoalRequest{Status: ptr(GoalStatus("bad"))}, true},
		{"valid status", UpdateGoalRequest{Status: ptr(GoalStatusCompleted)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateGoalInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdateGoalInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
