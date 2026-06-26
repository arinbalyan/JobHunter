-- ponytail: 4 tables, no more. Run log for stats, jobs/emails/queue for pipeline.

CREATE TABLE IF NOT EXISTS run_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow    TEXT NOT NULL,         -- scrape | send | followup | cleanup
    mode        TEXT,                  -- remote | onsite
    status      TEXT NOT NULL DEFAULT 'started', -- started | completed | failed
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    jobs_found    INT NOT NULL DEFAULT 0,
    emails_queued INT NOT NULL DEFAULT 0,
    emails_sent   INT NOT NULL DEFAULT 0,
    emails_failed  INT NOT NULL DEFAULT 0,
    error_msg   TEXT
);

CREATE TABLE IF NOT EXISTS jobs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_site  TEXT NOT NULL,
    title        TEXT NOT NULL,
    company_name TEXT NOT NULL DEFAULT '',
    company_url  TEXT NOT NULL DEFAULT '',
    job_url      TEXT NOT NULL,
    location     TEXT NOT NULL DEFAULT '',
    is_remote    BOOLEAN NOT NULL DEFAULT false,
    description  TEXT NOT NULL DEFAULT '',
    emails       JSONB NOT NULL DEFAULT '[]',
    quality_score INT NOT NULL DEFAULT 0,
    date_posted  TIMESTAMPTZ,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    run_id       UUID REFERENCES run_log(id)
);

CREATE TABLE IF NOT EXISTS email_queue (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id       UUID REFERENCES jobs(id),
    email_addr   TEXT NOT NULL,
    email_domain TEXT NOT NULL DEFAULT '',
    company_name TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',  -- pending | generating | generated | sending | sent | failed | skipped
    body         TEXT NOT NULL DEFAULT '',
    subject      TEXT NOT NULL DEFAULT '',
    llm_provider TEXT NOT NULL DEFAULT '',
    sent_at      TIMESTAMPTZ,
    error_msg    TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tracking (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_id     UUID REFERENCES email_queue(id),
    email_addr   TEXT NOT NULL,
    opened       BOOLEAN NOT NULL DEFAULT false,
    opened_at    TIMESTAMPTZ,
    clicks       INT NOT NULL DEFAULT 0,
    last_clicked_at TIMESTAMPTZ,
    replied      BOOLEAN NOT NULL DEFAULT false,
    reply_status TEXT,  -- positive | negative | neutral (from LLM triage)
    sent_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for dedup
CREATE INDEX IF NOT EXISTS idx_jobs_dedup ON jobs(company_name, job_url);
CREATE INDEX IF NOT EXISTS idx_email_queue_status ON email_queue(status);
CREATE INDEX IF NOT EXISTS idx_tracking_email ON tracking(email_id);
