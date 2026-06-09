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

const openAIAPIBase = "https://api.openai.com"

// openAIProvider implements Provider for OpenAI.
// Since the gateway speaks OpenAI format natively, this is largely a passthrough.
type openAIProvider struct {
	apiKey     string
	httpClient *http.Client
}

func newOpenAIProvider(apiKey string) *openAIProvider {
	return &openAIProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (p *openAIProvider) Name() string { return "openai" }

// openAIWireRequest is what we send to OpenAI (strip Packmule extension fields).
type openAIWireRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
	// Request usage info in the final streaming chunk
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

func (p *openAIProvider) Chat(ctx context.Context, model string, req *ChatRequest) (*ChatResponse, error) {
	wire := &openAIWireRequest{
		Model:       model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}

	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		openAIAPIBase+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{StatusCode: 0, Message: err.Error(), Provider: "openai"}
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
			Provider:   "openai",
		}
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("openai: unmarshal response: %w", err)
	}
	return &chatResp, nil
}

func (p *openAIProvider) ChatStream(ctx context.Context, model string, req *ChatRequest, w io.Writer) (*Usage, error) {
	wire := &openAIWireRequest{
		Model:       model,
		Messages:    req.Messages,
		Stream:      true,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
		StreamOptions: &openAIStreamOptions{IncludeUsage: true},
	}

	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		openAIAPIBase+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{StatusCode: 0, Message: err.Error(), Provider: "openai"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{StatusCode: resp.StatusCode, Message: string(b), Provider: "openai"}
	}

	// OpenAI SSE is already in the right format — pass through, capturing usage from final chunk.
	return p.passthrough(ctx, resp.Body, w)
}

func (p *openAIProvider) passthrough(ctx context.Context, body io.Reader, w io.Writer) (*Usage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	var finalUsage *Usage

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return finalUsage, ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" {
			fmt.Fprint(w, "\n")
			continue
		}

		fmt.Fprintln(w, line)

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.Usage != nil {
				finalUsage = chunk.Usage
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return finalUsage, fmt.Errorf("openai: stream read: %w", err)
	}
	return finalUsage, nil
}

func (p *openAIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
}
