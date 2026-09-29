// Package autostart starts aex when the user logs in to the OS, on the days picked: a value in the
// current user's Run registry key on Windows, a LaunchAgent on macOS, an XDG autostart entry on
// Linux. The entry runs aex with --autostart, which exits at once on a day not picked (Skip).
package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"aex/internal/dates"
	"aex/internal/settings"
)

// Flag is what the entry passes aex: main exits at once when Skip says so.
const Flag = "--autostart"

// daysSetting holds the days picked, in app settings: e.g. "mon,tue,wed,thu,fri". Not set means
// Workdays.
const daysSetting = "AUTOSTART_DAYS"

// ErrUnsupported is returned on systems aex does not know how to start with.
var ErrUnsupported = errors.New("starting aex when you log in is not supported on this system")

// Week lists the day names Days and SetDays use, Monday first.
var Week = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// Workdays are the days aex starts on until others are picked.
var Workdays = Week[:5]

// Supported reports whether this system has a way to start aex on login that aex knows.
func Supported() bool { return supported }

// Enabled reports whether the entry is there.
func Enabled() bool { return supported && enabled() }

// Enable adds (or replaces) the entry, pointing at this exe and passing on --data-dir if given.
func Enable() error {
	if !supported {
		return ErrUnsupported
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
	return enable(exe, append(args, Flag))
}

// Disable removes the entry; no error when it is not there.
func Disable() error {
	if !supported {
		return ErrUnsupported
	}
	return disable()
}

// Days are the days aex starts on, in Week's order.
func Days() []string {
	days, err := ParseDays(settings.Get(daysSetting))
	if err != nil || len(days) == 0 {
		return slices.Clone(Workdays)
	}
	return days
}

// SetDays saves the days aex starts on (at least one, names as ParseDays takes them).
func SetDays(days []string) error {
	days, err := ParseDays(strings.Join(days, ","))
	if err != nil {
		return err
	}
	if len(days) == 0 {
		return errors.New("pick at least one day")
	}
	value := strings.Join(days, ",")
	if err := settings.SaveAppSettings([]settings.Change{{Name: daysSetting, Value: &value}}); err != nil {
		return err
	}
	settings.Set(daysSetting, value)
	return nil
}

// ParseDays reads day names from Week, comma separated (any case, spaces allowed), into Week's
// order without repeats. "workdays" and "every-day" stand for those days.
func ParseDays(text string) ([]string, error) {
	picked := map[string]bool{}
	for _, word := range strings.Split(text, ",") {
		word = strings.ToLower(strings.TrimSpace(word))
		switch {
		case word == "":
		case word == "workdays":
			for _, d := range Workdays {
				picked[d] = true
			}
		case word == "every-day":
			for _, d := range Week {
				picked[d] = true
			}
		case slices.Contains(Week, word):
			picked[word] = true
		default:
			return nil, fmt.Errorf("not a day: %q (use %s, workdays or every-day)", word, strings.Join(Week, ", "))
		}
	}
	var days []string
	for _, d := range Week {
		if picked[d] {
			days = append(days, d)
		}
	}
	return days, nil
}

// Skip reports whether aex, started with Flag, should exit at once: today is not one of Days.
func Skip() bool { return !slices.Contains(Days(), dayName(dates.Now().Weekday())) }

// dayName is a weekday's name in Week.
func dayName(d time.Weekday) string { return Week[(int(d)+6)%7] }
