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

All markers run concurrently via `errgroup` using Tree-sitter AST queries:

| Category | Markers | Trigger | Example |
|---|---|---|---|
| **Concurrency Risk** | Go race detection, Java thread unsync, Python await race, Node closure, Dart post-await | Goroutine without channel/mutex, unsync field write, shared async state | Goroutine reading parent loop var |
| **Regulatory (DPDP)** | Aadhaar, PAN, UPI_ID, mobile, email | String literal or regex match | `"aadhaar_number"` field |
| **AI Efficiency** | Token bloat, export count, complexity, zombie code | LOC > threshold, unused export, cyclomatic > 15 | 500-line function with no tests |
| **AppSec** | SQL injection sink, SSRF, broken crypto, RBAC, hardcoded secret | Pattern match on call args, crypto constant, hardcoded string | `WHERE id=" + id`, `Math.random()` for token |

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
