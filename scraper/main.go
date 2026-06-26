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

// ponytail: ~100 lines. Read JSON from stdin, call scrappy, write JSON to stdout.
// No business logic, no config parsing, no filtering.

func main() {
	// Read the full input from stdin.
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("error reading stdin: %v", err)
	}

	var input scrappy.ScraperInput
	if err := json.Unmarshal(data, &input); err != nil {
		log.Fatalf("error parsing input JSON: %v", err)
	}

	engine := scrappy.NewEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	jobs, err := engine.ScrapeJobs(ctx, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scrape error: %v\n", err)
		// Still output partial results if any
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
