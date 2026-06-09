package desk

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// PathOverride is a minimal TOML file (.packmule.toml) that can live in any
// subdirectory to override the desk's LLM routing for that subtree.
type PathOverride struct {
	LLM *LLMConfig `toml:"llm,omitempty"`
}

// RoutingResolution is the result of resolving which model to use for a request.
type RoutingResolution struct {
	// Provider is the resolved provider name e.g. "anthropic", "openai", "google", "ollama".
	Provider string

	// Model is the resolved model name e.g. "claude-sonnet-4-20250514" or "gemma3:2b".
	Model string

	// Source describes what tier resolved this: "request", "task", "path", "desk", "global".
	Source string
}

const (
	// DefaultGatewayPort is the default localhost port for the Saddlebag gateway.
	DefaultGatewayPort = "7474"

	// DefaultGatewayURL is the default gateway base URL.
	DefaultGatewayURL = "http://localhost:" + DefaultGatewayPort

	// PathOverrideFileName is the name of the per-directory LLM override file.
	PathOverrideFileName = ".packmule.toml"
)

// GatewayURL returns the effective gateway URL for a desk, falling back to the default.
func GatewayURL(d *Desk) string {
	if d.Harness != nil && d.Harness.GatewayURL != "" {
		return d.Harness.GatewayURL
	}
	return DefaultGatewayURL
}

// ResolveModel resolves the effective provider+model for a given request.
//
// The resolution hierarchy is:
//  1. requestOverride — explicit "provider/model" string from the caller (e.g. X-Model header)
//  2. taskType        — key into desk [llm.routing] e.g. "plan", "implement", "classify"
//  3. pathDir         — walk up from pathDir looking for .packmule.toml with [llm.routing]
//  4. desk [llm]      — desk-level default_provider + default_model
//  5. hardcoded global default
//
// requestOverride and pathDir may be empty strings to skip those tiers.
func ResolveModel(d *Desk, taskType, requestOverride, pathDir string) RoutingResolution {
	// Tier 1: explicit request-level override
	if requestOverride != "" {
		provider, model := splitProviderModel(requestOverride, d)
		return RoutingResolution{Provider: provider, Model: model, Source: "request"}
	}

	// Tier 2: task-level routing from desk config
	if taskType != "" && d.LLM != nil && d.LLM.Routing != nil {
		if entry, ok := d.LLM.Routing[taskType]; ok && entry != "" {
			provider, model := splitProviderModel(entry, d)
			return RoutingResolution{Provider: provider, Model: model, Source: "task"}
		}
	}

	// Tier 3: path-level .packmule.toml override (walk up from pathDir)
	if pathDir != "" {
		if res, ok := resolveFromPath(pathDir, taskType, d); ok {
			return res
		}
	}

	// Tier 4: desk-level default
	if d.LLM != nil && d.LLM.DefaultProvider != "" {
		return RoutingResolution{
			Provider: d.LLM.DefaultProvider,
			Model:    d.LLM.DefaultModel,
			Source:   "desk",
		}
	}

	// Tier 5: hardcoded global default
	return RoutingResolution{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-20250514",
		Source:   "global",
	}
}

// FallbackChain returns the ordered provider fallback chain for a desk.
// The first entry is the primary; subsequent entries are tried in order.
func FallbackChain(d *Desk) []string {
	if d.LLM == nil || d.LLM.Fallback == nil {
		return []string{"anthropic", "openai"}
	}
	chain := make([]string, 0, 1+len(d.LLM.Fallback.Fallback))
	if d.LLM.Fallback.Primary != "" {
		chain = append(chain, d.LLM.Fallback.Primary)
	}
	chain = append(chain, d.LLM.Fallback.Fallback...)
	return chain
}

// OllamaBaseURL returns the configured Ollama base URL, or the default.
func OllamaBaseURL(d *Desk) string {
	if d.LLM != nil && d.LLM.Providers != nil {
		if cfg, ok := d.LLM.Providers["ollama"]; ok && cfg.BaseURL != "" {
			return cfg.BaseURL
		}
	}
	return "http://localhost:11434"
}

// splitProviderModel splits a "provider/model" string into its components.
// If there is no "/" separator, the whole string is treated as the model and
// the desk's default provider (or "anthropic") is used.
func splitProviderModel(entry string, d *Desk) (provider, model string) {
	if idx := strings.Index(entry, "/"); idx >= 0 {
		return entry[:idx], entry[idx+1:]
	}
	// No slash — treat as model name, use desk default provider
	if d.LLM != nil && d.LLM.DefaultProvider != "" {
		return d.LLM.DefaultProvider, entry
	}
	return "anthropic", entry
}

// resolveFromPath walks from pathDir up to / looking for .packmule.toml files
// that contain [llm.routing] entries for the given taskType.
func resolveFromPath(pathDir, taskType string, d *Desk) (RoutingResolution, bool) {
	dir := pathDir
	for {
		candidate := filepath.Join(dir, PathOverrideFileName)
		data, err := os.ReadFile(candidate)
		if err == nil {
			var override PathOverride
			if err := toml.Unmarshal(data, &override); err == nil {
				if override.LLM != nil && override.LLM.Routing != nil {
					if taskType != "" {
						if entry, ok := override.LLM.Routing[taskType]; ok && entry != "" {
							provider, model := splitProviderModel(entry, d)
							return RoutingResolution{Provider: provider, Model: model, Source: "path"}, true
						}
					}
					// No task-type match but file exists — don't fall through to default
					// from this file; continue walking up for a more specific match.
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return RoutingResolution{}, false
}
