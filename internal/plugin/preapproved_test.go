package plugin

import (
	"path/filepath"
	"strings"
	"testing"

	"aex/internal/secrets"
	"aex/internal/settings"
)

func TestPreApproved(t *testing.T) {
	dir := t.TempDir()
	settings.Credentials, _ = secrets.Open("file", dir, filepath.Join(dir, "credentials.json"))
	defer func(v string) { preApproved = v }(preApproved)
	// sha256("one"), sha256("two")
	one := "7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed"
	two := "3fc4ccfe745870e2c0d99f71f30ff0656c8dedd41cc1d7d3d376b0dbe685e2f3"
	path := filepath.Join(dir, "p.exe")

	preApproved = ""
	if PreApproved(one) || PreApproved("") {
		t.Error("pre-approved with none built in")
	}
	if s, _ := pluginState(path, one); s != NotApproved {
		t.Errorf("state = %v, want %v", s, NotApproved)
	}

	preApproved = one + "," + two
	if !PreApproved(one) || !PreApproved(two) || PreApproved(one[:10]) || PreApproved("") {
		t.Error("PreApproved wrong")
	}
	if s, _ := pluginState(path, two); s != Safe {
		t.Errorf("state = %v, want %v", s, Safe)
	}
	// A safe hash saved for another file does not matter.
	settings.Credentials.Set(hashKey(path), "00")
	if s, _ := pluginState(path, one); s != Safe {
		t.Errorf("state = %v, want %v", s, Safe)
	}
	if s, _ := state(path, one); s != Changed {
		t.Errorf("custom tool state = %v, want %v", s, Changed)
	}
}

func TestPreApprovedList(t *testing.T) {
	defer func(v string) { preApproved = v }(preApproved)
	preApproved = ""
	if l := PreApprovedList(); l != nil {
		t.Errorf("list = %v, want none", l)
	}
	one := "7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed"
	preApproved = strings.ToUpper(one) + ","
	if l := PreApprovedList(); len(l) != 1 || l[0].Hash != one {
		t.Errorf("list = %v, want %s", l, one)
	}
}
