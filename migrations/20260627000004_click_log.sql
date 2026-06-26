CREATE TABLE IF NOT EXISTS click_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_id UUID REFERENCES email_queue(id),
    url TEXT NOT NULL,
    clicked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_click_log_email ON click_log(email_id);
