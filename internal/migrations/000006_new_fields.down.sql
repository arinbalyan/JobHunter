-- Migration 000006: Rollback — remove domain and company_description fields
ALTER TABLE jobs DROP COLUMN IF EXISTS domain;
ALTER TABLE jobs DROP COLUMN IF EXISTS company_description;
