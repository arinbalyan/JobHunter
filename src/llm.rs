use serde::{Deserialize, Serialize};
use std::time::Instant;

// ponytail: one module for router + discovery + completion. All providers are OpenAI-compatible.

const MAX_FAILOVER: usize = 3;
const COOLDOWN_SECS: u64 = 30;
const MAX_FAILURES: u32 = 3;

#[derive(Debug, Clone)]
pub struct Provider {
    pub name: String,
    pub api_key: String,
    pub base_url: String,
    pub model_complex: String,
    pub model_simple: String,
    pub weight: u32,
}

#[derive(Debug, Clone)]
struct Health {
    failures: u32,
    cooldown_until: Option<Instant>,
}

#[derive(Debug)]
pub struct Router {
    providers: Vec<Provider>,
    health: Vec<Health>,
    rng: fastrand::Rng,
}

#[derive(Debug, Deserialize)]
struct ModelsResponse {
    data: Vec<ModelEntry>,
}

#[derive(Debug, Deserialize)]
struct ModelEntry {
    id: String,
}

impl Router {
    pub fn new(providers: Vec<Provider>) -> Self {
        let health = vec![
            Health { failures: 0, cooldown_until: None };
            providers.len()
        ];
        Router { providers, health, rng: fastrand::Rng::new() }
    }

    /// Select a healthy provider using weighted random selection.
    fn select(&mut self) -> Option<(usize, &Provider)> {
        let n = self.providers.len();
        // Build a list of healthy indices, weighted
        let mut candidates: Vec<usize> = Vec::new();
        for i in 0..n {
            let h = &self.health[i];
            if h.failures >= MAX_FAILURES {
                if let Some(cooldown) = h.cooldown_until {
                    if Instant::now() < cooldown {
                        continue; // still cooling down
                    }
                    // cooldown expired — reset
                    self.health[i].failures = 0;
                    self.health[i].cooldown_until = None;
                }
            }
            let weight = self.providers[i].weight.max(1) as usize;
            for _ in 0..weight { candidates.push(i); }
        }
        if candidates.is_empty() { return None; }
        let idx = candidates[self.rng.usize(0..candidates.len())];
        Some((idx, &self.providers[idx]))
    }

    /// Mark a provider as failed, potentially triggering cooldown.
    pub fn record_failure(&mut self, idx: usize) {
        if idx >= self.health.len() { return; }
        let h = &mut self.health[idx];
        h.failures += 1;
        if h.failures >= MAX_FAILURES {
            h.cooldown_until = Some(Instant::now() + std::time::Duration::from_secs(COOLDOWN_SECS));
            tracing::warn!("{} — {}/{} failures, cooling down for {}s",
                self.providers[idx].name, h.failures, MAX_FAILURES, COOLDOWN_SECS);
        }
    }

    /// Mark a provider as healthy after a successful call.
    pub fn record_success(&mut self, idx: usize) {
        if idx < self.health.len() {
            self.health[idx].failures = 0;
            self.health[idx].cooldown_until = None;
        }
    }

    /// Send a chat completion request with failover across providers.
    pub async fn complete(
        &mut self,
        messages: &[ChatMessage],
        complex: bool,
    ) -> anyhow::Result<String> {
        let mut last_err = anyhow::anyhow!("no providers available");

        for _ in 0..MAX_FAILOVER {
            let (idx, provider) = match self.select() {
                Some(p) => p,
                None => { return Err(last_err); }
            };

            let model = if complex { &provider.model_complex } else { &provider.model_simple };

            match send_completion(provider, model, messages).await {
                Ok(text) => {
                    self.record_success(idx);
                    return Ok(text);
                }
                Err(e) => {
                    tracing::warn!("{} failed: {}", provider.name, e);
                    self.record_failure(idx);
                    last_err = e;
                }
            }
        }

        Err(last_err)
    }

    /// Check /models endpoint for each provider and log available models.
    pub async fn discover_models(&self) {
        for provider in &self.providers {
            let url = format!("{}/models", provider.base_url.trim_end_matches('/'));
            match reqwest::Client::new()
                .get(&url)
                .header("Authorization", format!("Bearer {}", provider.api_key))
                .send()
                .await
            {
                Ok(resp) => {
                    if let Ok(body) = resp.json::<ModelsResponse>().await {
                        let models: Vec<&str> = body.data.iter().map(|m| m.id.as_str()).collect();
                        tracing::info!("{} — {} models available", provider.name, models.len());
                        // Check configured models
                        let complex_ok = models.contains(&provider.model_complex.as_str());
                        let simple_ok = models.contains(&provider.model_simple.as_str());
                        if !complex_ok {
                            tracing::warn!("{} — complex model '{}' not found in /models", provider.name, provider.model_complex);
                        }
                        if !simple_ok {
                            tracing::warn!("{} — simple model '{}' not found in /models", provider.name, provider.model_simple);
                        }
                    } else {
                        tracing::info!("{} — /models returned non-standard format", provider.name);
                    }
                }
                Err(e) => {
                    tracing::info!("{} — /models check failed: {}", provider.name, e);
                }
            }
        }
    }
}

// ── OpenAI-compatible chat completion ─────────────────────

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct ChatMessage {
    pub role: String,
    pub content: String,
}

#[derive(Serialize)]
struct CompletionRequest {
    model: String,
    messages: Vec<ChatMessage>,
    temperature: f64,
    max_tokens: u32,
}

#[derive(Deserialize)]
struct CompletionResponse {
    choices: Vec<Choice>,
}

#[derive(Deserialize)]
struct Choice {
    message: ChatMessage,
}

async fn send_completion(
    provider: &Provider,
    model: &str,
    messages: &[ChatMessage],
) -> anyhow::Result<String> {
    let url = format!("{}/chat/completions", provider.base_url.trim_end_matches('/'));
    let body = CompletionRequest {
        model: model.to_string(),
        messages: messages.to_vec(),
        temperature: 0.7,
        max_tokens: 2048,
    };

    let resp = reqwest::Client::new()
        .post(&url)
        .header("Authorization", format!("Bearer {}", provider.api_key))
        .json(&body)
        .send()
        .await?;

    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await.unwrap_or_default();
        anyhow::bail!("{} returned {}: {}", provider.name, status, text);
    }

    let data: CompletionResponse = resp.json().await?;
    Ok(data.choices.into_iter().next()
        .ok_or_else(|| anyhow::anyhow!("{} returned empty choices", provider.name))?
        .message.content)
}
