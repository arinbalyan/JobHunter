package tracker

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

// Database defines the minimal interface the tracker needs.
type Database interface {
	MarkOpened(ctx context.Context, trackingID string) error
	MarkClicked(ctx context.Context, trackingID string) error
	Close()
}

// Server is a lightweight HTTP server for email tracking.
type Server struct {
	server *http.Server
	db     Database
	port   int
}

// New creates a new tracking server.
func New(db Database, port int) *Server {
	return &Server{
		db:   db,
		port: port,
	}
}

// transparentGIF is a 1x1 transparent GIF pixel (43 bytes).
// Base64 encoded: R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7
var transparentGIF = func() []byte {
	data, _ := base64.StdEncoding.DecodeString("R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7")
	return data
}()

// Start begins listening on the configured port.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/track", s.handleTrack)
	mux.HandleFunc("/click", s.handleClick)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      withLogging(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Printf("[tracker] listening on :%d", s.port)

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("tracker server: %w", err)
	}
	return nil
}

// handleTrack serves a 1x1 transparent GIF and logs the open event.
func (s *Server) handleTrack(w http.ResponseWriter, r *http.Request) {
	trackingID := r.URL.Query().Get("id")

	// Log the open asynchronously (don't block the pixel)
	if trackingID != "" {
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.db.MarkOpened(ctx, id); err != nil {
				log.Printf("[tracker] failed to record open for %s: %v", id, err)
			}
		}(trackingID)
	}

	// Return tracking pixel
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(transparentGIF)
}

// handleClick redirects the user and logs the click event.
func (s *Server) handleClick(w http.ResponseWriter, r *http.Request) {
	trackingID := r.URL.Query().Get("id")
	targetURL := r.URL.Query().Get("url")

	// Log the click asynchronously
	if trackingID != "" {
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.db.MarkClicked(ctx, id); err != nil {
				log.Printf("[tracker] failed to record click for %s: %v", id, err)
			}
		}(trackingID)
	}

	// Decode and validate target URL
	if targetURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "missing url parameter")
		return
	}

	decodedURL, err := url.QueryUnescape(targetURL)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "invalid url parameter")
		return
	}

	http.Redirect(w, r, decodedURL, http.StatusFound)
}

// handleHealth returns a simple health check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// withLogging wraps a handler with basic request logging.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[tracker] %s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
