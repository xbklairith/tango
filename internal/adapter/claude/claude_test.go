package claude_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xb/ari/internal/adapter"
	"github.com/xb/ari/internal/adapter/claude"
	"github.com/xb/ari/internal/adapter/process"
)

// --- Task 03 tests ---

func TestClaudeAdapter_Type(t *testing.T) {
	a := claude.New()
	if got := a.Type(); got != "claude_local" {
		t.Errorf("Type() = %q, want %q", got, "claude_local")
	}
}

func TestClaudeAdapter_Models(t *testing.T) {
	a := claude.New()
	models := a.Models()

	if len(models) != 3 {
		t.Fatalf("Models() returned %d entries, want 3", len(models))
	}

	expected := []struct {
		id       string
		name     string
		provider string
	}{
		{"opus", "Claude Opus", "anthropic"},
		{"sonnet", "Claude Sonnet", "anthropic"},
		{"haiku", "Claude Haiku", "anthropic"},
	}

	for i, exp := range expected {
		m := models[i]
		if m.ID != exp.id {
			t.Errorf("Models()[%d].ID = %q, want %q", i, m.ID, exp.id)
		}
		if m.Name != exp.name {
			t.Errorf("Models()[%d].Name = %q, want %q", i, m.Name, exp.name)
		}
		if m.Provider != exp.provider {
			t.Errorf("Models()[%d].Provider = %q, want %q", i, m.Provider, exp.provider)
		}
	}
}

func TestClaudeAdapter_ImplementsInterface(t *testing.T) {
	var a adapter.Adapter = claude.New()
	if a == nil {
		t.Fatal("New() returned nil")
	}
	if a.Type() != "claude_local" {
		t.Errorf("interface Type() = %q, want %q", a.Type(), "claude_local")
	}
}

func TestClaudeAdapter_TestEnvironmentBasic_Found(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not in PATH, skipping")
	}

	a := claude.New()
	result, err := a.TestEnvironment(adapter.TestLevelBasic)
	if err != nil {
		t.Fatalf("TestEnvironment(Basic) returned error: %v", err)
	}
	if !result.Available {
		t.Errorf("TestEnvironment(Basic) Available = false, want true; message: %s", result.Message)
	}
	if result.Message == "" {
		t.Error("TestEnvironment(Basic) Message is empty, want non-empty")
	}
}

func TestClaudeAdapter_TestEnvironmentBasic_NotFound(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	a := claude.New()
	result, err := a.TestEnvironment(adapter.TestLevelBasic)
	if err != nil {
		t.Fatalf("TestEnvironment(Basic) returned error: %v", err)
	}
	if result.Available {
		t.Error("TestEnvironment(Basic) Available = true, want false when claude is not in PATH")
	}
	if result.Message == "" {
		t.Error("TestEnvironment(Basic) Message is empty, want non-empty")
	}
}

// --- Registry tests (Task 06 prerequisite) ---

func TestRegistry_ClaudeAdapterRegistered(t *testing.T) {
	reg := adapter.NewRegistry()
	reg.Register(claude.New())

	a, err := reg.Resolve("claude_local")
	if err != nil {
		t.Fatalf("Resolve(claude_local) returned error: %v", err)
	}
	if a == nil {
		t.Fatal("Resolve(claude_local) returned nil adapter")
	}
	if a.Type() != "claude_local" {
		t.Errorf("Resolve(claude_local).Type() = %q, want %q", a.Type(), "claude_local")
	}
}

func TestRegistry_ClaudeAndProcessCoexist(t *testing.T) {
	reg := adapter.NewRegistry()
	reg.Register(process.New())
	reg.Register(claude.New())

	ca, err := reg.Resolve("claude_local")
	if err != nil {
		t.Fatalf("Resolve(claude_local) returned error: %v", err)
	}
	if ca == nil {
		t.Fatal("Resolve(claude_local) returned nil")
	}
	if ca.Type() != "claude_local" {
		t.Errorf("claude adapter Type() = %q, want %q", ca.Type(), "claude_local")
	}

	pa, err := reg.Resolve("process")
	if err != nil {
		t.Fatalf("Resolve(process) returned error: %v", err)
	}
	if pa == nil {
		t.Fatal("Resolve(process) returned nil")
	}
	if pa.Type() != "process" {
		t.Errorf("process adapter Type() = %q, want %q", pa.Type(), "process")
	}
}

// --- Test helpers ---

func writeTempScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude-mock.sh")
	err := os.WriteFile(path, []byte("#!/bin/sh\n"+content), 0o755)
	if err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	return path
}

func makeInput(configJSON string, prompt string) adapter.InvokeInput {
	var raw json.RawMessage
	if configJSON != "" {
		raw = json.RawMessage(configJSON)
	}
	return adapter.InvokeInput{
		Agent: adapter.AgentContext{
			AdapterConfig: raw,
		},
		Prompt: prompt,
	}
}

func makeInputWithScript(t *testing.T, scriptContent string, extraConfigFields string, prompt string) adapter.InvokeInput {
	t.Helper()
	scriptPath := writeTempScript(t, scriptContent)
	cfgJSON := fmt.Sprintf(`{"claudePath":%q,"timeoutSeconds":10`, scriptPath)
	if extraConfigFields != "" {
		cfgJSON += "," + extraConfigFields
	}
	cfgJSON += "}"
	return makeInput(cfgJSON, prompt)
}

// --- Task 04 tests: BuildArgs ---

func TestExecute_BuildArgs_BasicFlags(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, "", "hello world")
	input.Agent.SystemPrompt = "You are a test agent"

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != adapter.RunStatusSucceeded {
		t.Fatalf("Status = %q, want %q; stderr: %s", result.Status, adapter.RunStatusSucceeded, result.Stderr)
	}

	stdout := result.Stdout
	for _, flag := range []string{
		"--print",
		"--output-format",
		"stream-json",
		"--model",
		"sonnet",
		"--append-system-prompt",
		"--dangerously-skip-permissions",
	} {
		if !strings.Contains(stdout, flag) {
			t.Errorf("expected flag %q in args, got stdout:\n%s", flag, stdout)
		}
	}

	if !strings.Contains(stdout, "You are a test agent") {
		t.Errorf("expected system prompt in args, got stdout:\n%s", stdout)
	}
}

func TestExecute_BuildArgs_WithResume(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, "", "test prompt")
	input.Run.SessionState = "sess-abc-123"

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--resume") {
		t.Errorf("expected --resume in args, got stdout:\n%s", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "sess-abc-123") {
		t.Errorf("expected session ID in args, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_BuildArgs_WithMaxBudget(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, `"maxBudgetUSD":5.00`, "test prompt")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--max-budget-usd") {
		t.Errorf("expected --max-budget-usd in args, got stdout:\n%s", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "5.00") {
		t.Errorf("expected 5.00 in args, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_BuildArgs_WithAllowedTools(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, `"allowedTools":["Read","Write"]`, "test prompt")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--allowedTools") {
		t.Errorf("expected --allowedTools in args, got stdout:\n%s", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "Read,Write") {
		t.Errorf("expected Read,Write in args, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_BuildArgs_SkipPermissionsFalse(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, `"skipPermissions":false`, "test prompt")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Contains(result.Stdout, "--dangerously-skip-permissions") {
		t.Errorf("--dangerously-skip-permissions should NOT be present when skipPermissions=false, stdout:\n%s", result.Stdout)
	}
}

func TestExecute_AppendSystemPromptUsed(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, "", "test prompt")
	input.Agent.SystemPrompt = "Be helpful"

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--append-system-prompt") {
		t.Errorf("expected --append-system-prompt in args, got stdout:\n%s", result.Stdout)
	}
	// Ensure we do NOT use --system-prompt (the plain version without "append-")
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "--system-prompt" {
			t.Errorf("should use --append-system-prompt, not --system-prompt")
		}
	}
}

// --- Task 04 tests: Execute behavior ---

func TestExecute_SuccessfulRun(t *testing.T) {
	script := `
echo '{"type":"system","subtype":"init","session_id":"sess-test-001","model":"claude-sonnet-4-6"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Hello!"}]}}'
echo '{"type":"result","subtype":"success","session_id":"sess-test-001","total_cost_usd":0.086,"usage":{"input_tokens":5000,"output_tokens":1200}}'
`
	input := makeInputWithScript(t, script, "", "say hello")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != adapter.RunStatusSucceeded {
		t.Errorf("Status = %q, want %q", result.Status, adapter.RunStatusSucceeded)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Usage.InputTokens != 5000 {
		t.Errorf("Usage.InputTokens = %d, want 5000", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 1200 {
		t.Errorf("Usage.OutputTokens = %d, want 1200", result.Usage.OutputTokens)
	}
	if result.SessionState != "sess-test-001" {
		t.Errorf("SessionState = %q, want %q", result.SessionState, "sess-test-001")
	}
}

func TestExecute_NonZeroExit(t *testing.T) {
	input := makeInputWithScript(t, `exit 1`, "", "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != adapter.RunStatusFailed {
		t.Errorf("Status = %q, want %q", result.Status, adapter.RunStatusFailed)
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestExecute_Timeout(t *testing.T) {
	script := `sleep 60`
	scriptPath := writeTempScript(t, script)
	cfgJSON := fmt.Sprintf(`{"claudePath":%q,"timeoutSeconds":1}`, scriptPath)
	input := makeInput(cfgJSON, "test")

	a := claude.New()
	start := time.Now()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != adapter.RunStatusTimedOut {
		t.Errorf("Status = %q, want %q", result.Status, adapter.RunStatusTimedOut)
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout took %v, expected < 5s", elapsed)
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	script := `sleep 60`
	scriptPath := writeTempScript(t, script)
	cfgJSON := fmt.Sprintf(`{"claudePath":%q,"timeoutSeconds":60}`, scriptPath)
	input := makeInput(cfgJSON, "test")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	a := claude.New()
	start := time.Now()
	result, err := a.Execute(ctx, input, adapter.Hooks{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != adapter.RunStatusCancelled {
		t.Errorf("Status = %q, want %q", result.Status, adapter.RunStatusCancelled)
	}
	if elapsed > 3*time.Second {
		t.Errorf("cancellation took %v, expected < 3s", elapsed)
	}
}

func TestExecute_EnvVarsInjected(t *testing.T) {
	script := `echo "$ARI_AGENT_ID"`
	input := makeInputWithScript(t, script, "", "test")
	input.EnvVars = map[string]string{"ARI_AGENT_ID": "test-agent-123"}

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "test-agent-123") {
		t.Errorf("expected ARI_AGENT_ID in stdout, got:\n%s", result.Stdout)
	}
}

func TestExecute_ApiKeyNotInArgs(t *testing.T) {
	script := `echo "ARGS: $0 $@"`
	input := makeInputWithScript(t, script, "", "test")
	input.EnvVars = map[string]string{"ARI_API_KEY": "secret-key-value"}

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Contains(result.Stdout, "secret-key-value") {
		t.Errorf("ARI_API_KEY should not appear in args, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_WorkingDirSet(t *testing.T) {
	script := `pwd`
	scriptPath := writeTempScript(t, script)
	cfgJSON := fmt.Sprintf(`{"claudePath":%q,"timeoutSeconds":10,"workingDir":"/tmp"}`, scriptPath)
	input := makeInput(cfgJSON, "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// On macOS, /tmp is a symlink to /private/tmp
	if !strings.Contains(result.Stdout, "/tmp") {
		t.Errorf("expected /tmp in stdout, got:\n%s", result.Stdout)
	}
}

func TestExecute_WorkingDirRelativeRejected(t *testing.T) {
	script := `echo ok`
	scriptPath := writeTempScript(t, script)
	cfgJSON := fmt.Sprintf(`{"claudePath":%q,"timeoutSeconds":10,"workingDir":"./relative"}`, scriptPath)
	input := makeInput(cfgJSON, "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err == nil {
		t.Fatal("expected error for relative workingDir, got nil")
	}
	if result.Status != adapter.RunStatusFailed {
		t.Errorf("Status = %q, want %q", result.Status, adapter.RunStatusFailed)
	}
}

func TestExecute_StdoutExcerptTruncated(t *testing.T) {
	// Emit more than maxExcerptBytes (set to 128 for this test)
	script := `head -c 500 /dev/zero | tr '\0' 'A'`
	scriptPath := writeTempScript(t, script)
	cfgJSON := fmt.Sprintf(`{"claudePath":%q,"timeoutSeconds":10,"maxExcerptBytes":128}`, scriptPath)
	input := makeInput(cfgJSON, "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(result.Stdout) > 128 {
		t.Errorf("Stdout length = %d, expected <= 128", len(result.Stdout))
	}
}

func TestExecute_HooksOnLogLineCalled(t *testing.T) {
	script := `
echo '{"type":"system","subtype":"init","session_id":"s1","model":"sonnet"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}'
echo '{"type":"result","subtype":"success","total_cost_usd":0.01,"usage":{"input_tokens":100,"output_tokens":50}}'
`
	input := makeInputWithScript(t, script, "", "test")

	var mu sync.Mutex
	var logLines []adapter.LogLine

	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			mu.Lock()
			logLines = append(logLines, line)
			mu.Unlock()
		},
	}

	a := claude.New()
	_, err := a.Execute(context.Background(), input, hooks)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	mu.Lock()
	count := len(logLines)
	mu.Unlock()

	if count < 3 {
		t.Errorf("expected at least 3 log lines, got %d", count)
	}
}

func TestExecute_HooksOnStatusChangeCalled(t *testing.T) {
	script := `echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/src/main.go"}}]}}'`
	input := makeInputWithScript(t, script, "", "test")

	var mu sync.Mutex
	var statusChanges []string

	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {},
		OnStatusChange: func(detail string) {
			mu.Lock()
			statusChanges = append(statusChanges, detail)
			mu.Unlock()
		},
	}

	a := claude.New()
	_, err := a.Execute(context.Background(), input, hooks)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(statusChanges) == 0 {
		t.Fatal("expected at least one status change, got none")
	}

	found := false
	for _, s := range statusChanges {
		if strings.Contains(s, "tool:Read") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected status change containing 'tool:Read', got: %v", statusChanges)
	}
}

func TestExecute_ConcurrentExecutions(t *testing.T) {
	script := `echo '{"type":"result","subtype":"success","total_cost_usd":0.01,"usage":{"input_tokens":10,"output_tokens":5}}'`
	input := makeInputWithScript(t, script, "", "test")

	a := claude.New()
	var wg sync.WaitGroup
	errs := make(chan error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := a.Execute(context.Background(), input, adapter.Hooks{})
			if err != nil {
				errs <- fmt.Errorf("Execute error: %v", err)
				return
			}
			if result.Status != adapter.RunStatusSucceeded {
				errs <- fmt.Errorf("Status = %q, want succeeded", result.Status)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

func TestExecute_NoSessionPersistenceNotPresent(t *testing.T) {
	// --no-session-persistence was removed to allow session resume to work.
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, "", "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Contains(result.Stdout, "--no-session-persistence") {
		t.Errorf("--no-session-persistence should NOT be in args, got stdout:\n%s", result.Stdout)
	}
}

// --- Task 05 tests: Session Resume ---

func TestExecute_SessionResume_Success(t *testing.T) {
	script := `
for arg in "$@"; do
  if [ "$arg" = "--resume" ]; then
    echo "RESUME_FOUND"
  fi
done
echo '{"type":"system","subtype":"init","session_id":"sess-resumed","model":"sonnet"}'
echo '{"type":"result","subtype":"success","session_id":"sess-resumed","total_cost_usd":0.01,"usage":{"input_tokens":100,"output_tokens":50}}'
`
	input := makeInputWithScript(t, script, "", "test")
	input.Run.SessionState = "sess-abc"

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != adapter.RunStatusSucceeded {
		t.Errorf("Status = %q, want %q", result.Status, adapter.RunStatusSucceeded)
	}
	if !strings.Contains(result.Stdout, "RESUME_FOUND") {
		t.Errorf("expected --resume flag to be passed, stdout:\n%s", result.Stdout)
	}
}

func TestExecute_SessionResume_Fallback(t *testing.T) {
	// Now only retries on unknown session errors, not generic failures
	script := `
HAS_RESUME=0
for arg in "$@"; do
  if [ "$arg" = "--resume" ]; then
    HAS_RESUME=1
  fi
done
if [ "$HAS_RESUME" = "1" ]; then
  echo "no conversation found with session id sess-old" >&2
  exit 1
fi
echo '{"type":"system","subtype":"init","session_id":"sess-fresh","model":"sonnet"}'
echo '{"type":"result","subtype":"success","session_id":"sess-fresh","total_cost_usd":0.02,"usage":{"input_tokens":200,"output_tokens":100}}'
`
	input := makeInputWithScript(t, script, "", "test")
	input.Run.SessionState = "sess-old"

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != adapter.RunStatusSucceeded {
		t.Errorf("Status = %q, want %q; stderr: %s", result.Status, adapter.RunStatusSucceeded, result.Stderr)
	}
	if result.SessionState != "sess-fresh" {
		t.Errorf("SessionState = %q, want %q", result.SessionState, "sess-fresh")
	}
}

func TestExecute_SessionResume_FallbackLogsWarning(t *testing.T) {
	script := `
HAS_RESUME=0
for arg in "$@"; do
  if [ "$arg" = "--resume" ]; then
    HAS_RESUME=1
  fi
done
if [ "$HAS_RESUME" = "1" ]; then
  echo "unknown session: sess-expired" >&2
  exit 1
fi
echo '{"type":"result","subtype":"success","total_cost_usd":0.01,"usage":{"input_tokens":10,"output_tokens":5}}'
`
	input := makeInputWithScript(t, script, "", "test")
	input.Run.SessionState = "sess-expired"

	var mu sync.Mutex
	var logLines []adapter.LogLine

	hooks := adapter.Hooks{
		OnLogLine: func(line adapter.LogLine) {
			mu.Lock()
			logLines = append(logLines, line)
			mu.Unlock()
		},
	}

	a := claude.New()
	result, err := a.Execute(context.Background(), input, hooks)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != adapter.RunStatusSucceeded {
		t.Errorf("Status = %q, want %q", result.Status, adapter.RunStatusSucceeded)
	}

	mu.Lock()
	defer mu.Unlock()

	foundWarning := false
	for _, line := range logLines {
		if line.Level == "warn" && strings.Contains(line.Message, "unknown session error") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected warning log about unknown session error, got none")
	}
}

func TestExecute_DisableResumeOnError(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, `"disableResumeOnError":true`, "test")
	input.Run.SessionState = "sess-abc"

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Contains(result.Stdout, "--resume") {
		t.Errorf("--resume should NOT be in args when disableResumeOnError=true, got stdout:\n%s", result.Stdout)
	}
}

// --- Task 2: Stdin delivery ---

func TestExecute_StdinDelivery(t *testing.T) {
	// Script reads from stdin instead of args
	script := `cat`
	input := makeInputWithScript(t, script, "", "hello from stdin")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "hello from stdin") {
		t.Errorf("expected prompt in stdout via stdin, got:\n%s", result.Stdout)
	}
}

func TestExecute_VerboseAlwaysPresent(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, "", "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--verbose") {
		t.Errorf("expected --verbose in args, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_PrintDashInArgs(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, "", "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	lines := strings.Split(result.Stdout, "\n")
	foundPrint := false
	for i, line := range lines {
		if strings.TrimSpace(line) == "--print" && i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "-" {
			foundPrint = true
			break
		}
	}
	if !foundPrint {
		t.Errorf("expected --print - in args, got stdout:\n%s", result.Stdout)
	}
}

// --- Task 3: Nesting prevention ---

func TestExecute_NestingVarsStripped(t *testing.T) {
	script := `echo "CC=${CLAUDE_CODE_TEST:-unset}" && echo "CE=${CLAUDECODE:-unset}"`
	input := makeInputWithScript(t, script, "", "test")

	// Set nesting vars
	t.Setenv("CLAUDE_CODE_TEST", "should-be-stripped")
	t.Setenv("CLAUDECODE", "should-be-stripped")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Contains(result.Stdout, "should-be-stripped") {
		t.Errorf("CLAUDE_CODE_* and CLAUDECODE should be stripped, got stdout:\n%s", result.Stdout)
	}
}

// --- Task 7: CLI flags ---

func TestExecute_EffortFlag(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, `"effort":"low"`, "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--effort") || !strings.Contains(result.Stdout, "low") {
		t.Errorf("expected --effort low in args, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_ChromeFlag(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, `"chrome":true`, "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--chrome") {
		t.Errorf("expected --chrome in args, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_MaxTurnsFlag(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, `"maxTurnsPerRun":25`, "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--max-turns") || !strings.Contains(result.Stdout, "25") {
		t.Errorf("expected --max-turns 25 in args, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_ExtraArgs(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, `"extraArgs":["--custom-flag","value"]`, "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(result.Stdout, "--custom-flag") || !strings.Contains(result.Stdout, "value") {
		t.Errorf("expected extra args in output, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_NoEffortWhenEmpty(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, "", "test")

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Contains(result.Stdout, "--effort") {
		t.Errorf("--effort should NOT be in args when not configured, got stdout:\n%s", result.Stdout)
	}
}

func TestExecute_NoSessionState_NoResumeFlag(t *testing.T) {
	script := `for arg in "$@"; do echo "$arg"; done`
	input := makeInputWithScript(t, script, "", "test")
	// No SessionState set

	a := claude.New()
	result, err := a.Execute(context.Background(), input, adapter.Hooks{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Contains(result.Stdout, "--resume") {
		t.Errorf("--resume should NOT be in args when no session state, got stdout:\n%s", result.Stdout)
	}
}
