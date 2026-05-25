// Command: cleanup
// Removes skipped jobs older than N days (fortnightly cleanup).
// Also marks stale pending items as skipped.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/logging"
	"github.com/arinbalyan/jobhunter/internal/migrations"
)

func main() {
	if val := os.Getenv("GOMEMLIMIT"); val == "" {
		debug.SetMemoryLimit(80 * 1024 * 1024)
	}

	startTime := time.Now()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	logger := logging.New(cfg.LogLevel, os.Stdout)
	logger.Info("Cleanup workflow starting...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
	}()

	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed: %v", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	if err := migrations.Run(cfg.DatabaseURL); err != nil {
		logger.Error("migrations failed: %v", err)
		os.Exit(1)
	}

	// Delete skipped jobs older than 14 days
	deleted, err := dbPool.DeleteOldSkippedJobs(ctx, 14)
	if err != nil {
		logger.Error("delete old skipped jobs: %v", err)
	} else {
		logger.Info("deleted %d skipped jobs older than 14 days", deleted)
	}

	// Mark stale pending items (>7 days) as skipped
	stalePendings, err := dbPool.MarkStalePendingQueue(ctx, 7)
	if err != nil {
		logger.Error("mark stale pending: %v", err)
	} else {
		logger.Info("marked %d stale pending items as skipped", stalePendings)
	}

	duration := time.Since(startTime)
	logger.Info("cleanup complete in %.0fs", duration.Seconds())

	// Telegram notification
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		msg := fmt.Sprintf(
			"<b>Cleanup Complete</b>\nDeleted skipped: %d\nStale skipped: %d\nDuration: %.0fs",
			deleted, stalePendings, duration.Seconds(),
		)
		body := fmt.Sprintf(`{"chat_id":"%s","text":"%s","parse_mode":"HTML"}`, cfg.TelegramChatID, strings.ReplaceAll(msg, `"`, `\"`))
		req, _ := http.NewRequestWithContext(ctx, "POST",
			fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.TelegramBotToken),
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}
}
