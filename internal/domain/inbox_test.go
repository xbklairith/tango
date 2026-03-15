package domain

import (
	"encoding/json"
	"testing"
)

// -------- TestValidateInboxStatusTransition --------

func TestValidateInboxStatusTransition_AllowedTransitions(t *testing.T) {
	allowed := []struct {
		from, to InboxStatus
	}{
		{InboxStatusPending, InboxStatusAcknowledged},
		{InboxStatusPending, InboxStatusResolved},
		{InboxStatusPending, InboxStatusExpired},
		{InboxStatusAcknowledged, InboxStatusResolved},
		{InboxStatusAcknowledged, InboxStatusExpired},
	}

	for _, tc := range allowed {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			if err := ValidateInboxStatusTransition(tc.from, tc.to); err != nil {
				t.Errorf("expected transition %s->%s to be allowed, got error: %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidateInboxStatusTransition_RejectedTransitions(t *testing.T) {
	rejected := []struct {
		from, to InboxStatus
	}{
		{InboxStatusResolved, InboxStatusPending},
		{InboxStatusResolved, InboxStatusAcknowledged},
		{InboxStatusAcknowledged, InboxStatusPending},
		{InboxStatusExpired, InboxStatusPending},
		{InboxStatusExpired, InboxStatusResolved},
		{InboxStatusExpired, InboxStatusAcknowledged},
		{InboxStatusResolved, InboxStatusExpired},
	}

	for _, tc := range rejected {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			if err := ValidateInboxStatusTransition(tc.from, tc.to); err == nil {
				t.Errorf("expected transition %s->%s to be rejected, but it was allowed", tc.from, tc.to)
			}
		})
	}
}

// -------- TestValidResolutionsForCategory --------

func TestValidResolutionsForCategory(t *testing.T) {
	tests := []struct {
		category InboxCategory
		expected []InboxResolution
	}{
		{
			category: InboxCategoryApproval,
			expected: []InboxResolution{InboxResolutionApproved, InboxResolutionRejected, InboxResolutionRequestRevision},
		},
		{
			category: InboxCategoryQuestion,
			expected: []InboxResolution{InboxResolutionAnswered, InboxResolutionDismissed},
		},
		{
			category: InboxCategoryDecision,
			expected: []InboxResolution{InboxResolutionAnswered, InboxResolutionDismissed},
		},
		{
			category: InboxCategoryAlert,
			expected: []InboxResolution{InboxResolutionDismissed},
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.category), func(t *testing.T) {
			got := ValidResolutionsForCategory(tc.category)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected %d resolutions for %s, got %d: %v", len(tc.expected), tc.category, len(got), got)
			}
			for i, r := range got {
				if r != tc.expected[i] {
					t.Errorf("resolution[%d]: expected %s, got %s", i, tc.expected[i], r)
				}
			}
		})
	}
}

func TestValidResolutionsForCategory_UnknownCategory(t *testing.T) {
	got := ValidResolutionsForCategory(InboxCategory("unknown"))
	if got != nil {
		t.Errorf("expected nil for unknown category, got %v", got)
	}
}

func TestIsValidResolutionForCategory(t *testing.T) {
	// Approval: approved, rejected, request_revision are valid; answered/dismissed are not.
	if !IsValidResolutionForCategory(InboxCategoryApproval, InboxResolutionApproved) {
		t.Error("expected approved to be valid for approval")
	}
	if !IsValidResolutionForCategory(InboxCategoryApproval, InboxResolutionRejected) {
		t.Error("expected rejected to be valid for approval")
	}
	if !IsValidResolutionForCategory(InboxCategoryApproval, InboxResolutionRequestRevision) {
		t.Error("expected request_revision to be valid for approval")
	}
	if IsValidResolutionForCategory(InboxCategoryApproval, InboxResolutionAnswered) {
		t.Error("expected answered to be invalid for approval")
	}
	if IsValidResolutionForCategory(InboxCategoryApproval, InboxResolutionDismissed) {
		t.Error("expected dismissed to be invalid for approval")
	}

	// Alert: only dismissed is valid.
	if !IsValidResolutionForCategory(InboxCategoryAlert, InboxResolutionDismissed) {
		t.Error("expected dismissed to be valid for alert")
	}
	if IsValidResolutionForCategory(InboxCategoryAlert, InboxResolutionApproved) {
		t.Error("expected approved to be invalid for alert")
	}
}

// -------- TestCategoryWakesAgent --------

func TestCategoryWakesAgent(t *testing.T) {
	tests := []struct {
		category InboxCategory
		expected bool
	}{
		{InboxCategoryApproval, true},
		{InboxCategoryQuestion, true},
		{InboxCategoryDecision, true},
		{InboxCategoryAlert, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.category), func(t *testing.T) {
			got := CategoryWakesAgent(tc.category)
			if got != tc.expected {
				t.Errorf("CategoryWakesAgent(%s) = %v, want %v", tc.category, got, tc.expected)
			}
		})
	}
}

func TestCategoryWakesAgent_UnknownCategory(t *testing.T) {
	if CategoryWakesAgent(InboxCategory("unknown")) {
		t.Error("expected unknown category to not wake agent")
	}
}

// -------- TestValidateCreateInboxItemRequest --------

func TestValidateCreateInboxItemRequest_Valid(t *testing.T) {
	req := CreateInboxItemRequest{
		Category: InboxCategoryQuestion,
		Type:     "clarify_requirement",
		Title:    "Need clarification on auth flow",
	}
	if err := ValidateCreateInboxItemRequest(req); err != nil {
		t.Errorf("expected valid request, got error: %v", err)
	}
}

func TestValidateCreateInboxItemRequest_ValidWithOptionalFields(t *testing.T) {
	urgency := InboxUrgencyCritical
	body := "Should login support SSO?"
	payload := json.RawMessage(`{"options":["a","b"]}`)
	req := CreateInboxItemRequest{
		Category: InboxCategoryApproval,
		Type:     "deploy_approval",
		Title:    "Approve deployment to prod",
		Urgency:  &urgency,
		Body:     &body,
		Payload:  payload,
	}
	if err := ValidateCreateInboxItemRequest(req); err != nil {
		t.Errorf("expected valid request, got error: %v", err)
	}
}

func TestValidateCreateInboxItemRequest_TitleRequired(t *testing.T) {
	req := CreateInboxItemRequest{
		Category: InboxCategoryAlert,
		Type:     "budget_threshold_80",
		Title:    "",
	}
	err := ValidateCreateInboxItemRequest(req)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if err.Error() != "title is required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateCreateInboxItemRequest_TitleTooLong(t *testing.T) {
	longTitle := make([]byte, 501)
	for i := range longTitle {
		longTitle[i] = 'a'
	}
	req := CreateInboxItemRequest{
		Category: InboxCategoryAlert,
		Type:     "budget_threshold_80",
		Title:    string(longTitle),
	}
	err := ValidateCreateInboxItemRequest(req)
	if err == nil {
		t.Fatal("expected error for title too long")
	}
	if err.Error() != "title must not exceed 500 characters" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateCreateInboxItemRequest_TypeRequired(t *testing.T) {
	req := CreateInboxItemRequest{
		Category: InboxCategoryAlert,
		Type:     "",
		Title:    "Some title",
	}
	err := ValidateCreateInboxItemRequest(req)
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	if err.Error() != "type is required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateCreateInboxItemRequest_TypeTooLong(t *testing.T) {
	longType := make([]byte, 101)
	for i := range longType {
		longType[i] = 'a'
	}
	req := CreateInboxItemRequest{
		Category: InboxCategoryAlert,
		Type:     string(longType),
		Title:    "Some title",
	}
	err := ValidateCreateInboxItemRequest(req)
	if err == nil {
		t.Fatal("expected error for type too long")
	}
	if err.Error() != "type must not exceed 100 characters" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateCreateInboxItemRequest_InvalidCategory(t *testing.T) {
	req := CreateInboxItemRequest{
		Category: InboxCategory("invalid"),
		Type:     "some_type",
		Title:    "Some title",
	}
	err := ValidateCreateInboxItemRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
	if err.Error() != "category must be one of: approval, question, decision, alert" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateCreateInboxItemRequest_InvalidUrgency(t *testing.T) {
	badUrgency := InboxUrgency("urgent")
	req := CreateInboxItemRequest{
		Category: InboxCategoryAlert,
		Type:     "some_type",
		Title:    "Some title",
		Urgency:  &badUrgency,
	}
	err := ValidateCreateInboxItemRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid urgency")
	}
	if err.Error() != "urgency must be one of: critical, normal, low" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateCreateInboxItemRequest_InvalidPayload(t *testing.T) {
	req := CreateInboxItemRequest{
		Category: InboxCategoryAlert,
		Type:     "some_type",
		Title:    "Some title",
		Payload:  json.RawMessage(`{invalid json`),
	}
	err := ValidateCreateInboxItemRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
	if err.Error() != "payload must be valid JSON" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// -------- Enum Valid() method tests --------

func TestInboxCategoryValid(t *testing.T) {
	validCategories := []InboxCategory{InboxCategoryApproval, InboxCategoryQuestion, InboxCategoryDecision, InboxCategoryAlert}
	for _, c := range validCategories {
		if !c.Valid() {
			t.Errorf("expected %s to be valid", c)
		}
	}
	if InboxCategory("bogus").Valid() {
		t.Error("expected bogus category to be invalid")
	}
}

func TestInboxUrgencyValid(t *testing.T) {
	validUrgencies := []InboxUrgency{InboxUrgencyCritical, InboxUrgencyNormal, InboxUrgencyLow}
	for _, u := range validUrgencies {
		if !u.Valid() {
			t.Errorf("expected %s to be valid", u)
		}
	}
	if InboxUrgency("bogus").Valid() {
		t.Error("expected bogus urgency to be invalid")
	}
}

func TestInboxStatusValid(t *testing.T) {
	validStatuses := []InboxStatus{InboxStatusPending, InboxStatusAcknowledged, InboxStatusResolved, InboxStatusExpired}
	for _, s := range validStatuses {
		if !s.Valid() {
			t.Errorf("expected %s to be valid", s)
		}
	}
	if InboxStatus("bogus").Valid() {
		t.Error("expected bogus status to be invalid")
	}
}

func TestInboxResolutionValid(t *testing.T) {
	validResolutions := []InboxResolution{
		InboxResolutionApproved, InboxResolutionRejected, InboxResolutionRequestRevision,
		InboxResolutionAnswered, InboxResolutionDismissed,
	}
	for _, r := range validResolutions {
		if !r.Valid() {
			t.Errorf("expected %s to be valid", r)
		}
	}
	if InboxResolution("bogus").Valid() {
		t.Error("expected bogus resolution to be invalid")
	}
}
