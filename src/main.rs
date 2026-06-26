use anyhow::Context;

mod config;
mod db;
mod llm;
mod scrape;
mod send;
mod smtp;
mod telegram;
mod tracker;

use clap::{Parser, Subcommand, ValueEnum};

#[derive(Parser)]
#[command(name = "jobhunter", about = "Job scraping and LLM-powered email outreach")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Scrape job boards, filter, and queue emails
    Scrape {
        /// Search mode: remote (global) or onsite (India/hybrid)
        #[arg(long, default_value = "remote")]
        mode: ScrapeMode,
    },
    /// Generate emails for queued jobs via LLM
    Send {
        /// Max concurrent LLM calls
        #[arg(long, default_value = "10")]
        max: usize,
    },
    /// HTTP tracking server (opens, clicks, health)
    Serve,
    /// Run diagnostics
    Doctor,
}

#[derive(Clone, ValueEnum)]
enum ScrapeMode {
    Remote,
    Onsite,
}

fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "jobhunter=info".into()),
        )
        .init();

    let cli = Cli::parse();
    let rt = tokio::runtime::Runtime::new()?;
    rt.block_on(run(cli))
}

async fn run(cli: Cli) -> anyhow::Result<()> {
    match cli.command {
        Commands::Scrape { mode } => {
            let cfg = config::Config::load()?;
            let telegram_cfg = cfg.telegram.clone();
            let scrape_mode = match mode { ScrapeMode::Remote => scrape::Mode::Remote, ScrapeMode::Onsite => scrape::Mode::Onsite };

            // ponytail: discover available models in background, don't block the scrape
            tokio::spawn(discover_models(cfg.clone()));

            let result = scrape::run(cfg, scrape_mode).await?;
            // ponytail: fire-and-forget telegram report, don't fail the run if it errors
            if let Err(e) = telegram::send_scrape_report(&telegram_cfg, &result).await {
                tracing::warn!("telegram report failed: {}", e);
            }
            Ok(())
        }
        Commands::Send { max } => {
            let cfg = config::Config::load()?;
            let result = send::run(cfg, Some(max)).await?;
            println!("📧 Send complete: {} total, {} generated, {} failed",
                result.total, result.generated, result.failed);
            Ok(())
        }
        Commands::Serve => {
            let cfg = config::Config::load()?;
            let db_url = std::env::var("DATABASE_URL").context("DATABASE_URL not set")?;
            let pool = db::connect(&db_url).await?;
            let port = cfg.tracking.port.unwrap_or(8080);
            tracker::run(pool, port).await
        }
        Commands::Doctor => doctor().await,
    }
}

async fn doctor() -> anyhow::Result<()> {
    println!("🏥 jobhunter doctor");
    println!("━━━━━━━━━━━━━━━━━━");

    match config::Config::load() {
        Ok(cfg) => println!("✅ config.toml — found user: {}", cfg.user.name),
        Err(e) => println!("❌ config.toml — {e}"),
    }

    match std::env::var("DATABASE_URL") {
        Ok(_) => println!("✅ DATABASE_URL — set"),
        Err(_) => println!("❌ DATABASE_URL — not set"),
    }

    match find_scraper() {
        Some(p) => println!("✅ scraper binary — found at {}", p.display()),
        None => println!("❌ scraper binary — not found"),
    }

    if let Ok(cfg) = config::Config::load() {
        for p in &cfg.llm.providers {
            match std::env::var(&p.api_key_env) {
                Ok(_) => println!("✅ {} — {} set", p.name, p.api_key_env),
                Err(_) => println!("⚠️  {} — {} not set", p.name, p.api_key_env),
            }
        }
        println!();
        println!("🔍 Checking /models endpoints...");
        check_provider_models(&cfg).await;
    }

    Ok(())
}

async fn discover_models(cfg: config::Config) {
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
    let router = llm::Router::new(providers);
    router.discover_models().await;
}

async fn check_provider_models(cfg: &config::Config) {
    for p in &cfg.llm.providers {
        let key = match std::env::var(&p.api_key_env) {
            Ok(k) => k,
            Err(_) => { println!("⚠️  {} — no key, skipping", p.name); continue; }
        };
        let url = format!("{}/models", p.base_url.trim_end_matches('/'));
        let client = reqwest::Client::new();
        match client.get(&url).header("Authorization", format!("Bearer {}", key)).send().await {
            Ok(resp) => match resp.text().await {
                Ok(body) => {
                    let has_complex = body.contains(&p.model_complex);
                    let has_simple = body.contains(&p.model_simple);
                    print!("✅ {} — models available", p.name);
                    if !has_complex { print!(" ⚠️ complex '{}' not found", p.model_complex); }
                    if !has_simple { print!(" ⚠️ simple '{}' not found", p.model_simple); }
                    println!();
                }
                Err(e) => println!("⚠️  {} — /models error: {}", p.name, e),
            },
            Err(e) => println!("⚠️  {} — /models request failed: {}", p.name, e),
        }
    }
}

fn find_scraper() -> Option<std::path::PathBuf> {
    let c = [std::path::PathBuf::from("./scraper"), std::path::PathBuf::from("/usr/local/bin/scraper")];
    c.into_iter().find(|p| p.exists())
}
