// Package plugin runs tools that live outside the aex exe: separate executables in the plugins
// folder next to it. The host side (List, Discover) finds them and turns each into a tool.Tool; the
// plugin side (Main) is what a plugin's main calls.
//
// Protocol: aex runs "<plugin> --aex-describe", which prints {"summary": "..."} as JSON, to list it
// in the menu (the tool name is the file name without .exe). Running the tool runs the plugin with
// the tool's args, in the same console, with the environment from settings.PluginEnv. No plugin
// runs, not even for --aex-describe, until the user approves its file (trust.go).
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
	Summary string `json:"summary"`
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
	// Summary is what the plugin describes itself as, only for a safe plugin (else "");
	// DescribeErr says why that failed.
	Summary     string
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
		p.Summary, p.DescribeErr = describe(o, path)
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
	return describe(o, p.Path)
}

// Delete deletes the plugin's file and forgets its safe hash.
func Delete(p Info) error {
	if err := os.Remove(p.Path); err != nil {
		return err
	}
	return forget(p.Path)
}

func describe(o *openFile, path string) (string, error) {
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
		return "", fmt.Errorf("%s failed: %w", describeFlag, err)
	}
	var d description
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		return "", fmt.Errorf("%s printed no valid JSON: %w", describeFlag, err)
	}
	return d.Summary, nil
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
		tools = append(tools, tool.Tool{Name: p.Name, Summary: summary, Run: runner(p.Name, p.Path)})
	}
	return tools
}

// ExitError is a plugin that exited non-zero. It printed its own error.
type ExitError struct {
	Name string
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("%s exited with code %d", e.Name, e.Code) }

// runner runs the plugin, asking for approval first unless it is safe.
func runner(name, path string) func(args []string) error {
	return func(args []string) error {
		o, err := approve(name, path)
		if err != nil {
			return err
		}
		defer o.close()
		cmd := exec.Command(path, args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Env = settings.PluginEnv(ui.Plain)
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

// Main runs t as a plugin: answers --aex-describe, else sets up like aex does and runs t with the
// command-line args, exiting 1 on error.
func Main(t tool.Tool) {
	if len(os.Args) == 2 && os.Args[1] == describeFlag {
		if err := json.NewEncoder(os.Stdout).Encode(description{Summary: t.Summary}); err != nil {
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
		err = t.Run(args)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", ui.Err.Bold(ui.Err.Red("✖ error:")), err)
		os.Exit(1)
	}
}
