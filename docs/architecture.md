# Architecture

## System Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Argus CLI / Embedded Library                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  repowise.Instance                                      │ │
│  │  ├─ Ingestion: Git walk, Tree-sitter parse, Registry  │ │
│  │  ├─ Analysis: Graph, Community, Flows, Health         │ │
│  │  ├─ Providers: LLM router (Anthropic, Gemini, etc.)   │ │
│  │  └─ Persistence: SQLite + auto-migration              │ │
│  └────────────────────────────────────────────────────────┘ │
└──────────────┬──────────────────────┬──────────────────────┘
               │                      │
        ┌──────▼────────┐      ┌──────▼───────────┐
        │ MCP Server    │      │ REST Server      │
        │ (stdio)       │      │ (gin, :8080)     │
        │ 20 tools      │      │ SSE, webhooks    │
        └───────────────┘      └──────────────────┘
               │                      │
        ┌──────▼──────┐         ┌─────▼──────────┐
        │ Claude Code │         │ Web Dashboard  │
        │ Desktop     │         │ Next.js :3000  │
        └─────────────┘         └────────────────┘
                                       │
                                ┌──────▼──────────┐
                                │ Cognee Service  │
                                │ (Neo4j, Qdrant) │
                                └─────────────────┘
```

## Package Map

| Package | Responsibility |
|---|---|
| `pkg/config` | Environment-based configuration (envconfig), LLM provider selection, compliance patterns |
| `pkg/logger` | Structured logging via zap, context propagation, metric tags |
| `pkg/repowise` | Public API: `Instance` lifecycle, job scheduling, resource management |
| `pkg/ingestion/parser` | Tree-sitter bindings, AST extraction, multi-language support (Go, Python, TypeScript, Java, Dart, Kotlin) |
| `pkg/ingestion/git` | Git log walks, commit history, author tracking, blame attribution |
| `pkg/ingestion/registry` | Symbol registry: functions, classes, types, their definitions and references |
| `pkg/analysis/graph` | Dependency graph (gonum), PageRank, reachability, call chains |
| `pkg/analysis/community` | Leiden algorithm, package clustering, cohesion metrics |
| `pkg/analysis/markers` | Biomarker pipeline (concurrency, compliance, efficiency, AppSec) |
| `pkg/server/mcp` | MCP server (mark3labs/mcp-go), 20 tools for Claude integration |
| `pkg/server/rest` | REST API (chi), dashboard export, Cognee webhook |
| `internal/providers` | LLM drivers: Anthropic, OpenAI, Gemini, tiered cost routing |
| `internal/models` | Domain entities: FileNode, Symbol, Deduction, HealthMarker |
| `cmd/repowise` | CLI entrypoint (cobra) |

## Analysis Pipeline

1. **Git Walk** — Extract commits, authors, timestamps, file change frequency
2. **Tree-Sitter Parse** — Build AST for each source file; extract symbols (functions, classes, types)
3. **Symbol Registry** — Deduplicate symbols, track definitions and references across files
4. **Dependency Graph** — Build call graph, import graph; compute PageRank centrality
5. **Community Detection** — Run Leiden clustering on graph; identify package and subsystem boundaries
6. **Execution Flows** — Trace entry points (main, handlers, tests) to leaf functions; compute criticality
7. **Dead Code Detection** — Find unreferenced functions, classes, and type definitions
8. **Biomarker Analysis** — Run concurrent marker pipeline (concurrency, compliance, efficiency, AppSec) via `errgroup`
9. **Persistence** — Serialize graph, deductions, metadata to SQLite; auto-migrate schema
10. **Export** — REST endpoints for dashboard, webhook delivery to Cognee, MCP tool responses

## Biomarker Matrix

Argus runs a **23-biomarker** pipeline concurrently via `errgroup`. Every file starts at 10.0; markers subtract points subject to per-category caps (see PHILOSOPHY.md). Final score clamped to [1.0, 10.0].

### Structural Quality Biomarkers (12 — ported from original repowise)

| Category | Cap | Markers | Trigger |
|---|---|---|---|
| **Structural Complexity** | −3.5 | brain_method, nested_complexity, bumpy_road | Cyclomatic ≥ 15 + centrality; nesting ≥ 4; sequential same-depth branches |
| **Size & API Complexity** | −2.0 | complex_method, large_method, primitive_obsession | Cyclomatic ≥ 9; NLOC > threshold; ≥ 6 primitives in signature |
| **Duplication** | −1.5 | dry_violation | Rabin–Karp rolling hash over Tree-sitter tokens; co-change weighted |
| **Test Coverage** | −2.0 | untested_hotspot, coverage_gap | LCOV/Cobertura: high-churn file + zero coverage; low coverage surface |
| **Organizational Risk** | −1.0 | developer_congestion, knowledge_loss | ≥ 5 active authors/file; primary author inactive in last 90 days |
| **Dead Code** | −1.0 | dead_code, unreferenced_symbols, zombie_exports | Zero incoming call edges (internal + exported) |

### Argus-Native Biomarkers (11 — compliance, AppSec, AI-agent efficiency)

| Category | Cap | Markers | Trigger |
|---|---|---|---|
| **Concurrency Risk** | uncapped | goroutine_shared_state, java_thread_unsync, py_await_race, js_closure_race, dart_state_after_await | Goroutine/thread mutating outer scope without sync |
| **DPDP Compliance** | uncapped | dpdp_aadhaar, dpdp_pan, dpdp_upi, dpdp_mobile, pii_email, untracked_consent, rbi_logger_gap, data_sovereignty | PII regex hit; missing consent check; non-Indian outbound routing |
| **AppSec** | uncapped | tainted_sql, ssrf_blind, broken_crypto, bypassed_rbac, hardcoded_secret | String-concat SQL; unvalidated URL; MD5/SHA1/DES; RBAC skip; literal secret |
| **AI-Agent Efficiency** | uncapped | token_bloat, hallucination_bait, phantom_coupling, zombie_exports | >50 tokens/line; duplicate symbol names; co-change without imports |

## Scoring Architecture

```
[]Marker (per file)
   → group by Category
   → sum Deduction per category
   → clamp each category sum to CategoryCaps[cat]
   → total = sum of clamped category deductions
   → FileScore.Final = max(1.0, 10.0 - total)
```

`models.FileScore` holds the result. `models.CategoryCaps` defines the per-category maximums. Score computation will live in `pkg/analysis/scorer.go` (Phase 5).

## Async Job Execution & SSE Chat Streaming

### Worker Pool (Phase 3.4)

The `JobManager` in `pkg/repowise/jobs.go` bounds analysis work to a fixed 3-goroutine pool with a buffered queue (32 items). The `Submit(jobID, fn)` method replaces raw `go func()` spawning in `Analyze()`, preventing unbounded goroutine proliferation during bulk repository processing. Each job is wrapped with panic recovery via `executeWork()`.

### SSE Chat Streaming (Phase 3.4)

The `chatStreamHandler` in `pkg/server/sse.go` exposes `GET /api/chat/stream?repoId=<id>&q=<query>` for token-level LLM streaming. It consumes token and error channels from `provider.ChatStream()`, flushing each token to the client as a Server-Sent Event. Client disconnection is detected via `r.Context().Done()`.

### Provider Wiring (Phase 3.4)

`RESTServer.SetProvider(p *providers.Router)` post-construction injects the active LLM router (Anthropic, Gemini, OpenAI) after the server is instantiated. This defers provider initialization until runtime and supports cost-based tier selection.

## Server Modes

### MCP Server (stdio)

Exposes 20 tools for Claude Code and Claude Desktop. Runs in foreground, listening on stdin/stdout. Tools include: graph queries (callers, callees, imports), flow traversal, health marker lookup, community analysis, dead code detection, compliance report.

```bash
argus mcp --repo-path /path/to/repo --data-dir ./data
```

### REST Server (Gin, :8080)

Serves dashboard, export endpoints, and Cognee webhooks. Supports SSE streaming for long-running queries. Healthcheck: `GET /health`.

```bash
argus server --port 8080 --repo-path /path/to/repo --data-dir ./data
```

Key endpoints:
- `GET /health` — Readiness check
- `GET /graph/summary` — Graph stats
- `POST /export/cognee` — Send graph snapshot to Cognee service (webhook)
- `POST /compliance/report` — DPDP compliance findings
- `POST /health/markers` — All biomarkers for a file or symbol

## LLM Provider Tiers

| Tier | Providers | Use Case | Example |
|---|---|---|---|
| **Cheap** | Gemini Flash, Gemini Flash 8B | Bulk analysis, token counting, pattern detection | "Summarize this function. Is it a test?" |
| **Premium** | Claude 3.5 Sonnet, GPT-4o | Deep reasoning, code review, architectural decisions | "Design a refactor for this class. Why?" |

Cost router selects based on query type. Environment var `REPOWISE_LLM_PROVIDER` selects default tier.

## Data Flow Diagram

### repowise init

```
User: argus init --repo-path /target

  1. Validate repo (git dir exists)
  2. Git walk → commit history, file change freq
  3. Tree-sitter parse all files → AST
  4. Extract symbols, calls, imports
  5. Build dependency graph (gonum)
  6. Run Leiden clustering
  7. Detect execution flows
  8. Run biomarker pipeline (errgroup)
  9. Persist to SQLite
 10. Return summary (files analyzed, symbols found, markers detected)
```

### MCP Tool: Graph query

```
Claude: "Who calls 'handlePayment'?"

  1. MCP stdin receives tool invocation
  2. Query registry for 'handlePayment' definition
  3. Look up all callers in dependency graph
  4. Return caller list with file, line, function
  5. Write to stdout (MCP JSON response)
```

### REST: Cognee export

```
POST /export/cognee

  1. Read graph summary from SQLite
  2. Convert FileNode, Symbol, DeductionNode to Cognee schema
  3. POST to Cognee service endpoint (webhook_url from env)
  4. Stream response via SSE if requested
```

## Technology Stack

| Layer | Technology | Why |
|---|---|---|
| Language | Go 1.21+ | Fast compile, zero-overhead concurrency, static binary |
| Parsing | go-tree-sitter, tree-sitter-go/python/typescript/java/dart/kotlin | AST extraction, symbol resolution |
| Graph | gonum/graph | Dependency graph, PageRank, traversal |
| Community Detection | Pure Go Leiden | Zero CGo, clustering without external libs |
| Persistence | modernc.org/sqlite | Zero CGo, portable, transactions |
| Logger | uber-go/zap | Structured logging, low overhead |
| CLI | cobra | Subcommands, flags, help generation |
| REST | chi | Lightweight router, middleware, streaming |
| Config | kelseyhightower/envconfig | Type-safe env var mapping |
| MCP | mark3labs/mcp-go | Protocol server, tool registration |
| Migrations | golang-migrate/migrate | Schema versioning, rollback |
| Testing | testify, go test | Native testing, no external deps |

## Deployment

### Local Development

```bash
cd backend
go run ./cmd/repowise init --repo-path /path/to/project --data-dir ./data
go run ./cmd/repowise server --port 8080 --data-dir ./data
```

### Docker

```dockerfile
FROM golang:1.21 as builder
WORKDIR /app
COPY backend .
RUN go build -o argus ./cmd/repowise

FROM gcr.io/distroless/base-debian12
COPY --from=builder /app/argus /
ENTRYPOINT ["/argus"]
```

### Kubernetes / Systemd

Deploy as sidecar or standalone service. SQLite data dir can be a persistent volume. MCP mode runs as a service for Claude Code integration.

Example systemd unit:

```ini
[Unit]
Description=Argus Codebase Intelligence
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/argus server --port 8080 --data-dir /var/lib/argus
WorkingDirectory=/var/lib/argus
Restart=on-failure
Environment="REPOWISE_OPENAI_API_KEY=..."

[Install]
WantedBy=multi-user.target
```
