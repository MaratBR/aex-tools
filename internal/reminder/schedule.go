package reminder

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"aex/internal/dates"
	"aex/internal/settings"
)

// Scheduled is a reminder set up on the Settings page: shown at Time on each of Days, while the
// window is open.
type Scheduled struct {
	ID      string   `json:"id"`
	Title   string   `json:"title,omitempty"`
	Message string   `json:"message"`
	Time    string   `json:"time"` // 15:04, in the configured timezone (dates.Now)
	Days    []string `json:"days"` // of Week
	Off     bool     `json:"off,omitempty"`
	Urgent  bool     `json:"urgent,omitempty"` // extra urgent: see Reminder.Urgent
	// Last is the day it was last shown (2006-01-02), so it shows once a day.
	Last string `json:"last,omitempty"`
}

// Week are the day names Scheduled.Days takes, Monday first.
var Week = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// MaxScheduled is how many reminders can be set up.
const MaxScheduled = 100

// late is how long after its time a reminder still shows, when aex was not running or the device
// was asleep then.
const late = 30 * time.Minute

// scheduledFile holds the reminders set up, in the data folder.
func scheduledFile() string { return filepath.Join(settings.DataDir, "reminders.json") }

var scheduledMu sync.Mutex

type scheduledList struct {
	Reminders []Scheduled `json:"reminders"`
}

// LoadScheduled reads the reminders set up; none when there is no file yet.
func LoadScheduled() ([]Scheduled, error) {
	scheduledMu.Lock()
	defer scheduledMu.Unlock()
	return load()
}

func load() ([]Scheduled, error) {
	b, err := os.ReadFile(scheduledFile())
	if errors.Is(err, os.ErrNotExist) {
		return []Scheduled{}, nil
	}
	if err != nil {
		return nil, err
	}
	var l scheduledList
	// A file edited on Windows may start with a byte order mark.
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &l); err != nil {
		return nil, fmt.Errorf("reading %s: %w", scheduledFile(), err)
	}
	if l.Reminders == nil {
		l.Reminders = []Scheduled{}
	}
	return l.Reminders, nil
}

func save(list []Scheduled) error {
	b, err := json.MarshalIndent(scheduledList{list}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(settings.DataDir, 0o755); err != nil {
		return err
	}
	tmp := scheduledFile() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, scheduledFile())
}

// Check cleans s (as Reminder.Clean) and checks its time and days, which it puts in Week's order.
func (s Scheduled) Check() (Scheduled, error) {
	r, err := Reminder{Title: s.Title, Message: s.Message}.Clean()
	if err != nil {
		return s, err
	}
	s.Title, s.Message = r.Title, r.Message
	if _, err := time.Parse("15:04", s.Time); err != nil || len(s.Time) != 5 {
		return s, fmt.Errorf("the time %q is not HH:MM (24-hour)", s.Time)
	}
	var days []string
	for _, d := range Week {
		if slices.Contains(s.Days, d) {
			days = append(days, d)
		}
	}
	for _, d := range s.Days {
		if !slices.Contains(Week, d) {
			return s, fmt.Errorf("not a day: %q (one of %v)", d, Week)
		}
	}
	if len(days) == 0 {
		return s, errors.New("pick at least one day")
	}
	s.Days = days
	return s, nil
}

// SaveScheduled replaces the reminders set up with list, checking each. A reminder without an ID
// gets one. When a reminder is new, or its time or days changed, and its time today has passed
// already, it is not shown today: it starts from its next time.
func SaveScheduled(list []Scheduled) ([]Scheduled, error) {
	if len(list) > MaxScheduled {
		return nil, fmt.Errorf("%d reminders at most", MaxScheduled)
	}
	scheduledMu.Lock()
	defer scheduledMu.Unlock()
	old, err := load()
	if err != nil {
		return nil, err
	}
	now := dates.Now()
	out := make([]Scheduled, 0, len(list))
	for i, s := range list {
		s, err := s.Check()
		if err != nil {
			return nil, fmt.Errorf("reminder %d: %w", i+1, err)
		}
		if s.ID == "" {
			s.ID = newID()
		}
		s.Last = ""
		i := slices.IndexFunc(old, func(o Scheduled) bool { return o.ID == s.ID })
		if i >= 0 && old[i].Time == s.Time && slices.Equal(old[i].Days, s.Days) {
			s.Last = old[i].Last
		} else if due, ok := s.dueToday(now); ok && !now.Before(due) {
			s.Last = now.Format(time.DateOnly)
		}
		out = append(out, s)
	}
	return out, save(out)
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// dueToday is when s shows today (in now's timezone), if today is one of its days.
func (s Scheduled) dueToday(now time.Time) (time.Time, bool) {
	if !slices.Contains(s.Days, Week[(int(now.Weekday())+6)%7]) {
		return time.Time{}, false
	}
	t, err := time.Parse("15:04", s.Time)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()), true
}

// due reports whether s shows now: on for one of its days, its time passed less than late ago,
// not shown today yet.
func (s Scheduled) due(now time.Time) bool {
	at, ok := s.dueToday(now)
	return ok && !s.Off && s.Last != now.Format(time.DateOnly) && !now.Before(at) && now.Sub(at) < late
}

// RunSchedule shows the reminders set up when they are due, until stop closes. It reads the file
// each time, so changes apply at once.
func RunSchedule(stop <-chan struct{}) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		showDue(dates.Now())
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

// lastErr is the last error showDue printed.
var lastErr string

func showDue(now time.Time) {
	scheduledMu.Lock()
	list, err := load()
	var shown []Scheduled
	if err == nil {
		for i, s := range list {
			if s.due(now) {
				list[i].Last = now.Format(time.DateOnly)
				shown = append(shown, s)
			}
		}
		if len(shown) > 0 {
			err = save(list)
		}
	}
	scheduledMu.Unlock()
	if err != nil {
		// Once, not every tick, until it changes.
		if msg := err.Error(); msg != lastErr {
			lastErr = msg
			fmt.Fprintf(os.Stderr, "reminders: %v\n", err)
		}
		return
	}
	lastErr = ""
	for _, s := range shown {
		if _, err := Open(Reminder{Title: s.Title, Message: s.Message, Urgent: s.Urgent}); err != nil {
			fmt.Fprintf(os.Stderr, "reminder %q not shown: %v\n", s.Message, err)
		}
	}
}
