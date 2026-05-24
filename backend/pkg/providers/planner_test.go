package providers_test

import (
	"testing"

	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/providers"
)

func TestCountPages_Basic(t *testing.T) {
	files := []models.FileNode{
		{Path: "cmd/main.go", IsFile: true},
		{Path: "pkg/utils.go", IsFile: true},
		{Path: "pkg/handlers.go", IsFile: true},
		{Path: "infra/Dockerfile", IsFile: true},
		{Path: "infra/app.tf", IsFile: true},
		{Path: ".github/workflows/ci.yml", IsFile: true},
	}
	symbolCount := 42
	communityCount := 3

	counts := providers.CountPages(files, symbolCount, communityCount)

	// Verify FilePage count
	if counts.FilePage != 6 {
		t.Errorf("FilePage count mismatch: got %d, want 6", counts.FilePage)
	}

	// Verify SymbolSpotlight = symbolCount
	if counts.SymbolSpotlight != 42 {
		t.Errorf("SymbolSpotlight count mismatch: got %d, want 42", counts.SymbolSpotlight)
	}

	// Verify ModulePage count (unique directories)
	// Expected: cmd, pkg, infra, .github/workflows = 4 unique
	if counts.ModulePage != 4 {
		t.Errorf("ModulePage count mismatch: got %d, want 4", counts.ModulePage)
	}

	// Verify InfraPage detection (Dockerfile, .tf, CI files)
	// Expected: 3 (Dockerfile, app.tf, ci.yml)
	if counts.InfraPage != 3 {
		t.Errorf("InfraPage count mismatch: got %d, want 3", counts.InfraPage)
	}

	// Verify APIContract count (.go files)
	if counts.APIContract != 3 {
		t.Errorf("APIContract count mismatch: got %d, want 3", counts.APIContract)
	}

	// Verify SCCPage = communityCount
	if counts.SCCPage != 3 {
		t.Errorf("SCCPage count mismatch: got %d, want 3", counts.SCCPage)
	}

	// Verify premium pages (always 1 each)
	if counts.RepoOverview != 1 {
		t.Errorf("RepoOverview should be 1, got %d", counts.RepoOverview)
	}
	if counts.ArchitectureDiagram != 1 {
		t.Errorf("ArchitectureDiagram should be 1, got %d", counts.ArchitectureDiagram)
	}
	if counts.Onboarding != 1 {
		t.Errorf("Onboarding should be 1, got %d", counts.Onboarding)
	}
}

func TestCountPages_EmptyFiles(t *testing.T) {
	counts := providers.CountPages([]models.FileNode{}, 0, 0)

	if counts.FilePage != 0 {
		t.Errorf("FilePage should be 0 for empty input, got %d", counts.FilePage)
	}
	if counts.SymbolSpotlight != 0 {
		t.Errorf("SymbolSpotlight should be 0 for empty input, got %d", counts.SymbolSpotlight)
	}
	if counts.ModulePage != 0 {
		t.Errorf("ModulePage should be 0 for empty input, got %d", counts.ModulePage)
	}
	if counts.SCCPage != 0 {
		t.Errorf("SCCPage should be 0 for empty input, got %d", counts.SCCPage)
	}

	// Premium pages should always be 1
	if counts.RepoOverview != 1 {
		t.Errorf("RepoOverview should be 1 regardless of input, got %d", counts.RepoOverview)
	}
	if counts.ArchitectureDiagram != 1 {
		t.Errorf("ArchitectureDiagram should be 1 regardless of input, got %d", counts.ArchitectureDiagram)
	}
	if counts.Onboarding != 1 {
		t.Errorf("Onboarding should be 1 regardless of input, got %d", counts.Onboarding)
	}
}

func TestCountPages_NonFileIgnored(t *testing.T) {
	files := []models.FileNode{
		{Path: "cmd", IsFile: false}, // directory, should be ignored
		{Path: "src/main.go", IsFile: true},
	}

	counts := providers.CountPages(files, 0, 0)

	if counts.FilePage != 1 {
		t.Errorf("FilePage should count only files, got %d", counts.FilePage)
	}
}

func TestCountPages_APIContractMultiLanguage(t *testing.T) {
	files := []models.FileNode{
		{Path: "src/main.go", IsFile: true},      // .go
		{Path: "src/app.ts", IsFile: true},       // .ts
		{Path: "src/App.tsx", IsFile: true},      // .tsx (not in list, so not counted)
		{Path: "src/App.java", IsFile: true},     // .java
		{Path: "src/script.py", IsFile: true},    // .py
		{Path: "README.md", IsFile: true},        // not API contract
	}

	counts := providers.CountPages(files, 0, 0)

	// Expected: .go, .ts, .java, .py = 4
	if counts.APIContract != 4 {
		t.Errorf("APIContract count mismatch: got %d, want 4", counts.APIContract)
	}
}

func TestBuildPlan_NoPremium(t *testing.T) {
	counts := providers.PageCounts{
		FilePage:            10,
		SymbolSpotlight:     5,
		ModulePage:          3,
		SCCPage:             2,
		APIContract:         8,
		InfraPage:           1,
		RepoOverview:        0, // no premium
		ArchitectureDiagram: 0,
		Onboarding:          0,
	}

	tc := providers.TieredConfig{
		ProviderName: "openai",
		CheapModel:   "gpt-4o-mini",
		MediumModel:  "gpt-4o",
		PremiumModel: "gpt-4",
	}

	plan := providers.BuildPlan(counts, tc, nil, nil)

	// No premium pages, so plan should not include any with tier="premium"
	for _, entry := range plan.Entries {
		if entry.Tier == "premium" {
			t.Errorf("plan should not include premium entries when counts are all zero, got %+v", entry)
		}
	}
	if plan.TotalPages != 29 { // 10+5+3+2+8+1
		t.Errorf("total pages mismatch: got %d, want 29", plan.TotalPages)
	}
}

func TestBuildPlan_TotalCost(t *testing.T) {
	counts := providers.PageCounts{
		FilePage:       5,
		ModulePage:     2,
		RepoOverview:   1,
		APIContract:    3,
		ArchitectureDiagram: 1,
	}

	tc := providers.TieredConfig{
		ProviderName: "openai",
		CheapModel:   "gpt-4o-mini",
		MediumModel:  "gpt-4o",
		PremiumModel: "gpt-4",
	}

	plan := providers.BuildPlan(counts, tc, nil, nil)

	// TotalCost should be > 0 when counts are non-zero
	if plan.TotalCost <= 0 {
		t.Errorf("TotalCost should be > 0, got %f", plan.TotalCost)
	}

	// TotalPages = sum of all counts
	expectedTotal := 5 + 2 + 1 + 3 + 1
	if plan.TotalPages != expectedTotal {
		t.Errorf("TotalPages mismatch: got %d, want %d", plan.TotalPages, expectedTotal)
	}

	// Verify sum of EstCost matches TotalCost
	var sumCost float64
	for _, entry := range plan.Entries {
		sumCost += entry.EstCost
	}
	if sumCost != plan.TotalCost {
		t.Errorf("TotalCost mismatch: sum of entries=%f, plan=%f", sumCost, plan.TotalCost)
	}
}

func TestBuildPlan_EmptyCounts(t *testing.T) {
	counts := providers.PageCounts{}

	tc := providers.TieredConfig{
		ProviderName: "openai",
		CheapModel:   "gpt-4o-mini",
		MediumModel:  "gpt-4o",
		PremiumModel: "gpt-4",
	}

	plan := providers.BuildPlan(counts, tc, nil, nil)

	if len(plan.Entries) > 0 {
		t.Errorf("empty counts should produce empty plan, got %d entries", len(plan.Entries))
	}
	if plan.TotalCost != 0 {
		t.Errorf("empty plan should have TotalCost=0, got %f", plan.TotalCost)
	}
	if plan.TotalPages != 0 {
		t.Errorf("empty plan should have TotalPages=0, got %d", plan.TotalPages)
	}
}

func TestBuildPlan_CorrectTierAssignment(t *testing.T) {
	counts := providers.PageCounts{
		FilePage:            1, // cheap
		ModulePage:          1, // medium
		RepoOverview:        1, // premium
	}

	tc := providers.TieredConfig{
		ProviderName: "openai",
		CheapModel:   "gpt-4o-mini",
		MediumModel:  "gpt-4o",
		PremiumModel: "gpt-4",
	}

	plan := providers.BuildPlan(counts, tc, nil, nil)

	// Verify tier assignments
	tierMap := make(map[string]string)
	for _, entry := range plan.Entries {
		tierMap[entry.PageType] = entry.Tier
	}

	if tierMap["file_page"] != "cheap" {
		t.Errorf("file_page should be cheap, got %s", tierMap["file_page"])
	}
	if tierMap["module_page"] != "medium" {
		t.Errorf("module_page should be medium, got %s", tierMap["module_page"])
	}
	if tierMap["repo_overview"] != "premium" {
		t.Errorf("repo_overview should be premium, got %s", tierMap["repo_overview"])
	}
}

func TestBuildPlan_ModelAssignment(t *testing.T) {
	counts := providers.PageCounts{
		FilePage:   1,
		ModulePage: 1,
		RepoOverview: 1,
	}

	tc := providers.TieredConfig{
		ProviderName: "openai",
		CheapModel:   "gpt-4o-mini",
		MediumModel:  "gpt-4o",
		PremiumModel: "gpt-4",
	}

	plan := providers.BuildPlan(counts, tc, nil, nil)

	// Verify model assignments match tiers
	modelMap := make(map[string]string)
	for _, entry := range plan.Entries {
		modelMap[entry.PageType] = entry.Model
	}

	if modelMap["file_page"] != "gpt-4o-mini" {
		t.Errorf("file_page should use gpt-4o-mini, got %s", modelMap["file_page"])
	}
	if modelMap["module_page"] != "gpt-4o" {
		t.Errorf("module_page should use gpt-4o, got %s", modelMap["module_page"])
	}
	if modelMap["repo_overview"] != "gpt-4" {
		t.Errorf("repo_overview should use gpt-4, got %s", modelMap["repo_overview"])
	}
}

func TestCountPages_ModuleUniqueness(t *testing.T) {
	files := []models.FileNode{
		{Path: "pkg/module1/file1.go", IsFile: true},
		{Path: "pkg/module1/file2.go", IsFile: true},
		{Path: "pkg/module2/file3.go", IsFile: true},
		{Path: "cmd/main.go", IsFile: true},
	}

	counts := providers.CountPages(files, 0, 0)

	// Unique modules: pkg/module1, pkg/module2, cmd = 3
	if counts.ModulePage != 3 {
		t.Errorf("ModulePage should count unique directories, got %d want 3", counts.ModulePage)
	}
}

func TestCountPages_RootLevelFiles(t *testing.T) {
	files := []models.FileNode{
		{Path: "README.md", IsFile: true},
		{Path: "go.mod", IsFile: true},
	}

	counts := providers.CountPages(files, 0, 0)

	// Root-level files have dir=".", should not be counted as modules
	if counts.ModulePage != 0 {
		t.Errorf("root-level files should not create module entries, got %d", counts.ModulePage)
	}
	if counts.FilePage != 2 {
		t.Errorf("FilePage should be 2, got %d", counts.FilePage)
	}
}
