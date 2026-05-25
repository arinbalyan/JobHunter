package db

import (
	"context"
	"fmt"
	"time"

	"github.com/arinbalyan/jobhunter/internal/stats"
)

// BulkInsertStats inserts multiple stats entries in a single batch.
func (p *Pool) BulkInsertStats(ctx context.Context, entries []stats.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	// Use a transaction for the batch
	tx, err := p.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, entry := range entries {
		tags := stats.MarshalTags(entry.Tags)
		_, err := tx.Exec(ctx,
			`INSERT INTO stats (plugin_id, event, value, tags, recorded_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			entry.PluginID, entry.Event, entry.Value, tags, entry.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("insert stat: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// GetStats retrieves stats for a given event within a time range.
func (p *Pool) GetStats(ctx context.Context, event string, since time.Duration) ([]stats.Entry, error) {
	rows, err := p.Query(ctx,
		`SELECT id, plugin_id, event, value, tags, recorded_at
		 FROM stats
		 WHERE event = $1 AND recorded_at > $2
		 ORDER BY recorded_at DESC`,
		event, time.Now().Add(-since),
	)
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}
	defer rows.Close()

	var entries []stats.Entry
	for rows.Next() {
		var e stats.Entry
		tagsJSON := ""
		err := rows.Scan(&e.ID, &e.PluginID, &e.Event, &e.Value, &tagsJSON, &e.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("scan stat: %w", err)
		}
		e.Tags, _ = stats.UnmarshalTags(tagsJSON)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// AggregateStats computes aggregate values for an event in a time range.
func (p *Pool) AggregateStats(ctx context.Context, event string, since time.Duration) (*stats.Aggregation, error) {
	agg := &stats.Aggregation{
		Event:   event,
		TimeFrom: time.Now().Add(-since),
		TimeTo:  time.Now(),
	}

	err := p.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(value), 0), COALESCE(AVG(value), 0),
		        COALESCE(MIN(value), 0), COALESCE(MAX(value), 0)
		 FROM stats
		 WHERE event = $1 AND recorded_at > $2`,
		event, agg.TimeFrom,
	).Scan(&agg.Count, &agg.Sum, &agg.Avg, &agg.Min, &agg.Max)
	if err != nil {
		return nil, fmt.Errorf("aggregate stats: %w", err)
	}

	return agg, nil
}

// RecordPluginState upserts the state of a plugin run.
func (p *Pool) RecordPluginState(ctx context.Context, pluginID, pluginName string, success bool) error {
	_, err := p.Exec(ctx,
		`INSERT INTO plugin_state (plugin_id, plugin_name, last_run_at, run_count, error_count)
		 VALUES ($1, $2, $3, 1, 0)
		 ON CONFLICT (plugin_id) DO UPDATE SET
		   plugin_name = EXCLUDED.plugin_name,
		   last_run_at = EXCLUDED.last_run_at,
		   run_count = plugin_state.run_count + 1,
		   error_count = CASE WHEN $4 THEN plugin_state.error_count ELSE plugin_state.error_count + 1 END,
		   last_success_at = CASE WHEN $4 THEN EXCLUDED.last_run_at ELSE plugin_state.last_success_at END,
		   updated_at = NOW()`,
		pluginID, pluginName, time.Now().UTC(), success,
	)
	if err != nil {
		return fmt.Errorf("record plugin state: %w", err)
	}
	return nil
}

// InsertApplication inserts or updates an application record.
func (p *Pool) InsertApplication(ctx context.Context, a *ApplicationRecord) (int64, error) {
	var id int64
	err := p.QueryRow(ctx,
		`INSERT INTO applications
			(plugin_id, job_id, company, title, email_sent_to, stage, score, notes, sent_at, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id`,
		a.PluginID, a.JobID, a.Company, a.Title, a.EmailSentTo, a.Stage,
		a.Score, a.Notes, a.SentAt, a.Metadata,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert application: %w", err)
	}
	return id, nil
}

// AdvanceApplicationStage moves an application to the next stage.
func (p *Pool) AdvanceApplicationStage(ctx context.Context, id int64, stage string) error {
	// Map stage to its timestamp column
	stageFields := map[string]string{
		"sent":      "sent_at = NOW()",
		"delivered": "",  // No specific timestamp for delivered
		"opened":    "opened_at = NOW()",
		"replied":   "replied_at = NOW()",
		"interview": "interview_at = NOW()",
		"offer":     "offer_at = NOW()",
		"rejected":  "rejected_at = NOW()",
	}

	setClause := "stage = $2, updated_at = NOW()"
	if field, ok := stageFields[stage]; ok && field != "" {
		setClause = fmt.Sprintf("stage = $2, %s, updated_at = NOW()", field)
	}

	_, err := p.Exec(ctx,
		`UPDATE applications SET `+setClause+` WHERE id = $1`,
		id, stage,
	)
	if err != nil {
		return fmt.Errorf("advance application stage: %w", err)
	}
	return nil
}

// IsBlacklisted checks if a domain or email is blacklisted.
func (p *Pool) IsBlacklisted(ctx context.Context, pattern string) (bool, error) {
	var count int
	err := p.QueryRow(ctx,
		`SELECT COUNT(*) FROM blacklist
		 WHERE pattern = $1
		   AND (expires_at IS NULL OR expires_at > NOW())`,
		pattern,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check blacklist: %w", err)
	}
	return count > 0, nil
}

// AddToBlacklist adds a pattern to the blacklist.
func (p *Pool) AddToBlacklist(ctx context.Context, pattern, reason string) error {
	_, err := p.Exec(ctx,
		`INSERT INTO blacklist (pattern, reason, hit_count)
		 VALUES ($1, $2, 1)
		 ON CONFLICT (pattern) DO UPDATE SET
		   hit_count = blacklist.hit_count + 1,
		   reason = CASE WHEN $2 != '' THEN $2 ELSE blacklist.reason END`,
		pattern, reason,
	)
	if err != nil {
		return fmt.Errorf("add to blacklist: %w", err)
	}
	return nil
}

// ApplicationRecord matches the applications table.
type ApplicationRecord struct {
	ID          int64     `json:"id"`
	PluginID    string    `json:"plugin_id"`
	JobID       *int64    `json:"job_id,omitempty"`
	Company     string    `json:"company"`
	Title       string    `json:"title"`
	EmailSentTo string    `json:"email_sent_to"`
	Stage       string    `json:"stage"`
	Score       int       `json:"score"`
	Notes       string    `json:"notes,omitempty"`
	SentAt      time.Time `json:"sent_at"`
	Metadata    string    `json:"metadata,omitempty"`
}
