// Package adapter defines the agent execution interface and supporting types.
package adapter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TestLevel controls how thorough the environment check is.
type TestLevel int

const (
	TestLevelBasic TestLevel = iota // fast: check binary/URL exists
	TestLevelFull                   // slow: attempt a no-op invocation
)

// TestResult reports the outcome of TestEnvironment.
type TestResult struct {
	Available bool
	Message   string
}

// RunStatus mirrors domain.RunStatus — kept here to avoid import cycles.
type RunStatus string

const (
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
	RunStatusTimedOut  RunStatus = "timed_out"
)

// ModelDefinition describes an AI model available through an adapter.
type ModelDefinition struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// AgentContext carries agent-level data into an invocation.
type AgentContext struct {
	ID            uuid.UUID       `json:"id"`
	Name          string          `json:"name"`
	ShortName     string          `json:"shortName"`
	Role          string          `json:"role"`
	AdapterConfig json.RawMessage `json:"adapterConfig"`
	SystemPrompt  string          `json:"systemPrompt"`
	Model         string          `json:"model"`
}

// SquadContext carries squad-level data into an invocation.
type SquadContext struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// RunContext carries per-invocation context.
type RunContext struct {
	RunID        uuid.UUID  `json:"runId"`
	WakeReason   string     `json:"wakeReason"` // "on_demand" | "timer" | "assignment" | "inbox_resolved" | "conversation_message"
	TaskID       *uuid.UUID `json:"taskId,omitempty"`
	SessionState string     `json:"sessionState,omitempty"` // sessionIdBefore — opaque blob from previous run
}

// CommentEntry is a single message in a conversation thread.
type CommentEntry struct {
	ID         uuid.UUID `json:"id"`
	AuthorType string    `json:"authorType"` // "agent" | "user" | "system"
	AuthorID   uuid.UUID `json:"authorId"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ConversationContext is non-nil when WakeReason == "conversation_message".
type ConversationContext struct {
	IssueID      uuid.UUID      `json:"issueId"`
	Messages     []CommentEntry `json:"messages"`
	SessionState string         `json:"sessionState"` // previous conversation session state
}

// InvokeInput is the full context passed to Execute().
type InvokeInput struct {
	Agent        AgentContext         `json:"agent"`
	Squad        SquadContext         `json:"squad"`
	Run          RunContext           `json:"run"`
	EnvVars      map[string]string    `json:"envVars"`
	Prompt       string               `json:"prompt"`
	Conversation *ConversationContext `json:"conversation,omitempty"`
}

// TokenUsage captures LLM usage for cost accounting.
type TokenUsage struct {
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
}

// InvokeResult is returned by Execute() when the run ends.
type InvokeResult struct {
	Status       RunStatus  `json:"status"`
	ExitCode     int        `json:"exitCode"`
	Usage        TokenUsage `json:"usage"`
	SessionState string     `json:"sessionState"` // sessionIdAfter — opaque state blob
	Stdout       string     `json:"stdout"`       // excerpt (up to MaxExcerptBytes)
	Stderr       string     `json:"stderr"`       // excerpt (up to MaxExcerptBytes)
}

// LogLine represents a single structured log line from an adapter.
type LogLine struct {
	Level     string         `json:"level"` // "debug" | "info" | "warn" | "error"
	Message   string         `json:"message"`
	Timestamp time.Time      `json:"timestamp"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// Hooks are callbacks called by the adapter during execution.
// All callbacks must be safe to call from a background goroutine.
type Hooks struct {
	// OnLogLine forwards a real-time log line for SSE streaming.
	// MUST NOT block; implementations may drop lines if the channel is full.
	OnLogLine func(line LogLine)

	// OnStatusChange signals a sub-status change (e.g., "awaiting_tool_result").
	OnStatusChange func(detail string)
}

// Adapter is the interface all agent runtime adapters must implement.
type Adapter interface {
	// Type returns the unique identifier for this adapter (matches adapter_type column).
	Type() string

	// Execute spawns/invokes the agent and blocks until the run completes
	// or the context is cancelled. Implementations must handle context cancellation
	// and return InvokeResult{Status: RunStatusCancelled} in that case.
	Execute(ctx context.Context, input InvokeInput, hooks Hooks) (InvokeResult, error)

	// TestEnvironment checks runtime prerequisites.
	// Called at startup; failures mark the adapter unavailable (REQ-049).
	TestEnvironment(level TestLevel) (TestResult, error)

	// Models returns the AI models this adapter supports.
	Models() []ModelDefinition
}
