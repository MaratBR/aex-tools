// Package plugin runs tools that live outside the aex exe: separate executables in the plugins
// folder next to it. The host side (List, Discover) finds them and turns each into a tool.Tool; the
// plugin side (Main) is what a plugin's main calls.
//
// Protocol: aex runs "<plugin> --aex-describe", which prints {"summary": "...", "access": [...]}
// as JSON, to list it with the tools (the tool name is the file name without .exe). A plugin that is a
// group of tools (see tool.Tool) also lists them: "tools": [{"name": "...", "summary": "..."}],
// each one maybe a group with its own "tools"; aex runs one as "<plugin> <sub-tool> [args]". Access
// is the plugin's, for all its tools. Running the tool
// describes it again, makes sure its access is granted (access.go), then runs it with the tool's
// args, in the same console, with the environment from settings.PluginEnv. No plugin runs, not
// even for --aex-describe, until the user approves its file (trust.go).
package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/ui"
)

const (
	describeFlag    = "--aex-describe"
	describeTimeout = 3 * time.Second
)

type description struct {
	Summary string    `json:"summary"`
	Access  []Access  `json:"access,omitempty"`
	Tools   []SubTool `json:"tools,omitempty"`
}

// SubTool is a tool of a plugin that is a group.
type SubTool struct {
	Name    string    `json:"name"`
	Summary string    `json:"summary"`
	Tools   []SubTool `json:"tools,omitempty"`
}

func describeTools(tools []tool.Tool) []SubTool {
	var d []SubTool
	for _, t := range tools {
		d = append(d, SubTool{t.Name, t.Summary, describeTools(t.Sub)})
	}
	return d
}

// Dir is the plugins folder: "plugins" next to the running exe.
func Dir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Join(filepath.Dir(exe), "plugins"), nil
}

// Info is one plugin in the plugins folder.
type Info struct {
	Name     string // tool name: the file name without .exe
	Path     string
	Size     int64
	Modified time.Time
	Hash     string // SHA-256 of the file's contents now
	State    State
	// Summary, Access and Tools are what the plugin describes itself as, only for a safe plugin
	// (else empty); DescribeErr says why that failed.
	Summary     string
	Access      []Access
	Tools       []SubTool
	DescribeErr error
}

// FileInfo is the file name, size and modified time, read without running anything.
func (p Info) FileInfo() string {
	return fmt.Sprintf("%s (%s)", filepath.Base(p.Path), sizeAndTime(p.Size, p.Modified))
}

func sizeAndTime(size int64, modified time.Time) string {
	return fmt.Sprintf("%.1f MB, modified %s", float64(size)/(1<<20), modified.Format("2006-01-02 15:04"))
}

// List lists the plugins in Dir, sorted by name, running --aex-describe on the safe ones only.
// A missing folder means no plugins; a file that cannot be read is skipped with a warning.
func List() ([]Info, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []Info
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, e := range entries {
		name, ok := pluginName(e)
		if !ok {
			continue
		}
		wg.Go(func() {
			p, err := inspect(name, filepath.Join(dir, e.Name()))
			if err != nil {
				ui.Warn("plugins: skipped %s: %v", e.Name(), err)
				return
			}
			mu.Lock()
			found = append(found, p)
			mu.Unlock()
		})
	}
	wg.Wait()
	slices.SortFunc(found, func(a, b Info) int { return strings.Compare(a.Name, b.Name) })
	return found, nil
}

// pluginName is the tool name for a plugins folder entry, if it is a plugin executable.
func pluginName(e os.DirEntry) (string, bool) {
	if !e.Type().IsRegular() {
		return "", false
	}
	if runtime.GOOS == "windows" {
		name, ok := strings.CutSuffix(e.Name(), ".exe")
		return name, ok && name != ""
	}
	info, err := e.Info()
	return e.Name(), err == nil && info.Mode()&0o111 != 0
}

// inspect reads the plugin's file info and hash, and describes it if it is safe.
func inspect(name, path string) (Info, error) {
	o, err := openPlugin(path)
	if err != nil {
		return Info{}, err
	}
	defer o.close()
	stat, err := o.f.Stat()
	if err != nil {
		return Info{}, err
	}
	p := Info{Name: name, Path: path, Size: stat.Size(), Modified: stat.ModTime(), Hash: o.hash}
	if p.State, err = state(path, o.hash); err != nil {
		return Info{}, err
	}
	if p.State == Safe {
		var d description
		d, p.DescribeErr = describe(o, path)
		p.Summary, p.Access, p.Tools = d.Summary, d.Access, d.Tools
	}
	return p, nil
}

// Describe runs --aex-describe on the plugin, asking for approval first unless it is safe.
func Describe(p Info) (string, error) {
	o, err := approve(p.Name, p.Path)
	if err != nil {
		return "", err
	}
	defer o.close()
	d, err := describe(o, p.Path)
	return d.Summary, err
}

// Delete deletes the plugin's file and forgets its safe hash.
func Delete(p Info) error {
	if err := os.Remove(p.Path); err != nil {
		return err
	}
	return forget(p.Path)
}

func describe(o *openFile, path string) (description, error) {
	ctx, cancel := context.WithTimeout(context.Background(), describeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, describeFlag)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := o.start(cmd)
	if err == nil {
		err = cmd.Wait()
	}
	if err != nil {
		return description{}, fmt.Errorf("%s failed: %w", describeFlag, err)
	}
	var d description
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		return description{}, fmt.Errorf("%s printed no valid JSON: %w", describeFlag, err)
	}
	return d, nil
}

// Discover is the plugins as tools. Safe ones show their summary, the others only their file.
// Plugins named like a tool in taken, or safe ones that fail to describe themselves, are skipped
// with a warning.
func Discover(taken []tool.Tool) []tool.Tool {
	plugins, err := List()
	if err != nil {
		ui.Warn("plugins: %v", err)
		return nil
	}
	var tools []tool.Tool
	for _, p := range plugins {
		if slices.ContainsFunc(taken, func(t tool.Tool) bool { return t.Name == p.Name }) {
			ui.Warn("plugins: skipped %s, a built-in tool has that name", p.Path)
			continue
		}
		summary := p.Summary
		switch {
		case p.State != Safe:
			summary = fmt.Sprintf("%s · %v, run it to review", p.FileInfo(), p.State)
		case p.DescribeErr != nil:
			ui.Warn("plugins: skipped %s: %v", p.Path, p.DescribeErr)
			continue
		}
		tools = append(tools, tool.Tool{Name: p.Name, Summary: summary, Run: runner(p.Name, p.Path, nil), Sub: subTools(p, nil, p.Tools)})
	}
	return tools
}

// subTools are the tools of a plugin that is a group, each run as "<plugin> <names...> [args]".
func subTools(p Info, names []string, tools []SubTool) []tool.Tool {
	var subs []tool.Tool
	for _, t := range tools {
		path := append(slices.Clip(names), t.Name)
		subs = append(subs, tool.Tool{Name: t.Name, Summary: t.Summary, Run: runner(p.Name, p.Path, path), Sub: subTools(p, path, t.Tools)})
	}
	return subs
}

// pickSub is sub, then the tools picked down to one that is not a group, when sub names a group
// of tools (the plugin itself for none). A name it does not know is left for the plugin to reject.
func pickSub(name string, sub []string, tools []SubTool) ([]string, error) {
	for _, n := range sub {
		i := slices.IndexFunc(tools, func(t SubTool) bool { return t.Name == n })
		if i < 0 {
			return sub, nil
		}
		tools = tools[i].Tools
	}
	if len(tools) == 0 {
		return sub, nil
	}
	var picked []string
	g := tool.Tool{Name: strings.Join(append([]string{name}, sub...), " "), Sub: pickTools(tools, func(path []string) { picked = path })}
	if err := g.Exec(nil); err != nil {
		return nil, err
	}
	return append(slices.Clip(sub), picked...), nil
}

// pickTools are tools as tool.Tools that only report the path to the one run.
func pickTools(tools []SubTool, picked func(path []string)) []tool.Tool {
	var ts []tool.Tool
	for _, t := range tools {
		ts = append(ts, tool.Tool{
			Name: t.Name, Summary: t.Summary,
			Run: func([]string) error { picked([]string{t.Name}); return nil },
			Sub: pickTools(t.Tools, func(path []string) { picked(append([]string{t.Name}, path...)) }),
		})
	}
	return ts
}

// ExitError is a plugin that exited non-zero. It printed its own error.
type ExitError struct {
	Name string
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("%s exited with code %d", e.Name, e.Code) }

// runner runs the plugin with the sub-tool names (none for the plugin itself) then args, asking for
// approval first unless it is safe, and for the access it asks for unless granted.
func runner(name, path string, sub []string) func(args []string) error {
	return func(args []string) error {
		o, err := approve(name, path)
		if err != nil {
			return err
		}
		approved := o.hash
		d, err := describe(o, path)
		if err != nil {
			return err
		}
		// A plugin approved just now was listed as a plain tool, its tools unknown: when it is a group,
		// ask for one here, where questions can be answered (the plugin cannot in the window).
		if len(args) == 0 {
			if sub, err = pickSub(name, sub, d.Tools); err != nil {
				return err
			}
		}
		pass, err := grant(name, path, approved, d.Access)
		if err != nil {
			return err
		}
		env, err := settings.PluginEnv(ui.Plain, pass)
		if err != nil {
			return err
		}
		// describe let go of the file; what runs must still be what was approved.
		if o, err = openPlugin(path); err != nil {
			return err
		}
		defer o.close()
		if o.hash != approved {
			return fmt.Errorf("plugin %s changed while starting, run it again", name)
		}
		cmd := exec.Command(path, append(slices.Clip(sub), args...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Env = env
		err = o.start(cmd)
		if err == nil {
			err = cmd.Wait()
		}
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return &ExitError{name, exit.ExitCode()}
		}
		return err
	}
}

// Main runs t (a tool, or a group of them) as a plugin that needs access (see Access): answers
// --aex-describe, else sets up like aex does and runs t with the command-line args, exiting 1 on
// error.
func Main(t tool.Tool, access ...Access) {
	if len(os.Args) == 2 && os.Args[1] == describeFlag {
		d := description{Summary: t.Summary, Access: access, Tools: describeTools(t.Sub)}
		if err := json.NewEncoder(os.Stdout).Encode(d); err != nil {
			os.Exit(1)
		}
		return
	}
	ui.Setup()
	dataDir, args, err := settings.TakeGlobalArgs(os.Args[1:])
	if err == nil {
		err = settings.InitPlugin(dataDir)
	}
	if err == nil {
		err = t.Exec(args)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", ui.Err.Bold(ui.Err.Red("✖ error:")), err)
		os.Exit(1)
	}
}
