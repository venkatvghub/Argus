package providers

import (
	"fmt"
	"strings"

	"github.com/venkatvghub/argus/pkg/config"
)

// ProviderStatus describes a provider's availability and endpoint.
type ProviderStatus struct {
	Name           string
	Available      bool // true if API key is set
	Endpoint       string
	DefaultCheap   string
	DefaultMedium  string
	DefaultPremium string
}

// defaultTierModels maps provider name → default models per tier [cheap, medium, premium].
// For openrouter, sensible cross-provider defaults are used.
var defaultTierModels = map[string][3]string{
	"anthropic":  {"claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-7"},
	"openai":     {"gpt-4o-mini", "gpt-4o", "gpt-4-turbo"},
	"openrouter": {"mistralai/mistral-7b-instruct", "openai/gpt-4o", "anthropic/claude-opus-4"},
	"gemini":     {"gemini-2.0-flash", "gemini-1.5-pro", "gemini-ultra"},
}

// openaiVariant classifies the OpenAI base URL into a display name and short endpoint label.
func openaiVariant(baseURL string) (name, endpoint string) {
	switch {
	case strings.Contains(baseURL, "openrouter.ai"):
		return "openrouter", "openrouter.ai"
	case baseURL == "" || baseURL == "https://api.openai.com/v1":
		return "openai", "api.openai.com"
	default:
		// Azure, local proxy, etc.
		return "openai", baseURL
	}
}

// DetectProviders returns the status of all known providers, distinguishing
// OpenAI direct from OpenRouter (or other custom base URLs).
func DetectProviders(cfg *config.Config) []ProviderStatus {
	if cfg == nil {
		return nil
	}
	var out []ProviderStatus

	// Anthropic
	out = append(out, ProviderStatus{
		Name:           "anthropic",
		Available:      cfg.AnthropicKey != "",
		Endpoint:       "api.anthropic.com",
		DefaultCheap:   defaultTierModels["anthropic"][0],
		DefaultMedium:  defaultTierModels["anthropic"][1],
		DefaultPremium: defaultTierModels["anthropic"][2],
	})

	// OpenAI / OpenRouter / custom — split by base URL
	variantName, endpoint := openaiVariant(cfg.OpenAIBaseURL)
	out = append(out, ProviderStatus{
		Name:           variantName,
		Available:      cfg.OpenAIKey != "",
		Endpoint:       endpoint,
		DefaultCheap:   defaultTierModels[variantName][0],
		DefaultMedium:  defaultTierModels[variantName][1],
		DefaultPremium: defaultTierModels[variantName][2],
	})

	// Gemini
	out = append(out, ProviderStatus{
		Name:           "gemini",
		Available:      cfg.GeminiKey != "",
		Endpoint:       "generativelanguage.googleapis.com",
		DefaultCheap:   defaultTierModels["gemini"][0],
		DefaultMedium:  defaultTierModels["gemini"][1],
		DefaultPremium: defaultTierModels["gemini"][2],
	})

	return out
}

// DefaultTieredConfig builds a TieredConfig using the first available provider
// and its default tier models. Returns error if no provider has an API key.
func DefaultTieredConfig(cfg *config.Config) (TieredConfig, error) {
	statuses := DetectProviders(cfg)
	for _, s := range statuses {
		if s.Available {
			// openrouter still uses the openai provider backend
			providerBackend := s.Name
			if providerBackend == "openrouter" {
				providerBackend = "openai"
			}
			return TieredConfig{
				ProviderName: providerBackend,
				CheapModel:   s.DefaultCheap,
				MediumModel:  s.DefaultMedium,
				PremiumModel: s.DefaultPremium,
			}, nil
		}
	}
	return TieredConfig{}, fmt.Errorf("no LLM provider configured: set ARGUS_OPENAI_API_KEY, ARGUS_ANTHROPIC_API_KEY, or ARGUS_GEMINI_API_KEY")
}
