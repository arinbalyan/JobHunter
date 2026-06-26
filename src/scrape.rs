use crate::config::Config;
use crate::db;
use anyhow::Context;
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use std::process::Stdio;
use tokio::process::Command;

// ponytail: bridge receives ScraperInput + extra fields (timeout_seconds) in one JSON.

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Mode { Remote, Onsite }

impl From<&str> for Mode {
    fn from(s: &str) -> Self {
        match s.to_lowercase().as_str() {
            "onsite" => Mode::Onsite,
            _ => Mode::Remote,
        }
    }
}

#[derive(Serialize)]
struct BridgeInput {
    #[serde(flatten)]
    scraper_input: ScraperInput,
    timeout_seconds: i32,
}

#[derive(Serialize)]
struct ScraperInput {
    search_terms: Vec<String>,
    locations: Vec<String>,
    sites: Vec<String>,
    remote_only: bool,
    results_wanted: i32,
    verify_email: bool,
    description_format: String,
}

#[derive(Debug, Deserialize, Clone, Serialize)]
pub struct Email {
    pub addr: String,
    #[serde(default)]
    pub verified: bool,
    #[serde(default)]
    pub source: String,
    #[serde(default)]
    pub role: bool,
}

#[derive(Debug, Deserialize)]
pub struct JobPost {
    pub title: String,
    pub company_name: Option<String>,
    pub company_url: Option<String>,
    pub job_url: String,
    pub location: Option<String>,
    #[serde(default)]
    pub is_remote: bool,
    pub description: Option<String>,
    pub site: String,
    #[serde(default)]
    pub emails: Vec<Email>,
    #[serde(default)]
    pub quality_score: i32,
}

const REJECTED_TITLES: &[&str] = &[
    "senior", "sr ", "staff", "principal", "lead", "manager",
    "director", "head of", "vp ", "vice president", "chief",
    "embedded", "firmware", "hardware", "qa ", "quality",
    "tester", "test engineer", "sdet", "manual",
    "data scientist", "data analyst", "data engineer",
    "devops engineer", "site reliability", "platform engineer",
    "network", "security engineer", "infosec", "sysadmin",
    "admin", "support", "it ", "help desk",
    "trainee", "apprentice", "intern", "fresher",
    "graduate", "entry level", "junior",
];

fn is_title_rejected(title: &str) -> bool {
    let lower = title.to_lowercase();
    REJECTED_TITLES.iter().any(|p| lower.contains(p))
}

const BLOCKED_EMAIL_PREFIXES: &[&str] = &["no-reply", "noreply", "do-not-reply", "donotreply"];
const BLOCKED_TLDS: &[&str] = &[".test", ".example", ".invalid", ".local", ".localhost"];

fn is_email_filtered(addr: &str) -> bool {
    let lower = addr.to_lowercase();
    BLOCKED_EMAIL_PREFIXES.iter().any(|p| lower.starts_with(p))
        || BLOCKED_TLDS.iter().any(|t| lower.ends_with(t))
}

fn filter_emails(emails: &[Email]) -> Vec<Email> {
    emails.iter().filter(|e| !is_email_filtered(&e.addr)).cloned().collect()
}

fn has_valid_email(emails: &[Email]) -> bool {
    emails.iter().any(|e| !is_email_filtered(&e.addr))
}

// ── Scrape execution ───────────────────────────────────────

pub struct ScrapeResult {
    pub mode: Mode,
    pub carried_over: i64,
    pub received: usize,
    pub filtered_title: usize,
    pub filtered_email: usize,
    pub inserted: usize,
    pub sites_count: usize,
    pub terms_count: usize,
    pub duration_secs: f64,
}

pub async fn run(config: Config, mode: Mode) -> anyhow::Result<ScrapeResult> {
    let start = std::time::Instant::now();
    let db_url = std::env::var("DATABASE_URL").context("DATABASE_URL not set")?;
    let pool = db::connect(&db_url).await?;

    // ── carry over unqueued jobs from previous runs ──
    let carried_over = carry_over_pending(&pool).await?;

    // ── run scraper ──
    let scraper_path = find_scraper()?;
    let preset = match mode {
        Mode::Onsite => &config.search.onsite,
        Mode::Remote => &config.search.remote,
    };
    let sites_count = preset.sites.len();
    let terms_count = preset.terms.len();
    let scrape_cfg = &config.scrape;

    let max_runtime_secs = scrape_cfg.max_runtime_minutes.unwrap_or(490) * 60;
    let results_wanted = scrape_cfg.results_wanted.unwrap_or(0);

    let input = BridgeInput {
        scraper_input: ScraperInput {
            search_terms: preset.terms.clone(),
            locations: preset.locations.clone(),
            sites: preset.sites.clone(),
            remote_only: preset.remote_only,
            results_wanted,
            verify_email: true,
            description_format: "markdown".to_string(),
        },
        timeout_seconds: max_runtime_secs,
    };

    let input_json = serde_json::to_string(&input)?;

    tracing::info!(
        "spawning scraper — {} locations, {} sites, {}s timeout, {} results_wanted",
        preset.locations.len(), preset.sites.len(), max_runtime_secs, results_wanted
    );

    let mut child = Command::new(&scraper_path)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::inherit())
        .spawn()
        .context("failed to spawn Go scraper subprocess")?;

    {
        let stdin = child.stdin.as_mut().unwrap();
        use tokio::io::AsyncWriteExt;
        stdin.write_all(input_json.as_bytes()).await?;
        stdin.shutdown().await?;
    }

    let output = child.wait_with_output().await?;
    if !output.status.success() {
        anyhow::bail!("scraper exited with status {}", output.status);
    }

    let jobs: Vec<JobPost> = serde_json::from_slice(&output.stdout)
        .context("failed to parse scraper JSON output")?;

    tracing::info!("received {} jobs from scraper", jobs.len());

    let mut inserted = 0;
    let mut filtered_title = 0;
    let mut filtered_email = 0;

    for job in &jobs {
        if is_title_rejected(&job.title) {
            filtered_title += 1;
            continue;
        }
        if !has_valid_email(&job.emails) {
            filtered_email += 1;
            continue;
        }
        let clean_emails = filter_emails(&job.emails);
        match insert_job(&pool, job, &clean_emails).await {
            Ok(_) => {
                if let Err(e) = queue_emails(&pool, job, &clean_emails).await {
                    tracing::warn!("failed to queue emails for {}: {}", job.title, e);
                }
                inserted += 1;
            }
            Err(e) => tracing::warn!("skipping job {}: {}", job.title, e),
        }
    }

    tracing::info!(
        "scrape done: {} received, {} title-filtered, {} email-filtered, {} inserted",
        jobs.len(), filtered_title, filtered_email, inserted
    );

    let duration_secs = start.elapsed().as_secs_f64();
    tracing::info!("scrape completed in {:.1}s", duration_secs);

    let mode_str = match mode { Mode::Remote => "remote", Mode::Onsite => "onsite" };
    db::write_run_log(&pool, "scrape", Some(mode_str),
        jobs.len() as i32, inserted as i32, 0, 0, None).await;

    Ok(ScrapeResult { mode, carried_over, received: jobs.len(), filtered_title, filtered_email, inserted, sites_count, terms_count, duration_secs })
}

// ── DB operations ──────────────────────────────────────────

async fn insert_job(pool: &PgPool, job: &JobPost, emails: &[Email]) -> anyhow::Result<()> {
    let emails_json = serde_json::to_value(emails)?;
    sqlx::query(
        r#"
        INSERT INTO jobs (source_site, title, company_name, company_url, job_url,
                          location, is_remote, description, emails, quality_score, fetched_at)
        SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now()
        WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE job_url = $5)
        "#,
    )
    .bind(&job.site)
    .bind(&job.title)
    .bind(job.company_name.as_deref().unwrap_or(""))
    .bind(job.company_url.as_deref().unwrap_or(""))
    .bind(&job.job_url)
    .bind(job.location.as_deref().unwrap_or(""))
    .bind(job.is_remote)
    .bind(job.description.as_deref().unwrap_or(""))
    .bind(&emails_json)
    .bind(job.quality_score)
    .execute(pool)
    .await?;
    Ok(())
}

async fn queue_emails(pool: &PgPool, job: &JobPost, emails: &[Email]) -> anyhow::Result<()> {
    for email in emails {
        let domain = email.addr.split('@').nth(1).unwrap_or("").to_string();
        sqlx::query(
            r#"
            INSERT INTO email_queue (job_id, email_addr, email_domain, company_name, status)
            SELECT j.id, $2, $3, $4, 'pending'
            FROM jobs j WHERE j.job_url = $1
            AND NOT EXISTS (
                SELECT 1 FROM email_queue
                WHERE email_addr = $2 AND company_name = $4
                AND created_at > now() - interval '30 days'
            )
            LIMIT 1
            "#,
        )
        .bind(&job.job_url)
        .bind(&email.addr)
        .bind(&domain)
        .bind(job.company_name.as_deref().unwrap_or(""))
        .execute(pool)
        .await?;
    }
    Ok(())
}

/// Queue emails from jobs in the last 7 days that haven't been queued yet.
/// Returns count of newly queued emails.
async fn carry_over_pending(pool: &PgPool) -> anyhow::Result<i64> {
    // ponytail: one INSERT-SELECT gets all unqueued emails from recent jobs.
    let result = sqlx::query(
        r#"
        INSERT INTO email_queue (job_id, email_addr, email_domain, company_name, status)
        SELECT j.id, e->>'addr', split_part(e->>'addr', '@', 2), j.company_name, 'pending'
        FROM jobs j
        CROSS JOIN LATERAL jsonb_array_elements(j.emails) AS e
        WHERE j.fetched_at > now() - interval '7 days'
        AND NOT EXISTS (
            SELECT 1 FROM email_queue eq
            WHERE eq.job_id = j.id AND eq.email_addr = e->>'addr'
        )
        AND NOT EXISTS (
            SELECT 1 FROM email_queue eq
            WHERE eq.email_addr = e->>'addr'
            AND eq.company_name = j.company_name
            AND eq.created_at > now() - interval '30 days'
        )
        "#,
    )
    .execute(pool)
    .await
    .context("failed to carry over pending jobs")?;

    let count = result.rows_affected() as i64;
    if count > 0 {
        tracing::info!("carried over {} unqueued emails from previous runs", count);
    }
    Ok(count)
}

fn find_scraper() -> anyhow::Result<std::path::PathBuf> {
    let candidates = vec![
        std::path::PathBuf::from("./scraper"),
        std::path::PathBuf::from("/usr/local/bin/scraper"),
    ];
    for p in &candidates { if p.exists() { return Ok(p.clone()); } }
    anyhow::bail!("scraper binary not found. Build: cd scraper && go build -o ../scraper .")
}
