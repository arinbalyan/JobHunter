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

// ReserveEmail atomically checks the dedup cooldown and inserts a reservation.
// Returns true if the email was reserved, false if blocked by cooldown.
// This is a single atomic SQL statement — no TOCTOU race possible.
func (p *Pool) ReserveEmail(ctx context.Context, e *EmailRecord, cooldownDays int) (bool, error) {
	var reserved bool
	err := p.QueryRow(ctx,
		`WITH reserved AS (
			INSERT INTO emails
				(job_id, recipient_email, subject, body_preview, sent_at, status,
				 template_used, tracking_id, message_id)
			SELECT $1, $2, $3, $4, NOW(), $6, $7, $8, $9
			WHERE NOT EXISTS (
				SELECT 1 FROM emails
				WHERE recipient_email = $2
				AND sent_at > NOW() - $10::INTERVAL
			)
			RETURNING id
		)
		SELECT EXISTS (SELECT 1 FROM reserved)`,
		e.JobID, e.RecipientEmail, e.Subject, e.BodyPreview,
		e.Status, e.TemplateUsed, e.TrackingID, e.MessageID,
		fmt.Sprintf("%d days", cooldownDays),
	).Scan(&reserved)
	if err != nil {
		return false, fmt.Errorf("reserve email: %w", err)
	}
	return reserved, nil
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

// GetTodaySentCount returns the number of emails sent today.
func (p *Pool) GetTodaySentCount(ctx context.Context) (int, error) {
	var count int
	err := p.QueryRow(ctx,
		`SELECT COUNT(*) FROM emails WHERE sent_at > CURRENT_DATE`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("today sent count: %w", err)
	}
	return count, nil
}

// MarkEmailSentByTrackingID updates an email record to 'sent' with message_id, looked up by tracking_id.
func (p *Pool) MarkEmailSentByTrackingID(ctx context.Context, trackingID, messageID string) error {
	_, err := p.Exec(ctx,
		`UPDATE emails SET status = 'sent', sent_at = NOW(), message_id = $2 WHERE tracking_id = $1`,
		trackingID, messageID,
	)
	if err != nil {
		return fmt.Errorf("mark email sent by tracking id: %w", err)
	}
	return nil
}
