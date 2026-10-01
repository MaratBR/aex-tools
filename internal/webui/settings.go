package webui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"aex/internal/aext"
	"aex/internal/autostart"
	"aex/internal/dates"
	"aex/internal/settings"
	"aex/internal/shortcut"
	"aex/internal/tool"
	"aex/internal/tools/configure"
)

// The settings form: what the configure tool asks for on a terminal, as one form in the window.

// SettingField is one setting in the form.
type SettingField struct {
	Name    string `json:"name"`
	Hint    string `json:"hint"`
	Rule    string `json:"rule,omitempty"`
	Default string `json:"default,omitempty"`
	Secret  bool   `json:"secret,omitempty"`
	// Value is the current value; for a secret only its masked ends, in Masked.
	Value  string `json:"value,omitempty"`
	Masked string `json:"masked,omitempty"`
	// Source is where the value comes from when not app settings (.env, environment variables).
	Source string `json:"source,omitempty"`
	// Overridden lists fallback sources that also set it, which app settings override.
	Overridden []string `json:"overridden,omitempty"`
	// Auto, for a setting that can be auto, is what auto means now (TZ_OFFSET_HOURS: the device's offset).
	Auto string `json:"auto,omitempty"`
	// Warning is something off about the value in effect.
	Warning string `json:"warning,omitempty"`
	// Widget is the widget it is for, shown under Settings > Widgets; "" for General.
	Widget string `json:"widget,omitempty"`
}

// SettingsForm is everything the form shows.
type SettingsForm struct {
	File     string         `json:"file"`
	Fields   []SettingField `json:"fields"`
	Shortcut *ShortcutInfo  `json:"shortcut,omitempty"` // nil when the OS has no app launcher aex supports
	// Autostart is nil when aex cannot start with this OS.
	Autostart *AutostartInfo `json:"autostart,omitempty"`
}

// ShortcutInfo is aex's entry in the OS app launcher.
type ShortcutInfo struct {
	Where  string `json:"where"`
	Exists bool   `json:"exists"`
}

// AutostartInfo is whether aex starts when the user logs in, and on which days (autostart.Week).
type AutostartInfo struct {
	Enabled bool     `json:"enabled"`
	Days    []string `json:"days"`
	// Suggested is whether onboarding turns it on: in release builds, not dev builds (rebuilt in
	// place, run from the repo).
	Suggested bool `json:"suggested"`
}

// SaveResult says how saving the form went. When Errors is set, nothing was saved.
type SaveResult struct {
	Errors map[string]string `json:"errors,omitempty"` // by setting name
	Saved  []string          `json:"saved,omitempty"`  // what was done, one line each
	// OldSession is set when AEXT_EMAIL changed while an AEXT session is cached: it belongs to the
	// old email, and the form offers to wipe it.
	OldSession bool `json:"oldSession,omitempty"`
}

var errBusy = errors.New("a tool is running: wait for it to finish")

// idle fails while a tool runs, which may be reading or changing the settings too.
func (a *App) idle() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return errBusy
	}
	return nil
}

// changed tells main the settings changed (to check the logins again).
func (a *App) changed() {
	if a.host.Changed != nil {
		a.host.Changed()
	}
}

// Settings gives the settings form.
func (a *App) Settings() SettingsForm {
	form := SettingsForm{File: settings.ConfigEnvFile}
	for _, s := range settings.All {
		f := SettingField{Name: s.Name, Hint: s.Hint, Rule: s.Rule, Default: s.Default, Secret: s.Secret,
			Overridden: configure.Overrides(s.Name), Widget: s.Widget}
		if v := settings.Get(s.Name); v != "" {
			if s.Secret {
				f.Masked = settings.Mask(v)
			} else {
				f.Value = v
			}
			if !settings.InAppSettings(s.Name) {
				f.Source = settings.Source(s.Name)
			}
		}
		if s.Name == "TZ_OFFSET_HOURS" {
			f.Auto = dates.DeviceTZLabel()
			if dates.TZMismatch() {
				f.Warning = fmt.Sprintf("This device is on %s, but dates are computed in %s. Pick auto to follow the device.",
					f.Auto, dates.TZLabel())
			}
		}
		form.Fields = append(form.Fields, f)
	}
	if shortcut.Supported() {
		form.Shortcut = &ShortcutInfo{Where: shortcut.Where(), Exists: shortcut.Exists()}
	}
	form.Autostart = a.Autostart()
	return form
}

// Autostart gives whether aex starts when the user logs in; nil when it cannot on this OS.
func (a *App) Autostart() *AutostartInfo {
	if !autostart.Supported() {
		return nil
	}
	return &AutostartInfo{Enabled: autostart.Enabled(), Days: autostart.Days(), Suggested: !settings.IsDev}
}

// SetAutostart saves the days aex starts on and makes it start when the user logs in (enabled) or
// not.
func (a *App) SetAutostart(enabled bool, days []string) error {
	if err := autostart.SetDays(days); err != nil {
		return err
	}
	if !enabled {
		return autostart.Disable()
	}
	return autostart.Enable()
}

// SaveSettings saves the changed settings, by name: an empty value removes a setting from app
// settings (a setting with a default goes back to it). All are checked before any is saved.
func (a *App) SaveSettings(changes map[string]string) (SaveResult, error) {
	if err := a.idle(); err != nil {
		return SaveResult{}, err
	}
	var res SaveResult
	names := make([]string, 0, len(changes))
	for name, value := range changes {
		value = strings.TrimSpace(value)
		changes[name] = value
		names = append(names, name)
		problem := ""
		switch {
		case value == "-": // how the terminal removes a setting: here that is an empty field
			problem = "clear the field to remove the setting"
		case value != "":
			problem = settings.Problem(name, value)
		}
		if problem != "" {
			if res.Errors == nil {
				res.Errors = map[string]string{}
			}
			res.Errors[name] = problem
		}
	}
	if res.Errors != nil {
		return res, nil
	}
	// In the form's order.
	order := func(name string) int {
		return slices.IndexFunc(settings.All, func(s settings.Setting) bool { return s.Name == name })
	}
	slices.SortFunc(names, func(x, y string) int { return order(x) - order(y) })
	defer a.changed()
	for _, name := range names {
		value := changes[name]
		if value == "" {
			value = "-"
		}
		before := settings.Get(name)
		msg, err := configure.Save(name, value)
		if err != nil {
			return res, err
		}
		res.Saved = append(res.Saved, msg)
		if name == "AEXT_EMAIL" && before != "" && before != settings.Get(name) {
			if c, err := aext.New(); err == nil && c.HasSession() {
				res.OldSession = true
			}
		}
	}
	return res, nil
}

// WipeSession wipes the cached AEXT session: the next AEXT tool asks for a login code.
func (a *App) WipeSession() error {
	if err := a.idle(); err != nil {
		return err
	}
	defer a.changed()
	c, err := aext.New()
	if err != nil {
		return err
	}
	return c.WipeSession()
}

// SetShortcut adds aex to the OS app launcher (add) or removes it.
func (a *App) SetShortcut(add bool) error {
	if add {
		return shortcut.Create()
	}
	if !shortcut.Exists() {
		return nil
	}
	return shortcut.Remove()
}

// WipeList lists what Wipe(all) deletes.
func (a *App) WipeList(all bool) ([]string, error) {
	what := settings.SettingsToWipe()
	if all {
		data, err := settings.DataToWipe()
		if err != nil {
			return nil, err
		}
		what = append(what, data...)
	}
	return what, nil
}

// Wipe deletes all settings, or (all) everything in the data folder but the plugins. The form
// has the user type CONFIRM first.
func (a *App) Wipe(all bool) error {
	if err := a.idle(); err != nil {
		return err
	}
	defer a.changed()
	if all {
		return settings.WipeAll()
	}
	return settings.WipeSettings()
}

// Open opens a folder the header links to (InfoLine.Open) in the file manager.
func (a *App) Open(path string) error {
	for _, l := range a.host.Header(false) {
		if l.Open != "" && l.Open == path {
			return tool.OpenFolder(path)
		}
	}
	return errors.New("not a folder the window links to: " + path)
}
