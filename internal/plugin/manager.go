package plugin

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/arinbalyan/jobhunter/internal/plugin/sdk"
)

// Manager loads and runs plugins.
type Manager struct {
	mu           sync.RWMutex
	registry     map[string]sdk.Plugin      // id -> plugin
	envProviders map[string]EnvProvider      // id -> env provider
}

// EnvProvider creates an Env for a plugin on each Execute call.
type EnvProvider func(plugin sdk.Plugin) sdk.Env

// NewManager creates a new plugin manager.
func NewManager() *Manager {
	return &Manager{
		registry:     make(map[string]sdk.Plugin),
		envProviders: make(map[string]EnvProvider),
	}
}

// Register adds a plugin to the manager.
func (m *Manager) Register(plugin sdk.Plugin, envProvider EnvProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry[plugin.ID()] = plugin
	m.envProviders[plugin.ID()] = envProvider
}

// Get returns a plugin by ID.
func (m *Manager) Get(id string) (sdk.Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.registry[id]
	return p, ok
}

// List returns all registered plugins.
func (m *Manager) List() []sdk.Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	plugins := make([]sdk.Plugin, 0, len(m.registry))
	for _, p := range m.registry {
		plugins = append(plugins, p)
	}
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].ID() < plugins[j].ID()
	})
	return plugins
}

// RunAll executes all registered plugins and returns their results.
func (m *Manager) RunAll(ctx context.Context) []PluginRunResult {
	m.mu.RLock()
	ids := make([]string, 0, len(m.registry))
	for id := range m.registry {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	sort.Strings(ids)

	var results []PluginRunResult
	for _, id := range ids {
		m.mu.RLock()
		plugin := m.registry[id]
		envProvider := m.envProviders[id]
		m.mu.RUnlock()

		result := m.run(ctx, plugin, envProvider)
		results = append(results, result)
	}
	return results
}

// RunByIDs executes specific plugins by their IDs.
func (m *Manager) RunByIDs(ctx context.Context, ids []string) []PluginRunResult {
	var results []PluginRunResult
	for _, id := range ids {
		m.mu.RLock()
		plugin, ok := m.registry[id]
		envProvider := m.envProviders[id]
		m.mu.RUnlock()

		if !ok {
			results = append(results, PluginRunResult{
				PluginID: id,
				Error:    fmt.Errorf("plugin %q not found", id),
			})
			continue
		}
		results = append(results, m.run(ctx, plugin, envProvider))
	}
	return results
}

// PluginRunResult holds the result of executing a single plugin.
type PluginRunResult struct {
	PluginID   string
	PluginName string
	Success    bool
	Message    string
	Duration   time.Duration
	Metrics    map[string]float64
	Error      error
}

func (m *Manager) run(ctx context.Context, plugin sdk.Plugin, ep EnvProvider) PluginRunResult {
	start := time.Now()
	log.Printf("[plugin] running %s (%s)...", plugin.Name(), plugin.ID())

	env := ep(plugin)
	result, err := plugin.Execute(ctx, env)

	duration := time.Since(start)
	base := PluginRunResult{
		PluginID:   plugin.ID(),
		PluginName: plugin.Name(),
		Duration:   duration,
	}

	if err != nil {
		base.Error = err
		base.Success = false
		base.Message = err.Error()
		log.Printf("[plugin] %s failed after %v: %v", plugin.Name(), duration, err)
		return base
	}

	if result != nil {
		base.Success = result.Success
		base.Message = result.Message
		base.Metrics = result.Metrics
	}

	status := "succeeded"
	if !base.Success {
		status = "completed with issues"
	}
	log.Printf("[plugin] %s %s in %v: %s", plugin.Name(), status, duration, base.Message)

	return base
}

// Summary generates a summary string of all plugin runs.
func Summary(results []PluginRunResult) string {
	if len(results) == 0 {
		return "No plugins ran."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Plugin Run Summary (%d plugins) ===\n", len(results)))
	successCount := 0
	var totalDuration time.Duration

	for _, r := range results {
		status := "✅"
		if r.Error != nil {
			status = "❌"
		} else if !r.Success {
			status = "⚠️"
		} else {
			successCount++
		}
		totalDuration += r.Duration
		sb.WriteString(fmt.Sprintf("  %s [%s] %s (%v)\n", status, r.PluginID, r.Message, r.Duration))
	}

	sb.WriteString(fmt.Sprintf("\n%d/%d plugins succeeded in %v\n", successCount, len(results), totalDuration))
	return sb.String()
}
