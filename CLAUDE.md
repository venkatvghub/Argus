# CLAUDE.md

Argus backend: Go library for repo ingestion, graph analysis, compliance markers, REST + MCP. Human docs live in [README.md](README.md) and [docs/](docs/) — do not duplicate them here.

## Commands

From repo root:

```bash
cd backend && go test ./...
cd backend && go build ./...
cd backend && go vet ./...
cd backend && go test ./pkg/<pkg>/... -run TestName
```

## While coding

- **Tunables** (paths, secrets, models, CORS, pool sizes): `config.Load()` / pass `*config.Config` — never inline in production code. Env reference: [docs/configuration.md](docs/configuration.md).
- **Backend Go details** (constants, packages, IDs): see `.claude/rules/backend-go.md` when editing `backend/**/*.go`.
- **Architecture & product context**: [docs/architecture.md](docs/architecture.md) — read on demand, not from memory here.
- **Changes**: minimal diffs; match existing package patterns; run `go test` for touched packages.

## PostgreSQL / GORM conventions

- **No FK constraints at DB level** — app manages referential integrity. GORM configured with `DisableForeignKeyConstraintWhenMigrating: true`.
- **Schema**: GORM `AutoMigrate` on startup (`persistence.New`). Add new GORM model fields/tables for schema changes — no migration files.
- **Type rules for GORM model fields**: `time.Time` → `timestamptz`, `float64` → `double precision`, strings → `text`, auto-int PK → `bigserial`, text PK → `text;primaryKey`, JSON → `jsonb`.
- **Connection**: `ARGUS_DATABASE_URL` env var. Start DB: `docker compose up -d postgres`.
