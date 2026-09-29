package ui

// Remote, when set, answers every prompt instead of the terminal: the GUI sets it, and tools keep
// calling Input, Confirm, Choose and WaitKey as they do on a console.
var Remote Prompter

// Prompter asks questions somewhere other than the terminal. Its methods block until answered;
// Input gets the trimmed Field title and runs Validate itself (asking again on an error).
type Prompter interface {
	Input(f Field) (string, error)
	Confirm(question string, defaultYes bool) (bool, error)
	Choose(title string, options []Option) (string, error)
	WaitKey()
	ClearScreen()
}
