package about

import (
	"os/exec"
	"strings"
	"testing"
)

// about.json must match the source: a dependency, license or version changed without running go
// generate fails here (and so does one aex cannot include).
func TestAboutJSONUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	out, err := exec.Command("go", "run", "./gen", "-check").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestRead(t *testing.T) {
	l, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	if l.License != "Apache-2.0" || !strings.Contains(l.Text, "Apache License") || l.Version == "" || l.Notice == "" {
		t.Errorf("license %q, version %q", l.License, l.Version)
	}
	for _, c := range l.ThirdParty {
		if c.License == "" || c.Version == "" && c.Name == "Go standard library" {
			t.Errorf("%s: license %q, version %q", c.Name, c.License, c.Version)
		}
	}
}
