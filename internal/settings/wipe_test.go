package settings

import (
	"os"
	"path/filepath"
	"testing"

	"aex/internal/secrets"
)

func TestWipe(t *testing.T) {
	t.Setenv("AEX_CREDENTIAL_STORE", "file")
	dir := t.TempDir()
	if err := Init(dir, ""); err != nil {
		t.Fatal(err)
	}
	token, hours := "secret", "6"
	if err := SaveAppSettings([]Change{{"JIRA_TOKEN", &token}, {"HOURS_PER_DAY", &hours}}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{secrets.AEXTSession, "plugin-safe-sha256:x"} {
		if err := Credentials.Set(key, "v"); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(PluginSettingsDir, "p.json"), filepath.Join(JiraExportDir, "a.csv"), filepath.Join(dir, "plugin-data", "x")} {
		os.MkdirAll(filepath.Dir(f), 0o755)
		os.WriteFile(f, []byte("x"), 0o600)
	}

	if err := WipeSettings(); err != nil {
		t.Fatal(err)
	}
	// .env files and the environment may still hold JIRA_TOKEN: only app settings are wiped.
	if InAppSettings("JIRA_TOKEN") || Get("HOURS_PER_DAY") != "8" {
		t.Error("WipeSettings kept JIRA_TOKEN or HOURS_PER_DAY in app settings")
	}
	if exists(PluginSettingsDir) || !exists(OutputDir) {
		t.Error("WipeSettings: plugin settings should be gone, output kept")
	}
	if v, _ := Credentials.Get(secrets.AEXTSession); v == "" {
		t.Error("WipeSettings removed the AEXT session")
	}

	if err := WipeAll(); err != nil {
		t.Fatal(err)
	}
	if exists(OutputDir) || exists(filepath.Join(dir, "plugin-data")) {
		t.Error("WipeAll kept output or plugin data")
	}
	if v, _ := Credentials.Get(secrets.AEXTSession); v != "" {
		t.Error("WipeAll kept the AEXT session")
	}
	if v, _ := Credentials.Get("plugin-safe-sha256:x"); v == "" {
		t.Error("WipeAll removed a plugin approval")
	}
	if Get("HOURS_PER_DAY") != "8" {
		t.Error("WipeAll did not restore defaults")
	}
}

func TestWipeAllRefusesAppFolder(t *testing.T) {
	t.Setenv("AEX_CREDENTIAL_STORE", "file")
	if err := Init(t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	DataDir = filepath.Dir(AppRoot) // only listed, never wiped
	if _, err := DataToWipe(); err == nil {
		t.Error("DataToWipe accepted a data folder holding the app")
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
