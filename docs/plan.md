# Argus Go Rewrite: Implementation Plan

---

## 1. Vision & Core Principles

Argus is being developed as a high-performance, purely static Go binary. It serves as both a standalone CLI tool and a programmatic engine for deep codebase intelligence.

### Foundational Mandates
- **Programmatic-First Architecture:** Core logic is decoupled from CLI/HTTP layers and exported in `pkg/`. It can be embedded in any Go application via `argus.New(config)`.
- **Zero-CGo Portability:** Use `modernc.org/sqlite` and pure Go implementations (e.g., Leiden) to ensure seamless cross-compilation to single static binaries.
- **Decoupled Execution:** No requirement to run from within a target repo. Use `--repo-path` to target any directory.
- **Externalized State:** Storage (`--data-dir`) and Documentation (`--docs-dir`) are configurable and kept outside the target source tree.
- **Tiered Model Strategy:** Explicitly support "cheap" (e.g., Gemini Flash) vs. "premium" (e.g., Claude Sonnet) model configurations for cost-optimized analysis.
- **Structured Forensics:** High-performance logging via `uber-go/zap` (ported from Argonaut) with context propagation.

---

## 2. Repository Structure

```text
Argus/
├── backend/               # High-performance Go implementation
│   ├── cmd/argus/      # CLI entrypoint (cobra)
│   ├── pkg/               # EXPORTED: Core Library
│   │   ├── argus/      # Public API: Instance management
│   │   ├── config/        # envconfig models
│   │   ├── logger/        # zap logger (Argonaut port)
│   │   └── ...
│   ├── internal/          # Engine internals
│   └── go.mod
├── web/                   # Next.js 15 Dashboard
├── docs/                  # Architecture and Phase plans
└── data/                  # Externalized state (SQLite, Docs)
```

---

## 3. Dependency Matrix

| Concern | Implementation | Notes |
|---|---|---|
| **CLI** | `spf13/cobra` | Industry standard for Go CLIs. |
| **Config** | `kelseyhightower/envconfig` | Struct-first, lightweight configuration. |
| **Logging** | `go.uber.org/zap` | High-performance, structured (Argonaut port). |
| **Persistence** | `modernc.org/sqlite` | Zero-CGo, fully static SQLite. |
| **Migrations** | `golang-migrate` | Versioned SQL schema management. |
| **AST** | `smacker/go-tree-sitter` | Grammar bindings for 12+ languages. |
| **Graph** | `gonum/graph` | PageRank, Centrality, and Community Detection. |
| **Tokenization** | `pkoukk/tiktoken-go` | For AI efficiency markers. |
| **HTTP** | `go-chi/chi` | Stdlib-compatible, lightweight routing. |
| **MCP** | `mark3labs/mcp-go` | Standard MCP protocol support. |

---

## 4. The Elite Biomarker Matrix

All markers are executed concurrently via `errgroup` and Tree-sitter AST queries.

### 4.1. Concurrency & Runtime Risk (New)
- **Go**: Goroutines writing to outer-scope variables without synchronization.
- **Java**: Thread/Runnable mutating class fields without `synchronized` or `Atomic`.
- **Python**: Logical races across `await` boundaries in shared async state.
- **Node.js**: Asynchronous closures with read-before-await / update-after-await patterns.
- **Dart/Flutter**: State field updates immediately following an `await` yield point.

### 4.2. Indian Regulatory Compliance (New)
- **dpdp_pii_exposure**: Detects Aadhaar, PAN, UPI_ID leaks to logs or unencrypted storage.
- **untracked_consent_mutation**: State updates missing preceding consent validation checks.
- **rbi_logger_audit_gap**: Financial handlers lacking correlation IDs in error/retry paths.
- **data_sovereignty_leak**: Outbound routing to non-Indian regions for PII-tagged data.

### 4.3. AI-Agent Efficiency (New)
- **token_bloat**: High token density vs. functional symbol count (context window drain).
- **hallucination_bait**: Overlapping symbol signatures in the same module causing LLM confusion.
- **phantom_coupling**: High Git co-change rate ($\ge 30\%$) despite zero structural imports.
- **zombie_exports**: Exported symbols with zero incoming call edges (dead interface weight).

### 4.4. Structural & AppSec (Ported + Enhanced)
- **broken_crypto**: Use of MD5, SHA1, or weak DES instances.
- **tainted_sql**: SQL injection via string concatenation in DB queries.
- **ssrf_blind**: Unvalidated user input used in outbound HTTP client URLs.
- **bypassed_rbac**: Deep ledger actions reachable without identity context.
- **Original 11**: Brain Method, Bumpy Road, Cyclomatic Complexity, etc.

---

## 5. Phase-Wise Implementation Roadmap

### Phase 1: Foundation & Programmatic Core (Weeks 1-4)
- [x] **1.1 Logger & Config:** Port Argonaut logger; define `envconfig` structs.
- [x] **1.2 Persistence:** Setup configurable SQLite and `golang-migrate`.
- [x] **1.3 Programmatic API:** Define `argus.Instance` and resource lifecycle.
- [x] **1.4 Basic Ingestion:** Tree-sitter wrapper and initial Git walk.

### Phase 2: Advanced Analysis & Biomarkers (Weeks 5-9)
- [x] **2.1 Graph Engine:** assembly of `gonum` graph and PageRank metrics.
- [x] **2.2 Community Detection:** Pure Go Leiden implementation.
- [x] **2.3 Concurrency & AppSec Markers:** Multi-language AST matchers.
- [x] **2.4 Regulatory & Efficiency Markers:** PII trackers and Token density metrics.

### Phase 3: Providers, MCP & REST (Weeks 10-14)
- [x] **3.1 Tiered Providers:** Anthropic/Gemini drivers with cost-routing logic.
- [x] **3.2 MCP Server:** 20 tools via `mark3labs/mcp-go`.
- [x] **3.3 REST API:** `chi` routers for Dashboard support and Cognee Export.
- [x] **3.4 SSE & Jobs:** Async chat streaming and worker-pool job execution.

### Phase 4: CLI & Final Distribution (Weeks 15-18)
- [ ] **4.1 Cobra CLI:** Implement all 20 subcommands with decoupled pathing.
- [ ] **4.2 Frontend Integration:** Connect Next.js `web/` to the Go REST server.
- [ ] **4.3 Hardening:** Cross-repo analysis and single-binary CI/CD pipelines.

### Phase 5: Structural Quality Engine — 12 Original Biomarkers (Weeks 19-26)

Implements the **12 foundational structural biomarkers** that form the deterministic, zero-LLM scoring layer. Each file receives a 10.0 base score; these markers deduct points subject to category caps (see PHILOSOPHY.md). Engine lives in `pkg/analysis/scorer.go`.

#### [x] 5.1 Cyclomatic Complexity & Control-Flow (Structural Complexity, cap −3.5)
- **brain_method**: Composite flag — NLOC > 50 AND cyclomatic ≥ 15 AND nesting ≥ 4 AND PageRank centrality in top 10%. Computed from Tree-sitter AST traversal + graph engine output. Deduction: up to −1.5.
- **nested_complexity**: Walk AST `if/for/switch/select` nodes; flag functions with max nesting depth ≥ 4. Deduction: up to −1.0.
- **bumpy_road**: Detect ≥ 3 sequential `if` or `case` blocks at the same nesting level within a single function body. Deduction: up to −1.0.

#### [x] 5.2 Size & Signature Metrics (Size & API Complexity, cap −2.0)
- **complex_method**: Cyclomatic complexity (McCabe) ≥ 9. Count `if/else/for/case/catch/&&/||` nodes per function in AST. Deduction: up to −0.8.
- **large_method**: NLOC > language threshold (Go: 60, Java/Python: 80, TS/JS: 60). Excludes blank lines and comment lines using Tree-sitter comment node filtering. Deduction: up to −0.6.
- **primitive_obsession**: Function parameter list with ≥ 6 primitive-typed params (int, string, bool, float). Extracted from AST `parameter_list` nodes. Deduction: up to −0.6.

#### [x] 5.3 Duplication (cap −1.5)
- **dry_violation**: Rabin–Karp rolling hash over Tree-sitter token sequences (window: 6 tokens, stride: 3). Clone pairs with similarity ≥ 80% across different files are flagged. Active clones (both files modified in last 90 days via git log) receive 1.5× deduction weight. Deduction: up to −1.5.

#### 5.4 Test Coverage Intelligence (cap −2.0)
Coverage data loaded from standard artifact formats; if no coverage file is present, markers are skipped (not penalized).
- **untested_hotspot**: File with churn ≥ 10 commits AND PageRank centrality top 20% AND line coverage < 20%. Deduction: up to −1.5.
- **coverage_gap**: File with line coverage < 60% (business logic files; test files excluded). Deduction scaled by (60 − coverage) / 60 × 0.5. Max deduction: −0.5.

Coverage artifact loading priority: `lcov.info` → `coverage.xml` (Cobertura) → `clover.xml`. Path configurable via `ARGUS_COVERAGE_FILE`.

#### 5.5 Organizational Risk via Git Analytics (cap −1.0)
Computed from git log during ingestion; stored in `FileNode.AuthorCount` and `FileNode.PrimaryAuthorLastCommit`.
- **developer_congestion**: ≥ 5 distinct authors touching the file in the last 90 days. Deduction: −0.5.
- **knowledge_loss**: Primary author (highest commit count) has not committed in ≥ 180 days AND file has churn ≥ 5. Deduction: −0.5.

**Model extension required:** Add `AuthorCount int`, `PrimaryAuthorLastCommit time.Time`, `LineCoverage float64` to `models.FileNode`.

#### 5.6 Dead Code Completion (cap −1.0)
Extends the existing `zombie_exports` marker to cover internal (unexported) symbols.
- **dead_code / unreferenced_symbols**: Walk all symbols in the graph; flag any function, type, or variable with zero incoming edges regardless of export visibility. Replaces `zombie_exports` which only covers exported names. Deduction: per unreferenced symbol, capped at −1.0 total.

#### 5.7 Scorer Engine (`pkg/analysis/scorer.go`)
- `ComputeFileScore(file string, markers []models.Marker) models.FileScore` — group markers by category, apply caps, clamp to [1.0, 10.0].
- `ComputeRepoScore(scores []models.FileScore) float64` — weighted average (weight = file centrality from PageRank; fallback: uniform).
- REST endpoint: `GET /api/score/file?path=<file>`, `GET /api/score/repo`.
- MCP tool: `get_file_score(path)`, `get_repo_score()`.

---

## 7. Frontend Evolution (Next.js 15)

The existing React/Next.js frontend in `web/` will be updated to reflect the new deep analysis capabilities.

### 7.1. UI Structural Changes
- **Compliance Dashboard:** A new sidebar section for "Compliance" to display DPDP, RBI, and SEBI regulatory biomarkers.
- **AI-Efficiency Metrics:** Enhanced "Health" breakdowns showing token bloat and hallucination risk to help developers optimize for AI agents.
- **Unified Knowledge Graph:** A new "Cognee" tab or overlay in the Graph view to visualize code-context fusion (Slack, Docs, Decisions).
- **Programmatic Settings:** A revamped Settings UI to manage tiered LLM models (Premium vs. Cheap) and external storage paths.

### 7.2. Integration Layer
- **API Parity:** Update `lib/api/types.ts` to include the new 30+ health markers and tiered configuration structs.
- **Real-time Status:** Improve the `Jobs` UI to show sub-phase progress (e.g., "Scanning PII", "Tracing BFS Flows").
- **SSE Consolidation:** Use the standardized `chi` SSE stream for unified chat and job progress updates.

---

## 8. Structural Architecture for Biomarkers

Markers are implemented as a concurrent pipeline using `errgroup`:

```go
type HealthMarker interface {
    Analyze(ctx context.Context, file *models.FileNode, graph *gonum.Graph) (*models.Deduction, error)
}

// In your processing loop:
g, ctx := errgroup.WithContext(ctx)
for _, marker := range registeredMarkers {
    marker := marker // pin
    g.Go(func() error {
        deduction, err := marker.Analyze(ctx, currentFile, repoGraph)
        // Safely append deductions using a mutex
        return err
    })
}
```

---

## 9. Cognee Integration

Argus acts as a headless graph provider for the Cognee Python central layer.
- **Webhook:** POSTs to `COGNEE_WEBHOOK_URL` on re-index completion.
- **Export:** `GET /api/export/cognee` provides incremental graph diffs (Entities & Relations).
- **Fusion:** Cognee merges argus code-graph with Slack and product documentation.
