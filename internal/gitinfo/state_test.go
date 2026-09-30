package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	if s := State(dir); s.Error == "" || !s.Pending() {
		t.Errorf("not a repo: %+v", s)
	}
	cmd := exec.Command("git", "-C", dir, "init", "-q", "-b", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if s := State(dir); s.Error != "" || s.Branch != "main" || s.Changed != 0 || s.Upstream || s.Pending() {
		t.Errorf("empty repo: %+v", s)
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	if s := State(dir); s.Changed != 2 || !s.Pending() {
		t.Errorf("two untracked files: %+v", s)
	}
}
