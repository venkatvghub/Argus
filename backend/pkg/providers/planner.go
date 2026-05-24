package providers

import (
	"path/filepath"
	"strings"

	"github.com/venkatvghub/argus/pkg/models"
)

// PlanEntry describes one row in the generation cost plan.
type PlanEntry struct {
	PageType string
	Tier     string
	Model    string
	Count    int
	EstCost  float64
}

// GenerationPlan is the full pre-run cost plan for wiki generation.
type GenerationPlan struct {
	Entries    []PlanEntry
	TotalCost  float64
	TotalPages int
}

// PageCounts holds the number of pages to generate per type, derived from analysis results.
type PageCounts struct {
	FilePage            int
	SymbolSpotlight     int
	ModulePage          int
	SCCPage             int
	APIContract         int
	InfraPage           int
	RepoOverview        int // always 0 or 1
	ArchitectureDiagram int // always 0 or 1
	Onboarding          int // always 0 or 1
}

// modelForTier returns the model name for a given tier.
func (tc TieredConfig) modelForTier(tier string) string {
	switch tier {
	case "cheap":
		return tc.CheapModel
	case "medium":
		return tc.MediumModel
	case "premium":
		return tc.PremiumModel
	default:
		return tc.MediumModel
	}
}

// BuildPlan computes a GenerationPlan from page counts and tier model assignments.
func BuildPlan(counts PageCounts, tc TieredConfig) GenerationPlan {
	entries := []struct {
		pageType string
		count    int
	}{
		{"file_page", counts.FilePage},
		{"symbol_spotlight", counts.SymbolSpotlight},
		{"module_page", counts.ModulePage},
		{"scc_page", counts.SCCPage},
		{"api_contract", counts.APIContract},
		{"infra_page", counts.InfraPage},
		{"repo_overview", counts.RepoOverview},
		{"architecture_diagram", counts.ArchitectureDiagram},
		{"onboarding", counts.Onboarding},
	}

	var plan GenerationPlan
	for _, e := range entries {
		if e.count == 0 {
			continue
		}
		tier := TierForPageType(e.pageType)
		model := tc.modelForTier(tier)
		cost := EstimatePageCost(e.pageType, model, e.count)
		plan.Entries = append(plan.Entries, PlanEntry{
			PageType: e.pageType,
			Tier:     tier,
			Model:    model,
			Count:    e.count,
			EstCost:  cost,
		})
		plan.TotalCost += cost
		plan.TotalPages += e.count
	}
	return plan
}

// CountPages computes PageCounts from analysis results.
// files = file nodes, symbolCount = total symbols, communityCount = number of communities from graph.
func CountPages(files []models.FileNode, symbolCount, communityCount int) PageCounts {
	var counts PageCounts
	counts.RepoOverview = 1
	counts.ArchitectureDiagram = 1
	counts.Onboarding = 1
	counts.SymbolSpotlight = symbolCount

	moduleSet := make(map[string]struct{})
	for _, f := range files {
		if !f.IsFile {
			continue
		}
		counts.FilePage++
		// Module = directory of the file (one level up)
		dir := filepath.Dir(f.Path)
		if dir != "." {
			moduleSet[dir] = struct{}{}
		}
		// Infra detection: Dockerfile, *.tf, *.yml CI files
		base := filepath.Base(f.Path)
		ext := filepath.Ext(f.Path)
		if base == "Dockerfile" || ext == ".tf" || isCI(f.Path) {
			counts.InfraPage++
		}
		// API contract: files with exported symbols (approximated by language)
		if ext == ".go" || ext == ".ts" || ext == ".java" || ext == ".py" {
			counts.APIContract++
		}
	}
	counts.ModulePage = len(moduleSet)
	counts.SCCPage = communityCount

	return counts
}

// isCI returns true for common CI config file paths.
func isCI(path string) bool {
	base := filepath.Base(path)
	return base == ".github" || strings.HasSuffix(path, ".github/workflows") ||
		base == "Jenkinsfile" || base == ".circleci" ||
		strings.HasSuffix(base, ".yml") && (strings.Contains(path, ".github") || strings.Contains(path, "ci"))
}
