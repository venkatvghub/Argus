package providers

import (
	"fmt"

	"github.com/venkatvghub/argus/pkg/config"
)

// ProviderStatus describes a provider's availability.
type ProviderStatus struct {
	Name            string
	Available       bool   // true if API key is set
	DefaultCheap    string
	DefaultMedium   string
	DefaultPremium  string
}

// defaultTierModels maps provider name → default models per tier.
var defaultTierModels = map[string][3]string{
	// [cheap, medium, premium]
	"anthropic": {"claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-7"},
	"openai":    {"gpt-4o-mini", "gpt-4o", "gpt-4"},
	"gemini":    {"gemini-2.0-flash", "gemini-1.5-pro", "gemini-ultra"},
}

// DetectProviders returns the status of all known providers.
func DetectProviders(cfg *config.Config) []ProviderStatus {
	specs := []struct {
		name   string
		hasKey bool
	}{
		{"anthropic", cfg.AnthropicKey != ""},
		{"openai", cfg.OpenAIKey != ""},
		{"gemini", cfg.GeminiKey != ""},
	}
	out := make([]ProviderStatus, 0, len(specs))
	for _, s := range specs {
		defaults := defaultTierModels[s.name]
		out = append(out, ProviderStatus{
			Name:           s.name,
			Available:      s.hasKey,
			DefaultCheap:   defaults[0],
			DefaultMedium:  defaults[1],
			DefaultPremium: defaults[2],
		})
	}
	return out
}

// DefaultTieredConfig builds a TieredConfig using the first available provider
// and its default tier models. Returns error if no provider has an API key.
func DefaultTieredConfig(cfg *config.Config) (TieredConfig, error) {
	statuses := DetectProviders(cfg)
	for _, s := range statuses {
		if s.Available {
			return TieredConfig{
				ProviderName: s.Name,
				CheapModel:   s.DefaultCheap,
				MediumModel:  s.DefaultMedium,
				PremiumModel: s.DefaultPremium,
			}, nil
		}
	}
	return TieredConfig{}, fmt.Errorf("no LLM provider configured: set ARGUS_OPENAI_API_KEY, ARGUS_ANTHROPIC_API_KEY, or ARGUS_GEMINI_API_KEY")
}
