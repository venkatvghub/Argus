package providers

import "strings"

// CostGate is the USD threshold above which the user must confirm before generation starts.
const CostGate = 10.0

// PageTokenHeuristic holds estimated input and output token counts for a page type.
type PageTokenHeuristic struct {
	Input  int
	Output int
}

// pageHeuristics maps page type → token estimate.
var pageHeuristics = map[string]PageTokenHeuristic{
	"file_page":            {Input: 2000, Output: 800},
	"symbol_spotlight":     {Input: 500, Output: 200},
	"module_page":          {Input: 4000, Output: 1500},
	"scc_page":             {Input: 6000, Output: 2000},
	"api_contract":         {Input: 3000, Output: 1200},
	"infra_page":           {Input: 2500, Output: 1000},
	"repo_overview":        {Input: 8000, Output: 3000},
	"architecture_diagram": {Input: 10000, Output: 4000},
	"onboarding":           {Input: 6000, Output: 2500},
}

// pageTier maps page type → tier name.
var pageTier = map[string]string{
	"file_page":            "cheap",
	"symbol_spotlight":     "cheap",
	"module_page":          "medium",
	"scc_page":             "medium",
	"api_contract":         "medium",
	"infra_page":           "medium",
	"repo_overview":        "premium",
	"architecture_diagram": "premium",
	"onboarding":           "premium",
}

// TierForPageType returns the generation tier for a page type.
func TierForPageType(pageType string) string {
	if t, ok := pageTier[pageType]; ok {
		return t
	}
	return "medium"
}

// modelPricing maps model name prefix → (input, output) cost per 1M tokens in USD.
// Includes both native model names and OpenRouter-style "provider/model" names.
// Prefix matching: try exact first, then longest prefix.
var modelPricing = []modelPrice{
	// Anthropic — native
	{"claude-3-5-haiku", 0.80, 4.00},
	{"claude-3-5-sonnet", 3.00, 15.00},
	{"claude-3-opus", 15.00, 75.00},
	{"claude-opus-4", 15.00, 75.00},
	{"claude-sonnet-4", 3.00, 15.00},
	{"claude-haiku-4", 0.80, 4.00},
	// Anthropic — OpenRouter style
	{"anthropic/claude-3-5-haiku", 0.80, 4.00},
	{"anthropic/claude-3-5-sonnet", 3.00, 15.00},
	{"anthropic/claude-3-opus", 15.00, 75.00},
	{"anthropic/claude-opus-4", 15.00, 75.00},
	{"anthropic/claude-sonnet-4", 3.00, 15.00},
	{"anthropic/claude-haiku-4", 0.80, 4.00},
	// OpenAI — native
	{"gpt-4o-mini", 0.15, 0.60},
	{"gpt-4o", 2.50, 10.00},
	{"gpt-4-turbo", 10.00, 30.00},
	{"gpt-4", 10.00, 30.00},
	// OpenAI — OpenRouter style
	{"openai/gpt-4o-mini", 0.15, 0.60},
	{"openai/gpt-4o", 2.50, 10.00},
	{"openai/gpt-4-turbo", 10.00, 30.00},
	{"openai/gpt-4", 10.00, 30.00},
	// Gemini — native
	{"gemini-2.0-flash", 0.075, 0.30},
	{"gemini-1.5-pro", 1.25, 5.00},
	{"gemini-1.5-flash", 0.075, 0.30},
	{"gemini-ultra", 10.00, 30.00},
	// Gemini — OpenRouter style
	{"google/gemini-2.0-flash", 0.075, 0.30},
	{"google/gemini-1.5-pro", 1.25, 5.00},
	{"google/gemini-1.5-flash", 0.075, 0.30},
	// Mistral — OpenRouter style
	{"mistralai/mistral-7b-instruct", 0.06, 0.06},
	{"mistralai/mistral-small", 0.20, 0.60},
	{"mistralai/mistral-medium", 2.70, 8.10},
	{"mistralai/mistral-large", 8.00, 24.00},
	{"mistralai/mixtral-8x7b", 0.45, 0.70},
	// Meta — OpenRouter style
	{"meta-llama/llama-3-8b-instruct", 0.06, 0.06},
	{"meta-llama/llama-3-70b-instruct", 0.52, 0.75},
	{"meta-llama/llama-3.1-8b-instruct", 0.06, 0.06},
	{"meta-llama/llama-3.1-70b-instruct", 0.52, 0.75},
	// Qwen — OpenRouter style
	{"qwen/qwen3.7-max", 0.38, 1.12},
	{"qwen/qwen-2.5-72b-instruct", 0.40, 1.20},
	{"qwen/qwen-turbo", 0.05, 0.20},
	// Gemini — OpenRouter style (additional variants)
	{"google/gemini-3.1-flash-lite", 0.04, 0.15},
	{"google/gemini-2.0-flash-lite", 0.04, 0.15},
	{"google/gemini-2.5-flash", 0.15, 0.60},
	{"google/gemini-flash-1.5", 0.075, 0.30},
	// OpenAI — additional
	{"openai/o3-mini", 1.10, 4.40},
	{"openai/o4-mini", 1.10, 4.40},
}

type modelPrice struct {
	prefix     string
	inputPer1M float64
	outPer1M   float64
}

// ModelCostPer1M returns (inputCostPer1M, outputCostPer1M) for a model name.
// Uses exact match first, then longest prefix match. Returns (0,0) if unknown.
func ModelCostPer1M(model string) (float64, float64) {
	var best *modelPrice
	for i := range modelPricing {
		mp := &modelPricing[i]
		if model == mp.prefix {
			return mp.inputPer1M, mp.outPer1M
		}
		if strings.HasPrefix(model, mp.prefix) {
			if best == nil || len(mp.prefix) > len(best.prefix) {
				best = mp
			}
		}
	}
	if best != nil {
		return best.inputPer1M, best.outPer1M
	}
	return 0, 0
}

// EstimatePageCost returns the estimated USD cost for generating count pages of the given type
// using the given model.
func EstimatePageCost(pageType, model string, count int) float64 {
	h, ok := pageHeuristics[pageType]
	if !ok {
		return 0
	}
	inPer1M, outPer1M := ModelCostPer1M(model)
	inputCost := float64(h.Input) * float64(count) * inPer1M / 1_000_000
	outputCost := float64(h.Output) * float64(count) * outPer1M / 1_000_000
	return inputCost + outputCost
}

// DynamicModelCostPer1M returns cost per 1M tokens from a discovered pricing map,
// falling back to the static modelPricing table if not found.
func DynamicModelCostPer1M(model string, dynamic map[string][2]float64) (float64, float64) {
	if p, ok := dynamic[model]; ok {
		return p[0], p[1]
	}
	return ModelCostPer1M(model)
}

// EstimatePageCostDynamic is like EstimatePageCost but uses dynamic pricing when available.
func EstimatePageCostDynamic(pageType, model string, count int, dynamic map[string][2]float64) float64 {
	h, ok := pageHeuristics[pageType]
	if !ok {
		return 0
	}
	inPer1M, outPer1M := DynamicModelCostPer1M(model, dynamic)
	inputCost := float64(h.Input) * float64(count) * inPer1M / 1_000_000
	outputCost := float64(h.Output) * float64(count) * outPer1M / 1_000_000
	return inputCost + outputCost
}
