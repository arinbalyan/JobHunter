package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Config holds scraper configuration matching scrappy's CLI flags.
type Config struct {
	// Search terms (comma-separated for multi-value cartesian product)
	SearchTerms []string

	// Locations (comma-separated for multi-value)
	Locations []string

	// Sites (comma-separated, empty = all 55+)
	Sites []string

	// Results wanted (0 = site default)
	ResultsWanted int

	// Hours old filter (only jobs posted within this window)
	HoursOld int

	// Remote/onsite filters
	RemoteOnly bool
	JobType    string // fulltime | parttime | contract | internship

	// Memory cap in MB (0 = unlimited)
	MemoryCapMB int

	// Proxy (socks5:// or http://)
	Proxy string

	// Email filter: only include jobs with at least one email
	EmailOnly bool

	// Minimum quality score (0-100)
	MinScore int

	// Path to scrappy binary (default: "scrappy" from PATH)
	BinaryPath string
}

// JobResult holds a single scraped job matching scrappy's JSON output.
type JobResult struct {
	ID          string     `json:"id,omitempty"`
	Title       string     `json:"title"`
	CompanyName string     `json:"company_name,omitempty"`
	CompanyURL  string     `json:"company_url,omitempty"`
	JobURL      string     `json:"job_url"`
	Location    string     `json:"location,omitempty"`
	IsRemote    bool       `json:"is_remote,omitempty"`
	Description string     `json:"description,omitempty"`
	JobType     string     `json:"job_type,omitempty"`
	DatePosted  *time.Time `json:"date_posted,omitempty"`
	Site        string     `json:"site"`
	Seniority   string     `json:"seniority,omitempty"`
	Department  string     `json:"department,omitempty"`
	Emails      []struct {
		Addr     string `json:"addr"`
		Verified bool   `json:"verified"`
		Source   string `json:"source"`
	} `json:"emails,omitempty"`
	QualityScore int `json:"quality_score,omitempty"`
}

// FlatEmails returns a deduplicated list of email addresses.
func (j *JobResult) FlatEmails() []string {
	seen := make(map[string]bool)
	var result []string
	for _, e := range j.Emails {
		if e.Addr != "" && !seen[e.Addr] {
			seen[e.Addr] = true
			result = append(result, e.Addr)
		}
	}
	return result
}

// Scraper wraps the scrappy CLI invocation.
type Scraper struct {
	config Config
}

// New creates a new Scraper.
func New(cfg Config) *Scraper {
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "scrappy"
	}
	return &Scraper{
		config: cfg,
	}
}

// Scrape performs job scraping by calling the scrappy CLI with --non-interactive.
// scrappy outputs a JSON array to stdout when --out is not specified.
func (s *Scraper) Scrape(ctx context.Context) ([]JobResult, error) {
	args := s.buildArgs()

	cmd := exec.CommandContext(ctx, s.config.BinaryPath, args...)
	if s.config.Proxy != "" {
		cmd.Env = append(cmd.Environ(), "HTTP_PROXY="+s.config.Proxy, "HTTPS_PROXY="+s.config.Proxy)
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("scrappy failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("scrappy exec: %w", err)
	}

	// scrappy outputs a JSON array to stdout
	var jobs []JobResult
	if err := json.Unmarshal(output, &jobs); err != nil {
		return nil, fmt.Errorf("parse scrappy output: %w", err)
	}

	return jobs, nil
}

// buildArgs constructs CLI arguments for scrappy's non-interactive mode.
func (s *Scraper) buildArgs() []string {
	var args []string

	// Search terms
	if len(s.config.SearchTerms) > 0 {
		args = append(args, "--search", strings.Join(s.config.SearchTerms, ","))
	}

	// Locations
	if len(s.config.Locations) > 0 {
		args = append(args, "--location", strings.Join(s.config.Locations, ","))
	}

	// Sites
	if len(s.config.Sites) > 0 {
		args = append(args, "--sites", strings.Join(s.config.Sites, ","))
	}

	// Results wanted
	if s.config.ResultsWanted > 0 {
		args = append(args, "--results-wanted", fmt.Sprintf("%d", s.config.ResultsWanted))
	}

	// Hours old — scrappy doesn't have a direct --hours-old flag,
	// but we can use --min-score combined with quality filtering.
	// For now, we rely on scrappy's built-in date filtering via its config.

	// Remote filter
	if s.config.RemoteOnly {
		args = append(args, "--remote-only")
	}

	// Job type
	if s.config.JobType != "" {
		args = append(args, "--job-type", s.config.JobType)
	}

	// Email filter
	if s.config.EmailOnly {
		args = append(args, "--email")
	}

	// Min score
	if s.config.MinScore > 0 {
		args = append(args, "--min-score", fmt.Sprintf("%d", s.config.MinScore))
	}

	// Memory cap
	if s.config.MemoryCapMB > 0 {
		args = append(args, "--memory-cap", fmt.Sprintf("%dMB", s.config.MemoryCapMB))
	}

	// Proxy
	if s.config.Proxy != "" {
		args = append(args, "--proxy", s.config.Proxy)
	}

	// Non-interactive mode (required for CLI-only execution)
	args = append(args, "--non-interactive")

	return args
}
