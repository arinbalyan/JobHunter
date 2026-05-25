package plugins

import (
	"context"
	"fmt"

	"github.com/arinbalyan/jobhunter/internal/plugin/sdk"
)

// RegisterBuiltinPlugins registers all built-in plugins with the manager.
func RegisterBuiltinPlugins(mgr *PluginRegistrar) {
	// Register the core job hunter agent as a plugin
	mgr.Register(&JobHunterPlugin{}, nil) // env will be injected by registrar
}

// PluginRegistrar abstracts the registration process.
type PluginRegistrar interface {
	Register(plugin sdk.Plugin, envProvider interface{})
}
