DROP TRIGGER IF EXISTS emails_updated_at_trigger ON emails;
DROP FUNCTION IF EXISTS update_emails_updated_at();

DROP TABLE IF EXISTS emails;
DROP TABLE IF EXISTS jobs;
