package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// claudeCodeProvider implements Provider by shelling out to the `claude` CLI
// in headless/print mode (`claude -p`). This allows using a Claude Pro/Max
// subscription without Anthropic API keys, drawing from the monthly Agent SDK
// credit pool (as of Anthropic's June 15, 2026 policy).
//
// Usage in desk TOML:
//
//	[llm.routing]
//	implement = "claude-code/claude-opus-4-20250514"
//	classify  = "claude-code/claude-sonnet-4-20250514"
type claudeCodeProvider struct {
	// claudeBin is the resolved path to the `claude` binary.
	claudeBin string
}

func newClaudeCodeProvider() (*claudeCodeProvider, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found in PATH (install from https://claude.ai/download): %w", err)
	}
	return &claudeCodeProvider{claudeBin: bin}, nil
}

func (p *claudeCodeProvider) Name() string { return "claude-code" }

// claudeJSONOutput is the structure returned by `claude -p --output-format json`.
// Fields documented at https://docs.anthropic.com/en/docs/claude-code/headless
type claudeJSONOutput struct {
	// Type is "result" for final output, "error" for failures.
	Type string `json:"type"`
	// Result holds the assistant's text response.
	Result string `json:"result"`
	// SubType distinguishes "success" from "error".
	SubType string `json:"subtype"`
	// Usage holds token counts (populated when available).
	Usage *claudeCodeUsage `json:"usage,omitempty"`
	// Error holds the error message when Type=="error".
	Error string `json:"error,omitempty"`
	// SessionID is the Claude Code session identifier.
	SessionID string `json:"session_id,omitempty"`
	// Model is the model that was actually used.
	Model string `json:"model,omitempty"`
}

type claudeCodeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Chat implements Provider.Chat by running `claude -p <prompt> --output-format json`.
// The full conversation is serialized as a single prompt since claude -p is
// stateless (single-turn headless mode).
func (p *claudeCodeProvider) Chat(ctx context.Context, model string, req *ChatRequest) (*ChatResponse, error) {
	prompt, system := buildClaudeCodePrompt(req)

	args := []string{"-p", prompt, "--output-format", "json"}
	if model != "" && model != "default" {
		args = append(args, "--model", model)
	}
	if system != "" {
		args = append(args, "--system-prompt", system)
	}
	if req.MaxTokens != nil {
		args = append(args, "--max-tokens", fmt.Sprintf("%d", *req.MaxTokens))
	}

	cmd := exec.CommandContext(ctx, p.claudeBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, &ProviderError{
			StatusCode: 502,
			Message:    "claude-code: " + msg,
			Provider:   "claude-code",
		}
	}

	var out claudeJSONOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, &ProviderError{
			StatusCode: 502,
			Message:    "claude-code: failed to parse output: " + err.Error(),
			Provider:   "claude-code",
		}
	}
	if out.Type == "error" || out.SubType == "error" {
		return nil, &ProviderError{
			StatusCode: 500,
			Message:    "claude-code: " + out.Error,
			Provider:   "claude-code",
		}
	}

	// Normalize to OpenAI-compatible response
	content, _ := json.Marshal(out.Result)
	resp := &ChatResponse{
		ID:      fmt.Sprintf("chatcmpl-cc-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   coalesce(out.Model, model),
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
	}
	if out.Usage != nil {
		resp.Usage = &Usage{
			PromptTokens:     out.Usage.InputTokens,
			CompletionTokens: out.Usage.OutputTokens,
			TotalTokens:      out.Usage.InputTokens + out.Usage.OutputTokens,
		}
	}
	return resp, nil
}

// ChatStream implements Provider.ChatStream.
// claude -p does not natively stream SSE; we run the process and emit a single
// content chunk once it completes, then write the [DONE] sentinel.
// For a snappier feel we emit the text word-by-word with tiny delays.
func (p *claudeCodeProvider) ChatStream(ctx context.Context, model string, req *ChatRequest, w io.Writer) (*Usage, error) {
	resp, err := p.Chat(ctx, model, req)
	if err != nil {
		return nil, err
	}

	chunkID := fmt.Sprintf("chatcmpl-cc-%d", time.Now().UnixNano())
	text := ""
	if len(resp.Choices) > 0 {
		text = resp.Choices[0].Message.ContentString()
	}

	// Emit role chunk first
	roleChunk := StreamChunk{
		ID:      chunkID,
		Object:  "chat.completion.chunk",
		Created: resp.Created,
		Model:   resp.Model,
		Choices: []StreamChoice{{Index: 0, Delta: StreamDelta{Role: "assistant"}}},
	}
	writeSSEChunk(w, roleChunk)

	// Stream word by word for a natural feel
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		word := scanner.Text() + " "
		chunk := StreamChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: resp.Created,
			Model:   resp.Model,
			Choices: []StreamChoice{{Index: 0, Delta: StreamDelta{Content: word}}},
		}
		writeSSEChunk(w, chunk)
	}

	// Final chunk with finish_reason and usage
	stop := "stop"
	finalChunk := StreamChunk{
		ID:      chunkID,
		Object:  "chat.completion.chunk",
		Created: resp.Created,
		Model:   resp.Model,
		Choices: []StreamChoice{{Index: 0, Delta: StreamDelta{}, FinishReason: &stop}},
		Usage:   resp.Usage,
	}
	writeSSEChunk(w, finalChunk)

	fmt.Fprint(w, "data: [DONE]\n\n")
	return resp.Usage, nil
}

// buildClaudeCodePrompt converts a multi-turn OpenAI messages array into a
// flat prompt string and extracts the system message (if any).
// claude -p is single-turn, so we simulate multi-turn by concatenating turns.
func buildClaudeCodePrompt(req *ChatRequest) (prompt, system string) {
	var sb strings.Builder
	for _, msg := range req.Messages {
		content := msg.ContentString()
		switch msg.Role {
		case "system":
			system = content
		case "user":
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString("Human: ")
			sb.WriteString(content)
		case "assistant":
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString("Assistant: ")
			sb.WriteString(content)
		}
	}
	prompt = strings.TrimSpace(sb.String())
	// Strip "Human: " prefix if it's the only/last message — claude -p
	// treats the argument as the user message directly.
	prompt = strings.TrimPrefix(prompt, "Human: ")
	return prompt, system
}

// coalesce returns the first non-empty string.
func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
