use serde::Deserialize;
use std::path::Path;

// ponytail: single config struct, no layers, no MergeIntoConfig.

#[derive(Debug, Deserialize)]
pub struct Config {
    pub profile: Profile,
    pub search: Search,
    pub email: EmailConfig,
    pub llm: LlmConfig,
    pub telegram: TelegramConfig,
    pub templates: Templates,
}

#[derive(Debug, Deserialize)]
pub struct Profile {
    pub name: String,
    pub resume_path: Option<String>,
    pub timezone: Option<String>,
    pub weekend_skip: Option<bool>,
}

#[derive(Debug, Deserialize)]
pub struct Search {
    pub remote: SearchPreset,
    pub onsite: SearchPreset,
}

#[derive(Debug, Deserialize)]
pub struct SearchPreset {
    pub terms: Vec<String>,
    pub locations: Vec<String>,
    pub sites: Vec<String>,
    pub remote_only: bool,
    pub hours_old: Option<i32>,
    pub min_score: Option<i32>,
}

#[derive(Debug, Deserialize)]
pub struct EmailConfig {
    pub daily_limit: i32,
    pub delay_seconds: u64,
    pub dedup_window_days: i32,
    pub cooldown_days: i32,
    pub from_name: Option<String>,
    pub subject_template: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct LlmConfig {
    pub max_tokens_per_run: i32,
    pub max_tokens_per_request: i32,
    pub temperature: f64,
    pub providers: Vec<LlmProvider>,
}

#[derive(Debug, Deserialize)]
pub struct LlmProvider {
    pub name: String,
    pub api_key_env: String,
    pub base_url: String,
    pub model_complex: String,
    pub model_simple: String,
    pub weight: u32,
}

#[derive(Debug, Deserialize)]
pub struct TelegramConfig {
    pub bot_token_env: String,
    pub chat_id: String,
}

#[derive(Debug, Deserialize)]
pub struct Templates {
    pub email_system: Template,
    pub email_user: Template,
    pub scoring_system: Option<Template>,
    pub scoring_user: Option<Template>,
}

#[derive(Debug, Deserialize)]
pub struct Template {
    pub content: String,
}

impl Config {
    pub fn load() -> anyhow::Result<Self> {
        let config_path = Self::find_path()?;
        let raw = std::fs::read_to_string(&config_path)?;
        let resolved = resolve_env_vars(&raw);
        let cfg: Config = toml::from_str(&resolved)?;
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
        anyhow::bail!(
            "config.toml not found. Create .data/config.toml or config.toml"
        );
    }
}

/// Resolve `$VAR` and `${VAR}` references in the raw TOML string.
/// Leaves literal `$$` as `$`. Non-existent vars resolve to empty string.
fn resolve_env_vars(raw: &str) -> String {
    let mut out = String::with_capacity(raw.len());
    let mut chars = raw.chars().peekable();
    while let Some(c) = chars.next() {
        if c == '$' {
            match chars.peek() {
                Some('$') => {
                    out.push('$');
                    chars.next();
                }
                Some('{') => {
                    chars.next(); // skip {
                    let mut name = String::new();
                    for ch in chars.by_ref() {
                        if ch == '}' {
                            break;
                        }
                        name.push(ch);
                    }
                    let val = std::env::var(&name).unwrap_or_default();
                    out.push_str(&val);
                }
                Some(_) => {
                    let mut name = String::new();
                    // ponytail: alphanumeric + underscore only for bare $VAR
                    for ch in chars.by_ref() {
                        if ch.is_alphanumeric() || ch == '_' {
                            name.push(ch);
                        } else {
                            out.push(ch);
                            break;
                        }
                    }
                    let val = std::env::var(&name).unwrap_or_default();
                    out.push_str(&val);
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
        std::env::set_var("TEST_KEY", "hello");
        let result = resolve_env_vars("prefix $TEST_KEY suffix");
        assert_eq!(result, "prefix hello suffix");
    }

    #[test]
    fn test_env_resolve_braced() {
        std::env::set_var("TEST_KEY", "world");
        let result = resolve_env_vars("prefix ${TEST_KEY} suffix");
        assert_eq!(result, "prefix world suffix");
    }

    #[test]
    fn test_env_resolve_escape() {
        let result = resolve_env_vars("price is $$5");
        assert_eq!(result, "price is $5");
    }

    #[test]
    fn test_env_resolve_missing() {
        let result = resolve_env_vars("hello $NONEXISTENT world");
        assert_eq!(result, "hello  world");
    }
}
