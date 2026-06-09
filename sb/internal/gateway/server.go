package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/marcusguttenplan/sb/internal/desk"
	"github.com/marcusguttenplan/sb/internal/ledger"
	"github.com/marcusguttenplan/sb/internal/trace"
)

// Server is the Packmule model gateway HTTP server.
// It listens on localhost and speaks OpenAI-compatible HTTP.
type Server struct {
	mux      *http.ServeMux
	httpSrv  *http.Server
	ledger   *ledger.Ledger
	budget   *Budget
	desk     *desk.Desk
	secrets  SecretResolver
	logger   *log.Logger
}

// SecretResolver retrieves API keys by provider name.
// In production, this calls into the Saddlebag keychain.
// In tests, it can be a simple map lookup.
type SecretResolver interface {
	APIKey(provider string) (string, error)
}

// Config holds everything needed to create a Server.
type Config struct {
	// Desk is the active desk configuration (routing, budget, Ollama URL).
	Desk *desk.Desk

	// Ledger is the token ledger (required).
	Ledger *ledger.Ledger

	// Secrets resolves API keys per provider.
	Secrets SecretResolver

	// Logger, if nil defaults to the standard logger.
	Logger *log.Logger
}

// New creates a new gateway Server from the given config.
func New(cfg Config) (*Server, error) {
	if cfg.Ledger == nil {
		return nil, fmt.Errorf("gateway: ledger is required")
	}
	if cfg.Secrets == nil {
		return nil, fmt.Errorf("gateway: secrets resolver is required")
	}
	if cfg.Desk == nil {
		cfg.Desk = &desk.Desk{}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	var limitUSD, warnUSD float64
	if cfg.Desk.LLM != nil && cfg.Desk.LLM.Budget != nil {
		limitUSD = cfg.Desk.LLM.Budget.DailyLimitUSD
		warnUSD = cfg.Desk.LLM.Budget.WarnAtUSD
	}

	s := &Server{
		mux:    http.NewServeMux(),
		ledger: cfg.Ledger,
		budget: NewBudget(cfg.Ledger, limitUSD, warnUSD),
		desk:   cfg.Desk,
		secrets: cfg.Secrets,
		logger: logger,
	}

	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/v1/models", s.handleModels)
	s.mux.HandleFunc("/health", s.handleHealth)

	return s, nil
}

// ListenAndServe starts the server on the given address (e.g. "127.0.0.1:7474").
// It blocks until the server is stopped.
func (s *Server) ListenAndServe(addr string) error {
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute, // long for streaming
		IdleTimeout:  120 * time.Second,
	}
	s.logger.Printf("[gateway] listening on http://%s", addr)
	return s.httpSrv.ListenAndServe()
}

// ListenOnFreePort starts the server on a random free localhost port.
// Returns the listener so the caller can retrieve the actual port.
func (s *Server) ListenOnFreePort() (net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s.httpSrv = &http.Server{
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Printf("[gateway] serve error: %v", err)
		}
	}()
	return ln, nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	// Return a minimal models list from the desk config
	type modelEntry struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	var models []modelEntry

	if s.desk.LLM != nil {
		if s.desk.LLM.DefaultModel != "" {
			models = append(models, modelEntry{
				ID:      s.desk.LLM.DefaultProvider + "/" + s.desk.LLM.DefaultModel,
				Object:  "model",
				OwnedBy: s.desk.LLM.DefaultProvider,
			})
		}
		for _, entry := range s.desk.LLM.Routing {
			provider, model := splitRoutingEntry(entry)
			models = append(models, modelEntry{
				ID:      provider + "/" + model,
				Object:  "model",
				OwnedBy: provider,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   models,
	})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is supported")
		return
	}

	// --- Parse traceparent ---
	tc := trace.ParseOrNew(r.Header.Get(trace.TraceParentHeader))
	childSpan, _ := tc.NewSpan()

	// --- Parse request body ---
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON: "+err.Error())
		return
	}

	// --- Read routing hints from headers (take priority over body fields) ---
	if v := r.Header.Get("X-Task-Type"); v != "" {
		req.XTaskType = v
	}
	if v := r.Header.Get("X-Model"); v != "" {
		req.XModel = v
	}

	// --- Resolve model via routing hierarchy ---
	pathDir := r.Header.Get("X-Path-Dir") // optional: directory of the file being edited
	resolution := desk.ResolveModel(s.desk, req.XTaskType, req.XModel, pathDir)

	provider := resolution.Provider
	model := resolution.Model

	// If the request already specifies a model in "provider/model" format, use that.
	if req.Model != "" && strings.Contains(req.Model, "/") {
		provider, model = splitRoutingEntry(req.Model)
	} else if req.Model != "" && resolution.Source == "global" {
		// Model specified without provider — use desk default provider
		model = req.Model
	}

	// --- Budget check ---
	budgetStatus, budgetErr := s.budget.Check()
	if budgetErr != nil {
		s.writeError(w, http.StatusTooManyRequests, "budget_exceeded", budgetErr.Error())
		return
	}

	// Add budget headers
	if budgetStatus.LimitUSD > 0 {
		w.Header().Set("X-Budget-Remaining", fmt.Sprintf("%.4f", budgetStatus.Remaining))
		w.Header().Set("X-Budget-Limit", fmt.Sprintf("%.2f", budgetStatus.LimitUSD))
		if budgetStatus.Warned {
			w.Header().Set("X-Budget-Warning", "approaching-limit")
		}
	}

	// --- Add trace headers ---
	w.Header().Set(trace.TraceParentHeader, childSpan.Header())
	w.Header().Set("X-Routing-Source", resolution.Source)
	w.Header().Set("X-Provider", provider)
	w.Header().Set("X-Model-Used", model)

	// --- Build provider ---
	p, err := s.buildProvider(provider)
	if err != nil {
		s.logger.Printf("[gateway] build provider %q: %v", provider, err)
		// Try fallback chain
		p, provider, model, err = s.tryFallback(provider, model)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "provider_unavailable", err.Error())
			return
		}
	}

	// --- Dispatch ---
	var usage *Usage
	var statusCode int

	if req.Stream {
		usage, statusCode, err = s.handleStream(r.Context(), w, p, model, &req, childSpan)
	} else {
		usage, statusCode, err = s.handleNonStream(r.Context(), w, p, model, &req, childSpan)
	}

	// --- Record ledger entry ---
	durationMS := time.Since(start).Milliseconds()
	entry := ledger.Entry{
		TraceID:       childSpan.TraceID,
		SpanID:        childSpan.SpanID,
		Desk:          s.desk.DeskMeta.Name,
		Provider:      provider,
		Model:         model,
		TaskType:      req.XTaskType,
		DurationMS:    durationMS,
		RoutingSource: resolution.Source,
		StatusCode:    statusCode,
		IsLocal:       ledger.IsLocalProvider(provider),
	}

	if usage != nil {
		entry.InputTokens = usage.PromptTokens
		entry.OutputTokens = usage.CompletionTokens
		entry.CacheReadTokens = usage.CacheReadInputTokens
		entry.CacheWriteTokens = usage.CacheCreationInputTokens
		entry.CostUSD = ledger.ComputeCost(provider, model,
			entry.InputTokens, entry.OutputTokens,
			entry.CacheReadTokens, entry.CacheWriteTokens)
	}

	if err != nil {
		entry.ErrorMessage = err.Error()
	}

	if appendErr := s.ledger.Append(entry); appendErr != nil {
		s.logger.Printf("[gateway] ledger append: %v", appendErr)
	}
}

func (s *Server) handleNonStream(ctx context.Context, w http.ResponseWriter, p Provider, model string, req *ChatRequest, tc *trace.Context) (*Usage, int, error) {
	resp, err := p.Chat(ctx, model, req)
	if err != nil {
		var pErr *ProviderError
		if errors.As(err, &pErr) {
			s.writeError(w, pErr.StatusCode, "provider_error", pErr.Message)
			return nil, pErr.StatusCode, err
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return nil, http.StatusInternalServerError, err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	return resp.Usage, http.StatusOK, nil
}

func (s *Server) handleStream(ctx context.Context, w http.ResponseWriter, p Provider, model string, req *ChatRequest, tc *trace.Context) (*Usage, int, error) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Flush immediately so the client sees headers
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Wrap writer with flusher so each chunk is sent immediately
	fw := &flushWriter{w: w}

	usage, err := p.ChatStream(ctx, model, req, fw)
	if err != nil {
		var pErr *ProviderError
		if errors.As(err, &pErr) {
			return usage, pErr.StatusCode, err
		}
		return usage, http.StatusInternalServerError, err
	}
	return usage, http.StatusOK, nil
}

// tryFallback attempts to build a provider from the desk fallback chain.
func (s *Server) tryFallback(failedProvider, model string) (Provider, string, string, error) {
	chain := desk.FallbackChain(s.desk)
	for _, entry := range chain {
		if entry == failedProvider {
			continue // skip the one that already failed
		}
		p, m := splitRoutingEntry(entry)
		if m == "" {
			m = model // keep the original model for same-provider fallback
		}
		provider, err := s.buildProvider(p)
		if err != nil {
			s.logger.Printf("[gateway] fallback provider %q unavailable: %v", p, err)
			continue
		}
		s.logger.Printf("[gateway] falling back to provider=%q model=%q", p, m)
		return provider, p, m, nil
	}
	return nil, "", "", fmt.Errorf("all providers in fallback chain failed")
}

// buildProvider creates a Provider for the given provider name, loading its API key.
func (s *Server) buildProvider(provider string) (Provider, error) {
	switch provider {
	case "anthropic":
		key, err := s.secrets.APIKey("anthropic")
		if err != nil {
			return nil, fmt.Errorf("anthropic API key: %w", err)
		}
		return newAnthropicProvider(key), nil
	case "openai":
		key, err := s.secrets.APIKey("openai")
		if err != nil {
			return nil, fmt.Errorf("openai API key: %w", err)
		}
		return newOpenAIProvider(key), nil
	case "google":
		key, err := s.secrets.APIKey("google")
		if err != nil {
			return nil, fmt.Errorf("google API key: %w", err)
		}
		return newGoogleProvider(key), nil
	case "ollama":
		baseURL := desk.OllamaBaseURL(s.desk)
		return newOllamaProvider(baseURL), nil
	default:
		return nil, fmt.Errorf("unknown provider: %q", provider)
	}
}

func (s *Server) writeError(w http.ResponseWriter, statusCode int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Message: msg,
			Type:    errType,
			Code:    strconv.Itoa(statusCode),
		},
	})
}

// splitRoutingEntry splits "provider/model" into its parts.
// Returns ("", entry) if no slash is present.
func splitRoutingEntry(entry string) (provider, model string) {
	idx := strings.Index(entry, "/")
	if idx < 0 {
		return "", entry
	}
	return entry[:idx], entry[idx+1:]
}

// flushWriter wraps an http.ResponseWriter and flushes after every Write call.
type flushWriter struct {
	w http.ResponseWriter
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if flusher, ok := fw.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}

// MapSecretResolver is a SecretResolver backed by a simple string map.
// Useful for tests and local dev without a keychain.
type MapSecretResolver struct {
	Keys map[string]string
}

func (m *MapSecretResolver) APIKey(provider string) (string, error) {
	if key, ok := m.Keys[provider]; ok && key != "" {
		return key, nil
	}
	return "", fmt.Errorf("no API key configured for provider %q", provider)
}
