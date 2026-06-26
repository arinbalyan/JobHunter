use crate::config::Config;
use crate::llm::{self, ChatMessage};
use anyhow::Context;
use sqlx::PgPool;
use std::sync::Arc;
use tokio::sync::Semaphore;

// ponytail: generates 3 talking points per company via LLM.

const MAX_CONCURRENT: usize = 10;

pub async fn run(cfg: Config, max_concurrent: Option<usize>) -> anyhow::Result<()> {
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

    // Fetch jobs without research notes, deduped by company
    let jobs = sqlx::query_as::<_, (uuid::Uuid, String, String, Option<String>)>(
        r#"SELECT DISTINCT ON (company_name) id, company_name, title, description
           FROM jobs WHERE research_notes IS NULL AND company_name != ''
           ORDER BY company_name, fetched_at DESC
           LIMIT 200"#
    )
    .fetch_all(&pool)
    .await?;

    let total = jobs.len();
    tracing::info!("researching {} companies", total);

    if total == 0 {
        return Ok(());
    }

    let semaphore = Arc::new(Semaphore::new(max_concurrent.unwrap_or(MAX_CONCURRENT)));
    let mut handles = Vec::with_capacity(total);
    let prompt = cfg.templates.research.content.clone();

    for (job_id, company, title, desc) in jobs {
        let permit = semaphore.clone().acquire_owned().await.unwrap();
        let pool = pool.clone();
        let router = router.clone();
        let prompt = prompt.clone();
        let desc_text = desc.unwrap_or_default();

        handles.push(tokio::spawn(async move {
            let _permit = permit;
            research_one(&pool, &router, &prompt, &job_id, &title, &company, &desc_text).await;
        }));
    }

    for h in handles { let _ = h.await; }
    tracing::info!("company research done");
    Ok(())
}

async fn research_one(
    pool: &PgPool,
    router: &Arc<tokio::sync::Mutex<llm::Router>>,
    prompt_template: &str,
    job_id: &uuid::Uuid,
    title: &str,
    company: &str,
    description: &str,
) {
    let user_prompt = prompt_template
        .replace("{title}", title)
        .replace("{company}", company)
        .replace("{description}", description);

    let msg = ChatMessage { role: "user".to_string(), content: user_prompt };

    let mut router = router.lock().await;
    let result = router.complete(&[msg], false).await; // simple model, just talking points

    let notes = match result {
        Ok(text) => text.trim().to_string(),
        Err(_) => return,
    };

    sqlx::query("UPDATE jobs SET research_notes = $1 WHERE id = $2")
        .bind(&notes)
        .bind(job_id)
        .execute(pool)
        .await
        .ok();

    tracing::info!("researched {} — {}", company, notes.lines().count());
}
