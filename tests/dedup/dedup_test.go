package dedup_test

import (
	"testing"

	"github.com/arinbalyan/jobhunter/internal/dedup"
)

func TestIsRejectedTitle(t *testing.T) {
	tests := []struct {
		title    string
		patterns []string
		want     bool
	}{
		{title: "Senior Software Engineer", patterns: []string{"teacher", "nurse"}, want: false},
		{title: "Math Teacher", patterns: []string{"teacher", "nurse"}, want: true},
		{title: "Registered Nurse", patterns: []string{"teacher", "nurse"}, want: true},
		{title: "Cashier", patterns: []string{"cashier", "retail"}, want: true},
		{title: "Software Engineer Intern", patterns: []string{"intern"}, want: true},
		{title: "Senior Golang Developer", patterns: []string{"teacher", "nurse", "cashier"}, want: false},
		{title: "Head of Engineering", patterns: []string{"teacher", "nurse"}, want: false},
	}

	for _, tt := range tests {
		got := dedup.IsRejectedTitle(tt.title, tt.patterns)
		if got != tt.want {
			t.Errorf("IsRejectedTitle(%q, %v) = %v, want %v", tt.title, tt.patterns, got, tt.want)
		}
	}
}

func TestFilterEmail(t *testing.T) {
	tests := []struct {
		email    string
		patterns []string
		want     bool // true = should be filtered (rejected)
	}{
		{email: "hr@company.com", patterns: []string{}, want: false},
		{email: "accommodation@company.com", patterns: []string{"starts_with:accommodation@"}, want: true},
		{email: "hr@company.com", patterns: []string{"starts_with:accommodation@"}, want: false},
		{email: "no-reply@company.com", patterns: []string{"contains:no-reply"}, want: true},
		{email: "noreply@company.com", patterns: []string{"contains:noreply"}, want: true},
		{email: "careers@company.tk", patterns: []string{"tld:.tk"}, want: true},
		{email: "careers@company.com", patterns: []string{"tld:.tk", "tld:.ml"}, want: false},
		{email: "accessibility@company.com", patterns: []string{"contains:accessibility"}, want: true},
	}

	for _, tt := range tests {
		got := dedup.FilterEmail(tt.email, tt.patterns)
		if got != tt.want {
			t.Errorf("FilterEmail(%q, %v) = %v, want %v", tt.email, tt.patterns, got, tt.want)
		}
	}
}

func TestFilterEmails(t *testing.T) {
	emails := []string{
		"hr@company.com",
		"no-reply@company.com",
		"careers@company.com",
		"accommodation@company.com",
	}
	patterns := []string{"contains:no-reply", "starts_with:accommodation@"}

	valid, invalid := dedup.FilterEmails(emails, patterns)
	if len(valid) != 2 {
		t.Errorf("expected 2 valid emails, got %d: %v", len(valid), valid)
	}
	if len(invalid) != 2 {
		t.Errorf("expected 2 invalid emails, got %d: %v", len(invalid), invalid)
	}
}
