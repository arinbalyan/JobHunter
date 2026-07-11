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
	// ponytail: log to stderr so Rust tracing picks it up in GH log
	fmt.Fprintf(os.Stderr, "bridge: started\n")

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("error reading stdin: %v", err)
	}
	fmt.Fprintf(os.Stderr, "bridge: read %d bytes from stdin\n", len(data))

	var raw bridgeInput
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Fatalf("error parsing input JSON: %v", err)
	}
	fmt.Fprintf(os.Stderr, "bridge: %d sites, %d search terms, %d locations, results_wanted=%d\n",
		len(raw.Sites), len(raw.SearchTerms), len(raw.Locations), raw.ResultsWanted)

	timeout := time.Duration(raw.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	fmt.Fprintf(os.Stderr, "bridge: timeout=%ds\n", raw.TimeoutSeconds)

	engine := scrappy.NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go ramMonitor(ctx, cancel, 5000, 10*time.Second)

	// ponytail: stream each job to stdout as it's scraped, so Rust inserts into DB
	// immediately instead of waiting for all sites to finish.
	fmt.Fprintf(os.Stderr, "bridge: calling ScrapeJobsStream\n")

	enc := json.NewEncoder(os.Stdout)
	jobCount := 0
	err = engine.ScrapeJobsStream(ctx, raw.ScraperInput, func(job scrappy.JobPost) {
		if err := enc.Encode(job); err != nil {
			fmt.Fprintf(os.Stderr, "encode error: %v\n", err)
		}
		jobCount++
		// ponytail: log every 1000th job so CI log shows progress
		if jobCount%1000 == 0 {
			fmt.Fprintf(os.Stderr, "bridge: streamed %d jobs\n", jobCount)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "scrape error: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "bridge: done, total jobs=%d\n", jobCount)
}
