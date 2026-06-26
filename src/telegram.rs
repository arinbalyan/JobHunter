use crate::config::TelegramConfig;
use crate::scrape::ScrapeResult;

// ponytail: one function, reads TELEGRAM_BOT_TOKEN from env.

pub async fn send_scrape_report(cfg: &TelegramConfig, r: &ScrapeResult) -> anyhow::Result<()> {
    let token = std::env::var("TELEGRAM_BOT_TOKEN")
        .map_err(|_| anyhow::anyhow!("TELEGRAM_BOT_TOKEN not set"))?;

    let mode_label = match r.mode {
        crate::scrape::Mode::Remote => "🌍 Remote",
        crate::scrape::Mode::Onsite => "🇮🇳 Onsite",
    };

    let text = format!(
        "🦀 <b>Scrape {mode_label}</b>\n\
         ━━━━━━━━━━━━━━━━━\n\
         📥 Received: {}\n\
         📤 Carried over: {}\n\
         🚫 Title filtered: {}\n\
         🚫 Email filtered: {}\n\
         ✅ Inserted: {}",
        r.received, r.carried_over, r.filtered_title, r.filtered_email, r.inserted
    );

    let url = format!(
        "https://api.telegram.org/bot{}/sendMessage?chat_id={}&text={}&parse_mode=HTML",
        token, cfg.chat_id, url_encode(&text)
    );

    let resp = reqwest::get(&url).await?;
    if !resp.status().is_success() {
        anyhow::bail!("telegram API returned {}", resp.status());
    }
    Ok(())
}

fn url_encode(s: &str) -> String {
    // ponytail: only the chars Telegram's API needs escaped.
    s.replace('&', "%26").replace('<', "%3C").replace('>', "%3E")
}
