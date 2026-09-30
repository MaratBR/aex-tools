//go:build !windows

// Package proc has helpers for starting other programs.
package proc

import "os/exec"

// HideConsole does nothing: only Windows gives console programs a window.
func HideConsole(*exec.Cmd) {}

// HideConsoleIfNone does nothing: only Windows gives console programs a window.
func HideConsoleIfNone(*exec.Cmd) {}
