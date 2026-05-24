# Argus — Codebase Intelligence for AI Agents

Argus is a high-performance Go structural intelligence engine for AI coding agents. It parses source repositories into dependency graphs, detects architectural patterns, identifies health risks, and enforces compliance markers—delivering 27× fewer tokens per query and 36% lower LLM costs than baseline approaches.

Inspired from repowise, Argus achieves 5–15ms startup and a 15–25MB memory footprint as a single static binary. It serves as both a standalone CLI and an embedded programmatic engine for Go applications.

## What Argus Adds

| Capability | Languages & Detail |
|---|---|
| **Concurrency Risk Detection** | Go goroutines, Java threads, Kotlin coroutines, Dart/Flutter async, Python asyncio, Node.js closures — race conditions and await-boundary violations |
| **Indian Regulatory Compliance** | DPDP-Act enforcement: Aadhaar, PAN, UPI_ID, mobile, email detection with AST-level tracking |
| **AI-Agent Efficiency Markers** | Token bloat detection, hallucination-prone code patterns, zombie exports, phantom coupling, cyclomatic complexity hotspots |
| **AppSec Biomarkers** | SQL injection sinks, SSRF vectors, broken crypto, RBAC bypasses, hardcoded secrets |
| **Dual Server Architecture** | MCP (stdio, 20 tools for Claude) + REST (Gin, SSE streaming, Cognee webhook export) |
| **Tiered LLM Routing** | Gemini Flash for bulk analysis, Claude Sonnet for deep review — cost-aware provider selection |

## Quick Start

### Prerequisites

- Go 1.24+
- Git
- (Optional) Docker for Cognee integration

### Clone & Build

```bash
git clone git@github.com:venkatvghub/Argus.git
cd Argus/backend
go build -o argus ./cmd/argus
```

### Configure

Copy `.env.example` to `.env`:

```bash
cp .env.example .env
```

Edit `.env` and set your API keys and preferences. See **[Configuration](docs/configuration.md)** for the full list of `ARGUS_*` variables and defaults.

### Analyse a Repository

Trigger deep analysis on any local repo (non-blocking — returns a job ID):

```bash
./argus analyze /path/to/target/repo --data-dir ./data
# {"job_id":"abc123","message":"analysis started"}
```

Wait for completion and stream progress:

```bash
./argus analyze /path/to/target/repo --data-dir ./data --wait
```

### Query Results

Once analysis completes, use `--repo-id` (printed or from `repos list`) to query:

```bash
# List all indexed repositories
./argus repos list --data-dir ./data

# Aggregate health score for a repo
./argus score repo --repo-id <id> --data-dir ./data

# File-level health score
./argus score file --file pkg/server/rest.go --repo-id <id> --data-dir ./data

# All biomarker findings for a file
./argus markers file --file pkg/server/rest.go --repo-id <id> --data-dir ./data

# All markers across a repo
./argus markers repo --repo-id <id> --data-dir ./data

# Search symbols
./argus symbols search --query ComputeFileScore --type function --data-dir ./data
```

### Run (MCP Mode)

For use with Claude Code / Claude Desktop:

```bash
./argus serve mcp --data-dir ./data
```

Starts an MCP server on stdin/stdout exposing 20 tools for structured codebase queries.

### Run (REST Mode)

For web dashboard and Cognee integration:

```bash
./argus serve rest --addr :8080 --data-dir ./data
```

Starts a REST server on `:8080`. Score endpoints: `GET /api/score/file?path=<file>&repo_id=<id>`, `GET /api/score/repo?repo_id=<id>`.

### LLM Provider & Model Tiers

Select a provider and model via environment variables. Argus supports a **cheap** tier for bulk analysis and a **premium** tier for deep review.

| Provider | Tier | Model | Environment Variables |
|---|---|---|---|
| **OpenAI** | Cheap | `gpt-4o-mini` | `ARGUS_LLM_PROVIDER=openai`<br>`ARGUS_OPENAI_API_KEY=sk-…`<br>`ARGUS_OPENAI_MODEL=gpt-4o-mini` |
| **OpenAI** | Premium | `gpt-4o` | `ARGUS_LLM_PROVIDER=openai`<br>`ARGUS_OPENAI_API_KEY=sk-…`<br>`ARGUS_OPENAI_MODEL=gpt-4o` |
| **Anthropic** | Cheap | `claude-3-5-haiku-20241022` | `ARGUS_LLM_PROVIDER=anthropic`<br>`ARGUS_ANTHROPIC_API_KEY=sk-ant-…`<br>`ARGUS_ANTHROPIC_MODEL=claude-3-5-haiku-20241022` |
| **Anthropic** | Premium | `claude-sonnet-4-5` | `ARGUS_LLM_PROVIDER=anthropic`<br>`ARGUS_ANTHROPIC_API_KEY=sk-ant-…`<br>`ARGUS_ANTHROPIC_MODEL=claude-sonnet-4-5` |
| **Gemini** | Cheap | `gemini-2.0-flash` | `ARGUS_LLM_PROVIDER=gemini`<br>`ARGUS_GEMINI_API_KEY=AIza…`<br>`ARGUS_GEMINI_MODEL=gemini-2.0-flash` |
| **Gemini** | Premium | `gemini-1.5-pro` | `ARGUS_LLM_PROVIDER=gemini`<br>`ARGUS_GEMINI_API_KEY=AIza…`<br>`ARGUS_GEMINI_MODEL=gemini-1.5-pro` |
| **OpenRouter** | Any | any model slug | `ARGUS_LLM_PROVIDER=openai`<br>`ARGUS_OPENAI_API_KEY=sk-or-…`<br>`ARGUS_OPENAI_MODEL=<slug>` ¹ |

> ¹ OpenRouter exposes an OpenAI-compatible API. Set `ARGUS_OPENAI_BASE_URL=https://openrouter.ai/api/v1` alongside `ARGUS_OPENAI_API_KEY=sk-or-…` and any [supported model slug](https://openrouter.ai/models) as `ARGUS_OPENAI_MODEL`.

## Documentation

- **[Configuration](docs/configuration.md)** — `ARGUS_*` environment variables, defaults, and path resolution
- **[Philosophy & Why Go](docs/PHILOSOPHY.md)** — Problem statement, rewrite rationale, foundational mandates, and biomarker categories
- **[Architecture](docs/architecture.md)** — System diagram, package responsibilities, analysis pipeline, server modes, LLM tiers
- **[Implementation Plan](docs/plan.md)** — Phase-wise roadmap (Weeks 1–18), structural design, Cognee integration

## License

MIT
