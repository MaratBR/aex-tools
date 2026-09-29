package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"aex/internal/secrets"
)

// SettingsToWipe lists what WipeSettings removes, for display.
func SettingsToWipe() []string {
	var secretNames []string
	for _, s := range AuthSettings {
		if s.Credential != "" {
			secretNames = append(secretNames, s.Name)
		}
	}
	return []string{
		"app settings " + ConfigEnvFile,
		fmt.Sprintf("secret settings (%s) in %s", strings.Join(secretNames, ", "), Credentials.Name()),
		"plugin settings " + PluginSettingsDir,
	}
}

// WipeSettings removes app settings, secret settings and plugin settings, then reloads settings as
// a restart would (app settings get their defaults again). Fallback sources (.env and the
// environment) are left alone.
func WipeSettings() error {
	err := wipeSettings()
	Reload()
	return err
}

func wipeSettings() error {
	var errs []error
	for _, s := range AuthSettings {
		if s.Credential != "" {
			errs = append(errs, Credentials.Delete(s.Credential))
		}
	}
	errs = append(errs, removeAll(ConfigEnvFile), removeAll(PluginSettingsDir))
	return errors.Join(errs...)
}

// DataToWipe lists what WipeAll removes besides the settings: the AEXT session and every entry of
// the data folder. It fails when the data folder holds the app itself, which WipeAll would delete.
func DataToWipe() ([]string, error) {
	entries, err := dataEntries()
	if err != nil {
		return nil, err
	}
	return append([]string{"AEXT session in " + Credentials.Name()}, entries...), nil
}

// WipeAll is WipeSettings plus the AEXT session, output files, plugin data and anything else in the
// data folder. Plugins and their approvals (kept in the credential store) are left alone.
func WipeAll() error {
	entries, err := dataEntries()
	if err != nil {
		return err
	}
	errs := []error{wipeSettings(), Credentials.Delete(secrets.AEXTSession)}
	for _, e := range entries {
		errs = append(errs, removeAll(e))
	}
	Reload()
	return errors.Join(errs...)
}

// dataEntries lists the data folder's entries WipeAll deletes: all but the credential file, which
// may hold plugin approvals (secrets deletes it once it is empty).
func dataEntries() ([]string, error) {
	if within(AppRoot, DataDir) {
		return nil, fmt.Errorf("the data folder %s holds the app (%s): not wiping it, delete its files by hand", DataDir, AppRoot)
	}
	if exe, err := os.Executable(); err == nil && within(exe, DataDir) {
		return nil, fmt.Errorf("the data folder %s holds the app (%s): not wiping it, delete its files by hand", DataDir, exe)
	}
	entries, err := os.ReadDir(DataDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	for _, e := range entries {
		path := filepath.Join(DataDir, e.Name())
		if !samePath(path, CredentialsFile) {
			list = append(list, path)
		}
	}
	slices.Sort(list)
	return list, nil
}

// Reload reads settings again, as a restart would.
func Reload() {
	openCredentials()
	load(embeddedEnv)
}

func removeAll(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
