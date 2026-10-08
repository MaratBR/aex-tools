// Package ide finds the IDEs installed on this device and the projects they opened lately, and
// opens a project in one, for the Composer's open-ide action (internal/composer). Each family of
// IDEs is a Provider: Visual Studio, Visual Studio Code, JetBrains.
package ide

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/appicon"
)

// IDE is an IDE found on this device.
type IDE struct {
	// ID names it in a composed tool: "rider", "vscode", "vs2022".
	ID   string `json:"id"`
	Name string `json:"name"`
	// Version, when known: "2024.3.1".
	Version string `json:"version,omitempty"`
	// Provider is its provider's name.
	Provider string `json:"provider"`
	// Path is what starts it: its exe, a launcher script, or on macOS its app.
	Path string `json:"path"`
	// data is the provider's own: where the IDE keeps its settings.
	data string
}

// Project is a project an IDE opened.
type Project struct {
	Path string `json:"path"`
	Name string `json:"name"`
	// Kind is solution, project, workspace or folder.
	Kind string `json:"kind"`
	// Opened is when it was last opened, when known.
	Opened time.Time `json:"opened,omitzero"`
}

// Provider finds a family of IDEs.
type Provider interface {
	// Name names the family: "JetBrains".
	Name() string
	// Known gives the name of the IDE id names, whether installed or not; ok is false when the
	// provider has no such IDE.
	Known(id string) (name string, ok bool)
	// Installed lists its IDEs found on this device, one per ID.
	Installed() []IDE
	// Icon is ide's icon as a PNG data: URL, "" when it has none.
	Icon(ide IDE) string
	// Projects lists the projects ide opened lately that are still there, newest first; nil when
	// it cannot tell.
	Projects(ide IDE) []Project
	// Command is the command that opens project in ide.
	Command(ide IDE, project string) *exec.Cmd
}

// Providers are the IDE providers, in the order their IDEs are listed.
var Providers = []Provider{VisualStudio, VSCode, JetBrains}

func providerOf(i IDE) Provider {
	for _, p := range Providers {
		if p.Name() == i.Provider {
			return p
		}
	}
	return nil
}

// cacheFor is how long what was found is reused: finding IDEs starts programs (vswhere) and reads
// folders, and the editor asks on every change.
const cacheFor = 30 * time.Second

var (
	mu         sync.Mutex
	found      []IDE
	foundAt    time.Time
	projects   = map[string][]Project{}
	projectsAt = map[string]time.Time{}
)

// Installed lists the IDEs found on this device, every provider's.
func Installed() []IDE {
	mu.Lock()
	defer mu.Unlock()
	if found != nil && time.Since(foundAt) < cacheFor {
		return found
	}
	list := []IDE{}
	var wg sync.WaitGroup
	each := make([][]IDE, len(Providers))
	for n, p := range Providers {
		wg.Go(func() { each[n] = p.Installed() })
	}
	wg.Wait()
	for _, l := range each {
		list = append(list, l...)
	}
	found, foundAt = list, time.Now()
	return list
}

// Find is the installed IDE id names; "vs" is the newest Visual Studio.
func Find(id string) (IDE, bool) {
	list := Installed()
	if i := slices.IndexFunc(list, func(i IDE) bool { return i.ID == id }); i >= 0 {
		return list[i], true
	}
	if id == "vs" {
		if i := slices.IndexFunc(list, func(i IDE) bool { return i.Provider == VisualStudio.Name() }); i >= 0 {
			return list[i], true
		}
	}
	return IDE{}, false
}

// Known gives the name of the IDE id names, installed or not; ok is false when no provider has it.
func Known(id string) (name string, ok bool) {
	for _, p := range Providers {
		if name, ok := p.Known(id); ok {
			return name, true
		}
	}
	return "", false
}

// Icon is i's icon as a PNG data: URL, "" when it has none.
func Icon(i IDE) string {
	if p := providerOf(i); p != nil {
		return p.Icon(i)
	}
	return ""
}

// Projects lists the projects i opened lately, newest first, at most max (0: all).
func Projects(i IDE, max int) []Project {
	mu.Lock()
	list, ok := projects[i.ID]
	fresh := ok && time.Since(projectsAt[i.ID]) < cacheFor
	mu.Unlock()
	if !fresh {
		if p := providerOf(i); p != nil {
			list = p.Projects(i)
		}
		slices.SortStableFunc(list, func(a, b Project) int { return b.Opened.Compare(a.Opened) })
		list = dedupe(list)
		mu.Lock()
		projects[i.ID], projectsAt[i.ID] = list, time.Now()
		mu.Unlock()
	}
	if max > 0 && len(list) > max {
		list = list[:max]
	}
	return list
}

// Open opens project in the IDE id names, without waiting for it.
func Open(id, project string) (IDE, error) {
	i, ok := Find(id)
	if !ok {
		if name, known := Known(id); known {
			return i, fmt.Errorf("%s is not found on this device", name)
		}
		return i, fmt.Errorf("no IDE %q", id)
	}
	cmd := providerOf(i).Command(i, project)
	cmd.Dir = filepath.Dir(project)
	if err := cmd.Start(); err != nil {
		return i, err
	}
	return i, cmd.Process.Release()
}

// exeIcon is the icon of the exe or app that starts i, "" for a script.
func exeIcon(i IDE) string {
	switch strings.ToLower(filepath.Ext(i.Path)) {
	case ".exe", ".app":
		return appicon.DataURL(i.Path)
	}
	return ""
}

// command opens project with the exe, or on macOS with the app, at path.
func command(path, project string) *exec.Cmd {
	if strings.HasSuffix(path, ".app") {
		return exec.Command("open", "-a", path, project)
	}
	return exec.Command(path, project)
}

// project is the project at path, opened at t, nil when it is not there any more.
func project(path string, t time.Time) *Project {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	p := &Project{Path: path, Name: filepath.Base(path), Opened: t, Kind: "folder"}
	if !info.IsDir() {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".sln", ".slnx", ".slnf":
			p.Kind = "solution"
		case ".code-workspace":
			p.Kind, p.Name = "workspace", strings.TrimSuffix(p.Name, filepath.Ext(p.Name))
		default:
			p.Kind = "project"
		}
	}
	return p
}

// dedupe drops projects listed again (by path, ignoring case on Windows), keeping the first.
func dedupe(list []Project) []Project {
	seen := map[string]bool{}
	var out []Project
	for _, p := range list {
		key := filepath.Clean(p.Path)
		if filepath.Separator == '\\' {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// newest is the last of paths as they sort (folders named by version sort oldest first), "" for none.
func newest(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	slices.SortFunc(paths, compareVersions)
	return paths[len(paths)-1]
}

// compareVersions compares two names by the numbers in them, in order ("2024.10" after "2024.9").
func compareVersions(a, b string) int {
	na, nb := numbers(a), numbers(b)
	for i := 0; i < len(na) && i < len(nb); i++ {
		if na[i] != nb[i] {
			return na[i] - nb[i]
		}
	}
	if len(na) != len(nb) {
		return len(na) - len(nb)
	}
	return strings.Compare(a, b)
}

func numbers(s string) []int {
	var out []int
	n, in := 0, false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n, in = n*10+int(c-'0'), true
		} else if in {
			out, n, in = append(out, n), 0, false
		}
	}
	if in {
		out = append(out, n)
	}
	return out
}

// userConfigDir is where apps keep their settings: %APPDATA%, ~/Library/Application Support or
// ~/.config.
func userConfigDir() string {
	dir, _ := os.UserConfigDir()
	return dir
}
