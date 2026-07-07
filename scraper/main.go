package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"

	scrappy "github.com/arinbalyan/scrappy/pkg/scrappy"
)

type bridgeInput struct {
	scrappy.ScraperInput
	TimeoutSeconds int `json:"timeout_seconds"`
}

// ramMonitor checks heap usage every interval and calls cancel() if threshold exceeded.
// Prevents OOM on GH Actions runners (7GB). Leaves headroom for OS + Rust process.
func ramMonitor(ctx context.Context, cancel context.CancelFunc, thresholdMB int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			allocMB := int(m.Alloc / 1024 / 1024)
			if allocMB > thresholdMB {
				fmt.Fprintf(os.Stderr, "RAM %dMB > %dMB limit — stopping scrape gracefully\n", allocMB, thresholdMB)
				cancel()
				return
			}
		}
	}
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("error reading stdin: %v", err)
	}

	var raw bridgeInput
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Fatalf("error parsing input JSON: %v", err)
	}

	timeout := time.Duration(raw.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	engine := scrappy.NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// ponytail: monitor RAM every 10s, cancel at 5GB (GH Actions has 7GB total)
	go ramMonitor(ctx, cancel, 5000, 10*time.Second)

	// Switch to ScrapeJobsFull for per-site stats
	result, err := engine.ScrapeJobsFull(ctx, raw.ScraperInput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scrape error: %v\n", err)
	}

	var jobs []scrappy.JobPost
	if result != nil {
		jobs = result.Jobs
	}

	if jobs == nil {
		jobs = []scrappy.JobPost{}
	}

	// NDJSON: first emit site stats as a metadata line, then one job per line.
	// Rust reads line-by-line — first line is stats, rest are job posts.
	if result != nil && len(result.Sites) > 0 {
		stats := struct {
			Type  string                 `json:"type"`
			Sites []scrappy.SiteResult    `json:"sites"`
			Total int                     `json:"total"`
		}{
			Type:  "site_stats",
			Sites: result.Sites,
			Total: len(result.Sites),
		}
		statsJSON, _ := json.Marshal(stats)
		fmt.Println(string(statsJSON))
	}

	enc := json.NewEncoder(os.Stdout)
	for _, job := range jobs {
		if err := enc.Encode(job); err != nil {
			fmt.Fprintf(os.Stderr, "encode error: %v\n", err)
		}
	}
}
