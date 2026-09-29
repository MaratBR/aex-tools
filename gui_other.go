//go:build !windows

package main

import (
	"os"

	"golang.org/x/term"
)

// hideOwnConsole: only Windows gives a GUI started from a launcher a console window.
func hideOwnConsole() {}

// saveConsole records the state of the terminal the window was started from and returns a
// function that sets it back, whatever a tool or plugin did to it meanwhile.
func saveConsole() (restore func()) {
	fd := int(os.Stdin.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		return func() {}
	}
	return func() { term.Restore(fd, state) }
}
