package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateCreateCostEventRequest(t *testing.T) {
	validID := uuid.New()

	tests := []struct {
		name    string
		req     CreateCostEventRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: CreateCostEventRequest{
				AgentID:     validID,
				AmountCents: 100,
				EventType:   "llm_call",
			},
			wantErr: false,
		},
		{
			name: "valid with all fields",
			req: CreateCostEventRequest{
				AgentID:      validID,
				AmountCents:  500,
				EventType:    "llm_call",
				Model:        strPtr("claude-4"),
				InputTokens:  costInt64Ptr(1000),
				OutputTokens: costInt64Ptr(500),
				Metadata:     json.RawMessage(`{"key":"value"}`),
			},
			wantErr: false,
		},
		{
			name: "missing agentId",
			req: CreateCostEventRequest{
				AmountCents: 100,
				EventType:   "llm_call",
			},
			wantErr: true,
			errMsg:  "agentId is required",
		},
		{
			name: "zero amountCents",
			req: CreateCostEventRequest{
				AgentID:     validID,
				AmountCents: 0,
				EventType:   "llm_call",
			},
			wantErr: true,
			errMsg:  "amountCents must be a positive integer",
		},
		{
			name: "negative amountCents",
			req: CreateCostEventRequest{
				AgentID:     validID,
				AmountCents: -10,
				EventType:   "llm_call",
			},
			wantErr: true,
			errMsg:  "amountCents must be a positive integer",
		},
		{
			name: "missing eventType",
			req: CreateCostEventRequest{
				AgentID:     validID,
				AmountCents: 100,
			},
			wantErr: true,
			errMsg:  "eventType is required",
		},
		{
			name: "eventType too long",
			req: CreateCostEventRequest{
				AgentID:     validID,
				AmountCents: 100,
				EventType:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			wantErr: true,
			errMsg:  "eventType must not exceed 50 characters",
		},
		{
			name: "model too long",
			req: CreateCostEventRequest{
				AgentID:     validID,
				AmountCents: 100,
				EventType:   "llm_call",
				Model:       strPtr(costLongStr(101)),
			},
			wantErr: true,
			errMsg:  "model must not exceed 100 characters",
		},
		{
			name: "negative inputTokens",
			req: CreateCostEventRequest{
				AgentID:     validID,
				AmountCents: 100,
				EventType:   "llm_call",
				InputTokens: costInt64Ptr(-1),
			},
			wantErr: true,
			errMsg:  "inputTokens must be non-negative",
		},
		{
			name: "negative outputTokens",
			req: CreateCostEventRequest{
				AgentID:      validID,
				AmountCents:  100,
				EventType:    "llm_call",
				OutputTokens: costInt64Ptr(-1),
			},
			wantErr: true,
			errMsg:  "outputTokens must be non-negative",
		},
		{
			name: "invalid metadata JSON",
			req: CreateCostEventRequest{
				AgentID:     validID,
				AmountCents: 100,
				EventType:   "llm_call",
				Metadata:    json.RawMessage(`{invalid`),
			},
			wantErr: true,
			errMsg:  "metadata must be valid JSON",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateCostEventRequest(tc.req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errMsg != "" && err.Error() != tc.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tc.errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBillingPeriod(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "middle of month",
			now:       time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "first of month",
			now:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "last of month",
			now:       time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
			wantStart: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end := BillingPeriod(tc.now)
			if !start.Equal(tc.wantStart) {
				t.Errorf("start = %v, want %v", start, tc.wantStart)
			}
			if !end.Equal(tc.wantEnd) {
				t.Errorf("end = %v, want %v", end, tc.wantEnd)
			}
		})
	}
}

func TestComputeThreshold(t *testing.T) {
	tests := []struct {
		name      string
		budget    *int64
		spent     int64
		wantTh    ThresholdStatus
		wantPctGe float64
		wantPctLt float64
	}{
		{
			name:   "nil budget",
			budget: nil,
			spent:  1000,
			wantTh: ThresholdOK,
		},
		{
			name:   "zero budget",
			budget: costInt64Ptr(0),
			spent:  1000,
			wantTh: ThresholdOK,
		},
		{
			name:      "under 80%",
			budget:    costInt64Ptr(1000),
			spent:     500,
			wantTh:    ThresholdOK,
			wantPctGe: 49,
			wantPctLt: 51,
		},
		{
			name:      "at 80%",
			budget:    costInt64Ptr(1000),
			spent:     800,
			wantTh:    ThresholdWarning,
			wantPctGe: 79,
			wantPctLt: 81,
		},
		{
			name:      "at 90%",
			budget:    costInt64Ptr(1000),
			spent:     900,
			wantTh:    ThresholdWarning,
			wantPctGe: 89,
			wantPctLt: 91,
		},
		{
			name:      "at 100%",
			budget:    costInt64Ptr(1000),
			spent:     1000,
			wantTh:    ThresholdExceeded,
			wantPctGe: 99,
			wantPctLt: 101,
		},
		{
			name:      "over 100%",
			budget:    costInt64Ptr(1000),
			spent:     1500,
			wantTh:    ThresholdExceeded,
			wantPctGe: 149,
			wantPctLt: 151,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			th, pct := ComputeThreshold(tc.budget, tc.spent)
			if th != tc.wantTh {
				t.Errorf("threshold = %q, want %q", th, tc.wantTh)
			}
			if tc.wantPctGe > 0 && pct < tc.wantPctGe {
				t.Errorf("pct = %f, want >= %f", pct, tc.wantPctGe)
			}
			if tc.wantPctLt > 0 && pct >= tc.wantPctLt {
				t.Errorf("pct = %f, want < %f", pct, tc.wantPctLt)
			}
		})
	}
}

// helpers (int64Ptr and longStr are unique to this file)
func costInt64Ptr(n int64) *int64 { return &n }
func costLongStr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
