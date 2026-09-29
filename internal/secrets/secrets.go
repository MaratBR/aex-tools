// Package secrets stores credentials (AEXT session, Jira API token, Google login) in the OS credential store:
// Windows Credential Manager, macOS Keychain, or the Secret Service on Linux (GNOME Keyring,
// KWallet). Where none is available (e.g. a headless Linux box) it falls back to a file readable
// only by the current user.
package secrets

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/zalando/go-keyring"
)

// Keys of the stored credentials.
const (
	AEXTSession        = "aext-session"
	JiraToken          = "jira-token"
	GoogleLogin        = "google-login"
	GoogleClientSecret = "google-client-secret"
)

// Store keeps credentials by key. Get returns "" for a missing key; Delete of a missing key is not an error.
type Store interface {
	// Name says where credentials are kept, for display.
	Name() string
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// Open returns the OS credential store, or a file store at fallbackFile when there is none or kind
// is "file" (AEX_CREDENTIAL_STORE). Credentials are scoped to scope (the data folder), so separate
// data folders keep separate logins. Reason says why the file store is used, "" otherwise.
func Open(kind, scope, fallbackFile string) (store Store, reason string) {
	file := &fileStore{path: fallbackFile}
	switch kind {
	case "file":
		return file, "AEX_CREDENTIAL_STORE=file"
	case "", "os":
	default:
		return file, fmt.Sprintf("unknown AEX_CREDENTIAL_STORE=%q, using file", kind)
	}
	k := &keyringStore{service: "aex:" + scope}
	// A lookup tells whether the store works: missing is fine, anything else means no store.
	if _, err := k.Get("probe"); err != nil {
		return file, "no OS credential store: " + err.Error()
	}
	return k, ""
}

type keyringStore struct{ service string }

func (k *keyringStore) Name() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows Credential Manager"
	case "darwin":
		return "macOS Keychain"
	default:
		return "Secret Service (keyring)"
	}
}

func (k *keyringStore) Get(key string) (string, error) {
	v, err := keyring.Get(k.service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: read %s: %w", k.Name(), key, err)
	}
	return v, nil
}

func (k *keyringStore) Set(key, value string) error {
	if err := keyring.Set(k.service, key, value); err != nil {
		return fmt.Errorf("%s: save %s: %w", k.Name(), key, err)
	}
	return nil
}

func (k *keyringStore) Delete(key string) error {
	if err := keyring.Delete(k.service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("%s: delete %s: %w", k.Name(), key, err)
	}
	return nil
}
