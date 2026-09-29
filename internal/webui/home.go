package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/plugin"
	"aex/internal/quota"
	"aex/internal/settings"
)

// The home page: a grid of widgets the user picks (frontend/home.js). A widget is a page of its own
// (HTML, CSS and JS), shown in a sandboxed frame of the grid that can reach the window only through
// messages (frontend/widgets/sdk.js): for its data (WidgetCall) and to run a tool. Built-in widgets
// are frontend/widgets/<id>.html and call the widget APIs below; a plugin's widgets (internal/plugin,
// widget.go) are "<plugin>/<id>", with their page and calls in the plugin.

// WidgetInfo is a widget that can be added to the home page.
type WidgetInfo struct {
	ID      string `json:"id"`               // built-in: its page is frontend/widgets/<id>.html; plugin: <plugin>/<id>
	Plugin  string `json:"plugin,omitempty"` // the plugin it comes from, for a plugin's widget
	Name    string `json:"name"`
	Summary string `json:"summary"`
	W       int    `json:"w"` // size it is added with, in grid cells
	H       int    `json:"h"`
}

var widgetCatalog = []WidgetInfo{
	{ID: "quota", Name: "AEXT quota", Summary: "Hours logged this month against the quota, and last month", W: 2, H: 2},
}

// widgetAPIs are the calls widgets make with aex.call(name, args). They never prompt: a widget has
// no run to ask its questions in, so one that needs a login says so instead.
var widgetAPIs = map[string]func(args map[string]any) (any, error){
	"quota": quotaAPI,
}

// Grid limits: sizes are clamped to them, so a layout from an older or edited file still fits.
const (
	maxWidgetW    = 4
	maxWidgetH    = 4
	maxWidgets    = 48
	homeFileLimit = 1 << 20
)

// HomeWidget is one widget placed on the home page.
type HomeWidget struct {
	ID     string `json:"id"`     // this placement (a widget can be added more than once)
	Widget string `json:"widget"` // WidgetInfo.ID
	W      int    `json:"w"`
	H      int    `json:"h"`
}

// HomeLayout is the home page: its widgets in grid order.
type HomeLayout struct {
	Widgets []HomeWidget `json:"widgets"`
}

// WidgetList is what can be added to the home page.
type WidgetList struct {
	Widgets []WidgetInfo `json:"widgets"`
	// Inactive are plugins not approved (or changed since): they may have widgets, unknown until
	// approved, since they cannot even describe themselves before.
	Inactive []InactivePlugin `json:"inactive,omitempty"`
}

// InactivePlugin is a plugin that is not safe.
type InactivePlugin struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// Widgets lists the widgets that can be added: built-in ones, then plugins' (which describes the
// safe plugins).
func (a *App) Widgets() WidgetList {
	list := WidgetList{Widgets: slices.Clone(widgetCatalog)}
	plugins, err := plugin.List()
	if err != nil {
		return list
	}
	for _, p := range plugins {
		if p.State != plugin.Safe {
			list.Inactive = append(list.Inactive, InactivePlugin{p.Name, p.State.String()})
			continue
		}
		for _, w := range p.Widgets {
			list.Widgets = append(list.Widgets, WidgetInfo{ID: p.Name + "/" + w.ID, Plugin: p.Name, Name: w.Name,
				Summary: w.Summary, W: min(max(w.W, 1), maxWidgetW), H: min(max(w.H, 1), maxWidgetH)})
		}
	}
	return list
}

// Home gives the home page as saved in the data folder (settings.HomeFile); empty when there is none.
// Built-in widgets no longer known are left out; plugins' stay, and say so when they cannot load.
func (a *App) Home() (HomeLayout, error) {
	layout := HomeLayout{Widgets: []HomeWidget{}}
	b, err := os.ReadFile(settings.HomeFile)
	if errors.Is(err, fs.ErrNotExist) {
		return layout, nil
	}
	if err != nil {
		return layout, err
	}
	var saved HomeLayout
	if err := json.Unmarshal(b, &saved); err != nil {
		return layout, fmt.Errorf("%s: %w", settings.HomeFile, err)
	}
	layout.Widgets = cleanWidgets(saved.Widgets)
	return layout, nil
}

// SaveHome saves the home page to the data folder.
func (a *App) SaveHome(layout HomeLayout) error {
	for _, w := range layout.Widgets {
		if !knownWidget(w.Widget) {
			return fmt.Errorf("unknown widget: %q", w.Widget)
		}
		if !placementID.MatchString(w.ID) {
			return fmt.Errorf("bad widget id: %q", w.ID)
		}
	}
	if len(layout.Widgets) > maxWidgets {
		return fmt.Errorf("at most %d widgets", maxWidgets)
	}
	layout.Widgets = cleanWidgets(layout.Widgets)
	b, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > homeFileLimit {
		return errors.New("home page too big to save")
	}
	if err := os.MkdirAll(filepath.Dir(settings.HomeFile), 0o700); err != nil {
		return err
	}
	return os.WriteFile(settings.HomeFile, append(b, '\n'), 0o600)
}

var placementID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// pluginWidget matches a plugin's widget id: <plugin>/<id>.
var pluginWidget = regexp.MustCompile(`^([^/\:*?"<>|]{1,128})/([a-z0-9][a-z0-9_-]{0,63})$`)

// knownWidget reports whether id is a built-in widget or looks like a plugin's.
func knownWidget(id string) bool {
	return slices.ContainsFunc(widgetCatalog, func(w WidgetInfo) bool { return w.ID == id }) ||
		pluginWidget.MatchString(id) && !strings.HasPrefix(id, ".")
}

// cleanWidgets drops unknown widgets and repeated ids, and clamps sizes to the grid.
func cleanWidgets(list []HomeWidget) []HomeWidget {
	out := []HomeWidget{}
	seen := map[string]bool{}
	for _, w := range list {
		if !knownWidget(w.Widget) || !placementID.MatchString(w.ID) || seen[w.ID] || len(out) == maxWidgets {
			continue
		}
		seen[w.ID] = true
		w.W, w.H = min(max(w.W, 1), maxWidgetW), min(max(w.H, 1), maxWidgetH)
		out = append(out, w)
	}
	return out
}

// WidgetPage is a widget's page, or why it cannot be shown.
type WidgetPage struct {
	HTML string `json:"html,omitempty"`
	// Problem is why a plugin's widget cannot load: the plugin is missing, not approved, changed or
	// not granted its access. Plugin names it, for the window to offer "plugins allow".
	Problem string `json:"problem,omitempty"`
	Plugin  string `json:"plugin,omitempty"`
}

// WidgetPage gives the page of widget (WidgetInfo.ID).
func (a *App) WidgetPage(widget string) (WidgetPage, error) {
	if m := pluginWidget.FindStringSubmatch(widget); m != nil {
		p, err := plugin.Find(m[1])
		if err == nil {
			var html string
			if html, err = plugin.WidgetPage(p, m[2]); err == nil {
				return WidgetPage{HTML: html}, nil
			}
		}
		if we, ok := errors.AsType[*plugin.WidgetError](err); ok {
			return WidgetPage{Problem: we.Error(), Plugin: we.Plugin}, nil
		}
		if errors.Is(err, plugin.ErrNoPlugin) {
			return WidgetPage{Problem: fmt.Sprintf("plugin %s is not in the plugins folder", m[1])}, nil
		}
		return WidgetPage{}, err
	}
	if !knownWidget(widget) {
		return WidgetPage{}, fmt.Errorf("unknown widget: %q", widget)
	}
	b, err := assets.ReadFile("frontend/widgets/" + widget + ".html")
	return WidgetPage{HTML: string(b)}, err
}

// WidgetCall runs a call of widget with args and gives its result: for a built-in widget one of
// widgetAPIs, for a plugin's one of its calls. The window only passes on calls from the widget's
// own frame.
func (a *App) WidgetCall(widget, name string, args map[string]any) (any, error) {
	if m := pluginWidget.FindStringSubmatch(widget); m != nil {
		p, err := plugin.Find(m[1])
		if err != nil {
			return nil, err
		}
		in, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		return plugin.WidgetCall(p, m[2], name, in)
	}
	if !knownWidget(widget) {
		return nil, fmt.Errorf("unknown widget: %q", widget)
	}
	api, ok := widgetAPIs[name]
	if !ok {
		return nil, fmt.Errorf("no widget API called %q", name)
	}
	return api(args)
}

// QuotaReply is the quota API's result: this month, then last month.
type QuotaReply struct {
	NeedLogin   bool          `json:"needLogin,omitempty"` // no valid AEXT session: nothing else is set
	Today       string        `json:"today,omitempty"`
	HoursPerDay float64       `json:"hoursPerDay,omitempty"`
	Country     string        `json:"country,omitempty"`
	Months      []quota.Month `json:"months,omitempty"`
	Warnings    []string      `json:"warnings,omitempty"` // leave warnings (quota --verbose)
}

func quotaAPI(map[string]any) (any, error) {
	c, err := aext.NewQuiet()
	if err != nil {
		return nil, err
	}
	if !c.HasSession() {
		return QuotaReply{NeedLogin: true}, nil
	}
	now := dates.Today()
	d, err := quota.FetchMonths(c, now)
	if errors.Is(err, aext.ErrNoSession) {
		return QuotaReply{NeedLogin: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return QuotaReply{
		Today:       now,
		HoursPerDay: settings.HoursPerDay(),
		Country:     settings.WorkingDaysCountry,
		Months:      []quota.Month{quota.ComputeMonth(now, now, d), quota.ComputeMonth(dates.PrevMonthStart(now), now, d)},
		Warnings:    d.Warnings,
	}, nil
}
