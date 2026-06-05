// Command: tracker
// Standalone email tracking server.
// Serves tracking pixel and click redirect endpoints.
//
// For Vercel: exports Handler() which Vercel calls as a serverless function.
// For local: main() starts an HTTP server.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/email/tracker"
	"github.com/arinbalyan/jobhunter/internal/logging"
)

// Global server state for Vercel — initialized once, reused across warm invocations.
var (
	globalServer *tracker.Server
	globalOnce   sync.Once
)

// initVercel initializes the server once for Vercel serverless invocations.
func initVercel() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()
	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}

	logger := logging.New(cfg.LogLevel, os.Stdout)
	globalServer = tracker.New(dbPool, cfg.TrackingServerPort, logger)
	globalServer.SetupRoutes()
}

// Handler is called by Vercel's Go runtime for each serverless invocation.
func Handler(w http.ResponseWriter, r *http.Request) {
	globalOnce.Do(initVercel)
	globalServer.ServeHTTP(w, r)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer dbPool.Close()

	logger := logging.New(cfg.LogLevel, os.Stdout)

	server := tracker.New(dbPool, cfg.TrackingServerPort, logger)

	logger.Info("starting tracking server on :%d", cfg.TrackingServerPort)
	if err := server.Start(ctx); err != nil {
		logger.Error("tracking server failed: %v", err)
		os.Exit(1)
	}
}
