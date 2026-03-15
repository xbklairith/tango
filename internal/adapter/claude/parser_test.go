package claude

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/xb/ari/internal/adapter"
)

// --- Event parsing tests ---

func TestParseEvent_SystemInit(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"sess-123","model":"claude-sonnet-4-6"}`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if collector.sessionID != "sess-123" {
		t.Errorf("expected session_id 'sess-123', got %q", collector.sessionID)
	}
	if len(logs) == 0 {
		t.Fatal("expected at least one log line")
	}
	if logs[0].Fields["sessionId"] != "sess-123" {
		t.Errorf("expected sessionId field 'sess-123', got %v", logs[0].Fields["sessionId"])
	}
	if !strings.Contains(logs[0].Message, "claude-sonnet-4-6") {
		t.Errorf("expected model in message, got %q", logs[0].Message)
	}
}

func TestParseEvent_AssistantWithToolUse(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/src/main.go"}}]}}`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Fields["toolName"] != "Read" {
		t.Errorf("expected toolName 'Read', got %v", logs[0].Fields["toolName"])
	}
	toolInput, ok := logs[0].Fields["toolInput"].(map[string]any)
	if !ok {
		t.Fatalf("expected toolInput to be map[string]any, got %T", logs[0].Fields["toolInput"])
	}
	if toolInput["file_path"] != "/src/main.go" {
		t.Errorf("expected file_path '/src/main.go', got %v", toolInput["file_path"])
	}
}

func TestParseEvent_AssistantWithText(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Let me read the file."}]}}`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Message != "Let me read the file." {
		t.Errorf("expected text message, got %q", logs[0].Message)
	}
	if logs[0].Fields["eventType"] != "assistant" {
		t.Errorf("expected eventType 'assistant', got %v", logs[0].Fields["eventType"])
	}
}

func TestParseEvent_AssistantWithMixedContent(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Let me read the file."},{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/src/main.go"}}]}}`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if len(logs) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(logs))
	}
	// First should be text
	if logs[0].Message != "Let me read the file." {
		t.Errorf("expected text message first, got %q", logs[0].Message)
	}
	// Second should be tool_use
	if logs[1].Fields["toolName"] != "Read" {
		t.Errorf("expected toolName 'Read', got %v", logs[1].Fields["toolName"])
	}
}

func TestParseEvent_ToolResult(t *testing.T) {
	input := `{"type":"tool_result","tool_use_id":"toolu_01","content":"file contents"}`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Level != "debug" {
		t.Errorf("expected level 'debug', got %q", logs[0].Level)
	}
	if logs[0].Fields["toolUseId"] != "toolu_01" {
		t.Errorf("expected toolUseId 'toolu_01', got %v", logs[0].Fields["toolUseId"])
	}
	if !strings.Contains(logs[0].Message, "file contents") {
		t.Errorf("expected message to contain 'file contents', got %q", logs[0].Message)
	}
}

func TestParseEvent_Result(t *testing.T) {
	input := `{"type":"result","subtype":"success","session_id":"sess-123","total_cost_usd":0.086,"usage":{"input_tokens":5000,"output_tokens":1200},"modelUsage":{"claude-sonnet-4-6":{"inputTokens":5000,"outputTokens":1200,"costUSD":0.086}}}`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if collector.resultEvent == nil {
		t.Fatal("expected resultEvent to be set")
	}
	if collector.resultEvent.TotalCostUSD != 0.086 {
		t.Errorf("expected total_cost_usd 0.086, got %f", collector.resultEvent.TotalCostUSD)
	}
	if collector.resultEvent.Usage.InputTokens != 5000 {
		t.Errorf("expected input_tokens 5000, got %d", collector.resultEvent.Usage.InputTokens)
	}
	if collector.resultEvent.Usage.OutputTokens != 1200 {
		t.Errorf("expected output_tokens 1200, got %d", collector.resultEvent.Usage.OutputTokens)
	}
	modelUse, ok := collector.resultEvent.ModelUsage["claude-sonnet-4-6"]
	if !ok {
		t.Fatal("expected modelUsage for claude-sonnet-4-6")
	}
	if modelUse.CostUSD != 0.086 {
		t.Errorf("expected costUSD 0.086, got %f", modelUse.CostUSD)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Fields["totalCostUSD"] != 0.086 {
		t.Errorf("expected totalCostUSD field 0.086, got %v", logs[0].Fields["totalCostUSD"])
	}
}

func TestParseEvent_RateLimitEvent(t *testing.T) {
	input := `{"type":"rate_limit_event","rate_limit_info":{"retryAfterMs":5000}}`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Level != "warn" {
		t.Errorf("expected level 'warn', got %q", logs[0].Level)
	}
	if logs[0].Fields["rateLimited"] != true {
		t.Errorf("expected rateLimited true, got %v", logs[0].Fields["rateLimited"])
	}
}

func TestParseEvent_MalformedJSON(t *testing.T) {
	input := `{broken json`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	// Should not panic
	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line (warning), got %d", len(logs))
	}
	if logs[0].Level != "warn" {
		t.Errorf("expected level 'warn', got %q", logs[0].Level)
	}
	if !strings.Contains(logs[0].Message, "failed to parse") {
		t.Errorf("expected parse failure message, got %q", logs[0].Message)
	}
}

func TestParseEvent_UnknownType(t *testing.T) {
	input := `{"type":"unknown_future_event"}`
	r := strings.NewReader(input + "\n")
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	// Should not crash
	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Level != "debug" {
		t.Errorf("expected level 'debug', got %q", logs[0].Level)
	}
	if logs[0].Fields["eventType"] != "unknown_future_event" {
		t.Errorf("expected eventType 'unknown_future_event', got %v", logs[0].Fields["eventType"])
	}
}

// --- eventCollector tests ---

func TestEventCollector_ExtractUsage(t *testing.T) {
	collector := &eventCollector{}

	// Simulate system/init
	collector.sessionID = "sess-abc"

	// Simulate result event
	collector.resultEvent = &claudeEvent{
		Type:         "result",
		SessionID:    "sess-abc",
		TotalCostUSD: 0.086,
		Model:        "claude-sonnet-4-6",
		Usage: &claudeUsage{
			InputTokens:  5000,
			OutputTokens: 1200,
		},
	}

	usage, sessionState, costUSD := collector.extract()

	if usage.InputTokens != 5000 {
		t.Errorf("expected InputTokens 5000, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 1200 {
		t.Errorf("expected OutputTokens 1200, got %d", usage.OutputTokens)
	}
	if usage.Provider != "anthropic" {
		t.Errorf("expected Provider 'anthropic', got %q", usage.Provider)
	}
	if usage.Model != "claude-sonnet-4-6" {
		t.Errorf("expected Model 'claude-sonnet-4-6', got %q", usage.Model)
	}
	if sessionState != "sess-abc" {
		t.Errorf("expected sessionState 'sess-abc', got %q", sessionState)
	}
	if costUSD != 0.086 {
		t.Errorf("expected costUSD 0.086, got %f", costUSD)
	}
}

func TestEventCollector_NoResultEvent(t *testing.T) {
	collector := &eventCollector{
		sessionID: "sess-xyz",
	}

	usage, sessionState, costUSD := collector.extract()

	if usage.InputTokens != 0 {
		t.Errorf("expected InputTokens 0, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 0 {
		t.Errorf("expected OutputTokens 0, got %d", usage.OutputTokens)
	}
	if sessionState != "sess-xyz" {
		t.Errorf("expected sessionState 'sess-xyz', got %q", sessionState)
	}
	if costUSD != 0 {
		t.Errorf("expected costUSD 0, got %f", costUSD)
	}
}

func TestEventCollector_EmptyCollector(t *testing.T) {
	collector := &eventCollector{}

	usage, sessionState, costUSD := collector.extract()

	if usage.InputTokens != 0 {
		t.Errorf("expected InputTokens 0, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 0 {
		t.Errorf("expected OutputTokens 0, got %d", usage.OutputTokens)
	}
	if usage.Provider != "anthropic" {
		t.Errorf("expected Provider 'anthropic', got %q", usage.Provider)
	}
	if sessionState != "" {
		t.Errorf("expected empty sessionState, got %q", sessionState)
	}
	if costUSD != 0 {
		t.Errorf("expected costUSD 0, got %f", costUSD)
	}
}

// --- streamAndParseEvents integration tests ---

func TestStreamAndParseEvents_FullStream(t *testing.T) {
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"sess-full","model":"claude-sonnet-4-6"}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/src/main.go"}}]}}`,
		`{"type":"tool_result","tool_use_id":"toolu_01","content":"package main"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"I read the file."}]}}`,
		`{"type":"result","subtype":"success","session_id":"sess-full","total_cost_usd":0.086,"usage":{"input_tokens":5000,"output_tokens":1200}}`,
	}
	input := strings.Join(lines, "\n") + "\n"
	r := strings.NewReader(input)
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	// Should have 5 log lines: system, tool_use, tool_result, text, result
	if len(logs) != 5 {
		t.Fatalf("expected 5 log lines, got %d", len(logs))
	}

	// Verify collector state
	if collector.sessionID != "sess-full" {
		t.Errorf("expected sessionID 'sess-full', got %q", collector.sessionID)
	}
	if collector.resultEvent == nil {
		t.Fatal("expected resultEvent to be set")
	}
	if collector.resultEvent.TotalCostUSD != 0.086 {
		t.Errorf("expected total_cost_usd 0.086, got %f", collector.resultEvent.TotalCostUSD)
	}

	// Verify extract
	usage, session, costUSD := collector.extract()
	if usage.InputTokens != 5000 {
		t.Errorf("expected InputTokens 5000, got %d", usage.InputTokens)
	}
	if session != "sess-full" {
		t.Errorf("expected session 'sess-full', got %q", session)
	}
	if costUSD != 0.086 {
		t.Errorf("expected costUSD 0.086, got %f", costUSD)
	}
}

func TestStreamAndParseEvents_ToolCallFields(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/src/main.go"}}]}}` + "\n"
	r := strings.NewReader(input)
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Fields["toolName"] != "Read" {
		t.Errorf("expected toolName 'Read', got %v", logs[0].Fields["toolName"])
	}
	toolInput, ok := logs[0].Fields["toolInput"].(map[string]any)
	if !ok {
		t.Fatalf("expected toolInput to be map[string]any, got %T", logs[0].Fields["toolInput"])
	}
	if toolInput["file_path"] != "/src/main.go" {
		t.Errorf("expected file_path '/src/main.go', got %v", toolInput["file_path"])
	}
}

func TestStreamAndParseEvents_RateLimitDetected(t *testing.T) {
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"sess-rl","model":"claude-sonnet-4-6"}`,
		`{"type":"rate_limit_event","rate_limit_info":{"retryAfterMs":5000}}`,
		`{"type":"result","subtype":"success","session_id":"sess-rl","total_cost_usd":0.01,"usage":{"input_tokens":100,"output_tokens":50}}`,
	}
	input := strings.Join(lines, "\n") + "\n"
	r := strings.NewReader(input)
	var buf bytes.Buffer
	collector := &eventCollector{}
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamAndParseEvents(r, &buf, 65536, hooks, collector)

	// Find the rate limit log line
	found := false
	for _, log := range logs {
		if log.Fields != nil && log.Fields["rateLimited"] == true {
			found = true
			if log.Level != "warn" {
				t.Errorf("expected level 'warn' for rate limit, got %q", log.Level)
			}
			break
		}
	}
	if !found {
		t.Error("expected a log line with rateLimited=true")
	}
}

// --- streamStderr tests ---

func TestStreamStderr_RateLimitFallback(t *testing.T) {
	input := "Error: rate limit exceeded, please wait\n"
	r := strings.NewReader(input)
	var buf bytes.Buffer
	var logs []adapter.LogLine
	var mu sync.Mutex
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			mu.Lock()
			logs = append(logs, line)
			mu.Unlock()
		},
	}

	streamStderr(r, &buf, 65536, hooks)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Fields["rateLimited"] != true {
		t.Errorf("expected rateLimited true, got %v", logs[0].Fields["rateLimited"])
	}
	if logs[0].Fields["source"] != "stderr" {
		t.Errorf("expected source 'stderr', got %v", logs[0].Fields["source"])
	}
	if logs[0].Level != "warn" {
		t.Errorf("expected level 'warn', got %q", logs[0].Level)
	}
}

func TestStreamStderr_429Fallback(t *testing.T) {
	input := "HTTP 429 Too Many Requests\n"
	r := strings.NewReader(input)
	var buf bytes.Buffer
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamStderr(r, &buf, 65536, hooks)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Fields["rateLimited"] != true {
		t.Errorf("expected rateLimited true, got %v", logs[0].Fields["rateLimited"])
	}
	if logs[0].Fields["source"] != "stderr" {
		t.Errorf("expected source 'stderr', got %v", logs[0].Fields["source"])
	}
}

func TestStreamStderr_NormalLine(t *testing.T) {
	input := "some error occurred in processing\n"
	r := strings.NewReader(input)
	var buf bytes.Buffer
	var logs []adapter.LogLine
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			logs = append(logs, line)
		},
	}

	streamStderr(r, &buf, 65536, hooks)

	if len(logs) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logs))
	}
	if logs[0].Level != "error" {
		t.Errorf("expected level 'error', got %q", logs[0].Level)
	}
	if logs[0].Fields != nil {
		if _, hasRL := logs[0].Fields["rateLimited"]; hasRL {
			t.Error("expected no rateLimited field for normal stderr line")
		}
	}
}
