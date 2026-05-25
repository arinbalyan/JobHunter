package plugins

import "github.com/arinbalyan/jobhunter/internal/plugin/sdk"

// RegisterBuiltinPlugins registers all built-in plugins with the manager.
func RegisterBuiltinPlugins(mgr PluginRegistrar) {
	mgr.Register(NewJobHunterPlugin(), nil)
}

// PluginRegistrar is the interface the plugin manager must implement.
type PluginRegistrar interface {
	Register(plugin sdk.Plugin, envProvider interface{})
}
