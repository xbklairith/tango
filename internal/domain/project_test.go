package domain

import "testing"

func TestProjectStatusValid(t *testing.T) {
	tests := []struct {
		s    ProjectStatus
		want bool
	}{
		{ProjectStatusActive, true},
		{ProjectStatusCompleted, true},
		{ProjectStatusArchived, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.s.Valid(); got != tt.want {
			t.Errorf("ProjectStatus(%q).Valid() = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestValidateProjectTransition(t *testing.T) {
	tests := []struct {
		name    string
		from, to ProjectStatus
		wantErr bool
	}{
		// Valid
		{"active->completed", ProjectStatusActive, ProjectStatusCompleted, false},
		{"active->archived", ProjectStatusActive, ProjectStatusArchived, false},
		{"completed->active", ProjectStatusCompleted, ProjectStatusActive, false},
		{"completed->archived", ProjectStatusCompleted, ProjectStatusArchived, false},
		{"archived->active", ProjectStatusArchived, ProjectStatusActive, false},
		// No-op
		{"active->active", ProjectStatusActive, ProjectStatusActive, false},
		// Invalid
		{"archived->completed", ProjectStatusArchived, ProjectStatusCompleted, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProjectTransition(%q, %q) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCreateProjectInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateProjectRequest
		wantErr bool
	}{
		{"valid", CreateProjectRequest{Name: "My Project"}, false},
		{"empty name", CreateProjectRequest{Name: ""}, true},
		{"name too long", CreateProjectRequest{Name: string(make([]byte, 256))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateProjectInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateProjectInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUpdateProjectInput(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdateProjectRequest
		wantErr bool
	}{
		{"empty (noop)", UpdateProjectRequest{}, false},
		{"valid name", UpdateProjectRequest{Name: strPtr("New name")}, false},
		{"empty name", UpdateProjectRequest{Name: strPtr("")}, true},
		{"name too long", UpdateProjectRequest{Name: strPtr(string(make([]byte, 256)))}, true},
		{"invalid status", UpdateProjectRequest{Status: ptr(ProjectStatus("bad"))}, true},
		{"valid status", UpdateProjectRequest{Status: ptr(ProjectStatusCompleted)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateProjectInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdateProjectInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
