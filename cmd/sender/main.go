// Command: sender
// Main entry point for the JobHunter agent.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/migrations"
	"github.com/arinbalyan/jobhunter/internal/plugin"
	"github.com/arinbalyan/jobhunter/internal/plugin/sdk"
	"github.com/arinbalyan/jobhunter/internal/report"
	"github.com/arinbalyan/jobhunter/internal/telegram"
	"github.com/arinbalyan/jobhunter/plugins"
)

func main() {
	if val := os.Getenv("GOMEMLIMIT"); val == "" {
		debug.SetMemoryLimit(80 * 1024 * 1024) // 80 MB
	}

	startTime := time.Now()

	// Load env config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load YAML config (overrides env values)
	yamlCfg, err := config.LoadYAML(".agent-data/config.yaml")
	if err != nil {
		log.Printf("Warning: could not load config.yaml: %v", err)
	} else {
		yamlCfg.MergeIntoConfig(cfg)
		log.Printf("Loaded config.yaml with %d rejection patterns, %d email filters",
			len(yamlCfg.RejectTitles), len(yamlCfg.EmailFilters))
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// Logger
	logger := logging.New(cfg.LogLevel, os.Stdout)
	logger.Info("JobHunter agent starting...")

	// Context with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

	// ── Quota Check ──────────────────────────────────────────────────────
	quota := report.NewQuotaTracker(ctx, dbPool, cfg.DailyEmailLimit)
	if quota.Exhausted() {
		logger.Warn("daily email quota exhausted (%d/%d sent today)", quota.TodayCount, cfg.DailyEmailLimit)
	} else {
		logger.Info("email quota: %d/%d used today, %d remaining", quota.TodayCount, cfg.DailyEmailLimit, quota.Remaining())
	}

	// ── Plugin System ────────────────────────────────────────────────────
	mgr := plugin.NewManager()
	pluginDB := db.NewPluginDB(dbPool)

	envProvider := func(p sdk.Plugin) sdk.Env {
		return plugin.NewEnv(p, pluginDB, cfg, logger)
	}
	reg := &registrar{mgr: mgr, envProvider: envProvider}
	plugins.RegisterBuiltinPlugins(reg)

	// ── Execute Plugins ──────────────────────────────────────────────────
	logger.Info("executing %d plugins...", len(mgr.List()))
	results := mgr.RunAll(ctx)

	// ── Summary & Report ─────────────────────────────────────────────────
	logger.Info("run complete")
	logger.Info("%s", plugin.Summary(results))

	// Send Telegram report
	tgBot := telegram.New(cfg.TelegramBotToken, cfg.TelegramChatID)
	if tgBot.Enabled() {
		telegramItems := make([]telegram.PluginReportItem, len(results))
		for i, r := range results {
			telegramItems[i] = telegram.PluginReportItem{
				PluginID:   r.PluginID,
				PluginName: r.PluginName,
				Success:    r.Success || r.Error == nil,
				Message:    r.Message,
				Duration:   r.Duration,
				Metrics:    r.Metrics,
			}
		}

		reportMsg := &telegram.ReportMessage{
			Title:         "JobHunter Run Complete",
			PluginResults: telegramItems,
			Duration:      time.Since(startTime),
			Timestamp:     time.Now().UTC(),
		}

		if err := tgBot.SendReport(ctx, reportMsg); err != nil {
			logger.Error("failed to send Telegram report: %v", err)
		} else {
			logger.Info("Telegram report sent")
		}
	}

	// Prepare next cycle: exit for cron, loop for daemon
	exitCode := 0
	for _, r := range results {
		if r.Error != nil || !r.Success {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

type registrar struct {
	mgr         *plugin.Manager
	envProvider func(sdk.Plugin) sdk.Env
}

func (r *registrar) Register(p sdk.Plugin, _ interface{}) {
	r.mgr.Register(p, r.envProvider)
}
