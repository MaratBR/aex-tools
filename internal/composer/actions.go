package composer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"aex/internal/proc"
	"aex/internal/tool"
	"aex/internal/ui"
)

// ToolAction runs an aex tool, as typed on the command line.
type ToolAction struct {
	// Tool is the tool's name, after its group's for one in a group: "quota", "cm-release pull-all".
	Tool string `json:"tool"`
	// Args are its args, as typed after it.
	Args string `json:"args,omitempty"`
}

func (a *ToolAction) Kind() string { return "tool" }

func (a *ToolAction) Describe() string {
	return strings.TrimSpace("aex " + a.Tool + " " + a.Args)
}

func (a *ToolAction) Check(c *Checker) {
	if strings.TrimSpace(a.Tool) == "" {
		c.Error("tool", "pick a tool")
		return
	}
	words := strings.Fields(a.Tool)
	if self := c.self != "" && (len(words) == 2 && words[0] == Group && strings.EqualFold(words[1], c.self) ||
		len(words) == 1 && strings.EqualFold(words[0], c.self)); self {
		c.Error("tool", "a composed tool cannot run itself")
		return
	}
	if _, t := resolve(a.Tool); t == nil {
		c.Warn("tool", "there is no tool %s now", a.Tool)
	}
}

func (a *ToolAction) Run(r *Run) error {
	path, t := resolve(a.Tool)
	if t == nil {
		return fmt.Errorf("there is no tool %s", a.Tool)
	}
	args := SplitArgs(a.Args)
	// A composed tool runs here, so one running itself (through others) is caught.
	if name := composedName(path); name != "" && !(len(args) == 1 && args[0] == "--help") {
		if len(args) > 0 {
			return errors.New("composed tools take no arguments")
		}
		entries, err := Load()
		if err != nil {
			return err
		}
		e := Find(entries, name)
		if e == nil {
			return fmt.Errorf("there is no composed tool %s", name)
		}
		if e.Err != nil {
			return fmt.Errorf("composed tool %s cannot be read: %w", name, e.Err)
		}
		return execute(r.Ctx, e.Composed, r.stack)
	}
	return t.Exec(args)
}

// resolve finds the tool line names: its path in the tool list and the tool, nil when there is
// none. A name alone also finds a tool in a group of custom or composed tools ("aex <name>"), when
// only one tool in the groups has it.
func resolve(line string) ([]string, *tool.Tool) {
	words := strings.Fields(line)
	if len(words) == 0 {
		return nil, nil
	}
	list := Tools()
	var path []string
	var t *tool.Tool
	for _, w := range words {
		i := slices.IndexFunc(list, func(t tool.Tool) bool { return t.Name == w })
		if i < 0 {
			t = nil
			break
		}
		t = &list[i]
		path = append(path, w)
		list = t.Sub
	}
	if t != nil {
		return path, t
	}
	if len(words) != 1 {
		return nil, nil
	}
	var hitPath []string
	var hit *tool.Tool
	all := Tools()
	for g := range all {
		for i := range all[g].Sub {
			if s := &all[g].Sub[i]; s.Name == words[0] && !s.IsGroup() {
				if hit != nil {
					return nil, nil
				}
				hitPath, hit = []string{all[g].Name, s.Name}, s
			}
		}
	}
	return hitPath, hit
}

// composedName is the composed tool a tool path runs, "" when it is not one.
func composedName(path []string) string {
	if len(path) == 2 && path[0] == Group {
		return path[1]
	}
	return ""
}

// RunAction runs a program.
type RunAction struct {
	// Path is the program: a path, or a name found on PATH.
	Path string `json:"path"`
	// Args are its args, as on a command line.
	Args string `json:"args,omitempty"`
	// Dir is the folder it runs in; empty: aex's.
	Dir string `json:"dir,omitempty"`
	// Detach starts it and goes on without waiting for it (for a program that stays open). Else
	// the step waits for it to exit and fails when its exit code is not 0.
	Detach bool `json:"detach,omitempty"`
	// SkipRunning does not start it when it is running already (a process runs that file), and
	// the step works.
	SkipRunning bool `json:"skipRunning,omitempty"`
}

func (a *RunAction) Kind() string { return "run" }

func (a *RunAction) Describe() string {
	s := "Run " + filepath.Base(expand(a.Path))
	if a.Args != "" {
		s += " " + a.Args
	}
	return s
}

func (a *RunAction) Check(c *Checker) {
	if strings.TrimSpace(a.Path) == "" {
		c.Error("path", "pick the program to run")
	} else if _, err := program(a.Path); err != nil {
		c.Warn("path", "%v", err)
	}
	if a.Dir != "" {
		if info, err := os.Stat(expand(a.Dir)); err != nil || !info.IsDir() {
			c.Warn("dir", "there is no folder %s", expand(a.Dir))
		}
	}
}

// program is the file the program at path (or a name on PATH) runs from.
func program(path string) (string, error) {
	p := expand(path)
	if strings.ContainsAny(p, `/\`) {
		info, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("there is no file %s", p)
		}
		if info.IsDir() && !isMacApp(p) {
			return "", fmt.Errorf("%s is a folder, not a program", p)
		}
		return p, nil
	}
	found, err := exec.LookPath(p)
	if err != nil {
		return "", fmt.Errorf("there is no program %s on PATH", p)
	}
	return found, nil
}

func (a *RunAction) Run(r *Run) error {
	exe, err := program(a.Path)
	if err != nil {
		return err
	}
	if a.SkipRunning {
		running, err := proc.Running(exe)
		if err != nil {
			r.Printf("%s", ui.Out.Yellow(fmt.Sprintf("Could not tell whether %s is running, starting it: %v", filepath.Base(exe), err)))
		} else if running {
			r.Printf("%s is running already, not started", filepath.Base(exe))
			return nil
		}
	}
	// A macOS app opens through open, which does not wait for it.
	if isMacApp(exe) {
		args := []string{"-a", exe}
		if a.Args != "" {
			args = append(append(args, "--args"), SplitArgs(a.Args)...)
		}
		if err := exec.CommandContext(r.Ctx, "open", args...).Run(); err != nil {
			return fmt.Errorf("opening %s: %w", filepath.Base(exe), err)
		}
		r.Printf("Opened %s", filepath.Base(exe))
		return nil
	}
	if a.Detach {
		// Not tied to the step: a program left open outlives it, and aex.
		cmd := exec.Command(exe, SplitArgs(a.Args)...)
		cmd.Dir = expand(a.Dir)
		if err := cmd.Start(); err != nil {
			return err
		}
		r.Printf("Started %s (process %d)", filepath.Base(exe), cmd.Process.Pid)
		return cmd.Process.Release()
	}
	cmd := exec.CommandContext(r.Ctx, exe, SplitArgs(a.Args)...)
	cmd.Dir = expand(a.Dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, r.Output(os.Stdout), r.Output(os.Stderr)
	proc.HideConsoleIfNone(cmd)
	err = cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		if r.Ctx.Err() != nil {
			return fmt.Errorf("stopped: %w", r.Ctx.Err())
		}
		return fmt.Errorf("%s exited with code %d", filepath.Base(exe), exit.ExitCode())
	}
	return err
}

// CheckAction runs its steps again and again until they work: a try works when none of its steps
// fails, or a return in it returns worked; a return that returns failed fails the try.
type CheckAction struct {
	// Do are the steps tried.
	Do Steps `json:"do,omitempty"`
	// Every is how many seconds to wait between tries; 0: 5.
	Every int `json:"every,omitempty"`
	// Timeout is after how many seconds to give up; nil: 300, 0: never.
	Timeout *int `json:"timeout,omitempty"`
	// Attempts is how many tries at most; 0: no limit.
	Attempts int `json:"attempts,omitempty"`
}

const defaultEvery, defaultTimeout = 5, 300

// second is the unit of Every and Timeout (shorter in tests).
var second = time.Second

func (a *CheckAction) Kind() string { return "check" }

func (a *CheckAction) Describe() string {
	switch len(a.Do) {
	case 0:
		return "Until it works"
	case 1:
		return "Until it works: " + a.Do[0].Title()
	}
	return fmt.Sprintf("Until it works: %s and %d more", a.Do[0].Title(), len(a.Do)-1)
}

func (a *CheckAction) every() time.Duration {
	if a.Every <= 0 {
		return defaultEvery * second
	}
	return time.Duration(a.Every) * second
}

func (a *CheckAction) timeout() time.Duration {
	if a.Timeout == nil {
		return defaultTimeout * second
	}
	return time.Duration(*a.Timeout) * second
}

func (a *CheckAction) Check(c *Checker) {
	if a.Every < 0 {
		c.Error("every", "seconds between tries cannot be negative")
	}
	if a.Timeout != nil && *a.Timeout < 0 {
		c.Error("timeout", "seconds to give up after cannot be negative (0: never)")
	}
	if a.Attempts < 0 {
		c.Error("attempts", "tries cannot be negative (0: no limit)")
	}
	if len(a.Do) == 0 {
		c.Error("do", "add the steps to try")
	}
	c.checkSteps(a.Do, c.step+".do.")
}

// try runs the steps once: nil when they worked.
func (a *CheckAction) try(r *Run) error {
	err := runSteps(r, a.Do)
	if ret, ok := errors.AsType[*Returned](err); ok {
		if !ret.Fail {
			return nil
		}
		if ret.Message != "" {
			return errors.New(ret.Message)
		}
		return errors.New("returned failed")
	}
	return err
}

func (a *CheckAction) Run(r *Run) error {
	if len(a.Do) == 0 {
		return errors.New("no steps to try")
	}
	started := time.Now()
	every, timeout := a.every(), a.timeout()
	in := r.nested()
	for try := 1; ; try++ {
		err := a.try(in)
		if err == nil {
			if try > 1 {
				r.Printf("Worked on try %d", try)
			}
			return nil
		}
		if r.Ctx.Err() != nil {
			return err
		}
		if a.Attempts > 0 && try >= a.Attempts {
			return fmt.Errorf("still failing after %d tries: %w", try, err)
		}
		if timeout > 0 && time.Since(started)+every > timeout {
			return fmt.Errorf("still failing after %s: %w", time.Since(started).Round(time.Second), err)
		}
		r.Printf("Try %d: %v; again in %s", try, err, every)
		if !sleep(r.Ctx, every) {
			return fmt.Errorf("stopped: %w", err)
		}
	}
}

var winVar = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_()]*)%`)

// winVars expands %NAME% on Windows; a variable not set stays as written.
func winVars(s string) string {
	if runtime.GOOS != "windows" {
		return s
	}
	return winVar.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := os.LookupEnv(m[1 : len(m)-1]); ok {
			return v
		}
		return m
	})
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isMacApp reports whether path is a macOS app (a folder ending in .app, on macOS).
func isMacApp(path string) bool {
	return runtime.GOOS == "darwin" && strings.HasSuffix(path, ".app")
}
