-- Migration 000004: Extend jobs table with full scrappy data, add pending queue

-- ─── Extend jobs table with complete scrappy fields ─────────────────
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS company_url     TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS job_url_direct  TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS job_type        TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS date_posted     TIMESTAMPTZ;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS salary_min      DOUBLE PRECISION;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS salary_max      DOUBLE PRECISION;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS salary_currency TEXT DEFAULT 'USD';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS salary_interval TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS skills          JSONB DEFAULT '[]';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS experience_range TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS job_level       TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS company_industry TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS company_logo_url TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS apply_method    TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS quality_score   INTEGER DEFAULT 0;

-- ─── Job Status Tracking ────────────────────────────────────────────
-- status values:
--   new        - freshly scraped, not yet evaluated
--   pending    - eligible for sending, queued
--   sent       - email sent
--   skipped    - skipped (dedup, no email, title reject, filtered)
--   failed     - SMTP error
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS status          TEXT NOT NULL DEFAULT 'new';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS skip_reason     TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS recipient_email TEXT DEFAULT '';

-- Index for the pending queue
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_status_fetched ON jobs(status, fetched_at);

-- ─── Email Queue Table ──────────────────────────────────────────────
-- Separate table for queued emails that the sender workflow picks up.
CREATE TABLE IF NOT EXISTS email_queue (
    id              BIGSERIAL PRIMARY KEY,
    job_id          BIGINT REFERENCES jobs(id) ON DELETE CASCADE,
    plugin_id       TEXT NOT NULL DEFAULT '',
    recipient_email TEXT NOT NULL,
    company         TEXT NOT NULL DEFAULT '',
    job_title       TEXT NOT NULL DEFAULT '',
    job_url         TEXT NOT NULL DEFAULT '',
    job_location    TEXT DEFAULT '',
    is_remote       BOOLEAN DEFAULT FALSE,
    job_type        TEXT DEFAULT '',
    job_description TEXT DEFAULT '',
    salary_min      DOUBLE PRECISION,
    salary_max      DOUBLE PRECISION,
    salary_currency TEXT DEFAULT 'USD',
    seniority       TEXT DEFAULT '',
    experience_range TEXT DEFAULT '',
    company_industry TEXT DEFAULT '',
    skills          JSONB DEFAULT '[]',
    source_site     TEXT DEFAULT '',
    quality_score   INTEGER DEFAULT 0,
    match_score     INTEGER DEFAULT 0,
    experience_match TEXT DEFAULT '',  -- qualified | underqualified | overqualified
    template        TEXT DEFAULT '',
    tracking_id     TEXT DEFAULT '',
    message_id      TEXT DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending',
    -- pending | sent | failed | bounced | skipped
    error_message   TEXT DEFAULT '',
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_queue_status ON email_queue(status);
CREATE INDEX IF NOT EXISTS idx_email_queue_created ON email_queue(created_at);
CREATE INDEX IF NOT EXISTS idx_email_queue_job ON email_queue(job_id);

-- ─── Run Log Table ──────────────────────────────────────────────────
-- Tracks every workflow run for audit/telemetry.
CREATE TABLE IF NOT EXISTS run_log (
    id              BIGSERIAL PRIMARY KEY,
    workflow        TEXT NOT NULL DEFAULT '',  -- scrape | send | cleanup
    status          TEXT NOT NULL DEFAULT '',
    jobs_scraped    INTEGER DEFAULT 0,
    jobs_pending    INTEGER DEFAULT 0,
    jobs_skipped    INTEGER DEFAULT 0,
    emails_sent     INTEGER DEFAULT 0,
    emails_failed   INTEGER DEFAULT 0,
    duration_ms     INTEGER DEFAULT 0,
    error_message   TEXT DEFAULT '',
    run_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_run_log_workflow ON run_log(workflow, run_at);
