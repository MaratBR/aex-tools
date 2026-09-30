package custom

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aex/internal/adapter"
	"aex/internal/secrets"
	"aex/internal/settings"
	"aex/internal/tool"
)

// never is an adapter that cannot work here.
type never struct{}

func (never) Name() string                                 { return "never" }
func (never) Supported() (bool, string)                    { return false, "it never works" }
func (never) Handles(path string) bool                     { return filepath.Ext(path) == ".nvr" }
func (never) FileTypes() (string, []string)                { return "Never scripts", []string{"*.nvr"} }
func (never) Describe(string) (adapter.Description, error) { return adapter.Description{}, nil }
func (never) Command(string, adapter.Description, []adapter.Arg, []string, adapter.Session) (*exec.Cmd, error) {
	return nil, nil
}

func TestUnsupportedAdapter(t *testing.T) {
	saved := allAdapters
	allAdapters = append([]adapter.Adapter{never{}}, saved...)
	t.Cleanup(func() { allAdapters = saved })
	dir := t.TempDir()
	settings.CustomToolsFile = filepath.Join(dir, "custom-tools.json")
	settings.Credentials = secrets.Memory("test", nil)

	for _, a := range Adapters() {
		if a.Name() == "never" {
			t.Error("Adapters() lists an unsupported adapter")
		}
	}
	if u := Unsupported(); len(u) != 1 || u[0].Adapter.Name() != "never" || u[0].Why != "it never works" {
		t.Errorf("Unsupported() = %+v", u)
	}
	if strings.Contains(AdapterNames(), "never") || !IsAdapter("never") {
		t.Errorf("AdapterNames() = %q, IsAdapter = %v", AdapterNames(), IsAdapter("never"))
	}

	script := filepath.Join(dir, "x.nvr")
	if err := os.WriteFile(script, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(script, "", ""); err == nil || !strings.Contains(err.Error(), "it never works") {
		t.Errorf("Add = %v, want why the adapter is not supported", err)
	}
	if _, err := Add(script, "", "never"); err == nil || !strings.Contains(err.Error(), "it never works") {
		t.Errorf("Add --adapter never = %v, want why it is not supported", err)
	}

	// One added elsewhere: not listed as a tool, and says why it cannot run.
	if err := save([]Entry{{Name: "x", Path: script, Adapter: "never"}}); err != nil {
		t.Fatal(err)
	}
	if groups := Discover([]tool.Tool{}); len(groups) != 0 {
		t.Errorf("Discover = %+v, want no groups", groups)
	}
	if i := Inspect(Entry{Name: "x", Path: script, Adapter: "never"}, false); i.Err == nil || !strings.Contains(i.Err.Error(), "not supported here") {
		t.Errorf("Inspect err = %v", i.Err)
	}
}
