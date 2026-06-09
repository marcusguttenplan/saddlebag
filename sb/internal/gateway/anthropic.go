package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const anthropicAPIBase = "https://api.anthropic.com"
const anthropicVersion = "2023-06-01"

// anthropicProvider implements Provider for Anthropic's Claude models.
type anthropicProvider struct {
	apiKey     string
	httpClient *http.Client
}

func newAnthropicProvider(apiKey string) *anthropicProvider {
	return &anthropicProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (p *anthropicProvider) Name() string { return "anthropic" }

// ---------------------------------------------------------------------------
// Anthropic-native request/response types
// ---------------------------------------------------------------------------

type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    string              `json:"system,omitempty"`
	Messages  []anthropicMessage  `json:"messages"`
	Tools     []anthropicTool     `json:"tools,omitempty"`
	Stream    bool                `json:"stream,omitempty"`
	// Anthropic thinking (extended reasoning)
	Thinking  *anthropicThinking  `json:"thinking,omitempty"`
}

type anthropicThinking struct {
	Type         string `json:"type"` // "enabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	// For tool_use
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	// For tool_result
	ToolUseID string             `json:"tool_use_id,omitempty"`
	Content   []anthropicContent `json:"content,omitempty"`
	// For image
	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/jpeg" etc.
	Data      string `json:"data"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"` // "message"
	Role         string             `json:"role"` // "assistant"
	Content      []anthropicContent `json:"content"`
	Model        string             `json:"model"`
	StopReason   string             `json:"stop_reason"`
	Usage        anthropicUsage     `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// Anthropic streaming event types
type anthropicStreamEvent struct {
	Type  string          `json:"type"`
	Index int             `json:"index"`
	Delta *anthropicDelta `json:"delta,omitempty"`
	Usage *anthropicUsage `json:"usage,omitempty"`
	// For content_block_start
	ContentBlock *anthropicContent `json:"content_block,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// ---------------------------------------------------------------------------
// Chat (non-streaming)
// ---------------------------------------------------------------------------

func (p *anthropicProvider) Chat(ctx context.Context, model string, req *ChatRequest) (*ChatResponse, error) {
	aReq, err := toAnthropicRequest(model, req, false)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(aReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		anthropicAPIBase+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{StatusCode: 0, Message: err.Error(), Provider: "anthropic"}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
			Provider:   "anthropic",
		}
	}

	var aResp anthropicResponse
	if err := json.Unmarshal(respBody, &aResp); err != nil {
		return nil, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	return fromAnthropicResponse(&aResp, model), nil
}

// ---------------------------------------------------------------------------
// ChatStream (streaming)
// ---------------------------------------------------------------------------

func (p *anthropicProvider) ChatStream(ctx context.Context, model string, req *ChatRequest, w io.Writer) (*Usage, error) {
	aReq, err := toAnthropicRequest(model, req, true)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(aReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		anthropicAPIBase+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{StatusCode: 0, Message: err.Error(), Provider: "anthropic"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    string(b),
			Provider:   "anthropic",
		}
	}

	return p.streamToOpenAI(ctx, resp.Body, model, w)
}

// streamToOpenAI reads Anthropic SSE events and translates them to OpenAI-format SSE.
func (p *anthropicProvider) streamToOpenAI(ctx context.Context, body io.Reader, model string, w io.Writer) (*Usage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	var finalUsage *Usage
	chunkID := fmt.Sprintf("chatcmpl-anthropic-%d", time.Now().UnixNano())

	// Track multi-block content (text + tool_use blocks)
	blockTypes := make(map[int]string) // index → "text" | "tool_use"
	toolIDs := make(map[int]string)    // index → tool call ID
	toolNames := make(map[int]string)  // index → tool name

	var eventType string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return finalUsage, ctx.Err()
		default:
		}

		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		switch eventType {
		case "content_block_start":
			var ev anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			if ev.ContentBlock != nil {
				blockTypes[ev.Index] = ev.ContentBlock.Type
				if ev.ContentBlock.Type == "tool_use" {
					toolIDs[ev.Index] = ev.ContentBlock.ID
					toolNames[ev.Index] = ev.ContentBlock.Name
					// Emit a delta with the tool_call start
					chunk := openAIChunkForToolStart(chunkID, model, ev.Index, ev.ContentBlock.ID, ev.ContentBlock.Name)
					writeSSEChunk(w, chunk)
				}
			}

		case "content_block_delta":
			var ev anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			if ev.Delta == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				chunk := openAIChunkForText(chunkID, model, ev.Delta.Text)
				writeSSEChunk(w, chunk)
			case "input_json_delta":
				chunk := openAIChunkForToolArgs(chunkID, model, ev.Index, ev.Delta.PartialJSON)
				writeSSEChunk(w, chunk)
			}

		case "message_delta":
			var ev anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			if ev.Usage != nil {
				finalUsage = &Usage{
					CompletionTokens: ev.Usage.OutputTokens,
				}
			}

		case "message_start":
			// Contains initial usage (input tokens)
			var ev struct {
				Message struct {
					Usage anthropicUsage `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			if finalUsage == nil {
				finalUsage = &Usage{}
			}
			finalUsage.PromptTokens = ev.Message.Usage.InputTokens
			finalUsage.CacheReadInputTokens = ev.Message.Usage.CacheReadInputTokens
			finalUsage.CacheCreationInputTokens = ev.Message.Usage.CacheCreationInputTokens

		case "message_stop":
			// Emit [DONE]
			fmt.Fprint(w, "data: [DONE]\n\n")
			if finalUsage != nil {
				finalUsage.TotalTokens = finalUsage.PromptTokens + finalUsage.CompletionTokens
			}
			return finalUsage, nil
		}

		_ = blockTypes
		_ = toolIDs
		_ = toolNames
	}

	if err := scanner.Err(); err != nil {
		return finalUsage, fmt.Errorf("anthropic: stream read: %w", err)
	}

	// Ensure [DONE] is sent even if message_stop event was missing
	fmt.Fprint(w, "data: [DONE]\n\n")
	return finalUsage, nil
}

// ---------------------------------------------------------------------------
// Normalization helpers
// ---------------------------------------------------------------------------

// toAnthropicRequest converts an OpenAI-format ChatRequest to Anthropic's format.
func toAnthropicRequest(model string, req *ChatRequest, stream bool) (*anthropicRequest, error) {
	aReq := &anthropicRequest{
		Model:     model,
		MaxTokens: 8192, // Anthropic requires max_tokens
		Stream:    stream,
	}
	if req.MaxTokens != nil {
		aReq.MaxTokens = *req.MaxTokens
	}

	// Extract system prompt (Anthropic uses a top-level field, not a message role)
	var messages []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == "system" {
			aReq.System = m.ContentString()
			continue
		}
		am, err := toAnthropicMessage(m)
		if err != nil {
			return nil, err
		}
		messages = append(messages, am)
	}
	aReq.Messages = messages

	// Convert tools
	for _, t := range req.Tools {
		schema := t.Function.Parameters
		if schema == nil {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		aReq.Tools = append(aReq.Tools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: schema,
		})
	}

	return aReq, nil
}

// toAnthropicMessage converts an OpenAI Message to Anthropic's format.
func toAnthropicMessage(m Message) (anthropicMessage, error) {
	am := anthropicMessage{Role: m.Role}

	// Handle tool result messages (role="tool" in OpenAI)
	if m.Role == "tool" {
		am.Role = "user"
		am.Content = []anthropicContent{{
			Type:      "tool_result",
			ToolUseID: m.ToolCallID,
			Content:   []anthropicContent{{Type: "text", Text: m.ContentString()}},
		}}
		return am, nil
	}

	// Handle assistant messages with tool calls
	if m.Role == "assistant" && len(m.ToolCalls) > 0 {
		// Add text content if present
		if text := m.ContentString(); text != "" {
			am.Content = append(am.Content, anthropicContent{Type: "text", Text: text})
		}
		for _, tc := range m.ToolCalls {
			var input json.RawMessage
			if tc.Function.Arguments != "" {
				input = json.RawMessage(tc.Function.Arguments)
			} else {
				input = json.RawMessage("{}")
			}
			am.Content = append(am.Content, anthropicContent{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
		return am, nil
	}

	// Standard text message
	am.Content = []anthropicContent{{Type: "text", Text: m.ContentString()}}
	return am, nil
}

// fromAnthropicResponse converts an Anthropic response to OpenAI format.
func fromAnthropicResponse(aResp *anthropicResponse, model string) *ChatResponse {
	msg := Message{Role: "assistant"}

	// Collect text and tool_use blocks
	var textParts []string
	var toolCalls []ToolCall

	for _, c := range aResp.Content {
		switch c.Type {
		case "text":
			textParts = append(textParts, c.Text)
		case "tool_use":
			var args string
			if c.Input != nil {
				args = string(c.Input)
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      c.Name,
					Arguments: args,
				},
			})
		}
	}

	if len(textParts) > 0 {
		b, _ := json.Marshal(strings.Join(textParts, ""))
		msg.Content = b
	}
	msg.ToolCalls = toolCalls

	finishReason := "stop"
	if aResp.StopReason == "tool_use" {
		finishReason = "tool_calls"
	}

	return &ChatResponse{
		ID:     aResp.ID,
		Object: "chat.completion",
		Model:  model,
		Choices: []Choice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: &Usage{
			PromptTokens:                 aResp.Usage.InputTokens,
			CompletionTokens:             aResp.Usage.OutputTokens,
			TotalTokens:                  aResp.Usage.InputTokens + aResp.Usage.OutputTokens,
			CacheReadInputTokens:         aResp.Usage.CacheReadInputTokens,
			CacheCreationInputTokens:     aResp.Usage.CacheCreationInputTokens,
		},
	}
}

// ---------------------------------------------------------------------------
// SSE chunk helpers
// ---------------------------------------------------------------------------

func openAIChunkForText(id, model, text string) StreamChunk {
	return StreamChunk{
		ID:     id,
		Object: "chat.completion.chunk",
		Model:  model,
		Choices: []StreamChoice{{
			Index: 0,
			Delta: StreamDelta{Content: text},
		}},
	}
}

func openAIChunkForToolStart(id, model string, index int, toolID, toolName string) StreamChunk {
	return StreamChunk{
		ID:     id,
		Object: "chat.completion.chunk",
		Model:  model,
		Choices: []StreamChoice{{
			Index: 0,
			Delta: StreamDelta{
				ToolCalls: []ToolCall{{
					ID:   toolID,
					Type: "function",
					Function: ToolCallFunction{Name: toolName},
				}},
			},
		}},
	}
}

func openAIChunkForToolArgs(id, model string, index int, partial string) StreamChunk {
	return StreamChunk{
		ID:     id,
		Object: "chat.completion.chunk",
		Model:  model,
		Choices: []StreamChoice{{
			Index: 0,
			Delta: StreamDelta{
				ToolCalls: []ToolCall{{
					Function: ToolCallFunction{Arguments: partial},
				}},
			},
		}},
	}
}

func writeSSEChunk(w io.Writer, chunk StreamChunk) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func (p *anthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")
}
