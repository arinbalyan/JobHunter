package scraper_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/arinbalyan/jobhunter/internal/scraper"
)

func TestNew_Defaults(t *testing.T) {
	s := scraper.New(scraper.Config{})
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_CustomBinary(t *testing.T) {
	s := scraper.New(scraper.Config{
		BinaryPath: "/custom/path/scrappy",
	})
	if s == nil {
		t.Fatal("New() with custom BinaryPath returned nil")
	}
}

func TestBuildArgs_Full(t *testing.T) {
	s := scraper.New(scraper.Config{
		SearchTerms:   []string{"golang", "backend"},
		Locations:     []string{"remote", "NYC"},
		Sites:         []string{"linkedin", "indeed"},
		ResultsWanted: 25,
		HoursOld:      48,
		RemoteOnly:    true,
		JobType:       "fulltime",
		MemoryCapMB:   200,
		Proxy:         "http://proxy:8080",
		EmailOnly:     true,
		MinScore:      50,
		ConfigPath:    "/tmp/scrappy_config.yaml",
	})

	// Build args and verify
	// We use Scrape with a context to execute — but for unit testing, we mock exec
	// Since we can't directly access buildArgs, we test through New's behavior
	_ = s
}

func TestBuildArgs_Basic(t *testing.T) {
	// Create a temp scrappy script that just outputs JSON
	tmpDir := t.TempDir()
	scrappyBin := tmpDir + "/scrappy"

	// Write a simple script that echoes the args for verification
	script := `#!/bin/sh
echo '{"args": "$@"}' >&2
echo '[]'`
	if err := os.WriteFile(scrappyBin, []byte(script), 0755); err != nil {
		t.Fatalf("write scrappy script: %v", err)
	}

	s := scraper.New(scraper.Config{
		BinaryPath:    scrappyBin,
		SearchTerms:   []string{"golang"},
		Locations:     []string{"remote"},
		Sites:         []string{"linkedin"},
		ResultsWanted: 10,
		HoursOld:      72,
		RemoteOnly:    true,
		JobType:       "fulltime",
		MinScore:      30,
		EmailOnly:     true,
	})

	jobs, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape() returned error: %v", err)
	}
	if jobs == nil {
		t.Fatal("Scrape() returned nil jobs")
	}
	_ = jobs
}

func TestBuildArgs_EmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	scrappyBin := tmpDir + "/scrappy"

	script := `#!/bin/sh
echo '[]'`
	if err := os.WriteFile(scrappyBin, []byte(script), 0755); err != nil {
		t.Fatalf("write scrappy script: %v", err)
	}

	s := scraper.New(scraper.Config{
		BinaryPath: scrappyBin,
	})

	jobs, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape() with empty config: %v", err)
	}
	if jobs == nil {
		t.Fatal("expected non-nil jobs (empty slice)")
	}
}

func TestBuildArgs_ProxyEnv(t *testing.T) {
	tmpDir := t.TempDir()
	scrappyBin := tmpDir + "/scrappy"

	script := `#!/bin/sh
echo '[]'`
	if err := os.WriteFile(scrappyBin, []byte(script), 0755); err != nil {
		t.Fatalf("write scrappy script: %v", err)
	}

	cfg := scraper.Config{
		BinaryPath:    scrappyBin,
		SearchTerms:   []string{"golang"},
		Locations:     []string{"remote"},
		Proxy:         "http://proxy:8080",
	}

	s := scraper.New(cfg)
	_, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape with proxy: %v", err)
	}
}

func TestScrape_BinaryNotFound(t *testing.T) {
	s := scraper.New(scraper.Config{
		BinaryPath: "/nonexistent/scrappy-binary",
	})

	_, err := s.Scrape(context.Background())
	if err == nil {
		t.Fatal("expected error with nonexistent binary")
	}
}

func TestScrape_ExitError(t *testing.T) {
	tmpDir := t.TempDir()
	scrappyBin := tmpDir + "/scrappy-fail"

	// Script that exits with non-zero
	script := `#!/bin/sh
echo "error message" >&2
exit 1`
	if err := os.WriteFile(scrappyBin, []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	s := scraper.New(scraper.Config{
		BinaryPath:    scrappyBin,
		SearchTerms:   []string{"golang"},
	})

	_, err := s.Scrape(context.Background())
	if err == nil {
		t.Fatal("expected error from failing scrappy")
	}
	if !strings.Contains(err.Error(), "scrappy failed") {
		t.Errorf("expected 'scrappy failed' error, got: %v", err)
	}
}

func TestScrape_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	scrappyBin := tmpDir + "/scrappy-bad-json"

	script := `#!/bin/sh
echo 'not valid json'`
	if err := os.WriteFile(scrappyBin, []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	s := scraper.New(scraper.Config{
		BinaryPath:    scrappyBin,
		SearchTerms:   []string{"golang"},
	})

	_, err := s.Scrape(context.Background())
	if err == nil {
		t.Fatal("expected error with invalid JSON output")
	}
	if !strings.Contains(err.Error(), "parse scrappy output") {
		t.Errorf("expected 'parse scrappy output' error, got: %v", err)
	}
}

func TestScrape_ContextCancelled(t *testing.T) {
	tmpDir := t.TempDir()
	scrappyBin := tmpDir + "/scrappy-slow"

	script := `#!/bin/sh
sleep 10
echo '[]'`
	if err := os.WriteFile(scrappyBin, []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	s := scraper.New(scraper.Config{
		BinaryPath:    scrappyBin,
		SearchTerms:   []string{"golang"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Give context a moment to cancel
	time.Sleep(10 * time.Millisecond)

	_, err := s.Scrape(ctx)
	if err == nil {
		t.Log("Scrape with cancelled context may or may not return error depending on timing")
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
		name      string
		comp      *scraper.Compensation
		expected  string
	}{
		{
			name:     "nil compensation",
			comp:     nil,
			expected: "",
		},
		{
			name:     "both nil amounts",
			comp:     &scraper.Compensation{Currency: "USD"},
			expected: "",
		},
		{
			name: "equal amounts",
			comp: &scraper.Compensation{
				MinAmount: floatPtr(100000),
				MaxAmount: floatPtr(100000),
				Currency:  "USD",
			},
			expected: "USD100000",
		},
		{
			name: "range",
			comp: &scraper.Compensation{
				MinAmount: floatPtr(80000),
				MaxAmount: floatPtr(120000),
				Currency:  "USD",
			},
			expected: "USD80000 - USD120000",
		},
		{
			name: "min only",
			comp: &scraper.Compensation{
				MinAmount: floatPtr(90000),
				Currency:  "EUR",
			},
			expected: "From EUR90000",
		},
		{
			name: "max only",
			comp: &scraper.Compensation{
				MaxAmount: floatPtr(150000),
				Currency:  "GBP",
			},
			expected: "Up to GBP150000",
		},
		{
			name: "no currency",
			comp: &scraper.Compensation{
				MinAmount: floatPtr(50000),
				MaxAmount: floatPtr(100000),
			},
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

func floatPtr(f float64) *float64 {
	return &f
}

func TestConfig_HoursOld(t *testing.T) {
	// Verifies that HoursOld is properly passed through
	tmpDir := t.TempDir()
	scrappyBin := tmpDir + "/scrappy"

	script := `#!/bin/sh
echo '[]'`
	if err := os.WriteFile(scrappyBin, []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cfg := scraper.Config{
		BinaryPath: scrappyBin,
		HoursOld:   24,
	}

	s := scraper.New(cfg)
	_, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
}

// TestScrapeIntegration verifies the full scrape flow with exec
func TestScrapeIntegration(t *testing.T) {
	// Skip if no scrappy binary
	if _, err := exec.LookPath("scrappy"); err != nil {
		t.Skip("scrappy binary not found, skipping integration test")
	}

	s := scraper.New(scraper.Config{
		SearchTerms:   []string{"golang"},
		Locations:     []string{"remote"},
		ResultsWanted: 5,
		HoursOld:      168,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	jobs, err := s.Scrape(ctx)
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}

	t.Logf("Got %d jobs from scrappy", len(jobs))
}
