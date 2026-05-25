package plugin

import (
	"os"
	"strings"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/plugin/sdk"
)

// env implements sdk.Env with scoped env var access.
type env struct {
	plugin   sdk.Plugin
	db       sdk.Database
	appCfg   *config.Config
	logger   *logging.Logger
}

// NewEnv creates a new Env for a plugin.
func NewEnv(plugin sdk.Plugin, db sdk.Database, appCfg *config.Config, log *logging.Logger) sdk.Env {
	return &env{
		plugin: plugin,
		db:     db,
		appCfg: appCfg,
		logger: log,
	}
}

// Getenv returns an env var with plugin-scoped fallback.
// Priority:
//  1. PLUGIN_{PLUGIN_ID}_{KEY} (plugin-scoped, highest priority)
//  2. {KEY} (global fallback)
//  3. empty string
func (e *env) Getenv(key string) string {
	// Try plugin-scoped env var first
	pluginKey := strings.ToUpper(e.plugin.ID())
	scopedKey := "PLUGIN_" + pluginKey + "_" + key
	if val := os.Getenv(scopedKey); val != "" {
		return val
	}

	// Fall back to global
	return os.Getenv(key)
}

func (e *env) DB() sdk.Database {
	return e.db
}

func (e *env) Logger() sdk.Logger {
	return e.logger.WithField("plugin", e.plugin.ID())
}

func (e *env) Config() interface{} {
	return e.appCfg
}

// Ensure the interface is satisfied.
var _ sdk.Env = (*env)(nil)
