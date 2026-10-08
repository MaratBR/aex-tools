package composer

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// AppWindow is the aex window, for the actions that act on it.
type AppWindow interface {
	// ShowHome shows the Home page.
	ShowHome() error
	// Minimise minimises the window.
	Minimise() error
}

// Window is the aex window while composed tools run in it (set by internal/webui); nil in a
// terminal, where the actions on it fail.
var Window AppWindow

var errNoWindow = errors.New("only in the aex window (not in a terminal)")

// HomeAction switches the aex window to Home.
type HomeAction struct{}

func (a *HomeAction) Kind() string     { return "home" }
func (a *HomeAction) Describe() string { return "Switch to Home" }
func (a *HomeAction) Check(c *Checker) {}

func (a *HomeAction) Run(r *Run) error {
	if Window == nil {
		return errNoWindow
	}
	return Window.ShowHome()
}

// MinimizeAction minimises the aex window.
type MinimizeAction struct{}

func (a *MinimizeAction) Kind() string     { return "minimize" }
func (a *MinimizeAction) Describe() string { return "Minimize aex" }
func (a *MinimizeAction) Check(c *Checker) {}

func (a *MinimizeAction) Run(r *Run) error {
	if Window == nil {
		return errNoWindow
	}
	return Window.Minimise()
}

// DelayAction waits.
type DelayAction struct {
	// Seconds to wait; fractions allowed.
	Seconds float64 `json:"seconds"`
}

func (a *DelayAction) Kind() string { return "delay" }

func (a *DelayAction) Describe() string { return "Wait " + seconds(a.Seconds) }

// seconds says a number of seconds plainly: "5 s", "1.5 s", "2 min".
func seconds(s float64) string {
	if s >= 60 && s == float64(int(s)) && int(s)%60 == 0 {
		return fmt.Sprintf("%d min", int(s)/60)
	}
	return strconv.FormatFloat(s, 'f', -1, 64) + " s"
}

func (a *DelayAction) Check(c *Checker) {
	if a.Seconds <= 0 {
		c.Error("seconds", "wait more than 0 seconds")
	} else if a.Seconds > 24*60*60 {
		c.Error("seconds", "wait a day at most")
	}
}

func (a *DelayAction) Run(r *Run) error {
	if !sleep(r.Ctx, time.Duration(a.Seconds*float64(second))) {
		return fmt.Errorf("stopped: %w", r.Ctx.Err())
	}
	return nil
}
