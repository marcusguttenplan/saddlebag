package desk

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTOML is a test helper that writes a TOML file to a temp directory.
func writeTOML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTOML: %v", err)
	}
	return path
}

func TestDeskLLMConfig_ParseFull(t *testing.T) {
	tmp := t.TempDir()
	toml := `
[desk]
name = "myproject"

[llm]
default_provider = "anthropic"
default_model    = "claude-sonnet-4-20250514"

[llm.routing]
plan      = "anthropic/claude-opus-4-20250514"
implement = "anthropic/claude-sonnet-4-20250514"
classify  = "ollama/gemma3:2b"
review    = "anthropic/claude-opus-4-20250514"

[llm.budget]
daily_limit_usd = 50.00
warn_at_usd     = 40.00

[llm.fallback]
primary  = "anthropic"
fallback = ["openai", "ollama/gemma3:latest"]

[llm.providers.ollama]
base_url = "http://localhost:11434"
models   = ["gemma3:latest", "gemma3:2b", "llama3.1:8b"]

[harness]
max_context_tokens     = 150000
progressive_disclosure = true
gateway_url            = "http://localhost:7474"
`
	path := writeTOML(t, tmp, "myproject.toml", toml)
	d, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if d.LLM == nil {
		t.Fatal("expected LLM config, got nil")
	}
	if d.LLM.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider: got %q, want %q", d.LLM.DefaultProvider, "anthropic")
	}
	if d.LLM.DefaultModel != "claude-sonnet-4-20250514" {
		t.Errorf("DefaultModel: got %q, want %q", d.LLM.DefaultModel, "claude-sonnet-4-20250514")
	}
	if d.LLM.Routing == nil {
		t.Fatal("expected Routing map, got nil")
	}
	if got := d.LLM.Routing["plan"]; got != "anthropic/claude-opus-4-20250514" {
		t.Errorf("Routing[plan]: got %q, want %q", got, "anthropic/claude-opus-4-20250514")
	}
	if got := d.LLM.Routing["classify"]; got != "ollama/gemma3:2b" {
		t.Errorf("Routing[classify]: got %q, want %q", got, "ollama/gemma3:2b")
	}
	if d.LLM.Budget == nil {
		t.Fatal("expected Budget, got nil")
	}
	if d.LLM.Budget.DailyLimitUSD != 50.00 {
		t.Errorf("DailyLimitUSD: got %f, want 50.00", d.LLM.Budget.DailyLimitUSD)
	}
	if d.LLM.Budget.WarnAtUSD != 40.00 {
		t.Errorf("WarnAtUSD: got %f, want 40.00", d.LLM.Budget.WarnAtUSD)
	}
	if d.LLM.Fallback == nil {
		t.Fatal("expected Fallback, got nil")
	}
	if d.LLM.Fallback.Primary != "anthropic" {
		t.Errorf("Fallback.Primary: got %q, want %q", d.LLM.Fallback.Primary, "anthropic")
	}
	if len(d.LLM.Fallback.Fallback) != 2 {
		t.Errorf("Fallback chain: got %d entries, want 2", len(d.LLM.Fallback.Fallback))
	}
	ollama, ok := d.LLM.Providers["ollama"]
	if !ok {
		t.Fatal("expected ollama provider config")
	}
	if ollama.BaseURL != "http://localhost:11434" {
		t.Errorf("ollama BaseURL: got %q, want %q", ollama.BaseURL, "http://localhost:11434")
	}
	if len(ollama.Models) != 3 {
		t.Errorf("ollama Models: got %d, want 3", len(ollama.Models))
	}
	if d.Harness == nil {
		t.Fatal("expected Harness config, got nil")
	}
	if d.Harness.GatewayURL != "http://localhost:7474" {
		t.Errorf("GatewayURL: got %q", d.Harness.GatewayURL)
	}
	if !d.Harness.ProgressiveDisclosure {
		t.Error("expected ProgressiveDisclosure=true")
	}
}

func TestDeskLLMConfig_ParseMinimal(t *testing.T) {
	// Existing desks without [llm] must still parse cleanly (backward compat).
	tmp := t.TempDir()
	toml := `
[desk]
name = "legacy"

[aws]
profile = "my-profile"
`
	path := writeTOML(t, tmp, "legacy.toml", toml)
	d, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if d.LLM != nil {
		t.Errorf("expected LLM nil for legacy desk, got %+v", d.LLM)
	}
	if d.Harness != nil {
		t.Errorf("expected Harness nil for legacy desk, got %+v", d.Harness)
	}
}

// ---------------------------------------------------------------------------
// Routing resolution tests
// ---------------------------------------------------------------------------

func minimalDesk() *Desk {
	return &Desk{
		DeskMeta: DeskMeta{Name: "test"},
		LLM: &LLMConfig{
			DefaultProvider: "anthropic",
			DefaultModel:    "claude-sonnet-4-20250514",
			Routing: map[string]string{
				"plan":      "anthropic/claude-opus-4-20250514",
				"classify":  "ollama/gemma3:2b",
				"implement": "anthropic/claude-sonnet-4-20250514",
			},
		},
	}
}

func TestResolveModel_RequestOverride(t *testing.T) {
	d := minimalDesk()
	r := ResolveModel(d, "plan", "openai/gpt-4o", "")
	if r.Provider != "openai" || r.Model != "gpt-4o" {
		t.Errorf("got provider=%q model=%q, want openai/gpt-4o", r.Provider, r.Model)
	}
	if r.Source != "request" {
		t.Errorf("source: got %q, want request", r.Source)
	}
}

func TestResolveModel_TaskRouting(t *testing.T) {
	d := minimalDesk()
	r := ResolveModel(d, "plan", "", "")
	if r.Provider != "anthropic" || r.Model != "claude-opus-4-20250514" {
		t.Errorf("got provider=%q model=%q, want anthropic/claude-opus-4-20250514", r.Provider, r.Model)
	}
	if r.Source != "task" {
		t.Errorf("source: got %q, want task", r.Source)
	}
}

func TestResolveModel_OllamaTask(t *testing.T) {
	d := minimalDesk()
	r := ResolveModel(d, "classify", "", "")
	if r.Provider != "ollama" || r.Model != "gemma3:2b" {
		t.Errorf("got provider=%q model=%q, want ollama/gemma3:2b", r.Provider, r.Model)
	}
}

func TestResolveModel_DeskDefault(t *testing.T) {
	d := minimalDesk()
	// Task type not in routing → falls through to desk default
	r := ResolveModel(d, "unknown-task", "", "")
	if r.Provider != "anthropic" || r.Model != "claude-sonnet-4-20250514" {
		t.Errorf("got provider=%q model=%q, want anthropic/claude-sonnet-4-20250514", r.Provider, r.Model)
	}
	if r.Source != "desk" {
		t.Errorf("source: got %q, want desk", r.Source)
	}
}

func TestResolveModel_GlobalDefault(t *testing.T) {
	d := &Desk{DeskMeta: DeskMeta{Name: "empty"}}
	r := ResolveModel(d, "", "", "")
	if r.Provider != "anthropic" || r.Model != "claude-sonnet-4-20250514" {
		t.Errorf("got provider=%q model=%q", r.Provider, r.Model)
	}
	if r.Source != "global" {
		t.Errorf("source: got %q, want global", r.Source)
	}
}

func TestResolveModel_PathOverride(t *testing.T) {
	tmp := t.TempDir()
	// "review" is NOT in minimalDesk()'s routing, so tier 2 misses and
	// falls through to tier 3 (path). The .packmule.toml provides it.
	overrideContent := `
[llm.routing]
review = "openai/gpt-4o"
`
	writeTOML(t, tmp, PathOverrideFileName, overrideContent)

	d := minimalDesk()
	r := ResolveModel(d, "review", "", tmp)
	if r.Provider != "openai" || r.Model != "gpt-4o" {
		t.Errorf("got provider=%q model=%q, want openai/gpt-4o", r.Provider, r.Model)
	}
	if r.Source != "path" {
		t.Errorf("source: got %q, want path", r.Source)
	}
}

func TestResolveModel_PathOverrideMiss(t *testing.T) {
	tmp := t.TempDir()
	// .packmule.toml exists but doesn't have the "verify" task.
	// "verify" is also absent from minimalDesk() routing, so it falls
	// all the way to the desk default.
	overrideContent := `
[llm.routing]
review = "openai/gpt-4o"
`
	writeTOML(t, tmp, PathOverrideFileName, overrideContent)

	d := minimalDesk()
	// "verify" not in path override and not in desk routing → desk default
	r := ResolveModel(d, "verify", "", tmp)
	if r.Source == "path" {
		t.Errorf("expected fallthrough from path, got path source for unmatched task")
	}
	if r.Source != "desk" {
		t.Errorf("expected desk default fallback, got source=%q", r.Source)
	}
}

func TestFallbackChain(t *testing.T) {
	d := minimalDesk()
	d.LLM.Fallback = &LLMFallback{
		Primary:  "anthropic",
		Fallback: []string{"openai", "ollama/gemma3:latest"},
	}
	chain := FallbackChain(d)
	if len(chain) != 3 {
		t.Fatalf("got %d entries, want 3", len(chain))
	}
	if chain[0] != "anthropic" {
		t.Errorf("chain[0]: got %q, want anthropic", chain[0])
	}
}

func TestFallbackChain_Default(t *testing.T) {
	d := &Desk{DeskMeta: DeskMeta{Name: "empty"}}
	chain := FallbackChain(d)
	if len(chain) != 2 {
		t.Fatalf("got %d entries, want 2 (default)", len(chain))
	}
}

func TestOllamaBaseURL_Default(t *testing.T) {
	d := &Desk{DeskMeta: DeskMeta{Name: "empty"}}
	if got := OllamaBaseURL(d); got != "http://localhost:11434" {
		t.Errorf("got %q, want default ollama URL", got)
	}
}

func TestGatewayURL_Default(t *testing.T) {
	d := &Desk{DeskMeta: DeskMeta{Name: "empty"}}
	if got := GatewayURL(d); got != DefaultGatewayURL {
		t.Errorf("got %q, want %q", got, DefaultGatewayURL)
	}
}

func TestGatewayURL_Custom(t *testing.T) {
	d := &Desk{
		DeskMeta: DeskMeta{Name: "test"},
		Harness:  &HarnessConfig{GatewayURL: "http://localhost:9090"},
	}
	if got := GatewayURL(d); got != "http://localhost:9090" {
		t.Errorf("got %q, want http://localhost:9090", got)
	}
}
