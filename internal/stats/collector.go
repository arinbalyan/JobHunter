package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Entry is a single stats data point — the core of the stats system.
// All plugins, the agent itself, and tracking events push entries here.
// They get batch-inserted into the database at the end of each run.
type Entry struct {
	ID        int64             `json:"id"`
	PluginID  string            `json:"plugin_id"`
	Event     string            `json:"event"`
	Value     float64           `json:"value"`
	Tags      map[string]string `json:"tags,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// Collector collects stats entries in memory and flushes to DB.
type Collector struct {
	mu      sync.Mutex
	entries []Entry
	batchSz int
}

// NewCollector creates a stats collector with the given batch size.
// Default batch size is 100 (flushes to DB every 100 entries).
func NewCollector(batchSize int) *Collector {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &Collector{
		entries: make([]Entry, 0, batchSize),
		batchSz: batchSize,
	}
}

// Record adds a stats entry to the collector.
func (c *Collector) Record(pluginID, event string, value float64, tags map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if tags == nil {
		tags = make(map[string]string)
	}
	if tags["plugin_id"] == "" {
		tags["plugin_id"] = pluginID
	}

	c.entries = append(c.entries, Entry{
		PluginID:  pluginID,
		Event:     event,
		Value:     value,
		Tags:      tags,
		Timestamp: time.Now().UTC(),
	})
}

// Flush writes all pending entries to the database.
func (c *Collector) Flush(ctx context.Context, db StatsDB) error {
	c.mu.Lock()
	entries := c.entries
	c.entries = make([]Entry, 0, c.batchSz)
	c.mu.Unlock()

	if len(entries) == 0 {
		return nil
	}

	return db.BulkInsertStats(ctx, entries)
}

// Stats returns a snapshot of all collected entries (for reporting).
func (c *Collector) Stats() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]Entry, len(c.entries))
	copy(result, c.entries)
	return result
}

// Len returns how many entries are currently buffered.
func (c *Collector) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Reset clears all entries without flushing.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make([]Entry, 0, c.batchSz)
}

// ============================================================================
// StatsDB — What the collector needs from the database
// ============================================================================

type StatsDB interface {
	BulkInsertStats(ctx context.Context, entries []Entry) error
}

// ============================================================================
// Aggregation Helpers
// ============================================================================

// Aggregation holds pre-computed stats for a time window.
type Aggregation struct {
	PluginID  string  `json:"plugin_id"`
	Event     string  `json:"event"`
	Count     int     `json:"count"`
	Sum       float64 `json:"sum"`
	Avg       float64 `json:"avg"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	TimeFrom  time.Time `json:"time_from"`
	TimeTo    time.Time `json:"time_to"`
}

// MarshalJSON serializes tags for DB storage.
func MarshalTags(tags map[string]string) string {
	if len(tags) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

// UnmarshalTags deserializes tags from DB storage.
func UnmarshalTags(data string) (map[string]string, error) {
	if data == "" {
		return make(map[string]string), nil
	}
	var tags map[string]string
	if err := json.Unmarshal([]byte(data), &tags); err != nil {
		return nil, fmt.Errorf("unmarshal tags: %w", err)
	}
	return tags, nil
}

// Event constants — canonical event names used across the system.
const (
	EventPluginRun        = "plugin_run"
	EventPluginFailed     = "plugin_failed"
	EventEmailSent        = "email_sent"
	EventEmailDelivered   = "email_delivered"
	EventEmailOpened      = "email_opened"
	EventEmailClicked     = "email_clicked"
	EventEmailReplied     = "email_replied"
	EventEmailBounced     = "email_bounced"
	EventJobScraped       = "job_scraped"
	EventJobMatched       = "job_matched"
	EventJobApplied       = "job_applied"
	EventLLMRequest       = "llm_request"
	EventLLMTokens        = "llm_tokens"
	EventFollowupSent     = "followup_sent"
	EventBounceDetected   = "bounce_detected"
	EventReplyDetected    = "reply_detected"
)
