package router

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
)

// ProviderKind identifies which LLM provider is used.
type ProviderKind string

const (
	ProviderOpenRouter  ProviderKind = "openrouter"
	ProviderGroq        ProviderKind = "groq"
	ProviderCerebras    ProviderKind = "cerebras"
	ProviderTogether    ProviderKind = "together"
	ProviderDeepInfra   ProviderKind = "deepinfra"
	ProviderFireworks   ProviderKind = "fireworks"
	ProviderHyperbolic  ProviderKind = "hyperbolic"
	ProviderSambaNova   ProviderKind = "sambanova"
	ProviderZAI         ProviderKind = "zai"
)

// TaskComplexity classifies the complexity of an LLM request.
type TaskComplexity int

const (
	TaskSimple  TaskComplexity = iota // Template fill, classification, scoring
	TaskMedium                        // Email generation, basic reasoning
	TaskComplex                       // Experience gap reasoning, strategy, negotiation
)

// ProviderStatus tracks the health and usage of a provider.
type ProviderStatus struct {
	Kind       ProviderKind
	BaseURL    string
	APIKey     string
	Model      string
	Healthy    bool
	LastUsed   int64
	TokenCount int64
	FailCount  int64
	mu         sync.RWMutex
}

// Router manages multiple LLM providers with round-robin and fallback.
type Router struct {
	providers  []*ProviderStatus
	rrIndex    atomic.Uint64
	totalTokens atomic.Int64
	maxTokens  int64
	mu         sync.RWMutex
}

// New creates a new LLM router.
// providers is a list of provider configurations in priority order.
// maxTokensPerRun limits total token consumption (0 = unlimited).
func New(providers []ProviderConfig, maxTokensPerRun int64) *Router {
	r := &Router{
		providers: make([]*ProviderStatus, len(providers)),
		maxTokens: maxTokensPerRun,
	}
	for i, p := range providers {
		r.providers[i] = &ProviderStatus{
			Kind:    p.Kind,
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
			Healthy: true,
		}
	}
	return r
}

// ProviderConfig holds configuration for a single LLM provider.
type ProviderConfig struct {
	Kind    ProviderKind
	APIKey  string
	BaseURL string
	Model   string
	Weight  int // higher = more likely to be selected
}

// CompletionRequest holds a request to an LLM provider.
type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
	JSONMode     bool // if true, request JSON-structured output
}

// CompletionResponse holds the LLM response.
type CompletionResponse struct {
	Content      string
	TokenUsage   int
	ProviderUsed ProviderKind
	ModelUsed    string
}

// Complete sends a completion request to the best available provider.
func (r *Router) Complete(ctx context.Context, task TaskComplexity, req *CompletionRequest) (*CompletionResponse, error) {
	// Check token budget
	if r.maxTokens > 0 && r.totalTokens.Load() >= r.maxTokens {
		return nil, fmt.Errorf("token budget exhausted (%d/%d)", r.totalTokens.Load(), r.maxTokens)
	}

	// Select the right provider based on task complexity
	provider := r.selectProvider(task)
	if provider == nil {
		return nil, fmt.Errorf("no healthy provider available")
	}

	// Cap max tokens per request
	maxTokens := req.MaxTokens
	if maxTokens <= 0 || maxTokens > 2048 {
		maxTokens = 2048
	}

	// Track remaining budget
	if r.maxTokens > 0 {
		remaining := r.maxTokens - r.totalTokens.Load()
		if int64(maxTokens) > remaining {
			maxTokens = int(remaining)
		}
	}

	// Build the request for this provider
	resp, err := r.callProvider(ctx, provider, req)
	if err != nil {
		provider.mu.Lock()
		provider.FailCount++
		provider.mu.Unlock()

		// Try next available provider as fallback
		fallback := r.nextHealthy(provider)
		if fallback != nil {
			return r.callProvider(ctx, fallback, req)
		}
		return nil, fmt.Errorf("all providers failed: %w", err)
	}

	// Track token usage
	r.totalTokens.Add(int64(resp.TokenUsage))
	provider.mu.Lock()
	provider.TokenCount += int64(resp.TokenUsage)
	provider.mu.Unlock()

	return resp, nil
}

// selectProvider picks the best provider for the given task complexity.
// Uses weighted selection: fast providers for simple, capable for complex.
func (r *Router) selectProvider(task TaskComplexity) *ProviderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Filter to healthy providers only
	var candidates []*ProviderStatus
	for _, p := range r.providers {
		if p.Healthy {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	// Fast providers for simple tasks
	fastProviders := map[ProviderKind]bool{
		ProviderGroq: true, ProviderCerebras: true,
		ProviderTogether: true, ProviderDeepInfra: true,
		ProviderFireworks: true, ProviderHyperbolic: true,
	}

	// Capable providers for complex tasks
	capableProviders := map[ProviderKind]bool{
		ProviderOpenRouter: true, ProviderSambaNova: true,
		ProviderZAI: true,
	}

	switch task {
	case TaskSimple:
		for _, p := range candidates {
			if fastProviders[p.Kind] {
				return p
			}
		}
	case TaskComplex:
		for _, p := range candidates {
			if capableProviders[p.Kind] {
				return p
			}
		}
	}

	// Weighted round-robin: providers with higher weight are picked more often
	// We double the index for higher weight models
	var weighted []*ProviderStatus
	for _, p := range candidates {
		weight := 1
		// Extract weight from provider's model (heuristic)
		if p.Kind == ProviderOpenRouter || p.Kind == ProviderGroq {
			weight = 3
		}
		for i := 0; i < weight; i++ {
			weighted = append(weighted, p)
		}
	}

	idx := r.rrIndex.Add(1) % uint64(len(weighted))
	return weighted[idx]
}

// nextHealthy returns the next healthy provider after the given one.
func (r *Router) nextHealthy(after *ProviderStatus) *ProviderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	start := 0
	for i, p := range r.providers {
		if p == after {
			start = i + 1
			break
		}
	}

	for i := start; i < len(r.providers); i++ {
		if r.providers[i].Healthy {
			return r.providers[i]
		}
	}
	// Wrap around
	for i := 0; i < start; i++ {
		if r.providers[i].Healthy {
			return r.providers[i]
		}
	}
	return nil
}

// callProvider sends the actual API request.
func (r *Router) callProvider(ctx context.Context, p *ProviderStatus, req *CompletionRequest) (*CompletionResponse, error) {
	// This will be dispatched to the correct provider implementation
	dispatch := getDispatcher(p.Kind)
	if dispatch == nil {
		return nil, fmt.Errorf("no dispatcher for provider: %s", p.Kind)
	}

	resp, err := dispatch(ctx, p.BaseURL, p.APIKey, p.Model, req)
	if err != nil {
		return nil, err
	}

	return &CompletionResponse{
		Content:      resp.Content,
		TokenUsage:   resp.TokenUsage,
		ProviderUsed: p.Kind,
		ModelUsed:    p.Model,
	}, nil
}

// Stats returns usage statistics for all providers.
func (r *Router) Stats() map[ProviderKind]struct {
	Tokens   int64
	Fails    int64
	Healthy  bool
} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[ProviderKind]struct {
		Tokens  int64
		Fails   int64
		Healthy bool
	})
	for _, p := range r.providers {
		p.mu.RLock()
		stats[p.Kind] = struct {
			Tokens  int64
			Fails   int64
			Healthy bool
		}{
			Tokens:  p.TokenCount,
			Fails:   p.FailCount,
			Healthy: p.Healthy,
		}
		p.mu.RUnlock()
	}
	return stats
}

// TotalTokensUsed returns total tokens consumed across all providers.
func (r *Router) TotalTokensUsed() int64 {
	return r.totalTokens.Load()
}

// tokenCost estimates token usage for a prompt.
func estimateTokens(texts ...string) int {
	total := 0
	for _, t := range texts {
		// Rough estimate: ~4 chars per token for English text
		total += int(math.Ceil(float64(len(t)) / 4.0))
	}
	return total
}
