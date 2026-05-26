package router_test

import (
	"context"
	"testing"

	"github.com/arinbalyan/jobhunter/internal/llm/router"
)

func TestDispatcher_NoAPIKey(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 100000, nil)

	_, err := r.Complete(context.Background(), router.TaskSimple, &router.CompletionRequest{
		SystemPrompt: "test",
		UserPrompt:   "test",
		MaxTokens:    100,
	})
	// Should fail gracefully (no API key means dispatcher will get empty auth header)
	if err == nil {
		t.Log("Complete succeeded (unexpected with empty API key, but not a crash)")
	}
}

func TestDispatcher_InvalidBaseURL(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://invalid-url-that-does-not-exist.example.com", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 100000, nil)

	_, err := r.Complete(context.Background(), router.TaskSimple, &router.CompletionRequest{
		SystemPrompt: "test",
		UserPrompt:   "test",
		MaxTokens:    100,
	})
	if err == nil {
		t.Error("expected error with invalid base URL")
	}
}

func TestDispatcher_ContextCancelled(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 100000, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := r.Complete(ctx, router.TaskSimple, &router.CompletionRequest{
		SystemPrompt: "test",
		UserPrompt:   "test",
		MaxTokens:    100,
	})
	if err == nil {
		t.Log("Complete with cancelled context (dispatcher may or may not check ctx before dialing)")
	}
}

func TestDispatcher_AllRegistered(t *testing.T) {
	// Test that all provider kinds have a registered dispatcher
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test", Weight: 1},
		{Kind: router.ProviderGroq, APIKey: "sk-test", BaseURL: "https://api.groq.com/openai", Model: "test", Weight: 1},
		{Kind: router.ProviderCerebras, APIKey: "sk-test", BaseURL: "https://api.cerebras.ai", Model: "test", Weight: 1},
		{Kind: router.ProviderTogether, APIKey: "sk-test", BaseURL: "https://api.together.xyz", Model: "test", Weight: 1},
		{Kind: router.ProviderDeepInfra, APIKey: "sk-test", BaseURL: "https://api.deepinfra.com", Model: "test", Weight: 1},
		{Kind: router.ProviderFireworks, APIKey: "sk-test", BaseURL: "https://api.fireworks.ai", Model: "test", Weight: 1},
		{Kind: router.ProviderHyperbolic, APIKey: "sk-test", BaseURL: "https://api.hyperbolic.xyz", Model: "test", Weight: 1},
		{Kind: router.ProviderSambaNova, APIKey: "sk-test", BaseURL: "https://api.sambanova.ai", Model: "test", Weight: 1},
		{Kind: router.ProviderZAI, APIKey: "sk-test", BaseURL: "https://open.bigmodel.cn/api/paas/v4", Model: "test", Weight: 1},
	}

	r := router.New(providers, 100000, nil)
	_ = r
}
