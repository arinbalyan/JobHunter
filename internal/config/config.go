package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application.
type Config struct {
	// Database
	DatabaseURL string

	// LLM Providers
	OpenRouterAPIKey string
	GroqAPIKey       string
	CerebrasAPIKey   string

	// LLM Settings
	ComplexModel          string
	SimpleModel           string
	MaxTokensPerRun       int64
	MaxTokensPerRequest   int

	// Email SMTP
	GmailUser    string
	GmailAppPass string
	EmailFromName string

	// Tracking Server
	TrackingServerURL  string
	TrackingServerPort int

	// IMAP
	IMAPUser string
	IMAPPass string
	IMAPHost string
	IMAPPort int

	// Scrappy
	ScrapyProxy      string
	ScrapyMemoryCapMB int

	// Application
	RunMode             string
	LogLevel            string
	MaxEmailsPerRun     int
	EmailDelaySeconds   int
	EmailDelay          time.Duration

	// Job Search
	JobSearchTerms  []string
	JobLocations    []string
	JobSites        []string
	JobResultsPerSite int
	JobHoursOld       int
	JobRemoteOnly     bool
	JobType           string

	// User Profile
	UserYearsExperience int
	UserCurrentRole     string
	UserTargetRoles     []string
}

// Load reads configuration from environment variables.
// It attempts to load .env file first, then falls back to OS env vars.
func Load() (*Config, error) {
	// Try loading .env file — it's okay if it doesn't exist
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL: getEnv("DATABASE_URL", ""),

		OpenRouterAPIKey: getEnv("OPENROUTER_API_KEY", ""),
		GroqAPIKey:       getEnv("GROQ_API_KEY", ""),
		CerebrasAPIKey:   getEnv("CEREBRAS_API_KEY", ""),

		ComplexModel: getEnv("LLM_COMPLEX_MODEL", "google/gemma-4-9b-it"),
		SimpleModel:  getEnv("LLM_SIMPLE_MODEL", "google/gemma-4-9b-it"),

		GmailUser:     getEnv("GMAIL_USER", ""),
		GmailAppPass:  getEnv("GMAIL_APP_PASS", ""),
		EmailFromName: getEnv("EMAIL_FROM_NAME", ""),

		TrackingServerURL:  getEnv("TRACKING_SERVER_URL", "http://localhost:8080"),
		TrackingServerPort: getEnvInt("TRACKING_SERVER_PORT", 8080),

		IMAPUser: getEnv("IMAP_USER", ""),
		IMAPPass: getEnv("IMAP_PASS", ""),
		IMAPHost: getEnv("IMAP_HOST", "imap.gmail.com"),
		IMAPPort: getEnvInt("IMAP_PORT", 993),

		ScrapyProxy:       getEnv("SCRAPY_PROXY", ""),
		ScrapyMemoryCapMB: getEnvInt("SCRAPY_MEMORY_CAP_MB", 100),

		RunMode:           getEnv("RUN_MODE", "once"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		MaxEmailsPerRun:   getEnvInt("MAX_EMAILS_PER_RUN", 10),
		EmailDelaySeconds: getEnvInt("EMAIL_DELAY_SECONDS", 30),

		JobResultsPerSite: getEnvInt("JOB_RESULTS_PER_SITE", 25),
		JobHoursOld:        getEnvInt("JOB_HOURS_OLD", 72),
		JobRemoteOnly:      getEnvBool("JOB_REMOTE_ONLY", true),
		JobType:            getEnv("JOB_TYPE", "fulltime"),

		UserYearsExperience: getEnvInt("USER_YEARS_EXPERIENCE", 0),
		UserCurrentRole:     getEnv("USER_CURRENT_ROLE", ""),
	}

	// Parse comma-separated lists
	cfg.JobSearchTerms = parseCommaList(getEnv("JOB_SEARCH_TERMS", ""))
	cfg.JobLocations = parseCommaList(getEnv("JOB_LOCATIONS", ""))
	cfg.JobSites = parseCommaList(getEnv("JOB_SITES", "all"))
	cfg.UserTargetRoles = parseJSONList(getEnv("USER_TARGET_ROLES", "[]"))

	// Parse durations
	cfg.EmailDelay = time.Duration(cfg.EmailDelaySeconds) * time.Second

	// Parse token limits
	if v := getEnv("LLM_MAX_TOKENS_PER_RUN", ""); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			cfg.MaxTokensPerRun = parsed
		}
	}
	cfg.MaxTokensPerRequest = getEnvInt("LLM_MAX_TOKENS_PER_REQUEST", 2048)

	// Validate required fields
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.GmailUser == "" || cfg.GmailAppPass == "" {
		return nil, fmt.Errorf("GMAIL_USER and GMAIL_APP_PASS are required")
	}
	if cfg.OpenRouterAPIKey == "" {
		return nil, fmt.Errorf("at least one LLM provider key is required (OPENROUTER_API_KEY)")
	}

	return cfg, nil
}

// LoadedFromEnv returns true if the .env file was loaded.
func LoadedFromEnv() bool {
	return true
}

// getEnv reads an env var with a default fallback.
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvInt reads an integer env var.
func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.Atoi(val)
		if err == nil {
			return parsed
		}
	}
	return defaultVal
}

// getEnvBool reads a boolean env var.
func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err == nil {
			return parsed
		}
	}
	return defaultVal
}

// parseCommaList splits a comma-separated string, trimming whitespace.
func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// parseJSONList tries to parse a JSON string array.
func parseJSONList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	// Simple parsing for ["a","b","c"] format
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"")
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// Validate checks that all required configuration is present and valid.
func (c *Config) Validate() error {
	var errs []string

	if c.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.GmailUser == "" || c.GmailAppPass == "" {
		errs = append(errs, "GMAIL_USER and GMAIL_APP_PASS are required")
	}
	if c.OpenRouterAPIKey == "" && c.GroqAPIKey == "" && c.CerebrasAPIKey == "" {
		errs = append(errs, "at least one LLM provider key is required (OPENROUTER_API_KEY, GROQ_API_KEY, or CEREBRAS_API_KEY)")
	}
	if len(c.JobSearchTerms) == 0 {
		errs = append(errs, "at least one JOB_SEARCH_TERM is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
