// Package proc has helpers for starting other programs.
package proc

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// HideConsole gives a console program a console of its own that is never shown, instead of sharing
// aex's (or, from the window, which has none, opening one): for programs whose output goes to pipes.
func HideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
