package providers_test

import (
	"testing"

	"github.com/venkatvghub/argus/pkg/providers"
)

func TestModelCostPer1M_Exact(t *testing.T) {
	tests := []struct {
		model     string
		wantIn    float64
		wantOut   float64
		wantDelta float64
	}{
		{"gpt-4o-mini", 0.15, 0.60, 0.001},
		{"claude-haiku-4-5-20251001", 0.80, 4.00, 0.001}, // prefix match
		{"gpt-4o", 2.50, 10.00, 0.001},
		{"claude-3-5-sonnet", 3.00, 15.00, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			inCost, outCost := providers.ModelCostPer1M(tt.model)
			if inCost < 0 || inCost > tt.wantIn+tt.wantDelta {
				t.Errorf("input cost mismatch: got %f, want %f", inCost, tt.wantIn)
			}
			if outCost < 0 || outCost > tt.wantOut+tt.wantDelta {
				t.Errorf("output cost mismatch: got %f, want %f", outCost, tt.wantOut)
			}
		})
	}
}

func TestModelCostPer1M_PrefixMatch(t *testing.T) {
	tests := []struct {
		model   string
		wantIn  float64
		wantOut float64
	}{
		{"gpt-4o-mini-2024-07-18", 0.15, 0.60},
		{"claude-3-5-sonnet-20250514", 3.00, 15.00},
		{"gpt-4-turbo-2024-04-09", 10.00, 30.00},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			inCost, outCost := providers.ModelCostPer1M(tt.model)
			if inCost != tt.wantIn {
				t.Errorf("input cost mismatch: got %f, want %f", inCost, tt.wantIn)
			}
			if outCost != tt.wantOut {
				t.Errorf("output cost mismatch: got %f, want %f", outCost, tt.wantOut)
			}
		})
	}
}

func TestModelCostPer1M_Unknown(t *testing.T) {
	tests := []string{
		"unknown-model-xyz",
		"llama-3",
		"my-custom-model",
		"",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			inCost, outCost := providers.ModelCostPer1M(tt)
			if inCost != 0 || outCost != 0 {
				t.Errorf("unknown model should return (0, 0), got (%f, %f)", inCost, outCost)
			}
		})
	}
}

func TestEstimatePageCost_KnownType(t *testing.T) {
	tests := []struct {
		pageType string
		model    string
		count    int
		wantZero bool
	}{
		{"file_page", "gpt-4o-mini", 10, false},
		{"module_page", "gpt-4o", 5, false},
		{"repo_overview", "gpt-4o", 1, false},
		{"architecture_diagram", "claude-3-5-sonnet", 1, false},
		{"onboarding", "gpt-4o-mini", 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.pageType+"_"+tt.model, func(t *testing.T) {
			cost := providers.EstimatePageCost(tt.pageType, tt.model, tt.count)
			if tt.wantZero && cost != 0 {
				t.Errorf("expected cost to be zero, got %f", cost)
			}
			if !tt.wantZero && cost <= 0 {
				t.Errorf("expected cost > 0 for known type/model, got %f", cost)
			}
		})
	}
}

func TestEstimatePageCost_UnknownType(t *testing.T) {
	tests := []struct {
		pageType string
		model    string
		count    int
	}{
		{"unknown_page", "gpt-4o-mini", 10},
		{"bogus_type", "claude-3-5-sonnet", 5},
		{"", "gpt-4o", 1},
	}

	for _, tt := range tests {
		t.Run(tt.pageType, func(t *testing.T) {
			cost := providers.EstimatePageCost(tt.pageType, tt.model, tt.count)
			if cost != 0 {
				t.Errorf("unknown page type should return 0, got %f", cost)
			}
		})
	}
}

func TestEstimatePageCost_UnknownModel(t *testing.T) {
	// Known page type with unknown model should return 0
	cost := providers.EstimatePageCost("file_page", "unknown-model-xyz", 10)
	if cost != 0 {
		t.Errorf("unknown model should return cost 0, got %f", cost)
	}
}

func TestTierForPageType(t *testing.T) {
	tests := []struct {
		pageType string
		wantTier string
	}{
		{"file_page", "cheap"},
		{"symbol_spotlight", "cheap"},
		{"module_page", "medium"},
		{"scc_page", "medium"},
		{"api_contract", "medium"},
		{"infra_page", "medium"},
		{"repo_overview", "premium"},
		{"architecture_diagram", "premium"},
		{"onboarding", "premium"},
		{"unknown_type", "medium"}, // default tier
	}

	for _, tt := range tests {
		t.Run(tt.pageType, func(t *testing.T) {
			tier := providers.TierForPageType(tt.pageType)
			if tier != tt.wantTier {
				t.Errorf("tier mismatch: got %s, want %s", tier, tt.wantTier)
			}
		})
	}
}

func TestModelCostPer1M_HasPositiveCosts(t *testing.T) {
	// Sanity check: known model returns positive costs
	in, out := providers.ModelCostPer1M("claude-haiku-4-5-20251001")
	if in <= 0 || out <= 0 {
		t.Errorf("known model should have positive costs, got input=%f output=%f", in, out)
	}
}
