package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DispatcherFunc dispatches a completion request to a specific LLM API.
type DispatcherFunc func(ctx context.Context, baseURL, model string, req *CompletionRequest) (*providerResponse, error)

// providerResponse is the standardised internal response from a provider.
type providerResponse struct {
	Content    string `json:"content"`
	TokenUsage int    `json:"token_usage"`
}

// Global dispatcher registry.
var dispatchers = make(map[ProviderKind]DispatcherFunc)

// registerDispatcher registers a dispatcher for a provider kind.
func registerDispatcher(kind ProviderKind, fn DispatcherFunc) {
	dispatchers[kind] = fn
}

// getDispatcher returns the dispatcher for the given provider kind.
func getDispatcher(kind ProviderKind) DispatcherFunc {
	return dispatchers[kind]
}

// defaultHTTPClient is reused across all API calls.
var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     60 * time.Second,
		DisableCompression:  false,
	},
}

// ─── OpenRouter Dispatcher ─────────────────────────────────────────
// OpenRouter uses OpenAI-compatible API format.

type openRouterRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func init() {
	registerDispatcher(ProviderOpenRouter, dispatchOpenRouter)
	registerDispatcher(ProviderGroq, dispatchOpenRouter)     // Groq uses OpenAI-compatible API
	registerDispatcher(ProviderCerebras, dispatchOpenRouter) // Cerebras uses OpenAI-compatible API
}

func dispatchOpenRouter(ctx context.Context, baseURL, model string, req *CompletionRequest) (*providerResponse, error) {
	// Build messages
	messages := []message{
		{Role: "system", Content: req.SystemPrompt},
		{Role: "user", Content: req.UserPrompt},
	}

	orReq := openRouterRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	// Enable JSON mode if requested
	if req.JSONMode {
		orReq.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(orReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := defaultHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*100)) // 100KB max
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var orResp openRouterResponse
	if err := json.Unmarshal(respBody, &orResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if orResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", orResp.Error.Message)
	}

	if len(orResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &providerResponse{
		Content:    orResp.Choices[0].Message.Content,
		TokenUsage: orResp.Usage.TotalTokens,
	}, nil
}
