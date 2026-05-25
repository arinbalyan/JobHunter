-- Migration 000002: Add email_performance table for learning/optimization
-- This tracks template performance across different job types for A/B learning.

CREATE TABLE IF NOT EXISTS email_performance (
    id              BIGSERIAL PRIMARY KEY,
    template_name   TEXT NOT NULL,
    job_seniority   TEXT DEFAULT '',
    experience_match TEXT DEFAULT '',  -- qualified | underqualified | overqualified
    industry        TEXT DEFAULT '',
    sent_count      INTEGER NOT NULL DEFAULT 0,
    open_count      INTEGER NOT NULL DEFAULT 0,
    click_count     INTEGER NOT NULL DEFAULT 0,
    reply_count     INTEGER NOT NULL DEFAULT 0,
    bounce_count    INTEGER NOT NULL DEFAULT 0,
    last_updated    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(template_name, job_seniority, experience_match, industry)
);

CREATE INDEX IF NOT EXISTS idx_email_performance_template ON email_performance(template_name);

-- Migration 000002: Add agent_rules table for behavioral guidelines
CREATE TABLE IF NOT EXISTS agent_rules (
    id              BIGSERIAL PRIMARY KEY,
    rule_category   TEXT NOT NULL DEFAULT '',
    rule_text       TEXT NOT NULL,
    priority        INTEGER NOT NULL DEFAULT 1,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Migration 000002: Add user_context table for profile persistence
CREATE TABLE IF NOT EXISTS user_context (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT DEFAULT '',
    current_role        TEXT DEFAULT '',
    years_experience    INTEGER DEFAULT 0,
    location            TEXT DEFAULT '',
    remote_preference   TEXT DEFAULT '',
    target_roles        JSONB DEFAULT '[]',
    industries          JSONB DEFAULT '[]',
    skills              JSONB DEFAULT '[]',
    salary_expectation  JSONB DEFAULT '{}',
    resume_link         TEXT DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
