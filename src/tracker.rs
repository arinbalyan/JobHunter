use axum::{extract::Query, http::StatusCode, response::IntoResponse, routing::get, Router};
use serde::Deserialize;
use sqlx::PgPool;
use std::net::SocketAddr;

// ponytail: 4 routes, one function run().

const TRACKING_GIF: &[u8] = &[
    0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00,
    0x80, 0x00, 0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00,
    0x21, 0xf9, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
    0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
];

#[derive(Deserialize)]
struct TrackQuery {
    e: String, // email_id
}

pub async fn run(pool: PgPool, port: u16) -> anyhow::Result<()> {
    let app = Router::new()
        .route("/track", get(track_open))
        .route("/click", get(track_click))
        .route("/health", get(health))
        .route("/version", get(version))
        .with_state(pool);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    tracing::info!("tracking server listening on {}", addr);

    axum::serve(
        tokio::net::TcpListener::bind(&addr).await?,
        app,
    )
    .await?;

    Ok(())
}

async fn track_open(
    Query(q): Query<TrackQuery>,
    pool: axum::extract::State<PgPool>,
) -> impl IntoResponse {
    sqlx::query(
        "UPDATE tracking SET opened = true, opened_at = COALESCE(opened_at, now()) WHERE email_id = $1::uuid"
    )
    .bind(&q.e)
    .execute(&*pool)
    .await
    .ok();

    (
        StatusCode::OK,
        [("Content-Type", "image/gif"), ("Cache-Control", "no-store, max-age=0")],
        TRACKING_GIF,
    )
}

async fn track_click(
    Query(q): Query<TrackQuery>,
    pool: axum::extract::State<PgPool>,
) -> impl IntoResponse {
    sqlx::query(
        "UPDATE tracking SET clicks = clicks + 1, last_clicked_at = now() WHERE email_id = $1::uuid"
    )
    .bind(&q.e)
    .execute(&*pool)
    .await
    .ok();

    // ponytail: redirect to a generic URL. No target URL stored yet.
    (
        StatusCode::FOUND,
        [("Location", "https://linkedin.com/in/arinbalyan")],
        "",
    )
}

async fn health() -> impl IntoResponse {
    (StatusCode::OK, "ok")
}

async fn version() -> impl IntoResponse {
    (StatusCode::OK, env!("CARGO_PKG_VERSION"))
}
