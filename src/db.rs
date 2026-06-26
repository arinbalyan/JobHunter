use sqlx::postgres::PgPoolOptions;
use sqlx::PgPool;
use std::time::Duration;

// ponytail: connect, migrate, log runs. No ORM, no query builder.

pub async fn connect(database_url: &str) -> anyhow::Result<PgPool> {
    let pool = PgPoolOptions::new()
        .max_connections(5)
        .acquire_timeout(Duration::from_secs(10))
        .connect(database_url)
        .await?;

    sqlx::migrate!("./migrations").run(&pool).await?;

    Ok(pool)
}

pub async fn write_run_log(
    pool: &PgPool,
    workflow: &str,
    mode: Option<&str>,
    jobs_found: i32,
    emails_queued: i32,
    emails_sent: i32,
    emails_failed: i32,
    error_msg: Option<&str>,
) {
    sqlx::query(
        r#"
        INSERT INTO run_log (workflow, mode, status, completed_at, jobs_found, emails_queued, emails_sent,
                             emails_failed, error_msg)
        VALUES ($1, $2, 'completed', now(), $3, $4, $5, $6, $7)
        "#,
    )
    .bind(workflow)
    .bind(mode)
    .bind(jobs_found)
    .bind(emails_queued)
    .bind(emails_sent)
    .bind(emails_failed)
    .bind(error_msg)
    .execute(pool)
    .await
    .ok();
}
