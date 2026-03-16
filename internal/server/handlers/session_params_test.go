package handlers

import (
	"encoding/json"
	"testing"
)

func TestSessionParams_MarshalUnmarshal(t *testing.T) {
	params := SessionParams{SessionID: "sess-123", Cwd: "/opt/workspace"}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	var parsed SessionParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.SessionID != "sess-123" {
		t.Errorf("SessionID = %q, want sess-123", parsed.SessionID)
	}
	if parsed.Cwd != "/opt/workspace" {
		t.Errorf("Cwd = %q, want /opt/workspace", parsed.Cwd)
	}
}

func TestParseSessionParams_JSON(t *testing.T) {
	raw := `{"sessionId":"sess-abc","cwd":"/home/user"}`
	params := parseSessionParams(raw)

	if params.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want sess-abc", params.SessionID)
	}
	if params.Cwd != "/home/user" {
		t.Errorf("Cwd = %q, want /home/user", params.Cwd)
	}
}

func TestParseSessionParams_LegacyBareString(t *testing.T) {
	raw := "sess-old-format-123"
	params := parseSessionParams(raw)

	if params.SessionID != "sess-old-format-123" {
		t.Errorf("SessionID = %q, want sess-old-format-123", params.SessionID)
	}
	if params.Cwd != "" {
		t.Errorf("Cwd = %q, want empty (legacy)", params.Cwd)
	}
}

func TestParseSessionParams_Empty(t *testing.T) {
	params := parseSessionParams("")
	if params.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", params.SessionID)
	}
}

func TestCanResumeSession_CwdMatch(t *testing.T) {
	params := SessionParams{SessionID: "sess-1", Cwd: "/opt/workspace"}
	if !canResumeSession(params, "/opt/workspace") {
		t.Error("should allow resume when cwd matches")
	}
}

func TestCanResumeSession_CwdMismatch(t *testing.T) {
	params := SessionParams{SessionID: "sess-1", Cwd: "/opt/workspace"}
	if canResumeSession(params, "/different/path") {
		t.Error("should block resume when cwd differs")
	}
}

func TestCanResumeSession_NoCwd(t *testing.T) {
	params := SessionParams{SessionID: "sess-1", Cwd: ""}
	if !canResumeSession(params, "/any/path") {
		t.Error("should allow resume when no cwd recorded")
	}
}

func TestCanResumeSession_NoSessionID(t *testing.T) {
	params := SessionParams{SessionID: "", Cwd: "/opt"}
	if canResumeSession(params, "/opt") {
		t.Error("should not resume when session ID is empty")
	}
}

func TestMarshalSessionParams(t *testing.T) {
	params := SessionParams{SessionID: "sess-abc", Cwd: "/workspace"}
	raw := marshalSessionParams(params)

	var parsed SessionParams
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if parsed.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q", parsed.SessionID)
	}
	if parsed.Cwd != "/workspace" {
		t.Errorf("Cwd = %q", parsed.Cwd)
	}
}
