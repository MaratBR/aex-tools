package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/proc"
	"aex/internal/settings"
)

// Widgets: a plugin can offer widgets for the window's home page (internal/webui/home.go). A widget
// is one HTML page (its HTML, CSS and JS) built into the plugin, so the approval of the plugin's
// file covers it too, plus calls it makes for its data:
//
//	<plugin> --aex-widget <id>              prints the widget's page
//	<plugin> --aex-widget-call <id> <call>  runs one of its calls: JSON args on stdin, and on stdout
//	                                        {"result": ...} or {"error": "..."}
//
// The widgets are listed in --aex-describe ("widgets": [{"id", "name", "summary", "w", "h"}]). aex
// runs neither flag unless the plugin is safe, and a call only with the plugin's access granted: it
// never asks, since a widget has no run to ask in (WidgetError says what is missing).

const (
	widgetFlag     = "--aex-widget"
	widgetCallFlag = "--aex-widget-call"
	widgetTimeout  = 20 * time.Second
	widgetMaxOut   = 4 << 20
)

// Option is what Main takes after the tool: the Access a plugin needs and the Widgets it offers.
type Option interface{ option() }

func (Access) option() {}
func (Widget) option() {}

// Widget is a widget a plugin offers (see Main). The window shows its page in a sandboxed frame with
// no network: its data comes from Calls, through aex.call(name, args) (widgets/sdk.js).
type Widget struct {
	ID      string // unique in the plugin: lowercase letters, digits, - and _
	Name    string
	Summary string
	W, H    int    // the size it is added with, in grid cells (1-4)
	HTML    string // its page, usually go:embed
	// Calls are its data calls. Each gets the call's args as JSON and returns what to send back
	// (marshalled to JSON), running in the plugin with its settings and granted access, but with no
	// way to ask questions.
	Calls map[string]func(args json.RawMessage) (any, error)
}

// WidgetInfo is a widget as a plugin describes it.
type WidgetInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	W       int    `json:"w"`
	H       int    `json:"h"`
}

var widgetID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func describeWidgets(ws []Widget) []WidgetInfo {
	var d []WidgetInfo
	for _, w := range ws {
		d = append(d, WidgetInfo{ID: w.ID, Name: w.Name, Summary: w.Summary, W: w.W, H: w.H})
	}
	return d
}

// serveWidget answers --aex-widget and --aex-widget-call (args after the flag) for Main.
func serveWidget(flag string, args []string, ws []Widget) error {
	if len(args) == 0 {
		return fmt.Errorf("%s needs a widget id", flag)
	}
	i := slices.IndexFunc(ws, func(w Widget) bool { return w.ID == args[0] })
	if i < 0 {
		return fmt.Errorf("no widget %q", args[0])
	}
	w := ws[i]
	if flag == widgetFlag {
		_, err := io.WriteString(os.Stdout, w.HTML)
		return err
	}
	if len(args) < 2 {
		return fmt.Errorf("%s needs a widget id and a call", flag)
	}
	reply := map[string]any{}
	call, ok := w.Calls[args[1]]
	in, err := io.ReadAll(io.LimitReader(os.Stdin, widgetMaxOut))
	switch {
	case err != nil:
		reply["error"] = err.Error()
	case !ok:
		reply["error"] = fmt.Sprintf("widget %s has no call %q", w.ID, args[1])
	default:
		if len(bytes.TrimSpace(in)) == 0 {
			in = []byte("{}")
		}
		result, err := call(in)
		if err != nil {
			reply["error"] = err.Error()
		} else {
			reply["result"] = result
		}
	}
	return json.NewEncoder(os.Stdout).Encode(reply)
}

// WidgetError is why a plugin's widget cannot load or call: the plugin is not safe, or has not been
// granted its access. The window shows it with a way to fix it (plugins allow <name>).
type WidgetError struct {
	Plugin string
	Reason string
}

func (e *WidgetError) Error() string { return fmt.Sprintf("plugin %s %s", e.Plugin, e.Reason) }

// Cached per file hash: a widget's page and the plugin's description (for its access).
var (
	widgetMu        sync.Mutex
	widgetPages     = map[string]string{}      // hash + " " + id
	widgetDescribed = map[string]description{} // hash
)

// openSafe opens the plugin at path, failing (without asking) unless it is safe.
func openSafe(name, path string) (*openFile, error) {
	o, err := openPlugin(path)
	if err != nil {
		return nil, err
	}
	s, err := state(path, o.hash)
	if err == nil && s != Safe {
		err = &WidgetError{name, s.String()}
	}
	if err != nil {
		o.close()
		return nil, err
	}
	return o, nil
}

// WidgetPage is the page of the plugin's widget id. The plugin must be safe.
func WidgetPage(p Info, id string) (string, error) {
	if !widgetID.MatchString(id) {
		return "", fmt.Errorf("bad widget id %q", id)
	}
	o, err := openSafe(p.Name, p.Path)
	if err != nil {
		return "", err
	}
	key := o.hash + " " + id
	widgetMu.Lock()
	page, ok := widgetPages[key]
	widgetMu.Unlock()
	if ok {
		o.close()
		return page, nil
	}
	env, err := widgetEnv(nil)
	if err != nil {
		o.close()
		return "", err
	}
	out, err := runWidget(o, p.Path, env, nil, widgetFlag, id)
	if err != nil {
		return "", err
	}
	widgetMu.Lock()
	widgetPages[key] = string(out)
	widgetMu.Unlock()
	return string(out), nil
}

// WidgetCall runs call of the plugin's widget id with args (JSON) and gives its result (JSON). The
// plugin must be safe and, when it asks for access, have it granted, with its settings set.
func WidgetCall(p Info, id, call string, args []byte) (json.RawMessage, error) {
	if !widgetID.MatchString(id) {
		return nil, fmt.Errorf("bad widget id %q", id)
	}
	o, err := openSafe(p.Name, p.Path)
	if err != nil {
		return nil, err
	}
	// o is let go of once the plugin starts (openFile.start); closing it again does nothing.
	defer func() {
		if o != nil {
			o.close()
		}
	}()
	approved := o.hash
	widgetMu.Lock()
	d, ok := widgetDescribed[approved]
	widgetMu.Unlock()
	if !ok {
		if d, err = describe(o, p.Path); err != nil {
			return nil, err
		}
		widgetMu.Lock()
		widgetDescribed[approved] = d
		widgetMu.Unlock()
		// describe let go of the file; what runs must still be what was approved.
		if o, err = openPlugin(p.Path); err != nil {
			return nil, err
		}
		if o.hash != approved {
			return nil, &WidgetError{p.Name, "changed while starting"}
		}
	}
	pass, err := granted(p.Name, p.Path, approved, d.Access)
	if err != nil {
		return nil, err
	}
	env, err := widgetEnv(pass)
	if err != nil {
		return nil, err
	}
	out, err := runWidget(o, p.Path, env, args, widgetCallFlag, id, call)
	if err != nil {
		return nil, err
	}
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(out, &reply); err != nil {
		return nil, fmt.Errorf("%s printed no valid JSON: %w", widgetCallFlag, err)
	}
	if reply.Error != "" {
		return nil, errors.New(reply.Error)
	}
	if reply.Result == nil {
		return json.RawMessage("null"), nil
	}
	return reply.Result, nil
}

// widgetEnv is the plugin's environment (settings.PluginEnv) with the secret settings in pass, and
// no way to reach aex for questions.
func widgetEnv(pass []string) ([]string, error) {
	env, err := settings.PluginEnv(true, pass)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(env, func(kv string) bool {
		name, _, _ := strings.Cut(strings.ToUpper(kv), "=")
		return name == promptsVar || name == promptTokenVar
	}), nil
}

// granted is grant without asking: the secret settings to pass, or a WidgetError when the access
// is not granted for this hash or a setting it needs is not set.
func granted(name, path, hash string, access []Access) ([]string, error) {
	if len(access) == 0 {
		return nil, nil
	}
	access = slices.Compact(slices.Sorted(slices.Values(access)))
	names := make([]string, len(access))
	for i, a := range access {
		if _, ok := accessKinds[a]; !ok {
			return nil, fmt.Errorf("plugin %s asks for unknown access %q", name, a)
		}
		names[i] = string(a)
	}
	have, err := settings.Credentials.Get(grantKey(path))
	if err != nil {
		return nil, err
	}
	if have != hash+" "+strings.Join(names, ",") {
		return nil, &WidgetError{name, "has not been granted access to " + strings.Join(names, ", ")}
	}
	var pass []string
	for _, a := range access {
		for _, s := range accessKinds[a].settings {
			if settings.Get(s) == "" {
				return nil, &WidgetError{name, "needs " + s + ", which is not set"}
			}
			if settings.IsSecret(s) {
				pass = append(pass, s)
			}
		}
	}
	return pass, nil
}

// runWidget runs the plugin (o, held open since hashing) with args, stdin as input, and gives what
// it printed. Nothing reaches the window: stderr is kept for the error.
func runWidget(o *openFile, path string, env []string, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), widgetTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	proc.HideConsole(cmd)
	var out, errOut capped
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := o.start(cmd)
	if err == nil {
		err = cmd.Wait()
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out after %v", args[0], widgetTimeout)
	}
	if err != nil {
		if msg := strings.TrimSpace(errOut.String()); msg != "" {
			return nil, fmt.Errorf("%s failed: %s", args[0], lastLine(msg))
		}
		return nil, fmt.Errorf("%s failed: %w", args[0], err)
	}
	if out.over {
		return nil, fmt.Errorf("%s printed more than %d MB", args[0], widgetMaxOut>>20)
	}
	return out.Bytes(), nil
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// capped is a buffer that keeps at most widgetMaxOut bytes.
type capped struct {
	bytes.Buffer
	over bool
}

func (c *capped) Write(p []byte) (int, error) {
	if room := widgetMaxOut - c.Len(); len(p) > room {
		c.over = true
		c.Buffer.Write(p[:max(room, 0)])
		return len(p), nil
	}
	return c.Buffer.Write(p)
}

// Allow approves the plugin (asking unless it is safe) and grants the access it asks for (asking
// unless granted), as running it would, without running it.
func Allow(p Info) error {
	o, err := approve(p.Name, p.Path)
	if err != nil {
		return err
	}
	approved := o.hash
	d, err := describe(o, p.Path)
	if err != nil {
		return err
	}
	_, err = grant(p.Name, p.Path, approved, d.Access)
	return err
}

// Find is the plugin called name in Dir, by file name only: nothing is opened or run. Enough for
// WidgetPage and WidgetCall, which check its file themselves.
func Find(name string) (Info, error) {
	dir, err := Dir()
	if err != nil {
		return Info{}, err
	}
	entries, _ := os.ReadDir(dir) // no folder: no plugin
	for _, e := range entries {
		if n, ok := pluginName(e); ok && n == name {
			return Info{Name: name, Path: filepath.Join(dir, e.Name())}, nil
		}
	}
	return Info{}, fmt.Errorf("plugin %s is not in the plugins folder: %w", name, ErrNoPlugin)
}

// ErrNoPlugin is Find's error for a plugin not in the plugins folder.
var ErrNoPlugin = errors.New("no such plugin")
