package sdk

import (
	"context"
	"time"
)

// ============================================================================
// Plugin SDK — The contract for all plugins in the JobHunter ecosystem.
//
// Any plugin (email outreach, job scraper enrichment, follow-up bots, etc.)
// implements the Plugin interface and is loaded by the plugin manager.
// ============================================================================

// Plugin is the interface every plugin must implement.
type Plugin interface {
	// ID returns a unique identifier for this plugin.
	ID() string

	// Name returns a human-readable name.
	Name() string

	// Description returns what this plugin does.
	Description() string

	// Execute runs the plugin's main logic.
	// ctx carries deadlines and cancellation.
	// env provides access to environment variables scoped to this plugin.
	Execute(ctx context.Context, env Env) (*Result, error)
}

// Env provides a plugin with access to:
// - its own env var namespace (e.g. PLUGIN_SENDGRID_API_KEY)
// - the shared database (read/write to stats, emails, etc.)
// - logging
type Env interface {
	// Getenv reads an env var, optionally prefixed with the plugin ID.
	// e.g. env.Getenv("API_KEY") returns PLUGIN_MYPLUGIN_API_KEY or just API_KEY
	Getenv(key string) string

	// DB returns the shared database handle for storing results.
	DB() Database

	// Logger returns a logger scoped to this plugin.
	Logger() Logger

	// Config returns the shared application config.
	Config() interface{}
}

// Result is what a plugin returns after execution.
type Result struct {
	// Success indicates whether the plugin ran without errors.
	Success bool `json:"success"`

	// Message is a human-readable summary.
	Message string `json:"message"`

	// Metrics holds arbitrary key-value data that gets recorded in stats.
	Metrics map[string]float64 `json:"metrics,omitempty"`

	// Data holds arbitrary structured data persisted as JSON.
	Data interface{} `json:"data,omitempty"`

	// Duration is how long the plugin took.
	Duration time.Duration `json:"-"`
}

// SimpleResult creates a basic success result.
func SimpleResult(msg string) *Result {
	return &Result{Success: true, Message: msg, Metrics: make(map[string]float64)}
}

// ErrorResult creates a failure result.
func ErrorResult(msg string) *Result {
	return &Result{Success: false, Message: msg, Metrics: make(map[string]float64)}
}

// ============================================================================
// Database interface (minimal — plugins only get what they need)
// ============================================================================

type Database interface {
	// Exec executes a raw query (for plugin-specific tables).
	Exec(ctx context.Context, query string, args ...interface{}) error

	// Query reads rows (for plugin-specific queries).
	Query(ctx context.Context, query string, args ...interface{}) (Rows, error)

	// InsertEmail records an email sent by a plugin.
	InsertEmail(ctx context.Context, e *EmailRecord) (int64, error)

	// RecordStat records a stats entry.
	RecordStat(ctx context.Context, s *StatEntry) error
}

type Rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close() error
}

// EmailRecord is the canonical email tracking record.
type EmailRecord struct {
	PluginID       string     `json:"plugin_id"`
	RecipientEmail string     `json:"recipient_email"`
	Subject        string     `json:"subject"`
	BodyPreview    string     `json:"body_preview"`
	SentAt         time.Time  `json:"sent_at"`
	Status         string     `json:"status"` // pending | sent | failed | bounced | opened | clicked | replied
	TrackingID     string     `json:"tracking_id"`
	MessageID      string     `json:"message_id,omitempty"`
	Metadata       string     `json:"metadata,omitempty"` // JSON blob for arbitrary plugin data
}

// StatEntry is a time-series stats entry.
type StatEntry struct {
	PluginID  string            `json:"plugin_id"`
	Event     string            `json:"event"`     // e.g. "email_sent", "email_opened", "scrape_complete"
	Value     float64           `json:"value"`     // e.g. 1.0, 0.5, duration_seconds
	Tags      map[string]string `json:"tags"`      // e.g. {"source":"linkedin","status":"success"}
	Timestamp time.Time         `json:"timestamp"`
}

// ============================================================================
// Logger interface
// ============================================================================

type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

// ============================================================================
// Plugin metadata helpers
// ============================================================================

// BasePlugin provides a no-op base for plugins to embed.
type BasePlugin struct {
	PluginID   string
	PluginName string
	PluginDesc string
}

func (b *BasePlugin) ID() string          { return b.PluginID }
func (b *BasePlugin) Name() string        { return b.PluginName }
func (b *BasePlugin) Description() string { return b.PluginDesc }
