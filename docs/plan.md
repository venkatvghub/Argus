# repowise Go Rewrite: Implementation Plan

---

## 1. Vision & Core Principles

repowise is being re-engineered from the ground up as a high-performance, purely static Go binary. It serves as both a standalone CLI tool and a programmatic engine for deep codebase intelligence.

### Foundational Mandates
- **Programmatic-First Architecture:** Core logic is decoupled from CLI/HTTP layers and exported in `pkg/`. It can be embedded in any Go application via `repowise.New(config)`.
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
│   ├── cmd/repowise/      # CLI entrypoint (cobra)
│   ├── pkg/               # EXPORTED: Core Library
│   │   ├── repowise/      # Public API: Instance management
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
- [x] **1.3 Programmatic API:** Define `repowise.Instance` and resource lifecycle.
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
- [ ] **3.4 SSE & Jobs:** Async chat streaming and worker-pool job execution.

### Phase 4: CLI & Final Distribution (Weeks 15-18)
- [ ] **4.1 Cobra CLI:** Implement all 20 subcommands with decoupled pathing.
- [ ] **4.2 Frontend Integration:** Connect Next.js `web/` to the Go REST server.
- [ ] **4.3 Hardening:** Cross-repo analysis and single-binary CI/CD pipelines.

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

repowise acts as a headless graph provider for the Cognee Python central layer.
- **Webhook:** POSTs to `COGNEE_WEBHOOK_URL` on re-index completion.
- **Export:** `GET /api/export/cognee` provides incremental graph diffs (Entities & Relations).
- **Fusion:** Cognee merges repowise code-graph with Slack and product documentation.
