# Configuration

All environment variables use the `ARGUS_` prefix. They are loaded as a singleton via `config.Load()` in `backend/pkg/config/config.go`.

Copy `backend/.env.example` to `backend/.env` and adjust values for local development.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `ARGUS_APP_NAME` | `argus` | Service identifier (MCP server name) |
| `ARGUS_ENV` | `development` | Runtime environment label |
| `ARGUS_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `ARGUS_DB_PATH` | `argus.db` (under `ARGUS_DATA_DIR`) | SQLite file name or path |
| `ARGUS_DATA_DIR` | `data` | Base directory for DB and externalized state |
| `ARGUS_COVERAGE_FILE` | `""` | Path to coverage artifact (`lcov.info`, `coverage.xml`, `clover.xml`). Auto-detected if empty. |
| `ARGUS_DOCS_DIR` | `docs` | Generated documentation output root |
| `ARGUS_LLM_PROVIDER` | `openai` | Active LLM (`openai`, `anthropic`, `gemini`) |
| `ARGUS_OPENAI_API_KEY` | — | Required for OpenAI |
| `ARGUS_OPENAI_BASE_URL` | `https://api.openai.com/v1` | OpenAI API base URL; set to `https://openrouter.ai/api/v1` for OpenRouter |
| `ARGUS_ANTHROPIC_API_KEY` | — | Required for Anthropic |
| `ARGUS_GEMINI_API_KEY` | — | Required for Gemini |
| `ARGUS_OPENAI_MODEL` | `gpt-4o-mini` | OpenAI model name |
| `ARGUS_ANTHROPIC_MODEL` | `claude-3-5-haiku-20241022` | Anthropic model name |
| `ARGUS_GEMINI_MODEL` | `gemini-2.0-flash` | Gemini model name |
| `ARGUS_PII_PATTERNS` | `AADHAAR,PAN,UPI_ID,MOBILE,EMAIL` | Enabled compliance PII scans (wired to `MarkerEngine`) |
| `ARGUS_TOKEN_BLOAT_THRESHOLD` | `50` | Max tokens/line before `token_bloat` marker |
| `ARGUS_WORKER_COUNT` | `3` | JobManager worker goroutines |
| `ARGUS_WORK_QUEUE_SIZE` | `32` | JobManager queue buffer |
| `ARGUS_JOB_LISTENER_BUFFER` | `10` | Per-subscriber SSE job update buffer |
| `ARGUS_MOCK_STREAM_TOKEN_DELAY_MS` | `50` | Stub LLM stream delay per token (ms) |
| `ARGUS_CORS_ALLOWED_ORIGINS` | — | Comma-separated browser origins for SSE (empty = no CORS headers) |

## Path resolution

- Relative `ARGUS_DB_PATH` values resolve under `ARGUS_DATA_DIR` via `config.ResolveDBPath()`.
- Documentation output resolves via `config.ResolveDocsPath()`.

## Adding a new setting

1. Add the field to `backend/pkg/config/config.go` with a `default:` tag.
2. Wire the value through `Instance`, servers, or providers as needed.
3. Document it in this file and in `backend/.env.example`.
