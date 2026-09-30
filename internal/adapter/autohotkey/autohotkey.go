// Package autohotkey is the tool adapter for AutoHotkey v2 scripts (.ahk, .ah2).
//
// AutoHotkey scripts declare no parameters: they get their args as A_Args, so a custom tool passes
// them on as given (Description.Rest). The summary is read from the script without running it: its
// ;@Ahk2Exe-SetDescription directive, else the comment at its top. A script that says
// #Requires AutoHotkey v1 is refused.
//
// Scripts are not Interactive: AutoHotkey asks in windows of its own (InputBox, MsgBox, Gui), not in
// a console. What they write to "*" (FileAppend) goes to the run's output. A script that stays
// running (hotkeys, #Persistent, a Gui) keeps its run going until it exits.
//
// It runs with AutoHotkey64.exe (AutoHotkey32.exe for a script that #Requires 32-bit) from the v2
// folder of the AutoHotkey install, else the first found on PATH. Windows only (Supported). When none is found,
// Describe fails saying how to install it, which shows wherever the custom tool is listed or run.
package autohotkey

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"aex/internal/adapter"
)

// notInstalled says how to install AutoHotkey v2 when it is not found.
const notInstalled = "AutoHotkey v2 is not installed. Install it with `winget install AutoHotkey.AutoHotkey`, " +
	"or download it from https://www.autohotkey.com/download/ (v2), then try again. A portable copy works too: " +
	"put the folder with AutoHotkey64.exe on PATH"

// Adapter is the AutoHotkey v2 tool adapter.
var Adapter adapter.Adapter = autohotkey{}

type autohotkey struct{}

func (autohotkey) Name() string { return "autohotkey" }

func (autohotkey) Supported() (bool, string) {
	if runtime.GOOS != "windows" {
		return false, "AutoHotkey runs only on Windows"
	}
	return true, ""
}

func (autohotkey) Handles(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".ahk" || ext == ".ah2"
}

func (autohotkey) FileTypes() (string, []string) {
	return "AutoHotkey scripts", []string{"*.ahk", "*.ah2"}
}

func (autohotkey) Describe(path string) (adapter.Description, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return adapter.Description{}, err
	}
	h := read(string(src))
	if h.major != 0 && h.major != 2 {
		return adapter.Description{}, fmt.Errorf("it is an AutoHotkey v%d script (#Requires): only v2 scripts can run", h.major)
	}
	exe := find(h.bits)
	switch {
	case exe == "" && h.bits != "" && find("") != "":
		return adapter.Description{}, fmt.Errorf("the script needs %s-bit AutoHotkey (#Requires): AutoHotkey%s.exe was not found; "+
			"reinstall AutoHotkey v2 from https://www.autohotkey.com/download/ (it has both)", h.bits, h.bits)
	case exe == "":
		return adapter.Description{}, errors.New(notInstalled)
	}
	return adapter.Description{Summary: h.summary, Rest: true, Runner: exe,
		Notes: []string{"AutoHotkey scripts declare no parameters: args reach the script as A_Args, as given."}}, nil
}

func (autohotkey) Command(path string, d adapter.Description, _ []adapter.Arg, rest []string, _ adapter.Session) (*exec.Cmd, error) {
	if d.Runner == "" {
		return nil, errors.New("no AutoHotkey to run it")
	}
	// /ErrorStdOut: errors in the script as it loads go to stderr (the run) instead of a message box.
	return exec.Command(d.Runner, append([]string{"/ErrorStdOut=UTF-8", path}, rest...)...), nil
}

// header is what read finds in a script.
type header struct {
	summary string
	major   int    // the AutoHotkey version #Requires asks for, 0 when it does not say
	bits    string // "32" or "64" when #Requires asks for one, else ""
}

var (
	requiresRe    = regexp.MustCompile(`(?i)^#Requires\s+AutoHotkey\b(.*)`)
	versionRe     = regexp.MustCompile(`(?i)^[<>=]*\s*v?(\d+)(?:\.[\w.-]*)?(?:\s|$)`)
	bitsRe        = regexp.MustCompile(`\b(32|64)-bit\b`)
	descriptionRe = regexp.MustCompile(`(?i)^;@Ahk2Exe-SetDescription\s+(.+)`)
)

// read reads the script's #Requires and its summary: its ;@Ahk2Exe-SetDescription, else the first
// paragraph of the comment at its top (; lines or a /* */ block, before any code; blank lines and
// #directives before it are skipped).
func read(src string) header {
	var h header
	var comment []string
	inBlock, started, done := false, false, false
	for line := range strings.Lines(strings.TrimPrefix(src, string(rune(0xFEFF)))) {
		line = strings.TrimSpace(line)
		if m := requiresRe.FindStringSubmatch(line); m != nil {
			rest := strings.TrimSpace(m[1])
			if v := versionRe.FindStringSubmatch(rest); v != nil {
				fmt.Sscan(v[1], &h.major)
			}
			if b := bitsRe.FindStringSubmatch(rest); b != nil {
				h.bits = b[1]
			}
			continue
		}
		if m := descriptionRe.FindStringSubmatch(line); m != nil {
			h.summary = strings.TrimSpace(m[1])
			continue
		}
		if done {
			continue
		}
		switch {
		case inBlock:
			text, end := line, false
			if i := strings.Index(line, "*/"); i >= 0 {
				text, end = line[:i], true
			}
			if text = strings.TrimSpace(strings.TrimLeft(text, "*")); text != "" {
				comment = append(comment, text)
			} else if len(comment) > 0 {
				done = true
			}
			if end {
				inBlock, done = false, true
			}
		case strings.HasPrefix(line, "/*"):
			inBlock, started = true, true
			if rest := strings.TrimSpace(strings.TrimPrefix(line, "/*")); strings.HasSuffix(rest, "*/") {
				inBlock, done = false, true
				rest = strings.TrimSpace(strings.TrimSuffix(rest, "*/"))
				if rest != "" {
					comment = append(comment, rest)
				}
			} else if rest != "" {
				comment = append(comment, rest)
			}
		case strings.HasPrefix(line, ";"):
			started = true
			if strings.HasPrefix(line, ";@") {
				continue // an Ahk2Exe directive, not text
			}
			if text := strings.TrimSpace(strings.TrimLeft(line, ";")); text != "" {
				comment = append(comment, text)
			} else if len(comment) > 0 {
				done = true
			}
		case line == "":
			if started {
				done = true
			}
		case strings.HasPrefix(line, "#"):
			if started {
				done = true
			}
		default:
			done = true // code
		}
	}
	if h.summary == "" {
		h.summary = strings.Join(comment, " ")
	}
	return h
}

// names are the AutoHotkey v2 executables, the one wanted first: 64-bit unless the script asks for
// 32-bit.
func names(bits string) []string {
	if bits == "32" {
		return []string{"AutoHotkey32.exe"}
	}
	if bits == "64" {
		return []string{"AutoHotkey64.exe"}
	}
	return []string{"AutoHotkey64.exe", "AutoHotkey32.exe"}
}

// find is the AutoHotkey v2 executable to run a script with, or "": in the v2 folder of the
// AutoHotkey install (from the registry, else its default folders), else on PATH.
func find(bits string) string {
	dirs := installDirs()
	for _, d := range []string{os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if d != "" {
			dirs = append(dirs, filepath.Join(d, "AutoHotkey"), filepath.Join(d, "Programs", "AutoHotkey"))
		}
	}
	for _, n := range names(bits) {
		for _, d := range dirs {
			if p := filepath.Join(d, "v2", n); isFile(p) {
				return p
			}
		}
	}
	for _, n := range names(bits) {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
