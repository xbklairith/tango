package domain

import (
	"testing"
)

func TestIssueTypeValid(t *testing.T) {
	tests := []struct {
		typ  IssueType
		want bool
	}{
		{IssueTypeTask, true},
		{IssueTypeConversation, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.typ.Valid(); got != tt.want {
			t.Errorf("IssueType(%q).Valid() = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestIssueStatusValid(t *testing.T) {
	tests := []struct {
		status IssueStatus
		want   bool
	}{
		{IssueStatusBacklog, true},
		{IssueStatusTodo, true},
		{IssueStatusInProgress, true},
		{IssueStatusDone, true},
		{IssueStatusBlocked, true},
		{IssueStatusCancelled, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.status.Valid(); got != tt.want {
			t.Errorf("IssueStatus(%q).Valid() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestIssuePriorityValid(t *testing.T) {
	tests := []struct {
		p    IssuePriority
		want bool
	}{
		{IssuePriorityCritical, true},
		{IssuePriorityHigh, true},
		{IssuePriorityMedium, true},
		{IssuePriorityLow, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.p.Valid(); got != tt.want {
			t.Errorf("IssuePriority(%q).Valid() = %v, want %v", tt.p, got, tt.want)
		}
	}
}

func TestCommentAuthorTypeValid(t *testing.T) {
	tests := []struct {
		a    CommentAuthorType
		want bool
	}{
		{CommentAuthorAgent, true},
		{CommentAuthorUser, true},
		{CommentAuthorSystem, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.a.Valid(); got != tt.want {
			t.Errorf("CommentAuthorType(%q).Valid() = %v, want %v", tt.a, got, tt.want)
		}
	}
}

func TestValidateIssueTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    IssueStatus
		to      IssueStatus
		wantErr bool
	}{
		// Valid transitions
		{"backlog->todo", IssueStatusBacklog, IssueStatusTodo, false},
		{"backlog->in_progress", IssueStatusBacklog, IssueStatusInProgress, false},
		{"backlog->cancelled", IssueStatusBacklog, IssueStatusCancelled, false},
		{"todo->in_progress", IssueStatusTodo, IssueStatusInProgress, false},
		{"todo->backlog", IssueStatusTodo, IssueStatusBacklog, false},
		{"todo->blocked", IssueStatusTodo, IssueStatusBlocked, false},
		{"todo->cancelled", IssueStatusTodo, IssueStatusCancelled, false},
		{"in_progress->done", IssueStatusInProgress, IssueStatusDone, false},
		{"in_progress->blocked", IssueStatusInProgress, IssueStatusBlocked, false},
		{"in_progress->cancelled", IssueStatusInProgress, IssueStatusCancelled, false},
		{"blocked->in_progress", IssueStatusBlocked, IssueStatusInProgress, false},
		{"blocked->todo", IssueStatusBlocked, IssueStatusTodo, false},
		{"blocked->cancelled", IssueStatusBlocked, IssueStatusCancelled, false},
		{"done->todo (reopen)", IssueStatusDone, IssueStatusTodo, false},
		{"cancelled->todo (reopen)", IssueStatusCancelled, IssueStatusTodo, false},

		// Invalid transitions
		{"backlog->done", IssueStatusBacklog, IssueStatusDone, true},
		{"backlog->blocked", IssueStatusBacklog, IssueStatusBlocked, true},
		{"todo->done", IssueStatusTodo, IssueStatusDone, true},
		{"in_progress->backlog", IssueStatusInProgress, IssueStatusBacklog, true},
		{"in_progress->todo", IssueStatusInProgress, IssueStatusTodo, true},
		{"done->in_progress", IssueStatusDone, IssueStatusInProgress, true},
		{"done->cancelled", IssueStatusDone, IssueStatusCancelled, true},
		{"cancelled->in_progress", IssueStatusCancelled, IssueStatusInProgress, true},
		{"cancelled->done", IssueStatusCancelled, IssueStatusDone, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIssueTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIssueTransition(%q, %q) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

func TestIsReopen(t *testing.T) {
	tests := []struct {
		name string
		from IssueStatus
		to   IssueStatus
		want bool
	}{
		{"done->todo", IssueStatusDone, IssueStatusTodo, true},
		{"cancelled->todo", IssueStatusCancelled, IssueStatusTodo, true},
		{"backlog->todo", IssueStatusBacklog, IssueStatusTodo, false},
		{"todo->in_progress", IssueStatusTodo, IssueStatusInProgress, false},
		{"done->backlog", IssueStatusDone, IssueStatusBacklog, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReopen(tt.from, tt.to); got != tt.want {
				t.Errorf("IsReopen(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestValidateCreateIssueInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateIssueRequest
		wantErr bool
	}{
		{"valid minimal", CreateIssueRequest{Title: "Fix bug"}, false},
		{"empty title", CreateIssueRequest{Title: ""}, true},
		{"title too long", CreateIssueRequest{Title: string(make([]byte, 501))}, true},
		{"invalid type", CreateIssueRequest{Title: "X", Type: ptr(IssueType("bad"))}, true},
		{"invalid status", CreateIssueRequest{Title: "X", Status: ptr(IssueStatus("bad"))}, true},
		{"invalid priority", CreateIssueRequest{Title: "X", Priority: ptr(IssuePriority("bad"))}, true},
		{"negative depth", CreateIssueRequest{Title: "X", RequestDepth: intPtr(-1)}, true},
		{"valid with all fields", CreateIssueRequest{
			Title:    "Full issue",
			Type:     ptr(IssueTypeConversation),
			Status:   ptr(IssueStatusTodo),
			Priority: ptr(IssuePriorityHigh),
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateIssueInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateIssueInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUpdateIssueInput(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdateIssueRequest
		wantErr bool
	}{
		{"empty (noop)", UpdateIssueRequest{}, false},
		{"valid title", UpdateIssueRequest{Title: strPtr("New title")}, false},
		{"empty title", UpdateIssueRequest{Title: strPtr("")}, true},
		{"title too long", UpdateIssueRequest{Title: strPtr(string(make([]byte, 501)))}, true},
		{"invalid type", UpdateIssueRequest{Type: ptr(IssueType("bad"))}, true},
		{"invalid status", UpdateIssueRequest{Status: ptr(IssueStatus("bad"))}, true},
		{"invalid priority", UpdateIssueRequest{Priority: ptr(IssuePriority("bad"))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateIssueInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdateIssueInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- helpers ---

func ptr[T any](v T) *T { return &v }
func intPtr(i int) *int { return &i }
