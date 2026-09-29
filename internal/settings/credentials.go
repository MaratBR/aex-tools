package settings

import (
	"os"
	"path/filepath"

	"aex/internal/secrets"
	"aex/internal/ui"
)

var (
	// Credentials holds the AEXT session and secret settings (JIRA_TOKEN): the OS credential store,
	// else the file CredentialsFile.
	Credentials secrets.Store
	// CredentialsNote says why the file fallback is used, "" when it is not.
	CredentialsNote string
	CredentialsFile string

	credVars = map[string]string{} // secret settings held in Credentials, guarded by mu
)

// credentialKey is the Credentials key a setting is kept under, "" for settings kept in app settings.
func credentialKey(name string) string {
	for _, s := range AuthSettings {
		if s.Name == name {
			return s.Credential
		}
	}
	return ""
}

// openCredentials opens the credential store (AEX_CREDENTIAL_STORE=file forces the file) and
// reads the secret settings from it.
func openCredentials() {
	CredentialsFile = filepath.Join(DataDir, "credentials.json")
	Credentials, CredentialsNote = secrets.Open(os.Getenv("AEX_CREDENTIAL_STORE"), DataDir, CredentialsFile)
	credVars = map[string]string{}
	for _, s := range AuthSettings {
		if s.Credential == "" {
			continue
		}
		v, err := Credentials.Get(s.Credential)
		if err != nil {
			ui.Warn("%v", err)
		}
		if v != "" {
			credVars[s.Name] = v
		}
	}
}

func credentialLayer() Layer {
	mu.RLock()
	defer mu.RUnlock()
	vars := make(map[string]string, len(credVars))
	for k, v := range credVars {
		vars[k] = v
	}
	return Layer{Credentials.Name(), vars}
}

// saveCredential stores (value non-nil) or deletes a secret setting in Credentials.
func saveCredential(name, key string, value *string) error {
	var err error
	if value == nil {
		err = Credentials.Delete(key)
	} else {
		err = Credentials.Set(key, *value)
	}
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if value == nil {
		delete(credVars, name)
	} else {
		credVars[name] = *value
	}
	return nil
}

// StoredIn says where SaveAppSettings keeps a setting: the app settings file or the credential store.
func StoredIn(name string) string {
	if credentialKey(name) != "" {
		return Credentials.Name()
	}
	return ConfigEnvFile
}
