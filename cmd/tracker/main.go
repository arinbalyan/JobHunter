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
)

func main() {
	// Load config (minimal — just DB and tracking server settings)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Database connection for tracking
	dbPool, err := db.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer dbPool.Close()

	// Create tracking server
	server := tracker.New(dbPool, cfg.TrackingServerPort)

	// Context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down tracker...")
		cancel()
	}()

	log.Printf("starting tracking server on :%d", cfg.TrackingServerPort)
	if err := server.Start(ctx); err != nil {
		log.Fatalf("tracking server failed: %v", err)
	}
}
