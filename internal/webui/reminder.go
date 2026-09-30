package webui

import (
	"fmt"

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

// Remind shows a reminder for a widget (sdk.js: aex.remind); it stays until dismissed.
func (a *App) Remind(title, message string) error {
	_, err := reminder.Open(reminder.Reminder{Title: title, Message: message})
	return err
}
