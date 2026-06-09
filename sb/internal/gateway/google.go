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

const googleAPIBase = "https://generativelanguage.googleapis.com"

// googleProvider implements Provider for Google's Gemini models.
type googleProvider struct {
	apiKey     string
	httpClient *http.Client
}

func newGoogleProvider(apiKey string) *googleProvider {
	return &googleProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (p *googleProvider) Name() string { return "google" }

// ---------------------------------------------------------------------------
// Google Gemini native types
// ---------------------------------------------------------------------------

type geminiRequest struct {
	Contents          []geminiContent     `json:"contents"`
	SystemInstruction *geminiContent      `json:"systemInstruction,omitempty"`
	Tools             []geminiTool        `json:"tools,omitempty"`
	GenerationConfig  *geminiGenConfig    `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text         string            `json:"text,omitempty"`
	FunctionCall *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
	InlineData   *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type geminiInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"` // base64
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiGenConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     float64  `json:"temperature,omitempty"`
	TopP            float64  `json:"topP,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content       geminiContent `json:"content"`
	FinishReason  string        `json:"finishReason"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// ---------------------------------------------------------------------------
// Chat (non-streaming)
// ---------------------------------------------------------------------------

func (p *googleProvider) Chat(ctx context.Context, model string, req *ChatRequest) (*ChatResponse, error) {
	gReq, err := toGeminiRequest(req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(gReq)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", googleAPIBase, model, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{StatusCode: 0, Message: err.Error(), Provider: "google"}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{StatusCode: resp.StatusCode, Message: string(respBody), Provider: "google"}
	}

	var gResp geminiResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return nil, fmt.Errorf("google: unmarshal response: %w", err)
	}

	return fromGeminiResponse(&gResp, model), nil
}

// ---------------------------------------------------------------------------
// ChatStream (streaming)
// ---------------------------------------------------------------------------

func (p *googleProvider) ChatStream(ctx context.Context, model string, req *ChatRequest, w io.Writer) (*Usage, error) {
	gReq, err := toGeminiRequest(req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(gReq)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", googleAPIBase, model, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{StatusCode: 0, Message: err.Error(), Provider: "google"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{StatusCode: resp.StatusCode, Message: string(b), Provider: "google"}
	}

	return p.streamToOpenAI(ctx, resp.Body, model, w)
}

func (p *googleProvider) streamToOpenAI(ctx context.Context, body io.Reader, model string, w io.Writer) (*Usage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	chunkID := fmt.Sprintf("chatcmpl-google-%d", time.Now().UnixNano())
	var finalUsage *Usage

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return finalUsage, ctx.Err()
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var gResp geminiResponse
		if err := json.Unmarshal([]byte(data), &gResp); err != nil {
			continue
		}

		if gResp.UsageMetadata != nil {
			finalUsage = &Usage{
				PromptTokens:     gResp.UsageMetadata.PromptTokenCount,
				CompletionTokens: gResp.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      gResp.UsageMetadata.TotalTokenCount,
			}
		}

		for _, candidate := range gResp.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					chunk := openAIChunkForText(chunkID, model, part.Text)
					writeSSEChunk(w, chunk)
				}
				// Tool calls in streaming: emit as tool_calls delta
				if part.FunctionCall != nil {
					// Generate a deterministic-ish ID
					toolID := fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, time.Now().UnixNano())
					startChunk := openAIChunkForToolStart(chunkID, model, 0, toolID, part.FunctionCall.Name)
					writeSSEChunk(w, startChunk)
					if len(part.FunctionCall.Args) > 0 {
						argsChunk := openAIChunkForToolArgs(chunkID, model, 0, string(part.FunctionCall.Args))
						writeSSEChunk(w, argsChunk)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return finalUsage, fmt.Errorf("google: stream read: %w", err)
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	return finalUsage, nil
}

// ---------------------------------------------------------------------------
// Normalization helpers
// ---------------------------------------------------------------------------

func toGeminiRequest(req *ChatRequest) (*geminiRequest, error) {
	gReq := &geminiRequest{}

	// Build GenerationConfig
	cfg := &geminiGenConfig{StopSequences: req.Stop}
	if req.MaxTokens != nil {
		cfg.MaxOutputTokens = *req.MaxTokens
	}
	if req.Temperature != nil {
		cfg.Temperature = *req.Temperature
	}
	if req.TopP != nil {
		cfg.TopP = *req.TopP
	}
	gReq.GenerationConfig = cfg

	// Build contents, extracting system prompt
	for _, m := range req.Messages {
		if m.Role == "system" {
			gReq.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: m.ContentString()}},
			}
			continue
		}

		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		// tool results use "user" role in Gemini
		if role == "tool" {
			role = "user"
		}

		gc := geminiContent{Role: role}

		// Tool result message
		if m.Role == "tool" {
			resp, _ := json.Marshal(map[string]string{"result": m.ContentString()})
			gc.Parts = []geminiPart{{
				FunctionResponse: &geminiFunctionResponse{
					Name:     m.Name,
					Response: resp,
				},
			}}
			gReq.Contents = append(gReq.Contents, gc)
			continue
		}

		// Assistant with tool calls
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				gc.Parts = append(gc.Parts, geminiPart{
					FunctionCall: &geminiFunctionCall{
						Name: tc.Function.Name,
						Args: json.RawMessage(tc.Function.Arguments),
					},
				})
			}
			if text := m.ContentString(); text != "" {
				gc.Parts = append([]geminiPart{{Text: text}}, gc.Parts...)
			}
			gReq.Contents = append(gReq.Contents, gc)
			continue
		}

		gc.Parts = []geminiPart{{Text: m.ContentString()}}
		gReq.Contents = append(gReq.Contents, gc)
	}

	// Tools
	if len(req.Tools) > 0 {
		var decls []geminiFunctionDecl
		for _, t := range req.Tools {
			decls = append(decls, geminiFunctionDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		gReq.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}

	return gReq, nil
}

func fromGeminiResponse(gResp *geminiResponse, model string) *ChatResponse {
	if len(gResp.Candidates) == 0 {
		return &ChatResponse{
			ID:     fmt.Sprintf("chatcmpl-google-%d", time.Now().UnixNano()),
			Object: "chat.completion",
			Model:  model,
		}
	}

	candidate := gResp.Candidates[0]
	msg := Message{Role: "assistant"}

	var textParts []string
	var toolCalls []ToolCall

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			toolID := fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, time.Now().UnixNano())
			toolCalls = append(toolCalls, ToolCall{
				ID:   toolID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      part.FunctionCall.Name,
					Arguments: string(part.FunctionCall.Args),
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
	if candidate.FinishReason == "STOP" {
		finishReason = "stop"
	} else if strings.Contains(candidate.FinishReason, "FUNCTION") {
		finishReason = "tool_calls"
	}

	var usage *Usage
	if gResp.UsageMetadata != nil {
		usage = &Usage{
			PromptTokens:     gResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: gResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gResp.UsageMetadata.TotalTokenCount,
		}
	}

	return &ChatResponse{
		ID:     fmt.Sprintf("chatcmpl-google-%d", time.Now().UnixNano()),
		Object: "chat.completion",
		Model:  model,
		Choices: []Choice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
}
