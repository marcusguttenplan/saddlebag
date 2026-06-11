package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- toAnthropicRequest unit tests (no HTTP) ---

func TestToAnthropicRequest_SystemPrompt(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{
			strMsg("system", "be helpful"),
			strMsg("user", "hello"),
		},
	}
	aReq, err := toAnthropicRequest("claude-3-5-sonnet", req, false)
	if err != nil {
		t.Fatal(err)
	}
	if aReq.System != "be helpful" {
		t.Errorf("system: want %q, got %q", "be helpful", aReq.System)
	}
	if len(aReq.Messages) != 1 {
		t.Errorf("messages: want 1 (system stripped), got %d", len(aReq.Messages))
	}
	if aReq.Messages[0].Role != "user" {
		t.Errorf("msg[0].role: want user, got %s", aReq.Messages[0].Role)
	}
}

func TestToAnthropicRequest_ToolDefinition(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"loc":{"type":"string"}}}`)
	req := &ChatRequest{
		Messages: []Message{strMsg("user", "weather?")},
		Tools: []Tool{{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  schema,
			},
		}},
	}
	aReq, err := toAnthropicRequest("claude-3-5-sonnet", req, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(aReq.Tools) != 1 {
		t.Fatalf("tools: want 1, got %d", len(aReq.Tools))
	}
	if aReq.Tools[0].Name != "get_weather" {
		t.Errorf("tool name: want get_weather, got %s", aReq.Tools[0].Name)
	}
}

func TestToAnthropicRequest_ToolResult(t *testing.T) {
	// role=tool in OpenAI → tool_result content block in Anthropic user message
	req := &ChatRequest{
		Messages: []Message{
			strMsg("user", "call it"),
			assistantWithTools("", ToolCall{ID: "tc1", Type: "function", Function: ToolCallFunction{Name: "fn", Arguments: "{}"}}),
			toolResultMsg("tc1", "42 degrees"),
		},
	}
	aReq, err := toAnthropicRequest("claude-3-5-sonnet", req, false)
	if err != nil {
		t.Fatal(err)
	}
	// Expect 3 messages: user, assistant, user (tool_result is wrapped in user role)
	if len(aReq.Messages) != 3 {
		t.Fatalf("messages: want 3, got %d: %+v", len(aReq.Messages), aReq.Messages)
	}
	last := aReq.Messages[2]
	if last.Role != "user" {
		t.Errorf("tool result role: want user, got %s", last.Role)
	}
	if len(last.Content) == 0 || last.Content[0].Type != "tool_result" {
		t.Errorf("tool result content type: want tool_result, got %+v", last.Content)
	}
	if last.Content[0].ToolUseID != "tc1" {
		t.Errorf("tool_use_id: want tc1, got %s", last.Content[0].ToolUseID)
	}
}

func TestToAnthropicRequest_AssistantWithToolCalls(t *testing.T) {
	args := `{"location":"NYC"}`
	req := &ChatRequest{
		Messages: []Message{
			strMsg("user", "weather?"),
			assistantWithTools("", ToolCall{
				ID:   "tc1",
				Type: "function",
				Function: ToolCallFunction{Name: "get_weather", Arguments: args},
			}),
		},
	}
	aReq, err := toAnthropicRequest("claude-3-5-sonnet", req, false)
	if err != nil {
		t.Fatal(err)
	}
	assistantMsg := aReq.Messages[1]
	if assistantMsg.Role != "assistant" {
		t.Errorf("role: want assistant, got %s", assistantMsg.Role)
	}
	if len(assistantMsg.Content) == 0 || assistantMsg.Content[0].Type != "tool_use" {
		t.Errorf("content[0]: want tool_use, got %+v", assistantMsg.Content)
	}
	if assistantMsg.Content[0].ID != "tc1" {
		t.Errorf("tool id: want tc1, got %s", assistantMsg.Content[0].ID)
	}
}

func TestToAnthropicRequest_CollapseConsecutiveUser(t *testing.T) {
	// Verify that CollapseConsecutiveRoles is wired into toAnthropicRequest.
	req := &ChatRequest{
		Messages: []Message{
			strMsg("user", "first"),
			strMsg("user", "second"),
		},
	}
	aReq, err := toAnthropicRequest("claude-3-5-sonnet", req, false)
	if err != nil {
		t.Fatal(err)
	}
	// Two consecutive user messages should be collapsed to one.
	if len(aReq.Messages) != 1 {
		t.Fatalf("expected 1 message after collapse, got %d", len(aReq.Messages))
	}
}

// --- fromAnthropicResponse unit tests ---

func TestFromAnthropicResponse_Text(t *testing.T) {
	aResp := &anthropicResponse{
		ID:         "msg_001",
		StopReason: "end_turn",
		Model:      "claude-3-5-sonnet",
		Content:    []anthropicContent{{Type: "text", Text: "Hello!"}},
		Usage:      anthropicUsage{InputTokens: 10, OutputTokens: 5},
	}
	resp := fromAnthropicResponse(aResp, "claude-3-5-sonnet")
	if len(resp.Choices) != 1 {
		t.Fatalf("choices: want 1, got %d", len(resp.Choices))
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason: want stop, got %s", resp.Choices[0].FinishReason)
	}
	if resp.Choices[0].Message.ContentString() != "Hello!" {
		t.Errorf("content: want Hello!, got %s", resp.Choices[0].Message.ContentString())
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 10 {
		t.Errorf("usage: want 10 prompt tokens, got %+v", resp.Usage)
	}
}

func TestFromAnthropicResponse_SingleToolCall(t *testing.T) {
	input := json.RawMessage(`{"location":"NYC"}`)
	aResp := &anthropicResponse{
		ID:         "msg_002",
		StopReason: "tool_use",
		Model:      "claude-3-5-sonnet",
		Content: []anthropicContent{{
			Type:  "tool_use",
			ID:    "toolu_01",
			Name:  "get_weather",
			Input: input,
		}},
		Usage: anthropicUsage{InputTokens: 20, OutputTokens: 15},
	}
	resp := fromAnthropicResponse(aResp, "claude-3-5-sonnet")
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("finish_reason: want tool_calls, got %s", resp.Choices[0].FinishReason)
	}
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 {
		t.Fatalf("tool_calls: want 1, got %d", len(calls))
	}
	if calls[0].ID != "toolu_01" {
		t.Errorf("tool id: want toolu_01, got %s", calls[0].ID)
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("tool name: want get_weather, got %s", calls[0].Function.Name)
	}
}

func TestFromAnthropicResponse_MultipleToolCalls(t *testing.T) {
	aResp := &anthropicResponse{
		ID:         "msg_003",
		StopReason: "tool_use",
		Content: []anthropicContent{
			{Type: "tool_use", ID: "toolu_01", Name: "read_file", Input: json.RawMessage(`{"path":"a.go"}`)},
			{Type: "tool_use", ID: "toolu_02", Name: "list_dir", Input: json.RawMessage(`{"path":"/src"}`)},
		},
		Usage: anthropicUsage{InputTokens: 30, OutputTokens: 20},
	}
	resp := fromAnthropicResponse(aResp, "model")
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 2 {
		t.Fatalf("tool_calls: want 2, got %d", len(calls))
	}
	if calls[0].ID != "toolu_01" || calls[1].ID != "toolu_02" {
		t.Errorf("tool ids: got %s, %s", calls[0].ID, calls[1].ID)
	}
	if calls[0].Function.Name != "read_file" || calls[1].Function.Name != "list_dir" {
		t.Errorf("tool names: got %s, %s", calls[0].Function.Name, calls[1].Function.Name)
	}
}

// --- HTTP round-trip tests (httptest mock server) ---

func newTestAnthropicProvider(serverURL string) *anthropicProvider {
	return &anthropicProvider{
		apiKey:     "test-key",
		httpClient: &http.Client{},
		baseURL:    serverURL,
	}
}

func TestAnthropicProvider_Chat_TextResponse(t *testing.T) {
	respBody := `{
		"id":"msg_001","type":"message","role":"assistant",
		"content":[{"type":"text","text":"Hello from Claude"}],
		"model":"claude-3-5-sonnet","stop_reason":"end_turn",
		"usage":{"input_tokens":10,"output_tokens":4}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, respBody)
	}))
	defer srv.Close()

	p := newTestAnthropicProvider(srv.URL)
	req := &ChatRequest{Messages: []Message{strMsg("user", "hi")}}
	resp, err := p.Chat(context.Background(), "model", req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].Message.ContentString() != "Hello from Claude" {
		t.Errorf("content: got %q", resp.Choices[0].Message.ContentString())
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("prompt tokens: want 10, got %d", resp.Usage.PromptTokens)
	}
}

func TestAnthropicProvider_Chat_ToolCallResponse(t *testing.T) {
	respBody := `{
		"id":"msg_002","type":"message","role":"assistant",
		"content":[{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"location":"NYC"}}],
		"model":"claude-3-5-sonnet","stop_reason":"tool_use",
		"usage":{"input_tokens":20,"output_tokens":10}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, respBody)
	}))
	defer srv.Close()

	p := newTestAnthropicProvider(srv.URL)
	schema := json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`)
	req := &ChatRequest{
		Messages: []Message{strMsg("user", "weather?")},
		Tools:    []Tool{{Type: "function", Function: ToolFunction{Name: "get_weather", Parameters: schema}}},
	}
	resp, err := p.Chat(context.Background(), "model", req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("finish_reason: want tool_calls, got %s", resp.Choices[0].FinishReason)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("tool_calls: want 1, got %d", len(resp.Choices[0].Message.ToolCalls))
	}
}

func TestAnthropicProvider_ChatStream_TextChunks(t *testing.T) {
	// Minimal Anthropic SSE stream for a plain text response.
	sseBody := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_003","usage":{"input_tokens":25,"output_tokens":0}}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":0}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	p := newTestAnthropicProvider(srv.URL)
	req := &ChatRequest{Messages: []Message{strMsg("user", "hi")}, Stream: true}

	var buf strings.Builder
	usage, err := p.ChatStream(context.Background(), "model", req, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if usage == nil {
		t.Fatal("expected usage, got nil")
	}
	if usage.PromptTokens != 25 || usage.CompletionTokens != 2 {
		t.Errorf("usage: want in=25 out=2, got in=%d out=%d", usage.PromptTokens, usage.CompletionTokens)
	}

	// Verify "Hello world" appears in the SSE output.
	output := buf.String()
	if !strings.Contains(output, "Hello") || !strings.Contains(output, "world") {
		t.Errorf("stream output missing text chunks: %q", output)
	}
	if !strings.Contains(output, "[DONE]") {
		t.Errorf("stream output missing [DONE] sentinel")
	}
}

func TestAnthropicProvider_ChatStream_MultiToolIndex(t *testing.T) {
	// Two tool_use blocks in one stream — each must emit with a distinct index.
	sseBody := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_004","usage":{"input_tokens":30,"output_tokens":0}}}`,
		"",
		// Block 0: read_file
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"read_file"}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a.go\"}"}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":0}`,
		"",
		// Block 1: list_dir
		"event: content_block_start",
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_02","name":"list_dir"}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"/src\"}"}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":1}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	p := newTestAnthropicProvider(srv.URL)
	req := &ChatRequest{Messages: []Message{strMsg("user", "do both")}, Stream: true}

	var buf strings.Builder
	_, err := p.ChatStream(context.Background(), "model", req, &buf)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the SSE output and verify index assignment.
	output := buf.String()
	index0Seen := false
	index1Seen := false

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") || strings.Contains(line, "[DONE]") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			for _, tc := range choice.Delta.ToolCalls {
				if tc.Index != nil {
					switch *tc.Index {
					case 0:
						index0Seen = true
					case 1:
						index1Seen = true
					}
				}
			}
		}
	}

	if !index0Seen {
		t.Error("no streaming chunk with tool call index=0 found")
	}
	if !index1Seen {
		t.Error("no streaming chunk with tool call index=1 found")
	}
}

