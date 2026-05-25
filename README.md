# Argus — Codebase Intelligence for AI Agents

Go structural intelligence engine: parses repos into dependency graphs, detects architectural risks, enforces compliance markers. Serves as a standalone CLI and embedded library. 27× fewer tokens per query vs baseline.

## Dashboard (Frontend + Backend)

Start both services for the full web dashboard:

**Terminal 1 — backend**
```bash
cd Argus/backend

# Copy and edit env (set your LLM key; CORS already includes localhost:3000)
cp .env.example .env

go build -o argus ./cmd/argus

# Run on :7337 so the Next.js proxy works with zero frontend config
ARGUS_DATA_DIR=./data ./argus serve rest --addr :7337
```

**Terminal 2 — frontend**
```bash
cd Argus/web

# Install dependencies (first time only)
pnpm install

# Start dev server on http://localhost:3000
pnpm dev
```

Open **http://localhost:3000**.

The Next.js dev server rewrites `/api/*` → `http://localhost:7337/api/*`, so no CORS or proxy config is needed.

> **Custom backend port:** If you prefer `:8080`, set `ARGUS_API_URL=http://localhost:8080` in `web/packages/web/.env.local` and run `argus serve rest --addr :8080`.

---

## Quick Start (5 min)

**Prerequisites:** Go 1.24+, Git

```bash
git clone git@github.com:venkatvghub/Argus.git
cd Argus/backend
go build -o argus ./cmd/argus

# Set API key (pick any one provider)
export ARGUS_ANTHROPIC_API_KEY=sk-ant-...   # or OPENAI / GEMINI
export ARGUS_DATA_DIR=./data

# Analyze a repo and generate wiki docs
./argus init /path/to/target/repo
```

`init` analyzes the repo, scores it, and generates wiki documentation with your LLM. Use `--index-only` to skip the LLM step.

## Commands

Global flags available on every command: `--data-dir`, `--repo-id`, `--log-level`.

### Analysis

| Command | Flags | Description |
|---|---|---|
| `argus init <repo-path>` | `--index-only`, `--yes`, `--coverage 0.20`, `--concurrency 5`, `--provider`, `--cheap-model`, `--medium-model`, `--premium-model`, `--resume <job-id>` | Full analysis + wiki generation |
| `argus analyze <repo-path>` | `--wait` | Analyze only (no LLM); returns job ID |

### Repositories

| Command | Flags | Description |
|---|---|---|
| `argus repos list` | `--json` | List indexed repos (ID, name, path) |

### Health Scores

| Command | Flags | Description |
|---|---|---|
| `argus score repo` | `--repo-id` | Aggregate health score (1–10) |
| `argus score file` | `--repo-id`, `--file <path>` | Per-file health score with category breakdown |

### Biomarkers

| Command | Flags | Description |
|---|---|---|
| `argus markers repo` | `--repo-id`, `--type`, `--severity`, `--category`, `--format json\|table` | All biomarker findings for a repo |
| `argus markers file` | `--repo-id`, `--file <path>`, `--type`, `--severity`, `--category`, `--format json\|table` | Findings for a specific file |
| `argus markers summary` | `--repo-id`, `--type`, `--severity`, `--category`, `--format json\|table` | Counts by severity / type / category |
| `argus markers top-files` | `--repo-id`, `--top 20`, `--type`, `--severity`, `--category`, `--format json\|table` | Files ranked by total deduction |

### Symbols

| Command | Flags | Description |
|---|---|---|
| `argus symbols search` | `--query <name>`, `--type function\|class\|variable\|import` | Search symbols across all repos |
| `argus symbols list` | `--repo-id` | List all symbols for a repo |

### Communities (graph clusters)

| Command | Flags | Description |
|---|---|---|
| `argus community show` | `--repo-id`, `--community-id <n>` | Nodes in a community cluster |

### Wiki

| Command | Flags | Description |
|---|---|---|
| `argus wiki list` | `--repo-id` | List generated wiki pages |
| `argus wiki get <page-id>` | — | Get a wiki page by ID |
| `argus wiki export <repo-id> <output-dir>` | — | Export all pages as `<type>/<subject>.md` files |

### Jobs

| Command | Flags | Description |
|---|---|---|
| `argus jobs list` | `--repo-id` | List wiki generation jobs |
| `argus jobs get <job-id>` | — | Get job status and details |

### Servers

| Command | Flags | Description |
|---|---|---|
| `argus serve mcp` | — | Start MCP stdio server (20 tools for Claude Code / Desktop) |
| `argus serve rest` | `--addr :8080` | Start REST HTTP server (SSE, score endpoints) |

### Misc

| Command | Description |
|---|---|
| `argus version` | Print version |

## Configuration

All settings use `ARGUS_` env vars. Copy `backend/.env.example` to `backend/.env` and edit.

Key variables:

| Variable | Default | Purpose |
|---|---|---|
| `ARGUS_DATA_DIR` | `data` | Base directory for DB and state |
| `ARGUS_LLM_PROVIDER` | `openai` | Active LLM backend |
| `ARGUS_OPENAI_API_KEY` | — | OpenAI or OpenRouter key |
| `ARGUS_ANTHROPIC_API_KEY` | — | Anthropic key |
| `ARGUS_GEMINI_API_KEY` | — | Gemini key |
| `ARGUS_COVERAGE` | `0.20` | Fraction of repo to generate wiki pages (0.10–1.0) |
| `ARGUS_LLM_MAX_RETRIES` | `3` | Retry attempts per LLM call |

Full reference: [docs/configuration.md](docs/configuration.md)

LLM provider setup and model tiers: [docs/providers.md](docs/providers.md)

## Documentation

| Doc | Contents |
|---|---|
| [docs/configuration.md](docs/configuration.md) | All `ARGUS_*` env vars, defaults, path resolution |
| [docs/providers.md](docs/providers.md) | Provider setup, model tiers, OpenRouter |
| [docs/architecture.md](docs/architecture.md) | System diagram, package layout, pipeline, server modes |
| [docs/PHILOSOPHY.md](docs/PHILOSOPHY.md) | Problem statement, rewrite rationale, biomarker categories |
| [docs/plan.md](docs/plan.md) | Phase-wise roadmap |

## License

MIT
