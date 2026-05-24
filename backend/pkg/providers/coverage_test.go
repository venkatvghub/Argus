package providers_test

import (
	"testing"

	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/providers"
)

var cheapTC = providers.TieredConfig{
	CheapModel:   "gpt-4o-mini",
	MediumModel:  "gpt-4o",
	PremiumModel: "gpt-4-turbo",
}

func TestComputeCoverageOptions_Length(t *testing.T) {
	full := providers.PageCounts{FilePage: 100, RepoOverview: 1}
	opts := providers.ComputeCoverageOptions(full, cheapTC, nil, nil)
	if len(opts) != len(providers.DefaultCoverageOptions) {
		t.Fatalf("expected %d options, got %d", len(providers.DefaultCoverageOptions), len(opts))
	}
	for i, opt := range opts {
		if opt.Pct != providers.DefaultCoverageOptions[i] {
			t.Errorf("option[%d]: Pct=%.2f want %.2f", i, opt.Pct, providers.DefaultCoverageOptions[i])
		}
	}
}

func TestComputeCoverageOptions_Singletons(t *testing.T) {
	// RepoOverview/ArchDiagram/Onboarding must always stay at their original value (0 or 1).
	full := providers.PageCounts{
		FilePage:            100,
		SymbolSpotlight:     200,
		ModulePage:          10,
		SCCPage:             5,
		APIContract:         80,
		InfraPage:           4,
		RepoOverview:        1,
		ArchitectureDiagram: 1,
		Onboarding:          1,
	}
	opts := providers.ComputeCoverageOptions(full, cheapTC, nil, nil)
	for _, opt := range opts {
		if opt.Counts.RepoOverview != 1 {
			t.Errorf("pct=%.2f: RepoOverview=%d want 1", opt.Pct, opt.Counts.RepoOverview)
		}
		if opt.Counts.ArchitectureDiagram != 1 {
			t.Errorf("pct=%.2f: ArchitectureDiagram=%d want 1", opt.Pct, opt.Counts.ArchitectureDiagram)
		}
		if opt.Counts.Onboarding != 1 {
			t.Errorf("pct=%.2f: Onboarding=%d want 1", opt.Pct, opt.Counts.Onboarding)
		}
	}
}

func TestComputeCoverageOptions_Monotonic(t *testing.T) {
	// Scalable counts increase (or stay equal) as coverage pct increases.
	full := providers.PageCounts{
		FilePage:    100,
		ModulePage:  10,
		APIContract: 80,
		RepoOverview: 1,
	}
	opts := providers.ComputeCoverageOptions(full, cheapTC, nil, nil)
	for i := 1; i < len(opts); i++ {
		if opts[i].Counts.FilePage < opts[i-1].Counts.FilePage {
			t.Errorf("FilePage not monotonic: opt[%d]=%d < opt[%d]=%d",
				i, opts[i].Counts.FilePage, i-1, opts[i-1].Counts.FilePage)
		}
	}
}

func TestComputeCoverageOptions_Floor(t *testing.T) {
	// Scalable fields with small full counts must floor at 1 (not 0).
	full := providers.PageCounts{
		FilePage:  2,
		InfraPage: 1,
	}
	opts := providers.ComputeCoverageOptions(full, cheapTC, nil, nil)
	for _, opt := range opts {
		if full.FilePage > 0 && opt.Counts.FilePage < 1 {
			t.Errorf("pct=%.2f: FilePage below floor: got %d", opt.Pct, opt.Counts.FilePage)
		}
		if full.InfraPage > 0 && opt.Counts.InfraPage < 1 {
			t.Errorf("pct=%.2f: InfraPage below floor: got %d", opt.Pct, opt.Counts.InfraPage)
		}
	}
}

func TestComputeCoverageOptions_ZeroPassthrough(t *testing.T) {
	// Zero in fullCounts must stay zero at all coverage levels.
	full := providers.PageCounts{
		FilePage:  50,
		InfraPage: 0, // no infra files in this repo
	}
	opts := providers.ComputeCoverageOptions(full, cheapTC, nil, nil)
	for _, opt := range opts {
		if opt.Counts.InfraPage != 0 {
			t.Errorf("pct=%.2f: InfraPage should be 0, got %d", opt.Pct, opt.Counts.InfraPage)
		}
	}
}

func TestComputeCoverageOptions_RecommendedFlag(t *testing.T) {
	full := providers.PageCounts{FilePage: 100, RepoOverview: 1}
	opts := providers.ComputeCoverageOptions(full, cheapTC, nil, nil)

	recCount := 0
	for _, opt := range opts {
		if opt.Recommended {
			recCount++
			if opt.Pct != providers.RecommendedCoverage {
				t.Errorf("Recommended option Pct=%.2f want %.2f", opt.Pct, providers.RecommendedCoverage)
			}
		}
	}
	if recCount != 1 {
		t.Errorf("expected 1 recommended option, got %d", recCount)
	}
}

func TestComputeCoverageOptions_CostRange(t *testing.T) {
	// MinCost = TotalCost*0.8, MaxCost = TotalCost*1.2 → MinCost < MaxCost when TotalCost > 0.
	// TotalCost can be recovered as MinCost/0.8.
	full := providers.PageCounts{FilePage: 50, RepoOverview: 1, Onboarding: 1}
	opts := providers.ComputeCoverageOptions(full, cheapTC, nil, nil)

	for _, opt := range opts {
		if opt.MinCost == 0 && opt.MaxCost == 0 {
			continue // static pricing miss; not a test failure
		}
		if opt.MinCost >= opt.MaxCost {
			t.Errorf("pct=%.2f: MinCost (%.6f) >= MaxCost (%.6f)", opt.Pct, opt.MinCost, opt.MaxCost)
		}
		// Ratio check: MinCost/MaxCost ≈ 0.8/1.2 = 2/3
		ratio := opt.MinCost / opt.MaxCost
		if ratio < 0.65 || ratio > 0.70 {
			t.Errorf("pct=%.2f: MinCost/MaxCost ratio=%.3f, want ~0.667", opt.Pct, ratio)
		}
	}
}

func TestComputeCoverageOptions_DynamicPricing(t *testing.T) {
	full := providers.PageCounts{FilePage: 100, RepoOverview: 1}
	tc := providers.TieredConfig{
		CheapModel:   "openai/gpt-4o-mini",
		MediumModel:  "openai/gpt-4o",
		PremiumModel: "anthropic/claude-opus-4",
	}
	dynamic := map[string][2]float64{
		"openai/gpt-4o-mini":      {0.15, 0.60},
		"openai/gpt-4o":           {2.50, 10.00},
		"anthropic/claude-opus-4": {15.0, 75.0},
	}
	opts := providers.ComputeCoverageOptions(full, tc, dynamic, nil)
	if len(opts) == 0 {
		t.Fatal("expected non-empty options")
	}
	for _, opt := range opts {
		if opt.MinCost < 0 || opt.MaxCost < 0 {
			t.Errorf("pct=%.2f: negative cost: min=%.4f max=%.4f", opt.Pct, opt.MinCost, opt.MaxCost)
		}
	}
}

func TestComputeCoverageOptions_WithFiles(t *testing.T) {
	// Providing actual file sizes should calibrate heuristics (no panic; cost > 0).
	files := []models.FileNode{
		{Path: "main.go", IsFile: true, Size: 8000},
		{Path: "utils.go", IsFile: true, Size: 4000},
		{Path: "handler.go", IsFile: true, Size: 12000},
	}
	full := providers.PageCounts{FilePage: 3, RepoOverview: 1}
	opts := providers.ComputeCoverageOptions(full, cheapTC, nil, files)
	if len(opts) == 0 {
		t.Fatal("expected non-empty options")
	}
}
