package router

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arinbalyan/jobhunter/internal/logging"
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

	// maxFailoverAttempts is the number of additional providers to try after
	// the initial provider fails (total max = maxFailoverAttempts + 1 providers).
	maxFailoverAttempts = 2

	// maxConsecutiveFailures before a provider is marked unhealthy.
	maxConsecutiveFailures = 3

	// cooldownDuration: an unhealthy provider is re-tested after this interval.
	cooldownDuration = 30 * time.Second
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
	Name               string       // Human-readable provider name (e.g. "openrouter")
	Kind               ProviderKind
	BaseURL            string
	APIKey             string
	Model              string
	Healthy            bool
	LastUsed           int64
	TokenCount         int64
	FailCount          int64
	ConsecutiveFailures int64      // incremented on each failure, reset on success
	LastFailureTime     int64      // unix timestamp of last failure (0 if never failed)
	mu                 sync.RWMutex
}

// Router manages multiple LLM providers with round-robin and failover.
type Router struct {
	logger     *logging.Logger
	providers  []*ProviderStatus
	rrIndex    atomic.Uint64
	totalTokens atomic.Int64
	maxTokens  int64
	mu         sync.RWMutex
}

// New creates a new LLM router.
// providers is a list of provider configurations in priority order.
// maxTokensPerRun limits total token consumption (0 = unlimited).
// logger is used for logging provider failures and failover decisions.
// If logger is nil, no log output is produced.
func New(providers []ProviderConfig, maxTokensPerRun int64, logger *logging.Logger) *Router {
	r := &Router{
		logger:    logger,
		providers: make([]*ProviderStatus, len(providers)),
		maxTokens: maxTokensPerRun,
	}
	for i, p := range providers {
		name := string(p.Kind)
		r.providers[i] = &ProviderStatus{
			Name:    name,
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

// Complete sends a completion request with automatic failover across providers.
// On failure (any non-context error), it retries up to maxFailoverAttempts additional
// providers before returning the final error. Already-tried providers are skipped.
func (r *Router) Complete(ctx context.Context, task TaskComplexity, req *CompletionRequest) (*CompletionResponse, error) {
	// Check token budget
	if r.maxTokens > 0 && r.totalTokens.Load() >= r.maxTokens {
		return nil, fmt.Errorf("token budget exhausted (%d/%d)", r.totalTokens.Load(), r.maxTokens)
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

	// Adjust the request with the capped token count
	cappedReq := *req
	cappedReq.MaxTokens = maxTokens

	var lastErr error
	tried := make(map[string]bool)

	for attempts := 0; attempts <= maxFailoverAttempts; attempts++ {
		// Select a fresh provider on each attempt
		provider := r.selectProvider(task)
		if provider == nil {
			return nil, fmt.Errorf("no healthy provider available")
		}

		// If we've already tried this provider, find another one
		if tried[provider.Name] {
			provider = r.findUntriedProvider(task, tried)
			if provider == nil {
				return nil, fmt.Errorf("all providers exhausted: %w", lastErr)
			}
		}

		tried[provider.Name] = true

		if r.logger != nil {
			r.logger.Debug("calling provider %s (attempt %d/%d)", provider.Name, attempts+1, maxFailoverAttempts+1)
		}

		resp, err := r.callProvider(ctx, provider, &cappedReq)
		if err == nil {
			// Success — track token usage, reset failure count, and return
			r.totalTokens.Add(int64(resp.TokenUsage))
			provider.mu.Lock()
			provider.TokenCount += int64(resp.TokenUsage)
			provider.ConsecutiveFailures = 0
			if !provider.Healthy {
				provider.Healthy = true
				providerName := provider.Name
				provider.mu.Unlock()
				if r.logger != nil {
					r.logger.Info("provider %s recovered and marked healthy", providerName)
				}
			} else {
				provider.mu.Unlock()
			}
			return resp, nil
		}

		lastErr = err

		// Increment fail count and track consecutives for health management
		provider.mu.Lock()
		provider.FailCount++
		provider.ConsecutiveFailures++
		provider.LastFailureTime = time.Now().Unix()
		if provider.ConsecutiveFailures >= maxConsecutiveFailures {
			provider.Healthy = false
			providerName := provider.Name
			provider.mu.Unlock()
			if r.logger != nil {
				r.logger.Warn("provider %s marked unhealthy after %d consecutive failures (cooling down for %.0fs)",
					providerName, maxConsecutiveFailures, cooldownDuration.Seconds())
			}
		} else {
			provider.mu.Unlock()
		}

		if r.logger != nil {
			r.logger.Warn("provider %s failed: %v — trying next provider", provider.Name, err)
		}
	}

	return nil, fmt.Errorf("all providers failed after %d attempts: %w", maxFailoverAttempts+1, lastErr)
}

// selectProvider picks the best provider for the given task complexity.
// Uses weighted selection: fast providers for simple, capable for complex.
// Auto-recovers providers whose cooldown has expired.
func (r *Router) selectProvider(task TaskComplexity) *ProviderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().Unix()

	// Filter to healthy providers only, and auto-recover those past cooldown
	var candidates []*ProviderStatus
	for _, p := range r.providers {
		if p.Healthy {
			candidates = append(candidates, p)
		} else if p.LastFailureTime > 0 && now-p.LastFailureTime > int64(cooldownDuration.Seconds()) {
			// Cooldown expired — recover this provider
			p.Healthy = true
			p.ConsecutiveFailures = 0
			if r.logger != nil {
				r.logger.Info("provider %s auto-recovered (cooldown expired)", p.Name)
			}
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

// findUntriedProvider returns the first healthy provider not in the tried map.
// It prefers providers suited for the given task complexity.
func (r *Router) findUntriedProvider(task TaskComplexity, tried map[string]bool) *ProviderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// First pass: filter healthy, untried providers
	var candidates []*ProviderStatus
	for _, p := range r.providers {
		if !tried[p.Name] && p.Healthy {
			candidates = append(candidates, p)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Prefer fast providers for simple tasks, capable for complex
	fastProviders := map[ProviderKind]bool{
		ProviderGroq: true, ProviderCerebras: true,
		ProviderTogether: true, ProviderDeepInfra: true,
		ProviderFireworks: true, ProviderHyperbolic: true,
	}
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

	// Fall back to first available candidate
	return candidates[0]
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


