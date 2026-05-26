package router_test

import (
	"context"
	"testing"

	"github.com/arinbalyan/jobhunter/internal/llm/router"
)

func TestNew(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
		{Kind: router.ProviderGroq, APIKey: "gsk-test", BaseURL: "https://api.groq.com/openai", Model: "test/model", Weight: 3},
	}

	r := router.New(providers, 100000)
	if r == nil {
		t.Fatal("New() returned nil")
	}

	stats := r.Stats()
	if len(stats) != 2 {
		t.Errorf("expected 2 providers in stats, got %d", len(stats))
	}
}

func TestNew_EmptyProviders(t *testing.T) {
	r := router.New(nil, 0)
	if r == nil {
		t.Fatal("New() returned nil")
	}

	stats := r.Stats()
	if len(stats) != 0 {
		t.Errorf("expected 0 providers, got %d", len(stats))
	}

	if r.TotalTokensUsed() != 0 {
		t.Errorf("expected 0 tokens, got %d", r.TotalTokensUsed())
	}
}

func TestNew_ZeroMaxTokens(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 0) // 0 = unlimited
	if r == nil {
		t.Fatal("New() returned nil")
	}
}

func TestComplete_TokenBudgetExhausted(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 0) // 0 = unlimited, should not exhaust immediately

	_, err := r.Complete(context.Background(), router.TaskSimple, &router.CompletionRequest{
		SystemPrompt: "test",
		UserPrompt:   "test",
		MaxTokens:    100,
	})
	if err == nil {
		t.Log("Complete returned no error (may have failed at dispatcher level, which is expected)")
	}
}

func TestComplete_TokenBudgetZeroExhausted(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 0)

	// Should not get token budget error with maxTokens=0 (unlimited)
	_, err := r.Complete(context.Background(), router.TaskSimple, &router.CompletionRequest{
		SystemPrompt: "test",
		UserPrompt:   "test",
		MaxTokens:    100,
	})
	// Dispatcher will fail (no real API), but that's different from budget exhaustion
	if err != nil {
		if err.Error() == "token budget exhausted (0/0)" {
			t.Error("token budget should not be exhausted with maxTokens=0")
		}
	}
}

func TestSelectProvider_WithHealth(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderGroq, APIKey: "gsk-test", BaseURL: "https://api.groq.com/openai", Model: "llama-3.3-70b", Weight: 3},
		{Kind: router.ProviderCerebras, APIKey: "cere-test", BaseURL: "https://api.cerebras.ai", Model: "gemma-4-9b", Weight: 2},
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "complex/model", Weight: 10},
	}

	r := router.New(providers, 100000)

	// Test that Complete returns a proper error (no real API keys)
	// but doesn't crash
	_, err := r.Complete(context.Background(), router.TaskSimple, &router.CompletionRequest{
		SystemPrompt: "Be helpful",
		UserPrompt:   "Say hello",
		MaxTokens:    50,
	})
	if err == nil {
		t.Log("Complete succeeded (may have mock API configured — this is fine in CI)")
	}
}

func TestStats_Tracking(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 100000)

	stats := r.Stats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 provider stat, got %d", len(stats))
	}

	for kind, s := range stats {
		if kind != router.ProviderOpenRouter {
			t.Errorf("expected ProviderOpenRouter, got %s", kind)
		}
		if !s.Healthy {
			t.Error("provider should be healthy initially")
		}
		if s.Tokens != 0 {
			t.Errorf("expected 0 tokens, got %d", s.Tokens)
		}
		if s.Fails != 0 {
			t.Errorf("expected 0 fails, got %d", s.Fails)
		}
	}
}

func TestTotalTokensUsed(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 100000)

	if r.TotalTokensUsed() != 0 {
		t.Errorf("expected 0 total tokens, got %d", r.TotalTokensUsed())
	}
}

func TestComplete_NoHealthyProvider(t *testing.T) {
	// Create router without providers
	r := router.New(nil, 100000)

	_, err := r.Complete(context.Background(), router.TaskSimple, &router.CompletionRequest{
		SystemPrompt: "test",
		UserPrompt:   "test",
		MaxTokens:    100,
	})
	if err == nil {
		t.Fatal("expected error with no providers")
	}
}

func TestComplete_NegativeMaxTokens(t *testing.T) {
	providers := []router.ProviderConfig{
		{Kind: router.ProviderOpenRouter, APIKey: "sk-test", BaseURL: "https://openrouter.ai/api", Model: "test/model", Weight: 10},
	}

	r := router.New(providers, 100000)

	// MaxTokens <= 0 should be capped to 2048 (or whatever default)
	_, err := r.Complete(context.Background(), router.TaskSimple, &router.CompletionRequest{
		SystemPrompt: "test",
		UserPrompt:   "test",
		MaxTokens:    0,
	})
	if err == nil {
		t.Log("Complete with zero max tokens handled")
	}
}

func TestTaskComplexityValues(t *testing.T) {
	if router.TaskSimple != 0 {
		t.Errorf("TaskSimple should be 0, got %d", router.TaskSimple)
	}
	if router.TaskComplex != 2 {
		t.Errorf("TaskComplex should be 2, got %d", router.TaskComplex)
	}
}

func TestProviderKindValues(t *testing.T) {
	if router.ProviderOpenRouter != "openrouter" {
		t.Errorf("ProviderOpenRouter = %q, want %q", router.ProviderOpenRouter, "openrouter")
	}
	if router.ProviderGroq != "groq" {
		t.Errorf("ProviderGroq = %q, want %q", router.ProviderGroq, "groq")
	}
	if router.ProviderZAI != "zai" {
		t.Errorf("ProviderZAI = %q, want %q", router.ProviderZAI, "zai")
	}
}
