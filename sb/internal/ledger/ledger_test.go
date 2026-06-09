package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLedger_AppendAndRead(t *testing.T) {
	tmp := t.TempDir()
	l, err := New(tmp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	entry := Entry{
		TraceID:      "abc123",
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		TaskType:     "implement",
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      ComputeCost("anthropic", "claude-sonnet-4-20250514", 1000, 500, 0, 0),
		DurationMS:   42,
		StatusCode:   200,
		RoutingSource: "task",
	}

	if err := l.Append(entry); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Read the file back directly
	date := time.Now().UTC().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(tmp, date+".jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var got Entry
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Provider != "anthropic" {
		t.Errorf("Provider: got %q, want anthropic", got.Provider)
	}
	if got.InputTokens != 1000 {
		t.Errorf("InputTokens: got %d, want 1000", got.InputTokens)
	}
	if got.Timestamp == "" {
		t.Error("Timestamp should be auto-set")
	}
}

func TestLedger_DailySummary(t *testing.T) {
	tmp := t.TempDir()
	l, err := New(tmp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Append 3 entries
	for i := 0; i < 3; i++ {
		if err := l.Append(Entry{
			Provider:     "anthropic",
			Model:        "claude-sonnet-4-20250514",
			InputTokens:  1000,
			OutputTokens: 500,
			CostUSD:      ComputeCost("anthropic", "claude-sonnet-4-20250514", 1000, 500, 0, 0),
			StatusCode:   200,
		}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	s, err := l.TodaySummary()
	if err != nil {
		t.Fatalf("TodaySummary: %v", err)
	}
	if s.CallCount != 3 {
		t.Errorf("CallCount: got %d, want 3", s.CallCount)
	}
	if s.TotalInputTokens != 3000 {
		t.Errorf("TotalInputTokens: got %d, want 3000", s.TotalInputTokens)
	}
	if s.TotalOutputTokens != 1500 {
		t.Errorf("TotalOutputTokens: got %d, want 1500", s.TotalOutputTokens)
	}
	if s.TotalCostUSD == 0 {
		t.Error("TotalCostUSD should be > 0")
	}
	if s.ErrorCount != 0 {
		t.Errorf("ErrorCount: got %d, want 0", s.ErrorCount)
	}
	if len(s.ByModel) != 1 {
		t.Errorf("ByModel: got %d entries, want 1", len(s.ByModel))
	}
}

func TestLedger_DailySummary_Empty(t *testing.T) {
	tmp := t.TempDir()
	l, err := New(tmp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s, err := l.DailySummary("2020-01-01")
	if err != nil {
		t.Fatalf("DailySummary on missing date: %v", err)
	}
	if s.CallCount != 0 {
		t.Errorf("expected empty summary, got %+v", s)
	}
}

func TestComputeCost_Anthropic(t *testing.T) {
	// 1M input + 1M output for Sonnet
	cost := ComputeCost("anthropic", "claude-sonnet-4-20250514", 1_000_000, 1_000_000, 0, 0)
	want := 3.00 + 15.00 // $18.00
	if cost != want {
		t.Errorf("cost: got %.4f, want %.4f", cost, want)
	}
}

func TestComputeCost_WithCache(t *testing.T) {
	// 100k cache read tokens (much cheaper)
	cost := ComputeCost("anthropic", "claude-sonnet-4-20250514", 0, 0, 100_000, 0)
	want := 0.03 // $0.30 per 1M × 0.1M
	if cost != want {
		t.Errorf("cost: got %.4f, want %.4f", cost, want)
	}
}

func TestComputeCost_Ollama(t *testing.T) {
	cost := ComputeCost("ollama", "gemma3:2b", 100_000, 50_000, 0, 0)
	if cost != 0 {
		t.Errorf("Ollama cost should be 0, got %f", cost)
	}
}

func TestComputeCost_UnknownModel(t *testing.T) {
	// Should return 0, not panic
	cost := ComputeCost("anthropic", "claude-unknown-model", 1000, 500, 0, 0)
	if cost != 0 {
		t.Errorf("Unknown model cost should be 0, got %f", cost)
	}
}

func TestLedger_MultipleAppends_Concurrent(t *testing.T) {
	tmp := t.TempDir()
	l, err := New(tmp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			done <- l.Append(Entry{
				Provider:    "openai",
				Model:       "gpt-4o",
				InputTokens: 100,
				StatusCode:  200,
			})
		}()
	}
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent Append: %v", err)
		}
	}

	s, err := l.TodaySummary()
	if err != nil {
		t.Fatalf("TodaySummary: %v", err)
	}
	if s.CallCount != 10 {
		t.Errorf("CallCount: got %d, want 10", s.CallCount)
	}
}
