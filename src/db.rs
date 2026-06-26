use sqlx::postgres::PgPoolOptions;
use sqlx::PgPool;
use std::time::Duration;

// ponytail: connect, migrate, query. No ORM, no query builder.

pub async fn connect(database_url: &str) -> anyhow::Result<PgPool> {
    let pool = PgPoolOptions::new()
        .max_connections(5)
        .acquire_timeout(Duration::from_secs(10))
        .connect(database_url)
        .await?;

    sqlx::migrate!("./migrations").run(&pool).await?;

    Ok(pool)
}
