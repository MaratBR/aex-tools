package ui

import "io"

// Remote, when set, answers every prompt instead of the terminal: the GUI sets it, and tools keep
// calling Input, Confirm, Choose, PickTool and WaitKey as they do on a console.
var Remote Prompter

// Prompter asks questions somewhere other than the terminal. Its methods block until answered;
// Input gets the trimmed Field title and runs Validate itself (asking again on an error).
type Prompter interface {
	Input(f Field) (string, error)
	Confirm(question string, defaultYes bool) (bool, error)
	Choose(title string, options []Option) (string, error)
	PickTool(title string, tools []Option) (string, error)
	WaitKey()
	ClearScreen()
}

// TerminalHost is a Prompter that can also show a program running in a pseudo-console
// (internal/pty) as a terminal: the window.
type TerminalHost interface {
	// OpenTerminal shows a terminal in the run: what is typed there is written to input, and its
	// size changes (columns, rows) go to resize. The program's screen (VT, UTF-8) is written to the
	// result, closed once the program exited.
	OpenTerminal(input io.Writer, resize func(cols, rows int)) io.WriteCloser
}
