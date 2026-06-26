use serde::Deserialize;
use std::path::Path;

// ponytail: only fields that Phase 1 reads. Extra TOML sections silently ignored.

#[derive(Debug, Deserialize, Clone)]
pub struct Config {
    pub user: User,
    pub search: Search,
    pub scrape: ScrapeConfig,
    pub telegram: TelegramConfig,
    pub llm: LlmConfig,
}

#[derive(Debug, Deserialize, Clone)]
pub struct User {
    pub name: String,
}

#[derive(Debug, Deserialize, Clone)]
pub struct Search {
    pub sites: Vec<String>,
    pub locations: Vec<String>,
    pub remote_only: bool,
}

#[derive(Debug, Deserialize, Clone)]
pub struct ScrapeConfig {
    pub max_runtime_minutes: Option<i32>,
    pub results_wanted: Option<i32>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct LlmConfig {
    pub providers: Vec<LlmProvider>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct LlmProvider {
    pub name: String,
    pub api_key_env: String,
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
                    for ch in chars.by_ref() {
                        if ch.is_alphanumeric() || ch == '_' { name.push(ch); }
                        else { out.push(ch); break; }
                    }
                    out.push_str(&std::env::var(&name).unwrap_or_default());
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
        assert_eq!(resolve_env_vars("prefix $TEST_KEY suffix"), "prefix hello suffix");
    }

    #[test]
    fn test_env_resolve_braced() {
        std::env::set_var("TEST_KEY", "world");
        assert_eq!(resolve_env_vars("prefix ${TEST_KEY} suffix"), "prefix world suffix");
    }

    #[test]
    fn test_env_resolve_escape() {
        assert_eq!(resolve_env_vars("price is $$5"), "price is $5");
    }
}
