# Contributing to JobHunter

We love contributions! Here's how to get started.

## Development Workflow

1. **Branch from `dev`** — all development happens on the `dev` branch
2. **Make your changes** — follow Go conventions
3. **Test** — `go build ./... && go vet ./...`
4. **Commit** — clear messages, one change per commit
5. **Push** and open a **PR to `dev`**

## Code Style

- Run `gofmt -s` before committing
- Run `go vet ./...` — no warnings
- Meaningful variable names, no stutter (`jobs.Jobs` → `jobs.List`)
- Comments on exported functions and types
- Errors are wrapped with context: `fmt.Errorf("scrape jobs: %w", err)`

## Adding a Plugin

1. Create your plugin file in `plugins/`
2. Implement `sdk.Plugin` interface
3. Register in `plugins/register.go`
4. Use scoped env vars: `PLUGIN_YOURBOT_API_KEY`
5. Add docs in `docs/guides/`

## Commit Messages

```
feat:     New feature
fix:      Bug fix
docs:     Documentation
chore:    Build, deps, config
refactor: Code restructure
test:     Tests
```

## PR Checklist

- [ ] Code compiles (`go build ./...`)
- [ ] No vet warnings (`go vet ./...`)
- [ ] .env.example updated if new env vars added
- [ ] Docs updated if behavior changes
- [ ] Gitignore updated if new generated files
