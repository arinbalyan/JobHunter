use crate::config::TelegramConfig;
use crate::scrape::ScrapeResult;

// ponytail: rich telegram message with full pipeline breakdown.

pub async fn send_scrape_report(cfg: &TelegramConfig, r: &ScrapeResult) -> anyhow::Result<()> {
    let token = std::env::var("TELEGRAM_BOT_TOKEN")
        .map_err(|_| anyhow::anyhow!("TELEGRAM_BOT_TOKEN not set"))?;

    let (mode_icon, mode_label) = match r.mode {
        crate::scrape::Mode::Remote => ("🌍", "Remote"),
        crate::scrape::Mode::Onsite => ("🇮🇳", "Onsite"),
    };
    let mins = r.duration_secs / 60.0;
    let secs = r.duration_secs % 60.0;

    let text = format!(
        concat!(
            "<b>{icon} Scrape  |  {mode}  |  {mins:.0}m {secs:.0}s</b>\n",
            "───────────────────────────\n",
            "📥 <b>Received:</b>      {received}\n",
            "📤 <b>Carried over:</b>  {carried}\n",
            "🚫 <b>Title filtered:</b>   {title_f}\n",
            "🚫 <b>Email filtered:</b>   {email_f}\n",
            "⏭️ <b>Dedup skipped:</b>   {skipped}\n",
            "✅ <b>Inserted:</b>         {inserted}\n",
            "───────────────────────────\n",
            "🌐 <b>Sites:</b> {sites}  |  🔍 <b>Terms:</b> {terms}"
        ),
        icon = mode_icon,
        mode = mode_label,
        mins = mins,
        secs = secs,
        received = r.received,
        carried = r.carried_over,
        title_f = r.filtered_title,
        email_f = r.filtered_email,
        skipped = r.dedup_skipped,
        inserted = r.inserted,
        sites = r.sites_count,
        terms = r.terms_count
    );

    let url = format!(
        "https://api.telegram.org/bot{}/sendMessage?chat_id={}&text={}&parse_mode=HTML",
        token, cfg.chat_id, url_encode(&text)
    );

    let resp = reqwest::get(&url).await?;
    if !resp.status().is_success() {
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("telegram API: {}", body);
    }
    Ok(())
}

fn url_encode(s: &str) -> String {
    s.replace('&', "%26").replace('<', "%3C").replace('>', "%3E")
}
