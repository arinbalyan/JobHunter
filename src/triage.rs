use crate::llm::{self, ChatMessage};

// ponytail: classify a recruiter reply as positive/negative/neutral/unclear.

pub async fn run(cfg: &crate::config::Config, reply: &str) -> anyhow::Result<String> {
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

    if providers.is_empty() {
        anyhow::bail!("no LLM providers with API keys configured");
    }

    let mut router = llm::Router::new(providers);
    let prompt = cfg.templates.triage.content.replace("{reply_text}", reply);
    let msg = ChatMessage { role: "user".to_string(), content: prompt };

    let result = router.complete(&[msg], false).await?;
    let classification = result.trim().to_lowercase();

    let valid = ["positive", "negative", "neutral", "unclear"];
    let classification = if valid.contains(&classification.as_str()) {
        classification
    } else {
        // Try to find a valid word in the response
        valid.iter().find(|v| classification.contains(*v))
            .unwrap_or(&"unclear")
            .to_string()
    };

    Ok(classification)
}
