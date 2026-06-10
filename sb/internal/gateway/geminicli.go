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

// geminiCLIProvider implements Provider by shelling out to the Google `gemini`
// CLI in headless mode (`gemini -p --output-format json`). Uses the user's
// Google subscription / Gemini Advanced credentials from ~/.gemini/ — no
// GOOGLE_API_KEY required.
//
// Usage in desk TOML:
//
//	[llm.routing]
//	plan     = "gemini-cli/gemini-2.5-pro"
//	classify = "gemini-cli/gemini-2.5-flash"
type geminiCLIProvider struct {
	geminiBin string
	model     string // default model if none specified at call time
}

func newGeminiCLIProvider(defaultModel string) (*geminiCLIProvider, error) {
	bin, err := exec.LookPath("gemini")
	if err != nil {
		return nil, fmt.Errorf("gemini CLI not found in PATH (install from https://ai.google.dev/gemini-api/docs/gemini-cli): %w", err)
	}
	return &geminiCLIProvider{geminiBin: bin, model: defaultModel}, nil
}

func (p *geminiCLIProvider) Name() string { return "gemini-cli" }

// geminiJSONOutput is the structure returned by `gemini -p --output-format json`.
type geminiJSONOutput struct {
	Response string          `json:"response"`
	Stats    *geminiStats    `json:"stats,omitempty"`
	Error    string          `json:"error,omitempty"`
	Model    string          `json:"model,omitempty"`
}

type geminiStats struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// geminiStreamEvent is a single NDJSON event from `--output-format stream-json`.
type geminiStreamEvent struct {
	Type    string `json:"type"`    // "content", "done", "error"
	Content string `json:"content,omitempty"`
	// Final stats on type=="done"
	Stats *geminiStats `json:"stats,omitempty"`
	Error string       `json:"error,omitempty"`
	Model string       `json:"model,omitempty"`
}

func (p *geminiCLIProvider) Chat(ctx context.Context, model string, req *ChatRequest) (*ChatResponse, error) {
	prompt, system := buildGeminiCLIPrompt(req)
	resolvedModel := coalesce(model, p.model)

	args := []string{"-p", prompt, "--output-format", "json"}
	if resolvedModel != "" {
		args = append(args, "--model", resolvedModel)
	}
	if system != "" {
		args = append(args, "--system-prompt", system)
	}

	cmd := exec.CommandContext(ctx, p.geminiBin, args...)
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
			Message:    "gemini-cli: " + msg,
			Provider:   "gemini-cli",
		}
	}

	var out geminiJSONOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, &ProviderError{
			StatusCode: 502,
			Message:    "gemini-cli: failed to parse output: " + err.Error(),
			Provider:   "gemini-cli",
		}
	}
	if out.Error != "" {
		return nil, &ProviderError{
			StatusCode: 500,
			Message:    "gemini-cli: " + out.Error,
			Provider:   "gemini-cli",
		}
	}

	content, _ := json.Marshal(out.Response)
	resp := &ChatResponse{
		ID:      fmt.Sprintf("chatcmpl-gcli-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   coalesce(out.Model, resolvedModel),
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
	if out.Stats != nil {
		resp.Usage = &Usage{
			PromptTokens:     out.Stats.InputTokens,
			CompletionTokens: out.Stats.OutputTokens,
			TotalTokens:      out.Stats.TotalTokens,
		}
	}
	return resp, nil
}

// ChatStream uses `--output-format stream-json` for real token-level streaming.
func (p *geminiCLIProvider) ChatStream(ctx context.Context, model string, req *ChatRequest, w io.Writer) (*Usage, error) {
	prompt, system := buildGeminiCLIPrompt(req)
	resolvedModel := coalesce(model, p.model)

	args := []string{"-p", prompt, "--output-format", "stream-json"}
	if resolvedModel != "" {
		args = append(args, "--model", resolvedModel)
	}
	if system != "" {
		args = append(args, "--system-prompt", system)
	}

	cmd := exec.CommandContext(ctx, p.geminiBin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("gemini-cli: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, &ProviderError{StatusCode: 502, Message: "gemini-cli: " + err.Error(), Provider: "gemini-cli"}
	}

	chunkID := fmt.Sprintf("chatcmpl-gcli-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	// Emit role chunk
	writeSSEChunk(w, StreamChunk{
		ID: chunkID, Object: "chat.completion.chunk", Created: created, Model: resolvedModel,
		Choices: []StreamChoice{{Index: 0, Delta: StreamDelta{Role: "assistant"}}},
	})

	var usage *Usage
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return nil, ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event geminiStreamEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue // skip malformed lines
		}

		switch event.Type {
		case "content":
			writeSSEChunk(w, StreamChunk{
				ID: chunkID, Object: "chat.completion.chunk", Created: created, Model: resolvedModel,
				Choices: []StreamChoice{{Index: 0, Delta: StreamDelta{Content: event.Content}}},
			})
		case "done":
			if event.Stats != nil {
				usage = &Usage{
					PromptTokens:     event.Stats.InputTokens,
					CompletionTokens: event.Stats.OutputTokens,
					TotalTokens:      event.Stats.TotalTokens,
				}
			}
		case "error":
			_ = cmd.Process.Kill()
			return nil, &ProviderError{StatusCode: 500, Message: "gemini-cli: " + event.Error, Provider: "gemini-cli"}
		}
	}

	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, &ProviderError{StatusCode: 502, Message: "gemini-cli: " + msg, Provider: "gemini-cli"}
	}

	// Final SSE chunk
	stop := "stop"
	writeSSEChunk(w, StreamChunk{
		ID: chunkID, Object: "chat.completion.chunk", Created: created, Model: resolvedModel,
		Choices: []StreamChoice{{Index: 0, Delta: StreamDelta{}, FinishReason: &stop}},
		Usage:   usage,
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	return usage, nil
}

// buildGeminiCLIPrompt converts OpenAI messages to a flat prompt + system string.
func buildGeminiCLIPrompt(req *ChatRequest) (prompt, system string) {
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
			sb.WriteString("User: ")
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
	prompt = strings.TrimPrefix(prompt, "User: ")
	return prompt, system
}
