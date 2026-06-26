mod config;
mod db;
mod scrape;

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
    Scrape {
        /// Search mode: remote | onsite
        #[arg(long, default_value = "remote")]
        mode: String,
    },
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
        Commands::Scrape { mode } => {
            let cfg = config::Config::load()?;
            scrape::run(cfg, &mode).await
        }
        Commands::Doctor => {
            doctor().await
        }
    }
}

async fn doctor() -> anyhow::Result<()> {
    println!("🏥 jobhunter doctor");
    println!("━━━━━━━━━━━━━━━━━━");

    // ponytail: check config, db, scraper binary — the three things that can break.

    match config::Config::load() {
        Ok(cfg) => println!("✅ config.toml — found profile: {}", cfg.profile.name),
        Err(e) => println!("❌ config.toml — {e}"),
    }

    match std::env::var("DATABASE_URL") {
        Ok(_) => println!("✅ DATABASE_URL — set"),
        Err(_) => println!("❌ DATABASE_URL — not set"),
    }

    match find_scraper_binary() {
        Some(p) => println!("✅ scraper binary — found at {}", p.display()),
        None => println!("❌ scraper binary — not found (build: cd scraper && go build -o ../scraper .)"),
    }

    // Check LLM provider env vars
    if let Ok(cfg) = config::Config::load() {
        for provider in &cfg.llm.providers {
            match std::env::var(&provider.api_key_env) {
                Ok(_) => println!("✅ {} — {} set", provider.name, provider.api_key_env),
                Err(_) => println!("⚠️  {} — {} not set (provider will be skipped)", provider.name, provider.api_key_env),
            }
        }
    }

    Ok(())
}

fn find_scraper_binary() -> Option<std::path::PathBuf> {
    let candidates = vec![
        std::path::PathBuf::from("./scraper"),
        std::path::PathBuf::from("/usr/local/bin/scraper"),
    ];
    candidates.into_iter().find(|p| p.exists())
}
