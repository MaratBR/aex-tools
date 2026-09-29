package powershell

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"aex/internal/adapter"
)

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{"": "''", "a b": "'a b'", "it's": "'it''s'", "$x `y": "'$x `y'", "a’b": "'a’’b'"} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestPathHint(t *testing.T) {
	for _, c := range []struct {
		name, typ string
		kind      adapter.Kind
		want      string
	}{
		{"Target", "System.IO.FileInfo", adapter.String, adapter.PathFile},
		{"Target", "IO.DirectoryInfo", adapter.String, adapter.PathFolder},
		{"ConfigPath", "string", adapter.String, adapter.PathFile},
		{"OutFile", "String", adapter.String, adapter.PathFile},
		{"RepoDir", "string", adapter.String, adapter.PathFolder},
		{"OutputFolder", "string", adapter.String, adapter.PathFolder},
		{"Name", "string", adapter.String, ""},
		{"Paths", "string[]", adapter.List, ""},
		{"MaxPath", "int", adapter.Int, ""},
	} {
		if got := pathHint(adapter.Param{Name: c.name, Type: c.typ, Kind: c.kind}); got != c.want {
			t.Errorf("pathHint(%s %s) = %q, want %q", c.typ, c.name, got, c.want)
		}
	}
}

func TestDescribeAndRun(t *testing.T) {
	if runtime.GOOS != "windows" && pwsh() == "" {
		t.Skip("no PowerShell")
	}
	path, _ := filepath.Abs(filepath.Join("testdata", "sample.ps1"))
	d, err := Adapter.Describe(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Summary != "Greets someone." || d.Rest {
		t.Errorf("summary %q, rest %v", d.Summary, d.Rest)
	}
	var names []string
	for _, p := range d.Params {
		names = append(names, p.Name+":"+string(p.Kind))
	}
	if want := []string{"Name:string", "Word:string", "Count:int", "Loud:switch", "Polite:bool", "Items:list", "Ratio:number"}; !slices.Equal(names, want) {
		t.Errorf("params %v, want %v", names, want)
	}
	if name := d.Params[0]; !name.Required || name.Help != "Who to greet.\nfirst name" || !slices.Equal(name.Aliases, []string{"n"}) {
		t.Errorf("Name = %+v", name)
	}
	if w := d.Params[1]; w.Required || w.Default != "hi" || !slices.Equal(w.Choices, []string{"hi", "hello"}) {
		t.Errorf("Word = %+v", w)
	}
	if p := d.Params[4]; p.Default != "$true" {
		t.Errorf("Polite default %q", p.Default)
	}
	if d.Params[6].Required {
		t.Error("Ratio should not be required")
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
	out, err := run("-n", "O'Brien \"$x\"", "--word=HELLO", "-Loud", "-Polite:false", "-Items", "a, b,c", "-Rat", "0.5")
	if want := "Name=O'Brien \"$x\"|Word=hello|Count=1|Loud=True|Polite=False|Items=3:a+b+c|Ratio=0.5"; out != want || err != nil {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	out, err = run("Юля", "hi", "2")
	if want := "Name=Юля|Word=hi|Count=2|Loud=False|Polite=True|Items=0:|Ratio=0"; out != want || err != nil {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	_, err = run("fail")
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 3 {
		t.Errorf("exit = %v, want code 3", err)
	}
}
