# Philosophy

## The Problem

AI coding agents read files but lack structural context. They see source code in isolation, missing co-change patterns, dead code clusters, ownership, and regulatory violations. This forces agents to invent history, re-analyze the same logic across files, and hallucinate about architectural intent.

Argus solves this by extracting codebase semantics as a structured graph: which functions call which, which tests cover what, which subsystems form communities, which code is at risk. This intelligence is then served via Claude MCP tools, reducing token bloat and improving agent accuracy.

Inspired from repowise, Argus gains speed, portability, and new risk-detection capabilities for compliance, concurrency, and security.

## Why Rewrite in Go

| Metric | Earlier Python implementation | Argus (Go) | Win |
|---|---|---|---|
| CLI cold start | 300–800ms | 5–15ms | 50–100× faster |
| Idle memory | ~120MB | 15–25MB | 5–8× smaller |
| Distribution | pip + venv + overhead | Single static binary | Seamless cross-compile |
| Cross-compile | Complex (requires host interpreters) | `GOOS=linux GOARCH=arm64 go build` | Native, one command |
| Concurrency model | GIL-bound threads, multiprocessing overhead | Goroutines with true parallelism | Lock-free graph analysis |
| Embedding | Via subprocess / REST only | Direct `import "github.com/venkatvghub/argus/pkg/argus"` | Native Go integration |

Python's runtime startup, garbage collection pauses, and GIL contention make it unsuitable for both CLI tools (every invocation pays the startup tax) and embedded engines. Go's fast compilation, zero-overhead concurrency, and static binaries align perfectly with Argus's use case: quick, repeated codebase analysis in agents and automation.

## Foundational Mandates

1. **Programmatic-First Architecture:** Core logic is decoupled from CLI/HTTP layers and exported in `pkg/`. It can be embedded in any Go application via `argus.New(config)`.

2. **Zero-CGo Portability:** Use `modernc.org/sqlite` and pure Go implementations (e.g., Leiden for community detection) to ensure seamless cross-compilation to single static binaries. No C dependencies, no build-time host setup.

3. **Decoupled Execution:** No requirement to run from within a target repo. Use `--repo-path` to analyze any directory. Analysis state lives in `--data-dir` and documentation in `--docs-dir`, separate from the target source tree.

4. **Externalized State:** All persistent data (SQLite graph, deduplicated docs, metadata) is configurable and kept outside the target source tree, enabling multi-repo analysis and archival.

5. **Tiered Model Strategy:** Explicitly support "cheap" (e.g., Gemini Flash) vs. "premium" (e.g., Claude Sonnet) model configurations for cost-optimized analysis. Router selects based on query complexity.

6. **Structured Forensics:** High-performance logging via `uber-go/zap` with context propagation and metric-tagged spans for observability.

## The 10.0 Scoring System

Every file in Argus starts with a perfect **10.0** base score. The engine subtracts points as biomarkers fire, subject to per-category caps that prevent any single concern from dominating the final rating. The result is always clamped to **[1.0, 10.0]**.

| Category | Markers | Max Deduction |
|---|---|---|
| Structural Complexity | brain_method, nested_complexity, bumpy_road | −3.5 |
| Size & API Complexity | complex_method, large_method, primitive_obsession | −2.0 |
| Duplication | dry_violation | −1.5 |
| Test Coverage | untested_hotspot, coverage_gap | −2.0 |
| Organizational Risk | developer_congestion, knowledge_loss | −1.0 |
| Dead Code | dead_code, unreferenced_symbols, zombie_exports | −1.0 |
| Compliance & AppSec | DPDP, RBI, concurrency, SQL injection, SSRF, crypto, RBAC | uncapped |
| AI-Agent Efficiency | token_bloat, hallucination_bait, phantom_coupling | uncapped |

The six structural categories above are the **12 foundational structural biomarkers**, ported and enhanced with Go AST precision. The Compliance and Efficiency categories are **11 Argus-native biomarkers** — together forming a **23-biomarker compilation engine**.

## Biomarker Philosophy

### Group 1: Structural Complexity (Cap −3.5)

These target deep control-flow architecture where code becomes fragile and unmaintainable.

- **brain_method**: A function that is simultaneously long, deeply nested, cyclomatic-complex, and a high-centrality vertex in the call graph. The single strongest predictor of legacy fragility.
- **nested_complexity**: Functions with control-flow nesting ≥ 4 levels deep (if → for → switch → if). Exponential test-case surface, zero readability.
- **bumpy_road**: Multiple conditional branches stacked sequentially at the same nesting depth. A function doing several unrelated jobs that should be split into flat, isolated handlers.

### Group 2: Size & API Complexity (Cap −2.0)

- **complex_method**: Cyclomatic Complexity ≥ 9 (McCabe). Every new branch is an untested execution lane.
- **large_method**: Non-Comment Lines of Code exceed language-specific thresholds. Length alone is a friction signal.
- **primitive_obsession**: Method signatures with ≥ 6 primitives in a row instead of a structured object. API erosion in progress.

### Group 3: Duplication (Cap −1.5)

- **dry_violation**: Cross-file code clones detected via Rabin–Karp rolling hash over Tree-sitter tokens. Rename-resistant. Active clones are weighted higher using Git co-change frequency.

### Group 4: Test Coverage Intelligence (Cap −2.0)

Parsed from LCOV, Cobertura, or Clover coverage artifacts and linked to architectural dependency depth.

- **untested_hotspot**: High-churn, high-centrality file with zero or critically low test coverage. The highest-risk refactoring target in the codebase.
- **coverage_gap**: Business-logic files missing meaningful test vectors, graded by uncovered surface depth.

### Group 5: Organizational Risk (Cap −1.0)

These connect structural patterns to human development bottlenecks via Git history.

- **developer_congestion**: Too many authors modifying the same file across overlapping commits — a monolithic structure disguised as a collaboration problem.
- **knowledge_loss**: Primary authors who built the file's core logic are no longer active contributors. Refactor or document before institutional context is permanently lost.

### Group 6: Dead Code (Cap −1.0)

- **dead_code / unreferenced_symbols**: Internal functions, exported interfaces, structs, and methods with zero incoming call-graph edges. Compiler bloat and maintenance overhead with no runtime value. (Exported-symbol variant: `zombie_exports`.)

### Group 7: Concurrency & Runtime Risk (Argus-native, uncapped)

Go goroutines writing to outer-scope variables without synchronization; Java threads mutating class fields without `synchronized` or `Atomic`; Python await-boundary races in shared async state; Node.js asynchronous closures with read-before-await patterns; Dart/Flutter state field updates immediately following await yield points. These patterns lead to data races, deadlocks, and nondeterministic failures under load.

### Group 8: Indian Regulatory Compliance — DPDP (Argus-native, uncapped)

Aadhaar, PAN, UPI_ID, mobile, email detection at the AST level. Flags code that touches sensitive personal data without encryption, audit logging, or retention policies. RBI logger audit gaps (financial handlers lacking correlation IDs). Data sovereignty leaks (PII routed to non-Indian regions). Supports custom regex patterns for SEBI and other compliance frameworks.

### Group 9: AI-Agent Efficiency (Argus-native, uncapped)

Token bloat (high token density vs. functional symbol count — context window drain), hallucination bait (overlapping symbol signatures causing LLM confusion), phantom coupling (high Git co-change despite zero structural imports), zombie exports (exported symbols with zero incoming call edges).

### Group 10: AppSec (Argus-native, uncapped)

SQL injection sinks, SSRF vectors, broken crypto (MD5, SHA1, weak DES), RBAC bypasses, hardcoded secrets. Detected at AST level via Tree-sitter queries across Go, Java, Python, TypeScript, Kotlin, and Terraform.

All 23 markers execute concurrently via `errgroup` and Tree-sitter AST queries, delivering results in seconds for large codebases.
