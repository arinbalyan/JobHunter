package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	scrappy "github.com/arinbalyan/scrappy/pkg/scrappy"
)

// ponytail: stdin has scrappy.ScraperInput + timeout_seconds.
// scrappy.ScraperInput already has SiteSearch/SiteLocation with json tags.

type bridgeInput struct {
	scrappy.ScraperInput
	TimeoutSeconds int `json:"timeout_seconds"`
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

	jobs, err := engine.ScrapeJobs(ctx, raw.ScraperInput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scrape error: %v\n", err)
	}

	if jobs == nil {
		jobs = []scrappy.JobPost{}
	}

	out, err := json.Marshal(jobs)
	if err != nil {
		log.Fatalf("error marshaling output: %v", err)
	}
	os.Stdout.Write(out)
}
