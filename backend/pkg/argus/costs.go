package argus

import (
	"context"
	"sync"

	"github.com/venkatvghub/argus/pkg/persistence"
	"github.com/venkatvghub/argus/pkg/providers"
)

// warnedModels tracks models whose unknown-pricing warning has already been emitted
// this process — prevents log spam when the same unknown model is called thousands of times.
var warnedModels sync.Map

// RecordCost records an LLM call cost entry for a repository.
// pricingMap is optional live pricing from provider discovery (e.g. OpenRouter);
// nil falls back to the static pricing table. CostUSD is 0 when model pricing is unknown in both.
func (i *Instance) RecordCost(ctx context.Context, repoID, model, operation string, inputTokens, outputTokens int, pricingMap map[string][2]float64) error {
	inPer1M, outPer1M := providers.DynamicModelCostPer1M(model, pricingMap)
	if !providers.IsKnownModel(model, pricingMap) {
		if _, alreadyWarned := warnedModels.LoadOrStore(model, true); !alreadyWarned {
			i.log.Warn("unknown model pricing — using conservative fallback ($3/$15 per 1M tokens); add model to pricing table or run with OpenRouter discovery", "model", model, "operation", operation)
		}
	}
	costUSD := float64(inputTokens)*inPer1M/1_000_000 + float64(outputTokens)*outPer1M/1_000_000
	return i.db.RecordLLMCost(ctx, persistence.LLMCostRecord{
		RepoID:       repoID,
		Model:        model,
		Operation:    operation,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      costUSD,
	})
}

// GetRepoCosts returns aggregated LLM cost data for a repository.
// by may be "day", "model", or "operation".
func (i *Instance) GetRepoCosts(ctx context.Context, repoID, by string) ([]persistence.CostGroup, error) {
	if _, err := i.db.GetRepository(ctx, repoID); err != nil {
		return nil, ErrRepoNotFound
	}
	groups, err := i.db.ListLLMCosts(ctx, repoID, by)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		return []persistence.CostGroup{}, nil
	}
	return groups, nil
}

// GetRepoCostSummary returns aggregate cost totals for a repository.
func (i *Instance) GetRepoCostSummary(ctx context.Context, repoID string) (persistence.CostSummary, error) {
	if _, err := i.db.GetRepository(ctx, repoID); err != nil {
		return persistence.CostSummary{}, ErrRepoNotFound
	}
	return i.db.GetLLMCostSummary(ctx, repoID)
}

// ActiveModelName returns the model name string for the currently configured LLM provider.
func (i *Instance) ActiveModelName() string {
	cfg := i.cfg
	if cfg == nil {
		return ""
	}
	switch cfg.LLMProvider {
	case "anthropic":
		return cfg.AnthropicModel
	case "gemini":
		return cfg.GeminiModel
	default:
		return cfg.OpenAIModel
	}
}
