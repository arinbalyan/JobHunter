package db

import (
	"context"
	"fmt"
	"time"
)

// FullJobRecord matches the extended jobs schema.
type FullJobRecord struct {
	JobID           string
	Title           string
	Company         string
	CompanyURL      string
	JobURL          string
	JobURLDirect    string
	Location        string
	IsRemote        bool
	Description     string
	JobType         string
	DatePosted      *time.Time
	Source          string
	Seniority       string
	Department      string
	CompanyIndustry string
	SalaryMin       *float64
	SalaryMax       *float64
	SalaryCurrency  string
	SalaryInterval  string
	ExperienceRange string
	JobLevel        string
	QualityScore    int
	Emails          string
	Status          string
	SkipReason      string
	RecipientEmail  string
}

// QueueItem represents an email_queue entry with all job metadata.
type QueueItem struct {
	ID              int64
	JobID           int64
	PluginID        string
	RecipientEmail  string
	Company         string
	JobTitle        string
	JobURL          string
	JobLocation     string
	IsRemote        bool
	JobType         string
	JobDescription  string
	SalaryMin       *float64
	SalaryMax       *float64
	SalaryCurrency  string
	Seniority       string
	Skills          string
	CompanyIndustry string
	ExperienceMatch string
	Status          string
	CreatedAt       time.Time
}

// InsertJobFull inserts a full job record. Returns (id, isNew, error).
// isNew is false if the job URL already existed.
func (p *Pool) InsertJobFull(ctx context.Context, j *FullJobRecord) (int64, bool, error) {
	var id int64
	err := p.QueryRow(ctx,
		`INSERT INTO jobs
			(job_id, title, company, company_url, job_url, job_url_direct,
			 location, is_remote, description, job_type, date_posted, source,
			 seniority, department, company_industry,
			 salary_min, salary_max, salary_currency, salary_interval,
			 experience_range, job_level, quality_score,
			 emails, status, skip_reason, recipient_email, fetched_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
		         $16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,NOW())
		 ON CONFLICT (url) DO UPDATE SET
		   fetched_at = NOW(),
		   status = CASE WHEN jobs.status = 'new' THEN $24 ELSE jobs.status END
		 RETURNING id`,
		j.JobID, j.Title, j.Company, j.CompanyURL, j.JobURL, j.JobURLDirect,
		j.Location, j.IsRemote, j.Description, j.JobType, j.DatePosted, j.Source,
		j.Seniority, j.Department, j.CompanyIndustry,
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryInterval,
		j.ExperienceRange, j.JobLevel, j.QualityScore,
		j.Emails, j.Status, j.SkipReason, j.RecipientEmail,
	).Scan(&id)
	if err != nil {
		// If unique constraint violation — job exists, return existing ID
		return 0, false, nil
	}
	return id, true, nil
}

// UpdateJobStatus updates a job's status.
func (p *Pool) UpdateJobStatus(ctx context.Context, jobID int64, status, skipReason, recipientEmail string) error {
	_, err := p.Exec(ctx,
		`UPDATE jobs SET status = $1, skip_reason = $2, recipient_email = $3
		 WHERE id = $4`,
		status, skipReason, recipientEmail, jobID,
	)
	return err
}

// DeleteOldSkippedJobs deletes skipped jobs older than N days.
func (p *Pool) DeleteOldSkippedJobs(ctx context.Context, days int) (int, error) {
	tag, err := p.Exec(ctx,
		`DELETE FROM jobs WHERE status = 'skipped' AND fetched_at < NOW() - $1::INTERVAL`,
		fmt.Sprintf("%d days", days),
	)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// GetPendingQueue returns pending email_queue items, oldest first.
func (p *Pool) GetPendingQueue(ctx context.Context, limit int) ([]QueueItem, error) {
	rows, err := p.Query(ctx,
		`SELECT id, job_id, recipient_email, company, job_title, job_url,
		        job_location, is_remote, job_type, job_description,
		        salary_min, salary_max, salary_currency,
		        seniority, company_industry, experience_match, skills,
		        status, created_at
		 FROM email_queue
		 WHERE status = 'pending'
		 ORDER BY created_at ASC
		 LIMIT $1`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query pending queue: %w", err)
	}
	defer rows.Close()

	var items []QueueItem
	for rows.Next() {
		var item QueueItem
		err := rows.Scan(
			&item.ID, &item.JobID, &item.RecipientEmail, &item.Company,
			&item.JobTitle, &item.JobURL, &item.JobLocation, &item.IsRemote,
			&item.JobType, &item.JobDescription,
			&item.SalaryMin, &item.SalaryMax, &item.SalaryCurrency,
			&item.Seniority, &item.CompanyIndustry, &item.ExperienceMatch, &item.Skills,
			&item.Status, &item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan queue: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateQueueStatus updates a queue item's status.
func (p *Pool) UpdateQueueStatus(ctx context.Context, id int64, status, errorMsg string) error {
	_, err := p.Exec(ctx,
		`UPDATE email_queue SET status = $1, error_message = $2, updated_at = NOW()
		 WHERE id = $3`,
		status, errorMsg, id,
	)
	return err
}

// MarkStalePendingQueue marks items pending > N days as skipped.
func (p *Pool) MarkStalePendingQueue(ctx context.Context, days int) (int, error) {
	tag, err := p.Exec(ctx,
		`UPDATE email_queue SET status = 'skipped', error_message = 'stale',
		 updated_at = NOW()
		 WHERE status = 'pending' AND created_at < NOW() - $1::INTERVAL`,
		fmt.Sprintf("%d days", days),
	)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// FollowUpCandidate is a sent email eligible for follow-up.
type FollowUpCandidate struct {
	ID              int64
	JobID           int64
	RecipientEmail  string
	Company         string
	JobTitle        string
	SentAt          time.Time
}

// FollowUpRecord holds a follow-up queue entry.
type FollowUpRecord struct {
	JobID           int64
	OriginalEmailID int64
	RecipientEmail  string
	Domain          string
	Company         string
	JobTitle        string
	Subject         string
	Body            string
	TrackingID      string
	MessageID       string
	Status          string
}

// GetFollowUpCandidates finds sent emails from N+ days ago with no reply.
func (p *Pool) GetFollowUpCandidates(ctx context.Context, minDaysAgo int) ([]FollowUpCandidate, error) {
	rows, err := p.Query(ctx,
		`SELECT e.id, e.job_id, e.recipient_email, eq.company, eq.job_title, e.sent_at
		 FROM emails e
		 JOIN email_queue eq ON eq.job_id = e.job_id
		 WHERE e.status = 'sent'
		   AND NOT e.replied
		   AND NOT e.bounced
		   AND e.sent_at < NOW() - $1::INTERVAL
		   AND e.sent_at > NOW() - $2::INTERVAL
		   AND NOT EXISTS (
		     SELECT 1 FROM email_queue f
		     WHERE f.job_id = e.job_id AND f.status = 'pending'
		   )
		 ORDER BY e.sent_at ASC`,
		fmt.Sprintf("%d days", minDaysAgo),
		fmt.Sprintf("%d days", minDaysAgo+14), // Don't look back more than 14 days
	)
	if err != nil {
		return nil, fmt.Errorf("query follow-up candidates: %w", err)
	}
	defer rows.Close()

	var candidates []FollowUpCandidate
	for rows.Next() {
		var c FollowUpCandidate
		if err := rows.Scan(&c.ID, &c.JobID, &c.RecipientEmail, &c.Company, &c.JobTitle, &c.SentAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// HasFollowUpForDomain checks if a follow-up was sent to this domain recently.
func (p *Pool) HasFollowUpForDomain(ctx context.Context, domain string, hoursBack int) (bool, error) {
	var count int
	err := p.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_queue
		 WHERE domain = $1
		   AND status IN ('pending', 'sent')
		   AND created_at > NOW() - $2::INTERVAL`,
		domain, fmt.Sprintf("%d hours", hoursBack),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check follow-up domain: %w", err)
	}
	return count > 0, nil
}

// InsertFollowUp inserts a follow-up queue item.
func (p *Pool) InsertFollowUp(ctx context.Context, f *FollowUpRecord) (int64, error) {
	var id int64
	err := p.QueryRow(ctx,
		`INSERT INTO email_queue
			(job_id, plugin_id, recipient_email, company, job_title,
			 subject, body_preview, tracking_id, message_id, domain, status,
			 is_follow_up, original_email_id)
		 VALUES ($1, 'followup', $2, $3, $4, $5, $6, $7, $8, $9, $10, true, $11)
		 RETURNING id`,
		f.JobID, f.RecipientEmail, f.Company, f.JobTitle,
		f.Subject, f.Body, f.TrackingID, f.MessageID, f.Domain, f.Status,
		f.OriginalEmailID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert follow-up: %w", err)
	}
	return id, nil
}

// RecordRunLog inserts a run_log entry.
func (p *Pool) RecordRunLog(ctx context.Context, workflow, status string,
	scraped, pending, skipped, sent, failed, durationMs int, errMsg string) error {

	_, err := p.Exec(ctx,
		`INSERT INTO run_log
			(workflow, status, jobs_scraped, jobs_pending, jobs_skipped,
			 emails_sent, emails_failed, duration_ms, error_message)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		workflow, status, scraped, pending, skipped, sent, failed, durationMs, errMsg,
	)
	return err
}
