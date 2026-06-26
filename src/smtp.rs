use crate::config::{EmailConfig, TrackingConfig};
use anyhow::Context;
use lettre::{
    message::{header::ContentType, Mailbox, MultiPart, SinglePart},
    transport::smtp::authentication::Credentials,
    AsyncSmtpTransport, AsyncTransport, Tokio1Executor,
};
use sqlx::PgPool;
use std::time::Duration;

// ponytail: sends generated emails via Gmail SMTP. Tracking pixel from config.

pub struct SmtpResult {
    pub sent: usize,
    pub failed: usize,
    pub quota_remaining: i32,
}

pub async fn send_generated(
    pool: &PgPool,
    email_cfg: &EmailConfig,
    tracking_cfg: &TrackingConfig,
) -> anyhow::Result<SmtpResult> {
    let username = email_cfg.from_addr.clone()
        .or_else(|| std::env::var("GMAIL_USER").ok())
        .context("GMAIL_USER not set. Set [email].from_addr or GMAIL_USER env var")?;

    let password = std::env::var("GMAIL_APP_PASS").context("GMAIL_APP_PASS not set")?;
    let from_name = email_cfg.from_name.clone().unwrap_or_default();
    let delay = Duration::from_secs(email_cfg.delay_seconds.unwrap_or(15));
    let daily_limit = email_cfg.daily_limit.unwrap_or(500);

    // Tracking pixel URL
    let base_url = tracking_cfg.server_url.as_deref().unwrap_or("https://tracker.jobhunter.dev");
    let pixel_url = format!("{}/track?e={{email_id}}", base_url.trim_end_matches('/'));

    // Daily quota
    let sent_today: i64 = sqlx::query_scalar(
        "SELECT COUNT(*) FROM email_queue WHERE status = 'sent' AND sent_at > now() - interval '24 hours'"
    )
    .fetch_one(pool)
    .await
    .unwrap_or(0);

    let quota_remaining = daily_limit - sent_today as i32;
    if quota_remaining <= 0 {
        tracing::info!("daily quota exhausted ({} sent today)", sent_today);
        return Ok(SmtpResult { sent: 0, failed: 0, quota_remaining: 0 });
    }

    // Fetch generated emails
    let emails = sqlx::query_as::<_, (uuid::Uuid, String, String, String, String)>(
        "SELECT id, email_addr, subject, body, company_name FROM email_queue WHERE status = 'generated' ORDER BY created_at ASC"
    )
    .fetch_all(pool)
    .await?;

    if emails.is_empty() {
        tracing::info!("no generated emails to send");
        return Ok(SmtpResult { sent: 0, failed: 0, quota_remaining });
    }

    tracing::info!("sending {} emails (quota: {}/{})", emails.len(), quota_remaining, daily_limit);

    let creds = Credentials::new(username.clone(), password);
    let mailer = AsyncSmtpTransport::<Tokio1Executor>::relay("smtp.gmail.com")
        .context("failed to create SMTP transport")?
        .credentials(creds)
        .port(587)
        .build();

    let mut sent = 0usize;
    let mut failed = 0usize;

    for (email_id, addr, subject, body, company) in &emails {
        if sent as i32 >= quota_remaining {
            tracing::warn!("quota reached, stopping");
            break;
        }

        let click_base = format!("{}/click?e={}&url=", base_url.trim_end_matches('/'), email_id);
        let pixel = format!(r#"<img src="{}" width="1" height="1" />"#, pixel_url.replace("{email_id}", &email_id.to_string()));
        let html_body = format!("{}{}", wrap_links(&body, &click_base), pixel);

        let from_addr: Mailbox = format!("{} <{}>", from_name, username)
            .parse().map_err(|e| anyhow::anyhow!("invalid from: {}", e))?;
        let to_addr: Mailbox = addr.parse()
            .map_err(|e| anyhow::anyhow!("invalid to {}: {}", addr, e))?;

        let email = lettre::Message::builder()
            .from(from_addr)
            .to(to_addr)
            .subject(subject.clone())
            .multipart(
                MultiPart::alternative()
                    .singlepart(SinglePart::builder()
                        .header(ContentType::TEXT_PLAIN)
                        .body(body.clone()))
                    .singlepart(SinglePart::builder()
                        .header(ContentType::TEXT_HTML)
                        .body(html_body))
            );

        let email = match email {
            Ok(e) => e,
            Err(e) => {
                tracing::warn!("build failed for {}: {}", addr, e);
                sqlx::query("UPDATE email_queue SET status = 'failed', error_msg = $2 WHERE id = $1")
                    .bind(email_id).bind(&e.to_string())
                    .execute(pool).await?;
                failed += 1;
                continue;
            }
        };

        match mailer.send(email).await {
            Ok(_) => {
                sqlx::query("UPDATE email_queue SET status = 'sent', sent_at = now() WHERE id = $1")
                    .bind(email_id).execute(pool).await?;
                sqlx::query("INSERT INTO tracking (email_id, email_addr, sent_at) VALUES ($1, $2, now()) ON CONFLICT DO NOTHING")
                    .bind(email_id).bind(addr).execute(pool).await?;
                sent += 1;
                tracing::info!("sent to {} ({}) — {}/{}", addr, company, sent, emails.len());
            }
            Err(e) => {
                tracing::warn!("send failed for {}: {}", addr, e);
                sqlx::query("UPDATE email_queue SET status = 'failed', error_msg = $2 WHERE id = $1")
                    .bind(email_id).bind(&e.to_string())
                    .execute(pool).await?;
                failed += 1;
            }
        }

        tokio::time::sleep(delay).await;
    }

    Ok(SmtpResult { sent, failed, quota_remaining })
}

/// Convert plain text body to HTML with tracked links.
fn wrap_links(body: &str, click_base: &str) -> String {
    // ponytail: replace \n with <br>, wrap https:// URLs in <a> tags with tracking.
    let mut out = String::with_capacity(body.len() * 2);
    let mut rest = body;
    while let Some(pos) = rest.find("https://") {
        out.push_str(&rest[..pos].replace('\n', "<br>"));
        let remaining = &rest[pos..];
        let end = remaining.find(|c: char| c.is_whitespace() || c == '<' || c == '>').unwrap_or(remaining.len());
        let url = &remaining[..end];
        let tracked = format!("{}{}", click_base, url_encode(url));
        out.push_str(&format!("<a href=\"{}\">{}</a>", tracked, url));
        rest = &remaining[end..];
    }
    out.push_str(&rest.replace('\n', "<br>"));
    out
}

fn url_encode(s: &str) -> String {
    s.replace('%', "%25").replace('&', "%26").replace('=', "%3D").replace('?', "%3F")
}
