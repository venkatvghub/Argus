Comprehensive Plan: argus init — Analyze → Score → LLM Wiki

  Full Flow

  argus init <repo-path> [flags]

  Step 1: ANALYZE
    └─ GitWalker.Walk() → nodes, symbols
    └─ GraphEngine.BuildGraph() + DetectCommunities()
    └─ MarkerEngine.Run() → markers (with Suggestion populated)
    └─ db.UpsertMarkers() + db.UpsertRepository()
    └─ engines[repoID], markers[repoID] in-memory

  Step 2: SCORE (immediate, in same process)
    └─ ComputeRepoScore() → float64
    └─ Print: "Repo health: 7.4/10  (142 files, 34 markers)"

  Step 3: PROVIDER SETUP (if not --index-only)
    └─ DetectAvailableProviders() → which API keys are set
    └─ If tty + not --provider: interactive_provider_select()
       └─ Show table: #, Provider, Status, Default Model
       └─ Prompt for number
       └─ If not --cheap-model: show model prompt for each tier
    └─ ValidateModels() → on ErrModelNotFound: list + numbered picker
    └─ Build TieredRouter{cheap, medium, premium}

  Step 4: PLAN TABLE
    └─ CountPagesByType(nodes, symbols) → page counts
    └─ EstimateCost(counts, tiers, pricing) → GenerationPlan
    └─ Print plan table (page type, count, model, est cost)
    └─ If total > $10: warn + require explicit confirm
    └─ If --yes: skip prompt

  Step 5: CHECKPOINT SETUP
    └─ If --resume <jobID>: load checkpoint.GetCompleted(jobID) → skip done pages
    └─ Else: checkpoint.Create(repoID) → new jobID
    └─ Print: "Job ID: abc123  (use --resume abc123 to continue if interrupted)"

  Step 6: WIKI GENERATION (8 levels, concurrent within level)
    L0: symbol_spotlight × N   [cheap, parallel, semaphore(concurrency)]
    L1: file_page × N          [cheap, parallel]
    L2: api_contract, infra_page [medium, parallel]
    L3: module_page, scc_page  [medium, parallel]
    L4: architecture_diagram   [premium]
    L5: repo_overview          [premium]
    L6: onboarding             [premium]
    └─ After each page: checkpoint.MarkComplete(jobID, pageID)
    └─ On interrupt: checkpoint file preserved → user can --resume

  Step 7: PERSIST + REPORT
    └─ db.UpsertWikiPages(repoID, pages)
    └─ Print: summary table (pages generated, actual cost, duration)

  ---
  Gap Table: What Needs to Be Built

  ┌─────────────────────────────────────────┬─────────┬─────────────────────────────────────────┐
  │                Component                │ Status  │                  Work                   │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ Marker.Suggestion field                 │ Missing │ Add to models.Marker                    │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ pkg/analysis/suggestions.go             │ Missing │ Static map, For(type) string            │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ MarkerEngine populates Suggestion       │ Missing │ Wire in markers.go                      │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ pkg/providers/pricing.go                │ Missing │ Token heuristics + pricing table        │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ pkg/providers/planner.go                │ Missing │ GenerationPlan, EstimateCost, BuildPlan │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ pkg/providers/selector.go               │ Missing │ Interactive tty selection flow          │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ Router.Active() method                  │ Missing │ Expose active provider name             │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ NewTieredRouter(cheap, med, prem)       │ Missing │ Constructor for 3-tier routing          │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ pkg/jobs/checkpoint.go                  │ Missing │ JSON file per job, complete tracking    │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ pkg/argus/wiki.go                       │ Missing │ GenerateWiki(ctx, repoID, plan, router) │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ cmd/argus/cmd/init.go                   │ Missing │ Full init command with all flags        │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ models.WikiPage + DB migration          │ Missing │ Store generated wiki pages              │
  ├─────────────────────────────────────────┼─────────┼─────────────────────────────────────────┤
  │ Remove model defaults from .env.example │ Pending │ Edit .env.example, config.go            │
  └─────────────────────────────────────────┴─────────┴─────────────────────────────────────────┘

  ---
  Phase Breakdown

  Phase A: Suggestions (small, high value, standalone)

  1. Add Suggestion string to models.Marker
  2. Create pkg/analysis/suggestions.go — static map (all 11 + Argus-specific AppSec markers)
  3. Populate Suggestion in MarkerEngine.Run() after each marker is created
  4. Update markers list / markers file CLI output to display suggestions

  Phase B: Cost Engine + Pricing (no LLM yet)

  1. pkg/providers/pricing.go — heuristics table, pricing table, EstimateCost(plan) float64
  2. pkg/providers/planner.go — GenerationPlan struct, BuildPlan(nodes, symbols, tiers) GenerationPlan
  3. pkg/providers/selector.go — SelectProvider(cfg, flags) TieredConfig (tty-aware)
  4. Router.Active() string + NewTieredRouter(cheap, medium, premium) in router.go

  Phase C: Checkpoint System

  1. pkg/jobs/checkpoint.go — Checkpoint struct, JSON files in ~/.argus/jobs/
  2. Job model in pkg/models/models.go — status enum already exists
  3. argus jobs list subcommand

  Phase D: Wiki Generation

  1. pkg/models/models.go — WikiPage struct
  2. pkg/persistence/migrations/000003_create_wiki_pages.up.sql
  3. pkg/argus/wiki.go — GenerateWiki(ctx, repoID, plan GenerationPlan, router TieredRouter) error
    - 8-level concurrent generation
    - Context assembly per page type (pull from graph + markers)
    - LLM call via tiered router
    - Checkpoint after each page

  Phase E: argus init Command

  1. cmd/argus/cmd/init.go — full command wiring
  2. Remove model defaults from backend/.env.example and config.go
  3. Wire all phases A–D together

  ---
  Suggestion Display in CLI Output

  Currently markers list shows:
    FILE                    TYPE              SEV    LINE  DEDUCTION
    src/auth/handler.go     brain_method      HIGH   142   1.5

  After Phase A, add:
    FILE                    TYPE              SEV    LINE  DEDUCTION
    src/auth/handler.go     brain_method      HIGH   142   1.5
    → Break this method into smaller focused functions. Extract complex
      conditional blocks, apply Command or Strategy pattern...

  ---
  Resume UX

  # First run (interrupted at L3)
  argus init ./myrepo
    Job ID: abc123
    ...generating...  [interrupted]

  # Resume
  argus init ./myrepo --resume abc123
    Resuming job abc123 (87/156 pages complete)
    Skipping: 87 already-completed pages
    ...continuing from L3...

  ---
  What to Implement First

  Recommended order (most value, least risk):

  1. Suggestions (Phase A) — standalone, no new deps, immediately visible in markers output
  2. Config cleanup — remove model env defaults from .env.example (tiny, blocks confusion)
  3. Cost engine (Phase B) — can be unit-tested without LLM, needed for init
  4. Checkpoint (Phase C) — needed before wiki gen to be resumable
  5. Wiki gen (Phase D) — needs B+C
  6. init command (Phase E) — wires everything

  Want me to start with Phase A (suggestions) now?
