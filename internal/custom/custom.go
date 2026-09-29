// Package custom runs custom tools: scripts added by path (custom-tools add), each run by the tool
// adapter (internal/adapter) for its kind of file, e.g. PowerShell for .ps1.
//
// A custom tool is approved like a plugin (internal/plugin): the SHA-256 of the script is saved as
// its safe hash the first time it runs, and every run hashes it again and asks again when it
// changed. The script is held open from hashing until it exits, so on Windows it cannot change in
// between (what it dot-sources or imports is not covered). Its parameters are read by the adapter
// without running it, so they show before it is approved; aex parses the tool's args against them
// and asks for the ones left out. When the script is in a git repo, the approval and every run show
// the repo, its commit, the last commit of the script and its changes not committed.
package custom

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/adapter"
	"aex/internal/adapter/powershell"
	"aex/internal/gitinfo"
	"aex/internal/plugin"
	"aex/internal/pty"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/ui"
)

// Adapters are the tool adapters; a script is run by the first that handles it.
var Adapters = []adapter.Adapter{powershell.Adapter}

func adapterFor(path string) adapter.Adapter {
	for _, a := range Adapters {
		if a.Handles(path) {
			return a
		}
	}
	return nil
}

func adapterNamed(name string) (adapter.Adapter, error) {
	for _, a := range Adapters {
		if a.Name() == name {
			return a, nil
		}
	}
	return nil, fmt.Errorf("no tool adapter %q (adapters: %s)", name, adapterNames())
}

func adapterNames() string {
	names := make([]string, len(Adapters))
	for i, a := range Adapters {
		names[i] = a.Name()
	}
	return strings.Join(names, ", ")
}

// Info is a custom tool as it is now.
type Info struct {
	Entry
	Size     int64
	Modified time.Time
	Hash     string // SHA-256 of the script now
	State    plugin.State
	Desc     adapter.Description
	// Err is why the script cannot be read or described (missing file, parse errors, ...).
	Err    error
	Git    *gitinfo.Info // nil when not in a git repo, or not asked for
	GitErr error
}

// FileInfo is the file's name, size and modified time.
func (i Info) FileInfo() string {
	return fmt.Sprintf("%s (%.1f KB, modified %s)", filepath.Base(i.Path), float64(i.Size)/1024, i.Modified.Format("2006-01-02 15:04"))
}

// descriptions caches what adapters described, by adapter, path and hash: describing starts a
// process (e.g. PowerShell), and the tool list is read again after every run.
var (
	descMu       sync.Mutex
	descriptions = map[string]adapter.Description{}
)

// describe describes the script at path, locked with hash, with a.
func describe(a adapter.Adapter, path, hash string) (adapter.Description, error) {
	key := a.Name() + "\x00" + path + "\x00" + hash
	descMu.Lock()
	d, ok := descriptions[key]
	descMu.Unlock()
	if ok {
		return d, nil
	}
	d, err := a.Describe(path)
	if err != nil {
		return d, err
	}
	descMu.Lock()
	descriptions[key] = d
	descMu.Unlock()
	return d, nil
}

// Inspect reads the custom tool's file, hash and state, describes it and (withGit) its git info.
func Inspect(e Entry, withGit bool) Info {
	i := Info{Entry: e}
	a, err := adapterNamed(e.Adapter)
	if err != nil {
		i.Err = err
		return i
	}
	l, err := plugin.Lock(e.Path)
	if err != nil {
		i.Err = err
		return i
	}
	defer l.Close()
	i.Hash = l.Hash()
	if stat, err := l.Stat(); err == nil {
		i.Size, i.Modified = stat.Size(), stat.ModTime()
	}
	if i.State, i.Err = l.State(e.Path); i.Err != nil {
		return i
	}
	i.Desc, i.Err = describe(a, e.Path, i.Hash)
	if withGit {
		i.Git, i.GitErr = gitinfo.Of(e.Path)
	}
	return i
}

// List inspects every custom tool, in parallel.
func List(withGit bool) ([]Info, error) {
	entries, err := Load()
	if err != nil {
		return nil, err
	}
	infos := make([]Info, len(entries))
	var wg sync.WaitGroup
	for n, e := range entries {
		wg.Go(func() { infos[n] = Inspect(e, withGit) })
	}
	wg.Wait()
	return infos, nil
}

// Discover is the custom tools as tools. Ones named like a tool in taken are skipped with a warning.
func Discover(taken []tool.Tool) []tool.Tool {
	infos, err := List(false)
	if err != nil {
		ui.Warn("custom tools: %v", err)
		return nil
	}
	var tools []tool.Tool
	for _, i := range infos {
		if slices.ContainsFunc(taken, func(t tool.Tool) bool { return strings.EqualFold(t.Name, i.Name) }) {
			ui.Warn("custom tools: skipped %s, another tool has that name", i.Name)
			continue
		}
		summary, warn := i.Desc.Summary, ""
		if summary == "" {
			summary = fmt.Sprintf("%s script %s", i.Adapter, filepath.Base(i.Path))
		}
		switch {
		case i.Err != nil:
			warn = "Custom tool cannot run: " + i.Err.Error()
			summary = filepath.Base(i.Path) + " · " + i.Err.Error()
		case i.State != plugin.Safe:
			warn = fmt.Sprintf("Custom tool %v: running it asks to approve it first", i.State)
		}
		tools = append(tools, tool.Tool{Name: i.Name, Summary: summary, Run: runner(i.Entry), Warn: warn})
	}
	return tools
}

// Details are the lines about a custom tool shown when approving it and by custom-tools: adapter,
// parameters and git info.
func Details(i Info) [][2]string {
	runner := i.Adapter
	if i.Desc.Runner != "" {
		runner += " (" + i.Desc.Runner + ")"
	}
	lines := [][2]string{{"Adapter", runner}}
	if len(i.Desc.Params) > 0 {
		names := make([]string, len(i.Desc.Params))
		for n, p := range i.Desc.Params {
			names[n] = "-" + p.Name
		}
		lines = append(lines, [2]string{"Params", strings.Join(names, " ")})
	}
	switch {
	case i.GitErr != nil:
		lines = append(lines, [2]string{"Git", i.GitErr.Error()})
	case i.Git == nil:
		lines = append(lines, [2]string{"Git", "not in a git repo"})
	default:
		lines = append(lines, i.Git.Lines()...)
	}
	return lines
}

// runner runs the custom tool: approval unless safe, then its args bound to its parameters, asking
// for those left out, then the script, all with the file locked.
func runner(e Entry) func(args []string) error {
	return func(args []string) error {
		a, err := adapterNamed(e.Adapter)
		if err != nil {
			return err
		}
		l, err := plugin.Lock(e.Path)
		if err != nil {
			return err
		}
		defer l.Close()
		i := Info{Entry: e, Hash: l.Hash()}
		if i.Desc, err = describe(a, e.Path, i.Hash); err != nil {
			return fmt.Errorf("custom tool %s: %w", e.Name, err)
		}
		if len(args) == 1 && slices.Contains([]string{"--help", "-h", "-?"}, args[0]) {
			fmt.Println(adapter.Usage(e.Name, i.Desc))
			return nil
		}
		i.Git, i.GitErr = gitinfo.Of(e.Path)
		if err := l.Approve(e.Name, e.Path, Details(i)); err != nil {
			return err
		}
		bound, rest, err := adapter.Bind(i.Desc, args)
		if err != nil {
			return err
		}
		if bound, err = ask(i.Desc, bound, len(args) == 0); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, ui.Err.Dim(runLine(i)))
		env, err := settings.PluginEnv(ui.Plain, nil)
		if err != nil {
			return err
		}
		// In the window the script has no console: an interactive one gets a terminal there.
		s := adapter.Session{Console: ui.Remote == nil}
		host, canShow := ui.Remote.(ui.TerminalHost)
		s.Terminal = !s.Console && i.Desc.Interactive && canShow && pty.Supported
		cmd, err := a.Command(e.Path, i.Desc, bound, rest, s)
		if err != nil {
			return err
		}
		cmd.Env = env
		code := 0
		if s.Terminal {
			code, err = runInTerminal(host, cmd)
		} else {
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			err = cmd.Run()
			if exit, ok := errors.AsType[*exec.ExitError](err); ok {
				code, err = exit.ExitCode(), nil
			}
		}
		if err == nil && code != 0 {
			return &plugin.ExitError{Name: e.Name, Code: code}
		}
		return err
	}
}

// Terminal size to start with: the window fits the columns to its width right away.
const termCols, termRows = 120, 24

// runInTerminal runs cmd in a pseudo-console that host shows, until it exits; code is its exit code.
func runInTerminal(host ui.TerminalHost, cmd *exec.Cmd) (code int, err error) {
	p, err := pty.Start(cmd, termCols, termRows)
	if err != nil {
		return 0, err
	}
	defer p.Close()
	screen := host.OpenTerminal(p, func(cols, rows int) { p.Resize(cols, rows) })
	shown := make(chan struct{})
	go func() {
		io.Copy(screen, p)
		close(shown)
	}()
	code, err = p.Wait()
	// Wait closed the pseudo-console, which flushes the rest of the screen and ends it.
	<-shown
	screen.Close()
	return code, err
}

// runLine says what runs: the script, its runner and where it stands in git.
func runLine(i Info) string {
	s := "▸ " + i.Path
	if i.Desc.Runner != "" {
		s += " · " + filepath.Base(i.Desc.Runner)
	}
	if i.Git != nil {
		s += " · " + i.Git.Short()
	}
	return s
}
