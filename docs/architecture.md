# repowise Platform — Architecture

---

## Overview

repowise is a codebase intelligence platform. It parses source repositories
into a structured dependency graph, runs analysis (community detection,
execution flows, dead code, health biomarkers), and exposes results via MCP
tools and a REST API consumed by a Next.js dashboard.

The target architecture separates concerns across three layers:

1. **repowise Go** — fast static analysis binary per repo (or shared across repos)
2. **Cognee** — unified Python knowledge graph fusing code, Slack, and product docs
3. **Agent MCP Servers** — persona-specific Claude agents (PM, Engineer, EM)

---

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│  SOURCE DATA                                                             │
│                                                                         │
│  Git Repos ──────────────────────────────────────────────────────────┐  │
│  Slack channels ─────────────────────────────────────────────────┐   │  │
│  Confluence / Notion / Linear / ADRs ────────────────────────┐   │   │  │
└──────────────────────────────────────────────────────────────┼───┼───┼──┘
                                                               │   │   │
                                           ┌───────────────────┘   │   │
                                           ▼                       │   │
┌──────────────────────────────────────────────────────────────────┼───┼──┐
│  repowise Go Binary  (:8080)                                      │   │  │
│                                                                   │   │  │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────────┐    │   │  │
│  │  Ingestion  │  │   Analysis   │  │    Server            │    │   │  │
│  │             │  │              │  │                      │    │   │  │
│  │ tree-sitter │  │ community    │  │ MCP (stdio)          │    │   │  │
│  │ graph build │  │ flows        │  │ REST API (chi)       │    │   │  │
│  │ git index   │  │ dead code    │  │ /api/export/cognee ──┼────┘   │  │
│  │ pipeline    │  │ health       │  │ SSE chat             │        │  │
│  │             │  │ blast radius │  │ jobs + scheduler     │        │  │
│  └─────────────┘  └──────────────┘  └──────────────────────┘        │  │
│                                                                       │  │
│  Persistence: modernc.org/sqlite · FTS5 · InMemory vector             │  │
└───────────────────────────────────────────────────────────────────────┼──┘
                                                                        │
                              ┌─────────────────────────────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Cognee Central Layer  (:8090)  — Python                                │
│                                                                         │
│  ┌──────────────────────────────────────────────────┐                  │
│  │  Ingestion Pipelines                             │                  │
│  │  RepoWiseCogneeIngester  ← /api/export/cognee    │                  │
│  │  SlackIngester           ← Slack API             │◄─── Slack        │
│  │  ProductNotesIngester    ← Confluence/Notion     │◄─── Prod notes   │
│  └──────────────────────────┬───────────────────────┘                  │
│                             ▼                                           │
│  ┌──────────────────────────────────────────────────┐                  │
│  │  Knowledge Graph (Neo4j / Kuzu)                  │                  │
│  │                                                  │                  │
│  │  CodeFile ──IMPORTS──► CodeFile                  │                  │
│  │  Symbol   ──CALLS───► Symbol                     │                  │
│  │  Symbol   ──MENTIONED_IN──► SlackMsg             │                  │
│  │  Feature  ──IMPLEMENTED_BY──► CodeFile           │                  │
│  │  Decision ──AFFECTS──► Symbol                    │                  │
│  │  Engineer ──OWNS──► CodeFile                     │                  │
│  │  Feature  ──DISCUSSED_IN──► SlackMsg             │                  │
│  └──────────────────────────┬───────────────────────┘                  │
│                             │  cognee.search() — semantic + graph      │
│  Qdrant (embeddings) ───────┘                                           │
└─────────────────────────────┬───────────────────────────────────────────┘
                              │
            ┌─────────────────┼─────────────────┐
            ▼                 ▼                 ▼
┌───────────────┐   ┌──────────────────┐   ┌──────────────┐
│  PM Agent     │   │  Engineer Agent  │   │  EM Agent    │
│  MCP :8101    │   │  MCP :8102       │   │  MCP :8103   │
│               │   │                  │   │              │
│ feature→code  │   │ explain symbol   │   │ knowledge    │
│ blast radius  │   │ why exists       │   │ risk report  │
│ ownership     │   │ safe to delete   │   │ tech debt    │
│ slack context │   │ callers          │   │ cross-repo   │
│ roadmap link  │   │ health           │   │ audit trail  │
└───────┬───────┘   └────────┬─────────┘   └──────┬───────┘
        └───────────────────┬┘                     │
                            ▼                      │
              ┌─────────────────────────┐           │
              │  Claude Code / Desktop  │◄──────────┘
              │  Slack bot              │
              │  Web UI (Next.js :3000) │
              └─────────────────────────┘
```

---

## Data Flow: repowise init

```
1. SCAN        detect repos in workspace; user selects
2. PARSE       tree-sitter per file → symbols + imports + calls (12 languages)
3. GRAPH       assemble gonum DiGraph
               nodes: CodeFile | Symbol | ExternalImport | Community
               edges: imports | defines | has_method | calls | type_use
4. METRICS     parallel (goroutine fan-out):
               PageRank · betweenness centrality · symbol PageRank
5. COMMUNITY   pure Go Leiden → gonum Louvain → directory fallback
               file-level clustering by import topology
               symbol-level clustering by call topology
6. FLOWS       BFS from entry points (main/handler/route scoring heuristic)
               traces call paths depth≤8; classifies intra vs cross-community
7. ANALYSIS    dead code reachability · health biomarkers · git co-change edges
8. GEN         optional: LLM doc page generation (text/template + provider)
9. EXPORT      POST webhook to COGNEE_WEBHOOK_URL with repo_id + stats
10. HOOK       offer post-commit hook for incremental re-indexing
```

---

## Data Flow: Cognee ingestion

```
repowise webhook arrives
        │
        ▼
RepoWiseCogneeIngester.on_webhook(event)
        │
        ├── GET /api/export/cognee?repo_id=X&since=<last_ts>
        │        returns: {entities[], relations[]}
        │
        ├── cognee.add(entities, dataset_name="code:{repo_id}")
        │        → chunked, embedded, stored in Qdrant
        │
        └── graph.add_edge(from, to, type, props)
                 → stored in Neo4j / Kuzu
                 → cross-repo edges resolved post-ingest

SlackIngester (scheduled, every N minutes)
        ├── pull new messages from configured channels
        ├── cognee.add(text, dataset_name="slack:{channel}")
        └── NER pass: detect symbol names → MENTIONED_IN edges

ProductNotesIngester (on-demand or scheduled)
        ├── pull pages from Confluence / Notion / Linear
        ├── cognee.add(content, dataset_name="notes:{source}")
        └── LLM extraction: Feature → IMPLEMENTED_BY → CodeFile edges
```

---

## Data Flow: Persona agent query

```
User: "What code implements the payments checkout feature?"
        │
        ▼
PM Agent MCP (:8101)
  tool: feature_to_code("payments checkout")
        │
        ├── cognee.search("payments checkout", node_types=["Feature"], graph_hops=2)
        │        → traverse IMPLEMENTED_BY → CodeFile nodes
        │
        ├── GET repowise /api/code-health?files=[...]
        │        → health scores, biomarkers
        │
        └── return: {files, symbols, health, owners, slack_mentions}
```

---

## Internal Go Package Responsibilities

### `internal/ingestion/parser`
Wraps `smacker/go-tree-sitter`. Loads grammar per language. Executes `.scm`
query files to extract: function/class/method declarations, import statements,
call expressions. Returns `ParsedFile` structs consumed by graph builder.

Supported languages: Python · TypeScript · JavaScript · Go · Rust · Java · C++ ·
Kotlin · Ruby · C# · Swift · Scala · PHP

### `internal/ingestion/graph`
Builds a `gonum/graph/multi.DirectedGraph`. Two tiers of nodes:
- File nodes: one per source file + one per unresolvable external import
- Symbol nodes: keyed `file_path::symbol_name`

Edge types: `imports | defines | has_method | calls | type_use | co_changes | dynamic_*`

Post-build metrics (parallel via goroutines):
- PageRank (file + symbol)
- Betweenness centrality (file + symbol)
- Strongly connected components (circular dep detection)

### `internal/ingestion/resolvers`
Per-language import resolution: converts raw import strings to canonical file
paths within the repo. Handles relative imports, module aliases, Go module
paths, TypeScript path aliases, Python package __init__ conventions.

### `internal/ingestion/git`
Uses `go-git/go-git`. Walks commit history up to configured limit. Builds
co-change frequency map: files that appear in the same commit → `co_changes`
edge weighted by frequency. Also extracts: commit authors per file (ownership),
blame hunks for knowledge risk scoring.

### `internal/analysis/community`
Three-tier fallback:
1. Pure Go Leiden (Traag et al. 2019) — guarantees connected partitions via
   refinement phase `isWellConnected()` check
2. `gonum/graph/community.Modularize` (Louvain) — if Leiden diverges
3. Directory grouping — final fallback for disconnected graphs

Input: undirected projection of dep graph (directed edges made symmetric).
Output: `map[nodeID]communityID` for both file-level and symbol-level graphs.

### `internal/analysis/flows`
BFS from scored entry points. Entry point scoring: composite of function name
patterns (main/handler/route/controller), PageRank, fan-out. Traces call paths
depth ≤ 8. Classifies each flow as intra-community or cross-community. Dedupes
overlapping paths.

### `internal/analysis/deadcode`
Graph reachability from all entry points. Symbols unreachable after full BFS
are dead code candidates. Filtered by: dynamic dispatch markers, exported
symbols, reflection-called patterns. Returns scored candidates.

### `internal/analysis/health`
11 biomarkers ported from Python:
- `brain_method` — high complexity + many callers
- `bumpy_road` — many nested conditionals
- `complex_method` — cyclomatic complexity threshold
- `coverage_gap` — low test coverage on high-churn file
- `developer_congestion` — too many authors, high churn
- `dry_violation` — duplicated code blocks (Rabin-Karp)
- `knowledge_loss` — single-owner high-complexity file
- `large_method` — LOC threshold
- `nested_complexity` — deep nesting depth
- `primitive_obsession` — overuse of primitives vs types
- `untested_hotspot` — high-churn file with no tests

Coverage parsers: Clover XML, Cobertura XML, LCOV. Duplication: Rabin-Karp
rolling hash tokenizer.

### `internal/server/export`
New package added for Cognee integration (Phase 4).

`GET /api/export/cognee?repo_id=X&since=<unix_ts>`
Returns full or incremental entity+relation graph in Cognee-compatible JSON.
Incremental mode: only nodes/edges modified after `since` timestamp.

Fires webhook `POST COGNEE_WEBHOOK_URL` with `ReindexEvent` after each
successful pipeline run.

### `internal/mcp/tools`
20 MCP tools via `mark3labs/mcp-go`. Transport: stdio (Claude Code/Desktop).
Each tool queries SQLite + in-memory graph → returns JSON. Same tool contract
as Python implementation.

Tools: `health · why · callers · community · search · risk · context ·
dependency · overview · dead_code · metrics · answer · symbol · diagram ·
annotate · flows · decision_records`

---

## Cognee Node + Edge Schema

```
Node types
──────────
CodeFile    path, repo_id, language, health_score, pagerank,
            betweenness, community_id, is_entry_point
Symbol      id (repo:file::name), name, kind (fn|class|method),
            file, repo_id, betweenness, pagerank, is_dead, is_entry_point
Community   id, repo_id, label, cohesion_score, size, algorithm
Feature     id, title, source, status, url
Decision    id, text, date, source, url
SlackMsg    id, channel_id, text, author_id, ts, permalink
Engineer    id, name, email, slack_id, github_login

Edge types
──────────
IMPORTS          CodeFile → CodeFile         weight (import count)
CALLS            Symbol → Symbol             confidence
CO_CHANGES       CodeFile → CodeFile         count, last_seen
DEFINES          CodeFile → Symbol
HAS_METHOD       Symbol → Symbol             (class → method)
MENTIONED_IN     Symbol → SlackMsg           confidence (NER score)
IMPLEMENTED_BY   Feature → CodeFile          confidence (LLM)
AFFECTS          Decision → Symbol           confidence (LLM)
OWNS             Engineer → CodeFile         commit_count
WROTE            Engineer → SlackMsg
DISCUSSED_IN     Feature → SlackMsg          confidence
```

---

## Persona Agent Tools Reference

### PM Agent (:8101)

| Tool | Description |
|---|---|
| `feature_to_code(feature)` | Cognee Feature→CodeFile traversal + health enrichment |
| `blast_radius_of_feature(feature)` | repowise /api/blast-radius on feature's code files |
| `who_owns_feature(feature)` | Feature→CodeFile→Engineer ownership chain |
| `slack_context_for_feature(feature)` | Feature→SlackMsg via DISCUSSED_IN edges |
| `roadmap_to_code(milestone)` | Linear milestone → linked Features → code |

### Engineer Agent (:8102)

| Tool | Description |
|---|---|
| `explain_symbol(symbol_id)` | code context + callers + Slack mentions combined |
| `why_does_this_exist(symbol_id)` | Decision+Feature nodes linked to symbol (graph_hops=3) |
| `safe_to_delete(symbol_id)` | dead code score + callers + recent Slack activity |
| `callers_of(symbol_id)` | repowise /api/symbols/{id}/callers |
| `health_of(module)` | repowise /api/code-health for module community |

### EM Agent (:8103)

| Tool | Description |
|---|---|
| `knowledge_risk_report()` | single-owner files + Slack expert identification |
| `tech_debt_map()` | health biomarkers across all repos |
| `cross_repo_impact(repo, change)` | CO_CHANGES edges across repo boundaries |
| `decision_audit_trail(feature)` | all ADRs, Slack decisions, PRDs linked to code |
| `team_velocity(since)` | git commit frequency + PR merge rate per engineer |

---

## Technology Stack Summary

### repowise Go binary
| Layer | Library |
|---|---|
| CLI | cobra |
| HTTP | chi |
| AST | smacker/go-tree-sitter |
| Graph | gonum/graph |
| Community | pure Go Leiden + gonum Modularize |
| Git | go-git/go-git |
| SQLite | modernc.org/sqlite (pure Go, zero CGo) |
| Migrations | golang-migrate |
| Vector | InMemory (default) / Qdrant Go client (opt-in) |
| MCP | mark3labs/mcp-go |
| LLM | anthropics/anthropic-sdk-go · sashabaranov/go-openai · google.golang.org/genai |
| TUI | charmbracelet/bubbletea + lipgloss |
| Scheduler | robfig/cron |
| Logging | log/slog |
| Config | gopkg.in/yaml.v3 |

### Cognee layer (Python)
| Layer | Library |
|---|---|
| Knowledge graph | cognee + Neo4j / Kuzu |
| Embeddings | Qdrant |
| HTTP client | httpx |
| MCP servers | FastMCP (mcp SDK) |
| Validation | pydantic |
| Slack | slack-sdk |

### Web UI (unchanged)
| Layer | Library |
|---|---|
| Framework | Next.js 15 + React 19 |
| Graph viz | @sigma/edge-curve · @xyflow/react |
| Charts | recharts |
| Components | Radix UI + Tailwind CSS 4 |
| Data fetch | SWR |
| Code highlight | shiki |

---

## What Does NOT Change

- `packages/web` — Next.js 15 dashboard, connects to Go REST :8080
- `packages/ui` — shared React component library
- `packages/types` — TypeScript API contract types (shared between UI and Go API)
- LiteLLM sidecar — proxy for DeepSeek / OpenRouter / exotic providers

---

## Deployment

```
┌────────────────────────┐
│  repowise-go  :8080    │  ← single static binary (GOOS cross-compiled)
│  SQLite file on disk   │
└────────────┬───────────┘
             │ webhook + REST export
┌────────────▼───────────┐
│  cognee-service :8090  │  ← Python, docker-compose or k8s pod
│  Neo4j / Kuzu          │
│  Qdrant                │
└────────────┬───────────┘
       ┌─────┼─────┐
       ▼     ▼     ▼
    :8101  :8102  :8103     ← persona MCP servers (Python, same docker-compose)
       └─────┼─────┘
             ▼
    Claude Code / Desktop   ← connects via stdio MCP config
    Web UI :3000            ← Next.js, talks to :8080
```

Single-machine dev setup: `docker-compose up` starts cognee + qdrant + neo4j +
3 MCP servers. repowise Go binary runs natively (or in same compose). Web UI
runs via `npm run dev`.

Production: repowise Go binary as systemd service or sidecar container.
Cognee + graph DB + vector DB as separate services with persistent volumes.
