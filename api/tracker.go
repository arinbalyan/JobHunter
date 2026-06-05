// Vercel serverless entry point for email open/click tracking.
// Completely self-contained — no imports from jobhunter internal packages.
package handler

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	db   *sql.DB
	once sync.Once
)

// 1x1 transparent GIF pixel
var trackingPixel []byte

func init() {
	var err error
	trackingPixel, err = base64.StdEncoding.DecodeString("R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7")
	if err != nil {
		panic(err)
	}
}

func initDB() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	var err error
	db, err = sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("tracker: db open: %v", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.PingContext(context.Background()); err != nil {
		log.Fatalf("tracker: db ping: %v", err)
	}
}

func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(initDB)

	switch r.URL.Path {
	case "/track":
		handleTrack(w, r)
	case "/click":
		handleClick(w, r)
	case "/health":
		handleHealth(w)
	case "/version":
		handleVersion(w)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","tracker":"jobhunter"}`)
	}
}

func handleTrack(w http.ResponseWriter, r *http.Request) {
	trackingID := r.URL.Query().Get("id")
	if trackingID != "" {
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			db.ExecContext(ctx, `UPDATE emails SET opened_at = COALESCE(opened_at, NOW()) WHERE tracking_id = $1`, id)
		}(trackingID)
	}

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(trackingPixel)
}

func handleClick(w http.ResponseWriter, r *http.Request) {
	redirectURL := r.URL.Query().Get("url")
	trackingID := r.URL.Query().Get("id")

	if trackingID != "" {
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			db.ExecContext(ctx, `UPDATE emails SET clicked_at = COALESCE(clicked_at, NOW()) WHERE tracking_id = $1`, id)
		}(trackingID)
	}

	if redirectURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "missing url parameter")
		return
	}

	if redirectURL[:4] != "http" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "invalid url")
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func handleHealth(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"healthy","db":"ok"}`)
}

func handleVersion(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"version":"1.0.0","tracker":"jobhunter"}`)
}
