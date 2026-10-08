package proc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunning(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	if ok, err := Running(exe); err != nil || !ok {
		t.Errorf("own exe: %v %v", ok, err)
	}
	if ok, err := Running(filepath.Join(t.TempDir(), "no-such-program.exe")); err != nil || ok {
		t.Errorf("missing exe: %v %v", ok, err)
	}
}
