// Package webui runs the tools in a desktop window (Wails, the system WebView2 on Windows)
// instead of the terminal. Tools stay console programs: what they print to stdout and stderr is
// streamed to the window, and their prompts (ui.Input, Confirm, Choose, WaitKey) are answered there
// through ui.Remote.
package webui

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"aex/internal/tool"
	"aex/internal/ui"
)

//go:embed frontend
var assets embed.FS

// Host is what the window needs from main: the tools, how to run one and the header lines.
type Host struct {
	Tools  func() []tool.Tool
	Run    func(name string, t *tool.Tool, args []string) (status string, ok bool)
	Header func(wait bool) []InfoLine // wait for the login checks to finish first
	// Ready, when set, runs once the window has loaded, like a tool: it may print and ask.
	Ready func()
	// Changed, when set, runs after the settings form changed settings (or wiped them).
	Changed func()
}

// InfoLine is one line of the header (the logins and settings under the tools): a label and styled text.
type InfoLine struct {
	Label    string    `json:"label"`
	State    string    `json:"state,omitempty"` // for a login: checking | in | out | unset | error
	Segments []Segment `json:"segments"`
	Open     string    `json:"open,omitempty"`   // a folder the line opens when clicked
	Action   string    `json:"action,omitempty"` // "settings": the line opens the settings form
}

// Segment is styled text: Kind is plain, dim, bold or warn.
type Segment struct {
	Text string `json:"text"`
	Kind string `json:"kind"`
}

// Run opens the window and returns when it is closed.
func Run(title string, host Host) error {
	a := &App{host: host, waiting: map[int]chan answer{}, fields: map[int]ui.Field{}}
	sub, err := fs.Sub(assets, "frontend")
	if err != nil {
		return err
	}
	return wails.Run(&options.App{
		Title:            title,
		Width:            1100,
		Height:           760,
		MinWidth:         640,
		MinHeight:        420,
		AssetServer:      &assetserver.Options{Assets: sub},
		BackgroundColour: &options.RGBA{R: 24, G: 26, B: 31, A: 255},
		OnStartup:        a.startup,
		Bind:             []any{a},
	})
}

// App is bound to the frontend: its exported methods are callable as window.go.webui.App.<Name>.
type App struct {
	host Host
	emit func(event string, data any) // to the frontend
	out  *stream                      // stdout and stderr, nil when not captured

	mu        sync.Mutex
	running   bool
	readyDone bool
	nextID    int
	waiting   map[int]chan answer
	fields    map[int]ui.Field // Input prompts still open, for Describe
}

type answer struct {
	value     string
	cancelled bool
}

func (a *App) startup(ctx context.Context) {
	a.emit = func(event string, data any) { runtime.EventsEmit(ctx, event, data) }
	ui.Remote = a
	out, err := captureOutput(func(text string) { a.emit("output", text) })
	if err != nil {
		runtime.LogErrorf(ctx, "capturing output: %v", err)
		return
	}
	a.out = out
}

// send emits a prompt or the end of a run, after the output printed before it.
func (a *App) send(event string, data any) {
	if a.out != nil {
		a.out.Sync()
	}
	a.emit(event, data)
}

// ToolInfo is one tool as the frontend sees it.
type ToolInfo struct {
	Name    string     `json:"name"`
	Summary string     `json:"summary"`
	Sub     []ToolInfo `json:"sub,omitempty"`
}

// Tools lists the tools: built-in ones, then plugins. Hidden ones are left out.
func (a *App) Tools() []ToolInfo {
	var list func([]tool.Tool) []ToolInfo
	list = func(ts []tool.Tool) []ToolInfo {
		var out []ToolInfo
		for _, t := range ts {
			if !t.Hidden {
				out = append(out, ToolInfo{Name: t.Name, Summary: t.Summary, Sub: list(t.Sub)})
			}
		}
		return out
	}
	return list(a.host.Tools())
}

// Header gives the header lines (logins, settings, folders); wait waits for the login checks first.
func (a *App) Header(wait bool) []InfoLine { return a.host.Header(wait) }

// Ready runs Host.Ready, the first time only; it reports whether it started it. Its output and
// questions follow as events, then a "finished" event with quiet set.
func (a *App) Ready() bool {
	a.mu.Lock()
	start := a.host.Ready != nil && !a.readyDone && !a.running
	if start {
		a.readyDone, a.running = true, true
	}
	a.mu.Unlock()
	if !start {
		return false
	}
	go func() {
		defer func() {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			a.send("finished", map[string]any{"quiet": true, "ok": true})
		}()
		a.host.Ready()
	}()
	return true
}

// Run starts the tool at path (group names, then the tool) with a space-separated args line.
// Output, prompts and the final "finished" event follow as events.
func (a *App) Run(path []string, args []string) error {
	t := a.find(path)
	if t == nil {
		return fmt.Errorf("unknown tool: %s", strings.Join(path, " "))
	}
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("a tool is already running")
	}
	a.running = true
	a.mu.Unlock()
	go func() {
		status, ok := a.host.Run(strings.Join(path, " "), t, args)
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		a.send("finished", map[string]any{"status": status, "ok": ok})
	}()
	return nil
}

func (a *App) find(path []string) *tool.Tool {
	list := a.host.Tools()
	var t *tool.Tool
	for _, name := range path {
		t = nil
		for i := range list {
			if list[i].Name == name && !list[i].Hidden {
				t = &list[i]
			}
		}
		if t == nil {
			return nil
		}
		list = t.Sub
	}
	return t
}

// Answer answers the open prompt id; cancelled answers it with a "cancelled" error.
func (a *App) Answer(id int, value string, cancelled bool) {
	a.mu.Lock()
	ch := a.waiting[id]
	delete(a.waiting, id)
	a.mu.Unlock()
	if ch != nil {
		ch <- answer{value, cancelled}
	}
}

// Describe gives the line shown under the open Input prompt id for value (Field.Describe).
func (a *App) Describe(id int, value string) string {
	a.mu.Lock()
	f, ok := a.fields[id]
	a.mu.Unlock()
	if !ok || f.Describe == nil {
		return ""
	}
	return f.Describe(value)
}

// prompt is a question sent to the frontend as a "prompt" event.
type prompt struct {
	ID          int         `json:"id"`
	Kind        string      `json:"kind"` // input | confirm | choose | key
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Secret      bool        `json:"secret,omitempty"`
	Describe    bool        `json:"describe,omitempty"`
	Value       string      `json:"value,omitempty"` // the answer to start with
	Error       string      `json:"error,omitempty"` // why the last answer was rejected
	Options     []ui.Option `json:"options,omitempty"`
	DefaultYes  bool        `json:"defaultYes,omitempty"`
}

// ask sends p and waits for its answer; f, when set, is the Input field it asks for (for Describe).
func (a *App) ask(p prompt, f *ui.Field) answer {
	ch := make(chan answer, 1)
	a.mu.Lock()
	a.nextID++
	p.ID = a.nextID
	a.waiting[p.ID] = ch
	if f != nil {
		a.fields[p.ID] = *f
	}
	a.mu.Unlock()
	a.send("prompt", p)
	ans := <-ch
	a.mu.Lock()
	delete(a.fields, p.ID)
	a.mu.Unlock()
	return ans
}

var errCancelled = errors.New("cancelled")

// The prompts below leave no record of the answer in the output, as the terminal ones do: the
// window shows each answer as the user's message.

func (a *App) Input(f ui.Field) (string, error) {
	p := prompt{Kind: "input", Title: f.Title, Description: f.Description, Placeholder: f.Placeholder,
		Secret: f.Secret, Describe: f.Describe != nil}
	for {
		ans := a.ask(p, &f)
		if ans.cancelled {
			return "", errCancelled
		}
		value := ans.value
		if f.Paste != nil {
			value = f.Paste(value)
		}
		value = strings.TrimSpace(value)
		if f.Validate != nil {
			if err := f.Validate(value); err != nil {
				p.Error, p.Value = err.Error(), ans.value
				continue
			}
		}
		return value, nil
	}
}

func (a *App) Confirm(question string, defaultYes bool) (bool, error) {
	ans := a.ask(prompt{Kind: "confirm", Title: question, DefaultYes: defaultYes}, nil)
	if ans.cancelled {
		return false, errCancelled
	}
	return ans.value == "yes", nil
}

func (a *App) Choose(title string, options []ui.Option) (string, error) {
	ans := a.ask(prompt{Kind: "choose", Title: title, Options: options}, nil)
	if ans.cancelled {
		return "", errCancelled
	}
	for _, o := range options {
		if o.Value == ans.value {
			return o.Value, nil
		}
	}
	return "", fmt.Errorf("not one of the choices: %q", ans.value)
}

func (a *App) WaitKey() { a.ask(prompt{Kind: "key", Title: "Press any key to continue"}, nil) }

// ClearScreen does nothing: the conversation keeps earlier runs, as a chat does.
func (a *App) ClearScreen() {}
