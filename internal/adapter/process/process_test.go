package process_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xb/ari/internal/adapter"
	"github.com/xb/ari/internal/adapter/process"
)

func TestProcessAdapter_Type(t *testing.T) {
	p := process.New()
	if p.Type() != "process" {
		t.Fatalf("expected type %q, got %q", "process", p.Type())
	}
}

func TestProcessAdapter_TestEnvironment(t *testing.T) {
	p := process.New()
	result, err := p.TestEnvironment(adapter.TestLevelBasic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Available {
		t.Fatal("expected process adapter to be available")
	}
}

func TestProcessAdapter_ExecuteSuccess(t *testing.T) {
	p := process.New()
	input := adapter.InvokeInput{
		Agent: adapter.AgentContext{
			ID:            uuid.New(),
			ShortName:     "echo",
			AdapterConfig: json.RawMessage(`{"command":"echo","args":["hello world"]}`),
		},
		Squad: adapter.SquadContext{ID: uuid.New()},
		Run:   adapter.RunContext{RunID: uuid.New(), WakeReason: "on_demand"},
		EnvVars: map[string]string{
			"ARI_API_URL": "http://localhost:3100/api",
		},
	}

	var logLines []adapter.LogLine
	var mu sync.Mutex
	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			mu.Lock()
			logLines = append(logLines, line)
			mu.Unlock()
		},
	}

	result, err := p.Execute(context.Background(), input, hooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != adapter.RunStatusSucceeded {
		t.Fatalf("expected status %q, got %q", adapter.RunStatusSucceeded, result.Status)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello world") {
		t.Fatalf("expected stdout to contain 'hello world', got %q", result.Stdout)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(logLines) == 0 {
		t.Fatal("expected at least one log line")
	}
}

func TestProcessAdapter_ExecuteFailure(t *testing.T) {
	p := process.New()
	input := adapter.InvokeInput{
		Agent: adapter.AgentContext{
			ShortName:     "false",
			AdapterConfig: json.RawMessage(`{"command":"false"}`),
		},
		Run: adapter.RunContext{RunID: uuid.New(), WakeReason: "on_demand"},
	}

	result, err := p.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != adapter.RunStatusFailed {
		t.Fatalf("expected status %q, got %q", adapter.RunStatusFailed, result.Status)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestProcessAdapter_ExecuteCancel(t *testing.T) {
	p := process.New()
	input := adapter.InvokeInput{
		Agent: adapter.AgentContext{
			ShortName:     "sleep",
			AdapterConfig: json.RawMessage(`{"command":"sleep","args":["30"]}`),
		},
		Run: adapter.RunContext{RunID: uuid.New(), WakeReason: "on_demand"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result, err := p.Execute(ctx, input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != adapter.RunStatusCancelled {
		t.Fatalf("expected status %q, got %q", adapter.RunStatusCancelled, result.Status)
	}
}

func TestProcessAdapter_SessionState(t *testing.T) {
	p := process.New()
	input := adapter.InvokeInput{
		Agent: adapter.AgentContext{
			ShortName:     "echo",
			AdapterConfig: json.RawMessage(`{"command":"echo","args":["{\"ari_session_state\":\"my-session-123\"}"]}`),
		},
		Run: adapter.RunContext{RunID: uuid.New(), WakeReason: "on_demand"},
	}

	result, err := p.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionState != "my-session-123" {
		t.Fatalf("expected session state %q, got %q", "my-session-123", result.SessionState)
	}
}

func TestProcessAdapter_EnvVarsInjected(t *testing.T) {
	p := process.New()
	input := adapter.InvokeInput{
		Agent: adapter.AgentContext{
			ShortName:     "env-test",
			AdapterConfig: json.RawMessage(`{"command":"sh","args":["-c","echo $ARI_TEST_VAR"]}`),
		},
		Run: adapter.RunContext{RunID: uuid.New(), WakeReason: "on_demand"},
		EnvVars: map[string]string{
			"ARI_TEST_VAR": "test-value-xyz",
		},
	}

	result, err := p.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stdout, "test-value-xyz") {
		t.Fatalf("expected ARI_TEST_VAR in output, got %q", result.Stdout)
	}
}

func TestProcessAdapter_Models(t *testing.T) {
	p := process.New()
	if models := p.Models(); models != nil {
		t.Fatalf("expected nil models, got %v", models)
	}
}
