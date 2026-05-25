// Command: sender
// Main entry point for the JobHunter agent.
// Runs migrations, then executes all registered plugins.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/migrations"
	"github.com/arinbalyan/jobhunter/internal/plugin"
	"github.com/arinbalyan/jobhunter/internal/plugin/sdk"
	"github.com/arinbalyan/jobhunter/internal/stats"
	"github.com/arinbalyan/jobhunter/plugins"
)

func main() {
	// Set memory limit if not already set
	if val := os.Getenv("GOMEMLIMIT"); val == "" {
		debug.SetMemoryLimit(80 * 1024 * 1024) // 80 MB
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// Initialize logger
	logger := logging.New(cfg.LogLevel, os.Stdout)
	logger.Info("JobHunter agent starting...")

	// Context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down gracefully...")
		cancel()
	}()

	// ── Database ─────────────────────────────────────────────────────────
	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed: %v", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	logger.Info("connected to database")

	// ── Migrations ───────────────────────────────────────────────────────
	logger.Info("running database migrations...")
	if err := migrations.Run(cfg.DatabaseURL); err != nil {
		logger.Error("migrations failed: %v", err)
		os.Exit(1)
	}
	logger.Info("migrations complete")

	// ── Plugin System ────────────────────────────────────────────────────
	mgr := plugin.NewManager()
	pluginDB := db.NewPluginDB(dbPool)
	statsCollector := stats.NewCollector(100)

	// Helper to create env for each plugin
	envProvider := func(p sdk.Plugin) sdk.Env {
		return plugin.NewEnv(p, pluginDB, cfg, logger)
	}

	// Register built-in plugins
	// Wrap registrar for the plugin registration
	reg := &registrar{mgr: mgr, envProvider: envProvider}
	plugins.RegisterBuiltinPlugins(reg)

	// ── Execute Plugins ──────────────────────────────────────────────────
	logger.Info("executing %d plugins...", len(mgr.List()))
	results := mgr.RunAll(ctx)

	// ── Flush stats ──────────────────────────────────────────────────────
	if err := statsCollector.Flush(ctx, dbPool); err != nil {
		logger.Error("failed to flush stats: %v", err)
	}

	// ── Summary ──────────────────────────────────────────────────────────
	logger.Info("run complete")
	logger.Info("%s", plugin.Summary(results))

	// Exit with non-zero if any plugin failed
	for _, r := range results {
		if r.Error != nil || !r.Success {
			os.Exit(1)
		}
	}
}

// registrar adapts the plugin manager to the plugins package's interface.
type registrar struct {
	mgr         *plugin.Manager
	envProvider func(sdk.Plugin) sdk.Env
}

func (r *registrar) Register(p sdk.Plugin, _ interface{}) {
	r.mgr.Register(p, r.envProvider)
}
