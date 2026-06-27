use serde::Deserialize;
use std::collections::HashMap;
use std::path::Path;

// ponytail: only fields that Phase 1 reads. Extra TOML sections silently ignored.

#[derive(Debug, Deserialize, Clone)]
pub struct Config {
    pub user: User,
    pub search: Search,
    pub scrape: ScrapeConfig,
    pub email: EmailConfig,
    pub tracking: TrackingConfig,
    pub telegram: TelegramConfig,
    pub templates: Templates,
    pub llm: LlmConfig,
    /// Optional path to scrappy's config.toml. If set, its [sites] section is loaded
    /// and merged into `sites` for per-site search term overrides.
    pub scrappy_config: Option<String>,
    pub sites: Option<HashMap<String, SiteConfig>>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct SiteConfig {
    #[serde(default, alias = "search")]
    pub search_terms: Vec<String>,
    pub location: Option<String>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct User {
    pub name: String,
    pub current_role: String,
    pub years_experience: i32,
    pub github: Option<String>,
    pub portfolio: Option<String>,
    pub resume_url: Option<String>,
    /// Rich context for LLM email generation. Describe skills, achievements,
    /// projects, education — anything that helps the LLM write a convincing email.
    pub context: Option<String>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct Search {
    pub remote: SearchPreset,
    pub onsite: SearchPreset,
}

#[derive(Debug, Deserialize, Clone)]
pub struct SearchPreset {
    pub terms: Vec<String>,
    pub locations: Vec<String>,
    pub sites: Vec<String>,
    pub remote_only: bool,
}

#[derive(Debug, Deserialize, Clone)]
pub struct ScrapeConfig {
    pub max_runtime_minutes: Option<i32>,
    pub results_wanted: Option<i32>,
    pub reject_titles: Option<Vec<String>>,
    pub blocked_email_prefixes: Option<Vec<String>>,
    pub blocked_email_contains: Option<Vec<String>>,
    pub blocked_tlds: Option<Vec<String>>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct LlmConfig {
    pub providers: Vec<LlmProvider>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct Templates {
    pub email_system: TemplateContent,
    pub email_user: TemplateContent,
    pub scoring: TemplateContent,
    pub research: TemplateContent,
    pub triage: TemplateContent,
}

#[derive(Debug, Deserialize, Clone)]
pub struct TemplateContent {
    pub content: String,
}

#[derive(Debug, Deserialize, Clone)]
pub struct LlmProvider {
    pub name: String,
    pub api_key_env: String,
    pub base_url: String,
    pub model_complex: String,
    pub model_simple: String,
    pub weight: u32,
}

#[derive(Debug, Deserialize, Clone)]
pub struct TrackingConfig {
    pub server_url: Option<String>,
    pub port: Option<u16>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct EmailConfig {
    pub daily_limit: Option<i32>,
    pub delay_seconds: Option<u64>,
    pub from_name: Option<String>,
    pub from_addr: Option<String>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct TelegramConfig {
    pub chat_id: String,
}

impl Config {
    pub fn load() -> anyhow::Result<Self> {
        let config_path = Self::find_path()?;
        let raw = std::fs::read_to_string(&config_path)?;
        let resolved = resolve_env_vars(&raw);
        let mut cfg: Config = toml::from_str(&resolved)?;

        // If scrappy_config is set, load scrappy's config.toml and merge its [sites] section
        if let Some(scrappy_path) = &cfg.scrappy_config {
            let scrappy_path = if scrappy_path.starts_with("~") {
                let home = std::env::var("HOME").unwrap_or_default();
                Path::new(&home).join(&scrappy_path[2..])
            } else {
                Path::new(scrappy_path).to_path_buf()
            };
            if scrappy_path.exists() {
                let scrappy_raw = std::fs::read_to_string(&scrappy_path)
                    .map_err(|e| anyhow::anyhow!("failed to read scrappy config {}: {}", scrappy_path.display(), e))?;
                // Parse only the [sites] section from scrappy's config
                #[derive(Deserialize)]
                struct ScrappyCfg {
                    sites: Option<HashMap<String, SiteConfig>>,
                }
                if let Ok(sc) = toml::from_str::<ScrappyCfg>(&scrappy_raw) {
                    if let Some(site_overrides) = sc.sites {
                        let mut combined = cfg.sites.take().unwrap_or_default();
                        for (name, scfg) in site_overrides {
                            combined.insert(name, scfg);
                        }
                        cfg.sites = Some(combined);
                    }
                }
            } else {
                tracing::warn!("scrappy_config not found: {}", scrappy_path.display());
            }
        }

        Ok(cfg)
    }

    fn find_path() -> anyhow::Result<std::path::PathBuf> {
        let candidates = vec![
            Path::new(".data/config.toml").to_path_buf(),
            Path::new("config.toml").to_path_buf(),
        ];
        for p in &candidates {
            if p.exists() {
                return Ok(p.clone());
            }
        }
        anyhow::bail!("config.toml not found. Create .data/config.toml or config.toml")
    }
}

fn resolve_env_vars(raw: &str) -> String {
    let mut out = String::with_capacity(raw.len());
    let mut chars = raw.chars().peekable();
    while let Some(c) = chars.next() {
        if c == '$' {
            match chars.peek() {
                Some('$') => { out.push('$'); chars.next(); }
                Some('{') => {
                    chars.next();
                    let mut name = String::new();
                    for ch in chars.by_ref() {
                        if ch == '}' { break; }
                        name.push(ch);
                    }
                    out.push_str(&std::env::var(&name).unwrap_or_default());
                }
                Some(_) => {
                    let mut name = String::new();
                    let mut delim: Option<char> = None;
                    for ch in chars.by_ref() {
                        if ch.is_alphanumeric() || ch == '_' { name.push(ch); }
                        else { delim = Some(ch); break; }
                    }
                    out.push_str(&std::env::var(&name).unwrap_or_default());
                    if let Some(d) = delim { out.push(d); }
                }
                None => out.push('$'),
            }
        } else {
            out.push(c);
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_env_resolve_bare() {
        std::env::set_var("TMP_TEST_BARE", "hello");
        assert_eq!(resolve_env_vars("prefix $TMP_TEST_BARE suffix"), "prefix hello suffix");
    }

    #[test]
    fn test_env_resolve_braced() {
        std::env::set_var("TMP_TEST_BRACED", "world");
        assert_eq!(resolve_env_vars("prefix ${TMP_TEST_BRACED} suffix"), "prefix world suffix");
    }

    #[test]
    fn test_env_resolve_escape() {
        assert_eq!(resolve_env_vars("price is $$5"), "price is $5");
    }
}
