package custom

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aex/internal/plugin"
	"aex/internal/secrets"
	"aex/internal/settings"
)

func TestRun(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil && runtime.GOOS == "windows" {
		t.Skip("no PowerShell")
	} else if _, err := exec.LookPath("pwsh"); err != nil && runtime.GOOS != "windows" {
		t.Skip("no PowerShell")
	}
	dir := t.TempDir()
	settings.CustomToolsFile = filepath.Join(dir, "custom-tools.json")
	settings.Credentials = secrets.Memory("test", nil)
	src, err := os.ReadFile(filepath.Join("..", "adapter", "powershell", "testdata", "sample.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "Say Hi.ps1")
	if err := os.WriteFile(script, src, 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Add(script, "")
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "Say-Hi" || e.Adapter != "powershell" {
		t.Errorf("added %+v", e)
	}
	if _, err := Add(script, "other"); err == nil {
		t.Error("added the same script twice")
	}
	if err := CheckName("say-hi"); err == nil {
		t.Error("name taken ignoring case was accepted")
	}

	tools := Discover(nil)
	if len(tools) != 1 || tools[0].Summary != "Greets someone." || !strings.Contains(tools[0].Warn, "not approved") {
		t.Fatalf("tools = %+v", tools)
	}
	run := tools[0].Run
	if err := run([]string{"-Name", "x"}); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Errorf("unapproved run: %v", err)
	}

	info := Inspect(e, true)
	if info.Git != nil || info.State != plugin.NotApproved || info.Err != nil {
		t.Errorf("info %+v", info)
	}
	key := filepath.Clean(script)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	settings.Credentials.Set("plugin-safe-sha256:"+key, info.Hash)

	out := capture(t, func() {
		if err := run([]string{"-Name", "bob smith", "-Loud", "-Items", "x,y"}); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(out, "Name=bob smith|Word=hi|Count=1|Loud=True|Polite=True|Items=2:x+y") {
		t.Errorf("output %q", out)
	}
	if err := run([]string{"-Word", "hi"}); err == nil || err.Error() != "missing -Name (see --help)" {
		t.Errorf("missing required: %v", err)
	}
	capture(t, func() {
		err := run([]string{"fail"})
		if exit, ok := errors.AsType[*plugin.ExitError](err); !ok || exit.Code != 3 {
			t.Errorf("exit: %v", err)
		}
	})

	if err := os.WriteFile(script, append(src, '#'), 0o644); err != nil {
		t.Fatal(err)
	}
	if i := Inspect(e, false); i.State != plugin.Changed {
		t.Errorf("state after change %v", i.State)
	}
	if _, err := Remove(e.Name); err != nil {
		t.Fatal(err)
	}
	if entries, _ := Load(); len(entries) != 0 {
		t.Errorf("left %+v", entries)
	}
}

// capture is what f prints to stdout.
func capture(t *testing.T, f func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = file, file
	f()
	os.Stdout, os.Stderr = stdout, stderr
	file.Close()
	b, _ := os.ReadFile(path)
	return string(b)
}
