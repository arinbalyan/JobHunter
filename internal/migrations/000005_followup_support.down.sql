DROP INDEX IF EXISTS idx_emails_followup;
DROP INDEX IF EXISTS idx_email_queue_followup;
DROP INDEX IF EXISTS idx_email_queue_domain;

ALTER TABLE email_queue DROP COLUMN IF EXISTS domain;
ALTER TABLE email_queue DROP COLUMN IF EXISTS is_follow_up;
ALTER TABLE email_queue DROP COLUMN IF EXISTS original_email_id;
ALTER TABLE email_queue DROP COLUMN IF EXISTS subject;
ALTER TABLE email_queue DROP COLUMN IF EXISTS body_preview;
