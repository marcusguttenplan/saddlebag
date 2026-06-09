package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"
)

// OllamaManager manages a local `ollama serve` subprocess.
// It is only used when `sb gateway start --with-ollama` is requested.
type OllamaManager struct {
	cmd     *exec.Cmd
	baseURL string
}

// NewOllamaManager creates an OllamaManager that will manage `ollama serve`.
func NewOllamaManager(baseURL string) *OllamaManager {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaManager{baseURL: baseURL}
}

// EnsureRunning checks if Ollama is already serving; if not, starts it.
// Returns nil if Ollama is ready (either pre-existing or freshly started).
func (m *OllamaManager) EnsureRunning(ctx context.Context) error {
	if m.isReady(ctx) {
		return nil // already running — don't manage it
	}

	// Start ollama serve
	ollamaBin, err := exec.LookPath("ollama")
	if err != nil {
		return fmt.Errorf("ollama not found in PATH: %w", err)
	}

	m.cmd = exec.CommandContext(ctx, ollamaBin, "serve")
	// Don't inherit stdout/stderr by default — redirect to /dev/null unless debug
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("starting ollama serve: %w", err)
	}

	// Wait for it to become ready (up to 30s)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if m.isReady(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}

	return fmt.Errorf("ollama serve did not become ready within 30s")
}

// Stop terminates the managed ollama serve process (if we started it).
// If Ollama was already running when EnsureRunning was called, this is a no-op.
func (m *OllamaManager) Stop() {
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}
	_ = m.cmd.Process.Kill()
	_ = m.cmd.Wait()
}

// isReady returns true if Ollama is responding on its health endpoint.
func (m *OllamaManager) isReady(ctx context.Context) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, m.baseURL+"/", nil)
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

// Managed returns true if we started Ollama (vs. it was already running).
func (m *OllamaManager) Managed() bool {
	return m.cmd != nil
}
