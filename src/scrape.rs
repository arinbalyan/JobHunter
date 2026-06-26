use crate::config::Config;
use crate::db;
use anyhow::Context;
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use std::process::Stdio;
use tokio::process::Command;

// ponytail: two types — ScraperInput in, Vec<JobPost> out. No intermediate structs.

// ── Types matching scrappy's ScraperInput ──────────────────
#[derive(Serialize)]
struct ScraperInput {
    search_terms: Vec<String>,
    locations: Vec<String>,
    sites: Vec<String>,
    remote_only: bool,
    results_wanted: i32,
    verify_email: bool,
    min_score: i32,
    hours_old: i32,
    description_format: String,
}

// ── Types matching scrappy's JobPost ───────────────────────
#[derive(Debug, Serialize, Deserialize, Clone)]
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
    pub id: Option<String>,
    pub title: String,
    pub company_name: Option<String>,
    pub company_url: Option<String>,
    pub job_url: String,
    pub location: Option<String>,
    #[serde(default)]
    pub is_remote: bool,
    pub description: Option<String>,
    pub date_posted: Option<String>,
    pub site: String,
    #[serde(default)]
    pub emails: Vec<Email>,
    #[serde(default)]
    pub quality_score: i32,
}

// ── Title rejection ────────────────────────────────────────
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

// ── Gentle email filter ────────────────────────────────────
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

pub async fn run(config: Config, mode: &str) -> anyhow::Result<()> {
    let preset = match mode {
        "remote" => &config.search.remote,
        "onsite" => &config.search.onsite,
        other => anyhow::bail!("unknown mode: {other}, use --mode remote|onsite"),
    };

    let scraper_path = find_scraper()?;

    let input = ScraperInput {
        search_terms: preset.terms.clone(),
        locations: preset.locations.clone(),
        sites: preset.sites.clone(),
        remote_only: preset.remote_only,
        results_wanted: 100000,
        verify_email: true,
        min_score: preset.min_score.unwrap_or(0),
        hours_old: preset.hours_old.unwrap_or(0),
        description_format: "markdown".to_string(),
    };

    let input_json = serde_json::to_string(&input)?;

    tracing::info!(
        "spawning scraper for {} terms, {} locations, {} sites",
        preset.terms.len(),
        preset.locations.len(),
        preset.sites.len()
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
        // ponytail: close stdin to signal EOF to the Go reader
        stdin.shutdown().await?;
    }

    let output = child.wait_with_output().await?;
    if !output.status.success() {
        anyhow::bail!("scraper exited with status {}", output.status);
    }

    let jobs: Vec<JobPost> = serde_json::from_slice(&output.stdout)
        .context("failed to parse scraper JSON output")?;

    tracing::info!("received {} jobs from scraper", jobs.len());

    let db_url = std::env::var("DATABASE_URL")
        .context("DATABASE_URL not set")?;
    let pool = db::connect(&db_url).await?;

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
            Err(e) => {
                tracing::warn!("skipping job {}: {}", job.title, e);
            }
        }
    }

    tracing::info!(
        "scrape done: {} received, {} title-filtered, {} email-filtered, {} inserted",
        jobs.len(), filtered_title, filtered_email, inserted
    );

    Ok(())
}

// ── DB insertion with atomic dedup ─────────────────────────
// ponytail: single INSERT with WHERE NOT EXISTS — no SELECT pre-check.

async fn insert_job(pool: &PgPool, job: &JobPost, emails: &[Email]) -> anyhow::Result<()> {
    let emails_json = serde_json::to_value(emails)?;
    let company = job.company_name.as_deref().unwrap_or("");

    sqlx::query(
        r#"
        INSERT INTO jobs (source_site, title, company_name, company_url, job_url,
                          location, is_remote, description, emails, quality_score,
                          date_posted, fetched_at)
        SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
               CASE WHEN $11::text IS NOT NULL AND $11::text != ''
                    THEN $11::timestamptz ELSE NULL END,
               now()
        WHERE NOT EXISTS (
            SELECT 1 FROM jobs WHERE job_url = $5
        )
        "#,
    )
    .bind(&job.site)
    .bind(&job.title)
    .bind(company)
    .bind(job.company_url.as_deref().unwrap_or(""))
    .bind(&job.job_url)
    .bind(job.location.as_deref().unwrap_or(""))
    .bind(job.is_remote)
    .bind(job.description.as_deref().unwrap_or(""))
    .bind(&emails_json)
    .bind(job.quality_score)
    .bind(&job.date_posted)
    .execute(pool)
    .await?;

    Ok(())
}

// ── Email queue insertion ───────────────────────────────────

async fn queue_emails(
    pool: &PgPool,
    job: &JobPost,
    emails: &[Email],
) -> anyhow::Result<()> {
    let company = job.company_name.as_deref().unwrap_or("");
    for email in emails {
        let domain = email.addr.split('@').nth(1).unwrap_or("").to_string();
        sqlx::query(
            r#"
            INSERT INTO email_queue (job_id, email_addr, email_domain, company_name, status)
            SELECT j.id, $2, $3, $4, 'pending'
            FROM jobs j
            WHERE j.job_url = $1
            AND NOT EXISTS (
                SELECT 1 FROM email_queue
                WHERE email_addr = $2
                  AND company_name = $4
                  AND created_at > now() - interval '30 days'
            )
            LIMIT 1
            "#,
        )
        .bind(&job.job_url)
        .bind(&email.addr)
        .bind(&domain)
        .bind(company)
        .execute(pool)
        .await?;
    }
    Ok(())
}

fn find_scraper() -> anyhow::Result<std::path::PathBuf> {
    let candidates = vec![
        std::path::PathBuf::from("./scraper"),
        std::path::PathBuf::from("./target/release/scraper"),
        std::path::PathBuf::from("/usr/local/bin/scraper"),
    ];
    for p in &candidates {
        if p.exists() {
            return Ok(p.clone());
        }
    }
    anyhow::bail!("scraper binary not found. Build: cd scraper && go build -o ../scraper .")
}
