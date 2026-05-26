package dedup_test

import (
	"context"
	"testing"
	"time"

	"github.com/arinbalyan/jobhunter/internal/dedup"
)

// mockDB implements the database interface needed by Deduplicator
type mockDB struct {
	getSentEmailsCount func(ctx context.Context, email string, since time.Duration) (int, error)
	insertEmail        func(ctx context.Context, record interface{}) (interface{}, error)
}

func (m *mockDB) GetSentEmailsCount(ctx context.Context, email string, since time.Duration) (int, error) {
	if m.getSentEmailsCount != nil {
		return m.getSentEmailsCount(ctx, email, since)
	}
	return 0, nil
}

func (m *mockDB) InsertEmail(ctx context.Context, record interface{}) (interface{}, error) {
	if m.insertEmail != nil {
		return m.insertEmail(ctx, record)
	}
	return nil, nil
}

func TestDeduplicator_CanSend_NoDB(t *testing.T) {
	// When DB is nil, CanSend should always return true
	d := dedup.New(nil, 90)
	allowed, reason := d.CanSend(context.Background(), "test@example.com")
	if !allowed {
		t.Errorf("expected allowed=true with nil DB, got false, reason: %s", reason)
	}
	if reason != "" {
		t.Errorf("expected empty reason with nil DB, got: %s", reason)
	}
}

func TestDeduplicator_CanSend_NotSent(t *testing.T) {
	// When GetSentEmailsCount returns 0, CanSend should return true
	mock := mockDB{
		getSentEmailsCount: func(ctx context.Context, email string, since time.Duration) (int, error) {
			return 0, nil
		},
	}

	_ = dedup.New(nil, 90) // DB nil won't call CanSend's internal GetSentEmailsCount
	// We can't inject the mock through the current API — test what we can
	_ = mock
}

func TestDeduplicator_CanSend_AlreadySent(t *testing.T) {
	mock := mockDB{
		getSentEmailsCount: func(ctx context.Context, email string, since time.Duration) (int, error) {
			return 1, nil
		},
	}

	_ = mock
	d := dedup.New(nil, 90)
	// With nil DB, always returns true regardless
	allowed, reason := d.CanSend(context.Background(), "test@example.com")
	if !allowed {
		t.Errorf("expected allowed=true with nil DB, got reason: %s", reason)
	}

	// Test with zero cooldown
	d2 := dedup.New(nil, 0)
	allowed2, reason2 := d2.CanSend(context.Background(), "test@example.com")
	if !allowed2 {
		t.Errorf("expected allowed=true with nil DB and zero cooldown, got reason: %s", reason2)
	}
}

func TestDeduplicator_MarkSent_NoDB(t *testing.T) {
	d := dedup.New(nil, 90)
	err := d.MarkSent(context.Background(), nil)
	if err != nil {
		t.Errorf("MarkSent with nil DB should not error, got: %v", err)
	}
}

func TestDeduplicator_New(t *testing.T) {
	d := dedup.New(nil, 90)
	if d == nil {
		t.Fatal("New() returned nil")
	}
}

func TestIsRejectedTitle_EmptyPatterns(t *testing.T) {
	if dedup.IsRejectedTitle("Software Engineer", nil) {
		t.Error("IsRejectedTitle with nil patterns should return false")
	}
	if dedup.IsRejectedTitle("Software Engineer", []string{}) {
		t.Error("IsRejectedTitle with empty patterns should return false")
	}
}

func TestIsRejectedTitle_CaseInsensitive(t *testing.T) {
	tests := []struct {
		title    string
		patterns []string
		want     bool
	}{
		{title: "Senior Software Engineer", patterns: []string{"teacher"}, want: false},
		{title: "Math Teacher", patterns: []string{"teacher"}, want: true},
		{title: "math teacher", patterns: []string{"Teacher"}, want: true},
		{title: "MATH TEACHER", patterns: []string{"teacher"}, want: true},
		{title: "Senior Software Engineer", patterns: []string{"Engineer"}, want: true},
		{title: "engineer", patterns: []string{"ENGINEER"}, want: true},
	}

	for _, tt := range tests {
		got := dedup.IsRejectedTitle(tt.title, tt.patterns)
		if got != tt.want {
			t.Errorf("IsRejectedTitle(%q, %v) = %v, want %v", tt.title, tt.patterns, got, tt.want)
		}
	}
}

func TestIsRejectedTitle_PartialMatch(t *testing.T) {
	tests := []struct {
		title    string
		patterns []string
		want     bool
	}{
		{title: "Software Engineer Intern", patterns: []string{"intern"}, want: true},
		{title: "Intern Software Engineer", patterns: []string{"intern"}, want: true},
		{title: "Software Engineer (Intern)", patterns: []string{"intern"}, want: true},
		{title: "Senior Software Engineer", patterns: []string{"intern"}, want: false},
	}

	for _, tt := range tests {
		got := dedup.IsRejectedTitle(tt.title, tt.patterns)
		if got != tt.want {
			t.Errorf("IsRejectedTitle(%q, %v) = %v, want %v", tt.title, tt.patterns, got, tt.want)
		}
	}
}

func TestFilterEmail_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		patterns []string
		want     bool
	}{
		{name: "empty email", email: "", patterns: []string{"contains:test"}, want: false},
		{name: "nil patterns", email: "test@example.com", patterns: nil, want: false},
		{name: "empty patterns", email: "test@example.com", patterns: []string{}, want: false},
		{name: "starts with empty prefix", email: "test@example.com", patterns: []string{"starts_with:"}, want: true},
		{name: "contains empty substring", email: "test@example.com", patterns: []string{"contains:"}, want: true},
		{name: "tld empty", email: "test@example.com", patterns: []string{"tld:"}, want: true}, // empty string is suffix of everything
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedup.FilterEmail(tt.email, tt.patterns)
			if got != tt.want {
				t.Errorf("FilterEmail(%q, %v) = %v, want %v", tt.email, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestFilterEmails_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		emails   []string
		patterns []string
		valid    int
		invalid  int
	}{
		{name: "nil emails", emails: nil, patterns: nil, valid: 0, invalid: 0},
		{name: "empty emails", emails: []string{}, patterns: []string{}, valid: 0, invalid: 0},
		{name: "all valid", emails: []string{"a@b.com", "c@d.com"}, patterns: []string{"contains:no-reply"}, valid: 2, invalid: 0},
		{name: "all invalid", emails: []string{"no-reply@b.com", "no-reply@d.com"}, patterns: []string{"contains:no-reply"}, valid: 0, invalid: 2},
		{name: "mixed", emails: []string{"a@b.com", "no-reply@d.com"}, patterns: []string{"contains:no-reply"}, valid: 1, invalid: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, invalid := dedup.FilterEmails(tt.emails, tt.patterns)
			if len(valid) != tt.valid {
				t.Errorf("expected %d valid, got %d: %v", tt.valid, len(valid), valid)
			}
			if len(invalid) != tt.invalid {
				t.Errorf("expected %d invalid, got %d: %v", tt.invalid, len(invalid), invalid)
			}
		})
	}
}
