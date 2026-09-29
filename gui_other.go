//go:build !windows

package main

// hideOwnConsole: only Windows gives a GUI started from a launcher a console window.
func hideOwnConsole() {}
