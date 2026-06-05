-- Migration 000005: Follow-up support for email_queue

-- Add follow-up tracking columns
ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS domain          TEXT DEFAULT '';
ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS is_follow_up   BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS original_email_id BIGINT REFERENCES emails(id) ON DELETE SET NULL;
ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS subject         TEXT DEFAULT '';
ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS body_preview    TEXT DEFAULT '';

-- Index for follow-up lookups
CREATE INDEX IF NOT EXISTS idx_email_queue_domain ON email_queue(domain);
CREATE INDEX IF NOT EXISTS idx_email_queue_followup ON email_queue(is_follow_up, status);

-- Index for sent email follow-up candidates
CREATE INDEX IF NOT EXISTS idx_emails_followup ON emails(status, replied, bounced, sent_at);
