-- ponytail: LLM match score (1-10) for each job, NULL until scored
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS llm_score INT;
CREATE INDEX IF NOT EXISTS idx_jobs_llm_score ON jobs(llm_score);
