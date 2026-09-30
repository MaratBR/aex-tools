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

// HideConsoleIfNone hides the program's console (see HideConsole) when aex has none to share, as in
// the window started from the Start menu or Explorer, where Windows would open one for it.
func HideConsoleIfNone(cmd *exec.Cmd) {
	if w, _, _ := procGetConsoleWindow.Call(); w == 0 {
		HideConsole(cmd)
	}
}

var procGetConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
