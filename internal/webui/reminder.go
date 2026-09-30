package webui

import (
	"fmt"

	"aex/internal/dates"
	"aex/internal/reminder"
)

// mark handles a mark in the output other than sync: a reminder (reminder.Mark) is shown, and
// stays after the tool that printed it exits.
func (a *App) mark(name, payload string) {
	if name != reminder.MarkName {
		return
	}
	r, err := reminder.Parse(payload)
	if err == nil {
		_, err = reminder.Open(r)
	}
	if err != nil {
		fmt.Fprintf(a.out.w, "reminder not shown: %v\n", err)
	}
}

// Remind shows a reminder for a widget (sdk.js: aex.remind) or the Settings page's Show now; it
// stays until dismissed.
func (a *App) Remind(title, message string, urgent bool) error {
	_, err := reminder.Open(reminder.Reminder{Title: title, Message: message, Urgent: urgent})
	return err
}

// RemindersInfo is the Settings page's Reminders: the reminders set up and the timezone their
// times are in (TZ_OFFSET_HOURS).
type RemindersInfo struct {
	Reminders []reminder.Scheduled `json:"reminders"`
	TZ        string               `json:"tz"`
}

// Reminders lists the reminders set up.
func (a *App) Reminders() (RemindersInfo, error) {
	list, err := reminder.LoadScheduled()
	return RemindersInfo{Reminders: list, TZ: dates.TZLabel()}, err
}

// SaveReminders replaces the reminders set up, checking each (reminder.SaveScheduled), and gives
// them back as saved.
func (a *App) SaveReminders(list []reminder.Scheduled) ([]reminder.Scheduled, error) {
	return reminder.SaveScheduled(list)
}
