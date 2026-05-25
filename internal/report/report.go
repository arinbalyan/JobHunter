package report

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/plugin"
	"github.com/arinbalyan/jobhunter/internal/stats"
	"github.com/arinbalyan/jobhunter/internal/telegram"
)

// RunReport aggregates data from all sources into a final report.
type RunReport struct {
	PluginResults []plugin.PluginRunResult
	StatsCollector *stats.Collector
	DB             *db.Pool
	StartTime      time.Time
	Config         interface {
		Getenv(string) string
		TelegramToken() string
		TelegramChat() string
	}
}

// Summary holds the final computed stats.
type Summary struct {
	TotalScraped    int
	JobsWithEmails  int
	EmailsSent      int
	EmailsFailed    int
	PendingCarryOver int
	SkippedDedup    int
	SkippedNoEmail  int
	TotalDuration   time.Duration
	DailyQuotaHit   bool
	GmailQuotaHit   bool
	CarryOverJobs   []db.EmailRecord
}

// Generate creates and sends the final report via Telegram.
func (r *RunReport) Generate(ctx context.Context) (*Summary, error) {
	summary := r.computeSummary()

	// Collect all metrics from all plugin results
	aggregateMetrics := make(map[string]float64)
	for _, pr := range r.PluginResults {
		for k, v := range pr.Metrics {
			aggregateMetrics[k] += v
		}
	}

	// Build Telegram report
	telegramItems := make([]telegram.PluginReportItem, len(r.PluginResults))
	for i, pr := range r.PluginResults {
		telegramItems[i] = telegram.PluginReportItem{
			PluginID:   pr.PluginID,
			PluginName: pr.PluginName,
			Success:    pr.Success,
			Message:    pr.Message,
			Duration:   pr.Duration,
			Metrics:    pr.Metrics,
		}
	}

	report := &telegram.ReportMessage{
		Title:         "JobHunter Run Complete",
		PluginResults: telegramItems,
		Stats:         summary.toMap(),
		Duration:      summary.TotalDuration,
		Timestamp:     time.Now().UTC(),
	}

	// Send via Telegram bot if configured
	tgBot := telegram.New(r.Config.TelegramToken(), r.Config.TelegramChat())
	if tgBot.Enabled() {
		if err := tgBot.SendReport(ctx, report); err != nil {
			return summary, fmt.Errorf("telegram report: %w", err)
		}
	}

	// Log summary to DB
	if r.DB != nil {
		if err := r.recordStats(ctx); err != nil {
			return summary, fmt.Errorf("record stats: %w", err)
		}
	}

	return summary, nil
}

func (r *RunReport) computeSummary() *Summary {
	s := &Summary{
		TotalDuration: time.Since(r.StartTime),
	}

	// Aggregate from plugin metrics
	for _, pr := range r.PluginResults {
		s.EmailsSent += int(pr.Metrics["emails_sent"])
		s.EmailsFailed += int(pr.Metrics["email_errors"])
		s.TotalScraped += int(pr.Metrics["jobs_scraped"])
		s.JobsWithEmails += int(pr.Metrics["jobs_matched"])

		if pr.Metrics["daily_quota_hit"] > 0 {
			s.DailyQuotaHit = true
		}
		if pr.Metrics["gmail_quota_hit"] > 0 {
			s.GmailQuotaHit = true
		}
		if pr.Metrics["skipped_dedup_exact"] > 0 || pr.Metrics["skipped_dedup_domain"] > 0 {
			s.SkippedDedup += int(pr.Metrics["skipped_dedup_exact"] + pr.Metrics["skipped_dedup_domain"])
		}
		s.SkippedNoEmail += int(pr.Metrics["skipped_no_recipients"])

		if pr.Metrics["pending_carried_over"] > 0 {
			s.PendingCarryOver += int(pr.Metrics["pending_carried_over"])
		}
	}

	return s
}

func (s *Summary) toMap() map[string]float64 {
	m := make(map[string]float64)
	m["total_scraped"] = float64(s.TotalScraped)
	m["jobs_with_emails"] = float64(s.JobsWithEmails)
	m["emails_sent"] = float64(s.EmailsSent)
	m["emails_failed"] = float64(s.EmailsFailed)
	m["skipped_dedup"] = float64(s.SkippedDedup)
	m["skipped_no_email"] = float64(s.SkippedNoEmail)
	m["pending_carry_over"] = float64(s.PendingCarryOver)
	m["duration_seconds"] = s.TotalDuration.Seconds()
	if s.DailyQuotaHit {
		m["daily_quota_hit"] = 1
	}
	if s.GmailQuotaHit {
		m["gmail_quota_hit"] = 1
	}
	return m
}

// SummaryLine returns a single-line summary for quick output.
func (s *Summary) SummaryLine() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("scraped=%d", s.TotalScraped))
	parts = append(parts, fmt.Sprintf("matched=%d", s.JobsWithEmails))
	parts = append(parts, fmt.Sprintf("sent=%d", s.EmailsSent))
	parts = append(parts, fmt.Sprintf("failed=%d", s.EmailsFailed))
	parts = append(parts, fmt.Sprintf("skipped=%d", s.SkippedDedup))
	if s.PendingCarryOver > 0 {
		parts = append(parts, fmt.Sprintf("carryover=%d", s.PendingCarryOver))
	}
	if s.DailyQuotaHit {
		parts = append(parts, "DAILY_QUOTA")
	}
	if s.GmailQuotaHit {
		parts = append(parts, "GMAIL_QUOTA")
	}
	return fmt.Sprintf("[%s] %s", formatDuration(s.TotalDuration), strings.Join(parts, " | "))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

func (r *RunReport) recordStats(ctx context.Context) error {
	if r.StatsCollector == nil {
		return nil
	}
	return r.StatsCollector.Flush(ctx, r.DB)
}

// ─── Carry-Over System ───────────────────────────────────────────────

// QuotaTracker tracks daily Gmail quotas and manages carry-over.
type QuotaTracker struct {
	DB            *db.Pool
	DailyLimit    int
	TodayCount    int
}

// NewQuotaTracker checks how many emails were sent today.
func NewQuotaTracker(ctx context.Context, pool *db.Pool, dailyLimit int) *QuotaTracker {
	todayCount := 0
	if pool != nil {
		if cnt, err := pool.GetTodaySentCount(ctx); err == nil {
			todayCount = cnt
		}
	}
	return &QuotaTracker{
		DB:         pool,
		DailyLimit: dailyLimit,
		TodayCount: todayCount,
	}
}

// Remaining returns how many emails can still be sent today.
func (q *QuotaTracker) Remaining() int {
	remaining := q.DailyLimit - q.TodayCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Exhausted returns true if daily quota has been reached.
func (q *QuotaTracker) Exhausted() bool {
	return q.TodayCount >= q.DailyLimit
}

// MarkSent increments the today counter.
func (q *QuotaTracker) MarkSent() {
	q.TodayCount++
}

// CarryOverManager handles pending jobs from previous runs.
type CarryOverManager struct {
	DB *db.Pool
}

// GetPendingJobs retrieves jobs that weren't sent due to quota limits.
func (m *CarryOverManager) GetPendingJobs(ctx context.Context) ([]db.EmailRecord, error) {
	if m.DB == nil {
		return nil, nil
	}
	return m.DB.GetPendingEmails(ctx)
}

// MarkProcessed marks a carry-over job as processed.
func (m *CarryOverManager) MarkProcessed(ctx context.Context, id int64) error {
	if m.DB == nil {
		return nil
	}
	return m.DB.MarkEmailProcessed(ctx, id)
}
