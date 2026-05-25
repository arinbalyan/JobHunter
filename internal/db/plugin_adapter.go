package db

import (
	"context"
	"fmt"

	"github.com/arinbalyan/jobhunter/internal/plugin/sdk"
)

// PluginDB adapts the Pool to the plugin SDK's Database interface.
type PluginDB struct {
	pool *Pool
}

// NewPluginDB creates a new plugin DB adapter.
func NewPluginDB(pool *Pool) *PluginDB {
	return &PluginDB{pool: pool}
}

func (p *PluginDB) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := p.pool.Exec(ctx, query, args...)
	return err
}

func (p *PluginDB) Query(ctx context.Context, query string, args ...interface{}) (sdk.Rows, error) {
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &pluginRows{rows: rows}, nil
}

func (p *PluginDB) InsertEmail(ctx context.Context, e *sdk.EmailRecord) (int64, error) {
	// Convert plugin sdk.EmailRecord to our internal db.EmailRecord
	record := &EmailRecord{
		RecipientEmail: e.RecipientEmail,
		Subject:        e.Subject,
		BodyPreview:    e.BodyPreview,
		SentAt:         e.SentAt,
		Status:         e.Status,
		TrackingID:     e.TrackingID,
		MessageID:      e.MessageID,
	}
	// Store plugin_id in body_preview prefix or a separate metadata field
	// We use status prefix for now: "plugin_id:status"
	if e.PluginID != "" {
		record.Status = fmt.Sprintf("%s:%s", e.PluginID, e.Status)
	}
	return p.pool.InsertEmail(ctx, record)
}

func (p *PluginDB) RecordStat(ctx context.Context, s *sdk.StatEntry) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO stats (plugin_id, event, value, tags, recorded_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.PluginID, s.Event, s.Value, s.Tags, s.Timestamp,
	)
	return err
}

// pluginRows wraps pgx.Rows for the sdk.Rows interface.
type pluginRows struct {
	rows interface {
		Next() bool
		Scan(dest ...interface{}) error
		Close()
	}
}

func (r *pluginRows) Next() bool                  { return r.rows.Next() }
func (r *pluginRows) Scan(dest ...interface{}) error { return r.rows.Scan(dest...) }
func (r *pluginRows) Close() error                 { r.rows.Close(); return nil }
