use crate::config::Config;
use crate::llm::{self, ChatMessage};
use anyhow::Context;
use sqlx::PgPool;
use std::sync::Arc;
use tokio::sync::Semaphore;

// ponytail: scores unscored jobs (1-10) via LLM. Simple prompt, numeric response.

const MAX_CONCURRENT: usize = 10;

pub struct ScoreResult {
    pub total: usize,
    pub scored: usize,
    pub failed: usize,
}

pub async fn run(cfg: Config, max_concurrent: Option<usize>) -> anyhow::Result<ScoreResult> {
    let db_url = std::env::var("DATABASE_URL").context("DATABASE_URL not set")?;
    let pool = crate::db::connect(&db_url).await?;

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

    // Fetch unscored jobs
    let jobs = sqlx::query_as::<_, (uuid::Uuid, String, String, Option<String>)>(
        r#"SELECT id, title, company_name, description FROM jobs WHERE llm_score IS NULL LIMIT 1000"#
    )
    .fetch_all(&pool)
    .await?;

    let total = jobs.len();
    tracing::info!("scoring {} unscored jobs", total);

    if total == 0 {
        return Ok(ScoreResult { total: 0, scored: 0, failed: 0 });
    }

    let semaphore = Arc::new(Semaphore::new(max_concurrent.unwrap_or(MAX_CONCURRENT)));
    let mut handles = Vec::with_capacity(total);
    let prompt = cfg.templates.scoring.content.clone();
    let role = cfg.user.current_role.clone();
    let years = cfg.user.years_experience;

    for (job_id, title, company, desc) in jobs {
        let permit = semaphore.clone().acquire_owned().await.unwrap();
        let pool = pool.clone();
        let router = router.clone();
        let prompt = prompt.clone();
        let role = role.clone();
        let desc_text = desc.unwrap_or_default();

        handles.push(tokio::spawn(async move {
            let _permit = permit;
            score_one(&pool, &router, &prompt, &role, years, &job_id, &title, &company, &desc_text).await;
        }));
    }

    for h in handles { let _ = h.await; }

    let scored = sqlx::query_scalar::<_, i64>(
        "SELECT COUNT(*) FROM jobs WHERE llm_score IS NOT NULL AND llm_score > 0"
    )
    .fetch_one(&pool)
    .await
    .unwrap_or(0) as usize;

    let failed = sqlx::query_scalar::<_, i64>(
        "SELECT COUNT(*) FROM jobs WHERE llm_score IS NOT NULL AND llm_score = 0"
    )
    .fetch_one(&pool)
    .await
    .unwrap_or(0) as usize;

    tracing::info!("scoring done: {} scored, {} failed", scored, failed);
    Ok(ScoreResult { total, scored, failed })
}

async fn score_one(
    pool: &PgPool,
    router: &Arc<tokio::sync::Mutex<llm::Router>>,
    prompt_template: &str,
    role: &str,
    years: i32,
    job_id: &uuid::Uuid,
    title: &str,
    company: &str,
    description: &str,
) {
    let user_prompt = prompt_template
        .replace("{current_role}", role)
        .replace("{years_experience}", &years.to_string())
        .replace("{title}", title)
        .replace("{company}", company)
        .replace("{description}", description)
        .replace("{skills}", "");

    let msg = ChatMessage { role: "user".to_string(), content: user_prompt };

    let mut router = router.lock().await;
    let result = router.complete(&[msg], false).await; // simple model

    let score = match result {
        Ok(text) => {
            // Extract first number from response
            let n: i32 = text.chars()
                .filter(|c| c.is_ascii_digit())
                .collect::<String>()
                .chars()
                .take(2)
                .collect::<String>()
                .parse()
                .unwrap_or(0);
            n.clamp(1, 10)
        }
        Err(_) => 0,
    };

    sqlx::query("UPDATE jobs SET llm_score = $1 WHERE id = $2")
        .bind(score)
        .bind(job_id)
        .execute(pool)
        .await
        .ok();

    if score > 0 {
        tracing::info!("scored {} at {} — {}/10", title, company, score);
    }
}
