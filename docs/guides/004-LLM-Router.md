# 004-LLM Router

## Overview

JobHunter uses a multi-provider LLM router that distributes requests across all configured providers based on task complexity, availability, and cost. OpenRouter models are discovered dynamically at startup.

## Provider Strategy

The router supports 10+ providers, all using the OpenAI-compatible API format:

| Provider | Speed | Best For |
|----------|-------|----------|
| OpenRouter | Varies | Complex reasoning, email generation (28+ free models auto-discovered) |
| Groq | Very fast | Classification, scoring, simple tasks |
| Together AI | Fast | General inference |
| DeepInfra | Fast | General inference |
| Fireworks AI | Fast | General inference |
| Hyperbolic | Fast | General inference |
| SambaNova | Fast | General inference |
| Cerebras | Very fast | Simple tasks |
| NVIDIA NIM | Fast | Nemotron models |
| Z.AI | Moderate | GLM-4 |

## Dynamic Model Discovery

For OpenRouter, models are not hardcoded. On startup, the agent:

1. Calls `https://openrouter.ai/api/v1/models`
2. Filters all models with `:free` suffix or zero pricing
3. Sorts by context length descending (most capable first)
4. Injects all discovered models into the router with weighted priority

This means new free models are automatically picked up without code changes.

## Routing Logic

```
Task Classification
    |
    +-- TaskSimple  -> Prefer fast providers (Groq, Cerebras, Together, etc.)
    +-- TaskMedium  -> Weighted round-robin across all healthy providers
    +-- TaskComplex -> Prefer capable providers (OpenRouter, SambaNova, Z.AI)
```

## Round-Robin

- Providers are selected using weighted atomic counter
- Higher context = higher weight for OpenRouter models
- Failed providers are automatically skipped
- Fallback to next provider if primary fails
- Emergency catch-all: `openrouter/free` as last resort

## Token Budget Management

```
LLM_MAX_TOKENS_PER_RUN=100000      Hard cap per execution
LLM_MAX_TOKENS_PER_REQUEST=2048    Max per single API call
```

- Requests are capped to stay within budget
- When budget is exhausted, remaining requests fail gracefully
- Token tracking is per-provider for visibility

## Configuration

```env
# OpenRouter (required for auto-discovery of free models)
OPENROUTER_API_KEY=sk-or-...

# Optional: additional providers for faster simple tasks
GROQ_API_KEY=gsk-...
TOGETHER_API_KEY=tgp_...
DEEPINFRA_API_KEY=...
FIREWORKS_API_KEY=fw_...
HYPERBOLIC_API_KEY=...
SAMBANOVA_API_KEY=...
CEREBRAS_API_KEY=csk-...
NVIDIA_API_KEY=nvapi-...
ZAI_API_KEY=...

# Model preferences (optional - dynamic discovery used as fallback)
LLM_COMPLEX_MODEL=google/gemma-4-26b-a4b-it:free
LLM_SIMPLE_MODEL=google/gemma-4-9b-it

# Budget
LLM_MAX_TOKENS_PER_RUN=100000
LLM_MAX_TOKENS_PER_REQUEST=2048
```

## Health Checks

- Failed providers are marked unhealthy and skipped
- Provider stats available via `router.Stats()`
- Fallback chain ensures reliability

## Usage in Plugins

```go
// The router is used internally for email generation.
// Plugins can access it via env.Config().(*config.Config)
// and use the router package directly if needed.
```
