package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Prompts go to stderr so stdout stays clean.

// One reader for the whole process so lines typed or piped ahead of a prompt are not lost.
var stdin = bufio.NewReader(os.Stdin)

func IsInteractive() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

func AssertInteractive(what string) error {
	if !IsInteractive() {
		return fmt.Errorf("%s needs an interactive terminal", what)
	}
	return nil
}

// Ask prints question and returns the next line of input, trimmed.
func Ask(question string) (string, error) {
	fmt.Fprint(os.Stderr, question)
	line, err := stdin.ReadString('\n')
	if err != nil && (line == "" || !errors.Is(err, io.EOF)) {
		if errors.Is(err, io.EOF) {
			return "", errors.New("input closed")
		}
		return "", err
	}
	// PowerShell piping into an exe may send a UTF-8 byte order mark first.
	return strings.TrimSpace(strings.TrimPrefix(line, "\xef\xbb\xbf")), nil
}

// Confirm asks a yes/no question; an empty answer means defaultYes.
func Confirm(question string, defaultYes bool) (bool, error) {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	a, err := Ask(fmt.Sprintf("%s %s ", question, Err.Dim(hint)))
	if err != nil {
		return false, err
	}
	a = strings.ToLower(a)
	if a == "" {
		return defaultYes, nil
	}
	return a == "y" || a == "yes", nil
}

// PromptSecret reads a line from the terminal without echoing it.
func PromptSecret(question string) (string, error) {
	fd := int(os.Stdin.Fd())
	fmt.Fprint(os.Stderr, question)
	old, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer func() {
		term.Restore(fd, old)
		fmt.Fprintln(os.Stderr)
	}()

	var input []rune
	for {
		r, _, err := stdin.ReadRune()
		if err != nil {
			return "", err
		}
		switch {
		case r == '\r' || r == '\n':
			// A pasted "\r\n" would otherwise leave "\n" behind as an empty answer to the next prompt.
			// Only looks at bytes already read: Peek would wait for more input.
			if r == '\r' && stdin.Buffered() > 0 {
				if b, _ := stdin.Peek(1); b[0] == '\n' {
					stdin.ReadByte()
				}
			}
			return string(input), nil
		case r == 3: // Ctrl+C
			return "", errors.New("cancelled")
		case r == 8 || r == 127: // Backspace
			if len(input) > 0 {
				input = input[:len(input)-1]
			}
		case r >= ' ':
			input = append(input, r)
		}
	}
}

// WaitKey returns after any key press (after Enter when stdin is not a terminal).
func WaitKey() {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		stdin.ReadString('\n')
		return
	}
	defer term.Restore(fd, old)
	stdin.ReadByte()
	// The rest of an escape sequence (arrow keys) would otherwise reach the next prompt.
	stdin.Discard(stdin.Buffered())
}
