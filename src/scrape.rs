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
    email_enrich: bool,
    description_format: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    site_search: Option<std::collections::HashMap<String, Vec<String>>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    site_location: Option<std::collections::HashMap<String, String>>,
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

#[derive(Debug, Default, Deserialize)]
pub struct Location {
    pub city: Option<String>,
    pub state: Option<String>,
    pub country: Option<String>,
}

impl Location {
    fn display(&self) -> String {
        let parts: Vec<&str> = [self.city.as_deref(), self.state.as_deref(), self.country.as_deref()]
            .into_iter().flatten().filter(|s| !s.is_empty()).collect();
        if parts.is_empty() { "remote".into() } else { parts.join(", ") }
    }
}

#[derive(Debug, Deserialize)]
pub struct JobPost {
    pub title: String,
    pub company_name: Option<String>,
    pub company_url: Option<String>,
    pub job_url: String,
    #[serde(default)]
    pub location: Option<Location>,
    #[serde(default)]
    pub is_remote: bool,
    pub description: Option<String>,
    pub site: String,
    #[serde(default)]
    pub emails: Vec<Email>,
    #[serde(default)]
    pub quality_score: i32,
}

fn is_title_rejected(title: &str, patterns: &[String]) -> bool {
    let lower = title.to_lowercase();
    patterns.iter().any(|p| lower.contains(&p.to_lowercase()))
}

fn is_email_filtered(addr: &str, cfg: &crate::config::ScrapeConfig) -> bool {
    let lower = addr.to_lowercase();
    let prefixes = cfg.blocked_email_prefixes.as_deref().unwrap_or(&[]);
    let contains = cfg.blocked_email_contains.as_deref().unwrap_or(&[]);
    let tlds = cfg.blocked_tlds.as_deref().unwrap_or(&[]);
    prefixes.iter().any(|p| lower.starts_with(&p.to_lowercase()))
        || contains.iter().any(|p| lower.contains(&p.to_lowercase()))
        || tlds.iter().any(|t| lower.ends_with(&t.to_lowercase()))
}

fn filter_emails(emails: &[Email], cfg: &crate::config::ScrapeConfig) -> Vec<Email> {
    emails.iter().filter(|e| !is_email_filtered(&e.addr, cfg)).cloned().collect()
}

fn has_valid_email(emails: &[Email], cfg: &crate::config::ScrapeConfig) -> bool {
    emails.iter().any(|e| !is_email_filtered(&e.addr, cfg))
}

/// Build per-site search/location maps from optional config section.
fn build_site_config(sites: &Option<std::collections::HashMap<String, crate::config::SiteConfig>>)
    -> (Option<std::collections::HashMap<String, Vec<String>>>, Option<std::collections::HashMap<String, String>>)
{
    let sites = match sites { Some(s) => s, None => return (None, None) };
    let mut search = std::collections::HashMap::new();
    let mut location = std::collections::HashMap::new();
    for (name, cfg) in sites {
        if !cfg.search_terms.is_empty() {
            search.insert(name.clone(), cfg.search_terms.clone());
        }
        if let Some(ref loc) = cfg.location {
            location.insert(name.clone(), loc.clone());
        }
    }
    let s = if search.is_empty() { None } else { Some(search) };
    let l = if location.is_empty() { None } else { Some(location) };
    (s, l)
}

// ── Scrape execution ───────────────────────────────────────

pub struct ScrapeResult {
    pub mode: Mode,
    pub carried_over: i64,
    pub received: usize,
    pub filtered_title: usize,
    pub filtered_email: usize,
    pub inserted: usize,
    pub dedup_skipped: usize,
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

    let scrape_mode_str = match mode { Mode::Remote => "remote", Mode::Onsite => "onsite" };
    let (site_search, site_location) = build_site_config(&config.sites);

    let input = BridgeInput {
        scraper_input: ScraperInput {
            search_terms: preset.terms.clone(),
            locations: preset.locations.clone(),
            sites: preset.sites.clone(),
            remote_only: preset.remote_only,
            results_wanted,
            verify_email: true,
            email_enrich: false,  // ponytail: generic hr@/careers@ inboxes, not real people
            description_format: "markdown".to_string(),
            site_search,
            site_location,
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

    // ponytail: read NDJSON line-by-line, insert incrementally.
    // A killed process only loses in-flight lines, not the entire run.
    let stdout = child.stdout.take().context("no stdout from scraper")?;
    let reader = tokio::io::BufReader::new(stdout);
    use tokio::io::AsyncBufReadExt;
    let mut lines = reader.lines();

    let mut inserted = 0;
    let mut dedup_skipped = 0;
    let mut filtered_title = 0;
    let mut filtered_email = 0;
    let mut parse_errors = 0;

    // ponytail: read lines until EOF or broken pipe. Don't propagate read errors
    // — GH may kill the job at the timeout, and we still want run_log + Telegram.
    loop {
        let line = match lines.next_line().await {
            Ok(Some(l)) => l,
            Ok(None) => break,
            Err(e) => {
                tracing::warn!("scraper stdout read error: {} — partial results saved", e);
                break;
            }
        };
        if line.trim().is_empty() { continue; }

        // NDJSON first line may be site_stats metadata — skip it (handled elsewhere)
        if line.contains("\"type\":\"site_stats\"") {
            continue;
        }

        let job: JobPost = match serde_json::from_str(&line) {
            Ok(j) => j,
            Err(e) => {
                parse_errors += 1;
                if parse_errors <= 5 {
                    tracing::warn!("failed to parse job line: {}", e);
                }
                continue;
            }
        };

        let reject_pats = scrape_cfg.reject_titles.as_deref().unwrap_or(&[]);
        if is_title_rejected(&job.title, reject_pats) {
            filtered_title += 1;
            continue;
        }
        if !has_valid_email(&job.emails, scrape_cfg) {
            filtered_email += 1;
            continue;
        }
        let clean_emails = filter_emails(&job.emails, scrape_cfg);
        match insert_job(&pool, &job, &clean_emails, scrape_mode_str).await {
            Ok(rows) => {
                if rows > 0 {
                    if let Err(e) = queue_emails(&pool, &job, &clean_emails).await {
                        tracing::warn!("failed to queue emails for {}: {}", job.title, e);
                    }
                    inserted += 1;
                } else {
                    dedup_skipped += 1;
                }
            }
            Err(e) => tracing::warn!("skipping job {}: {}", job.title, e),
        }
    }

    let received = inserted + dedup_skipped + filtered_title + filtered_email;
    let duration_secs = start.elapsed().as_secs_f64();

    let status = match child.wait().await {
        Ok(s) => s,
        Err(e) => {
            tracing::warn!("scraper wait error: {} — partial results saved", e);
            // ponytail: still write run_log even if child was killed
            db::write_run_log(&pool, "scrape", Some(scrape_mode_str),
                received as i32, inserted as i32, 0, 0, None).await;
            return Ok(ScrapeResult {
                mode, carried_over, received, filtered_title,
                filtered_email, inserted, dedup_skipped,
                sites_count, terms_count, duration_secs,
            });
        }
    };
    if !status.success() {
        tracing::warn!("scraper exited with status {} — partial results saved", status);
    }

    tracing::info!(
        "scrape done: {} received, {} title-filt, {} email-filt, {} dedup-skip, {} inserted",
        received, filtered_title, filtered_email, dedup_skipped, inserted
    );
    tracing::info!("scrape completed in {:.1}s", duration_secs);

    db::write_run_log(&pool, "scrape", Some(scrape_mode_str),
        received as i32, inserted as i32, 0, 0, None).await;

    Ok(ScrapeResult { mode, carried_over, received, filtered_title, filtered_email, inserted, dedup_skipped, sites_count, terms_count, duration_secs })
}

// ── DB operations ──────────────────────────────────────────

/// Returns number of rows inserted (0 = dedup skipped, 1 = new).
async fn insert_job(pool: &PgPool, job: &JobPost, emails: &[Email], mode: &str) -> anyhow::Result<u64> {
    let emails_json = serde_json::to_value(emails)?;
    sqlx::query(
        r#"
        INSERT INTO jobs (source_site, title, company_name, company_url, job_url,
                          location, is_remote, description, emails, quality_score, fetched_at,
                          scrape_mode)
        SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), $11
        WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE job_url = $5)
        "#,
    )
    .bind(&job.site)
    .bind(&job.title)
    .bind(job.company_name.as_deref().unwrap_or(""))
    .bind(job.company_url.as_deref().unwrap_or(""))
    .bind(&job.job_url)
    .bind(job.location.as_ref().map(|l| l.display()).unwrap_or_default())
    .bind(job.is_remote)
    .bind(job.description.as_deref().unwrap_or(""))
    .bind(&emails_json)
    .bind(job.quality_score)
    .bind(mode)
    .execute(pool)
    .await
    .context("failed to insert job")
    .map(|r| r.rows_affected())
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
        std::path::PathBuf::from("./scraper/scraper"),
        std::path::PathBuf::from("./scraper"),
        std::path::PathBuf::from("/usr/local/bin/scraper"),
    ];
    for p in &candidates { if p.exists() { return Ok(p.clone()); } }
    anyhow::bail!("scraper binary not found. Build: cd scraper && go build -o . .")
}
