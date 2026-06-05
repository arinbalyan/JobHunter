// Vercel serverless entry point for the JobHunter email tracking server.
// Vercel's Go runtime requires `package handler` with an exported `Handler` function.
package handler

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/email/tracker"
	"github.com/arinbalyan/jobhunter/internal/logging"
)

var (
	server *tracker.Server
	once   sync.Once
)

func initServer() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("tracker: config load failed: %v", err)
	}

	ctx := context.Background()
	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("tracker: db connect failed: %v", err)
	}

	logger := logging.New(cfg.LogLevel, os.Stdout)
	server = tracker.New(dbPool, cfg.TrackingServerPort, logger)
	server.SetupRoutes()
}

func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(initServer)
	server.ServeHTTP(w, r)
}
