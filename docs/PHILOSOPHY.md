# Philosophy

## The Problem

AI coding agents read files but lack structural context. They see source code in isolation, missing co-change patterns, dead code clusters, ownership, and regulatory violations. This forces agents to invent history, re-analyze the same logic across files, and hallucinate about architectural intent.

repowise solves this by extracting codebase semantics as a structured graph: which functions call which, which tests cover what, which subsystems form communities, which code is at risk. This intelligence is then served via Claude MCP tools, reducing token bloat and improving agent accuracy. The Python repowise delivers 27× fewer tokens per query and 36% cheaper LLM costs.

Argus takes this further by rewriting repowise in Go—gaining speed, portability, and new risk-detection capabilities for compliance, concurrency, and security.

## Why Rewrite in Go

| Metric | Python repowise | Argus (Go) | Win |
|---|---|---|---|
| CLI cold start | 300–800ms | 5–15ms | 50–100× faster |
| Idle memory | ~120MB | 15–25MB | 5–8× smaller |
| Distribution | pip + venv + overhead | Single static binary | Seamless cross-compile |
| Cross-compile | Complex (requires host interpreters) | `GOOS=linux GOARCH=arm64 go build` | Native, one command |
| Concurrency model | GIL-bound threads, multiprocessing overhead | Goroutines with true parallelism | Lock-free graph analysis |
| Embedding | Via subprocess / REST only | Direct `import "github.com/argus/pkg/repowise"` | Native Go integration |

Python's runtime startup, garbage collection pauses, and GIL contention make it unsuitable for both CLI tools (every invocation pays the startup tax) and embedded engines. Go's fast compilation, zero-overhead concurrency, and static binaries align perfectly with repowise's use case: quick, repeated codebase analysis in agents and automation.

## Foundational Mandates

1. **Programmatic-First Architecture:** Core logic is decoupled from CLI/HTTP layers and exported in `pkg/`. It can be embedded in any Go application via `repowise.New(config)`.

2. **Zero-CGo Portability:** Use `modernc.org/sqlite` and pure Go implementations (e.g., Leiden for community detection) to ensure seamless cross-compilation to single static binaries. No C dependencies, no build-time host setup.

3. **Decoupled Execution:** No requirement to run from within a target repo. Use `--repo-path` to analyze any directory. Analysis state lives in `--data-dir` and documentation in `--docs-dir`, separate from the target source tree.

4. **Externalized State:** All persistent data (SQLite graph, deduplicated docs, metadata) is configurable and kept outside the target source tree, enabling multi-repo analysis and archival.

5. **Tiered Model Strategy:** Explicitly support "cheap" (e.g., Gemini Flash) vs. "premium" (e.g., Claude Sonnet) model configurations for cost-optimized analysis. Router selects based on query complexity.

6. **Structured Forensics:** High-performance logging via `uber-go/zap` with context propagation and metric-tagged spans for observability.

## Biomarker Philosophy

Argus detects four categories of structural risk, each surfaced as a queryable marker in the graph:

### Concurrency & Runtime Risk

Go goroutines writing to outer-scope variables without synchronization; Java threads mutating class fields without `synchronized` or `Atomic`; Python await-boundary races in shared async state; Node.js asynchronous closures with read-before-await patterns; Dart/Flutter state field updates immediately following await yield points. These patterns lead to data races, deadlocks, and nondeterministic failures under load.

### Indian Regulatory Compliance (DPDP)

Aadhaar, PAN, UPI_ID, mobile, email detection at the AST level. Flags code that touches sensitive personal data without encryption, audit logging, or retention policies. Supports custom regex patterns for compliance frameworks beyond DPDP.

### AI-Agent Efficiency

Token bloat (code that outputs more than it does), hallucination bait (unused exports, shadowed names, phantom coupling), zombie functions (unreachable code), cyclomatic complexity hotspots. These markers help agents avoid exploring dead-end code and focus on high-signal logic.

### Structural & AppSec

SQL injection sinks, SSRF vectors, broken crypto (weak algorithms, hardcoded keys), RBAC bypasses, hardcoded secrets. Ported from the original repowise, enhanced with AST-level precision.

All markers execute concurrently via `errgroup` and Tree-sitter AST queries, delivering results in seconds for large codebases.
