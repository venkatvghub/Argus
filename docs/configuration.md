# Configuration

All environment variables use the `REPOWISE_` prefix. They are loaded as a singleton via `config.Load()` in `backend/pkg/config/config.go`.

Copy `backend/.env.example` to `backend/.env` and adjust values for local development.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `REPOWISE_APP_NAME` | `argus` | Service identifier (MCP server name) |
| `REPOWISE_ENV` | `development` | Runtime environment label |
| `REPOWISE_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `REPOWISE_DB_PATH` | `argus.db` (under `REPOWISE_DATA_DIR`) | SQLite file name or path |
| `REPOWISE_DATA_DIR` | `data` | Base directory for DB and externalized state |
| `REPOWISE_DOCS_DIR` | `docs` | Generated documentation output root |
| `REPOWISE_LLM_PROVIDER` | `openai` | Active LLM (`openai`, `anthropic`, `gemini`) |
| `REPOWISE_OPENAI_API_KEY` | — | Required for OpenAI |
| `REPOWISE_ANTHROPIC_API_KEY` | — | Required for Anthropic |
| `REPOWISE_GEMINI_API_KEY` | — | Required for Gemini |
| `REPOWISE_OPENAI_MODEL` | `gpt-4o-mini` | OpenAI model name |
| `REPOWISE_ANTHROPIC_MODEL` | `claude-3-5-haiku-20241022` | Anthropic model name |
| `REPOWISE_GEMINI_MODEL` | `gemini-2.0-flash` | Gemini model name |
| `REPOWISE_PII_PATTERNS` | `AADHAAR,PAN,UPI_ID,MOBILE,EMAIL` | Enabled compliance PII scans (wired to `MarkerEngine`) |
| `REPOWISE_TOKEN_BLOAT_THRESHOLD` | `50` | Max tokens/line before `token_bloat` marker |
| `REPOWISE_WORKER_COUNT` | `3` | JobManager worker goroutines |
| `REPOWISE_WORK_QUEUE_SIZE` | `32` | JobManager queue buffer |
| `REPOWISE_JOB_LISTENER_BUFFER` | `10` | Per-subscriber SSE job update buffer |
| `REPOWISE_MOCK_STREAM_TOKEN_DELAY_MS` | `50` | Stub LLM stream delay per token (ms) |
| `REPOWISE_CORS_ALLOWED_ORIGINS` | — | Comma-separated browser origins for SSE (empty = no CORS headers) |

## Path resolution

- Relative `REPOWISE_DB_PATH` values resolve under `REPOWISE_DATA_DIR` via `config.ResolveDBPath()`.
- Documentation output resolves via `config.ResolveDocsPath()`.

## Adding a new setting

1. Add the field to `backend/pkg/config/config.go` with a `default:` tag.
2. Wire the value through `Instance`, servers, or providers as needed.
3. Document it in this file and in `backend/.env.example`.
