mod config;
mod db;
mod scrape;
mod telegram;

use clap::{Parser, Subcommand};

#[derive(Parser)]
#[command(name = "jobhunter", about = "Job scraping and LLM-powered email outreach")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Scrape job boards, filter, and queue emails
    Scrape,
    /// Run diagnostics
    Doctor,
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
        Commands::Scrape => {
            let cfg = config::Config::load()?;
            let telegram_cfg = cfg.telegram.clone();
            let result = scrape::run(cfg).await?;
            // ponytail: fire-and-forget telegram report, don't fail the run if it errors
            if let Err(e) = telegram::send_scrape_report(&telegram_cfg, &result).await {
                tracing::warn!("telegram report failed: {}", e);
            }
            Ok(())
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
    }

    Ok(())
}

fn find_scraper() -> Option<std::path::PathBuf> {
    let c = [std::path::PathBuf::from("./scraper"), std::path::PathBuf::from("/usr/local/bin/scraper")];
    c.into_iter().find(|p| p.exists())
}
