package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Config holds scraper configuration.
type Config struct {
	Sites           []string
	SearchTerms     []string
	Locations       []string
	ResultsPerSite  int
	HoursOld        int
	RemoteOnly      bool
	JobType         string
	MemoryCapMB     int
	Proxy           string
}

// JobResult holds a single scraped job in our internal format.
type JobResult struct {
	ID          string    `json:"id,omitempty"`
	Title       string    `json:"title"`
	Company     string    `json:"company_name,omitempty"`
	CompanyURL  string    `json:"company_url,omitempty"`
	JobURL      string    `json:"job_url"`
	Location    string    `json:"location,omitempty"`
	IsRemote    bool      `json:"is_remote,omitempty"`
	Description string    `json:"description,omitempty"`
	JobType     string    `json:"job_type,omitempty"`
	DatePosted  *time.Time `json:"date_posted,omitempty"`
	Source      string    `json:"site"`
	Seniority   string    `json:"seniority,omitempty"`
	Emails      []string  `json:"emails,omitempty"`
}

// Scraper wraps the scrappy engine via CLI invocation.
type Scraper struct {
	config Config
}

// New creates a new Scraper.
func New(cfg Config) *Scraper {
	return &Scraper{
		config: cfg,
	}
}

// Scrape performs job scraping by calling the scrappy CLI.
func (s *Scraper) Scrape(ctx context.Context) ([]JobResult, error) {
	args := s.buildArgs()

	cmd := exec.CommandContext(ctx, "scrappy", args...)
	if s.config.Proxy != "" {
		cmd.Env = append(cmd.Environ(), "HTTP_PROXY="+s.config.Proxy, "HTTPS_PROXY="+s.config.Proxy)
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("scrappy failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("scrappy exec: %w", err)
	}

	// Try array parse first
	var jobs []JobResult
	if err := json.Unmarshal(output, &jobs); err != nil {
		// Fallback: JSON lines (one JSON object per line)
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var job JobResult
			if err := json.Unmarshal([]byte(line), &job); err == nil && job.Title != "" {
				jobs = append(jobs, job)
			}
		}
	}

	return jobs, nil
}

// buildArgs constructs CLI arguments for scrappy.
func (s *Scraper) buildArgs() []string {
	args := []string{"scrape"}
	for _, site := range s.config.Sites {
		args = append(args, "--sites", site)
	}
	for _, term := range s.config.SearchTerms {
		args = append(args, "--search-term", term)
	}
	for _, loc := range s.config.Locations {
		args = append(args, "--location", loc)
	}
	if s.config.ResultsPerSite > 0 {
		args = append(args, "--results-wanted", fmt.Sprintf("%d", s.config.ResultsPerSite))
	}
	if s.config.HoursOld > 0 {
		args = append(args, "--hours-old", fmt.Sprintf("%d", s.config.HoursOld))
	}
	if s.config.RemoteOnly {
		args = append(args, "--remote-only")
	}
	if s.config.JobType != "" {
		args = append(args, "--job-type", s.config.JobType)
	}
	if s.config.MemoryCapMB > 0 {
		args = append(args, "--memory-cap-mb", fmt.Sprintf("%d", s.config.MemoryCapMB))
	}
	args = append(args, "--output", "json")
	return args
}
