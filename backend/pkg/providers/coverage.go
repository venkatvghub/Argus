package providers

import "github.com/venkatvghub/argus/pkg/models"

// RecommendedCoverage is the default fraction of pages to generate (20%).
const RecommendedCoverage = 0.20

// DefaultCoverageOptions lists the coverage fractions offered in the interactive table.
var DefaultCoverageOptions = []float64{0.10, 0.15, 0.20, 0.30, 0.40, 0.50}

// CoverageOption is one row in the interactive coverage selection table.
type CoverageOption struct {
	Pct         float64
	Recommended bool
	Counts      PageCounts // page counts at this coverage level
	MinCost     float64    // estimate × 0.8
	MaxCost     float64    // estimate × 1.2
}

// scaleCounts returns a PageCounts scaled to the given fraction of fullCounts.
// RepoOverview, ArchitectureDiagram, and Onboarding are preserved as-is from
// fullCounts (not scaled; each may be 0 or 1). All other PageCounts numeric
// fields are scaled by pct and floored to 1 when the original fullCounts value was > 0.
func scaleCounts(fullCounts PageCounts, pct float64) PageCounts {
	clamp := func(full int) int {
		if full == 0 {
			return 0
		}
		v := int(float64(full) * pct)
		if v < 1 {
			v = 1
		}
		return v
	}

	return PageCounts{
		FilePage:            clamp(fullCounts.FilePage),
		SymbolSpotlight:     clamp(fullCounts.SymbolSpotlight),
		ModulePage:          clamp(fullCounts.ModulePage),
		SCCPage:             clamp(fullCounts.SCCPage),
		APIContract:         clamp(fullCounts.APIContract),
		InfraPage:           clamp(fullCounts.InfraPage),
		RepoOverview:        fullCounts.RepoOverview,        // always 0 or 1
		ArchitectureDiagram: fullCounts.ArchitectureDiagram, // always 0 or 1
		Onboarding:          fullCounts.Onboarding,          // always 0 or 1
	}
}

// ComputeCoverageOptions builds a CoverageOption for each percentage in DefaultCoverageOptions.
// fullCounts is the 100% count (from CountPages). tc and dynamic are used for cost estimation.
// files is forwarded to BuildPlan for token heuristic calibration.
func ComputeCoverageOptions(fullCounts PageCounts, tc TieredConfig, dynamic map[string][2]float64, files []models.FileNode) []CoverageOption {
	opts := make([]CoverageOption, 0, len(DefaultCoverageOptions))
	for _, pct := range DefaultCoverageOptions {
		scaled := scaleCounts(fullCounts, pct)
		plan := BuildPlan(scaled, tc, dynamic, files)
		opts = append(opts, CoverageOption{
			Pct:         pct,
			Recommended: pct == RecommendedCoverage,
			Counts:      scaled,
			MinCost:     plan.TotalCost * 0.8,
			MaxCost:     plan.TotalCost * 1.2,
		})
	}
	return opts
}
