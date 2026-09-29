// Package shortcut adds aex to the OS app launcher and removes it again: a Start menu shortcut on
// Windows, an app in ~/Applications on macOS, a .desktop entry on Linux.
package shortcut

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"aex/internal/settings"
	"aex/internal/ui"
)

// ErrUnsupported is returned on systems without a known app launcher.
var ErrUnsupported = errors.New("adding aex to the app launcher is not supported on this system")

// askedSetting is saved to app settings once the first-launch question was answered.
const askedSetting = "SHORTCUT_ASKED"

// Supported reports whether this system has a launcher to add aex to.
func Supported() bool { return supported }

// Where names the launcher, e.g. "Start menu".
func Where() string { return where }

// Path is the shortcut file (or app bundle) the launcher shows.
func Path() (string, error) { return path() }

// Exists reports whether the shortcut is there.
func Exists() bool {
	p, err := path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Create adds (or replaces) the shortcut, pointing at this exe and passing on --data-dir if given.
func Create() error {
	if !supported {
		return ErrUnsupported
	}
	p, err := path()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	var args []string
	if settings.DataDirFromArg {
		args = []string{"--data-dir", settings.DataDir}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return create(p, exe, args)
}

// Remove deletes the shortcut; no error when it is not there.
func Remove() error {
	if !supported {
		return ErrUnsupported
	}
	p, err := path()
	if err != nil {
		return err
	}
	return remove(p)
}

// OfferOnce asks, the first time the menu opens, whether to add aex to the launcher. The answer is
// remembered in app settings; configure adds or removes the shortcut later. Failures only warn.
func OfferOnce() {
	if !supported || settings.Get(askedSetting) != "" || !ui.IsInteractive() {
		return
	}
	if !Exists() {
		add, err := ui.Confirm(fmt.Sprintf("Add aex to the %s? (configure can add or remove it later)", where), true)
		if err != nil {
			return
		}
		if add {
			if err := Create(); err != nil {
				ui.Warn("could not add aex to the %s: %v", where, err)
				return
			}
			p, _ := path()
			fmt.Println(ui.Out.Green("Added to the " + where + ": " + p))
		}
		fmt.Println()
	}
	yes := "yes"
	if err := settings.SaveAppSettings([]settings.Change{{Name: askedSetting, Value: &yes}}); err != nil {
		ui.Warn("could not save %s: %v", askedSetting, err)
		return
	}
	settings.Set(askedSetting, yes)
}
