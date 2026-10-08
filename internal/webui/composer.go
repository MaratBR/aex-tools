package webui

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"aex/internal/appicon"
	"aex/internal/composer"
	"aex/internal/ide"
	"aex/internal/settings"
)

// The Composer page (frontend/composer.js): makes and edits composed tools (internal/composer),
// with blocks or as JSON.

// ComposedInfo is a composed tool kept, as the editor opens it.
type ComposedInfo struct {
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Steps   int    `json:"steps"`
	// JSON is the tool as kept, indented; Error why it cannot be read, when it cannot.
	JSON  string `json:"json"`
	Error string `json:"error,omitempty"`
}

// ComposerState is everything the Composer page shows.
type ComposerState struct {
	File  string          `json:"file"`
	Group string          `json:"group"` // composed tools run as "<group> <name>"
	Tools []ComposedInfo  `json:"tools"`
	Kinds []composer.Kind `json:"kinds"`
	IDEs  []IDEInfo       `json:"ides"` // the IDEs found on this device
	// Suggestions are the likely steps of each kind of action that has some, by kind.
	Suggestions map[string][]composer.Suggestion `json:"suggestions"`
	// Places are where the orient action puts windows.
	Places []composer.Place `json:"places"`
	Lines  []ShortcutTool   `json:"lines"` // the tools a tool step can run
	Error  string           `json:"error,omitempty"`
}

// IDEInfo is an IDE found on this device, with its icon and the projects it opened lately.
type IDEInfo struct {
	ide.IDE
	Icon     string        `json:"icon,omitempty"` // a data: URL
	Projects []ide.Project `json:"projects"`
}

// maxProjects is how many of an IDE's recent projects the editor offers.
const maxProjects = 40

// Composer gives the composed tools and what their steps can be.
func (a *App) Composer() ComposerState {
	s := ComposerState{File: settings.ComposedToolsFile, Group: composer.Group, Tools: []ComposedInfo{},
		Kinds: composer.Kinds, IDEs: []IDEInfo{}, Suggestions: composer.Suggestions(), Places: composer.Places}
	for _, i := range ide.Installed() {
		s.IDEs = append(s.IDEs, IDEInfo{IDE: i, Icon: ide.Icon(i), Projects: ide.Projects(i, maxProjects)})
	}
	lines, _ := toolsAPI(a, nil)
	s.Lines = lines.([]ShortcutTool)
	entries, err := composer.Load()
	if err != nil {
		s.Error = err.Error()
	}
	for _, e := range entries {
		info := ComposedInfo{Name: e.Name, Summary: e.Summary, Steps: len(e.Steps), JSON: string(e.Raw)}
		if b, err := composer.Marshal(e.Composed); err == nil && e.Err == nil {
			info.JSON = string(b)
		}
		if e.Err != nil {
			info.Error = e.Err.Error()
		}
		s.Tools = append(s.Tools, info)
	}
	return s
}

// ComposerResult is what checking or saving a composed tool found. When Error is set (its JSON
// cannot be read) or Problems has errors, it was not saved.
type ComposerResult struct {
	Error    string            `json:"error,omitempty"`
	Problems composer.Problems `json:"problems"`
	Saved    bool              `json:"saved,omitempty"`
	// Reloaded is false when it was saved while a tool runs: the tool list follows once it ends.
	Reloaded bool `json:"reloaded,omitempty"`
}

// ComposerCheck checks the composed tool text (JSON) as replacing the one called old ("" for new).
func (a *App) ComposerCheck(old, text string) ComposerResult {
	c, err := composer.Parse([]byte(text))
	if err != nil {
		return ComposerResult{Error: err.Error(), Problems: composer.Problems{}}
	}
	p, err := composer.Check(c, old)
	if err != nil {
		return ComposerResult{Error: err.Error(), Problems: composer.Problems{}}
	}
	return ComposerResult{Problems: append(composer.Problems{}, p...)}
}

// ComposerSave saves the composed tool text (JSON), replacing the one called old ("" adds it),
// unless it has errors.
func (a *App) ComposerSave(old, text string) (ComposerResult, error) {
	c, err := composer.Parse([]byte(text))
	if err != nil {
		return ComposerResult{Error: err.Error(), Problems: composer.Problems{}}, nil
	}
	p, err := composer.Put(old, c)
	if err != nil {
		return ComposerResult{}, err
	}
	r := ComposerResult{Problems: append(composer.Problems{}, p...), Saved: !p.HasErrors()}
	if r.Saved {
		r.Reloaded = a.reload()
	}
	return r, nil
}

// ComposerDelete removes the composed tool called name.
func (a *App) ComposerDelete(name string) (bool, error) {
	if err := composer.Remove(name); err != nil {
		return false, err
	}
	return a.reload(), nil
}

// reload reads the tool list again (Host.Reload), unless a tool runs: it is read again when it
// ends anyway. It reports whether it did.
func (a *App) reload() bool {
	if a.host.Reload == nil {
		return false
	}
	// No tool starts meanwhile: it would read the tool list as it changes.
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return false
	}
	a.running = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()
	a.host.Reload()
	return true
}

// ComposerWindows are the screens and the windows open now, for the orient action's editor; each
// window's icon (its program's) by program path.
type ComposerWindows struct {
	Screens []composer.Screen     `json:"screens"`
	Windows []composer.OpenWindow `json:"windows"`
	Icons   map[string]string     `json:"icons"`
}

// ComposerWindows lists the screens and the windows open now.
func (a *App) ComposerWindows() (ComposerWindows, error) {
	screens, wins, err := composer.ListWindows(a.ctx)
	if err != nil {
		return ComposerWindows{}, err
	}
	w := ComposerWindows{Screens: screens, Windows: wins, Icons: map[string]string{}}
	for _, win := range wins {
		if _, ok := w.Icons[win.Path]; !ok && win.Path != "" {
			w.Icons[win.Path] = appicon.DataURL(win.Path)
		}
	}
	return w, nil
}

// ShowHome shows the Home page (composer.AppWindow).
func (a *App) ShowHome() error {
	a.emit("page", "home")
	return nil
}

// Minimise minimises the window (composer.AppWindow).
func (a *App) Minimise() error {
	runtime.WindowMinimise(a.ctx)
	return nil
}

// ComposerBrowse opens a file (kind "file") or folder dialog titled title, starting where current
// points: the path picked, "" when closed without a pick.
func (a *App) ComposerBrowse(kind, title, current string) (string, error) {
	opts := runtime.OpenDialogOptions{Title: title, DefaultDirectory: startDir(current)}
	if kind == "folder" {
		return runtime.OpenDirectoryDialog(a.ctx, opts)
	}
	return runtime.OpenFileDialog(a.ctx, opts)
}
