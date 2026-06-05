package dedup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arinbalyan/jobhunter/internal/db"
)

// Deduplicator checks if we've already contacted a recipient.
type Deduplicator struct {
	DB                *db.Pool
	EmailCooldownDays int
}

// New creates a deduplicator.
func New(pool *db.Pool, emailCooldownDays int) *Deduplicator {
	return &Deduplicator{
		DB:                pool,
		EmailCooldownDays: emailCooldownDays,
	}
}

// CanSend checks if we can send to this recipient (SELECT-only pre-check).
// This is NOT the final gate — ReserveEmail's atomic INSERT is.
// Returns (allowed, reason).
func (d *Deduplicator) CanSend(ctx context.Context, recipientEmail string) (bool, string) {
	if d.DB == nil {
		return true, ""
	}
	cooldown := time.Duration(d.EmailCooldownDays) * 24 * time.Hour
	count, err := d.DB.GetSentEmailsCount(ctx, recipientEmail, cooldown)
	if err != nil {
		return true, "" // On error, allow send (fail open)
	}
	if count > 0 {
		return false, fmt.Sprintf("already sent to %s within %d days", recipientEmail, d.EmailCooldownDays)
	}
	return true, ""
}

// MarkSent atomically checks the dedup cooldown and reserves a slot.
// Uses ReserveEmail which is a single atomic SQL statement — no TOCTOU race.
// If the recipient is within cooldown, the INSERT is silently skipped.
func (d *Deduplicator) MarkSent(ctx context.Context, record *db.EmailRecord) error {
	if d.DB == nil {
		return nil
	}
	_, err := d.DB.ReserveEmail(ctx, record, d.EmailCooldownDays)
	return err
}

// IsRejectedTitle checks if a job title should be rejected.
// NOTE: Duplicated in internal/config/yaml_config.go (YAMLConfig.RejectTitle).
// Both are tested; yaml_config.go is the canonical production version.
func IsRejectedTitle(title string, rejectPatterns []string) bool {
	titleLow := strings.ToLower(title)
	for _, pattern := range rejectPatterns {
		if strings.Contains(titleLow, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// FilterEmail checks if an email matches any filter pattern.
// NOTE: Duplicated in internal/config/yaml_config.go (YAMLConfig.FilterEmail).
// Both are tested; yaml_config.go is the canonical production version.
func FilterEmail(email string, filterPatterns []string) bool {
	emailLow := strings.ToLower(email)
	for _, pattern := range filterPatterns {
		patLow := strings.ToLower(pattern)
		switch {
		case strings.HasPrefix(patLow, "starts_with:"):
			prefix := strings.TrimPrefix(patLow, "starts_with:")
			if strings.HasPrefix(emailLow, prefix) {
				return true
			}
		case strings.HasPrefix(patLow, "contains:"):
			substr := strings.TrimPrefix(patLow, "contains:")
			if strings.Contains(emailLow, substr) {
				return true
			}
		case strings.HasPrefix(patLow, "tld:"):
			tld := strings.TrimPrefix(patLow, "tld:")
			if strings.HasSuffix(emailLow, tld) {
				return true
			}
		}
	}
	return false
}

// FilterEmails filters a list of emails, returning valid and invalid.
func FilterEmails(emails []string, filterPatterns []string) (valid, invalid []string) {
	for _, e := range emails {
		if FilterEmail(e, filterPatterns) {
			invalid = append(invalid, e)
		} else {
			valid = append(valid, e)
		}
	}
	return
}
