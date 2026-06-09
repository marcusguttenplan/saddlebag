package gateway

import (
	"context"
	"io"
)

// Provider is the interface every upstream model provider must implement.
// All providers normalize to/from the OpenAI-compatible types in types.go.
//
// Implementations: anthropicProvider, openAIProvider, googleProvider, ollamaProvider.
type Provider interface {
	// Name returns the provider identifier: "anthropic", "openai", "google", "ollama".
	Name() string

	// Chat sends a non-streaming chat completion request and returns the full response.
	// ctx is cancelled if the budget is exceeded or the client disconnects.
	Chat(ctx context.Context, model string, req *ChatRequest) (*ChatResponse, error)

	// ChatStream sends a streaming chat completion request.
	// The provider writes OpenAI-compatible SSE chunks to w.
	// It must write the terminal "data: [DONE]\n\n" sentinel before returning.
	// The returned Usage is populated after all chunks have been sent (may be nil
	// if the provider does not report usage in the stream).
	ChatStream(ctx context.Context, model string, req *ChatRequest, w io.Writer) (*Usage, error)
}

// ProviderError wraps an upstream provider error with its HTTP status code.
type ProviderError struct {
	StatusCode int
	Message    string
	Provider   string
}

func (e *ProviderError) Error() string {
	return e.Provider + ": " + e.Message
}

// IsRetryable returns true for provider errors that warrant a retry or fallback.
func (e *ProviderError) IsRetryable() bool {
	return e.StatusCode == 429 || e.StatusCode >= 500
}
