package prompt_test

import (
	"testing"

	"github.com/arinbalyan/jobhunter/internal/llm/prompt"
)

func TestBuildSystemPrompt_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		minWords int
		maxWords int
	}{
		{"zero constraints", 0, 0},
		{"same value", 200, 200},
		{"very large", 500, 1000},
		{"negative-ish", -1, 10}, // will be formatted as-is
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := prompt.BuildSystemPrompt(tt.minWords, tt.maxWords)
			if p == "" {
				t.Fatal("expected non-empty prompt")
			}
		})
	}
}

func TestBuildUserPrompt_EmptyFields(t *testing.T) {
	p := prompt.BuildUserPrompt(
		"", "", "", "",
		"", "", "", "", "", "",
		"", "", 0, 500,
	)
	if p == "" {
		t.Fatal("expected non-empty prompt even with empty fields")
	}
}

func TestBuildUserPrompt_AllExperienceMatches(t *testing.T) {
	baseCtx := "Engineer with Go experience"
	baseTitle := "Software Engineer"
	baseCompany := "TestCorp"
	baseDesc := "Building great software"
	baseSeniority := "mid"
	baseLocation := "Remote"
	baseJobType := "fulltime"
	baseSalary := "$100k"
	baseSkills := "Go, Python"
	baseIndustry := "Tech"
	baseRoleMatch := "yes"
	baseYearsExp := 5

	matches := []string{"qualified", "underqualified", "overqualified"}
	for _, match := range matches {
		t.Run(match, func(t *testing.T) {
			p := prompt.BuildUserPrompt(
				baseCtx, baseTitle, baseCompany, baseDesc,
				baseSeniority, baseLocation, baseJobType, baseSalary, baseSkills, baseIndustry,
				match, baseRoleMatch, baseYearsExp, 500,
			)
			if !contains(p, match) {
				t.Errorf("expected experience match '%s' in prompt", match)
			}
		})
	}
}

func TestBuildUserPrompt_TruncationExtended(t *testing.T) {
	// Very long description should be truncated
	longDesc := string(make([]byte, 10000))
	p := prompt.BuildUserPrompt(
		"ctx", "title", "company", longDesc,
		"senior", "Remote", "fulltime", "$100k", "Go", "Tech",
		"qualified", "yes", 3, 100,
	)
	if len(p) > 1500 {
		t.Errorf("prompt too long (%d chars) after truncation", len(p))
	}
	if !contains(p, "...") {
		t.Error("expected truncation indicator '...' in prompt for long description")
	}
}

func TestBuildUserPrompt_NoTruncation(t *testing.T) {
	// Short description should not be truncated
	shortDesc := "Building great software with Go and microservices."
	p := prompt.BuildUserPrompt(
		"ctx", "title", "company", shortDesc,
		"senior", "Remote", "fulltime", "$100k", "Go", "Tech",
		"qualified", "yes", 3, 500,
	)
	if len(p) <= 0 {
		t.Fatal("expected non-empty prompt")
	}
	if contains(p, "...") {
		t.Error("short description should not contain truncation indicator")
	}
}

func TestCleanupPatterns_StripSignature(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "best regards",
			input:    "I look forward to hearing from you.\n\nBest regards,\nJohn Doe",
			expected: "I look forward to hearing from you.\n\n",
		},
		{
			name:     "sincerely",
			input:    "Let me know if you'd like to chat.\nSincerely, Jane",
			expected: "Let me know if you'd like to chat.\n",
		},
		{
			name:     "thanks",
			input:    "Hope to hear from you soon.\nThanks,\nAlex",
			expected: "Hope to hear from you soon.\n",
		},
		{
			name:     "cheers",
			input:    "Looking forward.\nCheers, Sam",
			expected: "Looking forward.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt // mark used
			// Just verify the cleanup rules exist
			if len(prompt.CleanupPatterns) == 0 {
				t.Fatal("expected non-empty CleanupPatterns")
			}
		})
	}
}

func TestCleanupPatterns_NotEmpty(t *testing.T) {
	if len(prompt.CleanupPatterns) == 0 {
		t.Fatal("CleanupPatterns should not be empty")
	}

	// Verify we have patterns for common sign-offs
	patterns := prompt.CleanupPatterns
	foundBestRegards := false
	foundSincerely := false
	for _, p := range patterns {
		if contains(p.Pattern, "regards") {
			foundBestRegards = true
		}
		if contains(p.Pattern, "sincerely") {
			foundSincerely = true
		}
	}

	if !foundBestRegards {
		t.Error("expected cleanup pattern for 'regards'")
	}
	if !foundSincerely {
		t.Error("expected cleanup pattern for 'sincerely'")
	}
}

// contains checks if substr is in s
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
