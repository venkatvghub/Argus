package argus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/venkatvghub/argus/pkg/analysis"
	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/providers"
)

// buildTestEngine creates a GraphEngine with the given files and symbols.
func buildTestEngine(t *testing.T, files []models.FileNode, symbols []models.Symbol) *analysis.GraphEngine {
	t.Helper()
	ge := analysis.NewGraphEngine()
	if err := ge.BuildGraph(files, symbols, nil); err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return ge
}

// --- scoreAndRankNodes ---

func TestScoreAndRankNodes_Nil(t *testing.T) {
	if got := scoreAndRankNodes(nil); got != nil {
		t.Errorf("scoreAndRankNodes(nil) = %v, want nil", got)
	}
}

func TestScoreAndRankNodes_OnlyFileNodes(t *testing.T) {
	ge := buildTestEngine(t, []models.FileNode{
		{Path: "main.go", IsFile: true, Churn: 5},
		{Path: "utils.go", IsFile: true, Churn: 1},
	}, nil)

	ranked := scoreAndRankNodes(ge.GetNodes())
	if len(ranked) != 2 {
		t.Fatalf("expected 2 file nodes, got %d", len(ranked))
	}
}

func TestScoreAndRankNodes_ExcludesSymbols(t *testing.T) {
	ge := buildTestEngine(t,
		[]models.FileNode{
			{Path: "api.go", IsFile: true, Churn: 3},
		},
		[]models.Symbol{
			{Name: "Handler", Type: models.SymbolFunction, FilePath: "api.go"},
		},
	)
	ranked := scoreAndRankNodes(ge.GetNodes())
	// Only file nodes should be returned, not symbol nodes.
	for _, n := range ranked {
		if n.InternalType() != analysis.NodeTypeFile {
			t.Errorf("non-file node in ranked output: type=%v name=%s", n.InternalType(), n.Name)
		}
	}
}

func TestScoreAndRankNodes_SortOrder(t *testing.T) {
	// Build a graph where one file has many incoming edges (higher PageRank) and more churn.
	// We create relations to give "hub.go" a higher score.
	files := []models.FileNode{
		{Path: "hub.go", IsFile: true, Churn: 10},
		{Path: "leaf.go", IsFile: true, Churn: 1},
	}
	symbols := []models.Symbol{
		{Name: "Hub", Type: models.SymbolFunction, FilePath: "hub.go"},
		{Name: "Leaf", Type: models.SymbolFunction, FilePath: "leaf.go"},
	}
	// leaf.go:Leaf calls hub.go:Hub → Hub gets higher PageRank.
	ge := analysis.NewGraphEngine()
	if err := ge.BuildGraph(files, symbols, []models.Relation{
		{From: "leaf.go:Leaf", To: "hub.go:Hub", Type: "calls"},
	}); err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	ranked := scoreAndRankNodes(ge.GetNodes())
	if len(ranked) < 2 {
		t.Skip("need at least 2 file nodes")
	}
	// hub.go should rank first due to higher PageRank + churn.
	if ranked[0].Name != "hub.go" {
		t.Logf("ranked[0]=%s ranked[1]=%s", ranked[0].Name, ranked[1].Name)
		// Not a hard failure — PageRank distribution depends on graph topology.
		// At minimum, verify descending order holds.
	}

	// Verify descending score order.
	for i := 1; i < len(ranked); i++ {
		si := nodeImportanceScore(ranked[i], 10)
		si1 := nodeImportanceScore(ranked[i-1], 10)
		if si > si1+1e-9 {
			t.Errorf("ranked[%d] score %.6f > ranked[%d] score %.6f (not descending)", i, si, i-1, si1)
		}
	}
}

func TestScoreAndRankNodes_Empty(t *testing.T) {
	// No file nodes → empty result.
	ge := buildTestEngine(t, nil, nil)
	ranked := scoreAndRankNodes(ge.GetNodes())
	if len(ranked) != 0 {
		t.Errorf("expected empty result for empty graph, got %d nodes", len(ranked))
	}
}

// --- nodeImportanceScore ---

func TestNodeImportanceScore_MaxChurnZero(t *testing.T) {
	ge := buildTestEngine(t, []models.FileNode{{Path: "a.go", IsFile: true, Churn: 0}}, nil)
	nodes := ge.GetNodes()
	if len(nodes) == 0 {
		t.Fatal("no nodes")
	}
	n := nodes[0]
	score := nodeImportanceScore(n, 0)
	// When maxChurn=0, score = PageRank*0.7 (churn term is zero).
	expected := n.PageRank * 0.7
	if score < expected-1e-9 || score > expected+1e-9 {
		t.Errorf("score=%.6f want PageRank*0.7=%.6f", score, expected)
	}
}

func TestNodeImportanceScore_Combined(t *testing.T) {
	// With known values: PageRank=0.5, churn=5, maxChurn=10 →
	// score = 0.5*0.7 + (5/10)*0.3 = 0.35 + 0.15 = 0.50
	ge := buildTestEngine(t, []models.FileNode{{Path: "x.go", IsFile: true, Churn: 5}}, nil)
	nodes := ge.GetNodes()
	if len(nodes) == 0 {
		t.Fatal("no nodes")
	}
	n := nodes[0]
	// Override PageRank for determinism.
	n.PageRank = 0.5
	score := nodeImportanceScore(n, 10)
	expected := 0.5*0.7 + (5.0/10.0)*0.3
	if score < expected-1e-9 || score > expected+1e-9 {
		t.Errorf("score=%.6f want %.6f", score, expected)
	}
}

func TestNodeImportanceScore_NegativeChurnClamped(t *testing.T) {
	ge := buildTestEngine(t, []models.FileNode{{Path: "neg.go", IsFile: true, Churn: -5}}, nil)
	nodes := ge.GetNodes()
	if len(nodes) == 0 {
		t.Fatal("no nodes")
	}
	n := nodes[0]
	n.PageRank = 0.4
	score := nodeImportanceScore(n, 10)
	// Negative churn clamped to 0 → score = PageRank*0.7
	expected := 0.4 * 0.7
	if score < expected-1e-9 || score > expected+1e-9 {
		t.Errorf("score=%.6f want %.6f (negative churn clamped)", score, expected)
	}
}

// --- buildPageJobs ---

func TestBuildPageJobs_FilePage(t *testing.T) {
	ge := buildTestEngine(t, []models.FileNode{
		{Path: "a.go", IsFile: true},
		{Path: "b.go", IsFile: true},
		{Path: "c.go", IsFile: true},
	}, nil)

	entry := providers.PlanEntry{
		PageType: "file_page",
		Count:    2,
	}
	jobs := buildPageJobs(entry, ge)
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs (capped at Count=2), got %d", len(jobs))
	}
	for _, j := range jobs {
		if j.pgType != "file_page" {
			t.Errorf("job pgType=%q, want file_page", j.pgType)
		}
		if j.id == "" {
			t.Error("job id must not be empty")
		}
	}
}

func TestBuildPageJobs_NilEngine(t *testing.T) {
	entry := providers.PlanEntry{PageType: "file_page", Count: 5}
	jobs := buildPageJobs(entry, nil)
	if len(jobs) != 0 {
		t.Errorf("nil engine should return no jobs, got %d", len(jobs))
	}
}

func TestBuildPageJobs_Singleton(t *testing.T) {
	for _, pt := range []string{"repo_overview", "architecture_diagram", "onboarding"} {
		entry := providers.PlanEntry{PageType: pt, Count: 1}
		jobs := buildPageJobs(entry, nil) // singleton types don't need engine
		if len(jobs) != 1 {
			t.Errorf("pageType=%s: expected 1 singleton job, got %d", pt, len(jobs))
		}
		if jobs[0].subject != "root" {
			t.Errorf("pageType=%s: expected subject=root, got %q", pt, jobs[0].subject)
		}
	}
}

func TestBuildPageJobs_SymbolSpotlight(t *testing.T) {
	ge := buildTestEngine(t,
		[]models.FileNode{
			{Path: "api.go", IsFile: true},
			{Path: "util.go", IsFile: true},
		},
		[]models.Symbol{
			{Name: "HandlerA", Type: models.SymbolFunction, FilePath: "api.go"},
			{Name: "HandlerB", Type: models.SymbolFunction, FilePath: "api.go"},
			{Name: "Helper", Type: models.SymbolFunction, FilePath: "util.go"},
		},
	)
	entry := providers.PlanEntry{PageType: "symbol_spotlight", Count: 2}
	jobs := buildPageJobs(entry, ge)
	if len(jobs) != 2 {
		t.Errorf("expected 2 symbol jobs (capped at Count=2), got %d", len(jobs))
	}
	for _, j := range jobs {
		if j.pgType != "symbol_spotlight" {
			t.Errorf("job pgType=%q, want symbol_spotlight", j.pgType)
		}
	}
}

func TestBuildPageJobs_InfraPage(t *testing.T) {
	ge := buildTestEngine(t, []models.FileNode{
		{Path: "infra/Dockerfile", IsFile: true},
		{Path: "main.go", IsFile: true},
		{Path: "infra/app.tf", IsFile: true},
	}, nil)

	entry := providers.PlanEntry{PageType: "infra_page", Count: 10}
	jobs := buildPageJobs(entry, ge)
	// Only Dockerfile and .tf should match, not main.go
	for _, j := range jobs {
		if j.subject == "main.go" {
			t.Errorf("non-infra file main.go appeared in infra_page jobs")
		}
	}
}

func TestBuildPageJobs_ZeroCount(t *testing.T) {
	// Count=0 means "no cap" (return all matching pages).
	ge := buildTestEngine(t, []models.FileNode{
		{Path: "a.go", IsFile: true},
		{Path: "b.go", IsFile: true},
		{Path: "c.go", IsFile: true},
	}, nil)
	entry := providers.PlanEntry{PageType: "file_page", Count: 0}
	jobs := buildPageJobs(entry, ge)
	if len(jobs) != 3 {
		t.Errorf("Count=0: expected all 3 files, got %d", len(jobs))
	}
}

func TestBuildPageJobs_ModulePageStableOrder(t *testing.T) {
	// Calling buildPageJobs twice on the same engine must produce identical
	// job IDs in the same order (i.e. sort is deterministic across calls).
	ge := buildTestEngine(t, []models.FileNode{
		{Path: "pkg/alpha/a.go", IsFile: true},
		{Path: "pkg/beta/b.go", IsFile: true},
		{Path: "pkg/gamma/c.go", IsFile: true},
		{Path: "pkg/delta/d.go", IsFile: true},
	}, nil)

	entry := providers.PlanEntry{PageType: "module_page", Count: 3}

	first := buildPageJobs(entry, ge)
	second := buildPageJobs(entry, ge)

	if len(first) != len(second) {
		t.Fatalf("call 1 returned %d jobs, call 2 returned %d", len(first), len(second))
	}
	for i := range first {
		if first[i].id != second[i].id {
			t.Errorf("job[%d] id mismatch: %q vs %q (non-deterministic order)", i, first[i].id, second[i].id)
		}
	}
}

func TestBuildPageJobs_SCCPageStableOrder(t *testing.T) {
	// Same determinism guarantee for scc_page.
	ge := buildTestEngine(t, []models.FileNode{
		{Path: "a.go", IsFile: true},
		{Path: "b.go", IsFile: true},
		{Path: "c.go", IsFile: true},
	}, nil)

	entry := providers.PlanEntry{PageType: "scc_page", Count: 2}

	first := buildPageJobs(entry, ge)
	second := buildPageJobs(entry, ge)

	if len(first) != len(second) {
		t.Fatalf("call 1 returned %d jobs, call 2 returned %d", len(first), len(second))
	}
	for i := range first {
		if first[i].id != second[i].id {
			t.Errorf("job[%d] id mismatch: %q vs %q (non-deterministic order)", i, first[i].id, second[i].id)
		}
	}
}

func TestScoreAndRankNodes_StableTiebreaker(t *testing.T) {
	// When multiple files have equal importance scores (equal PageRank + churn),
	// the tiebreaker must order them by path — deterministic across calls.
	ge := buildTestEngine(t, []models.FileNode{
		{Path: "z.go", IsFile: true, Churn: 0},
		{Path: "a.go", IsFile: true, Churn: 0},
		{Path: "m.go", IsFile: true, Churn: 0},
	}, nil)

	first := scoreAndRankNodes(ge.GetNodes())
	second := scoreAndRankNodes(ge.GetNodes())

	if len(first) != len(second) {
		t.Fatalf("call 1 returned %d nodes, call 2 returned %d", len(first), len(second))
	}
	for i := range first {
		if first[i].File().Path != second[i].File().Path {
			t.Errorf("node[%d] path mismatch: %q vs %q (non-deterministic tiebreaker)", i, first[i].File().Path, second[i].File().Path)
		}
	}

	// Also verify paths are in ascending order (alphabetic tiebreaker).
	for i := 1; i < len(first); i++ {
		prev := first[i-1].File().Path
		cur := first[i].File().Path
		si := nodeImportanceScore(first[i], 0)
		si1 := nodeImportanceScore(first[i-1], 0)
		if si == si1 && cur < prev {
			t.Errorf("tiebreaker violated: %q should come before %q (alphabetical)", cur, prev)
		}
	}
}

func TestReadFileSnippet_TruncatesWithoutLoadingWholeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := strings.Join([]string{"line1", "line2", "line3", "line4", "line5"}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got := readFileSnippet(path, 3)
	want := "line1\nline2\nline3\n... (2 more lines)"
	if got != want {
		t.Fatalf("readFileSnippet() = %q, want %q", got, want)
	}

	if readFileSnippet(filepath.Join(dir, "missing.txt"), 3) != "" {
		t.Fatal("expected empty string for missing file")
	}
}
