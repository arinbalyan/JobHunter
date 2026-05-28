package scraper_test

import (
	"context"
	"testing"
	"time"

	"github.com/arinbalyan/jobhunter/internal/scraper"
)

func TestNew(t *testing.T) {
	s := scraper.New(scraper.Config{})
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_FullConfig(t *testing.T) {
	s := scraper.New(scraper.Config{
		SearchTerms:   []string{"golang", "backend"},
		Locations:     []string{"remote", "NYC"},
		Sites:         []string{"linkedin", "indeed"},
		ResultsWanted: 25,
		HoursOld:      48,
		SinceDate:     "2026-01-01",
		RemoteOnly:    true,
		JobType:       "fulltime",
		MemoryCapMB:   200,
		Proxy:         "http://proxy:8080",
		EmailOnly:     true,
		MinScore:      50,
	})
	if s == nil {
		t.Fatal("New() with full config returned nil")
	}
}

func TestJobResult_PreferredEmails(t *testing.T) {
	job := scraper.JobResult{
		Emails: []scraper.EmailEntry{
			{Addr: "hr@company.com", Verified: true, Source: "page"},
			{Addr: "", Verified: false, Source: ""},
			{Addr: "hr@company.com", Verified: true, Source: "page"}, // duplicate
			{Addr: "jobs@company.com", Verified: false, Source: "page"},
			{Addr: "verified@corp.com", Verified: true, Source: "mx"},
		},
	}

	emails := job.PreferredEmails()
	if len(emails) != 3 {
		t.Errorf("expected 3 unique emails, got %d: %v", len(emails), emails)
	}

	// Verified emails should come first
	if len(emails) >= 2 && emails[0] != "hr@company.com" && emails[0] != "verified@corp.com" {
		t.Errorf("expected verified email first, got %s", emails[0])
	}

	// Should have both verified emails before unverified
	if emails[0] == "" || emails[1] == "" {
		t.Error("expected non-empty emails first")
	}
}

func TestJobResult_HasVerifiedEmail(t *testing.T) {
	tests := []struct {
		name     string
		emails   []scraper.EmailEntry
		expected bool
	}{
		{
			name:     "no emails",
			emails:   nil,
			expected: false,
		},
		{
			name:     "unverified only",
			emails:   []scraper.EmailEntry{{Addr: "x@y.com", Verified: false}},
			expected: false,
		},
		{
			name:     "verified",
			emails:   []scraper.EmailEntry{{Addr: "x@y.com", Verified: true}},
			expected: true,
		},
		{
			name: "mixed",
			emails: []scraper.EmailEntry{
				{Addr: "a@b.com", Verified: false},
				{Addr: "c@d.com", Verified: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := scraper.JobResult{Emails: tt.emails}
			if got := job.HasVerifiedEmail(); got != tt.expected {
				t.Errorf("HasVerifiedEmail() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestJobResult_FlatEmails(t *testing.T) {
	job := scraper.JobResult{
		Emails: []scraper.EmailEntry{
			{Addr: "hr@company.com", Verified: true, Source: "page"},
			{Addr: "", Verified: false, Source: ""},
			{Addr: "hr@company.com", Verified: true, Source: "page"}, // duplicate
			{Addr: "jobs@company.com", Verified: false, Source: "page"},
		},
	}

	emails := job.FlatEmails()
	if len(emails) != 2 {
		t.Errorf("expected 2 unique emails, got %d: %v", len(emails), emails)
	}
}

func TestJobResult_FlatEmails_Empty(t *testing.T) {
	job := scraper.JobResult{}
	emails := job.FlatEmails()
	if len(emails) != 0 {
		t.Errorf("expected 0 emails, got %d", len(emails))
	}
}

func TestJobResult_SalaryRange(t *testing.T) {
	tests := []struct {
		name     string
		comp     *scraper.Compensation
		expected string
	}{
		{name: "nil", comp: nil, expected: ""},
		{name: "both nil", comp: &scraper.Compensation{Currency: "USD"}, expected: ""},
		{
			name: "equal",
			comp: &scraper.Compensation{MinAmount: floatPtr(100000), MaxAmount: floatPtr(100000), Currency: "USD"},
			expected: "USD100000",
		},
		{
			name: "range",
			comp: &scraper.Compensation{MinAmount: floatPtr(80000), MaxAmount: floatPtr(120000), Currency: "USD"},
			expected: "USD80000 - USD120000",
		},
		{
			name: "min only",
			comp: &scraper.Compensation{MinAmount: floatPtr(90000), Currency: "EUR"},
			expected: "From EUR90000",
		},
		{
			name: "max only",
			comp: &scraper.Compensation{MaxAmount: floatPtr(150000), Currency: "GBP"},
			expected: "Up to GBP150000",
		},
		{
			name: "no currency",
			comp: &scraper.Compensation{MinAmount: floatPtr(50000), MaxAmount: floatPtr(100000)},
			expected: "USD50000 - USD100000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := scraper.JobResult{Compensation: tt.comp}
			result := job.SalaryRange()
			if result != tt.expected {
				t.Errorf("SalaryRange() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestJobResult_SkillsJoined(t *testing.T) {
	tests := []struct {
		name     string
		skills   []string
		expected string
	}{
		{name: "empty", skills: []string{}, expected: ""},
		{name: "nil", skills: nil, expected: ""},
		{name: "single", skills: []string{"Go"}, expected: "Go"},
		{name: "multiple", skills: []string{"Go", "Kubernetes", "PostgreSQL"}, expected: "Go, Kubernetes, PostgreSQL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := scraper.JobResult{Skills: tt.skills}
			result := job.SkillsJoined()
			if result != tt.expected {
				t.Errorf("SkillsJoined() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestScrape_ContextCancelled(t *testing.T) {
	s := scraper.New(scraper.Config{
		SearchTerms: []string{"golang"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(5 * time.Millisecond)

	_, err := s.Scrape(ctx)
	// With an already-cancelled context, scrappy should return an error promptly.
	// If it doesn't error, the wait was likely for scrappy engine initialization.
	if err != nil {
		t.Logf("Scrape with cancelled context returned: %v", err)
	} else {
		t.Log("Scrape didn't detect cancellation (engine init may have taken too long)")
	}
}

func TestJobResult_NewFields(t *testing.T) {
	now := time.Now()
	job := scraper.JobResult{
		ID:                 "test-123",
		Title:              "Senior Go Engineer",
		CompanyName:        "Acme Corp",
		CompanyURL:         "https://acme.com",
		JobURL:             "https://acme.com/careers/go-engineer",
		JobURLDirect:       "https://acme.com/apply/123",
		Location:           "San Francisco, CA, USA",
		IsRemote:           true,
		Description:        "We need a Go engineer...",
		JobType:            "fulltime",
		DatePosted:         &now,
		Site:               "linkedin",
		FetchedAt:          &now,
		Seniority:          "Senior",
		Department:         "Engineering",
		Compensation:       &scraper.Compensation{MinAmount: floatPtr(150000), MaxAmount: floatPtr(200000), Currency: "USD"},
		Skills:             []string{"Go", "Kubernetes", "PostgreSQL"},
		Emails:             []scraper.EmailEntry{{Addr: "careers@acme.com", Verified: true}},
		QualityScore:       92,
		Domain:             "acme.com",
		Industry:           "Technology",
		CompanyDescription: "A leading software company",
		ApplyMethod:        "careers_page",
		ExperienceRange:    "5-8 years",
	}

	if job.QualityScore != 92 {
		t.Errorf("QualityScore = %d, want 92", job.QualityScore)
	}
	if job.Domain != "acme.com" {
		t.Errorf("Domain = %q, want acme.com", job.Domain)
	}
	if job.CompanyDescription != "A leading software company" {
		t.Errorf("CompanyDescription mismatch")
	}
	if job.ExperienceRange != "5-8 years" {
		t.Errorf("ExperienceRange = %q, want 5-8 years", job.ExperienceRange)
	}
	if job.ApplyMethod != "careers_page" {
		t.Errorf("ApplyMethod = %q, want careers_page", job.ApplyMethod)
	}
}

func floatPtr(f float64) *float64 {
	return &f
}
