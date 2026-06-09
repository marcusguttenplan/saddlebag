// Package gateway implements the Packmule model gateway — a localhost
// OpenAI-compatible HTTP proxy with hierarchical routing, semantic normalization
// across providers, token ledger, budget enforcement, and W3C trace propagation.
package gateway

import "encoding/json"

// ---------------------------------------------------------------------------
// OpenAI-compatible request/response types
// These are the canonical types that all provider adapters normalize to/from.
// ---------------------------------------------------------------------------

// ChatRequest is an OpenAI-compatible /v1/chat/completions request body.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`

	// Optional generation parameters
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`

	// Tools / function calling
	Tools      []Tool  `json:"tools,omitempty"`
	ToolChoice any     `json:"tool_choice,omitempty"`

	// Packmule extensions (passed in headers or request body; stripped before forwarding)
	// XTaskType is the routing task type ("plan", "implement", "classify", "review", "verify").
	XTaskType string `json:"x_task_type,omitempty"`
	// XModel is an explicit provider/model override ("openai/gpt-4o").
	XModel string `json:"x_model,omitempty"`
}

// Message is a single turn in a conversation.
type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`

	// ToolCalls is populated on assistant messages that invoke tools.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID is set on tool-result messages.
	ToolCallID string `json:"tool_call_id,omitempty"`

	// Name is set on tool-result messages.
	Name string `json:"name,omitempty"`
}

// ContentString returns the message content as a plain string.
// Returns empty string if content is a structured array.
func (m *Message) ContentString() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	return string(m.Content)
}

// Tool represents an OpenAI-format function tool definition.
type Tool struct {
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a single callable function.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolCall is an assistant's request to call a tool.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the function name and arguments for a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

// ChatResponse is an OpenAI-compatible non-streaming response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"` // "chat.completion"
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice is a single completion choice.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage holds token counts for a completed call.
type Usage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	// Anthropic cache fields (present when using prompt caching)
	CacheReadInputTokens  int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// ---------------------------------------------------------------------------
// Streaming types (SSE)
// ---------------------------------------------------------------------------

// StreamChunk is an OpenAI-compatible streaming delta event.
type StreamChunk struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"` // "chat.completion.chunk"
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []StreamChoice  `json:"choices"`
	Usage   *Usage          `json:"usage,omitempty"` // only on final chunk
}

// StreamChoice is a single streaming choice delta.
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// StreamDelta holds the incremental content for a streaming turn.
type StreamDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

// ErrorResponse is an OpenAI-compatible error response body.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail holds the error message and type.
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}
