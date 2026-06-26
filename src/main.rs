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
    /// Scrape job boards, filter, and queue
    Scrape(scrape::Args),
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
        Commands::Scrape(args) => scrape::run(args).await,
    }
}
