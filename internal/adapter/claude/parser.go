// Package claude implements the "claude_local" adapter which spawns Claude Code CLI as a subprocess.
package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/xb/ari/internal/adapter"
)

// claudeEvent represents a single stream-json event from the Claude CLI.
// The Type field is the discriminator; other fields are populated based on the event type.
type claudeEvent struct {
	Type    string `json:"type"`    // "system", "assistant", "tool_result", "result", "rate_limit_event"
	Subtype string `json:"subtype"` // e.g., "init" for system, "success"/"error" for result

	// system/init fields
	SessionID string `json:"session_id"`
	Model     string `json:"model"`

	// assistant fields
	Message *claudeMessage `json:"message"`

	// tool_result fields
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`

	// result fields
	TotalCostUSD float64                    `json:"total_cost_usd"`
	Usage        *claudeUsage               `json:"usage"`
	ModelUsage   map[string]*claudeModelUse `json:"modelUsage"`

	// rate_limit_event fields
	RateLimitInfo *claudeRateLimitInfo `json:"rate_limit_info"`
}

// claudeMessage represents the message field in an assistant event.
type claudeMessage struct {
	Content []claudeContentBlock `json:"content"`
	Usage   *claudeUsage         `json:"usage"`
}

// claudeContentBlock represents a single content block in an assistant message.
type claudeContentBlock struct {
	Type  string         `json:"type"`  // "text" or "tool_use"
	Text  string         `json:"text"`  // for type == "text"
	ID    string         `json:"id"`    // for type == "tool_use"
	Name  string         `json:"name"`  // for type == "tool_use"
	Input map[string]any `json:"input"` // for type == "tool_use"
}

// claudeUsage represents token usage in stream-json events.
type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// claudeModelUse represents per-model usage in the result event.
type claudeModelUse struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUSD      float64 `json:"costUSD"`
}

// claudeRateLimitInfo contains rate limit details from a rate_limit_event.
type claudeRateLimitInfo struct {
	RetryAfterMs int `json:"retryAfterMs"`
}

// eventCollector accumulates key events during streaming for post-run extraction.
type eventCollector struct {
	sessionID   string
	resultEvent *claudeEvent
}

// extract returns token usage and session state from collected events.
func (ec *eventCollector) extract() (adapter.TokenUsage, string) {
	usage := adapter.TokenUsage{Provider: "anthropic"}

	if ec.resultEvent != nil {
		if ec.resultEvent.Usage != nil {
			usage.InputTokens = ec.resultEvent.Usage.InputTokens
			usage.OutputTokens = ec.resultEvent.Usage.OutputTokens
		}
		usage.Model = ec.resultEvent.Model
		// Session ID from result event takes precedence (final confirmation)
		if ec.resultEvent.SessionID != "" {
			ec.sessionID = ec.resultEvent.SessionID
		}
	}

	return usage, ec.sessionID
}

// streamAndParseEvents reads stdout line by line, parsing each as a stream-json event,
// forwarding structured log lines via hooks, and collecting key events.
func streamAndParseEvents(r io.Reader, buf *bytes.Buffer, maxBytes int, hooks adapter.Hooks, collector *eventCollector) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Accumulate into bounded buffer
		if buf.Len() < maxBytes {
			remaining := maxBytes - buf.Len()
			if len(line)+1 > remaining {
				buf.WriteString(line[:remaining])
			} else {
				buf.WriteString(line)
				buf.WriteByte('\n')
			}
		}

		// Parse the stream-json event
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "{") {
			// Not a JSON line — forward as plain text
			if hooks.OnLogLine != nil {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "info",
					Message:   line,
					Timestamp: time.Now(),
				})
			}
			continue
		}

		var event claudeEvent
		if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
			// Malformed JSON — skip event, log warning (REQ-CLA-031)
			if hooks.OnLogLine != nil {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "warn",
					Message:   fmt.Sprintf("failed to parse stream-json event: %s", line),
					Timestamp: time.Now(),
				})
			}
			continue
		}

		// Process event by type
		switch event.Type {
		case "system":
			if event.Subtype == "init" && event.SessionID != "" {
				collector.sessionID = event.SessionID
			}
			if hooks.OnLogLine != nil {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "info",
					Message:   fmt.Sprintf("session initialized (model: %s)", event.Model),
					Timestamp: time.Now(),
					Fields:    map[string]any{"eventType": "system", "sessionId": event.SessionID},
				})
			}

		case "assistant":
			if event.Message != nil {
				for _, block := range event.Message.Content {
					switch block.Type {
					case "tool_use":
						// Extract tool call into structured fields (REQ-CLA-009)
						fields := map[string]any{
							"eventType": "assistant",
							"toolName":  block.Name,
							"toolInput": block.Input,
						}
						if hooks.OnLogLine != nil {
							hooks.OnLogLine(adapter.LogLine{
								Level:     "info",
								Message:   fmt.Sprintf("Tool: %s", block.Name),
								Timestamp: time.Now(),
								Fields:    fields,
							})
						}
						// Update status for UI (REQ-CLA-023)
						if hooks.OnStatusChange != nil {
							detail := fmt.Sprintf("tool:%s", block.Name)
							if filePath, ok := block.Input["file_path"].(string); ok {
								detail = fmt.Sprintf("tool:%s %s", block.Name, filePath)
							} else if cmd, ok := block.Input["command"].(string); ok {
								if len(cmd) > 50 {
									cmd = cmd[:50] + "..."
								}
								detail = fmt.Sprintf("tool:%s %s", block.Name, cmd)
							}
							hooks.OnStatusChange(detail)
						}
					case "text":
						if hooks.OnLogLine != nil {
							hooks.OnLogLine(adapter.LogLine{
								Level:     "info",
								Message:   block.Text,
								Timestamp: time.Now(),
								Fields:    map[string]any{"eventType": "assistant"},
							})
						}
					}
				}
			}

		case "tool_result":
			if hooks.OnLogLine != nil {
				// Truncate large tool results for the log line
				content := event.Content
				if len(content) > 200 {
					content = content[:200] + "...(truncated)"
				}
				hooks.OnLogLine(adapter.LogLine{
					Level:     "debug",
					Message:   fmt.Sprintf("Tool result [%s]: %s", event.ToolUseID, content),
					Timestamp: time.Now(),
					Fields:    map[string]any{"eventType": "tool_result", "toolUseId": event.ToolUseID},
				})
			}

		case "result":
			collector.resultEvent = &event
			if hooks.OnLogLine != nil {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "info",
					Message:   fmt.Sprintf("Run complete (cost: $%.4f)", event.TotalCostUSD),
					Timestamp: time.Now(),
					Fields: map[string]any{
						"eventType":    "result",
						"totalCostUSD": event.TotalCostUSD,
					},
				})
			}

		case "rate_limit_event":
			// Primary rate-limit detection via stream-json (REQ-CLA-020)
			if hooks.OnLogLine != nil {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "warn",
					Message:   "Rate limit encountered",
					Timestamp: time.Now(),
					Fields:    map[string]any{"rateLimited": true, "eventType": "rate_limit_event"},
				})
			}

		default:
			// Unknown event type — forward as debug
			if hooks.OnLogLine != nil {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "debug",
					Message:   line,
					Timestamp: time.Now(),
					Fields:    map[string]any{"eventType": event.Type},
				})
			}
		}
	}
}

// streamStderr reads stderr, accumulates into buffer, and detects rate-limit errors.
// This is a fallback — primary rate-limit detection is via stream-json rate_limit_event.
func streamStderr(r io.Reader, buf *bytes.Buffer, maxBytes int, hooks adapter.Hooks) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if buf.Len() < maxBytes {
			remaining := maxBytes - buf.Len()
			if len(line)+1 > remaining {
				buf.WriteString(line[:remaining])
			} else {
				buf.WriteString(line)
				buf.WriteByte('\n')
			}
		}

		// Detect rate limiting in stderr as fallback (REQ-CLA-020)
		if hooks.OnLogLine != nil {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "rate limit") || strings.Contains(lower, "429") {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "warn",
					Message:   line,
					Timestamp: time.Now(),
					Fields:    map[string]any{"rateLimited": true, "source": "stderr"},
				})
			} else {
				hooks.OnLogLine(adapter.LogLine{
					Level:     "error",
					Message:   line,
					Timestamp: time.Now(),
				})
			}
		}
	}
}
