# 002-Plugin System
## Architecture
JobHunter uses a plugin-based architecture. Every feature — including the core job hunter agent — is a plugin.

## The Plugin Interface
All plugins implement `internal/plugin/sdk/sdk.go`:

```go
type Plugin interface {
    ID() string
    Name() string
    Description() string
    Execute(ctx context.Context, env Env) (*Result, error)
}
```

## Built-in Plugins

| Plugin ID | Name | Description |
|-----------|------|-------------|
| `jobhunter` | Job Hunter Agent | Scrapes job boards, matches jobs, sends outreach emails |

## Writing a Plugin

### 1. Create the file
```go
// plugins/mybot.go
package plugins

import (
    "context"
    "github.com/arinbalyan/jobhunter/internal/plugin/sdk"
)

type MyBotPlugin struct {
    sdk.BasePlugin
}

func NewMyBotPlugin() *MyBotPlugin {
    return &MyBotPlugin{
        BasePlugin: sdk.BasePlugin{
            PluginID:   "mybot",
            PluginName: "My Custom Bot",
            PluginDesc: "Does something useful",
        },
    }
}

func (p *MyBotPlugin) Execute(ctx context.Context, env sdk.Env) (*sdk.Result, error) {
    // env.Getenv("API_KEY")           → reads PLUGIN_MYBOT_API_KEY or global API_KEY
    // env.DB()                         → database handle
    // env.Logger()                     → scoped logger
    // env.Config()                     → shared app config

    return sdk.SimpleResult("done"), nil
}
```

### 2. Register it
```go
// plugins/register.go
func RegisterBuiltinPlugins(mgr PluginRegistrar) {
    mgr.Register(NewJobHunterPlugin(), nil)
    mgr.Register(NewMyBotPlugin(), nil) // ← add yours
}
```

### 3. Scoped Environment Variables
Each plugin gets its own namespace:
```bash
# .env
PLUGIN_MYBOT_API_KEY=xxx
PLUGIN_MYBOT_MAX_ITEMS=50
```

The SDK's `Getenv` checks `PLUGIN_{ID}_{KEY}` first, then falls back to `{KEY}`.

## Plugin Lifecycle
1. **Registration** — plugins are registered at startup
2. **Execution** — `RunAll()` or `RunByIDs()` calls each plugin's `Execute()`
3. **Result Collection** — results are collected and summarized
4. **Stats Flush** — stats from all plugins are flushed to DB

## Database Access
Plugins get a restricted database interface (not the full pool):
- `InsertEmail()` — record sent emails (tracking works automatically)
- `RecordStat()` — push time-series stats
- `Exec()` / `Query()` — for plugin-specific tables
