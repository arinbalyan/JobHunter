package config_test

import (
	"os"
	"testing"

	"github.com/arinbalyan/jobhunter/internal/config"
)

func TestLoad_HappyPath(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	os.Setenv("GMAIL_USER", "test@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "test-pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	os.Setenv("JOB_SEARCH_TERMS", "golang,backend,rust")
	os.Setenv("JOB_LOCATIONS", "remote")
	os.Setenv("CONTACT_NAME", "Test User")
	os.Setenv("USER_YEARS_EXPERIENCE", "5")
	os.Setenv("EMAIL_DELAY_SECONDS", "60")
	os.Setenv("JOB_REMOTE_ONLY", "true")
	os.Setenv("LLM_MAX_TOKENS_PER_RUN", "500000")
	os.Setenv("TRACKING_SERVER_PORT", "9090")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	if cfg.DatabaseURL != "postgres://user:pass@localhost/db" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://user:pass@localhost/db")
	}
	if cfg.GmailUser != "test@gmail.com" {
		t.Errorf("GmailUser = %q, want %q", cfg.GmailUser, "test@gmail.com")
	}
	if cfg.GmailAppPass != "test-pass" {
		t.Errorf("GmailAppPass = %q, want %q", cfg.GmailAppPass, "test-pass")
	}
	if cfg.ContactName != "Test User" {
		t.Errorf("ContactName = %q, want %q", cfg.ContactName, "Test User")
	}
	if cfg.UserYearsExperience != 5 {
		t.Errorf("UserYearsExperience = %d, want 5", cfg.UserYearsExperience)
	}
	if cfg.EmailDelaySeconds != 60 {
		t.Errorf("EmailDelaySeconds = %d, want 60", cfg.EmailDelaySeconds)
	}
	if cfg.EmailDelay.Seconds() != 60 {
		t.Errorf("EmailDelay = %v, want 60s", cfg.EmailDelay)
	}
	if !cfg.JobRemoteOnly {
		t.Error("JobRemoteOnly = false, want true")
	}
	if cfg.MaxTokensPerRun != 500000 {
		t.Errorf("MaxTokensPerRun = %d, want 500000", cfg.MaxTokensPerRun)
	}
	if cfg.TrackingServerPort != 9090 {
		t.Errorf("TrackingServerPort = %d, want 9090", cfg.TrackingServerPort)
	}
	if len(cfg.JobSearchTerms) != 3 {
		t.Errorf("len(JobSearchTerms) = %d, want 3; got %v", len(cfg.JobSearchTerms), cfg.JobSearchTerms)
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	os.Setenv("GMAIL_USER", "test@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "test-pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	os.Setenv("JOB_SEARCH_TERMS", "golang")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.TrackingServerPort != 8080 {
		t.Errorf("default TrackingServerPort = %d, want 8080", cfg.TrackingServerPort)
	}
	if cfg.EmailDelaySeconds != 30 {
		t.Errorf("default EmailDelaySeconds = %d, want 30", cfg.EmailDelaySeconds)
	}
	if cfg.MaxEmailsPerRun != 10 {
		t.Errorf("default MaxEmailsPerRun = %d, want 10", cfg.MaxEmailsPerRun)
	}
	if cfg.JobResultsPerSite != 25 {
		t.Errorf("default JobResultsPerSite = %d, want 25", cfg.JobResultsPerSite)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.MaxTokensPerRequest != 2048 {
		t.Errorf("default MaxTokensPerRequest = %d, want 2048", cfg.MaxTokensPerRequest)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Clearenv()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() should not fail on missing env: %v", err)
	}

	err = cfg.Validate()
	if err == nil {
		t.Fatal("Validate() should fail when all required fields are missing")
	}
}

func TestLoad_IntParsingEdgeCases(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	os.Setenv("GMAIL_USER", "test@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "test-pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	os.Setenv("JOB_SEARCH_TERMS", "golang")

	// Non-numeric env vars should fall back to defaults
	os.Setenv("EMAIL_DELAY_SECONDS", "not-a-number")
	os.Setenv("TRACKING_SERVER_PORT", "invalid")
	os.Setenv("USER_YEARS_EXPERIENCE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.EmailDelaySeconds != 30 {
		t.Errorf("EmailDelaySeconds = %d, want default 30", cfg.EmailDelaySeconds)
	}
	if cfg.TrackingServerPort != 8080 {
		t.Errorf("TrackingServerPort = %d, want default 8080", cfg.TrackingServerPort)
	}
	if cfg.UserYearsExperience != 0 {
		t.Errorf("UserYearsExperience = %d, want 0", cfg.UserYearsExperience)
	}
}

func TestLoad_BoolParsingEdgeCases(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	os.Setenv("GMAIL_USER", "test@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "test-pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	os.Setenv("JOB_SEARCH_TERMS", "golang")

	// Invalid bool values should fall back to default
	os.Setenv("JOB_REMOTE_ONLY", "not-a-bool")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.JobRemoteOnly != true {
		t.Errorf("JobRemoteOnly = %v, want default true", cfg.JobRemoteOnly)
	}
}

func TestValidate_HappyPath(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	os.Setenv("GMAIL_USER", "test@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "test-pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	os.Setenv("JOB_SEARCH_TERMS", "golang")

	cfg, _ := config.Load()
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}
}

func TestValidate_AllErrors(t *testing.T) {
	os.Clearenv()
	os.Setenv("JOB_SEARCH_TERMS", "golang") // Only set search terms

	cfg, _ := config.Load()
	cfg.JobSearchTerms = nil // Clear to trigger all errors

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() should fail")
	}

	errStr := err.Error()
	// Should contain all missing field errors
	if !containsSub(errStr, "DATABASE_URL") {
		t.Error("expected DATABASE_URL error")
	}
	if !containsSub(errStr, "GMAIL_USER") {
		t.Error("expected GMAIL_USER error")
	}
	if !containsSub(errStr, "JOB_SEARCH_TERM") {
		t.Error("expected JOB_SEARCH_TERM error")
	}
	if !containsSub(errStr, "LLM provider") {
		t.Error("expected LLM provider error")
	}
}

func TestParseCommaList(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a,b,c", 3},
		{"a, b, c", 3},
		{"", 0},
		{"single", 1},
		{",,,", 0},
		{"a,,b,,c", 3},
	}

	for _, tt := range tests {
		result := cfgParseCommaList(tt.input)
		if len(result) != tt.want {
			t.Errorf("parseCommaList(%q) = %d items, want %d; got %v", tt.input, len(result), tt.want, result)
		}
	}
}

func TestParseJSONList(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{`["a","b","c"]`, 3},
		{`["a", "b"]`, 2},
		{"[]", 0},
		{"", 0},
		{`["single"]`, 1},
	}

	for _, tt := range tests {
		result := cfgParseJSONList(tt.input)
		if len(result) != tt.want {
			t.Errorf("parseJSONList(%q) = %d items, want %d; got %v", tt.input, len(result), tt.want, result)
		}
	}
}

func TestGetActiveProviders_OpenRouterOnly(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	os.Setenv("GMAIL_USER", "test@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "test-pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	os.Setenv("JOB_SEARCH_TERMS", "golang")

	cfg, _ := config.Load()
	providers := cfg.GetActiveProviders()

	if len(providers) == 0 {
		t.Fatal("GetActiveProviders() returned empty")
	}

	// Should have OpenRouter entries
	found := false
	for _, p := range providers {
		if p.Kind == "openrouter" {
			found = true
			if p.APIKey != "sk-or-v1-test" {
				t.Errorf("OpenRouter APIKey = %q, want sk-or-v1-test", p.APIKey)
			}
			break
		}
	}
	if !found {
		t.Error("expected OpenRouter provider")
	}
}

func TestGetActiveProviders_Multiple(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	os.Setenv("GMAIL_USER", "test@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "test-pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	os.Setenv("GROQ_API_KEY", "gsk-test")
	os.Setenv("TOGETHER_API_KEY", "tog-test")
	os.Setenv("JOB_SEARCH_TERMS", "golang")

	cfg, _ := config.Load()
	providers := cfg.GetActiveProviders()

	// Should have openrouter + groq + together
	kinds := make(map[string]int)
	for _, p := range providers {
		kinds[p.Kind]++
	}

	if kinds["openrouter"] == 0 {
		t.Error("expected openrouter provider")
	}
	if kinds["groq"] == 0 {
		t.Error("expected groq provider")
	}
	if kinds["together"] == 0 {
		t.Error("expected together provider")
	}
}

func TestGetActiveProviders_Empty(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	os.Setenv("GMAIL_USER", "test@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "test-pass")
	os.Setenv("JOB_SEARCH_TERMS", "golang")

	cfg, _ := config.Load()
	providers := cfg.GetActiveProviders()

	if len(providers) != 0 {
		t.Errorf("expected 0 providers with no API keys, got %d", len(providers))
	}
}

func TestConfig_TelegramAccessors(t *testing.T) {
	cfg := &config.Config{}
	if cfg.TelegramToken() != "" {
		t.Error("expected empty TelegramToken")
	}
	if cfg.TelegramChat() != "" {
		t.Error("expected empty TelegramChat")
	}

	cfg.TelegramBotToken = "bot123:abc"
	cfg.TelegramChatID = "-12345"

	if cfg.TelegramToken() != "bot123:abc" {
		t.Errorf("TelegramToken() = %q, want %q", cfg.TelegramToken(), "bot123:abc")
	}
	if cfg.TelegramChat() != "-12345" {
		t.Errorf("TelegramChat() = %q, want %q", cfg.TelegramChat(), "-12345")
	}
}

// Helper functions that access unexported functions via the public API
func cfgParseCommaList(s string) []string {
	os.Clearenv()
	os.Setenv("JOB_SEARCH_TERMS", s)
	os.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
	os.Setenv("GMAIL_USER", "t@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	cfg, _ := config.Load()
	return cfg.JobSearchTerms
}

func cfgParseJSONList(s string) []string {
	os.Clearenv()
	os.Setenv("USER_TARGET_ROLES", s)
	os.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
	os.Setenv("GMAIL_USER", "t@gmail.com")
	os.Setenv("GMAIL_APP_PASS", "pass")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	os.Setenv("JOB_SEARCH_TERMS", "golang")
	cfg, _ := config.Load()
	return cfg.UserTargetRoles
}

func containsSub(s, sub string) bool {
	return len(s) >= len(sub) && containsStr(s, sub)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
