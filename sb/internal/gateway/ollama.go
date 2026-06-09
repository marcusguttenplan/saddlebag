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

const ollamaDefaultBase = "http://localhost:11434"

// ollamaProvider implements Provider for locally-running Ollama models.
// Ollama's API is OpenAI-compatible at /api/chat, so normalization is minimal.
type ollamaProvider struct {
	baseURL    string
	httpClient *http.Client
}

func newOllamaProvider(baseURL string) *ollamaProvider {
	if baseURL == "" {
		baseURL = ollamaDefaultBase
	}
	return &ollamaProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Minute}, // longer timeout for local models
	}
}

func (p *ollamaProvider) Name() string { return "ollama" }

// ollamaRequest is the Ollama /api/chat request format.
// Ollama is largely OpenAI-compatible but uses "stream" bool and slightly different options.
type ollamaRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Options  *ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	NumPredict  *int     `json:"num_predict,omitempty"` // max_tokens equivalent
	Stop        []string `json:"stop,omitempty"`
}

// ollamaResponse is the Ollama non-streaming response.
type ollamaResponse struct {
	Model      string  `json:"model"`
	Message    Message `json:"message"`
	DoneReason string  `json:"done_reason"`
	Done       bool    `json:"done"`
	// Token counts (approximate)
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

// ollamaStreamChunk is a single Ollama streaming event.
type ollamaStreamChunk struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`
	Done    bool    `json:"done"`
	// Final chunk only
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

func (p *ollamaProvider) Chat(ctx context.Context, model string, req *ChatRequest) (*ChatResponse, error) {
	oReq := p.toOllamaRequest(model, req, false)
	body, err := json.Marshal(oReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{StatusCode: 0, Message: "ollama unavailable: " + err.Error(), Provider: "ollama"}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{StatusCode: resp.StatusCode, Message: string(respBody), Provider: "ollama"}
	}

	var oResp ollamaResponse
	if err := json.Unmarshal(respBody, &oResp); err != nil {
		return nil, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	contentJSON, _ := json.Marshal(oResp.Message.ContentString())
	return &ChatResponse{
		ID:     fmt.Sprintf("chatcmpl-ollama-%d", time.Now().UnixNano()),
		Object: "chat.completion",
		Model:  model,
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:    "assistant",
				Content: contentJSON,
			},
			FinishReason: oResp.DoneReason,
		}},
		Usage: &Usage{
			PromptTokens:     oResp.PromptEvalCount,
			CompletionTokens: oResp.EvalCount,
			TotalTokens:      oResp.PromptEvalCount + oResp.EvalCount,
		},
	}, nil
}

func (p *ollamaProvider) ChatStream(ctx context.Context, model string, req *ChatRequest, w io.Writer) (*Usage, error) {
	oReq := p.toOllamaRequest(model, req, true)
	body, err := json.Marshal(oReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{StatusCode: 0, Message: "ollama unavailable: " + err.Error(), Provider: "ollama"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{StatusCode: resp.StatusCode, Message: string(b), Provider: "ollama"}
	}

	return p.streamToOpenAI(ctx, resp.Body, model, w)
}

// streamToOpenAI reads Ollama's newline-delimited JSON chunks and writes OpenAI SSE.
func (p *ollamaProvider) streamToOpenAI(ctx context.Context, body io.Reader, model string, w io.Writer) (*Usage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	chunkID := fmt.Sprintf("chatcmpl-ollama-%d", time.Now().UnixNano())
	var finalUsage *Usage

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return finalUsage, ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk ollamaStreamChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}

		if chunk.Done {
			finalUsage = &Usage{
				PromptTokens:     chunk.PromptEvalCount,
				CompletionTokens: chunk.EvalCount,
				TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			return finalUsage, nil
		}

		// Emit OpenAI-format SSE chunk
		openAIChunk := StreamChunk{
			ID:     chunkID,
			Object: "chat.completion.chunk",
			Model:  model,
			Choices: []StreamChoice{{
				Index: 0,
				Delta: StreamDelta{Content: chunk.Message.ContentString()},
			}},
		}
		writeSSEChunk(w, openAIChunk)
	}

	if err := scanner.Err(); err != nil {
		return finalUsage, fmt.Errorf("ollama: stream read: %w", err)
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	return finalUsage, nil
}

func (p *ollamaProvider) toOllamaRequest(model string, req *ChatRequest, stream bool) *ollamaRequest {
	oReq := &ollamaRequest{
		Model:    model,
		Messages: req.Messages,
		Stream:   stream,
	}
	if req.MaxTokens != nil || req.Temperature != nil || req.TopP != nil || len(req.Stop) > 0 {
		oReq.Options = &ollamaOptions{
			Temperature: req.Temperature,
			TopP:        req.TopP,
			NumPredict:  req.MaxTokens,
			Stop:        req.Stop,
		}
	}
	return oReq
}
