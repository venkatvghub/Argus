package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/constants"
	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/providers"
)

var (
	initIndexOnly    bool
	initYes          bool
	initResume       string
	initProvider     string
	initCheapModel   string
	initMediumModel  string
	initPremiumModel string
	initConcurrency  int
)

var initCmd = &cobra.Command{
	Use:   "init <repo-path>",
	Short: "Analyze a repository and generate wiki documentation",
	Long: `init performs full codebase analysis, computes health scores, and generates
wiki documentation using an LLM provider. Use --index-only to skip wiki generation.`,
	Args: cobra.ExactArgs(1),
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	repoPath := args[0]

	// ── Step 1: Analyze ────────────────────────────────────────────────────
	fmt.Fprintf(os.Stderr, "\n  Analyzing %s\n\n", repoPath)

	jobID, err := instance.Analyze(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("analyze: %w", err)
	}

	// Wait for analysis to complete (always -- init is synchronous)
	if err := waitForJob(ctx, jobID); err != nil {
		return err
	}

	// Compute repoID (mirrors argus.go: sha256(absPath)[:constants.RepoIDLength])
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	repoID := fmt.Sprintf("%x", sha256.Sum256([]byte(absPath)))[:constants.RepoIDLength]

	// ── Step 2: Score ─────────────────────────────────────────────────────
	score, err := instance.GetRepoScore(ctx, repoID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠  Could not compute health score: %v\n", err)
	} else {
		markers, _ := instance.GetRepoMarkers(ctx, repoID)
		files, _ := instance.GetRepoFiles(ctx, repoID)
		fmt.Fprintf(os.Stderr, "  Health score: %.1f/10  (%d files, %d markers)\n\n",
			score, len(files), len(markers))
	}

	// ── Step 3: Index-only exit ─────────────────────────────────────────
	if initIndexOnly {
		fmt.Fprintln(os.Stderr, "  --index-only: skipping wiki generation.")
		return nil
	}

	// ── Step 4: Provider setup ──────────────────────────────────────────
	cfg := instance.Config()

	tc, err := resolveProviderConfig(cfg)
	if err != nil {
		return fmt.Errorf("provider setup: %w", err)
	}

	// ── Step 5: Count pages + build plan ────────────────────────────────
	files, _ := instance.GetRepoFiles(ctx, repoID)
	symbols, _ := instance.GetRepoSymbols(ctx, repoID)
	communityCount, _ := instance.GetCommunityCount(ctx, repoID)
	counts := providers.CountPages(files, len(symbols), communityCount)
	plan := providers.BuildPlan(counts, tc)

	if plan.TotalPages == 0 {
		fmt.Fprintln(os.Stderr, "  No pages to generate (run analyze first).")
		return nil
	}

	// ── Step 6: Print plan table ────────────────────────────────────────
	printPlanTable(plan)

	// ── Step 7: Cost gate ────────────────────────────────────────────────
	if !initYes && plan.TotalCost > providers.CostGate {
		fmt.Fprintf(os.Stderr, "\n  ⚠  Estimated cost ($%.2f) exceeds $%.0f. Proceed? [y/N] ",
			plan.TotalCost, providers.CostGate)
		var resp string
		fmt.Fscanln(os.Stdin, &resp)
		if resp != "y" && resp != "Y" {
			fmt.Fprintln(os.Stderr, "  Aborted.")
			return nil
		}
	} else if !initYes {
		fmt.Fprintf(os.Stderr, "\n  Proceed with generation? [Y/n] ")
		var resp string
		fmt.Fscanln(os.Stdin, &resp)
		if resp == "n" || resp == "N" {
			fmt.Fprintln(os.Stderr, "  Aborted.")
			return nil
		}
	}

	// ── Step 8: Build tiered router ─────────────────────────────────────
	router, err := providers.NewTieredRouter(cfg, tc)
	if err != nil {
		return fmt.Errorf("build router: %w", err)
	}

	// ── Step 9: Checkpoint setup ─────────────────────────────────────────
	var wikiJobID string
	var completedPageIDs map[string]struct{}

	if initResume != "" {
		wikiJobID = initResume
		completed, err := instance.GetCompletedWikiPages(ctx, wikiJobID)
		if err != nil {
			return fmt.Errorf("load resume checkpoint: %w", err)
		}
		completedPageIDs = completed
		fmt.Fprintf(os.Stderr, "\n  Resuming job %s (%d/%d pages already complete)\n",
			wikiJobID, len(completedPageIDs), plan.TotalPages)
	} else {
		wikiJobID, err = instance.CreateWikiJob(ctx, repoID, plan.TotalPages)
		if err != nil {
			return fmt.Errorf("create wiki job: %w", err)
		}
		fmt.Fprintf(os.Stderr, "\n  Job ID: %s  (resume with --resume %s)\n", wikiJobID, wikiJobID)
	}

	// ── Step 10: Wiki generation ─────────────────────────────────────────
	fmt.Fprintln(os.Stderr, "\n  Generating wiki pages...")
	start := time.Now()

	if err := instance.GenerateWiki(ctx, repoID, wikiJobID, plan, router, completedPageIDs, initConcurrency); err != nil {
		fmt.Fprintf(os.Stderr, "\n  ⚠  Generation interrupted: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Resume with: argus init %s --resume %s\n", repoPath, wikiJobID)
		return err
	}

	elapsed := time.Since(start).Round(time.Second)
	fmt.Fprintf(os.Stderr, "\n  ✓ Wiki generation complete  (%d pages in %s)\n\n", plan.TotalPages-len(completedPageIDs), elapsed)
	return nil
}

// waitForJob polls until the analysis job completes or fails.
func waitForJob(ctx context.Context, jobID string) error {
	frame := 0
	lastPhase := ""
	start := time.Now()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr)
			return ctx.Err()
		case <-ticker.C:
		}

		job, ok := instance.Jobs.GetJob(jobID)
		if !ok {
			return fmt.Errorf("job %s not found", jobID)
		}

		current := job.Progress
		spinner := spinnerFrames[frame%len(spinnerFrames)]
		frame++

		if phase(current) != phase(lastPhase) && lastPhase != "" {
			fmt.Fprintf(os.Stderr, "\r\033[K  \033[32m✓\033[0m  %s\n", phaseLabel(lastPhase))
		}
		lastPhase = current

		switch job.Status {
		case models.JobStatusCompleted:
			elapsed := time.Since(start).Round(time.Millisecond)
			fmt.Fprintf(os.Stderr, "\r\033[K  \033[32m✓\033[0m  %s\n", phaseLabel(current))
			fmt.Fprintf(os.Stderr, "  \033[2mdone in %s\033[0m\n\n", elapsed)
			return nil
		case models.JobStatusFailed:
			fmt.Fprintf(os.Stderr, "\r\033[K  \033[31m✗\033[0m  Failed: %s\n", job.Error)
			return fmt.Errorf("analysis failed: %s", job.Error)
		default:
			line := renderProgressLine(spinner, current)
			fmt.Fprintf(os.Stderr, "\r\033[K  \033[33m%s\033[0m  %s", spinner, line)
		}
	}
}

// resolveProviderConfig determines TieredConfig from flags + env + interactive selection.
func resolveProviderConfig(cfg *config.Config) (providers.TieredConfig, error) {
	// If --provider flag given, use it directly
	if initProvider != "" {
		statuses := providers.DetectProviders(cfg)
		var ps *providers.ProviderStatus
		for i := range statuses {
			if statuses[i].Name == initProvider {
				ps = &statuses[i]
				break
			}
		}
		if ps == nil {
			return providers.TieredConfig{}, fmt.Errorf("unknown provider %q", initProvider)
		}
		tc := providers.TieredConfig{
			ProviderName: ps.Name,
			CheapModel:   coalesce(initCheapModel, ps.DefaultCheap),
			MediumModel:  coalesce(initMediumModel, ps.DefaultMedium),
			PremiumModel: coalesce(initPremiumModel, ps.DefaultPremium),
		}
		return tc, nil
	}

	// Check if running interactively (tty)
	fi, _ := os.Stdin.Stat()
	isTTY := (fi.Mode() & os.ModeCharDevice) != 0

	statuses := providers.DetectProviders(cfg)

	if isTTY {
		return interactiveProviderSelect(statuses)
	}

	// Non-interactive: use default (first available)
	tc, err := providers.DefaultTieredConfig(cfg)
	if err != nil {
		return tc, err
	}
	// Apply flag overrides
	tc.CheapModel = coalesce(initCheapModel, tc.CheapModel)
	tc.MediumModel = coalesce(initMediumModel, tc.MediumModel)
	tc.PremiumModel = coalesce(initPremiumModel, tc.PremiumModel)
	return tc, nil
}

// interactiveProviderSelect shows a provider selection table and prompts the user.
func interactiveProviderSelect(statuses []providers.ProviderStatus) (providers.TieredConfig, error) {
	fmt.Fprintln(os.Stderr, "\n  Available Providers")
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────")
	fmt.Fprintf(os.Stderr, "  %-4s %-12s %-14s %-28s\n", "#", "Provider", "Status", "Default (cheap/medium/premium)")
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────")

	defaultIdx := -1
	for i, s := range statuses {
		status := "✗ no key"
		mods := "—"
		if s.Available {
			status = "✓ configured"
			mods = fmt.Sprintf("%s / %s / %s", s.DefaultCheap, s.DefaultMedium, s.DefaultPremium)
			if defaultIdx < 0 {
				defaultIdx = i + 1
			}
		}
		fmt.Fprintf(os.Stderr, "  %-4d %-12s %-14s %-28s\n", i+1, s.Name, status, mods)
	}
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────")

	if defaultIdx < 0 {
		return providers.TieredConfig{}, fmt.Errorf("no LLM provider configured: set ARGUS_OPENAI_API_KEY, ARGUS_ANTHROPIC_API_KEY, or ARGUS_GEMINI_API_KEY")
	}

	fmt.Fprintf(os.Stderr, "\n  Select provider [%d]: ", defaultIdx)
	var input string
	fmt.Fscanln(os.Stdin, &input)
	if input == "" {
		input = fmt.Sprintf("%d", defaultIdx)
	}

	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(statuses) {
		return providers.TieredConfig{}, fmt.Errorf("invalid selection %q", input)
	}

	selected := statuses[idx-1]
	if !selected.Available {
		return providers.TieredConfig{}, fmt.Errorf("provider %s has no API key configured", selected.Name)
	}

	tc := providers.TieredConfig{
		ProviderName: selected.Name,
		CheapModel:   coalesce(initCheapModel, selected.DefaultCheap),
		MediumModel:  coalesce(initMediumModel, selected.DefaultMedium),
		PremiumModel: coalesce(initPremiumModel, selected.DefaultPremium),
	}
	return tc, nil
}

// printPlanTable prints the generation plan in a formatted table.
func printPlanTable(plan providers.GenerationPlan) {
	fmt.Fprintln(os.Stderr, "\n  Page Generation Plan")
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────")
	fmt.Fprintf(os.Stderr, "  %-4s %-22s %6s  %-24s  %9s\n", "#", "Page Type", "Pages", "Model", "Est. Cost")
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────")
	for i, e := range plan.Entries {
		model := e.Model
		if len(model) > 24 {
			model = model[:21] + "..."
		}
		fmt.Fprintf(os.Stderr, "  %-4d %-22s %6d  %-24s  $%8.2f\n",
			i+1, e.PageType, e.Count, model, e.EstCost)
	}
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────")
	fmt.Fprintf(os.Stderr, "  %-4s %-22s %6d  %-24s  $%8.2f\n",
		"", "TOTAL", plan.TotalPages, "", plan.TotalCost)
}

// coalesce returns a if non-empty, else b.
func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func init() {
	initCmd.Flags().BoolVar(&initIndexOnly, "index-only", false, "skip wiki generation, only analyze and score")
	initCmd.Flags().BoolVar(&initYes, "yes", false, "skip all confirmation prompts")
	initCmd.Flags().StringVar(&initResume, "resume", "", "resume a previous wiki generation job ID")
	initCmd.Flags().StringVar(&initProvider, "provider", "", "force LLM provider (anthropic|openai|gemini)")
	initCmd.Flags().StringVar(&initCheapModel, "cheap-model", "", "override cheap-tier model")
	initCmd.Flags().StringVar(&initMediumModel, "medium-model", "", "override medium-tier model")
	initCmd.Flags().StringVar(&initPremiumModel, "premium-model", "", "override premium-tier model")
	initCmd.Flags().IntVar(&initConcurrency, "concurrency", 5, "max concurrent LLM calls")
}
