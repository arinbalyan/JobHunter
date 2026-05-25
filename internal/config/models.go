package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// OpenRouterModel represents a model from OpenRouter's API.
type OpenRouterModel struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	ContextLength  int               `json:"context_length"`
	Pricing        map[string]string `json:"pricing"`
	Architecture   map[string]interface{} `json:"architecture,omitempty"`
	Description    string            `json:"description,omitempty"`
}

// FetchFreeModels queries the OpenRouter API for all models with ":free" suffix.
// Returns them sorted by context length descending (most capable first).
func FetchFreeModels(apiKey string) ([]OpenRouterModel, error) {
	if apiKey == "" {
		return nil, nil
	}

	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Data []OpenRouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse models: %w", err)
	}

	// Filter for free models only
	var free []OpenRouterModel
	seen := make(map[string]bool)
	for _, m := range result.Data {
		// Check if model has ":free" suffix or has zero pricing
		isFree := false
		if len(m.ID) >= 5 && m.ID[len(m.ID)-5:] == ":free" {
			isFree = true
		}
		// Also check pricing
		if !isFree && m.Pricing != nil {
			if completion, ok := m.Pricing["completion"]; ok && (completion == "0" || completion == "0.0") {
				isFree = true
			}
		}
		if isFree && !seen[m.ID] {
			seen[m.ID] = true
			free = append(free, m)
		}
	}

	// Sort: largest context first (most capable)
	sort.Slice(free, func(i, j int) bool {
		return free[i].ContextLength > free[j].ContextLength
	})

	return free, nil
}
