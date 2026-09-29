package settings

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"aex/internal/secrets"
)

// What aex passes to plugins (internal/plugin) so they see the same settings as aex.
const (
	pluginAppRootVar  = "AEX_PLUGIN_APP_ROOT"
	pluginEmbeddedVar = "AEX_PLUGIN_EMBEDDED_ENV"
	// A secret setting the plugin was granted, e.g. AEX_PLUGIN_SECRET_JIRA_TOKEN.
	pluginSecretPrefix = "AEX_PLUGIN_SECRET_"
)

// IsPlugin is true in a plugin process (InitPlugin). A plugin never opens the credential store: it
// sees only the secret settings aex passed it, and none from .env files or the environment.
var IsPlugin bool

// IsSecret reports whether a setting is kept in the credential store (JIRA_TOKEN).
func IsSecret(name string) bool { return credentialKey(name) != "" }

// PluginEnv is the environment for a plugin: this process's, plus the data folder, the app root,
// (release builds) the .env built into the exe and the values of the secret settings in pass.
// Other secret settings are left out. plain passes on --plain.
func PluginEnv(plain bool, pass []string) ([]string, error) {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(strings.ToUpper(kv), "=")
		return slices.Contains([]string{"AEX_DATA_DIR", pluginAppRootVar, pluginEmbeddedVar}, name) ||
			strings.HasPrefix(name, pluginSecretPrefix) || IsSecret(name)
	})
	env = append(env, "AEX_DATA_DIR="+DataDir, pluginAppRootVar+"="+AppRoot)
	if !IsDev {
		env = append(env, pluginEmbeddedVar+"="+embeddedEnv)
	}
	if plain {
		env = append(env, "AEX_TUI=0")
	}
	for _, name := range pass {
		if !IsSecret(name) {
			return nil, fmt.Errorf("%s is not a secret setting", name)
		}
		env = append(env, pluginSecretPrefix+name+"="+Get(name))
	}
	return env, nil
}

// InitPlugin is Init for a plugin run by aex: app root and built-in .env come from aex (PluginEnv).
func InitPlugin(dataDir string) error {
	IsPlugin = true
	return Init(dataDir, os.Getenv(pluginEmbeddedVar))
}

// openPluginCredentials takes the secret settings aex passed out of the environment (so they do not
// reach processes the plugin starts) into an in-memory credential store.
func openPluginCredentials() {
	Credentials = secrets.Memory("secrets passed by aex", nil)
	credVars = map[string]string{}
	for _, s := range AuthSettings {
		if s.Credential == "" {
			continue
		}
		v, ok := os.LookupEnv(pluginSecretPrefix + s.Name)
		os.Unsetenv(pluginSecretPrefix + s.Name)
		if ok && v != "" {
			credVars[s.Name] = v
			Credentials.Set(s.Credential, v)
		}
	}
}
