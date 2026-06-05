-- Migration 000006: Add domain and company_description fields
-- These are provided by scrappy's JobPost but weren't stored in DB.

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS domain               TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS company_description  TEXT DEFAULT '';
