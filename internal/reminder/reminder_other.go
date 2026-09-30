//go:build !windows

package reminder

import (
	"os/exec"
	"runtime"
)

// Elsewhere a reminder is a system notification with a sound: the system keeps it, so done closes
// at once.
func open(r Reminder) (<-chan struct{}, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		// The texts go as arguments, never into the script.
		cmd = exec.Command("osascript", "-e", "on run argv", "-e",
			`display notification (item 2 of argv) with title (item 1 of argv) sound name "Glass"`, "-e", "end run",
			r.Heading(), r.Message)
	} else {
		cmd = exec.Command("notify-send", "--urgency=critical", "--app-name=aex", "--", r.Heading(), r.Message)
	}
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	close(done)
	return done, nil
}
