package db

import (
	"context"
	"fmt"
	"time"
)

// EmailRecord represents a sent email with tracking status.
type EmailRecord struct {
	ID             int64      `json:"id"`
	JobID          *int64     `json:"job_id,omitempty"`
	RecipientEmail string     `json:"recipient_email"`
	Subject        string     `json:"subject"`
	BodyPreview    string     `json:"body_preview"`
	SentAt         time.Time  `json:"sent_at"`
	Status         string     `json:"status"`
	TemplateUsed   string     `json:"template_used,omitempty"`
	TrackingID     string     `json:"tracking_id"`
	MessageID      string     `json:"message_id,omitempty"`
	Opened         bool       `json:"opened"`
	OpenedAt       *time.Time `json:"opened_at,omitempty"`
	Clicked        bool       `json:"clicked"`
	ClickedAt      *time.Time `json:"clicked_at,omitempty"`
	Replied        bool       `json:"replied"`
	RepliedAt      *time.Time `json:"replied_at,omitempty"`
	Bounced        bool       `json:"bounced"`
	BouncedAt      *time.Time `json:"bounced_at,omitempty"`
	BounceType     string     `json:"bounce_type,omitempty"`
}

// JobRecord represents a scraped job posting.
type JobRecord struct {
	ID          int64      `json:"id"`
	JobID       string     `json:"job_id"`
	Title       string     `json:"title"`
	Company     string     `json:"company"`
	Location    string     `json:"location,omitempty"`
	IsRemote    bool       `json:"is_remote"`
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url"`
	Source      string     `json:"source"`
	DatePosted  *time.Time `json:"date_posted,omitempty"`
	FetchedAt   time.Time  `json:"fetched_at"`
	Seniority   string     `json:"seniority,omitempty"`
	Emails      string     `json:"emails,omitempty"` // JSON array
}

// InsertEmail inserts a new email tracking record.
func (p *Pool) InsertEmail(ctx context.Context, e *EmailRecord) (int64, error) {
	var id int64
	err := p.QueryRow(ctx,
		`INSERT INTO emails
			(job_id, recipient_email, subject, body_preview, sent_at, status,
			 template_used, tracking_id, message_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id`,
		e.JobID, e.RecipientEmail, e.Subject, e.BodyPreview, e.SentAt,
		e.Status, e.TemplateUsed, e.TrackingID, e.MessageID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert email: %w", err)
	}
	return id, nil
}

// MarkOpened marks an email as opened by tracking ID.
func (p *Pool) MarkOpened(ctx context.Context, trackingID string) error {
	_, err := p.Exec(ctx,
		`UPDATE emails SET opened = true, opened_at = $1
		 WHERE tracking_id = $2 AND NOT opened`,
		time.Now().UTC(), trackingID,
	)
	if err != nil {
		return fmt.Errorf("mark opened: %w", err)
	}
	return nil
}

// MarkClicked marks an email as clicked by tracking ID.
func (p *Pool) MarkClicked(ctx context.Context, trackingID string) error {
	_, err := p.Exec(ctx,
		`UPDATE emails SET clicked = true, clicked_at = $1
		 WHERE tracking_id = $2 AND NOT clicked`,
		time.Now().UTC(), trackingID,
	)
	if err != nil {
		return fmt.Errorf("mark clicked: %w", err)
	}
	return nil
}

// MarkBounced marks an email as bounced.
func (p *Pool) MarkBounced(ctx context.Context, messageID string, bounceType string) error {
	_, err := p.Exec(ctx,
		`UPDATE emails SET bounced = true, bounced_at = $1, bounce_type = $2, status = 'bounced'
		 WHERE message_id = $3`,
		time.Now().UTC(), bounceType, messageID,
	)
	if err != nil {
		return fmt.Errorf("mark bounced: %w", err)
	}
	return nil
}

// MarkReplied marks an email as replied.
func (p *Pool) MarkReplied(ctx context.Context, messageID string) error {
	_, err := p.Exec(ctx,
		`UPDATE emails SET replied = true, replied_at = $1, status = 'replied'
		 WHERE message_id = $2`,
		time.Now().UTC(), messageID,
	)
	if err != nil {
		return fmt.Errorf("mark replied: %w", err)
	}
	return nil
}

// GetRecentEmails returns emails sent in the last N hours.
func (p *Pool) GetRecentEmails(ctx context.Context, hours int) ([]EmailRecord, error) {
	rows, err := p.Query(ctx,
		`SELECT id, recipient_email, subject, tracking_id, message_id, status, opened, clicked, replied, bounced, sent_at
		 FROM emails
		 WHERE sent_at > $1
		 ORDER BY sent_at DESC`,
		time.Now().Add(-time.Duration(hours)*time.Hour),
	)
	if err != nil {
		return nil, fmt.Errorf("query recent emails: %w", err)
	}
	defer rows.Close()

	var emails []EmailRecord
	for rows.Next() {
		var e EmailRecord
		err := rows.Scan(
			&e.ID, &e.RecipientEmail, &e.Subject, &e.TrackingID, &e.MessageID,
			&e.Status, &e.Opened, &e.Clicked, &e.Replied, &e.Bounced, &e.SentAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan email: %w", err)
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}

// InsertJob inserts a scraped job, skipping duplicates by URL.
func (p *Pool) InsertJob(ctx context.Context, j *JobRecord) (int64, bool, error) {
	var id int64
	err := p.QueryRow(ctx,
		`INSERT INTO jobs
			(job_id, title, company, location, is_remote, description, url, source,
			 date_posted, fetched_at, seniority, emails)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (url) DO NOTHING
		 RETURNING id`,
		j.JobID, j.Title, j.Company, j.Location, j.IsRemote, j.Description,
		j.URL, j.Source, j.DatePosted, j.FetchedAt, j.Seniority, j.Emails,
	).Scan(&id)
	if err != nil {
		// If the URL already exists, it's a duplicate — return 0, false
		return 0, false, nil
	}
	return id, true, nil
}

// GetRecentJobs returns all jobs fetched in the last N hours.
func (p *Pool) GetRecentJobs(ctx context.Context, hours int) ([]JobRecord, error) {
	rows, err := p.Query(ctx,
		`SELECT id, job_id, title, company, location, is_remote, description, url,
		        source, date_posted, fetched_at, seniority
		 FROM jobs
		 WHERE fetched_at > $1
		 ORDER BY fetched_at DESC`,
		time.Now().Add(-time.Duration(hours)*time.Hour),
	)
	if err != nil {
		return nil, fmt.Errorf("query recent jobs: %w", err)
	}
	defer rows.Close()

	var jobs []JobRecord
	for rows.Next() {
		var j JobRecord
		err := rows.Scan(
			&j.ID, &j.JobID, &j.Title, &j.Company, &j.Location, &j.IsRemote,
			&j.Description, &j.URL, &j.Source, &j.DatePosted, &j.FetchedAt, &j.Seniority,
		)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// GetSentEmailsCount returns how many emails were sent to a given address.
func (p *Pool) GetSentEmailsCount(ctx context.Context, email string, since time.Duration) (int, error) {
	var count int
	err := p.QueryRow(ctx,
		`SELECT COUNT(*) FROM emails
		 WHERE recipient_email = $1 AND sent_at > $2`,
		email, time.Now().Add(-since),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count sent emails: %w", err)
	}
	return count, nil
}
