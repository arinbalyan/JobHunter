package db

import (
	"context"
	"time"
)

// RecordWorkflowRun is a convenience wrapper around RecordRunLog.
func RecordWorkflowRun(ctx context.Context, pool *Pool, workflow, status string, scraped, pending, skipped, sent, failed int, dur time.Duration, errMsg string) {
	if pool == nil {
		return
	}
	_ = pool.RecordRunLog(ctx, workflow, status, scraped, pending, skipped, sent, failed, int(dur.Milliseconds()), errMsg)
}
