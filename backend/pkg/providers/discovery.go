package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/venkatvghub/argus/pkg/config"
)

// openRouterHTTPClient is used for model discovery calls; 15 s prevents indefinite hangs.
var openRouterHTTPClient = &http.Client{Timeout: 15 * time.Second}

// OpenRouterModel holds model metadata returned by OpenRouter's /models endpoint.
type OpenRouterModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Pricing struct {
		Prompt     string `json:"prompt"`     // cost per token as string, e.g. "0.000001"
		Completion string `json:"completion"` // cost per token as string
	} `json:"pricing"`
}

// ClassifyTier buckets a model name into "cheap", "medium", or "premium" based on name heuristics.
// Returns "" if the model doesn't match any tier.
// Priority when multiple match: premium > medium > cheap.
func ClassifyTier(model string) string {
	lower := strings.ToLower(model)

	isPremium := strings.Contains(lower, "opus") ||
		strings.Contains(lower, "ultra") ||
		strings.Contains(lower, "large") ||
		strings.Contains(lower, "gpt-4-turbo") ||
		strings.Contains(lower, "gpt-4-0") ||
		strings.Contains(lower, "gpt-4-1") ||
		strings.Contains(lower, "gpt-4-3") ||
		strings.Contains(lower, "gpt-4-5")

	isMedium := strings.Contains(lower, "gpt-4o") ||
		strings.Contains(lower, "sonnet") ||
		strings.Contains(lower, "pro") ||
		strings.Contains(lower, "medium") ||
		strings.Contains(lower, "70b") ||
		strings.Contains(lower, "mixtral")

	isCheap := strings.Contains(lower, "mini") ||
		strings.Contains(lower, "haiku") ||
		strings.Contains(lower, "flash") ||
		strings.Contains(lower, "7b") ||
		strings.Contains(lower, "8b") ||
		strings.Contains(lower, "instruct") ||
		strings.Contains(lower, "small") ||
		strings.Contains(lower, "lite") ||
		strings.Contains(lower, "nano")

	switch {
	case isPremium:
		return "premium"
	case isMedium:
		return "medium"
	case isCheap:
		return "cheap"
	default:
		return ""
	}
}

// BucketByTier groups model IDs into cheap/medium/premium slices using classifyTier.
// Models that don't match any tier are excluded.
func BucketByTier(models []string) map[string][]string {
	buckets := map[string][]string{
		"cheap":   {},
		"medium":  {},
		"premium": {},
	}
	for _, m := range models {
		tier := ClassifyTier(m)
		if tier != "" {
			buckets[tier] = append(buckets[tier], m)
		}
	}
	// Sort each bucket for stable output
	for k := range buckets {
		sort.Strings(buckets[k])
	}
	return buckets
}

// DiscoverModels fetches available models for the given provider config.
// For OpenRouter, also returns per-model pricing parsed from the API response.
// Returns (models []string, pricingMap map[string][2]float64, err error)
// pricingMap keys are model IDs; values are [inputPer1M, outputPer1M].
func DiscoverModels(ctx context.Context, cfg *config.Config) ([]string, map[string][2]float64, error) {
	variantName, _ := openaiVariant(cfg.OpenAIBaseURL)

	switch variantName {
	case "openrouter":
		return discoverOpenRouterModels(ctx, cfg)
	default:
		// Use provider-specific ListModels; no pricing data available
		return discoverViaListModels(ctx, cfg, variantName)
	}
}

// DiscoverOpenRouterModelsFromURL is like discoverOpenRouterModels but accepts an explicit URL.
// Used in tests to point at a mock HTTP server instead of the live OpenRouter API.
func DiscoverOpenRouterModelsFromURL(ctx context.Context, cfg *config.Config, url string) ([]string, map[string][2]float64, error) {
	return fetchOpenRouterModels(ctx, cfg, url)
}

// discoverOpenRouterModels calls OpenRouter's /models endpoint to get models + live pricing.
func discoverOpenRouterModels(ctx context.Context, cfg *config.Config) ([]string, map[string][2]float64, error) {
	return fetchOpenRouterModels(ctx, cfg, "https://openrouter.ai/api/v1/models")
}

func fetchOpenRouterModels(ctx context.Context, cfg *config.Config, url string) ([]string, map[string][2]float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("openrouter discover: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAIKey)
	req.Header.Set("HTTP-Referer", openRouterReferer)
	req.Header.Set("X-Title", openRouterTitle)

	resp, err := openRouterHTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("openrouter discover: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("openrouter discover: status %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Data []OpenRouterModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("openrouter discover: decode: %w", err)
	}

	models := make([]string, 0, len(result.Data))
	pricing := make(map[string][2]float64, len(result.Data))

	for _, m := range result.Data {
		promptStr := m.Pricing.Prompt
		completionStr := m.Pricing.Completion

		// Skip free/experimental models (both pricing fields are "0" or empty)
		if (promptStr == "" || promptStr == "0") && (completionStr == "" || completionStr == "0") {
			continue
		}

		models = append(models, m.ID)

		// Parse pricing: OpenRouter returns cost-per-token; multiply by 1M for per-1M cost
		promptF, _ := strconv.ParseFloat(promptStr, 64)
		completionF, _ := strconv.ParseFloat(completionStr, 64)
		pricing[m.ID] = [2]float64{promptF * 1_000_000, completionF * 1_000_000}
	}

	sort.Strings(models)
	return models, pricing, nil
}

// discoverViaListModels calls the appropriate provider's ListModels.
// Returns empty pricing map since non-OpenRouter providers don't expose pricing via API.
func discoverViaListModels(ctx context.Context, cfg *config.Config, variantName string) ([]string, map[string][2]float64, error) {
	emptyPricing := map[string][2]float64{}

	switch variantName {
	case "anthropic":
		p := NewAnthropicProvider(cfg)
		models, err := p.ListModels(ctx)
		if err != nil {
			return nil, nil, err
		}
		return models, emptyPricing, nil
	case "gemini":
		p := NewGeminiProvider(cfg)
		models, err := p.ListModels(ctx)
		if err != nil {
			return nil, nil, err
		}
		return models, emptyPricing, nil
	default:
		// openai (direct) or unknown
		p := NewOpenAIProvider(cfg)
		models, err := p.ListModels(ctx)
		if err != nil {
			return nil, nil, err
		}
		return models, emptyPricing, nil
	}
}
