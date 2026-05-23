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

### Run (MCP Mode)

For use with Claude Code / Claude Desktop:

```bash
./argus mcp --repo-path /path/to/target/repo --data-dir ./data
```

This starts an MCP server listening on stdin/stdout, exposing 20 tools for structured codebase queries.

### Run (REST Mode)

For web dashboard and Cognee integration:

```bash
./argus server --port 8080 --repo-path /path/to/target/repo --data-dir ./data
```

Starts a Gin REST server on `:8080`. Health check: `curl http://localhost:8080/health`

## Documentation

- **[Configuration](docs/configuration.md)** — `ARGUS_*` environment variables, defaults, and path resolution
- **[Philosophy & Why Go](docs/PHILOSOPHY.md)** — Problem statement, rewrite rationale, foundational mandates, and biomarker categories
- **[Architecture](docs/architecture.md)** — System diagram, package responsibilities, analysis pipeline, server modes, LLM tiers
- **[Implementation Plan](docs/plan.md)** — Phase-wise roadmap (Weeks 1–18), structural design, Cognee integration

## License

MIT
