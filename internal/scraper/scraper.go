package scraper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arinbalyan/scrappy/pkg/scrappy"
)

// Config matches scrappy's CLI flags and library ScraperInput.
type Config struct {
	SearchTerms   []string
	Locations     []string
	Sites         []string
	ResultsWanted int
	HoursOld      int
	SinceDate     string   // new: YYYY-MM-DD or RFC3339
	RemoteOnly    bool
	JobType       string
	MemoryCapMB   int
	Proxy         string
	EmailOnly     bool
	MinScore      int
	BinaryPath    string // kept for backward compat, library ignores it
	ConfigPath    string // kept for backward compat, library ignores it
}

// Compensation holds salary data from scrappy's JSON output.
type Compensation = scrappy.Compensation

// EmailEntry mirrors scrappy's email struct.
type EmailEntry = scrappy.Email

// JobResult mirrors scrappy's public JobPost type exactly.
type JobResult struct {
	ID                 string             `json:"id,omitempty"`
	Title              string             `json:"title"`
	CompanyName        string             `json:"company_name,omitempty"`
	CompanyURL         string             `json:"company_url,omitempty"`
	JobURL             string             `json:"job_url"`
	JobURLDirect       string             `json:"job_url_direct,omitempty"`
	Location           string             `json:"location,omitempty"`
	IsRemote           bool               `json:"is_remote,omitempty"`
	Description        string             `json:"description,omitempty"`
	JobType            string             `json:"job_type,omitempty"`
	DatePosted         *time.Time         `json:"date_posted,omitempty"`
	Site               string             `json:"site"`
	FetchedAt          *time.Time         `json:"fetched_at,omitempty"`
	Seniority          string             `json:"seniority,omitempty"`
	Department         string             `json:"department,omitempty"`
	Compensation       *Compensation      `json:"compensation"`
	Skills             []string           `json:"skills"`
	Emails             []EmailEntry       `json:"emails,omitempty"`
	QualityScore       int                `json:"quality_score,omitempty"`
	Domain             string             `json:"domain,omitempty"`
	Industry           string             `json:"industry,omitempty"`
	CompanyDescription string             `json:"company_description,omitempty"`
	ApplyMethod        string             `json:"apply_method,omitempty"`
	ExperienceRange    string             `json:"experience_range,omitempty"`
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

// Scraper wraps the scrappy Go library.
type Scraper struct {
	engine  *scrappy.Engine
	config  Config
}

func New(cfg Config) *Scraper {
	return &Scraper{
		engine: scrappy.NewEngine(),
		config: cfg,
	}
}

// Scrape uses the scrappy Go library to scrape jobs.
func (s *Scraper) Scrape(ctx context.Context) ([]JobResult, error) {
	input := s.buildInput()
	jobs, err := s.engine.ScrapeJobs(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("scrappy: %w", err)
	}
	return jobPostsToResults(jobs), nil
}

func (s *Scraper) buildInput() scrappy.ScraperInput {
	input := scrappy.ScraperInput{
		Sites:        toSites(s.config.Sites),
		SearchTerms:  s.config.SearchTerms,
		Locations:    s.config.Locations,
		ResultsWanted: s.config.ResultsWanted,
		HoursOld:     s.config.HoursOld,
		SinceDate:    s.config.SinceDate,
		RemoteOnly:   s.config.RemoteOnly,
		JobType:      scrappy.JobType(s.config.JobType),
		EmailsOnly:   s.config.EmailOnly,
		MinScore:     s.config.MinScore,
		Proxy:        s.config.Proxy,
		MemoryCapMB:  s.config.MemoryCapMB,
	}
	if len(s.config.SearchTerms) == 1 {
		input.SearchTerm = s.config.SearchTerms[0]
	}
	if len(s.config.Locations) == 1 {
		input.Location = s.config.Locations[0]
	}
	return input
}

func toSites(names []string) []scrappy.Site {
	sites := make([]scrappy.Site, len(names))
	for i, n := range names {
		sites[i] = scrappy.Site(n)
	}
	return sites
}

func jobPostsToResults(jobs []scrappy.JobPost) []JobResult {
	out := make([]JobResult, len(jobs))
	for i, j := range jobs {
		loc := ""
		if j.Location.City != "" || j.Location.State != "" || j.Location.Country != "" {
			parts := []string{}
			if j.Location.City != "" {
				parts = append(parts, j.Location.City)
			}
			if j.Location.State != "" {
				parts = append(parts, j.Location.State)
			}
			if j.Location.Country != "" {
				parts = append(parts, j.Location.Country)
			}
			loc = strings.Join(parts, ", ")
		}
		out[i] = JobResult{
			ID:                 j.ID,
			Title:              j.Title,
			CompanyName:        j.CompanyName,
			CompanyURL:         j.CompanyURL,
			JobURL:             j.JobURL,
			JobURLDirect:       j.JobURLDirect,
			Location:           loc,
			IsRemote:           j.IsRemote,
			Description:        j.Description,
			JobType:            j.JobType,
			DatePosted:         j.DatePosted,
			Site:               string(j.Site),
			FetchedAt:          j.FetchedAt,
			Seniority:          j.Seniority,
			Department:         j.Department,
			Compensation:       (*Compensation)(j.Compensation),
			Skills:             j.Skills,
			Emails:             emailsToEntries(j.Emails),
			QualityScore:       j.QualityScore,
			Domain:             j.Domain,
			Industry:           j.Industry,
			CompanyDescription: j.CompanyDescription,
			ApplyMethod:        j.ApplyMethod,
			ExperienceRange:    j.ExperienceRange,
		}
	}
	return out
}

func emailsToEntries(emails []scrappy.Email) []EmailEntry {
	out := make([]EmailEntry, len(emails))
	for i, e := range emails {
		out[i] = EmailEntry(e)
	}
	return out
}
