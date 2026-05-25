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

// QueueItem represents an email_queue entry.
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
		        salary_min, salary_max, status, created_at
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
			&item.SalaryMin, &item.SalaryMax, &item.Status, &item.CreatedAt,
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
