// Package reminder shows reminders: a message on top of every window on every screen, with a soft
// chime, until it is dismissed. Tools, plugins and widgets trigger them (see Show, Mark and the
// widgets' aex.remind).
package reminder

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Reminder is what a reminder shows.
type Reminder struct {
	Title   string `json:"title,omitempty"` // "Reminder" when empty
	Message string `json:"message"`
	// Urgent (extra urgent) reminders chime twice when they show, and twice again every
	// urgentEvery until closed or urgentFor passes; the card is marked red.
	Urgent bool `json:"urgent,omitempty"`
}

// How an urgent reminder keeps chiming: twice (chimeGap apart, the chime being a little shorter)
// every urgentEvery, for urgentFor. Variables for the tests.
var (
	chimeGap    = 2 * time.Second
	urgentEvery = 30 * time.Second
	urgentFor   = 10 * time.Minute
)

// Longest title and message shown, in characters; longer ones are cut.
const (
	MaxTitle   = 100
	MaxMessage = 1000
)

// EnvVar is set to "window" in the window's process, so tools and plugins it runs (which inherit
// it) hand their reminders to the window as marks in their output instead of showing them.
const EnvVar = "AEX_REMINDERS"

// MarkName is the name of the output mark that shows a reminder: "\x1b]aex-remind;<payload>\x07",
// the payload being the reminder as JSON in base64 (see Mark). The window's output stream takes
// such marks out of what is printed.
const MarkName = "remind"

// Clean trims r, cuts it to MaxTitle and MaxMessage, and fails when there is no message.
func (r Reminder) Clean() (Reminder, error) {
	r.Title = cut(strings.TrimSpace(r.Title), MaxTitle)
	r.Message = cut(strings.TrimSpace(r.Message), MaxMessage)
	if r.Message == "" {
		return r, errors.New("a reminder needs a message")
	}
	return r, nil
}

// Heading is the title shown: Title, or "Reminder" when it has none.
func (r Reminder) Heading() string {
	if r.Title == "" {
		return "Reminder"
	}
	return r.Title
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// Show shows r. Run by the window (a tool, or a plugin it started) it hands r to the window, which
// keeps showing it after the tool exits, and returns at once. Otherwise (in a terminal) it shows r
// itself and waits until it is dismissed, since the reminder goes away with the process.
func Show(r Reminder) error {
	r, err := r.Clean()
	if err != nil {
		return err
	}
	if InWindow() {
		_, err := fmt.Fprint(os.Stdout, Mark(r))
		return err
	}
	done, err := Open(r)
	if err != nil {
		return err
	}
	<-done
	return nil
}

// InWindow reports whether Show hands reminders to the window (it runs this process), rather than
// showing them itself and waiting.
func InWindow() bool { return os.Getenv(EnvVar) == "window" }

// Mark is the output mark that has the window show r.
func Mark(r Reminder) string {
	b, _ := json.Marshal(r)
	return "\x1b]aex-" + MarkName + ";" + base64.StdEncoding.EncodeToString(b) + "\x07"
}

// Parse reads the payload of a mark (see Mark).
func Parse(payload string) (Reminder, error) {
	var r Reminder
	b, err := base64.StdEncoding.DecodeString(payload)
	if err == nil {
		err = json.Unmarshal(b, &r)
	}
	if err != nil {
		return r, fmt.Errorf("reading a reminder: %w", err)
	}
	return r.Clean()
}

// Open shows r and returns at once; done closes once it is dismissed. On Windows it is a card on
// every screen, on top of all windows, with a chime; elsewhere a system notification (done closes
// at once, the system keeps it).
func Open(r Reminder) (done <-chan struct{}, err error) {
	r, err = r.Clean()
	if err != nil {
		return nil, err
	}
	return open(r)
}
