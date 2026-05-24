package providers

import (
	"os"
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
// dynamic provides live pricing (e.g. from OpenRouter discovery); nil falls back to static table.
// files is used to compute actual average token counts per file; nil uses default heuristics.
func BuildPlan(counts PageCounts, tc TieredConfig, dynamic map[string][2]float64, files []models.FileNode) GenerationPlan {
	heuristics := computeHeuristics(files)

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
		cost := estimateWithHeuristics(e.pageType, model, e.count, dynamic, heuristics)
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

// computeHeuristics derives token heuristics from actual repo files.
// Falls back to package-level defaults for types not computable from files.
func computeHeuristics(files []models.FileNode) map[string]PageTokenHeuristic {
	// Start from defaults
	h := make(map[string]PageTokenHeuristic, len(pageHeuristics))
	for k, v := range pageHeuristics {
		h[k] = v
	}

	if len(files) == 0 {
		return h
	}

	// Estimate average tokens per file: bytes / 4 is a standard LLM approximation.
	var totalBytes int64
	var fileCount int
	for _, f := range files {
		if f.IsFile && f.Size > 0 {
			totalBytes += f.Size
			fileCount++
		}
	}
	if fileCount == 0 {
		return h
	}

	avgTokens := int(totalBytes/int64(fileCount)) / 4
	if avgTokens < 200 {
		avgTokens = 200 // floor: tiny files still need context prompt overhead
	}

	// file_page input = actual avg tokens + prompt overhead; output scales with it.
	fp := h["file_page"]
	fp.Input = avgTokens + 500              // +500 for system prompt / instructions
	fp.Output = max(200, avgTokens/3)       // output ~1/3 of input
	h["file_page"] = fp

	// symbol_spotlight is a focused excerpt — cap at 1/4 of file avg.
	sp := h["symbol_spotlight"]
	sp.Input = max(300, avgTokens/4) + 300
	sp.Output = max(100, avgTokens/12)
	h["symbol_spotlight"] = sp

	// module_page aggregates several files — scale by sqrt(files per module).
	mp := h["module_page"]
	mp.Input = avgTokens*3 + 1000
	mp.Output = max(500, avgTokens/2)
	h["module_page"] = mp

	return h
}

// estimateWithHeuristics computes page cost using repo-derived heuristics + dynamic pricing.
func estimateWithHeuristics(pageType, model string, count int, dynamic map[string][2]float64, heuristics map[string]PageTokenHeuristic) float64 {
	h, ok := heuristics[pageType]
	if !ok {
		return 0
	}
	inPer1M, outPer1M := DynamicModelCostPer1M(model, dynamic)
	inputCost := float64(h.Input) * float64(count) * inPer1M / 1_000_000
	outputCost := float64(h.Output) * float64(count) * outPer1M / 1_000_000
	return inputCost + outputCost
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
	if base == "Jenkinsfile" {
		return true
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yml" && ext != ".yaml" {
		return false
	}

	return hasCIDirectory(pathSegments(path))
}

func pathSegments(path string) []string {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(os.PathSeparator) {
		return nil
	}
	return strings.Split(clean, string(os.PathSeparator))
}

func hasCIDirectory(segments []string) bool {
	if len(segments) <= 1 {
		return false
	}
	dirs := segments[:len(segments)-1]
	for i, seg := range dirs {
		switch {
		case seg == ".github", seg == ".circleci", seg == "ci":
			return true
		case seg == "workflows" && i > 0 && dirs[i-1] == ".github":
			return true
		case isCINamedDirectory(seg):
			return true
		}
	}
	return false
}

func isCINamedDirectory(name string) bool {
	lower := strings.ToLower(name)
	return lower == "ci" || lower == ".circleci" || strings.HasSuffix(lower, "-ci")
}
