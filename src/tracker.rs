use axum::{extract::Query, http::StatusCode, response::IntoResponse, routing::get, Router};
use serde::Deserialize;
use sqlx::PgPool;
use std::net::SocketAddr;

// ponytail: tracking routes + inbox dashboard at / and /inbox.

const TRACKING_GIF: &[u8] = &[
    0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00,
    0x80, 0x00, 0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00,
    0x21, 0xf9, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
    0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
];

#[derive(Deserialize)]
struct TrackQuery {
    e: String,
}

pub async fn run(pool: PgPool, port: u16) -> anyhow::Result<()> {
    let app = Router::new()
        .route("/", get(dashboard))
        .route("/track", get(track_open))
        .route("/click", get(track_click))
        .route("/health", get(health))
        .route("/version", get(version))
        .with_state(pool);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    tracing::info!("server listening on {}", addr);

    axum::serve(tokio::net::TcpListener::bind(&addr).await?, app).await?;
    Ok(())
}

async fn dashboard(pool: axum::extract::State<PgPool>) -> impl IntoResponse {
    let (total_jobs, scored_jobs, avg_score, queue_pending, queue_sent, queue_failed, opens, total_sent) = fetch_stats(&pool).await;

    let open_pct = if total_sent > 0 { opens * 100 / total_sent } else { 0 };

    let html = format!(
        r#"<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>JobHunter Inbox</title>
<style>
*{{margin:0;padding:0;box-sizing:border-box}}
body{{font-family:-apple-system,system-ui,sans-serif;background:#f5f5f5;color:#222;padding:2rem}}
h1{{font-size:1.5rem;margin-bottom:1.5rem;color:#333}}
.grid{{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:1rem;margin-bottom:2rem}}
.card{{background:#fff;border-radius:8px;padding:1.25rem;box-shadow:0 1px 3px rgba(0,0,0,.1)}}
.card .num{{font-size:1.8rem;font-weight:700;color:#0a7cff}}
.card .label{{font-size:.8rem;color:#666;margin-top:.25rem}}
.card.green .num{{color:#16a34a}}
.card.yellow .num{{color:#d97706}}
.card.red .num{{color:#dc2626}}
.card.purple .num{{color:#7c3aed}}
table{{width:100%;border-collapse:collapse;font-size:.85rem}}
th,td{{padding:.5rem .75rem;text-align:left;border-bottom:1px solid #eee}}
th{{background:#fafafa;font-weight:600;color:#555}}
tr:hover{{background:#f8f9ff}}
.footer{{margin-top:2rem;font-size:.75rem;color:#999}}
</style></head>
<body>
<h1>📊 JobHunter Pipeline</h1>
<div class="grid">
  <div class="card"><div class="num">{total_jobs}</div><div class="label">Total Jobs</div></div>
  <div class="card green"><div class="num">{scored_jobs}</div><div class="label">Scored (avg {avg_score:.1})</div></div>
  <div class="card purple"><div class="num">{queue_pending}</div><div class="label">Pending Queue</div></div>
  <div class="card green"><div class="num">{queue_sent}</div><div class="label">Sent</div></div>
  <div class="card yellow"><div class="num">{opens}</div><div class="label">Opens ({open_pct}%)</div></div>
  <div class="card red"><div class="num">{queue_failed}</div><div class="label">Failed</div></div>
</div>
<h2>Recent Jobs</h2>
<table><tr><th>Title</th><th>Company</th><th>Score</th><th>Source</th><th>Fetched</th></tr>
{recent}
</table>
<div class="footer">jobhunter v{version} · DB: {total_jobs} jobs · <a href="/health">health</a></div>
</body></html>"#,
        total_jobs = total_jobs,
        scored_jobs = scored_jobs,
        avg_score = avg_score,
        queue_pending = queue_pending,
        queue_sent = queue_sent,
        opens = opens,
        open_pct = open_pct,
        queue_failed = queue_failed,
        recent = recent_jobs_html(&pool).await,
        version = env!("CARGO_PKG_VERSION"),
    );

    (StatusCode::OK, [("Content-Type", "text/html; charset=utf-8")], html)
}

async fn q1(pool: &PgPool, sql: &str) -> i64 {
    sqlx::query_scalar::<_, i64>(sql).fetch_one(pool).await.unwrap_or_default()
}

async fn qf(pool: &PgPool, sql: &str) -> f64 {
    sqlx::query_scalar::<_, f64>(sql).fetch_one(pool).await.unwrap_or_default()
}

async fn fetch_stats(pool: &PgPool) -> (i64, i64, f64, i64, i64, i64, i64, i64) {
    (
        q1(pool, "SELECT COUNT(*) FROM jobs").await,
        q1(pool, "SELECT COUNT(*) FROM jobs WHERE llm_score IS NOT NULL").await,
        qf(pool, "SELECT COALESCE(AVG(llm_score::float), 0) FROM jobs WHERE llm_score IS NOT NULL").await,
        q1(pool, "SELECT COUNT(*) FROM email_queue WHERE status IN ('pending','generating')").await,
        q1(pool, "SELECT COUNT(*) FROM email_queue WHERE status = 'sent'").await,
        q1(pool, "SELECT COUNT(*) FROM email_queue WHERE status = 'failed'").await,
        q1(pool, "SELECT COUNT(*) FROM tracking WHERE opened = true").await,
        q1(pool, "SELECT COUNT(*) FROM tracking").await,
    )
}

async fn recent_jobs_html(pool: &PgPool) -> String {
    let rows = sqlx::query_as::<_, (String, String, Option<i32>, String, chrono::DateTime<chrono::Utc>)>(
        "SELECT title, company_name, llm_score, source_site, fetched_at FROM jobs ORDER BY fetched_at DESC LIMIT 20"
    ).fetch_all(pool).await.unwrap_or_default();

    let mut out = String::new();
    for (title, company, score, site, fetched) in &rows {
        let s = score.map(|s| s.to_string()).unwrap_or_else(|| "-".to_string());
        out.push_str(&format!("<tr><td>{}</td><td>{}</td><td>{}</td><td>{}</td><td>{}</td></tr>",
            html_esc(title), html_esc(company), s, site, fetched.format("%b %d")));
    }
    out
}

fn html_esc(s: &str) -> String {
    s.replace('&', "&amp;").replace('<', "&lt;").replace('>', "&gt;").replace('"', "&quot;")
}

// ── Tracking routes ────────────────────────────────────────

async fn track_open(Query(q): Query<TrackQuery>, pool: axum::extract::State<PgPool>) -> impl IntoResponse {
    sqlx::query("UPDATE tracking SET opened = true, opened_at = COALESCE(opened_at, now()) WHERE email_id = $1::uuid")
        .bind(&q.e).execute(&*pool).await.ok();
    (StatusCode::OK, [("Content-Type", "image/gif"), ("Cache-Control", "no-store, max-age=0")], TRACKING_GIF)
}

async fn track_click(Query(q): Query<TrackQuery>, pool: axum::extract::State<PgPool>) -> impl IntoResponse {
    sqlx::query("UPDATE tracking SET clicks = clicks + 1, last_clicked_at = now() WHERE email_id = $1::uuid")
        .bind(&q.e).execute(&*pool).await.ok();
    (StatusCode::FOUND, [("Location", "https://linkedin.com/in/arinbalyan")], "")
}

async fn health() -> impl IntoResponse { (StatusCode::OK, "ok") }
async fn version() -> impl IntoResponse { (StatusCode::OK, env!("CARGO_PKG_VERSION")) }
