package tracker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// Database defines the minimal interface the tracker needs.
type Database interface {
	MarkOpened(ctx context.Context, trackingID string) error
	MarkClicked(ctx context.Context, trackingID string) error
	Close()
}

// maxConcurrentTracking is the maximum number of concurrent tracking DB writes.
// Beyond this, DB writes are processed synchronously to limit goroutine growth.
const maxConcurrentTracking = 100

// Server is a full-featured HTTP server for email tracking with telemetry.
type Server struct {
	server    *http.Server
	db        Database
	port      int
	startTime time.Time
	mux       *http.ServeMux

	trackHits   atomic.Int64
	clickHits   atomic.Int64
	trackErrors atomic.Int64
	clickErrors atomic.Int64
	totalBytes  atomic.Int64

	sem chan struct{} // bounds concurrent goroutines spawned by track/click
}

// New creates a new tracking server.
func New(db Database, port int) *Server {
	return &Server{
		db:        db,
		port:      port,
		startTime: time.Now(),
		sem:       make(chan struct{}, maxConcurrentTracking),
	}
}

var transparentGIF = func() []byte {
	data, _ := base64.StdEncoding.DecodeString("R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7")
	return data
}()

// Start begins listening on the configured port.
func (s *Server) Start(ctx context.Context) error {
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/track", s.handleTrack)
	s.mux.HandleFunc("/click", s.handleClick)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/stats", s.handleStats)
	s.mux.HandleFunc("/version", s.handleVersion)
	s.mux.HandleFunc("/", s.handleRoot)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:     loggingMiddleware(s.mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Printf("[tracker] listening on :%d", s.port)

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

func (s *Server) handleTrack(w http.ResponseWriter, r *http.Request) {
	trackingID := r.URL.Query().Get("id")
	if trackingID != "" {
		// Try to acquire semaphore; if full, process synchronously.
		// This limits goroutine growth under burst traffic.
		select {
		case s.sem <- struct{}{}:
			go func(id string) {
				defer func() { <-s.sem }()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.db.MarkOpened(ctx, id); err != nil {
					log.Printf("[tracker] open failed for %s: %v", id, err)
					s.trackErrors.Add(1)
					return
				}
				s.trackHits.Add(1)
			}(trackingID)
		default:
			// Semaphore full — process synchronously to avoid unbounded goroutines
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.db.MarkOpened(ctx, trackingID); err != nil {
				log.Printf("[tracker] open failed for %s: %v", trackingID, err)
				s.trackErrors.Add(1)
			}
			s.trackHits.Add(1)
		}
	}

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Timing-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	n, _ := w.Write(transparentGIF)
	s.totalBytes.Add(int64(n))
}

func (s *Server) handleClick(w http.ResponseWriter, r *http.Request) {
	trackingID := r.URL.Query().Get("id")
	targetURL := r.URL.Query().Get("url")

	if trackingID != "" {
		// Bounded semaphore to prevent goroutine explosion
		select {
		case s.sem <- struct{}{}:
			go func(id string) {
				defer func() { <-s.sem }()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.db.MarkClicked(ctx, id); err != nil {
					log.Printf("[tracker] click failed for %s: %v", id, err)
					s.clickErrors.Add(1)
					return
				}
				s.clickHits.Add(1)
			}(trackingID)
		default:
			// Semaphore full — process synchronously
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.db.MarkClicked(ctx, trackingID); err != nil {
				log.Printf("[tracker] click failed for %s: %v", trackingID, err)
				s.clickErrors.Add(1)
				return
			}
			s.clickHits.Add(1)
		}
	}

	if targetURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing url parameter"})
		return
	}

	decodedURL, err := url.QueryUnescape(targetURL)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid url"})
		return
	}

	// Validate the redirect target: only https:// is allowed.
	// This prevents the tracking server from being used as an open redirect.
	parsed, err := url.Parse(decodedURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid redirect target — only https:// URLs allowed"})
		return
	}

	http.Redirect(w, r, decodedURL, http.StatusFound)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "ok",
		"version":       "1.0.0",
		"uptime":        time.Since(s.startTime).Round(time.Second).String(),
		"uptime_seconds": int(time.Since(s.startTime).Seconds()),
		"go_version":    runtime.Version(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime).Round(time.Second)
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"server": map[string]interface{}{
			"uptime":         uptime.String(),
			"uptime_seconds": int(time.Since(s.startTime).Seconds()),
			"started_at":     s.startTime.UTC().Format(time.RFC3339),
			"go_version":     runtime.Version(),
			"os":             runtime.GOOS + "/" + runtime.GOARCH,
			"goroutines":     runtime.NumGoroutine(),
			"memory_mb":      fmt.Sprintf("%.1f MB", float64(memStats.Alloc)/1024/1024),
			"heap_objects":   memStats.HeapObjects,
			"gc_cycles":      memStats.NumGC,
		},
		"tracking": map[string]interface{}{
			"opens_total":  s.trackHits.Load(),
			"clicks_total": s.clickHits.Load(),
			"open_errors":  s.trackErrors.Load(),
			"click_errors": s.clickErrors.Load(),
			"bytes_served": s.totalBytes.Load(),
		},
		"endpoints": []map[string]string{
			{"path": "/track", "method": "GET", "description": "Tracking pixel (1x1 GIF + log open)"},
			{"path": "/click", "method": "GET", "description": "Click redirect + log click"},
			{"path": "/health", "method": "GET", "description": "Health check with uptime"},
			{"path": "/stats", "method": "GET", "description": "Full telemetry JSON"},
			{"path": "/version", "method": "GET", "description": "Version info"},
		},
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"version": "1.0.0",
		"name":    "jobhunter-tracker",
		"go":      runtime.Version(),
		"os":      runtime.GOOS + "/" + runtime.GOARCH,
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, `JobHunter Tracking Server

Endpoints:
  GET /track?id=<uuid>     Tracking pixel (returns 1x1 transparent GIF)
  GET /click?id=<uuid>&url= Click redirect (logs click, redirects to url)
  GET /health              Health check
  GET /stats               Full telemetry JSON
  GET /version             Version info

Deployed: %s
Go: %s
`, s.startTime.UTC().Format(time.RFC3339), runtime.Version())
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[tracker] %s %s %s (from %s)", r.Method, r.URL.Path, time.Since(start), extractIP(r))
	})
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	if idx := strings.LastIndexByte(r.RemoteAddr, ':'); idx > 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}
