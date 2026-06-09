package desk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/marcusguttenplan/sb/internal/config"
)

// Desk represents a context bundle loaded from a TOML file
type Desk struct {
	DeskMeta DeskMeta          `toml:"desk"`
	AWS      *AWSConfig        `toml:"aws,omitempty"`
	GCP      *GCPConfig        `toml:"gcp,omitempty"`
	Git      *GitConfig        `toml:"git,omitempty"`
	SSH      *SSHConfig        `toml:"ssh,omitempty"`
	Env      map[string]string `toml:"env,omitempty"`
	LLM      *LLMConfig        `toml:"llm,omitempty"`
	Harness  *HarnessConfig    `toml:"harness,omitempty"`
}

type DeskMeta struct {
	Name       string `toml:"name"`
	WorkingDir string `toml:"working_dir,omitempty"`
}

type AWSConfig struct {
	Profile string `toml:"profile"`
}

type GCPConfig struct {
	Config string `toml:"config"`
}

type GitConfig struct {
	Email string `toml:"email"`
	Name  string `toml:"name"`
}

type SSHConfig struct {
	Key string `toml:"key"`
}

// LLMConfig holds all model routing and budget configuration for a desk.
type LLMConfig struct {
	// DefaultProvider is the fallback provider when no task-level routing matches.
	// Valid values: "anthropic", "openai", "google", "ollama"
	DefaultProvider string `toml:"default_provider,omitempty"`

	// DefaultModel is the fallback model when no task-level routing matches.
	DefaultModel string `toml:"default_model,omitempty"`

	// Routing defines per-task-type model selection.
	// Keys: plan, implement, classify, review, verify
	// Values: "<provider>/<model>" e.g. "anthropic/claude-opus-4-20250514"
	//         or "ollama/gemma3:2b" for local models.
	Routing map[string]string `toml:"routing,omitempty"`

	// Budget controls daily spend limits.
	Budget *LLMBudget `toml:"budget,omitempty"`

	// Fallback defines the provider fallback chain when the primary is unavailable.
	Fallback *LLMFallback `toml:"fallback,omitempty"`

	// Providers holds provider-specific configuration keyed by provider name.
	Providers map[string]LLMProviderConfig `toml:"providers,omitempty"`
}

// LLMBudget defines daily spend controls.
type LLMBudget struct {
	// DailyLimitUSD is the hard-stop daily spend limit in USD.
	// Gateway returns HTTP 429 when this is exceeded.
	DailyLimitUSD float64 `toml:"daily_limit_usd,omitempty"`

	// WarnAtUSD is the soft threshold in USD at which a warning header is added.
	WarnAtUSD float64 `toml:"warn_at_usd,omitempty"`
}

// LLMFallback defines the provider fallback chain.
type LLMFallback struct {
	// Primary is the first-choice provider name.
	Primary string `toml:"primary,omitempty"`

	// Fallback is the ordered list of providers to try if Primary is unavailable.
	// Ollama entries use the format "ollama/<model>" e.g. "ollama/gemma3:latest".
	Fallback []string `toml:"fallback,omitempty"`
}

// LLMProviderConfig holds provider-specific settings.
// Currently only Ollama requires extra config; other providers use credentials from keychain.
type LLMProviderConfig struct {
	// BaseURL overrides the default API base URL for this provider.
	// Required for Ollama: "http://localhost:11434"
	BaseURL string `toml:"base_url,omitempty"`

	// Models is the list of locally available model names (Ollama only).
	Models []string `toml:"models,omitempty"`
}

// HarnessConfig controls context assembly behavior (Block A/B/C architecture).
type HarnessConfig struct {
	// MaxContextTokens is the soft cap on assembled context (all blocks combined).
	MaxContextTokens int `toml:"max_context_tokens,omitempty"`

	// ProgressiveDisclosure enables eliding older plan/task detail from Block B.
	ProgressiveDisclosure bool `toml:"progressive_disclosure,omitempty"`

	// GatewayURL is the base URL for the Saddlebag model gateway.
	// Defaults to http://localhost:7474 if not set.
	GatewayURL string `toml:"gateway_url,omitempty"`
}

// ID returns the desk identifier (filename without extension)
func (d *Desk) ID(filename string) string {
	return strings.TrimSuffix(filepath.Base(filename), ".toml")
}

// LoadAll reads all desk definitions from ~/.saddlebag/desks/
func LoadAll() (map[string]*Desk, error) {
	desksDir := config.DesksDir()

	entries, err := os.ReadDir(desksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*Desk{}, nil
		}
		return nil, fmt.Errorf("reading desks dir: %w", err)
	}

	desks := make(map[string]*Desk)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		d, err := LoadFile(filepath.Join(desksDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("loading desk %s: %w", entry.Name(), err)
		}

		id := strings.TrimSuffix(entry.Name(), ".toml")
		desks[id] = d
	}

	return desks, nil
}

// LoadFile reads a single desk TOML file
func LoadFile(path string) (*Desk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var d Desk
	if err := toml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &d, nil
}
