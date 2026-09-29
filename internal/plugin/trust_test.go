package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"aex/internal/secrets"
	"aex/internal/settings"
)

func TestState(t *testing.T) {
	dir := t.TempDir()
	settings.Credentials, _ = secrets.Open("file", dir, filepath.Join(dir, "credentials.json"))
	path := filepath.Join(dir, "p.exe")
	if err := os.WriteFile(path, []byte("one"), 0o755); err != nil {
		t.Fatal(err)
	}
	check := func(want State) {
		t.Helper()
		o, err := openPlugin(path)
		if err != nil {
			t.Fatal(err)
		}
		defer o.close()
		if got, err := state(path, o.hash); got != want || err != nil {
			t.Errorf("state = %v, %v; want %v", got, err, want)
		}
	}
	check(NotApproved)

	o, err := openPlugin(path)
	if err != nil {
		t.Fatal(err)
	}
	// sha256("one")
	if want := "7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed"; o.hash != want {
		t.Errorf("hash = %s, want %s", o.hash, want)
	}
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(path, []byte("two"), 0o755); err == nil {
			t.Error("file could be written while open")
		}
		if err := os.Rename(path, path+".old"); err == nil {
			t.Error("file could be renamed while open")
		}
	}
	settings.Credentials.Set(hashKey(path), o.hash)
	o.close()
	check(Safe)

	if err := os.WriteFile(path, []byte("onf"), 0o755); err != nil {
		t.Fatal(err)
	}
	check(Changed)
	if err := forget(path); err != nil {
		t.Fatal(err)
	}
	check(NotApproved)
}

// The file stays locked from hashing until the plugin starts, so starting it must still work.
func TestStartWhileOpen(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	o, err := openPlugin(exe)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	if err := o.start(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
