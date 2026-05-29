// Command: tracker
// Standalone email tracking server.
// Serves tracking pixel and click redirect endpoints.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/email/tracker"
	"github.com/arinbalyan/jobhunter/internal/logging"
)

func main() {
	// Load config (minimal — just DB and tracking server settings)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Database connection with cancellable context
	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer dbPool.Close()

	// Structured logger
	logger := logging.New(cfg.LogLevel, os.Stdout)

	// Create tracking server
	server := tracker.New(dbPool, cfg.TrackingServerPort, logger)

	logger.Info("starting tracking server on :%d", cfg.TrackingServerPort)
	if err := server.Start(ctx); err != nil {
		logger.Error("tracking server failed: %v", err)
		os.Exit(1)
	}
}
