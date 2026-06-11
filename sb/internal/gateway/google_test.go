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

// --- toGeminiRequest unit tests ---

func TestToGeminiRequest_SystemInstruction(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{
			strMsg("system", "you are helpful"),
			strMsg("user", "hi"),
		},
	}
	gReq, err := toGeminiRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if gReq.SystemInstruction == nil {
		t.Fatal("systemInstruction: want non-nil, got nil")
	}
	if len(gReq.SystemInstruction.Parts) == 0 || gReq.SystemInstruction.Parts[0].Text != "you are helpful" {
		t.Errorf("systemInstruction text: want 'you are helpful', got %+v", gReq.SystemInstruction)
	}
	// System message should not appear in Contents.
	for _, c := range gReq.Contents {
		if c.Role == "system" {
			t.Error("system role found in contents — should have been extracted to systemInstruction")
		}
	}
}

func TestToGeminiRequest_RoleMapping(t *testing.T) {
	// OpenAI "assistant" → Gemini "model"
	req := &ChatRequest{
		Messages: []Message{
			strMsg("user", "hello"),
			strMsg("assistant", "hi there"),
		},
	}
	gReq, err := toGeminiRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(gReq.Contents) != 2 {
		t.Fatalf("contents: want 2, got %d", len(gReq.Contents))
	}
	if gReq.Contents[1].Role != "model" {
		t.Errorf("assistant→model: want model, got %s", gReq.Contents[1].Role)
	}
}

func TestToGeminiRequest_ToolDefinitions(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"loc":{"type":"string"}}}`)
	req := &ChatRequest{
		Messages: []Message{strMsg("user", "weather?")},
		Tools:    []Tool{{Type: "function", Function: ToolFunction{Name: "get_weather", Parameters: schema}}},
	}
	gReq, err := toGeminiRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(gReq.Tools) != 1 {
		t.Fatalf("tools: want 1, got %d", len(gReq.Tools))
	}
	if len(gReq.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("functionDeclarations: want 1, got %d", len(gReq.Tools[0].FunctionDeclarations))
	}
	if gReq.Tools[0].FunctionDeclarations[0].Name != "get_weather" {
		t.Errorf("name: want get_weather, got %s", gReq.Tools[0].FunctionDeclarations[0].Name)
	}
}

func TestToGeminiRequest_ToolResult(t *testing.T) {
	// role=tool in OpenAI → functionResponse part in Gemini user message
	req := &ChatRequest{
		Messages: []Message{
			strMsg("user", "call it"),
			{
				Role: "assistant",
				ToolCalls: []ToolCall{{
					ID:   "tc1",
					Type: "function",
					Function: ToolCallFunction{Name: "get_weather", Arguments: `{"location":"NYC"}`},
				}},
			},
			{Role: "tool", Name: "get_weather", ToolCallID: "tc1", Content: json.RawMessage(`"72 degrees"`)},
		},
	}
	gReq, err := toGeminiRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	// Find the tool result message.
	var found bool
	for _, c := range gReq.Contents {
		for _, p := range c.Parts {
			if p.FunctionResponse != nil && p.FunctionResponse.Name == "get_weather" {
				found = true
			}
		}
	}
	if !found {
		t.Error("functionResponse part for get_weather not found in contents")
	}
}

// --- fromGeminiResponse unit tests ---

func TestFromGeminiResponse_TextResponse(t *testing.T) {
	gResp := &geminiResponse{
		Candidates: []geminiCandidate{{
			Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: "Hello from Gemini"}}},
			FinishReason: "STOP",
		}},
		UsageMetadata: &geminiUsage{PromptTokenCount: 10, CandidatesTokenCount: 4, TotalTokenCount: 14},
	}
	resp := fromGeminiResponse(gResp, "gemini-2.0-flash")
	if resp.Choices[0].Message.ContentString() != "Hello from Gemini" {
		t.Errorf("content: want 'Hello from Gemini', got %q", resp.Choices[0].Message.ContentString())
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason: want stop, got %s", resp.Choices[0].FinishReason)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("prompt tokens: want 10, got %d", resp.Usage.PromptTokens)
	}
}

func TestFromGeminiResponse_ToolCall(t *testing.T) {
	gResp := &geminiResponse{
		Candidates: []geminiCandidate{{
			Content: geminiContent{
				Role: "model",
				Parts: []geminiPart{{
					FunctionCall: &geminiFunctionCall{
						Name: "get_weather",
						Args: json.RawMessage(`{"location":"NYC"}`),
					},
				}},
			},
			FinishReason: "STOP",
		}},
	}
	resp := fromGeminiResponse(gResp, "gemini-2.0-flash")
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 {
		t.Fatalf("tool_calls: want 1, got %d", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("tool name: want get_weather, got %s", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"location":"NYC"}` {
		t.Errorf("arguments: want %q, got %q", `{"location":"NYC"}`, calls[0].Function.Arguments)
	}
}

func TestFromGeminiResponse_EmptyCandidates(t *testing.T) {
	gResp := &geminiResponse{Candidates: nil}
	resp := fromGeminiResponse(gResp, "gemini")
	// Should not panic; should return an empty but valid response.
	if resp == nil {
		t.Fatal("expected non-nil response for empty candidates")
	}
}

// --- HTTP round-trip tests ---

func newTestGoogleProvider(serverURL string) *googleProvider {
	return &googleProvider{
		apiKey:     "test-key",
		httpClient: &http.Client{},
		baseURL:    serverURL,
	}
}

func TestGoogleProvider_Chat_TextResponse(t *testing.T) {
	respBody := `{
		"candidates":[{"content":{"role":"model","parts":[{"text":"Hello from Gemini"}]},"finishReason":"STOP"}],
		"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":4,"totalTokenCount":14}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, respBody)
	}))
	defer srv.Close()

	p := newTestGoogleProvider(srv.URL)
	req := &ChatRequest{Messages: []Message{strMsg("user", "hi")}}
	resp, err := p.Chat(context.Background(), "gemini-2.0-flash", req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].Message.ContentString() != "Hello from Gemini" {
		t.Errorf("content: got %q", resp.Choices[0].Message.ContentString())
	}
}

func TestGoogleProvider_ChatStream_TextChunks(t *testing.T) {
	// Two streaming chunks of text.
	sseBody := strings.Join([]string{
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hello "}]},"finishReason":""}]}`,
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"totalTokenCount":12}}`,
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	p := newTestGoogleProvider(srv.URL)
	req := &ChatRequest{Messages: []Message{strMsg("user", "hi")}, Stream: true}

	var buf strings.Builder
	usage, err := p.ChatStream(context.Background(), "gemini-2.0-flash", req, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if usage == nil || usage.PromptTokens != 10 {
		t.Errorf("usage: want in=10, got %+v", usage)
	}
	output := buf.String()
	if !strings.Contains(output, "Hello") || !strings.Contains(output, "world") {
		t.Errorf("stream output missing text: %q", output)
	}
	if !strings.Contains(output, "[DONE]") {
		t.Error("stream output missing [DONE]")
	}
}

func TestGoogleProvider_ChatStream_ToolCallStableIDs(t *testing.T) {
	// Two tool calls in the same stream — each must have a distinct stable ID,
	// and that ID must be consistent across start and args chunks.
	sseBody := strings.Join([]string{
		// First chunk: read_file call
		`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"a.go"}}}]},"finishReason":""}]}`,
		// Second chunk: list_dir call
		`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"list_dir","args":{"path":"/src"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":10,"totalTokenCount":30}}`,
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	p := newTestGoogleProvider(srv.URL)
	req := &ChatRequest{Messages: []Message{strMsg("user", "do both")}, Stream: true}

	var buf strings.Builder
	_, err := p.ChatStream(context.Background(), "gemini-2.0-flash", req, &buf)
	if err != nil {
		t.Fatal(err)
	}

	// Collect all tool call chunks and verify distinct indices and IDs.
	type toolChunkInfo struct {
		index *int
		id    string
		name  string
	}
	var toolChunks []toolChunkInfo

	scanner := bufio.NewScanner(strings.NewReader(buf.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") || strings.Contains(line, "[DONE]") {
			continue
		}
		var chunk StreamChunk
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			for _, tc := range choice.Delta.ToolCalls {
				if tc.ID != "" || tc.Function.Name != "" {
					toolChunks = append(toolChunks, toolChunkInfo{
						index: tc.Index,
						id:    tc.ID,
						name:  tc.Function.Name,
					})
				}
			}
		}
	}

	if len(toolChunks) < 2 {
		t.Fatalf("expected at least 2 tool start chunks, got %d", len(toolChunks))
	}

	// First call should be index 0, second should be index 1.
	if toolChunks[0].index == nil || *toolChunks[0].index != 0 {
		t.Errorf("first tool call: want index=0, got %v", toolChunks[0].index)
	}
	if toolChunks[1].index == nil || *toolChunks[1].index != 1 {
		t.Errorf("second tool call: want index=1, got %v", toolChunks[1].index)
	}

	// IDs must be distinct.
	if toolChunks[0].id == toolChunks[1].id {
		t.Errorf("tool call IDs should be distinct, both are %q", toolChunks[0].id)
	}
}

func TestGoogleProvider_ChatStream_SameNameTwice(t *testing.T) {
	// Two calls to the same function (e.g., read_file twice) must get distinct IDs and indices.
	sseBody := strings.Join([]string{
		`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"a.go"}}}]},"finishReason":""}]}`,
		`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"b.go"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":15,"candidatesTokenCount":8,"totalTokenCount":23}}`,
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	p := newTestGoogleProvider(srv.URL)
	req := &ChatRequest{Messages: []Message{strMsg("user", "read both files")}, Stream: true}

	var buf strings.Builder
	_, err := p.ChatStream(context.Background(), "gemini-2.0-flash", req, &buf)
	if err != nil {
		t.Fatal(err)
	}

	// Collect start chunks (those with an ID set).
	var ids []string
	var indices []int
	scanner := bufio.NewScanner(strings.NewReader(buf.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") || strings.Contains(line, "[DONE]") {
			continue
		}
		var chunk StreamChunk
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			for _, tc := range choice.Delta.ToolCalls {
				if tc.ID != "" {
					ids = append(ids, tc.ID)
					if tc.Index != nil {
						indices = append(indices, *tc.Index)
					}
				}
			}
		}
	}

	if len(ids) < 2 {
		t.Fatalf("expected 2 start chunks (one per read_file call), got %d", len(ids))
	}
	if ids[0] == ids[1] {
		t.Errorf("same-name tool calls should have distinct IDs, both %q", ids[0])
	}
	if len(indices) >= 2 && indices[0] == indices[1] {
		t.Errorf("same-name tool calls should have distinct indices, both %d", indices[0])
	}
}
