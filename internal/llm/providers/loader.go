package providers

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LLMProviderConfig holds the full provider configuration from llm.yaml
type LLMProviderConfig struct {
	Providers []ProviderEntry `yaml:"providers"`
}

type ProviderEntry struct {
	Name              string          `yaml:"name"`
	EnvKey            string          `yaml:"env_key"`
	BaseURL           string          `yaml:"base_url"`
	Models            ModelMap         `yaml:"models"`
	SupportsReasoning bool            `yaml:"supports_reasoning"`
	Weight            int             `yaml:"weight"`
}

type ModelMap struct {
	Complex   string `yaml:"complex"`
	Simple    string `yaml:"simple"`
	Reasoning string `yaml:"reasoning"`
}

// ResolvedProvider is a fully resolved provider ready for the router.
type ResolvedProvider struct {
	Name     string
	Kind     string // lowercased name, used as ProviderKind
	APIKey   string
	BaseURL  string
	Model    string // the model selected based on task
	Weight   int
}

// LoadProviders loads the LLM provider config and resolves API keys.
// Returns only providers that have a valid API key set.
func LoadProviders(path string) (*LLMProviderConfig, error) {
	cfg := &LLMProviderConfig{}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read llm config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse llm config: %w", err)
	}

	return cfg, nil
}

// GetActiveProviders returns all providers with valid API keys.
// taskType is one of: complex, simple, reasoning.
func (c *LLMProviderConfig) GetActiveProviders(taskType string) []ResolvedProvider {
	if c == nil {
		return nil
	}

	var result []ResolvedProvider
	for _, p := range c.Providers {
		apiKey := os.Getenv(p.EnvKey)
		if apiKey == "" {
			continue
		}

		model := selectModel(p, taskType)
		if model == "" {
			continue
		}

		weight := p.Weight
		if weight <= 0 {
			weight = 1
		}

		result = append(result, ResolvedProvider{
			Name:    p.Name,
			Kind:    strings.ToLower(p.Name),
			APIKey:  apiKey,
			BaseURL: p.BaseURL,
			Model:   model,
			Weight:  weight,
		})
	}

	return result
}

func selectModel(p ProviderEntry, taskType string) string {
	switch taskType {
	case "complex":
		if p.Models.Complex != "" {
			return p.Models.Complex
		}
	case "simple":
		if p.Models.Simple != "" {
			return p.Models.Simple
		}
	case "reasoning":
		if p.SupportsReasoning && p.Models.Reasoning != "" {
			return p.Models.Reasoning
		}
		// fallback to complex if reasoning not available
		if p.Models.Complex != "" {
			return p.Models.Complex
		}
	}

	// Last resort fallback
	if p.Models.Complex != "" {
		return p.Models.Complex
	}
	return p.Models.Simple
}


