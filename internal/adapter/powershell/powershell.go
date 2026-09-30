// Package powershell is the tool adapter for PowerShell scripts (.ps1).
//
// Parameters are read from the script's param() block with the PowerShell parser (describe.ps1),
// which runs nothing of the script: types, [Parameter(Mandatory)], HelpMessage, [ValidateSet],
// [Alias], defaults, and comment-based help (.SYNOPSIS, .PARAMETER). The script runs with
// -Command "& '<path>' -Name 'value' ...": values are quoted literals, so switches and bools take
// $false and lists are arrays, which -File cannot pass.
//
// Scripts are Interactive (Read-Host can be anywhere): in the window they run in a pseudo-console
// the window shows as a terminal, so everything PowerShell asks works as in a console.
//
// It runs with Windows PowerShell (powershell.exe) on Windows, else PowerShell 7 (pwsh); also pwsh
// when the script says #Requires -PSEdition Core, or when only pwsh can parse it.
package powershell

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"aex/internal/adapter"
	"aex/internal/proc"
)

//go:embed describe.ps1
var describeScript string

const describeTimeout = 20 * time.Second

// Adapter is the PowerShell tool adapter.
var Adapter adapter.Adapter = powershell{}

type powershell struct{}

func (powershell) Name() string { return "powershell" }

// Supported: PowerShell 7 (pwsh) can be installed on every OS aex runs on.
func (powershell) Supported() (bool, string) { return true, "" }

func (powershell) Handles(path string) bool { return strings.EqualFold(filepath.Ext(path), ".ps1") }

func (powershell) FileTypes() (string, []string) { return "PowerShell scripts", []string{"*.ps1"} }

// pathHint guesses whether a parameter takes a file or folder path: by its type ([IO.FileInfo],
// [IO.DirectoryInfo]), else, for a string, by its name (…File, …Path; …Folder, …Dir, …Directory).
func pathHint(p adapter.Param) string {
	t := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(p.Type, "System."), "system."))
	name := strings.ToLower(p.Name)
	switch {
	case t == "io.fileinfo":
		return adapter.PathFile
	case t == "io.directoryinfo":
		return adapter.PathFolder
	case t != "string" || p.Kind != adapter.String:
		return ""
	case strings.HasSuffix(name, "folder") || strings.HasSuffix(name, "dir") || strings.HasSuffix(name, "directory"):
		return adapter.PathFolder
	case strings.HasSuffix(name, "file") || strings.HasSuffix(name, "path"):
		return adapter.PathFile
	}
	return ""
}

// described is what describe.ps1 prints.
type described struct {
	Summary  string          `json:"summary"`
	Params   []adapter.Param `json:"params"`
	HasParam bool            `json:"hasParam"`
	Editions []string        `json:"editions"`
	Errors   []string        `json:"errors"`
	Notes    []string        `json:"notes"`
}

func (powershell) Describe(path string) (adapter.Description, error) {
	hosts := hostsFor()
	if len(hosts) == 0 {
		return adapter.Description{}, errors.New("neither powershell.exe nor pwsh is installed")
	}
	var failed error
	for _, host := range hosts {
		d, err := describeWith(host, path)
		if err != nil {
			failed = err
			continue
		}
		if len(d.Errors) > 0 {
			failed = fmt.Errorf("%s cannot parse it: %s", host, strings.Join(d.Errors, "; "))
			continue
		}
		// A script for PowerShell 7 only is run by pwsh, whichever parsed it.
		if slices.ContainsFunc(d.Editions, func(e string) bool { return strings.EqualFold(e, "Core") }) {
			if host = pwsh(); host == "" {
				return adapter.Description{}, errors.New("the script needs PowerShell 7 (#Requires -PSEdition Core): pwsh is not installed")
			}
		}
		for i := range d.Params {
			if d.Params[i].Type == "" {
				d.Params[i].Type = "object"
			}
			d.Params[i].Hint = pathHint(d.Params[i])
		}
		return adapter.Description{Summary: d.Summary, Params: d.Params, Rest: !d.HasParam, Runner: host, Notes: d.Notes,
			// Read-Host may be anywhere in the script or what it calls: there is no telling it will not ask.
			Interactive: true}, nil
	}
	return adapter.Description{}, failed
}

// hostsFor is the PowerShells to try, in order: Windows PowerShell first on Windows, then pwsh.
func hostsFor() []string {
	var hosts []string
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("powershell.exe"); err == nil {
			hosts = append(hosts, p)
		}
	}
	if p := pwsh(); p != "" {
		hosts = append(hosts, p)
	}
	return hosts
}

func pwsh() string {
	p, err := exec.LookPath("pwsh")
	if err != nil {
		return ""
	}
	return p
}

func describeWith(host, path string) (described, error) {
	ctx, cancel := context.WithTimeout(context.Background(), describeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, host, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", describeScript)
	cmd.Env = append(os.Environ(), "AEX_SCRIPT_PATH="+path)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	proc.HideConsole(cmd)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return described{}, fmt.Errorf("reading its parameters with %s failed: %s", filepath.Base(host), msg)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(out.String()))
	if err != nil {
		return described{}, fmt.Errorf("reading its parameters with %s: unexpected output: %w", filepath.Base(host), err)
	}
	var d described
	if err := json.Unmarshal(raw, &d); err != nil {
		return described{}, fmt.Errorf("reading its parameters with %s: %w", filepath.Base(host), err)
	}
	return d, nil
}

func (powershell) Command(path string, d adapter.Description, args []adapter.Arg, rest []string, s adapter.Session) (*exec.Cmd, error) {
	if d.Runner == "" {
		return nil, errors.New("no PowerShell to run it")
	}
	cmd := exec.Command(d.Runner, append(flags(s), "-Command", invocation(path, args, rest, s))...)
	if piped(s) {
		proc.HideConsole(cmd)
	}
	return cmd, nil
}

// piped reports whether the script's output goes to a pipe, with no input: neither aex's console
// nor a terminal of its own.
func piped(s adapter.Session) bool { return !s.Console && !s.Terminal }

// flags are PowerShell's own options. Piped, -NonInteractive makes a question fail instead of
// waiting on an input that never comes.
func flags(s adapter.Session) []string {
	f := []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass"}
	if piped(s) {
		f = append(f, "-NonInteractive")
	}
	return f
}

// invocation is the -Command text that runs the script with args and rest, exiting with the
// script's exit code. -Command rather than -EncodedCommand, which writes errors to stderr as
// CLIXML.
func invocation(path string, args []adapter.Arg, rest []string, s adapter.Session) string {
	var b strings.Builder
	if piped(s) {
		// Output goes to a pipe, in the console code page unless told otherwise. (A pseudo-console
		// gives UTF-8 whatever the code page.)
		b.WriteString("[Console]::OutputEncoding = [Text.Encoding]::UTF8; ")
	}
	b.WriteString("$global:LASTEXITCODE = 0; & " + quote(path))
	for _, a := range args {
		b.WriteString(" " + argument(a))
	}
	for _, r := range rest {
		b.WriteString(" " + quote(r))
	}
	b.WriteString("; exit $LASTEXITCODE")
	return b.String()
}

// argument is a as PowerShell: "-Name:$true" for switches and bools, "-Name 'a','b'" for lists,
// else "-Name 'value'".
func argument(a adapter.Arg) string {
	name := "-" + a.Param.Name
	switch a.Param.Kind {
	case adapter.Bool, adapter.Switch:
		if a.On {
			return name + ":$true"
		}
		return name + ":$false"
	case adapter.List:
		items := make([]string, len(a.List))
		for i, v := range a.List {
			items[i] = quote(v)
		}
		return name + " @(" + strings.Join(items, ",") + ")"
	}
	return name + " " + quote(a.Text)
}

// quote is s as a PowerShell single-quoted string, where nothing is expanded. PowerShell also takes
// the typographic single quotes as quotes, so they are doubled too.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'', '‘', '’', '‚', '‛':
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}
