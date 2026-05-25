package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arinbalyan/jobhunter/internal/config"
)

func TestLoadYAML_Defaults(t *testing.T) {
	// Non-existent file should return defaults
	cfg, err := config.LoadYAML("/tmp/nonexistent_config.yaml")
	if err != nil {
		t.Fatalf("LoadYAML on missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Email.MaxPerRun != 10 {
		t.Errorf("expected MaxPerRun=10, got %d", cfg.Email.MaxPerRun)
	}
	if cfg.Dedup.EmailCooldownDays != 90 {
		t.Errorf("expected EmailCooldownDays=90, got %d", cfg.Dedup.EmailCooldownDays)
	}
	if cfg.MaxRuntime != 350 {
		t.Errorf("expected MaxRuntime=350, got %d", cfg.MaxRuntime)
	}
}

func TestLoadYAML_CustomFile(t *testing.T) {
	yamlContent := `
user:
  name: "Test User"
  years_experience: 5
search:
  terms:
    - "golang developer"
    - "backend engineer"
  locations:
    - "remote"
  remote_only: true
reject_titles:
  - "teacher"
  - "nurse"
email_filters:
  - "contains:no-reply"
  - "tld:.tk"
dedup:
  email_cooldown_days: 60
email:
  max_per_run: 5
  delay_seconds: 60
  daily_limit: 200
  min_words: 100
  max_words: 400
llm:
  complex_model: "test/model:free"
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

	if cfg.User.Name != "Test User" {
		t.Errorf("Name = %q, want %q", cfg.User.Name, "Test User")
	}
	if cfg.User.YearsExperience != 5 {
		t.Errorf("YearsExperience = %d, want 5", cfg.User.YearsExperience)
	}
	if len(cfg.Search.Terms) != 2 {
		t.Errorf("len(Terms) = %d, want 2", len(cfg.Search.Terms))
	}
	if !cfg.Search.RemoteOnly {
		t.Error("RemoteOnly = false, want true")
	}
	if cfg.Dedup.EmailCooldownDays != 60 {
		t.Errorf("EmailCooldownDays = %d, want 60", cfg.Dedup.EmailCooldownDays)
	}
	if cfg.Email.MaxPerRun != 5 {
		t.Errorf("MaxPerRun = %d, want 5", cfg.Email.MaxPerRun)
	}
	if cfg.Email.MinWords != 100 {
		t.Errorf("MinWords = %d, want 100", cfg.Email.MinWords)
	}
	if cfg.Email.MaxWords != 400 {
		t.Errorf("MaxWords = %d, want 400", cfg.Email.MaxWords)
	}
	if cfg.LLM.ComplexModel != "test/model:free" {
		t.Errorf("ComplexModel = %q, want %q", cfg.LLM.ComplexModel, "test/model:free")
	}
	if cfg.LLM.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", cfg.LLM.Temperature)
	}
	if cfg.MaxRuntime != 300 {
		t.Errorf("MaxRuntime = %d, want 300", cfg.MaxRuntime)
	}
}

func TestRejectTitle(t *testing.T) {
	cfg, _ := config.LoadYAML("/tmp/nonexistent")

	// Manually set patterns
	cfg.RejectTitles = []string{"teacher", "nurse", "cashier", "intern"}

	tests := []struct {
		title string
		want  bool
	}{
		{"Senior Software Engineer", false},
		{"Math Teacher", true},
		{"Registered Nurse", true},
		{"Cashier", true},
		{"Software Engineer Intern", true},
		{"Golang Developer", false},
	}

	for _, tt := range tests {
		got := cfg.RejectTitle(tt.title)
		if got != tt.want {
			t.Errorf("RejectTitle(%q) = %v, want %v", tt.title, got, tt.want)
		}
	}
}

func TestFilterEmail(t *testing.T) {
	cfg, _ := config.LoadYAML("/tmp/nonexistent")
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
	}

	for _, tt := range tests {
		got := cfg.FilterEmail(tt.email)
		if got != tt.want {
			t.Errorf("FilterEmail(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}
