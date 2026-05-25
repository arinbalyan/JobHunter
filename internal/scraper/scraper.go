package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Config matches scrappy's CLI flags (now includes HoursOld and compensation).
type Config struct {
	SearchTerms   []string
	Locations     []string
	Sites         []string
	ResultsWanted int
	HoursOld      int      // new: --hours-old filter
	RemoteOnly    bool
	JobType       string
	MemoryCapMB   int
	Proxy         string
	EmailOnly     bool
	MinScore      int
	BinaryPath    string
	ConfigPath    string
}

// Compensation holds salary data from scrappy's JSON output.
type Compensation struct {
	Interval string   `json:"interval,omitempty"`
	MinAmount *float64 `json:"min_amount,omitempty"`
	MaxAmount *float64 `json:"max_amount,omitempty"`
	Currency string   `json:"currency,omitempty"`
}

// EmailEntry mirrors scrappy's email struct.
type EmailEntry struct {
	Addr     string `json:"addr"`
	Verified bool   `json:"verified"`
	Source   string `json:"source"`
}

// JobResult mirrors scrappy's JSON output exactly, including the new compensation field.
type JobResult struct {
	ID           string       `json:"id,omitempty"`
	Title        string       `json:"title"`
	CompanyName  string       `json:"company_name,omitempty"`
	CompanyURL   string       `json:"company_url,omitempty"`
	JobURL       string       `json:"job_url"`
	Location     string       `json:"location,omitempty"`
	IsRemote     bool         `json:"is_remote,omitempty"`
	Description  string       `json:"description,omitempty"`
	JobType      string       `json:"job_type,omitempty"`
	DatePosted   *time.Time   `json:"date_posted,omitempty"`
	Site         string       `json:"site"`
	Seniority    string       `json:"seniority,omitempty"`
	Department   string       `json:"department,omitempty"`
	Compensation *Compensation `json:"compensation,omitempty"`
	Skills       []string     `json:"skills,omitempty"`
	Emails       []EmailEntry `json:"emails,omitempty"`
	QualityScore int          `json:"quality_score,omitempty"`
}

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

// SalaryRange returns a human-readable salary string.
func (j *JobResult) SalaryRange() string {
	if j.Compensation == nil {
		return ""
	}
	c := j.Compensation
	if c.MinAmount == nil && c.MaxAmount == nil {
		return ""
	}
	curr := c.Currency
	if curr == "" {
		curr = "USD"
	}
	if c.MinAmount != nil && c.MaxAmount != nil && *c.MinAmount == *c.MaxAmount {
		return fmt.Sprintf("%s%.0f", curr, *c.MinAmount)
	}
	if c.MinAmount != nil && c.MaxAmount != nil {
		return fmt.Sprintf("%s%.0f - %s%.0f", curr, *c.MinAmount, curr, *c.MaxAmount)
	}
	if c.MinAmount != nil {
		return fmt.Sprintf("From %s%.0f", curr, *c.MinAmount)
	}
	return fmt.Sprintf("Up to %s%.0f", curr, *c.MaxAmount)
}

// SkillsJoined returns skills as a comma-separated string.
func (j *JobResult) SkillsJoined() string {
	return strings.Join(j.Skills, ", ")
}

type Scraper struct {
	config Config
}

func New(cfg Config) *Scraper {
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "scrappy"
	}
	return &Scraper{config: cfg}
}

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
	var jobs []JobResult
	if err := json.Unmarshal(output, &jobs); err != nil {
		return nil, fmt.Errorf("parse scrappy output: %w", err)
	}
	return jobs, nil
}

func (s *Scraper) buildArgs() []string {
	var args []string

	// Config path first
	if s.config.ConfigPath != "" {
		args = append(args, "--config", s.config.ConfigPath)
	}

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

	// Hours old filter (new in scrappy)
	if s.config.HoursOld > 0 {
		args = append(args, "--hours-old", fmt.Sprintf("%d", s.config.HoursOld))
	}

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

	// Non-interactive mode
	args = append(args, "--non-interactive")

	return args
}
