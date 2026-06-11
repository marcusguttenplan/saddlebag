package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/marcusguttenplan/sb/internal/desk"
	"github.com/marcusguttenplan/sb/internal/ledger"
	"github.com/marcusguttenplan/sb/internal/trace"
)

// Server is the saddlebag policy proxy.
//
// It sits in front of the Packmule gateway (pm serve, default :7475) and:
//  1. Resolves routing metadata (provider, model, task type) from the request
//  2. Performs a Cedar authorization check (Phase 1a: allow-all stub)
//  3. Reverse-proxies authorized requests to pm on PackmuleURL
//  4. Records an audit trail entry in the saddlebag ledger (~/.saddlebag/ledger/)
//     (budget enforcement lives in pm; sb only writes attribution/identity audit rows)
//
// /v1/ledger and /health are served locally — they never hit pm.
// /v1/models and /v1/chat/completions are proxied through.
type Server struct {
	mux        *http.ServeMux
	httpSrv    *http.Server
	proxy      *httputil.ReverseProxy
	packmuleURL string
	ledger     *ledger.Ledger
	desk       *desk.Desk
	logger     *log.Logger
}

// SecretResolver satisfies the gateway.Config API used by cmd/gateway.go.
// The proxy does not use API keys itself — they live in pm — but the interface
// is kept so cmd code does not need conditional compilation.
type SecretResolver interface {
	APIKey(provider string) (string, error)
}

// Config holds everything needed to create a policy-proxy Server.
type Config struct {
	// Desk is the active desk configuration (used for routing metadata only).
	Desk *desk.Desk

	// Ledger is the saddlebag audit ledger (~/.saddlebag/ledger/).
	Ledger *ledger.Ledger

	// Secrets is accepted for API compatibility but unused by the proxy.
	// API keys are held by pm, not sb.
	Secrets SecretResolver

	// Logger, if nil defaults to the standard logger.
	Logger *log.Logger

	// PackmuleURL is the base URL of the pm gateway process.
	// Default: "http://localhost:7475"
	PackmuleURL string
}

// New creates a new policy-proxy Server.
func New(cfg Config) (*Server, error) {
	if cfg.Ledger == nil {
		return nil, fmt.Errorf("gateway: ledger is required")
	}
	if cfg.Desk == nil {
		cfg.Desk = &desk.Desk{}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	pmURL := cfg.PackmuleURL
	if pmURL == "" {
		pmURL = "http://localhost:7475"
	}

	target, err := url.Parse(pmURL)
	if err != nil {
		return nil, fmt.Errorf("gateway: invalid packmule URL %q: %w", pmURL, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Printf("[proxy] upstream error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "Packmule gateway unavailable: " + err.Error(),
				"type":    "upstream_error",
				"code":    strconv.Itoa(http.StatusBadGateway),
			},
		})
	}

	s := &Server{
		mux:         http.NewServeMux(),
		proxy:       proxy,
		packmuleURL: pmURL,
		ledger:      cfg.Ledger,
		desk:        cfg.Desk,
		logger:      logger,
	}

	// Locally served routes
	s.mux.HandleFunc("/v1/ledger", s.handleLedger)
	s.mux.HandleFunc("/health", s.handleHealth)

	// Proxied routes — with routing metadata injection
	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/v1/models", s.handleProxy)
	s.mux.HandleFunc("/v1/embeddings", s.handleProxy)

	return s, nil
}

// ListenAndServe starts the server on the given address (e.g. "127.0.0.1:7474").
func (s *Server) ListenAndServe(addr string) error {
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}
	s.logger.Printf("[proxy] listening on http://%s → pm %s", addr, s.packmuleURL)
	return s.httpSrv.ListenAndServe()
}

// ListenOnFreePort starts the server on a random free localhost port.
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
	go s.httpSrv.Serve(ln) //nolint:errcheck
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
// Locally served handlers
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Also check pm health and include it in the response.
	pmHealthy := s.checkPMHealth()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"pm":       pmHealthyStatus(pmHealthy),
		"pm_url":   s.packmuleURL,
	})
}

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported")
		return
	}
	summary, err := s.ledger.TodaySummary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ledger_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// ---------------------------------------------------------------------------
// Chat completions: policy check → enrich headers → proxy → audit log
// ---------------------------------------------------------------------------

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is supported")
		return
	}

	// Parse traceparent — propagate to pm.
	tc := trace.ParseOrNew(r.Header.Get(trace.TraceParentHeader))
	childSpan, _ := tc.NewSpan()
	r.Header.Set(trace.TraceParentHeader, childSpan.Header())

	// Phase 1a Cedar stub: allow all.
	// Phase 1b: evaluate Cedar policy with desk identity + task type.
	// if !s.cedarAllow(r) { writeError(w, 403, "forbidden", "Cedar policy denied"); return }

	// Inject routing metadata so pm can skip re-resolution if desired.
	// pm still does its own routing; these headers are informational.
	var taskType string
	if v := r.Header.Get("X-Task-Type"); v != "" {
		taskType = v
	}

	if s.desk != nil {
		resolution := desk.ResolveModel(s.desk, taskType, r.Header.Get("X-Model"), r.Header.Get("X-Path-Dir"))
		// Only set these if pm hasn't already received them from the client.
		if r.Header.Get("X-Sb-Provider") == "" {
			r.Header.Set("X-Sb-Provider", resolution.Provider)
			r.Header.Set("X-Sb-Model", resolution.Model)
			r.Header.Set("X-Sb-Routing-Source", resolution.Source)
		}
	}

	// Use a response recorder to capture headers pm sends back.
	rr := newResponseRecorder(w)
	s.proxy.ServeHTTP(rr, r)

	// Audit trail: write a thin ledger entry to ~/ .saddlebag/ledger/ with
	// attribution info. Token counts come from X-* headers pm sets on the response.
	entry := ledger.Entry{
		TraceID:       childSpan.TraceID,
		SpanID:        childSpan.SpanID,
		Desk:          s.desk.DeskMeta.Name,
		Provider:      coalesce(rr.Header().Get("X-Provider"), r.Header.Get("X-Sb-Provider")),
		Model:         coalesce(rr.Header().Get("X-Model-Used"), r.Header.Get("X-Sb-Model")),
		TaskType:      taskType,
		DurationMS:    time.Since(start).Milliseconds(),
		RoutingSource: coalesce(rr.Header().Get("X-Routing-Source"), "proxy"),
		StatusCode:    rr.status,
	}

	if err := s.ledger.Append(entry); err != nil {
		s.logger.Printf("[proxy] audit ledger append: %v", err)
	}
}

// handleProxy is a bare reverse-proxy handler for routes that need no special treatment.
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	s.proxy.ServeHTTP(w, r)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *Server) checkPMHealth() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.packmuleURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func pmHealthyStatus(ok bool) string {
	if ok {
		return "ok"
	}
	return "unavailable"
}

func writeError(w http.ResponseWriter, statusCode int, errType, msg string) {
	if statusCode <= 0 || statusCode > 999 {
		statusCode = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"message": msg,
			"type":    errType,
			"code":    strconv.Itoa(statusCode),
		},
	})
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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

// responseRecorder wraps http.ResponseWriter to capture the status code and
// response headers set by pm (for audit logging).
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

// MapSecretResolver is kept for API compatibility with cmd/gateway.go.
// The proxy never calls APIKey — keys live in pm.
type MapSecretResolver struct {
	Keys map[string]string
}

func (m *MapSecretResolver) APIKey(provider string) (string, error) {
	if key, ok := m.Keys[provider]; ok && key != "" {
		return key, nil
	}
	return "", fmt.Errorf("no API key configured for provider %q", provider)
}
