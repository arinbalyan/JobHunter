# 004-LLM Router

## Overview
JobHunter uses a multi-provider LLM router that distributes requests across
OpenRouter, Groq, and Cerebras based on task complexity and availability.

## Provider Strategy

| Provider | Speed | Best For | Model |
|----------|-------|----------|-------|
| **OpenRouter** | Varies | Complex reasoning, email generation | Configurable (default: `google/gemma-4-9b-it`) |
| **Groq** | Very fast | Classification, scoring, simple tasks | Same model |
| **Cerebras** | Very fast | Classification, scoring, simple tasks | Same model |

## Routing Logic

```
Task Classification
    │
    ├── TaskSimple  → Prefer Groq → Prefer Cerebras → Fallback to OpenRouter
    ├── TaskMedium  → Round-robin across all healthy providers
    └── TaskComplex → Prefer OpenRouter (most capable)
```

## Round-Robin
When multiple providers are healthy and eligible:
- Providers are selected using a weighted atomic counter
- Failed providers are automatically skipped
- Fallback to next provider if primary fails

## Token Budget Management
```go
LLM_MAX_TOKENS_PER_RUN=50000      // Hard cap per execution
LLM_MAX_TOKENS_PER_REQUEST=2048   // Max per single API call
```

- Requests are capped to stay within budget
- When budget is exhausted, remaining requests fail gracefully
- Token tracking is per-provider for visibility

## Configuration

```env
# Required: at least one provider
OPENROUTER_API_KEY=sk-or-...

# Optional: for faster simple tasks
GROQ_API_KEY=gsk-...
CEREBRAS_API_KEY=csk-...

# Models
LLM_COMPLEX_MODEL=google/gemma-4-9b-it
LLM_SIMPLE_MODEL=google/gemma-4-9b-it

# Budget
LLM_MAX_TOKENS_PER_RUN=50000
LLM_MAX_TOKENS_PER_REQUEST=2048
```

## Health Checks
- Failed providers are marked unhealthy and skipped
- Periodic health checks (future: auto-recovery)
- Provider stats available via `router.Stats()`

## Usage in Plugins
```go
// The router is used internally for email generation.
// Plugins can access it via env.Config().(*config.Config)
// and use the router package directly if needed.
```
