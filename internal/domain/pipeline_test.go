package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateCreatePipelineInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreatePipelineRequest
		wantErr bool
	}{
		{
			name:    "valid input",
			input:   CreatePipelineRequest{Name: "My Pipeline"},
			wantErr: false,
		},
		{
			name:    "valid with description",
			input:   CreatePipelineRequest{Name: "My Pipeline", Description: strPtr("A good pipeline")},
			wantErr: false,
		},
		{
			name:    "empty name",
			input:   CreatePipelineRequest{Name: ""},
			wantErr: true,
		},
		{
			name:    "name too long",
			input:   CreatePipelineRequest{Name: string(make([]byte, 201))},
			wantErr: true,
		},
		{
			name:    "name at max length",
			input:   CreatePipelineRequest{Name: string(make([]byte, 200))},
			wantErr: false,
		},
		{
			name:    "description too long",
			input:   CreatePipelineRequest{Name: "ok", Description: strPtr(string(make([]byte, 2001)))},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreatePipelineInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreatePipelineInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUpdatePipelineInput(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdatePipelineRequest
		wantErr bool
	}{
		{
			name:    "empty update (no-op)",
			input:   UpdatePipelineRequest{},
			wantErr: false,
		},
		{
			name:    "valid name update",
			input:   UpdatePipelineRequest{Name: strPtr("New Name")},
			wantErr: false,
		},
		{
			name:    "empty name rejected",
			input:   UpdatePipelineRequest{Name: strPtr("")},
			wantErr: true,
		},
		{
			name:    "name too long",
			input:   UpdatePipelineRequest{Name: strPtr(string(make([]byte, 201)))},
			wantErr: true,
		},
		{
			name:    "valid isActive update",
			input:   UpdatePipelineRequest{IsActive: boolPtr(false)},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdatePipelineInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdatePipelineInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCreateStageInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateStageRequest
		wantErr bool
	}{
		{
			name:    "valid input",
			input:   CreateStageRequest{Name: "Implementation", Position: 1},
			wantErr: false,
		},
		{
			name:    "valid with agent",
			input:   CreateStageRequest{Name: "Review", Position: 2, AssignedAgentID: uuidPtr(uuid.New())},
			wantErr: false,
		},
		{
			name:    "empty name",
			input:   CreateStageRequest{Name: "", Position: 1},
			wantErr: true,
		},
		{
			name:    "name too long",
			input:   CreateStageRequest{Name: string(make([]byte, 201)), Position: 1},
			wantErr: true,
		},
		{
			name:    "position zero",
			input:   CreateStageRequest{Name: "Stage", Position: 0},
			wantErr: true,
		},
		{
			name:    "negative position",
			input:   CreateStageRequest{Name: "Stage", Position: -1},
			wantErr: true,
		},
		{
			name:    "position 1 valid",
			input:   CreateStageRequest{Name: "Stage", Position: 1},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateStageInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateStageInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUpdateStageInput(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdateStageRequest
		wantErr bool
	}{
		{
			name:    "empty update (no-op)",
			input:   UpdateStageRequest{},
			wantErr: false,
		},
		{
			name:    "valid name update",
			input:   UpdateStageRequest{Name: strPtr("New Stage")},
			wantErr: false,
		},
		{
			name:    "empty name rejected",
			input:   UpdateStageRequest{Name: strPtr("")},
			wantErr: true,
		},
		{
			name:    "name too long",
			input:   UpdateStageRequest{Name: strPtr(string(make([]byte, 201)))},
			wantErr: true,
		},
		{
			name:    "valid position update",
			input:   UpdateStageRequest{Position: intPtr(3)},
			wantErr: false,
		},
		{
			name:    "position zero rejected",
			input:   UpdateStageRequest{Position: intPtr(0)},
			wantErr: true,
		},
		{
			name:    "negative position rejected",
			input:   UpdateStageRequest{Position: intPtr(-1)},
			wantErr: true,
		},
		{
			name:    "nil fields accepted",
			input:   UpdateStageRequest{Name: nil, Position: nil, AssignedAgentID: nil},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateStageInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdateStageInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// helpers — boolPtr and uuidPtr are unique to this file;
// strPtr and intPtr are already declared in squad_test.go / issue_test.go.

func boolPtr(b bool) *bool              { return &b }
func uuidPtr(u uuid.UUID) *uuid.UUID    { return &u }
