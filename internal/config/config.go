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

	// LLM Providers (all added dynamically from env)
	OpenRouterAPIKey string
	GroqAPIKey       string
	CerebrasAPIKey   string
	NvidiaAPIKey     string
	SambaNovaAPIKey  string
	HuggingFaceAPIKey string
	ReplicateAPIToken string
	DeepInfraAPIKey  string
	FireworksAPIKey  string
	CodestralAPIKey  string
	HyperbolicAPIKey string
	GoogleAPIKey     string
	TogetherAPIKey   string
	ZAIAPIKey        string

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

	// Telegram
	TelegramBotToken  string
	TelegramChatID    string

	// Application
	RunMode             string
	LogLevel            string
	MaxEmailsPerRun     int
	EmailDelaySeconds   int
	EmailDelay          time.Duration
	DailyEmailLimit     int
	MinWords            int
	MaxWords            int

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

	// Contact Info
	ContactName      string
	ContactEmail     string
	ContactPhone     string
	ContactPortfolio string
	ContactGithub    string
	ContactLinkedin  string
	ContactCodolio   string
	ResumeDriveLink  string
}

// ProviderConfig holds configuration for a single LLM provider.
type ProviderConfig struct {
	Kind    string // openrouter | groq | cerebras | nvidia | sambanova | together | deepinfra | fireworks | hyperbolic | zai
	APIKey  string
	BaseURL string
	Model   string
	Weight  int // higher = more preferred for complex tasks
}

// GetActiveProviders returns all configured LLM providers.
func (c *Config) GetActiveProviders() []ProviderConfig {
	providers := []ProviderConfig{}

	// ─── OpenRouter: Dynamic free model picker ──────────────
	// On startup, fetches all models from OpenRouter, filters those with ":free" suffix,
	// sorts by context length (largest = most capable first), and injects them all.
	// This means new free models are automatically picked up without code changes.
	if c.OpenRouterAPIKey != "" {
		// Priority 1: User's preferred complex model (if specified)
		if c.ComplexModel != "" {
			providers = append(providers,
				ProviderConfig{Kind: "openrouter", APIKey: c.OpenRouterAPIKey, BaseURL: "https://openrouter.ai/api", Model: c.ComplexModel, Weight: 10},
			)
		}

		// Priority 2: User's preferred simple model
		if c.SimpleModel != "" && c.SimpleModel != c.ComplexModel {
			providers = append(providers,
				ProviderConfig{Kind: "openrouter", APIKey: c.OpenRouterAPIKey, BaseURL: "https://openrouter.ai/api", Model: c.SimpleModel, Weight: 5},
			)
		}

		// Priority 3: Dynamically discovered free models
		// Ranks by context length: bigger context = more capable = higher weight
		freeModels, err := FetchFreeModels(c.OpenRouterAPIKey)
		if err == nil && len(freeModels) > 0 {
			for i, m := range freeModels {
				// Weight: higher context = higher weight, capped at 8, min 1
				weight := 8 - i
				if weight < 1 {
					weight = 1
				}
				if weight > 8 {
					weight = 8
				}
				providers = append(providers,
					ProviderConfig{
						Kind:    "openrouter",
						APIKey:  c.OpenRouterAPIKey,
						BaseURL: "https://openrouter.ai/api",
						Model:   m.ID,
						Weight:  weight,
					},
				)
			}
		} else {
			// Fallback: if API call fails, use a few well-known free models
			providers = append(providers,
				ProviderConfig{Kind: "openrouter", APIKey: c.OpenRouterAPIKey, BaseURL: "https://openrouter.ai/api", Model: "google/gemma-4-26b-a4b-it:free", Weight: 6},
				ProviderConfig{Kind: "openrouter", APIKey: c.OpenRouterAPIKey, BaseURL: "https://openrouter.ai/api", Model: "deepseek/deepseek-v4-flash:free", Weight: 5},
				ProviderConfig{Kind: "openrouter", APIKey: c.OpenRouterAPIKey, BaseURL: "https://openrouter.ai/api", Model: "meta-llama/llama-3.3-70b-instruct:free", Weight: 4},
			)
		}

		// Priority 4: Emergency catch-all — OpenRouter's free router
		providers = append(providers,
			ProviderConfig{Kind: "openrouter", APIKey: c.OpenRouterAPIKey, BaseURL: "https://openrouter.ai/api", Model: "openrouter/free", Weight: 0},
		)
	}

	// --- Groq (fast inference, good for simple tasks) ---
	if c.GroqAPIKey != "" {
		providers = append(providers,
			ProviderConfig{Kind: "groq", APIKey: c.GroqAPIKey, BaseURL: "https://api.groq.com/openai", Model: "llama-3.3-70b-versatile", Weight: 3},
			ProviderConfig{Kind: "groq", APIKey: c.GroqAPIKey, BaseURL: "https://api.groq.com/openai", Model: "llama-3.1-8b-instant", Weight: 2},
			ProviderConfig{Kind: "groq", APIKey: c.GroqAPIKey, BaseURL: "https://api.groq.com/openai", Model: "openai/gpt-oss-120b", Weight: 1},
		)
	}

	// --- Cerebras (wafer-scale, very fast) ---
	if c.CerebrasAPIKey != "" {
		providers = append(providers,
			ProviderConfig{Kind: "cerebras", APIKey: c.CerebrasAPIKey, BaseURL: "https://api.cerebras.ai", Model: c.SimpleModel, Weight: 2},
		)
	}

	// --- Together AI ---
	if c.TogetherAPIKey != "" {
		providers = append(providers,
			ProviderConfig{Kind: "together", APIKey: c.TogetherAPIKey, BaseURL: "https://api.together.xyz", Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo", Weight: 2},
			ProviderConfig{Kind: "together", APIKey: c.TogetherAPIKey, BaseURL: "https://api.together.xyz", Model: "google/gemma-4-9b-it", Weight: 2},
		)
	}

	// --- DeepInfra ---
	if c.DeepInfraAPIKey != "" {
		providers = append(providers,
			ProviderConfig{Kind: "deepinfra", APIKey: c.DeepInfraAPIKey, BaseURL: "https://api.deepinfra.com", Model: "meta-llama/Llama-3.3-70B-Instruct", Weight: 2},
		)
	}

	// --- Fireworks AI ---
	if c.FireworksAPIKey != "" {
		providers = append(providers,
			ProviderConfig{Kind: "fireworks", APIKey: c.FireworksAPIKey, BaseURL: "https://api.fireworks.ai", Model: "accounts/fireworks/models/llama-v3p3-70b-instruct", Weight: 2},
		)
	}

	// --- Hyperbolic ---
	if c.HyperbolicAPIKey != "" {
		providers = append(providers,
			ProviderConfig{Kind: "hyperbolic", APIKey: c.HyperbolicAPIKey, BaseURL: "https://api.hyperbolic.xyz", Model: "meta-llama/Llama-3.3-70B-Instruct", Weight: 2},
		)
	}

	// --- SambaNova ---
	if c.SambaNovaAPIKey != "" {
		providers = append(providers,
			ProviderConfig{Kind: "sambanova", APIKey: c.SambaNovaAPIKey, BaseURL: "https://api.sambanova.ai", Model: "Meta-Llama-3.3-70B-Instruct", Weight: 2},
		)
	}

	// --- ZAI (GLM) ---
	if c.ZAIAPIKey != "" {
		providers = append(providers,
			ProviderConfig{Kind: "zai", APIKey: c.ZAIAPIKey, BaseURL: "https://open.bigmodel.cn/api/paas/v4", Model: "GLM-4-Plus", Weight: 1},
		)
	}

	return providers
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL: getEnv("DATABASE_URL", ""),

		OpenRouterAPIKey: getEnv("OPENROUTER_API_KEY", ""),
		GroqAPIKey:       getEnv("GROQ_API_KEY", ""),
		CerebrasAPIKey:   getEnv("CEREBRAS_API_KEY", ""),
		NvidiaAPIKey:     getEnv("NVIDIA_API_KEY", ""),
		SambaNovaAPIKey:  getEnv("SAMBANOVA_API_KEY", ""),
		HuggingFaceAPIKey: getEnv("HUGGINGFACE_API_KEY", ""),
		ReplicateAPIToken: getEnv("REPLICATE_API_TOKEN", ""),
		DeepInfraAPIKey:  getEnv("DEEPINFRA_API_KEY", ""),
		FireworksAPIKey:  getEnv("FIREWORKS_API_KEY", ""),
		CodestralAPIKey:  getEnv("CODESTRAL_API_KEY", ""),
		HyperbolicAPIKey: getEnv("HYPERBOLIC_API_KEY", ""),
		GoogleAPIKey:     getEnv("GOOGLE_API_KEY", ""),
		TogetherAPIKey:   getEnv("TOGETHER_API_KEY", ""),
		ZAIAPIKey:        getEnv("ZAI_API_KEY", ""),

		ComplexModel: getEnv("LLM_COMPLEX_MODEL", "google/gemma-4-26b-a4b-it:free"),
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

		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),

		ScrapyProxy:       getEnv("SCRAPY_PROXY", ""),
		ScrapyMemoryCapMB: getEnvInt("SCRAPY_MEMORY_CAP_MB", 100),

		RunMode:           getEnv("RUN_MODE", "once"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		MaxEmailsPerRun:   getEnvInt("MAX_EMAILS_PER_RUN", 10),
		EmailDelaySeconds: getEnvInt("EMAIL_DELAY_SECONDS", 30),
		DailyEmailLimit:   getEnvInt("DAILY_EMAIL_LIMIT", 500),

		JobResultsPerSite: getEnvInt("JOB_RESULTS_PER_SITE", 25),
		JobHoursOld:        getEnvInt("JOB_HOURS_OLD", 72),
		JobRemoteOnly:      getEnvBool("JOB_REMOTE_ONLY", true),
		JobType:            getEnv("JOB_TYPE", "fulltime"),

		UserYearsExperience: getEnvInt("USER_YEARS_EXPERIENCE", 0),
		UserCurrentRole:     getEnv("USER_CURRENT_ROLE", ""),

		ContactName:      getEnv("CONTACT_NAME", ""),
		ContactEmail:     getEnv("CONTACT_EMAIL", ""),
		ContactPhone:     getEnv("CONTACT_PHONE", ""),
		ContactPortfolio: getEnv("CONTACT_PORTFOLIO", ""),
		ContactGithub:    getEnv("CONTACT_GITHUB", ""),
		ContactLinkedin:  getEnv("CONTACT_LINKEDIN", ""),
		ContactCodolio:   getEnv("CONTACT_CODOLIO", ""),
		ResumeDriveLink:  getEnv("RESUME_DRIVE_LINK", ""),
	}

	// Parse comma-separated lists
	cfg.JobSearchTerms = parseCommaList(getEnv("JOB_SEARCH_TERMS", ""))
	cfg.JobLocations = parseCommaList(getEnv("JOB_LOCATIONS", ""))
	cfg.JobSites = parseCommaList(getEnv("JOB_SITES", "all"))
	cfg.UserTargetRoles = parseJSONList(getEnv("USER_TARGET_ROLES", "[]"))

	// Parse durations
	cfg.EmailDelay = time.Duration(cfg.EmailDelaySeconds) * time.Second

	// Token limits
	if v := getEnv("LLM_MAX_TOKENS_PER_RUN", ""); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			cfg.MaxTokensPerRun = parsed
		}
	}
	cfg.MaxTokensPerRequest = getEnvInt("LLM_MAX_TOKENS_PER_REQUEST", 2048)

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.Atoi(val)
		if err == nil {
			return parsed
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err == nil {
			return parsed
		}
	}
	return defaultVal
}

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

func parseJSONList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
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

// TelegramToken returns the bot token.
func (c *Config) TelegramToken() string {
	return c.TelegramBotToken
}

// TelegramChat returns the chat ID.
func (c *Config) TelegramChat() string {
	return c.TelegramChatID
}

func (c *Config) Validate() error {
	var errs []string
	if c.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.GmailUser == "" || c.GmailAppPass == "" {
		errs = append(errs, "GMAIL_USER and GMAIL_APP_PASS are required")
	}
	if len(c.GetActiveProviders()) == 0 {
		errs = append(errs, "at least one LLM provider key is required")
	}
	if len(c.JobSearchTerms) == 0 {
		errs = append(errs, "at least one JOB_SEARCH_TERM is required")
	}
	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
