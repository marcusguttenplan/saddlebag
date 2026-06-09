// Package ledger implements the append-only token ledger for the Packmule gateway.
//
// The ledger records every model call with token counts, cost, and trace context.
// It is stored as newline-delimited JSON (JSONL) in daily rotating files:
//
//	~/.saddlebag/ledger/YYYY-MM-DD.jsonl
//
// Each line is a LedgerEntry. Writes are atomic (os.O_APPEND | os.O_SYNC) so
// the ledger is safe to read from Campfire or `sb ledger show` while the gateway
// is writing to it.
package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is a single record appended to the ledger for each model API call.
type Entry struct {
	// Timestamp is the UTC completion time in RFC3339Nano format.
	Timestamp string `json:"ts"`

	// TraceID is the W3C traceparent trace-id component (32 hex chars).
	TraceID string `json:"trace_id,omitempty"`

	// SpanID is the W3C traceparent parent-id component (16 hex chars).
	SpanID string `json:"span_id,omitempty"`

	// Desk is the active desk name when the call was made.
	Desk string `json:"desk,omitempty"`

	// Provider is the upstream provider used: "anthropic", "openai", "google", "ollama".
	Provider string `json:"provider"`

	// Model is the exact model name sent to the provider.
	Model string `json:"model"`

	// TaskType is the routing task type: "plan", "implement", "classify", "review", "verify".
	TaskType string `json:"task_type,omitempty"`

	// InputTokens is the number of prompt tokens reported by the provider.
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the number of completion tokens reported by the provider.
	OutputTokens int `json:"output_tokens"`

	// CacheReadTokens is the number of tokens read from the provider's prompt cache.
	// Non-zero for Anthropic when cache_control breakpoints are active.
	CacheReadTokens int `json:"cache_read_tokens,omitempty"`

	// CacheWriteTokens is the number of tokens written to the provider's prompt cache.
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`

	// CostUSD is the computed cost in US dollars. Zero for local (Ollama) models.
	CostUSD float64 `json:"cost_usd"`

	// IsLocal is true when the model was served by a local provider (Ollama).
	// Cost is always 0.00 for local models.
	IsLocal bool `json:"is_local,omitempty"`

	// DurationMS is the wall-clock latency from gateway receipt to first byte
	// of the upstream response (excludes streaming transfer time).
	DurationMS int64 `json:"duration_ms"`

	// RoutingSource indicates which routing tier resolved this model:
	// "request", "task", "skill", "path", "desk", "global".
	RoutingSource string `json:"routing_source,omitempty"`

	// StatusCode is the HTTP status returned by the upstream provider.
	// 200 on success; 4xx/5xx on error.
	StatusCode int `json:"status_code"`

	// ErrorMessage holds the provider error message on non-200 responses.
	ErrorMessage string `json:"error,omitempty"`
}

// Ledger manages the append-only JSONL ledger files.
type Ledger struct {
	dir string
	mu  sync.Mutex
}

// New creates a Ledger that writes to the given directory.
// The directory is created if it does not exist.
func New(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("ledger: create dir %s: %w", dir, err)
	}
	return &Ledger{dir: dir}, nil
}

// Append writes a single entry to today's ledger file.
// It is safe to call from multiple goroutines.
func (l *Ledger) Append(e Entry) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("ledger: marshal entry: %w", err)
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	path := l.todayPath()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("ledger: open %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("ledger: write: %w", err)
	}
	return nil
}

// DailySummary returns aggregated spend for the given date (YYYY-MM-DD).
func (l *Ledger) DailySummary(date string) (*Summary, error) {
	path := filepath.Join(l.dir, date+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Summary{Date: date}, nil
		}
		return nil, fmt.Errorf("ledger: read %s: %w", path, err)
	}

	s := &Summary{Date: date}
	byModel := make(map[string]*ModelSummary)

	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		s.TotalCostUSD += e.CostUSD
		s.TotalInputTokens += e.InputTokens
		s.TotalOutputTokens += e.OutputTokens
		s.CallCount++
		if e.StatusCode != 200 && e.StatusCode != 0 {
			s.ErrorCount++
		}

		key := e.Provider + "/" + e.Model
		if _, ok := byModel[key]; !ok {
			byModel[key] = &ModelSummary{Provider: e.Provider, Model: e.Model}
		}
		ms := byModel[key]
		ms.CostUSD += e.CostUSD
		ms.InputTokens += e.InputTokens
		ms.OutputTokens += e.OutputTokens
		ms.CallCount++
	}

	for _, ms := range byModel {
		s.ByModel = append(s.ByModel, *ms)
	}
	return s, nil
}

// TodaySummary returns the summary for today's date.
func (l *Ledger) TodaySummary() (*Summary, error) {
	return l.DailySummary(todayDate())
}

// todayPath returns the full path to today's ledger file.
func (l *Ledger) todayPath() string {
	return filepath.Join(l.dir, todayDate()+".jsonl")
}

func todayDate() string {
	return time.Now().UTC().Format("2006-01-02")
}

// splitLines splits data on newlines, trimming carriage returns.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := data[start:i]
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// Summary aggregates ledger entries for a single day.
type Summary struct {
	// Date is the ledger date in YYYY-MM-DD format.
	Date string `json:"date"`

	// TotalCostUSD is the total spend for the day in USD.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// TotalInputTokens is the total prompt token count for the day.
	TotalInputTokens int `json:"total_input_tokens"`

	// TotalOutputTokens is the total completion token count for the day.
	TotalOutputTokens int `json:"total_output_tokens"`

	// CallCount is the total number of API calls recorded.
	CallCount int `json:"call_count"`

	// ErrorCount is the number of calls with non-200 status.
	ErrorCount int `json:"error_count"`

	// ByModel is a per-model breakdown.
	ByModel []ModelSummary `json:"by_model,omitempty"`
}

// ModelSummary is per-model spend within a daily summary.
type ModelSummary struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CallCount    int     `json:"call_count"`
}
