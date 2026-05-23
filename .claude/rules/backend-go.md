---
alwaysApply: false
paths: backend/**/*.go
---

# Backend Go conventions

## No hardcoded values

| Kind | Where |
|---|---|
| Deploy/runtime tuning | `pkg/config.Config` + `ARGUS_*` → [docs/configuration.md](../../docs/configuration.md) |
| Internal algorithm defaults | `pkg/analysis/defaults.go` |
| Cross-package identifiers | `pkg/constants` (e.g. `RepoIDLength`, `APIVersion`) |
| Domain enums | `pkg/models` (job statuses, etc.) |
| Package-local constants | `constants.go` in `argus`, `persistence`, `providers` |

New tunable: add to `pkg/config/config.go`, wire callers, update `docs/configuration.md` and `backend/.env.example`. Tests may use inline fixture literals.

## Entry points (where to edit)

| Area | Package / type |
|---|---|
| Orchestration | `pkg/argus` — `Instance`, `JobManager` |
| Ingestion | `pkg/ingestion` — `GitWalker`, `TreeSitterParser`, `LanguageRegistry` |
| Analysis | `pkg/analysis` — `GraphEngine`, `MarkerEngine` |
| HTTP / SSE | `pkg/server` — `RESTServer`, `chatStreamHandler`, CORS allowlist |
| MCP | `pkg/server` — `MCPServer` (stdio) |
| LLM | `pkg/providers` — `Router`, provider implementations |
| Storage | `pkg/persistence` — SQLite + `migrations/` |

Pipeline: git walk → tree-sitter parse → graph + communities → markers → SQLite + in-memory maps on `Instance`.

## Invariants

- **Repo ID**: `sha256(absPath)[:constants.RepoIDLength]` — not a UUID; keys `Instance.engines` / `markers` and API `repo_id`.
- **Jobs**: use `JobManager.Submit`; bounded worker pool from config — no unbounded `go func()` for analysis.
- **SSE chat**: requires `repoID` query param; CORS from `ARGUS_CORS_ALLOWED_ORIGINS` only (never `*`).
- **Servers are library types** — wired by caller; no single blessed `main` in this repo.
