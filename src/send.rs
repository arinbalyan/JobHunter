use crate::config::Config;
use crate::llm::{self, ChatMessage};
use crate::smtp;
use anyhow::Context;
use sqlx::PgPool;
use std::sync::Arc;
use tokio::sync::Semaphore;

// ponytail: one module for email generation. Takes pending queue rows → generates → updates DB.

const MAX_CONCURRENT: usize = 10;

pub struct SendResult {
    pub total: usize,
    pub generated: usize,
    pub failed: usize,
}

/// Context fetched from DB for a single email to generate.
struct EmailJob {
    queue_id: uuid::Uuid,
    email_addr: String,
    company_name: String,
    job_title: String,
    job_description: String,
    job_location: String,
    scrape_mode: String,
}

pub async fn run(cfg: Config, max_concurrent: Option<usize>) -> anyhow::Result<SendResult> {
    let db_url = std::env::var("DATABASE_URL").context("DATABASE_URL not set")?;
    let pool = crate::db::connect(&db_url).await?;

    // Build router from configured providers
    let providers: Vec<llm::Provider> = cfg.llm.providers.iter().filter_map(|p| {
        let key = std::env::var(&p.api_key_env).ok()?;
        Some(llm::Provider {
            name: p.name.clone(),
            api_key: key,
            base_url: p.base_url.clone(),
            model_complex: p.model_complex.clone(),
            model_simple: p.model_simple.clone(),
            weight: p.weight,
        })
    }).collect();

    if providers.is_empty() {
        anyhow::bail!("no LLM providers with API keys configured");
    }

    let router = Arc::new(tokio::sync::Mutex::new(llm::Router::new(providers)));

    // Fetch pending email queue entries with job details
    let emails = fetch_pending(&pool).await?;
    let total = emails.len();
    tracing::info!("generating emails for {} pending queue entries", total);

    if total == 0 {
        return Ok(SendResult { total: 0, generated: 0, failed: 0 });
    }

    let semaphore = Arc::new(Semaphore::new(max_concurrent.unwrap_or(MAX_CONCURRENT)));
    let mut handles = Vec::with_capacity(total);
    let templates = cfg.templates.clone();
    let user_info = cfg.user.clone();

    for job in emails {
        let permit = semaphore.clone().acquire_owned().await.unwrap();
        let pool = pool.clone();
        let router = router.clone();
        let templates = templates.clone();
        let user_info = user_info.clone();

        handles.push(tokio::spawn(async move {
            let _permit = permit;
            match generate_one(&pool, &router, &templates, &user_info, &job).await {
                Ok(()) => tracing::info!("generated email for {}", job.email_addr),
                Err(e) => tracing::warn!("failed to generate email for {}: {}", job.email_addr, e),
            }
        }));
    }

    // Wait for all generation to complete
    for h in handles { let _ = h.await; }

    let generated = count_status(&pool, "generated").await;
    let failed = count_status(&pool, "failed").await;

    tracing::info!("email generation done: {} generated, {} failed", generated, failed);

    // ── Phase 2: Send generated emails ──
    tracing::info!("sending generated emails...");
    let (sent, send_failed) = match smtp::send_generated(&pool, &cfg.email, &cfg.tracking).await {
        Ok(sr) => {
            tracing::info!("send done: {} sent, {} failed, {}/{} quota",
                sr.sent, sr.failed, sr.quota_remaining, cfg.email.daily_limit.unwrap_or(500));
            (sr.sent, sr.failed)
        }
        Err(e) => {
            tracing::warn!("send phase failed: {}", e);
            (0, 0)
        }
    };

    crate::db::write_run_log(&pool, "send", None,
        0, total as i32, sent as i32, send_failed as i32,
        if send_failed > 0 { Some("some sends failed") } else { None }).await;

    Ok(SendResult { total, generated, failed })
}

/// Fetch pending email_queue entries with job details.
async fn fetch_pending(pool: &PgPool) -> anyhow::Result<Vec<EmailJob>> {
    let rows = sqlx::query_as::<_, (uuid::Uuid, String, String, String, String, String, String)>(
        r#"
        SELECT eq.id, eq.email_addr, eq.company_name,
               j.title, COALESCE(j.description, ''), COALESCE(j.location, ''),
               COALESCE(j.scrape_mode, '')
        FROM email_queue eq
        JOIN jobs j ON j.id = eq.job_id
        WHERE eq.status = 'pending'
        ORDER BY eq.created_at ASC
        LIMIT 500
        "#,
    )
    .fetch_all(pool)
    .await?;

    Ok(rows.into_iter().map(|(id, addr, company, title, desc, loc, mode)| {
        EmailJob {
            queue_id: id,
            email_addr: addr,
            company_name: company,
            job_title: title,
            job_description: desc,
            job_location: loc,
            scrape_mode: mode,
        }
    }).collect())
}

/// Generate a single email via LLM, fall back to template on failure.
async fn generate_one(
    pool: &PgPool,
    router: &Arc<tokio::sync::Mutex<llm::Router>>,
    templates: &crate::config::Templates,
    user: &crate::config::User,
    job: &EmailJob,
) -> anyhow::Result<()> {
    // Mark as generating
    sqlx::query("UPDATE email_queue SET status = 'generating' WHERE id = $1")
        .bind(job.queue_id)
        .execute(pool)
        .await?;

    // Select per-mode context, fallback to default
    let context = match job.scrape_mode.as_str() {
        "remote" => user.context_remote.as_ref().or(user.context.as_ref()),
        "onsite" => user.context_onsite.as_ref().or(user.context.as_ref()),
        _ => user.context.as_ref(),
    }.cloned().unwrap_or_else(|| format!(
        "Role: {}, Experience: {} years, Name: {}",
        user.current_role, user.years_experience, user.name
    ));

    // Select per-mode templates, fallback to default
    let system_template = match job.scrape_mode.as_str() {
        "remote" => templates.email_system_remote.as_ref().unwrap_or(&templates.email_system),
        "onsite" => templates.email_system_onsite.as_ref().unwrap_or(&templates.email_system),
        _ => &templates.email_system,
    };
    let user_template = match job.scrape_mode.as_str() {
        "remote" => templates.email_user_remote.as_ref().unwrap_or(&templates.email_user),
        "onsite" => templates.email_user_onsite.as_ref().unwrap_or(&templates.email_user),
        _ => &templates.email_user,
    };

    let user_prompt = fill_placeholders(&user_template.content, &[
        ("context", &context),
        ("title", &job.job_title),
        ("company", &job.company_name),
        ("description", &job.job_description),
        ("location", &job.job_location),
        ("seniority", ""),
        ("job_type", ""),
        ("salary", ""),
        ("skills", ""),
        ("industry", ""),
        ("experience_match", ""),
    ]);

    let messages = vec![
        ChatMessage { role: "system".to_string(), content: system_template.content.clone() },
        ChatMessage { role: "user".to_string(), content: user_prompt },
    ];

    let mut router = router.lock().await;
    let result = router.complete(&messages, true).await;

    match result {
        Ok(response) => {
            let (subject, mut body) = parse_email_response(&response);
            append_signature(&mut body, user);
            sqlx::query(
                r#"UPDATE email_queue SET status = 'generated', body = $2, subject = $3 WHERE id = $1"#
            )
            .bind(job.queue_id)
            .bind(&body)
            .bind(&subject)
            .execute(pool)
            .await?;
            Ok(())
        }
        Err(_e) => {
            // Fallback to template
            let subject = format!("Application for {} at {}", job.job_title, job.company_name);
            let mut body = format!(
                "Hi there,\n\nI am writing to express my interest in the {} position at {}. \
                 I believe my background as a {} with {} years of experience makes me a strong \
                 candidate for this role.\n\nI would love the opportunity to discuss how I can \
                 contribute to the team.",
                job.job_title, job.company_name, user.current_role, user.years_experience
            );
            append_signature(&mut body, user);
            sqlx::query(
                r#"UPDATE email_queue SET status = 'generated', body = $2, subject = $3 WHERE id = $1"#
            )
            .bind(job.queue_id)
            .bind(&body)
            .bind(&subject)
            .execute(pool)
            .await?;
            tracing::info!("template fallback for {}", job.email_addr);
            Ok(())
        }
    }
}

/// Append signature footer with links.
fn append_signature(body: &mut String, user: &crate::config::User) {
    body.push_str("\n\n---\nRegards,\n");
    body.push_str(&user.name);
    if let Some(ref g) = user.github {
        body.push_str(&format!("\nGitHub: {}", g));
    }
    if let Some(ref p) = user.portfolio {
        body.push_str(&format!("\nPortfolio: {}", p));
    }
    if let Some(ref r) = user.resume_url {
        body.push_str(&format!("\nResume: {}", r));
    }
}

/// Parse LLM response into subject + body.
/// Expected format: "SUBJECT: ...\n\n..."
fn parse_email_response(response: &str) -> (String, String) {
    let trimmed = response.trim();
    if let Some(rest) = trimmed.strip_prefix("SUBJECT:") {
        let rest = rest.trim();
        if let Some(body_start) = rest.find("\n\n") {
            let subject = rest[..body_start].trim().to_string();
            let body = rest[body_start..].trim().to_string();
            return (subject, body);
        }
        // No body separator, whole rest is subject
        (rest.to_string(), String::new())
    } else {
        // No SUBJECT: prefix, first line is subject
        let first_newline = trimmed.find('\n').unwrap_or(trimmed.len());
        (trimmed[..first_newline].to_string(), trimmed[first_newline..].trim().to_string())
    }
}

/// Simple placeholder replacement.
fn fill_placeholders(template: &str, pairs: &[(&str, &str)]) -> String {
    let mut result = template.to_string();
    for (key, value) in pairs {
        result = result.replace(&format!("{{{}}}", key), value);
    }
    result
}

async fn count_status(pool: &PgPool, status: &str) -> usize {
    sqlx::query_scalar::<_, i64>("SELECT COUNT(*) FROM email_queue WHERE status = $1")
        .bind(status)
        .fetch_one(pool)
        .await
        .unwrap_or(0) as usize
}
