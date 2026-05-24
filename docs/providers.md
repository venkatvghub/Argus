# LLM Providers

Argus supports three LLM backends: OpenAI (including OpenRouter), Anthropic, and Gemini. Each is selected via `ARGUS_LLM_PROVIDER` and configured with the corresponding API key.

## Provider setup

| Provider | Env vars |
|---|---|
| **OpenAI** | `ARGUS_LLM_PROVIDER=openai`<br>`ARGUS_OPENAI_API_KEY=sk-…`<br>`ARGUS_OPENAI_MODEL=gpt-4o-mini` |
| **Anthropic** | `ARGUS_LLM_PROVIDER=anthropic`<br>`ARGUS_ANTHROPIC_API_KEY=sk-ant-…`<br>`ARGUS_ANTHROPIC_MODEL=claude-3-5-haiku-20241022` |
| **Gemini** | `ARGUS_LLM_PROVIDER=gemini`<br>`ARGUS_GEMINI_API_KEY=AIza…`<br>`ARGUS_GEMINI_MODEL=gemini-2.0-flash` |
| **OpenRouter** | `ARGUS_LLM_PROVIDER=openai`<br>`ARGUS_OPENAI_API_KEY=sk-or-…`<br>`ARGUS_OPENAI_BASE_URL=https://openrouter.ai/api/v1`<br>`ARGUS_OPENAI_MODEL=<any slug>` |

## Model tiers

`argus init` uses a three-tier router: **cheap** for bulk page generation, **medium** for module and API pages, **premium** for repo overview and architecture pages.

| Provider | Cheap | Medium | Premium |
|---|---|---|---|
| OpenAI | `gpt-4o-mini` | `gpt-4o` | `gpt-4-turbo` |
| Anthropic | `claude-haiku-4-5-20251001` | `claude-sonnet-4-6` | `claude-opus-4-7` |
| Gemini | `gemini-2.0-flash` | `gemini-1.5-pro` | `gemini-ultra` |
| OpenRouter | `mistralai/mistral-7b-instruct` | `openai/gpt-4o` | `anthropic/claude-opus-4` |

Override any tier at runtime:

```bash
./argus init /path/to/repo \
  --cheap-model gemini-2.0-flash \
  --medium-model claude-sonnet-4-6 \
  --premium-model claude-opus-4-7
```

Or set for every run via env:

```
ARGUS_CHEAP_MODEL=gemini-2.0-flash
ARGUS_MEDIUM_MODEL=claude-sonnet-4-6
ARGUS_PREMIUM_MODEL=claude-opus-4-7
```

## Resilience

Each tier has independent retry and circuit-breaker settings (see `ARGUS_LLM_*` vars in [configuration.md](configuration.md)):

- Exponential backoff with jitter (`LLM_RETRY_*`)
- Circuit breaker: opens after N consecutive transient failures, half-opens after reset timeout
- Permanent errors (401, 403) never trip the circuit breaker
- Streaming responses bypass retry (partial output cannot be replayed)

## Model discovery (OpenRouter)

When `ARGUS_OPENAI_BASE_URL` points to OpenRouter, `argus init` fetches live model pricing from the `/models` endpoint and presents a numbered menu for tier selection. Free/experimental models (zero pricing) are excluded.

Pass `--yes` to skip interactive selection and use tier defaults, or pre-set `ARGUS_CHEAP_MODEL` / `ARGUS_MEDIUM_MODEL` / `ARGUS_PREMIUM_MODEL` to skip discovery entirely.
