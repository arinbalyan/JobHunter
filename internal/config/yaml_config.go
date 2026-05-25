package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// YAMLConfig holds all user-facing configuration loaded from .agent-data/config.yaml
type YAMLConfig struct {
	User        UserConfig        `yaml:"user"`
	Search      SearchConfig      `yaml:"search"`
	RejectTitles []string          `yaml:"reject_titles"`
	EmailFilters []string          `yaml:"email_filters"`
	Dedup       DedupConfig        `yaml:"dedup"`
	Email       EmailSendingConfig `yaml:"email"`
	LLM         LLMConfig          `yaml:"llm"`
	MaxRuntime  int                `yaml:"max_runtime_minutes"`
}

type UserConfig struct {
	Name            string `yaml:"name"`
	CurrentRole     string `yaml:"current_role"`
	YearsExperience int    `yaml:"years_experience"`
	Email           string `yaml:"email"`
	Phone           string `yaml:"phone"`
	Portfolio       string `yaml:"portfolio"`
	Github          string `yaml:"github"`
	Linkedin        string `yaml:"linkedin"`
	Codolio         string `yaml:"codolio"`
	ResumeDriveLink string `yaml:"resume_drive_link"`
}

type SearchConfig struct {
	Terms          []string        `yaml:"terms"`
	Locations      []string        `yaml:"locations"`
	Sites          []string        `yaml:"sites"`
	RemoteOnly     bool            `yaml:"remote_only"`
	JobType        string          `yaml:"job_type"`
	ResultsPerSite int             `yaml:"results_per_site"`
	HoursOld       int             `yaml:"hours_old"`
	Onsite         OnsiteConfig    `yaml:"onsite"`
}

type OnsiteConfig struct {
	Enabled         bool     `yaml:"enabled"`
	Terms           []string `yaml:"terms"`
	Locations       []string `yaml:"locations"`
	Sites           []string `yaml:"sites"`
	RemoteOnly      bool     `yaml:"remote_only"`
	JobType         string   `yaml:"job_type"`
	ResultsPerSite  int      `yaml:"results_per_site"`
	HoursOld        int      `yaml:"hours_old"`
	MaxEmailsPerDay int      `yaml:"max_emails_per_day"`
}

type DedupConfig struct {
	EmailCooldownDays  int `yaml:"email_cooldown_days"`
	DomainCooldownHours int `yaml:"domain_cooldown_hours"`
	CompanyCooldownHours int `yaml:"company_cooldown_hours"`
}

type EmailSendingConfig struct {
	MaxPerRun         int           `yaml:"max_per_run"`
	DelaySeconds      int           `yaml:"delay_seconds"`
	DailyLimit        int           `yaml:"daily_limit"`
	RetryAttempts     int           `yaml:"retry_attempts"`
	RetryDelaySeconds int           `yaml:"retry_delay_seconds"`
	MinWords          int           `yaml:"min_words"`
	MaxWords          int           `yaml:"max_words"`
}

type LLMConfig struct {
	ComplexModel       string  `yaml:"complex_model"`
	SimpleModel        string  `yaml:"simple_model"`
	MaxTokensPerRun    int64   `yaml:"max_tokens_per_run"`
	MaxTokensPerReq    int     `yaml:"max_tokens_per_request"`
	Temperature        float64 `yaml:"temperature"`
}

// LoadYAML loads config from the .agent-data/config.yaml file.
// Returns defaults if file doesn't exist.
func LoadYAML(path string) (*YAMLConfig, error) {
	cfg := defaultYAMLConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

// MergeIntoConfig merges YAML settings into the env-based Config.
func (yc *YAMLConfig) MergeIntoConfig(cfg *Config) {
	// User profile from YAML overrides env vars if set
	if yc.User.Name != "" {
		cfg.ContactName = yc.User.Name
	}
	if yc.User.Email != "" {
		cfg.ContactEmail = yc.User.Email
	}
	if yc.User.Phone != "" {
		cfg.ContactPhone = yc.User.Phone
	}
	if yc.User.Portfolio != "" {
		cfg.ContactPortfolio = yc.User.Portfolio
	}
	if yc.User.Github != "" {
		cfg.ContactGithub = yc.User.Github
	}
	if yc.User.Linkedin != "" {
		cfg.ContactLinkedin = yc.User.Linkedin
	}
	if yc.User.Codolio != "" {
		cfg.ContactCodolio = yc.User.Codolio
	}
	if yc.User.ResumeDriveLink != "" {
		cfg.ResumeDriveLink = yc.User.ResumeDriveLink
	}
	if yc.User.CurrentRole != "" {
		cfg.UserCurrentRole = yc.User.CurrentRole
	}
	if yc.User.YearsExperience > 0 {
		cfg.UserYearsExperience = yc.User.YearsExperience
	}

	// Search terms from YAML
	if len(yc.Search.Terms) > 0 {
		cfg.JobSearchTerms = yc.Search.Terms
	}
	if len(yc.Search.Locations) > 0 {
		cfg.JobLocations = yc.Search.Locations
	}
	if len(yc.Search.Sites) > 0 {
		cfg.JobSites = yc.Search.Sites
	}
	if yc.Search.RemoteOnly {
		cfg.JobRemoteOnly = true
	}
	if yc.Search.JobType != "" {
		cfg.JobType = yc.Search.JobType
	}
	if yc.Search.ResultsPerSite > 0 {
		cfg.JobResultsPerSite = yc.Search.ResultsPerSite
	}
	if yc.Search.HoursOld > 0 {
		cfg.JobHoursOld = yc.Search.HoursOld
	}

	// Email settings
	if yc.Email.MaxPerRun > 0 {
		cfg.MaxEmailsPerRun = yc.Email.MaxPerRun
	}
	if yc.Email.DelaySeconds > 0 {
		cfg.EmailDelaySeconds = yc.Email.DelaySeconds
	}
	if yc.Email.DailyLimit > 0 {
		cfg.DailyEmailLimit = yc.Email.DailyLimit
	}
	cfg.EmailDelay = time.Duration(cfg.EmailDelaySeconds) * time.Second

	// LLM settings
	if yc.LLM.ComplexModel != "" {
		cfg.ComplexModel = yc.LLM.ComplexModel
	}
	if yc.LLM.SimpleModel != "" {
		cfg.SimpleModel = yc.LLM.SimpleModel
	}
	if yc.LLM.MaxTokensPerRun > 0 {
		cfg.MaxTokensPerRun = yc.LLM.MaxTokensPerRun
	}
	if yc.LLM.MaxTokensPerReq > 0 {
		cfg.MaxTokensPerRequest = yc.LLM.MaxTokensPerReq
	}
}

// RejectTitle checks if a title matches any rejection pattern.
func (yc *YAMLConfig) RejectTitle(title string) bool {
	titleLow := strings.ToLower(title)
	for _, pattern := range yc.RejectTitles {
		if strings.Contains(titleLow, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// FilterEmail checks if an email matches any filter pattern.
func (yc *YAMLConfig) FilterEmail(email string) bool {
	emailLow := strings.ToLower(email)
	for _, pattern := range yc.EmailFilters {
		switch {
		case strings.HasPrefix(pattern, "starts_with:"):
			prefix := strings.TrimPrefix(pattern, "starts_with:")
			if strings.HasPrefix(emailLow, strings.ToLower(prefix)) {
				return true
			}
		case strings.HasPrefix(pattern, "contains:"):
			substr := strings.TrimPrefix(pattern, "contains:")
			if strings.Contains(emailLow, strings.ToLower(substr)) {
				return true
			}
		case strings.HasPrefix(pattern, "tld:"):
			tld := strings.TrimPrefix(pattern, "tld:")
			if strings.HasSuffix(emailLow, strings.ToLower(tld)) {
				return true
			}
		}
	}
	return false
}

func defaultYAMLConfig() *YAMLConfig {
	return &YAMLConfig{
		User: UserConfig{
			Name:            "Applicant",
			CurrentRole:     "Software Engineer",
			YearsExperience: 0,
		},
		Search: SearchConfig{
			Terms:          []string{"software engineer", "backend engineer"},
			Locations:      []string{"remote"},
			Sites:          []string{"all"},
			RemoteOnly:     true,
			JobType:        "fulltime",
			ResultsPerSite: 25,
			HoursOld:       72,
		},
		Dedup: DedupConfig{
			EmailCooldownDays:   90,
			DomainCooldownHours: 24,
			CompanyCooldownHours: 24,
		},
		Email: EmailSendingConfig{
			MaxPerRun:         10,
			DelaySeconds:      30,
			DailyLimit:        500,
			RetryAttempts:     3,
			RetryDelaySeconds: 5,
			MinWords:          120,
			MaxWords:          300,
		},
		LLM: LLMConfig{
			ComplexModel:    "google/gemma-4-26b-a4b-it:free",
			SimpleModel:     "google/gemma-4-9b-it",
			MaxTokensPerRun: 100000,
			MaxTokensPerReq: 2048,
			Temperature:     0.7,
		},
		MaxRuntime: 350,
	}
}
