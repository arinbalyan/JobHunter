-- Migration 000003: Stats and plugin tracking tables

-- ─── Stats: time-series data for all events across all plugins ────────────
CREATE TABLE IF NOT EXISTS stats (
    id          BIGSERIAL PRIMARY KEY,
    plugin_id   TEXT NOT NULL DEFAULT '',
    event       TEXT NOT NULL,           -- email_sent, email_opened, scrape_complete, plugin_run, etc.
    value       DOUBLE PRECISION NOT NULL DEFAULT 0,
    tags        JSONB NOT NULL DEFAULT '{}',  -- {"source":"linkedin","status":"success"}
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stats_event       ON stats(event);
CREATE INDEX IF NOT EXISTS idx_stats_plugin      ON stats(plugin_id);
CREATE INDEX IF NOT EXISTS idx_stats_recorded_at ON stats(recorded_at);
CREATE INDEX IF NOT EXISTS idx_stats_event_time  ON stats(event, recorded_at);

-- ─── Plugin state: tracks per-plugin metadata and last run ────────────────
CREATE TABLE IF NOT EXISTS plugin_state (
    id              BIGSERIAL PRIMARY KEY,
    plugin_id       TEXT NOT NULL UNIQUE,
    plugin_name     TEXT NOT NULL DEFAULT '',
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at     TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    run_count       INTEGER NOT NULL DEFAULT 0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    config_json     JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Job Applications: tracks entire application pipeline ─────────────────
CREATE TABLE IF NOT EXISTS applications (
    id              BIGSERIAL PRIMARY KEY,
    plugin_id       TEXT NOT NULL DEFAULT '',
    job_id          BIGINT REFERENCES jobs(id) ON DELETE SET NULL,
    company         TEXT NOT NULL,
    title           TEXT NOT NULL,
    email_sent_to   TEXT NOT NULL DEFAULT '',
    stage           TEXT NOT NULL DEFAULT 'sent',
    -- Pipeline: queued → sent → delivered → opened → replied → interview → offer → accepted | rejected
    score           INTEGER DEFAULT 0,
    notes           TEXT DEFAULT '',
    sent_at         TIMESTAMPTZ,
    opened_at       TIMESTAMPTZ,
    replied_at      TIMESTAMPTZ,
    interview_at    TIMESTAMPTZ,
    offer_at        TIMESTAMPTZ,
    rejected_at     TIMESTAMPTZ,
    outcome         TEXT DEFAULT '',
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_applications_stage    ON applications(stage);
CREATE INDEX IF NOT EXISTS idx_applications_company  ON applications(company);
CREATE INDEX IF NOT EXISTS idx_applications_plugin   ON applications(plugin_id);

-- ─── Blacklist: domains/email addresses that bounced or rejected ──────────
CREATE TABLE IF NOT EXISTS blacklist (
    id              BIGSERIAL PRIMARY KEY,
    pattern         TEXT NOT NULL UNIQUE,  -- domain or email pattern
    reason          TEXT NOT NULL DEFAULT '',  -- bounced | spam_complaint | manual
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,
    hit_count       INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_blacklist_pattern ON blacklist(pattern);
