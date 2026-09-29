package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOf(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=T", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, s string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	outside := filepath.Join(t.TempDir(), "x.ps1")
	os.WriteFile(outside, nil, 0o644)
	if i, err := Of(outside); i != nil || err != nil {
		t.Errorf("outside a repo: %+v, %v", i, err)
	}

	run("init", "-q", "-b", "main")
	run("remote", "add", "origin", "https://user:secret@example.com/r.git")
	path := write("s.ps1", "one\n")
	i, err := Of(path)
	if err != nil {
		t.Fatal(err)
	}
	if i.Head != "" || i.Status != "??" || i.Last != nil || i.Remote != "https://example.com/r.git" || i.File != "s.ps1" {
		t.Errorf("untracked: %+v", i)
	}

	run("add", "s.ps1")
	run("commit", "-q", "-m", "first")
	if i, err = Of(path); err != nil || i.Pending() || i.Branch != "main" || i.Last == nil || i.Last.Subject != "first" || i.Last.Short != i.Head {
		t.Errorf("committed: %+v, %v", i, err)
	}

	write("s.ps1", "one\ntwo\nthree\n")
	if i, err = Of(path); err != nil || i.Status != " M" || i.Added != 2 || i.Deleted != 0 || i.Change() != "changes not committed (+2 −0 lines)" {
		t.Errorf("modified: %+v, %v", i, err)
	}
	run("add", "s.ps1")
	if i, _ = Of(path); i.Change() != "changes staged, not committed (+2 −0 lines)" {
		t.Errorf("staged: %q", i.Change())
	}
}
