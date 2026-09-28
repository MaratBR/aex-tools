// Package ui holds terminal helpers: ANSI colors, line prompts and raw key reads.
package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// Palette styles text with ANSI escapes, or passes it through when colors are off.
type Palette struct{ on bool }

func (p Palette) wrap(open, close int, s string) string {
	if !p.on {
		return s
	}
	return fmt.Sprintf("\x1b[%dm%s\x1b[%dm", open, s, close)
}

// On reports whether this palette styles text.
func (p Palette) On() bool { return p.on }

func (p Palette) Bold(s string) string   { return p.wrap(1, 22, s) }
func (p Palette) Dim(s string) string    { return p.wrap(2, 22, s) }
func (p Palette) Red(s string) string    { return p.wrap(31, 39, s) }
func (p Palette) Green(s string) string  { return p.wrap(32, 39, s) }
func (p Palette) Yellow(s string) string { return p.wrap(33, 39, s) }
func (p Palette) Cyan(s string) string   { return p.wrap(36, 39, s) }

// Out styles text written to stdout, Err text written to stderr. Both are off until Setup.
var Out, Err Palette

// Setup turns on ANSI handling in the console (Windows) and decides per stream whether to color:
// only on a terminal; NO_COLOR=1 disables, FORCE_COLOR=1 forces.
func Setup() {
	Out = Palette{on: colorEnabled(os.Stdout)}
	Err = Palette{on: colorEnabled(os.Stderr)}
}

func colorEnabled(f *os.File) bool {
	tty := term.IsTerminal(int(f.Fd()))
	vt := tty && enableVT(f)
	if force, ok := os.LookupEnv("FORCE_COLOR"); ok {
		return force != "0" && force != "false"
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return vt && os.Getenv("TERM") != "dumb"
}

// Warn prints "Warning: <message>" to stderr.
func Warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Err.Bold(Err.Yellow("▲ Warning:")), fmt.Sprintf(format, args...))
}
