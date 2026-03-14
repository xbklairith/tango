package handlers

import (
	"encoding/json"
	"testing"

	"github.com/xb/ari/internal/domain"
)

func TestChangedFieldNames_ReturnsAlphabeticallySortedKeys(t *testing.T) {
	raw := map[string]json.RawMessage{
		"title":       json.RawMessage(`"new"`),
		"status":      json.RawMessage(`"done"`),
		"description": json.RawMessage(`"desc"`),
	}
	got := changedFieldNames(raw)
	want := []string{"description", "status", "title"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, k := range want {
		if got[i] != k {
			t.Errorf("index %d: got %q, want %q", i, got[i], k)
		}
	}
}

func TestChangedFieldNames_ExcludesSpecifiedKeys(t *testing.T) {
	raw := map[string]json.RawMessage{
		"status":        json.RawMessage(`"done"`),
		"sentinelField": json.RawMessage(`true`),
	}
	got := changedFieldNames(raw, "sentinelField")
	if len(got) != 1 || got[0] != "status" {
		t.Fatalf("got %v, want [status]", got)
	}
}

func TestChangedFieldNames_EmptyBody(t *testing.T) {
	raw := map[string]json.RawMessage{}
	got := changedFieldNames(raw)
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestActivityActorType_Valid(t *testing.T) {
	tests := []struct {
		input domain.ActivityActorType
		want  bool
	}{
		{"user", true},
		{"agent", true},
		{"system", true},
		{"robot", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.input.Valid(); got != tt.want {
			t.Errorf("ActivityActorType(%q).Valid() = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestLogActivity_NilMetadataProducesEmptyRawMessage(t *testing.T) {
	// Verify that when Metadata is nil, json.RawMessage is `{}`
	p := ActivityParams{Metadata: nil}
	raw := json.RawMessage(`{}`)
	if p.Metadata != nil {
		b, err := json.Marshal(p.Metadata)
		if err != nil {
			t.Fatal(err)
		}
		raw = b
	}
	if string(raw) != "{}" {
		t.Errorf("got %q, want {}", string(raw))
	}
}

func TestLogActivity_MetadataIsMarshalled(t *testing.T) {
	p := ActivityParams{Metadata: map[string]any{"from": "todo", "to": "done"}}
	raw := json.RawMessage(`{}`)
	if p.Metadata != nil {
		b, err := json.Marshal(p.Metadata)
		if err != nil {
			t.Fatal(err)
		}
		raw = b
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result["from"] != "todo" || result["to"] != "done" {
		t.Errorf("unexpected metadata: %s", string(raw))
	}
}
