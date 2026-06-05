CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ─── Jobs: stores scraped job postings ─────────────────────────────
CREATE TABLE IF NOT EXISTS jobs (
    id              BIGSERIAL PRIMARY KEY,
    job_id          TEXT NOT NULL,
    title           TEXT NOT NULL,
    company         TEXT NOT NULL,
    location        TEXT DEFAULT '',
    is_remote       BOOLEAN DEFAULT FALSE,
    description     TEXT DEFAULT '',
    url             TEXT NOT NULL UNIQUE,
    source          TEXT NOT NULL DEFAULT '',
    date_posted     TIMESTAMPTZ,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    seniority       TEXT DEFAULT '',
    department      TEXT DEFAULT '',
    industry        TEXT DEFAULT '',
    compensation    JSONB DEFAULT '{}',
    emails          JSONB DEFAULT '[]',
    quality_score   INTEGER DEFAULT 0,
    raw_data        JSONB DEFAULT '{}'
);

-- ─── Emails: tracking for sent emails ──────────────────────────────
CREATE TABLE IF NOT EXISTS emails (
    id              BIGSERIAL PRIMARY KEY,
    job_id          BIGINT REFERENCES jobs(id) ON DELETE SET NULL,
    recipient_email TEXT NOT NULL,
    subject         TEXT NOT NULL DEFAULT '',
    body_preview    TEXT DEFAULT '',
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status          TEXT NOT NULL DEFAULT 'pending',
    template_used   TEXT DEFAULT '',
    tracking_id     TEXT NOT NULL UNIQUE,
    message_id      TEXT DEFAULT '',
    opened          BOOLEAN NOT NULL DEFAULT FALSE,
    opened_at       TIMESTAMPTZ,
    clicked         BOOLEAN NOT NULL DEFAULT FALSE,
    clicked_at      TIMESTAMPTZ,
    replied         BOOLEAN NOT NULL DEFAULT FALSE,
    replied_at      TIMESTAMPTZ,
    bounced         BOOLEAN NOT NULL DEFAULT FALSE,
    bounced_at      TIMESTAMPTZ,
    bounce_type     TEXT DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Indexes for performance ───────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_jobs_source_date   ON jobs(source, date_posted);
CREATE INDEX IF NOT EXISTS idx_jobs_fetched_at    ON jobs(fetched_at);
CREATE INDEX IF NOT EXISTS idx_emails_status      ON emails(status);
CREATE INDEX IF NOT EXISTS idx_emails_sent_at     ON emails(sent_at);
CREATE INDEX IF NOT EXISTS idx_emails_tracking    ON emails(tracking_id);
CREATE INDEX IF NOT EXISTS idx_emails_message_id  ON emails(message_id);
CREATE INDEX IF NOT EXISTS idx_emails_recipient   ON emails(recipient_email);
CREATE INDEX IF NOT EXISTS idx_jobs_url           ON jobs(url);

-- ─── Auto-update updated_at on emails ──────────────────────────────
CREATE OR REPLACE FUNCTION update_emails_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'emails_updated_at_trigger'
    ) THEN
        CREATE TRIGGER emails_updated_at_trigger
            BEFORE UPDATE ON emails
            FOR EACH ROW
            EXECUTE FUNCTION update_emails_updated_at();
    END IF;
END;
$$;
