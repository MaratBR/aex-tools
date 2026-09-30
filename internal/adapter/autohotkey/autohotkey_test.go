package autohotkey

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aex/internal/adapter"
)

func TestRead(t *testing.T) {
	for _, c := range []struct {
		src  string
		want header
	}{
		{"#Requires AutoHotkey v2.0\n#SingleInstance\n\n; Does a thing.\n; Quickly.\n\n; not this\nx := 1\n", header{"Does a thing. Quickly.", 2, ""}},
		{"\uFEFF/*\n * Block summary\n * on two lines\n *\n * more\n */\nx := 1", header{"Block summary on two lines", 0, ""}},
		{"/* One line */\n", header{"One line", 0, ""}},
		{";@Ahk2Exe-SetDescription From the directive\n; Comment\n", header{"From the directive", 0, ""}},
		{";@Ahk2Exe-SetVersion 1.0\n; Comment\n", header{"Comment", 0, ""}},
		{"#Requires AutoHotkey >=2.1-alpha.3 64-bit\nx := 1\n; late comment", header{"", 2, "64"}},
		{"#Requires AutoHotkey v1.1.33\n", header{"", 1, ""}},
		{"#requires autohotkey 32-bit\n", header{"", 0, "32"}},
		{"MsgBox 'hi'\n; after code", header{}},
	} {
		if got := read(c.src); got != c.want {
			t.Errorf("read(%q) = %+v, want %+v", c.src, got, c.want)
		}
	}
}

func TestHandles(t *testing.T) {
	for path, want := range map[string]bool{`C:\a\b.ahk`: true, "x.AHK": true, "x.ah2": true, "x.ps1": false, "ahk": false} {
		if got := Adapter.Handles(path); got != want {
			t.Errorf("Handles(%q) = %v", path, got)
		}
	}
}

func TestDescribeAndRun(t *testing.T) {
	if find("") == "" {
		t.Skip("no AutoHotkey v2")
	}
	path, _ := filepath.Abs(filepath.Join("testdata", "sample.ahk"))
	d, err := Adapter.Describe(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Summary != "Echoes its args. One per line, then exits with code 3 when the first is \"fail\"." || !d.Rest || len(d.Params) > 0 {
		t.Errorf("described %+v", d)
	}
	run := func(args ...string) (string, error) {
		bound, rest, err := adapter.Bind(d, args)
		if err != nil {
			t.Fatal(err)
		}
		cmd, err := Adapter.Command(path, d, bound, rest, adapter.Session{})
		if err != nil {
			t.Fatal(err)
		}
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	out, err := run("a b", `q"uote`, "Юля")
	if want := `3:|a b|q"uote|Юля`; out != want || err != nil {
		t.Errorf("got %q, %v, want %q", out, err, want)
	}
	_, err = run("fail")
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 3 {
		t.Errorf("exit = %v, want code 3", err)
	}
}

func TestNotInstalled(t *testing.T) {
	if find("") != "" {
		t.Skip("AutoHotkey v2 is installed")
	}
	path, _ := filepath.Abs(filepath.Join("testdata", "sample.ahk"))
	if _, err := Adapter.Describe(path); err == nil || !strings.Contains(err.Error(), "winget install AutoHotkey.AutoHotkey") {
		t.Errorf("Describe = %v, want how to install it", err)
	}
}
