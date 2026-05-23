# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
cd Argus/backend && go build ./...

# Run all tests
cd Argus/backend && go test ./...

# Run a single package's tests
cd Argus/backend && go test ./pkg/analysis/...

# Run a single test
cd Argus/backend && go test ./pkg/analysis/... -run TestGraphEngine

# Lint
cd Argus/backend && go vet ./...
```

## Configuration

All env vars are prefixed `REPOWISE_`. Loaded as a singleton via `config.Load()`.

| Variable | Default | Purpose |
|---|---|---|
| `REPOWISE_DB_PATH` | `argus.db` (under `REPOWISE_DATA_DIR`) | SQLite file name/path |
| `REPOWISE_DATA_DIR` | `data` | Base directory for DB and externalized state |
| `REPOWISE_DOCS_DIR` | `docs` | Generated documentation output root |
| `REPOWISE_LLM_PROVIDER` | `openai` | Active LLM (`openai`, `anthropic`, `gemini`) |
| `REPOWISE_OPENAI_API_KEY` | — | Required for OpenAI |
| `REPOWISE_ANTHROPIC_API_KEY` | — | Required for Anthropic |
| `REPOWISE_GEMINI_API_KEY` | — | Required for Gemini |
| `REPOWISE_PII_PATTERNS` | `AADHAAR,PAN,UPI_ID,MOBILE,EMAIL` | Enabled compliance PII scans (wired to `MarkerEngine`) |
| `REPOWISE_TOKEN_BLOAT_THRESHOLD` | `50` | Max tokens/line before `token_bloat` marker |
| `REPOWISE_WORKER_COUNT` | `3` | JobManager worker goroutines |
| `REPOWISE_WORK_QUEUE_SIZE` | `32` | JobManager queue buffer |
| `REPOWISE_JOB_LISTENER_BUFFER` | `10` | Per-subscriber SSE job update buffer |
| `REPOWISE_MOCK_STREAM_TOKEN_DELAY_MS` | `50` | Stub LLM stream delay per token (ms) |
| `REPOWISE_CORS_ALLOWED_ORIGINS` | — | Comma-separated browser origins for SSE (empty = no CORS headers) |
| `REPOWISE_OPENAI_MODEL` | `gpt-4o-mini` | OpenAI model name |
| `REPOWISE_ANTHROPIC_MODEL` | `claude-3-5-haiku-20241022` | Anthropic model name |
| `REPOWISE_GEMINI_MODEL` | `gemini-2.0-flash` | Gemini model name |

Do not hardcode values that belong in config. If a new setting is operator- or deploy-tunable (paths, URLs, ports, API keys, model names, CORS, feature flags), add it to `pkg/config/config.go` with a `REPOWISE_` env var, a sensible `default:` tag, and a row in this table.

## Coding conventions

### No hardcoded values

Production code must not scatter magic numbers, strings, paths, timeouts, or buffer sizes inline. Use the right layer:

| Kind of value | Where it belongs | Example |
|---|---|---|
| Deploy / runtime tuning | `pkg/config.Config` + `REPOWISE_*` env var | DB path, LLM provider, CORS origins |
| Internal tunables (same for all deployments) | Named `const` in owning package | `pkg/analysis/defaults.go` (PageRank, Leiden, phantom coupling), `pkg/constants` (API version, repo ID length) |
| Domain thresholds & enums | Named `const` in the owning package or `pkg/models` | Marker score caps, job status strings |
| Derived identifiers | Documented algorithm + shared constant | `sha256(absPath)[:constants.RepoIDLength]` for repo IDs |

Rules when writing or reviewing code:

1. **Never** inline secrets, file paths, hostnames, ports, model names, or provider IDs — read them from `config.Load()` (or pass `*config.Config` down from the caller).
2. **Replace magic numbers** with named constants and a short comment when the meaning is not obvious from the name alone (e.g. `const tokenBloatThreshold = 50` not `if tokens > 50`).
3. **Reuse existing constants** — job statuses live in `models.JobStatus*`, marker thresholds in `analysis/defaults.go`, cross-package IDs in `pkg/constants`, worker-pool sizing via `REPOWISE_WORKER_*` env vars.
4. **New tunables** — add to `config.go`, wire through `Instance` / servers / providers, and update the Configuration table above.
5. **Tests** may use inline literals for fixtures and timeouts; keep production paths config-driven.

If you find a hardcoded production value during a change, extract it in the same PR rather than copying the pattern.

## Architecture

**Argus** is a code intelligence backend. It ingests git repos, builds a structural graph, runs compliance/efficiency markers, and exposes results via REST and MCP.

### Data flow

```
git repo path
   → ingestion.GitWalker.Walk()       # reads HEAD tree, extracts FileNodes + git metrics (churn, ownership)
   → ingestion.TreeSitterParser       # parses source → []Symbol (functions, classes, vars, imports)
   → analysis.GraphEngine.BuildGraph() # gonum directed graph: file nodes + symbol nodes, "contains" edges
   → analysis.GraphEngine.DetectCommunities() # Louvain community detection
   → analysis.MarkerEngine.Run()      # compliance + efficiency markers on files and graph
   → persistence.DB.UpsertRepository() # SQLite
   → repowise.Instance (in-memory engines/markers maps)
```

Analysis runs as a background goroutine; progress is tracked by `repowise.JobManager` and streamed to clients via SSE (`GET /api/events?jobId=<id>`).

### Package responsibilities

| Package | Role |
|---|---|
| `pkg/repowise` | `Instance` — top-level orchestrator; `JobManager` — async job lifecycle with pub/sub |
| `pkg/ingestion` | `GitWalker` — git traversal + churn/ownership metrics; `TreeSitterParser` — AST extraction; `LanguageRegistry` — maps extensions to Tree-sitter grammars |
| `pkg/analysis` | `GraphEngine` — gonum directed graph + PageRank; `MarkerEngine` — compliance/efficiency findings |
| `pkg/server` | `RESTServer` (chi router) + SSE handler; `MCPServer` (stdio, via `mcp-go`) |
| `pkg/providers` | `Provider` interface; `Router` selects active LLM from config |
| `pkg/persistence` | SQLite via `modernc/sqlite`; embedded SQL migrations in `migrations/` |
| `pkg/models` | Shared domain types: `Repository`, `FileNode`, `Symbol`, `Marker`, `Job` |
| `pkg/config` | Singleton config loaded from env |
| `pkg/logger` | Structured logger wrapper |

### Supported languages (Tree-sitter)

Go, JavaScript/MJS/CJS, TypeScript/TSX, Java, Python.

### Marker categories

- **DPDP compliance**: Aadhaar regex, PAN regex, UPI ID regex, untracked consent mutations, RBI logging gaps, data sovereignty (PII in non-Indian cloud regions)
- **AI-agent efficiency**: token bloat (>`REPOWISE_TOKEN_BLOAT_THRESHOLD` tokens/line via tiktoken `cl100k_base`), hallucination bait (duplicate symbol names), zombie exports (symbols with zero incoming call edges), phantom coupling

### Server modes

- **REST**: `RESTServer.Routes()` — mount chi router, serve `GET/POST /api/*`
- **MCP**: `MCPServer.Run()` — `server.ServeStdio()`, exposes tools: `list_repos`, `index_repo`, `search_symbols`, `get_file_markers`, `get_community_graph`

There is no `main.go` in this repo; both server types are library-style and must be wired by a caller.

### Repository ID

Derived as `sha256(absPath)[:constants.RepoIDLength]` (12 hex chars) — not a UUID. Used as the key in `Instance.engines` and `Instance.markers` maps and as the `repo_id` in API calls.
