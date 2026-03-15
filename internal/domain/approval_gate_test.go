package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateApprovalGate(t *testing.T) {
	validGate := func() *ApprovalGate {
		return &ApprovalGate{
			Name:          "Deploy Gate",
			ActionPattern: "deploy",
			TimeoutHours:  24,
		}
	}

	t.Run("valid gate passes", func(t *testing.T) {
		if err := ValidateApprovalGate(validGate()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		g := validGate()
		g.Name = ""
		if err := ValidateApprovalGate(g); err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("name too long", func(t *testing.T) {
		g := validGate()
		g.Name = string(make([]byte, 101))
		if err := ValidateApprovalGate(g); err == nil {
			t.Error("expected error for name > 100 chars")
		}
	})

	t.Run("missing actionPattern", func(t *testing.T) {
		g := validGate()
		g.ActionPattern = ""
		if err := ValidateApprovalGate(g); err == nil {
			t.Error("expected error for missing actionPattern")
		}
	})

	t.Run("actionPattern too long", func(t *testing.T) {
		g := validGate()
		g.ActionPattern = string(make([]byte, 101))
		if err := ValidateApprovalGate(g); err == nil {
			t.Error("expected error for actionPattern > 100 chars")
		}
	})

	t.Run("timeoutHours too low", func(t *testing.T) {
		g := validGate()
		g.TimeoutHours = 0
		if err := ValidateApprovalGate(g); err == nil {
			t.Error("expected error for timeoutHours < 1")
		}
	})

	t.Run("timeoutHours too high", func(t *testing.T) {
		g := validGate()
		g.TimeoutHours = 169
		if err := ValidateApprovalGate(g); err == nil {
			t.Error("expected error for timeoutHours > 168")
		}
	})

	t.Run("valid autoResolution rejected", func(t *testing.T) {
		g := validGate()
		g.AutoResolution = "rejected"
		if err := ValidateApprovalGate(g); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("valid autoResolution approved", func(t *testing.T) {
		g := validGate()
		g.AutoResolution = "approved"
		if err := ValidateApprovalGate(g); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("invalid autoResolution", func(t *testing.T) {
		g := validGate()
		g.AutoResolution = "invalid"
		if err := ValidateApprovalGate(g); err == nil {
			t.Error("expected error for invalid autoResolution")
		}
	})

	t.Run("negative requiredApprovers", func(t *testing.T) {
		g := validGate()
		g.RequiredApprovers = -1
		if err := ValidateApprovalGate(g); err == nil {
			t.Error("expected error for negative requiredApprovers")
		}
	})
}

func TestNormalizeApprovalGate(t *testing.T) {
	t.Run("fills defaults for empty fields", func(t *testing.T) {
		g := &ApprovalGate{
			Name:          "Test",
			ActionPattern: "test",
			TimeoutHours:  12,
		}
		NormalizeApprovalGate(g)

		if g.ID == uuid.Nil {
			t.Error("expected ID to be generated")
		}
		if g.AutoResolution != DefaultAutoResolution {
			t.Errorf("expected autoResolution %q, got %q", DefaultAutoResolution, g.AutoResolution)
		}
		if g.RequiredApprovers != 1 {
			t.Errorf("expected requiredApprovers 1, got %d", g.RequiredApprovers)
		}
	})

	t.Run("preserves existing ID", func(t *testing.T) {
		id := uuid.New()
		g := &ApprovalGate{
			ID:            id,
			Name:          "Test",
			ActionPattern: "test",
			TimeoutHours:  12,
		}
		NormalizeApprovalGate(g)

		if g.ID != id {
			t.Errorf("expected ID %s to be preserved, got %s", id, g.ID)
		}
	})

	t.Run("preserves existing autoResolution", func(t *testing.T) {
		g := &ApprovalGate{
			Name:           "Test",
			ActionPattern:  "test",
			TimeoutHours:   12,
			AutoResolution: "approved",
		}
		NormalizeApprovalGate(g)

		if g.AutoResolution != "approved" {
			t.Errorf("expected autoResolution 'approved', got %q", g.AutoResolution)
		}
	})
}

func TestFindGateByID(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	gates := []ApprovalGate{
		{ID: id1, Name: "Gate 1"},
		{ID: id2, Name: "Gate 2"},
	}

	t.Run("found", func(t *testing.T) {
		g := FindGateByID(gates, id2)
		if g == nil {
			t.Fatal("expected to find gate")
		}
		if g.Name != "Gate 2" {
			t.Errorf("expected 'Gate 2', got %q", g.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		g := FindGateByID(gates, uuid.New())
		if g != nil {
			t.Error("expected nil for not found gate")
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		g := FindGateByID(nil, uuid.New())
		if g != nil {
			t.Error("expected nil for empty slice")
		}
	})
}
