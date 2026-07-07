-- ponytail: tag jobs with scrape mode so email generation can pick per-mode context/templates
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS scrape_mode TEXT;
