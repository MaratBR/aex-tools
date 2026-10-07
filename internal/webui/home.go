package webui

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/google"
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
	W       int    `json:"w"` // size it is added with, in twelfths of the grid (gridColumns), whatever its columns
	H       int    `json:"h"`
	// Refresh is how often, in seconds, the home page tells it to load again (sdk.js: aex.onRefresh)
	// while auto refresh is on (HomeLayout.AutoRefreshOff); 0: only after runs.
	Refresh int `json:"refresh,omitempty"`
	// Debug marks a widget for developing aex or a plugin: offered, and usable, only with --debug
	// (settings.Debug).
	Debug bool `json:"debug,omitempty"`
	// Settings are the settings a placement starts with; none: {}.
	Settings json.RawMessage `json:"settings,omitempty"`
}

var widgetCatalog = []WidgetInfo{
	{ID: "quota", Name: "AEXT quota", Summary: "Hours logged this month against the quota, and last month", W: 6, H: 2, Refresh: 60},
	{ID: "google-calendars", Name: "Google Calendar", Summary: "What is on now in a Google calendar you pick, and what comes in the next working days", W: 6, H: 2, Refresh: 60},
	{ID: "jira-tickets", Name: "Jira tickets", Summary: "Open tickets assigned to you, where you are in fields you pick, or matching JQL; in tabs", W: 6, H: 2, Refresh: 60},
	{ID: "cat", Name: "Cat as a service", Summary: "A random cat from cataas.com, a new one on click", W: 3, H: 1},
	{ID: "2048", Name: "2048", Summary: "The sliding tiles game: join the numbers to get to 2048", W: 4, H: 2},
	{ID: "clock", Name: "Clock", Summary: "The time in one or two time zones you pick, Central Time by default", W: 3, H: 1},
	{ID: "git-status", Name: "Git status", Summary: "Branch, changes and commits to push or pull in the git repos you pick", W: 6, H: 2, Refresh: 5},
	{ID: "shortcuts", Name: "Shortcuts", Summary: "Buttons that run the tools you pick, each with a name or an icon", W: 2, H: 1},
}

// renamedWidgets are widgets that became others: a placement of one is loaded as the other.
var renamedWidgets = map[string]string{"cm-release/cm-repos-state": "git-status"}

// widgetAPIs are the calls widgets make with aex.call(name, args). They never prompt: a widget has
// no run to ask its questions in, so one that needs a login says so instead.
var widgetAPIs = map[string]func(args map[string]any) (any, error){
	"quota":           quotaAPI,
	"worklogCheck":    worklogCheckAPI,
	"googleCalendars": googleCalendarsAPI,
	"googleNow":       googleNowAPI,
	"jiraFields":      jiraFieldsAPI,
	"jiraTickets":     jiraTicketsAPI,
	"jiraOpen":        jiraOpenAPI,
	"cat":             catAPI,
	"gitStatus":       gitStatusAPI,
	"gitClient":       gitClientAPI,
	"gitOpenClient":   gitOpenClientAPI,
	"gitDefaultRepos": gitDefaultReposAPI,
}

// windowAPIs are widget calls that need the window: a dialog the user opened from the widget.
var windowAPIs = map[string]func(a *App, args map[string]any) (any, error){
	"chooseFolder": chooseFolderAPI,
	"tools":        toolsAPI,
}

// Grid limits: sizes are clamped to them, so a layout from an older or edited file still fits.
const (
	gridColumns   = 12 // the grid's columns at full width by default, and what WidgetInfo.W is in
	oldColumns    = 4  // the grid's columns before, still in home.json files without Columns, and plugins' sizes
	maxWidgetH    = 4
	maxWidgets    = 48
	maxRow        = maxWidgets * maxWidgetH // a widget's top row is at most this: room for gaps, not endless
	homeFileLimit = 1 << 20
	maxSettings   = 8 << 10 // a placement's settings, as JSON
	minRefresh    = 5       // seconds: a plugin's widget refreshes at most this often, each call starts the plugin
	minHomeWidth  = 900     // px: the home page's width at least, so the grid keeps all its columns (home.js: colsFor)
	maxHomeWidth  = 8000
)

// The grid's columns at full width that can be picked (HomeLayout.Columns): more for finer widths
// and places (home.js: minColumns, maxColumns).
const (
	minColumns = 2
	maxColumns = 64
)

func validColumns(n int) bool { return n >= minColumns && n <= maxColumns }

// HomeWidget is one widget placed on the home page.
type HomeWidget struct {
	ID     string `json:"id"`     // this placement (a widget can be added more than once)
	Widget string `json:"widget"` // WidgetInfo.ID
	W      int    `json:"w"`
	H      int    `json:"h"`
	// X and Y are its place at full width: the column (0 to Columns-W) and row its top left
	// corner is in, with gaps allowed. None (a file from before) places it where it fits first,
	// after the widgets that have one, as the grid used to pack them.
	X *int `json:"x,omitempty"`
	Y *int `json:"y,omitempty"`
	// Settings are this placement's own, a JSON object the widget keeps (sdk.js: aex.settings,
	// aex.saveSettings), such as which calendar it shows.
	Settings json.RawMessage `json:"settings,omitempty"`
}

// HomeLayout is the home page: its widgets, top to bottom and left to right.
type HomeLayout struct {
	Widgets []HomeWidget `json:"widgets"`
	// AutoRefreshOff turns off refreshing widgets on their own (WidgetInfo.Refresh), on by default.
	AutoRefreshOff bool `json:"autoRefreshOff,omitempty"`
	// Width is how wide the home page is at most, in px, centred in the window; 0: the window's
	// width. Set by dragging its edges in Edit.
	Width int `json:"width,omitempty"`
	// Columns is the grid's columns at full width, minColumns to maxColumns (picked in Edit): widths
	// and places are in them. A file from before has none, its widths are in oldColumns.
	Columns int `json:"columns,omitempty"`
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
	list := WidgetList{Widgets: slices.DeleteFunc(slices.Clone(widgetCatalog), func(w WidgetInfo) bool { return w.Debug && !settings.Debug })}
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
			if w.Debug && !settings.Debug {
				continue
			}
			info := WidgetInfo{ID: p.Name + "/" + w.ID, Plugin: p.Name, Name: w.Name, Debug: w.Debug,
				Summary: w.Summary, W: min(max(w.W, 1), oldColumns) * (gridColumns / oldColumns), H: min(max(w.H, 1), maxWidgetH)}
			if w.Refresh > 0 {
				info.Refresh = max(w.Refresh, minRefresh)
			}
			list.Widgets = append(list.Widgets, info)
		}
	}
	if settings.Debug {
		list.Widgets = append(list.Widgets, unknownDebugWidget())
	}
	return list
}

// unknownDebugWidget is a widget aex does not know, offered with --debug to see how one looks on the
// home page (as after a downgrade, or with a widget removed from aex): a made-up id and settings,
// new each time.
func unknownDebugWidget() WidgetInfo {
	id := "debug-unknown-" + strings.ToLower(rand.Text()[:8])
	s, _ := json.Marshal(map[string]any{"note": "made up by the Unknown widget debug entry", "seed": rand.Text()})
	return WidgetInfo{ID: id, Name: "Unknown widget", Summary: "A widget aex does not know, with made-up settings: to see how one shows",
		W: 3, H: 1, Debug: true, Settings: s}
}

// Home gives the home page as saved in the data folder (settings.HomeFile); empty when there is none.
// Widgets no longer known (built-in or plugins') stay, and say so when they cannot load.
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
	cols := saved.Columns
	switch {
	case cols == 0:
		for i := range saved.Widgets {
			saved.Widgets[i].W *= gridColumns / oldColumns
		}
		cols = gridColumns
	case !validColumns(cols):
		rescale(saved.Widgets, max(cols, 1), gridColumns)
		cols = gridColumns
	}
	for i, w := range saved.Widgets {
		if to, ok := renamedWidgets[w.Widget]; ok {
			saved.Widgets[i].Widget = to
		}
	}
	layout.Columns = cols
	layout.Widgets = cleanWidgets(saved.Widgets, cols)
	layout.AutoRefreshOff = saved.AutoRefreshOff
	layout.Width = homeWidth(saved.Width)
	return layout, nil
}

// SaveHome saves the home page to the data folder.
func (a *App) SaveHome(layout HomeLayout) error {
	for _, w := range layout.Widgets {
		if !validWidgetID(w.Widget) {
			return fmt.Errorf("bad widget: %q", w.Widget)
		}
		if !placementID.MatchString(w.ID) {
			return fmt.Errorf("bad widget id: %q", w.ID)
		}
		if w.Settings != nil && !validSettings(w.Settings) {
			return fmt.Errorf("widget %s: settings must be a JSON object of at most %d KB", w.ID, maxSettings>>10)
		}
	}
	if len(layout.Widgets) > maxWidgets {
		return fmt.Errorf("at most %d widgets", maxWidgets)
	}
	if layout.Columns == 0 {
		layout.Columns = gridColumns
	}
	if !validColumns(layout.Columns) {
		return fmt.Errorf("columns: %d is not from %d to %d", layout.Columns, minColumns, maxColumns)
	}
	layout.Widgets = cleanWidgets(layout.Widgets, layout.Columns)
	layout.Width = homeWidth(layout.Width)
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

// builtinWidget matches a built-in widget's id, whether or not aex has it.
var builtinWidget = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// validWidgetID reports whether id looks like a widget's, built-in or a plugin's: a placement of a
// widget aex does not know (any more, or yet) is kept, and shows that it is unknown.
func validWidgetID(id string) bool {
	return builtinWidget.MatchString(id) || pluginWidget.MatchString(id) && !strings.HasPrefix(id, ".")
}

// knownWidget reports whether id is a built-in widget or looks like a plugin's.
func knownWidget(id string) bool {
	return slices.ContainsFunc(widgetCatalog, func(w WidgetInfo) bool { return w.ID == id }) ||
		pluginWidget.MatchString(id) && !strings.HasPrefix(id, ".")
}

// debugWidget reports whether id is a built-in debug widget that cannot be used without --debug.
func debugWidget(id string) bool {
	return !settings.Debug && slices.ContainsFunc(widgetCatalog, func(w WidgetInfo) bool { return w.ID == id && w.Debug })
}

// validSettings reports whether s is a JSON object small enough to keep.
func validSettings(s json.RawMessage) bool {
	var m map[string]any
	return len(s) <= maxSettings && json.Unmarshal(s, &m) == nil && m != nil
}

// homeWidth clamps the home page's width: 0 (none) stays, others go between minHomeWidth and
// maxHomeWidth.
func homeWidth(w int) int {
	if w <= 0 {
		return 0
	}
	return min(max(w, minHomeWidth), maxHomeWidth)
}

// rescale converts widths and columns of a grid of from columns to one of to (home.js: setColumns).
func rescale(list []HomeWidget, from, to int) {
	conv := func(n int) int { return int(math.Round(float64(n) * float64(to) / float64(from))) }
	for i, w := range list {
		list[i].W = max(conv(w.W), 1)
		if w.X != nil {
			x := conv(*w.X)
			list[i].X = &x
		}
	}
}

// cleanWidgets drops bad widget ids and repeated placement ids, clamps sizes to a grid of cols
// columns, places the widgets (placeWidgets), and compacts settings, dropping those that are not a
// JSON object or too big.
func cleanWidgets(list []HomeWidget, cols int) []HomeWidget {
	out := []HomeWidget{}
	seen := map[string]bool{}
	for _, w := range list {
		if !validWidgetID(w.Widget) || !placementID.MatchString(w.ID) || seen[w.ID] || len(out) == maxWidgets {
			continue
		}
		seen[w.ID] = true
		w.W, w.H = min(max(w.W, 1), cols), min(max(w.H, 1), maxWidgetH)
		if w.Settings != nil {
			var b bytes.Buffer
			if !validSettings(w.Settings) || json.Compact(&b, w.Settings) != nil {
				w.Settings = nil
			} else {
				w.Settings = b.Bytes()
			}
		}
		out = append(out, w)
	}
	return placeWidgets(out, cols)
}

// placeWidgets gives every widget a place in the grid of cols columns, none overlapping another, and
// sorts them by it (top to bottom, then left to right). A widget with a place keeps it (clamped to
// the grid), unless an earlier one (by that order) is there: it then goes down to the first row where
// it fits. A widget without one goes where it fits first from the top left, in their order.
func placeWidgets(list []HomeWidget, cols int) []HomeWidget {
	var placed, rest []HomeWidget
	for _, w := range list {
		if w.X == nil || w.Y == nil {
			w.X, w.Y = nil, nil
			rest = append(rest, w)
			continue
		}
		x, y := min(max(*w.X, 0), cols-w.W), min(max(*w.Y, 0), maxRow)
		w.X, w.Y = &x, &y
		placed = append(placed, w)
	}
	slices.SortStableFunc(placed, cmpPlace)
	out := make([]HomeWidget, 0, len(list))
	for _, w := range placed {
		for overlapsAny(out, *w.X, *w.Y, w.W, w.H) {
			*w.Y++
		}
		out = append(out, w)
	}
	for _, w := range rest {
		x, y := firstFit(out, w.W, w.H, cols)
		w.X, w.Y = &x, &y
		out = append(out, w)
	}
	slices.SortStableFunc(out, cmpPlace)
	return out
}

// cmpPlace orders widgets by their place: top to bottom, then left to right.
func cmpPlace(a, b HomeWidget) int {
	if *a.Y != *b.Y {
		return *a.Y - *b.Y
	}
	return *a.X - *b.X
}

// overlapsAny reports whether a widget of w×h at x, y would overlap one of list (all placed).
func overlapsAny(list []HomeWidget, x, y, w, h int) bool {
	return slices.ContainsFunc(list, func(o HomeWidget) bool {
		return x < *o.X+o.W && *o.X < x+w && y < *o.Y+o.H && *o.Y < y+h
	})
}

// firstFit is the first place, row by row from the top left, where a widget of w×h overlaps none of
// list (all placed), in a grid of cols columns.
func firstFit(list []HomeWidget, w, h, cols int) (x, y int) {
	for y = 0; ; y++ {
		for x = 0; x+w <= cols; x++ {
			if !overlapsAny(list, x, y, w, h) {
				return x, y
			}
		}
	}
}

// WidgetPage is a widget's page, or why it cannot be shown.
type WidgetPage struct {
	HTML string `json:"html,omitempty"`
	// Problem is why a plugin's widget cannot load: the plugin is missing, not approved, changed or
	// not granted its access. Plugin names it, for the window to offer "plugins allow".
	Problem string `json:"problem,omitempty"`
	Plugin  string `json:"plugin,omitempty"`
	// Unknown is set when aex has no widget with that id: Problem says so, shown as a warning.
	Unknown bool `json:"unknown,omitempty"`
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
		if validWidgetID(widget) {
			return WidgetPage{Problem: fmt.Sprintf("unknown widget: this aex has no widget %q, it may come from another version. Remove it in Edit", widget), Unknown: true}, nil
		}
		return WidgetPage{}, fmt.Errorf("unknown widget: %q", widget)
	}
	if debugWidget(widget) {
		return WidgetPage{Problem: plugin.ErrDebugWidget.Error()}, nil
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
	if debugWidget(widget) {
		return nil, plugin.ErrDebugWidget
	}
	if api, ok := windowAPIs[name]; ok {
		return api(a, args)
	}
	api, ok := widgetAPIs[name]
	if !ok {
		return nil, fmt.Errorf("no widget API called %q", name)
	}
	return api(args)
}

// QuotaReply is the quota API's result: the month asked for (this month by default), then the one before.
type QuotaReply struct {
	NeedLogin   bool          `json:"needLogin,omitempty"` // no valid AEXT session: nothing else is set
	Today       string        `json:"today,omitempty"`
	HoursPerDay float64       `json:"hoursPerDay,omitempty"`
	WarnHours   float64       `json:"warnHours"` // a month over short by less than this is a warning, not a failure
	Country     string        `json:"country,omitempty"`
	Months      []quota.Month `json:"months,omitempty"`
	Warnings    []string      `json:"warnings,omitempty"` // leave warnings (quota --verbose)
	Debug       *QuotaDebug   `json:"debug,omitempty"`    // only with --debug: the widget's debug view
}

// QuotaDebug is everything the quota was computed from, for the quota widget's debug view.
type QuotaDebug struct {
	Now           string                    `json:"now"`    // in TZ_OFFSET_HOURS
	Device        string                    `json:"device"` // the device's clock and zone
	TZOffset      any                       `json:"tzOffset"`
	IgnoredStatus []string                  `json:"ignoredStatuses"`
	Took          string                    `json:"took"`
	Raw           quota.Raw                 `json:"raw"`
	WorkingDays   map[string]bool           `json:"workingDays"`
	LeaveDays     map[string]quota.LeaveDay `json:"leaveDays"`
	OffDays       map[string]bool           `json:"offDays"`
	HoursByDay    map[string]float64        `json:"hoursByDay"`
}

func quotaAPI(args map[string]any) (any, error) {
	now := dates.Today()
	month := now
	if m, _ := args["month"].(string); m != "" {
		if _, err := time.Parse("2006-01", m); err != nil {
			return nil, fmt.Errorf("bad month %q", m)
		}
		month = m + "-01"
	}
	c, err := aext.NewQuiet()
	if err != nil {
		return nil, err
	}
	if !c.HasSession() {
		return QuotaReply{NeedLogin: true}, nil
	}
	prev := dates.PrevMonthStart(month)
	start := time.Now()
	d, err := quota.Fetch(c, prev, dates.MonthEnd(month))
	if errors.Is(err, aext.ErrNoSession) {
		return QuotaReply{NeedLogin: true}, nil
	}
	if err != nil {
		return nil, err
	}
	reply := QuotaReply{
		Today:       now,
		HoursPerDay: settings.HoursPerDay(),
		WarnHours:   settings.QuotaWarnHours(),
		Country:     settings.WorkingDaysCountry,
		Months:      []quota.Month{quota.ComputeMonth(month, now, d), quota.ComputeMonth(prev, now, d)},
		Warnings:    d.Warnings,
	}
	if settings.Debug {
		var tz any = "auto"
		if hours, ok := settings.TZOffsetHours(); ok {
			tz = hours
		}
		reply.Debug = &QuotaDebug{
			Now:           dates.Now().Format(time.RFC3339),
			Device:        time.Now().Format(time.RFC3339 + " MST"),
			TZOffset:      tz,
			IgnoredStatus: settings.IgnoredLeaveStatuses,
			Took:          time.Since(start).Round(time.Millisecond).String(),
			Raw:           d.Raw,
			WorkingDays:   d.WorkingDays,
			LeaveDays:     d.LeaveDays,
			OffDays:       d.OffDays,
			HoursByDay:    d.HoursByDay,
		}
	}
	return reply, nil
}

// CalendarsReply is the googleCalendars API's result.
type CalendarsReply struct {
	NotConfigured bool           `json:"notConfigured,omitempty"` // no OAuth client: nothing else is set
	NeedLogin     bool           `json:"needLogin,omitempty"`     // no valid Google login: nothing else is set
	Email         string         `json:"email,omitempty"`
	NoCalendar    bool           `json:"noCalendar,omitempty"` // logged in without Calendar access
	Calendars     []CalendarInfo `json:"calendars,omitempty"`
}

// CalendarInfo is a calendar as the widget shows it.
type CalendarInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color,omitempty"`
	Access  string `json:"access"`
	Primary bool   `json:"primary,omitempty"`
	Hidden  bool   `json:"hidden,omitempty"`
}

func googleCalendarsAPI(map[string]any) (any, error) {
	c, err := google.NewQuiet()
	if errors.Is(err, google.ErrNotConfigured) {
		return CalendarsReply{NotConfigured: true}, nil
	}
	if err != nil {
		return nil, err
	}
	if !c.HasLogin() {
		return CalendarsReply{NeedLogin: true}, nil
	}
	if !c.Granted(google.CalendarScope) {
		return CalendarsReply{Email: c.Email(), NoCalendar: true}, nil
	}
	list, err := c.Calendars()
	if errors.Is(err, google.ErrNoLogin) {
		return CalendarsReply{NeedLogin: true}, nil
	}
	if err != nil {
		return nil, err
	}
	reply := CalendarsReply{Email: c.Email(), Calendars: []CalendarInfo{}}
	for _, cal := range list {
		reply.Calendars = append(reply.Calendars, CalendarInfo{ID: cal.ID, Name: cal.Name(), Color: cal.Color, Access: cal.AccessRole,
			Primary: cal.Primary, Hidden: cal.Hidden})
	}
	return reply, nil
}

// NowReply is the googleNow API's result: what is on now in one calendar, and what comes next.
type NowReply struct {
	NotConfigured bool   `json:"notConfigured,omitempty"` // no OAuth client: nothing else is set
	NeedLogin     bool   `json:"needLogin,omitempty"`     // no valid Google login: nothing else is set
	Email         string `json:"email,omitempty"`
	NoCalendar    bool   `json:"noCalendar,omitempty"` // logged in without Calendar access
	// Missing: the calendar is not in the account's calendar list (any more).
	Missing  bool         `json:"missing,omitempty"`
	Calendar CalendarInfo `json:"calendar"`
	Now      string       `json:"now,omitempty"` // RFC 3339, when the events were read
	Events   []EventInfo  `json:"events"`
	// Upcoming: the events starting later, until the end of the upcomingDays-th working day after today.
	Upcoming []EventInfo `json:"upcoming"`
	Until    string      `json:"until,omitempty"` // YYYY-MM-DD, the last day Upcoming covers
}

// EventInfo is an event on now, as the widget shows it.
type EventInfo struct {
	Title     string `json:"title"`
	Start     string `json:"start"` // RFC 3339, or YYYY-MM-DD when AllDay
	End       string `json:"end"`   // exclusive: an all-day event ends on the day after
	AllDay    bool   `json:"allDay,omitempty"`
	Free      bool   `json:"free,omitempty"`      // shown as free (transparent)
	Tentative bool   `json:"tentative,omitempty"` // not confirmed yet
}

// googleNowAPI gives the events on now in args.calendar (a calendar ID from googleCalendars),
// leaving out those the user declined.
func googleNowAPI(args map[string]any) (any, error) {
	id, _ := args["calendar"].(string)
	if id == "" {
		return nil, errors.New("no calendar given")
	}
	c, err := google.NewQuiet()
	if errors.Is(err, google.ErrNotConfigured) {
		return NowReply{NotConfigured: true}, nil
	}
	if err != nil {
		return nil, err
	}
	if !c.HasLogin() {
		return NowReply{NeedLogin: true}, nil
	}
	if !c.Granted(google.CalendarScope) {
		return NowReply{Email: c.Email(), NoCalendar: true}, nil
	}
	list, err := c.Calendars()
	if errors.Is(err, google.ErrNoLogin) {
		return NowReply{NeedLogin: true}, nil
	}
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(list, func(cal google.Calendar) bool { return cal.ID == id })
	if i < 0 {
		return NowReply{Email: c.Email(), Missing: true, Events: []EventInfo{}}, nil
	}
	cal := list[i]
	now := time.Now()
	last := upcomingUntil(dates.Today())
	events, err := c.Events(id, now, dates.DayStart(dates.AddDays(last, 1)))
	if errors.Is(err, google.ErrNoLogin) {
		return NowReply{NeedLogin: true}, nil
	}
	if err != nil {
		return nil, err
	}
	reply := NowReply{Email: c.Email(), Now: now.Format(time.RFC3339), Events: []EventInfo{}, Upcoming: []EventInfo{}, Until: last,
		Calendar: CalendarInfo{ID: cal.ID, Name: cal.Name(), Color: cal.Color, Access: cal.AccessRole, Primary: cal.Primary, Hidden: cal.Hidden}}
	for _, e := range events {
		if e.Declined() {
			continue
		}
		info := EventInfo{Title: e.Summary, Free: e.Transparency == "transparent", Tentative: e.Status == "tentative"}
		var start time.Time
		if e.Start.DateTime != "" {
			info.Start, info.End = e.Start.DateTime, e.End.DateTime
			start, _ = time.Parse(time.RFC3339, e.Start.DateTime)
		} else {
			info.Start, info.End, info.AllDay = e.Start.Date, e.End.Date, true
			start = dates.DayStart(e.Start.Date)
		}
		if start.After(now) {
			reply.Upcoming = append(reply.Upcoming, info)
		} else {
			reply.Events = append(reply.Events, info)
		}
	}
	return reply, nil
}

// upcomingDays is how many working days after today the calendar widget looks ahead.
const upcomingDays = 3

// upcomingUntil is the upcomingDays-th working day after today: by the AEXT working-days calendar
// when there is a session, else counting Monday to Friday. Kept for the day, as it is asked each minute.
var upcoming struct {
	sync.Mutex
	today, until string
}

func upcomingUntil(today string) string {
	upcoming.Lock()
	defer upcoming.Unlock()
	if upcoming.today == today {
		return upcoming.until
	}
	until, fromAEXT := nthWorkingDay(today, upcomingDays, aextWorkingDays(today))
	if fromAEXT {
		upcoming.today, upcoming.until = today, until
	}
	return until
}

// aextWorkingDays is the working days of the next few weeks after today, or nil without a session.
func aextWorkingDays(today string) map[string]bool {
	c, err := aext.NewQuiet()
	if err != nil || !c.HasSession() {
		return nil
	}
	days, err := c.WorkingDays(dates.AddDays(today, 1), dates.AddDays(today, 21))
	if err != nil {
		return nil
	}
	working := map[string]bool{}
	for _, d := range days {
		working[d.Date] = *d.IsWorkingDay
	}
	return working
}

// nthWorkingDay is the n-th working day after today, by working (a day missing from it is counted
// Monday to Friday); true when working had every day it looked at.
func nthWorkingDay(today string, n int, working map[string]bool) (string, bool) {
	day, complete := today, working != nil
	for n > 0 {
		day = dates.AddDays(day, 1)
		isWorking, ok := working[day]
		if !ok {
			complete = false
			wd := dates.DayStart(day).Weekday()
			isWorking = wd != time.Saturday && wd != time.Sunday
		}
		if isWorking {
			n--
		}
	}
	return day, complete
}
