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


