package settings

import (
	"os"
	"slices"
	"strings"
)

// What aex passes to plugins (internal/plugin) so they see the same settings as aex.
const (
	pluginAppRootVar  = "AEX_PLUGIN_APP_ROOT"
	pluginEmbeddedVar = "AEX_PLUGIN_EMBEDDED_ENV"
)

// PluginEnv is the environment for a plugin: this process's, plus the data folder, the app root and
// (release builds) the .env built into the exe. plain passes on --plain.
func PluginEnv(plain bool) []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		for _, name := range []string{"AEX_DATA_DIR", pluginAppRootVar, pluginEmbeddedVar} {
			if strings.HasPrefix(strings.ToUpper(kv), name+"=") {
				return true
			}
		}
		return false
	})
	env = append(env, "AEX_DATA_DIR="+DataDir, pluginAppRootVar+"="+AppRoot)
	if !IsDev {
		env = append(env, pluginEmbeddedVar+"="+embeddedEnv)
	}
	if plain {
		env = append(env, "AEX_TUI=0")
	}
	return env
}

// InitPlugin is Init for a plugin run by aex: app root and built-in .env come from aex (PluginEnv).
func InitPlugin(dataDir string) error {
	return Init(dataDir, os.Getenv(pluginEmbeddedVar))
}
