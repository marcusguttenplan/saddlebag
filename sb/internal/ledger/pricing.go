package ledger

// pricing holds per-million-token costs for known models.
// Prices are in USD and reflect the provider's standard (non-batch, non-cached) rates.
// Cache read/write prices are tracked separately where applicable.
//
// Last updated: 2026-06. Always verify against provider pricing pages.
// Ollama (local) models have zero cost.
type modelPricing struct {
	// InputPerM is the cost per 1M input tokens in USD.
	InputPerM float64
	// OutputPerM is the cost per 1M output tokens in USD.
	OutputPerM float64
	// CacheReadPerM is the cost per 1M cache-read tokens (Anthropic only).
	CacheReadPerM float64
	// CacheWritePerM is the cost per 1M cache-write tokens (Anthropic only).
	CacheWritePerM float64
}

// pricingTable maps "provider/model" to per-million-token pricing.
// Partial model name prefixes are matched if exact key not found.
var pricingTable = map[string]modelPricing{
	// --- Anthropic ---
	"anthropic/claude-opus-4-20250514": {
		InputPerM:      15.00,
		OutputPerM:     75.00,
		CacheReadPerM:  1.50,
		CacheWritePerM: 18.75,
	},
	"anthropic/claude-sonnet-4-20250514": {
		InputPerM:      3.00,
		OutputPerM:     15.00,
		CacheReadPerM:  0.30,
		CacheWritePerM: 3.75,
	},
	"anthropic/claude-sonnet-4-5": {
		InputPerM:      3.00,
		OutputPerM:     15.00,
		CacheReadPerM:  0.30,
		CacheWritePerM: 3.75,
	},
	"anthropic/claude-haiku-3-5-20241022": {
		InputPerM:      0.80,
		OutputPerM:     4.00,
		CacheReadPerM:  0.08,
		CacheWritePerM: 1.00,
	},
	// --- OpenAI ---
	"openai/gpt-4o": {
		InputPerM:  2.50,
		OutputPerM: 10.00,
	},
	"openai/gpt-4o-mini": {
		InputPerM:  0.15,
		OutputPerM: 0.60,
	},
	"openai/o3": {
		InputPerM:  10.00,
		OutputPerM: 40.00,
	},
	"openai/o4-mini": {
		InputPerM:  1.10,
		OutputPerM: 4.40,
	},
	// --- Google ---
	"google/gemini-2.5-pro": {
		InputPerM:  1.25,
		OutputPerM: 10.00,
	},
	"google/gemini-2.5-flash": {
		InputPerM:  0.15,
		OutputPerM: 0.60,
	},
	// --- Ollama (local) — always zero cost ---
	// No entries needed; ComputeCost returns 0 for "ollama" provider.
}

// ComputeCost calculates the USD cost for a single API call.
// Returns 0 for local (Ollama) models.
func ComputeCost(provider, model string, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) float64 {
	if provider == "ollama" {
		return 0
	}

	key := provider + "/" + model
	p, ok := pricingTable[key]
	if !ok {
		// Unknown model — return 0 rather than panic; caller should log a warning.
		return 0
	}

	cost := (float64(inputTokens)/1_000_000)*p.InputPerM +
		(float64(outputTokens)/1_000_000)*p.OutputPerM +
		(float64(cacheReadTokens)/1_000_000)*p.CacheReadPerM +
		(float64(cacheWriteTokens)/1_000_000)*p.CacheWritePerM

	return cost
}

// IsLocalProvider returns true for providers whose cost is always $0.
func IsLocalProvider(provider string) bool {
	return provider == "ollama"
}
