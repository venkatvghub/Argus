package cmd

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	initForceWiki    bool
	initResume       string
	initProvider     string
	initCheapModel   string
	initMediumModel  string
	initPremiumModel string
	initConcurrency  int
	initCoverage     float64
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

	if initCoverage != 0 && (initCoverage < 0.10 || initCoverage > 1.0) {
		return fmt.Errorf("--coverage must be between 0.10 and 1.0, got %.2f", initCoverage)
	}

	// ── Step 1: Analyze ────────────────────────────────────────────────────
	fmt.Fprintf(os.Stderr, "\n  Analyzing %s\n\n", repoPath)

	// --force-wiki: clear the persisted HEAD checkpoint so Analyze performs a
	// full re-analysis and populates the in-memory graph engine (required by wiki).
	if initForceWiki {
		absForce, _ := filepath.Abs(repoPath)
		forceRepoID := fmt.Sprintf("%x", sha256.Sum256([]byte(absForce)))[:constants.RepoIDLength]
		if resetErr := instance.ResetAnalysisCheckpoint(ctx, forceRepoID); resetErr != nil {
			fmt.Fprintf(os.Stderr, "  ⚠  Could not reset analysis checkpoint: %v\n", resetErr)
		}
	}

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

	// ── Incremental short-circuit ────────────────────────────────────────
	if instance.IsRepoUpToDate(repoID) && !initForceWiki {
		fmt.Fprintf(os.Stderr, "  Repository is up-to-date (HEAD unchanged). Nothing to regenerate.\n\n")
		return nil
	}

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

	// ── Step 3.5: Count full pages (needed for coverage selection) ───────
	files, _ := instance.GetRepoFiles(ctx, repoID)
	symbols, _ := instance.GetRepoSymbols(ctx, repoID)
	communityCount, _ := instance.GetCommunityCount(ctx, repoID)
	allCounts := providers.CountPages(files, len(symbols), communityCount)

	// ── Step 3.6: Coverage selection ─────────────────────────────────────
	cfg := instance.Config()

	// Determine coverage fraction: flag > env/default; interactive only on TTY.
	coveragePct := cfg.Coverage
	if initCoverage > 0 {
		coveragePct = initCoverage
	}

	fi, statErr := os.Stdin.Stat()
	isTTY := statErr == nil && (fi.Mode()&os.ModeCharDevice) != 0

	if isTTY && initCoverage == 0 && !initYes {
		// Use DefaultTieredConfig for cost estimates in the table.
		// The actual provider is selected next; this is display-only.
		tempTC, tcErr := providers.DefaultTieredConfig(cfg)
		if tcErr != nil {
			// No provider configured yet; skip cost display but still show page counts.
			tempTC = providers.TieredConfig{}
		}
		coverageOpts := providers.ComputeCoverageOptions(allCounts, tempTC, nil, files)
		coveragePct = interactiveCoverageSelect(coverageOpts)
	} else if initYes && initCoverage == 0 {
		coveragePct = providers.RecommendedCoverage
	}

	// ── Step 4: Provider setup ──────────────────────────────────────────
	// If resuming a paused (rate-limited) job, warn before model selection so the
	// user knows to pick a different model or provider to avoid hitting the same limit.
	if initResume != "" && isTTY {
		if prevJob, jobErr := instance.GetWikiJob(ctx, initResume); jobErr == nil && prevJob.Status == models.WikiJobPaused {
			fmt.Fprintf(os.Stderr, "\n  ⚠  Job %s was paused (rate limit). Select a different provider or models below to avoid hitting the same limit.\n", initResume)
		}
	}

	tc, pricingMap, err := resolveProviderConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("provider setup: %w", err)
	}

	// ── Step 5: Scale counts by coverage + build plan ───────────────────
	// scaleCounts is exported from coverage.go as an internal helper via
	// ComputeCoverageOptions; replicate the same scaling inline here so
	// the final plan accurately reflects the chosen coverage.
	scaledCounts := scaledPageCounts(allCounts, coveragePct)
	plan := providers.BuildPlan(scaledCounts, tc, pricingMap, files)

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
	router, err := providers.NewTieredRouter(cfg, tc, pricingMap)
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
	start := time.Now()
	frame := 0
	estimatedTotal := plan.TotalPages - len(completedPageIDs)

	onProgress := func(done, total int) {
		spinner := spinnerFrames[frame%len(spinnerFrames)]
		frame++
		bar := progressBar(done, total, 20)
		pct := 0
		if total > 0 {
			pct = done * 100 / total
		}
		fmt.Fprintf(os.Stderr, "\r\033[K  \033[33m%s\033[0m  Generating wiki pages...  %s  %d/%d  (%d%%)",
			spinner, bar, done, total, pct)
	}
	onProgress(0, estimatedTotal) // show initial bar

	if err := instance.GenerateWiki(ctx, repoID, wikiJobID, plan, router, completedPageIDs, initConcurrency, onProgress); err != nil {
		fmt.Fprintf(os.Stderr, "\n  ⚠  Generation interrupted: %v\n", err)
		fmt.Fprintf(os.Stderr, "\n  Models used:\n")
		fmt.Fprintf(os.Stderr, "    cheap:   %s\n", tc.CheapModel)
		fmt.Fprintf(os.Stderr, "    medium:  %s\n", tc.MediumModel)
		fmt.Fprintf(os.Stderr, "    premium: %s\n", tc.PremiumModel)
		fmt.Fprintf(os.Stderr, "\n  To switch models on resume, pass --cheap-model, --medium-model, or --premium-model.\n")
		fmt.Fprintf(os.Stderr, "  Resume with: argus init %s --resume %s\n", repoPath, wikiJobID)
		return err
	}

	elapsed := time.Since(start).Round(time.Second)
	fmt.Fprintf(os.Stderr, "\r\033[K  \033[32m✓\033[0m  Wiki generation complete  (%d pages in %s)\n\n", estimatedTotal, elapsed)
	return nil
}

// scaledPageCounts scales fullCounts to the given coverage fraction, matching
// the logic in providers.scaleCounts (singletons always stay at 0 or 1).
func scaledPageCounts(fullCounts providers.PageCounts, pct float64) providers.PageCounts {
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
	return providers.PageCounts{
		FilePage:            clamp(fullCounts.FilePage),
		SymbolSpotlight:     clamp(fullCounts.SymbolSpotlight),
		ModulePage:          clamp(fullCounts.ModulePage),
		SCCPage:             clamp(fullCounts.SCCPage),
		APIContract:         clamp(fullCounts.APIContract),
		InfraPage:           clamp(fullCounts.InfraPage),
		RepoOverview:        fullCounts.RepoOverview,
		ArchitectureDiagram: fullCounts.ArchitectureDiagram,
		Onboarding:          fullCounts.Onboarding,
	}
}

// interactiveCoverageSelect prints a coverage selection table and prompts the user.
// Returns the selected coverage fraction.
func interactiveCoverageSelect(opts []providers.CoverageOption) float64 {
	defaultIdx := 1 // 1-based
	for i, o := range opts {
		if o.Recommended {
			defaultIdx = i + 1
			break
		}
	}

	fmt.Fprintln(os.Stderr, "\n  Coverage Options")
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────────────────────────────")
	fmt.Fprintf(os.Stderr, "  %-4s %-14s %6s %6s %5s %5s %5s %5s %5s   %s\n",
		"#", "Coverage", "Pages", "File", "Sym", "Mod", "API", "SCC", "Onb", "Est. Cost")
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────────────────────────────")

	for i, o := range opts {
		label := fmt.Sprintf("%.0f%%", o.Pct*100)
		if o.Recommended {
			label = fmt.Sprintf("%.0f%% (rec)", o.Pct*100)
		}
		totalPages := o.Counts.FilePage + o.Counts.SymbolSpotlight + o.Counts.ModulePage +
			o.Counts.SCCPage + o.Counts.APIContract + o.Counts.InfraPage +
			o.Counts.RepoOverview + o.Counts.ArchitectureDiagram + o.Counts.Onboarding

		costStr := fmt.Sprintf("$%.2f – $%.2f", o.MinCost, o.MaxCost)
		if o.MinCost == 0 && o.MaxCost == 0 {
			costStr = "—"
		}

		fmt.Fprintf(os.Stderr, "  %-4d %-14s %6d %6d %5d %5d %5d %5d %5d   %s\n",
			i+1, label, totalPages,
			o.Counts.FilePage, o.Counts.SymbolSpotlight, o.Counts.ModulePage,
			o.Counts.APIContract, o.Counts.SCCPage, o.Counts.Onboarding,
			costStr,
		)
	}
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────────────────────────────")

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "\n  Select coverage [%d]: ", defaultIdx)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return opts[defaultIdx-1].Pct
		}
		idx, err := strconv.Atoi(line)
		if err != nil || idx < 1 || idx > len(opts) {
			fmt.Fprintf(os.Stderr, "  Invalid selection %q — enter a number between 1 and %d.\n", line, len(opts))
			continue
		}
		return opts[idx-1].Pct
	}
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
			line := renderProgressLine(current)
			fmt.Fprintf(os.Stderr, "\r\033[K  \033[33m%s\033[0m  %s", spinner, line)
		}
	}
}

// resolveProviderConfig determines TieredConfig from flags + env + interactive selection.
// Also returns the live pricing map (non-nil only for OpenRouter discovery).
func resolveProviderConfig(ctx context.Context, cfg *config.Config) (providers.TieredConfig, map[string][2]float64, error) {
	// If --provider flag given, skip discovery — no live pricing available.
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
			return providers.TieredConfig{}, nil, fmt.Errorf("unknown provider %q", initProvider)
		}
		tc := providers.TieredConfig{
			ProviderName: ps.Name,
			CheapModel:   coalesce(initCheapModel, ps.DefaultCheap),
			MediumModel:  coalesce(initMediumModel, ps.DefaultMedium),
			PremiumModel: coalesce(initPremiumModel, ps.DefaultPremium),
		}
		return tc, nil, nil
	}

	// Check if running interactively (tty)
	fi, statErr := os.Stdin.Stat()
	isTTY := statErr == nil && (fi.Mode()&os.ModeCharDevice) != 0

	statuses := providers.DetectProviders(cfg)

	if isTTY {
		tc, pm, err := interactiveProviderSelect(ctx, cfg, statuses)
		return tc, pm, err
	}

	// Non-interactive: use default (first available), no live pricing.
	tc, err := providers.DefaultTieredConfig(cfg)
	if err != nil {
		return tc, nil, err
	}
	tc.CheapModel = coalesce(initCheapModel, tc.CheapModel)
	tc.MediumModel = coalesce(initMediumModel, tc.MediumModel)
	tc.PremiumModel = coalesce(initPremiumModel, tc.PremiumModel)
	return tc, nil, nil
}

// interactiveProviderSelect shows a provider selection table and prompts the user.
// Also returns the live pricing map from model discovery (non-nil for OpenRouter).
func interactiveProviderSelect(ctx context.Context, cfg *config.Config, statuses []providers.ProviderStatus) (providers.TieredConfig, map[string][2]float64, error) {
	fmt.Fprintln(os.Stderr, "\n  Available Providers")
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────────────────")
	fmt.Fprintf(os.Stderr, "  %-4s %-12s %-14s %-22s  %s\n", "#", "Provider", "Status", "Endpoint", "Default (cheap/medium/premium)")
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────────────────")

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
		fmt.Fprintf(os.Stderr, "  %-4d %-12s %-14s %-22s  %s\n", i+1, s.Name, status, s.Endpoint, mods)
	}
	fmt.Fprintln(os.Stderr, "  ──────────────────────────────────────────────────────────────────────")

	if defaultIdx < 0 {
		return providers.TieredConfig{}, nil, fmt.Errorf("no LLM provider configured: set ARGUS_OPENAI_API_KEY, ARGUS_ANTHROPIC_API_KEY, or ARGUS_GEMINI_API_KEY")
	}

	fmt.Fprintf(os.Stderr, "\n  Select provider [%d]: ", defaultIdx)
	var input string
	fmt.Fscanln(os.Stdin, &input)
	if input == "" {
		input = fmt.Sprintf("%d", defaultIdx)
	}

	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(statuses) {
		return providers.TieredConfig{}, nil, fmt.Errorf("invalid selection %q", input)
	}

	selected := statuses[idx-1]
	if !selected.Available {
		return providers.TieredConfig{}, nil, fmt.Errorf("provider %s has no API key configured", selected.Name)
	}

	// openrouter uses the openai backend (same HTTP client, different base URL)
	providerBackend := selected.Name
	if providerBackend == "openrouter" {
		providerBackend = "openai"
	}

	cheap, medium, premium, pricingMap, err := selectTierModels(ctx, cfg, selected)
	if err != nil {
		return providers.TieredConfig{}, nil, err
	}

	tc := providers.TieredConfig{
		ProviderName: providerBackend,
		CheapModel:   cheap,
		MediumModel:  medium,
		PremiumModel: premium,
	}
	return tc, pricingMap, nil
}

// selectTierModels discovers available models (if needed) and prompts user to pick per tier.
// Returns chosen models and the live pricing map (non-nil when OpenRouter discovery succeeded).
func selectTierModels(ctx context.Context, cfg *config.Config, selected providers.ProviderStatus) (cheap, medium, premium string, pricingMap map[string][2]float64, err error) {
	// Coalesce: CLI flag > env var > provider default
	cheapOverride := coalesce(initCheapModel, cfg.CheapModel)
	mediumOverride := coalesce(initMediumModel, cfg.MediumModel)
	premiumOverride := coalesce(initPremiumModel, cfg.PremiumModel)

	// Always fetch pricing from provider API — needed for cost estimation even when
	// models are pre-set via env/flags. Model list is only used for interactive prompts.
	fmt.Fprintln(os.Stderr, "  Fetching available models...")
	discoveredModels, pricingMap, discoverErr := providers.DiscoverModels(ctx, cfg)
	if discoverErr != nil {
		fmt.Fprintf(os.Stderr, "  ⚠  Model discovery failed (%v); cost estimates may show $0.00.\n", discoverErr)
		pricingMap = nil
	}

	// If all three tiers already set, skip interactive prompts.
	if cheapOverride != "" && mediumOverride != "" && premiumOverride != "" {
		fmt.Fprintf(os.Stderr, "  cheap:   %s (from env/flag)\n", cheapOverride)
		fmt.Fprintf(os.Stderr, "  medium:  %s (from env/flag)\n", mediumOverride)
		fmt.Fprintf(os.Stderr, "  premium: %s (from env/flag)\n", premiumOverride)
		return cheapOverride, mediumOverride, premiumOverride, pricingMap, nil
	}

	buckets := providers.BucketByTier(discoveredModels)

	cheap = pickTierModel(ctx, "cheap  (high-volume, low-cost pages)", cheapOverride, selected.DefaultCheap, buckets["cheap"], pricingMap)
	medium = pickTierModel(ctx, "medium (module/API/SCC pages)", mediumOverride, selected.DefaultMedium, buckets["medium"], pricingMap)
	premium = pickTierModel(ctx, "premium (overview/arch/onboarding)", premiumOverride, selected.DefaultPremium, buckets["premium"], pricingMap)
	return cheap, medium, premium, pricingMap, nil
}

// pickTierModel shows a numbered list of candidates for a tier and prompts the user.
// If override is non-empty it is returned immediately. Falls back to fallbackDefault if no candidates.
func pickTierModel(_ context.Context, label, override, fallbackDefault string, candidates []string, pricingMap map[string][2]float64) string {
	if override != "" {
		fmt.Fprintf(os.Stderr, "  %s\n    → %s (from env/flag)\n", label, override)
		return override
	}

	// Cap candidates list at 8, sorted (already sorted by BucketByTier)
	shown := candidates
	if len(shown) > 8 {
		shown = shown[:8]
	}

	if len(shown) == 0 {
		// No candidates — fall back to default with simple prompt
		return promptModel(label, fallbackDefault)
	}

	fmt.Fprintf(os.Stderr, "\n  %s\n", label)
	for i, m := range shown {
		costHint := ""
		if p, ok := pricingMap[m]; ok && (p[0] > 0 || p[1] > 0) {
			costHint = fmt.Sprintf("  ($%.4f/$%.4f per 1M in/out)", p[0], p[1])
		}
		fmt.Fprintf(os.Stderr, "    %d) %s%s\n", i+1, m, costHint)
	}
	fmt.Fprintf(os.Stderr, "    %d) type a model name\n", len(shown)+1)
	fmt.Fprintf(os.Stderr, "    Model [1 = %s]: ", shown[0])

	var input string
	fmt.Fscanln(os.Stdin, &input)
	if input == "" {
		return shown[0]
	}

	// Numeric selection
	n := 0
	for _, c := range input {
		if c < '0' || c > '9' {
			n = -1
			break
		}
		n = n*10 + int(c-'0')
	}
	if n >= 1 && n <= len(shown) {
		return shown[n-1]
	}
	if n == len(shown)+1 {
		// User selected the "type a model name" option.
		fmt.Fprintf(os.Stderr, "    Model name: ")
		var custom string
		fmt.Fscanln(os.Stdin, &custom)
		if custom != "" {
			return custom
		}
		return shown[0]
	}
	// Non-numeric input → treat as literal model name
	if input != "" {
		return input
	}
	return shown[0]
}

// promptModel shows the current default and lets the user accept or override it.
func promptModel(label, defaultModel string) string {
	fmt.Fprintf(os.Stderr, "  %s\n    Model [%s]: ", label, defaultModel)
	var input string
	fmt.Fscanln(os.Stdin, &input)
	if input == "" {
		return defaultModel
	}
	return input
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
	initCmd.Flags().BoolVar(&initForceWiki, "force-wiki", false, "force wiki regeneration even if HEAD is unchanged")
	initCmd.Flags().StringVar(&initResume, "resume", "", "resume a previous wiki generation job ID")
	initCmd.Flags().StringVar(&initProvider, "provider", "", "force LLM provider (anthropic|openai|gemini)")
	initCmd.Flags().StringVar(&initCheapModel, "cheap-model", "", "override cheap-tier model")
	initCmd.Flags().StringVar(&initMediumModel, "medium-model", "", "override medium-tier model")
	initCmd.Flags().StringVar(&initPremiumModel, "premium-model", "", "override premium-tier model")
	initCmd.Flags().IntVar(&initConcurrency, "concurrency", 5, "max concurrent LLM calls")
	initCmd.Flags().Float64Var(&initCoverage, "coverage", 0, "coverage fraction 0.10-1.0 (default: interactive)")
}
