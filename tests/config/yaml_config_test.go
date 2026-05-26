package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
)



func TestLoadYAML_FullConfig(t *testing.T) {
	yamlContent := `
user:
  name: "Test User"
  current_role: "Senior Go Developer"
  years_experience: 5
  email: "test@example.com"
  phone: "+1-555-0123"
  portfolio: "https://test.dev"
  github: "testuser"
  linkedin: "testuser-linkedin"
  codolio: "testuser-codolio"
  resume_drive_link: "https://drive.google.com/resume.pdf"
search:
  terms:
    - "golang developer"
    - "backend engineer"
  locations:
    - "remote"
    - "new york"
  sites:
    - "linkedin"
    - "indeed"
  remote_only: true
  job_type: "fulltime"
  results_per_site: 30
  hours_old: 48
  onsite:
    enabled: true
    terms:
      - "devops engineer"
    locations:
      - "san francisco"
    remote_only: false
    max_emails_per_day: 10
reject_titles:
  - "teacher"
  - "nurse"
  - "cashier"
  - "intern"
email_filters:
  - "contains:no-reply"
  - "tld:.tk"
  - "starts_with:accommodation@"
dedup:
  email_cooldown_days: 60
  domain_cooldown_hours: 12
  company_cooldown_hours: 48
email:
  max_per_run: 5
  delay_seconds: 60
  daily_limit: 200
  retry_attempts: 5
  retry_delay_seconds: 10
llm:
  complex_model: "test/complex-model:free"
  simple_model: "test/simple-model"
  max_tokens_per_run: 50000
  max_tokens_per_request: 4096
  temperature: 0.5
max_runtime_minutes: 300
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := config.LoadYAML(cfgPath)
	if err != nil {
		t.Fatalf("LoadYAML: %v", err)
	}

	// User config
	if cfg.User.Name != "Test User" {
		t.Errorf("Name = %q, want %q", cfg.User.Name, "Test User")
	}
	if cfg.User.CurrentRole != "Senior Go Developer" {
		t.Errorf("CurrentRole = %q, want %q", cfg.User.CurrentRole, "Senior Go Developer")
	}
	if cfg.User.YearsExperience != 5 {
		t.Errorf("YearsExperience = %d, want 5", cfg.User.YearsExperience)
	}
	if cfg.User.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", cfg.User.Email, "test@example.com")
	}
	if cfg.User.Phone != "+1-555-0123" {
		t.Errorf("Phone = %q, want %q", cfg.User.Phone, "+1-555-0123")
	}
	if cfg.User.Portfolio != "https://test.dev" {
		t.Errorf("Portfolio = %q, want %q", cfg.User.Portfolio, "https://test.dev")
	}
	if cfg.User.Github != "testuser" {
		t.Errorf("Github = %q, want %q", cfg.User.Github, "testuser")
	}
	if cfg.User.Linkedin != "testuser-linkedin" {
		t.Errorf("Linkedin = %q, want %q", cfg.User.Linkedin, "testuser-linkedin")
	}
	if cfg.User.Codolio != "testuser-codolio" {
		t.Errorf("Codolio = %q, want %q", cfg.User.Codolio, "testuser-codolio")
	}

	// Search config
	if len(cfg.Search.Terms) != 2 {
		t.Errorf("len(Terms) = %d, want 2", len(cfg.Search.Terms))
	}
	if len(cfg.Search.Locations) != 2 {
		t.Errorf("len(Locations) = %d, want 2", len(cfg.Search.Locations))
	}
	if len(cfg.Search.Sites) != 2 {
		t.Errorf("len(Sites) = %d, want 2", len(cfg.Search.Sites))
	}
	if !cfg.Search.RemoteOnly {
		t.Error("RemoteOnly = false, want true")
	}
	if cfg.Search.ResultsPerSite != 30 {
		t.Errorf("ResultsPerSite = %d, want 30", cfg.Search.ResultsPerSite)
	}
	if cfg.Search.HoursOld != 48 {
		t.Errorf("HoursOld = %d, want 48", cfg.Search.HoursOld)
	}

	// Onsite config
	if !cfg.Search.Onsite.Enabled {
		t.Error("Onsite.Enabled = false, want true")
	}
	if len(cfg.Search.Onsite.Terms) != 1 || cfg.Search.Onsite.Terms[0] != "devops engineer" {
		t.Errorf("Onsite.Terms = %v, want [devops engineer]", cfg.Search.Onsite.Terms)
	}

	// Reject titles
	if len(cfg.RejectTitles) != 4 {
		t.Errorf("len(RejectTitles) = %d, want 4", len(cfg.RejectTitles))
	}

	// Email filters
	if len(cfg.EmailFilters) != 3 {
		t.Errorf("len(EmailFilters) = %d, want 3", len(cfg.EmailFilters))
	}

	// Dedup
	if cfg.Dedup.EmailCooldownDays != 60 {
		t.Errorf("EmailCooldownDays = %d, want 60", cfg.Dedup.EmailCooldownDays)
	}
	if cfg.Dedup.DomainCooldownHours != 12 {
		t.Errorf("DomainCooldownHours = %d, want 12", cfg.Dedup.DomainCooldownHours)
	}
	if cfg.Dedup.CompanyCooldownHours != 48 {
		t.Errorf("CompanyCooldownHours = %d, want 48", cfg.Dedup.CompanyCooldownHours)
	}

	// Email config
	if cfg.Email.MaxPerRun != 5 {
		t.Errorf("MaxPerRun = %d, want 5", cfg.Email.MaxPerRun)
	}
	if cfg.Email.DelaySeconds != 60 {
		t.Errorf("DelaySeconds = %d, want 60", cfg.Email.DelaySeconds)
	}
	if cfg.Email.DailyLimit != 200 {
		t.Errorf("DailyLimit = %d, want 200", cfg.Email.DailyLimit)
	}
	if cfg.Email.RetryAttempts != 5 {
		t.Errorf("RetryAttempts = %d, want 5", cfg.Email.RetryAttempts)
	}
	if cfg.Email.RetryDelaySeconds != 10 {
		t.Errorf("RetryDelaySeconds = %d, want 10", cfg.Email.RetryDelaySeconds)
	}

	// LLM config
	if cfg.LLM.ComplexModel != "test/complex-model:free" {
		t.Errorf("ComplexModel = %q, want %q", cfg.LLM.ComplexModel, "test/complex-model:free")
	}
	if cfg.LLM.SimpleModel != "test/simple-model" {
		t.Errorf("SimpleModel = %q, want %q", cfg.LLM.SimpleModel, "test/simple-model")
	}
	if cfg.LLM.MaxTokensPerRun != 50000 {
		t.Errorf("MaxTokensPerRun = %d, want 50000", cfg.LLM.MaxTokensPerRun)
	}
	if cfg.LLM.MaxTokensPerReq != 4096 {
		t.Errorf("MaxTokensPerReq = %d, want 4096", cfg.LLM.MaxTokensPerReq)
	}
	if cfg.LLM.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", cfg.LLM.Temperature)
	}

	// Max runtime
	if cfg.MaxRuntime != 300 {
		t.Errorf("MaxRuntime = %d, want 300", cfg.MaxRuntime)
	}
}

func TestLoadYAML_ReadError(t *testing.T) {
	// Directory instead of file should produce an error
	_, err := config.LoadYAML(t.TempDir())
	if err == nil {
		t.Error("LoadYAML on a directory should return error")
	}
}

func TestMergeIntoConfig(t *testing.T) {
	// Create a YAML config with partial overrides
	yc := &config.YAMLConfig{
		User: config.UserConfig{
			Name:            "Override Name",
			YearsExperience: 8,
		},
		Search: config.SearchConfig{
			Terms:     []string{"data engineer"},
			Sites:     []string{"glassdoor"},
			RemoteOnly: true,
		},
		Email: config.EmailSendingConfig{
			MaxPerRun:    15,
			DelaySeconds: 45,
		},
		LLM: config.LLMConfig{
			ComplexModel:    "override/model",
			MaxTokensPerRun: 200000,
		},
	}

	// Create a base config with some values
	cfg := &config.Config{
		ContactName:     "Original Name",
		ContactGithub:   "original",
		JobSearchTerms:  []string{"original term"},
		JobSites:        []string{"original site"},
		MaxEmailsPerRun: 5,
		EmailDelaySeconds: 20,
		ComplexModel:    "original/model",
		MaxTokensPerRun: 50000,
	}

	yc.MergeIntoConfig(cfg)

	// Overridden values
	if cfg.ContactName != "Override Name" {
		t.Errorf("ContactName = %q, want %q", cfg.ContactName, "Override Name")
	}
	if cfg.UserYearsExperience != 8 {
		t.Errorf("UserYearsExperience = %d, want 8", cfg.UserYearsExperience)
	}
	if len(cfg.JobSearchTerms) != 1 || cfg.JobSearchTerms[0] != "data engineer" {
		t.Errorf("JobSearchTerms = %v, want [data engineer]", cfg.JobSearchTerms)
	}
	if cfg.JobRemoteOnly != true {
		t.Error("JobRemoteOnly = false, want true")
	}
	if cfg.MaxEmailsPerRun != 15 {
		t.Errorf("MaxEmailsPerRun = %d, want 15", cfg.MaxEmailsPerRun)
	}
	if cfg.EmailDelaySeconds != 45 {
		t.Errorf("EmailDelaySeconds = %d, want 45", cfg.EmailDelaySeconds)
	}
	if cfg.EmailDelay != 45*time.Second {
		t.Errorf("EmailDelay = %v, want 45s", cfg.EmailDelay)
	}
	if cfg.ComplexModel != "override/model" {
		t.Errorf("ComplexModel = %q, want %q", cfg.ComplexModel, "override/model")
	}
	if cfg.MaxTokensPerRun != 200000 {
		t.Errorf("MaxTokensPerRun = %d, want 200000", cfg.MaxTokensPerRun)
	}

	// Unset values should remain
	if cfg.ContactGithub != "original" {
		t.Errorf("ContactGithub should remain %q, got %q", "original", cfg.ContactGithub)
	}
}

func TestMergeIntoConfig_EmptyYAML(t *testing.T) {
	yc := &config.YAMLConfig{}
	cfg := &config.Config{
		ContactName: "Original",
		JobSearchTerms: []string{"original"},
	}

	yc.MergeIntoConfig(cfg)

	if cfg.ContactName != "Original" {
		t.Errorf("ContactName should remain %q, got %q", "Original", cfg.ContactName)
	}
	if len(cfg.JobSearchTerms) != 1 || cfg.JobSearchTerms[0] != "original" {
		t.Errorf("JobSearchTerms should remain original")
	}
}

func TestRejectTitle_CaseInsensitive(t *testing.T) {
	cfg, _ := config.LoadYAML("/tmp/nonexistent_xxxxx")
	cfg.RejectTitles = []string{"teacher", "nurse"}

	tests := []struct {
		title string
		want  bool
	}{
		{"Senior Software Engineer", false},
		{"Math Teacher", true},
		{"math teacher", true},
		{"MATH TEACHER", true},
		{"Senior Nurse Practitioner", true},
		{"NURSE Manager", true},
		{"Golang Developer", false},
		{"Teacher Assistant", true},
	}

	for _, tt := range tests {
		got := cfg.RejectTitle(tt.title)
		if got != tt.want {
			t.Errorf("RejectTitle(%q) = %v, want %v", tt.title, got, tt.want)
		}
	}
}

func TestRejectTitle_EmptyPatterns(t *testing.T) {
	cfg, _ := config.LoadYAML("/tmp/nonexistent_xxxxx")
	cfg.RejectTitles = []string{}

	tests := []string{
		"Senior Software Engineer",
		"Math Teacher",
		"Nurse",
	}

	for _, title := range tests {
		if cfg.RejectTitle(title) {
			t.Errorf("RejectTitle(%q) = true, want false with empty patterns", title)
		}
	}
}

func TestFilterEmail_PrefixPattern(t *testing.T) {
	cfg, _ := config.LoadYAML("/tmp/nonexistent_xxxxx")
	cfg.EmailFilters = []string{"starts_with:accommodation@"}

	tests := []struct {
		email string
		want  bool
	}{
		{"accommodation@company.com", true},
		{"ACCOMMODATION@company.com", true},
		{"hr@company.com", false},
		{"accommodation_request@company.com", false}, // doesn't start with prefix
	}

	for _, tt := range tests {
		got := cfg.FilterEmail(tt.email)
		if got != tt.want {
			t.Errorf("FilterEmail(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}

func TestFilterEmail_ContainsPattern(t *testing.T) {
	cfg, _ := config.LoadYAML("/tmp/nonexistent_xxxxx")
	cfg.EmailFilters = []string{"contains:no-reply"}

	tests := []struct {
		email string
		want  bool
	}{
		{"no-reply@company.com", true},
		{"NO-REPLY@company.com", true},
		{"hr@company.com", false},
		{"jobs-no-reply-2@company.com", true},
	}

	for _, tt := range tests {
		got := cfg.FilterEmail(tt.email)
		if got != tt.want {
			t.Errorf("FilterEmail(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}

func TestFilterEmail_TLDPattern(t *testing.T) {
	cfg, _ := config.LoadYAML("/tmp/nonexistent_xxxxx")
	cfg.EmailFilters = []string{"tld:.tk", "tld:.ml"}

	tests := []struct {
		email string
		want  bool
	}{
		{"careers@company.tk", true},
		{"careers@company.ml", true},
		{"careers@company.TK", true},
		{"careers@company.com", false},
		{"user@tk.domain.com", false}, // .tk is not the TLD
	}

	for _, tt := range tests {
		got := cfg.FilterEmail(tt.email)
		if got != tt.want {
			t.Errorf("FilterEmail(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}

func TestFilterEmail_MultiplePatterns(t *testing.T) {
	cfg, _ := config.LoadYAML("/tmp/nonexistent_xxxxx")
	cfg.EmailFilters = []string{
		"starts_with:accommodation@",
		"contains:no-reply",
		"tld:.tk",
		"tld:.ml",
	}

	tests := []struct {
		email string
		want  bool
	}{
		{"hr@company.com", false},
		{"accommodation@company.com", true},
		{"no-reply@company.com", true},
		{"careers@company.tk", true},
		{"careers@company.ml", true},
		{"careers@company.com", false},
		{"noreply@company.tk", true}, // matches .tk tld filter
	}

	for _, tt := range tests {
		got := cfg.FilterEmail(tt.email)
		if got != tt.want {
			t.Errorf("FilterEmail(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}
