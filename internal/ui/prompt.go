package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Prompts go to stderr so stdout stays clean. Line prompts below are used when the interactive
// ones (form.go) are off.

// One reader for the whole process so lines typed or piped ahead of a prompt are not lost.
var stdin = bufio.NewReader(os.Stdin)

func IsInteractive() bool { return Remote != nil || term.IsTerminal(int(os.Stdin.Fd())) }

func AssertInteractive(what string) error {
	if !IsInteractive() {
		return fmt.Errorf("%s needs an interactive terminal", what)
	}
	return nil
}

// readLine prints prompt and returns the next line of input, trimmed.
func readLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
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

func lineInput(f Field, check func(string) error) (string, error) {
	prompt := f.Title + ": "
	if f.Title == "" {
		prompt = "> "
	}
	if f.Description != "" {
		fmt.Fprintln(os.Stderr, Err.Bold(f.Title))
		for _, l := range strings.Split(f.Description, "\n") {
			fmt.Fprintln(os.Stderr, Err.Dim("  "+l))
		}
		prompt = "  > "
	}
	for {
		read := readLine
		if f.Secret {
			read = lineSecret
		}
		value, err := read(prompt)
		if err != nil {
			return "", err
		}
		if err := check(value); err != nil {
			fmt.Fprintln(os.Stderr, "  "+Err.Red(err.Error()))
			continue
		}
		return value, nil
	}
}

func lineConfirm(question string, defaultYes bool) (bool, error) {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	a, err := readLine(fmt.Sprintf("%s %s ", question, Err.Dim(hint)))
	if err != nil {
		return false, err
	}
	a = strings.ToLower(a)
	if a == "" {
		return defaultYes, nil
	}
	return a == "y" || a == "yes", nil
}

func lineChoose(title string, options []Option) (string, error) {
	fmt.Fprintln(os.Stderr, Err.Bold(title))
	for i, o := range options {
		fmt.Fprintf(os.Stderr, "  %s %s\n", Err.Cyan(fmt.Sprintf("%d)", i+1)), o.Label)
	}
	for {
		a, err := readLine(Err.Dim("  choice [1]: "))
		if err != nil {
			return "", err
		}
		if a == "" {
			return options[0].Value, nil
		}
		if n, err := strconv.Atoi(a); err == nil && n >= 1 && n <= len(options) {
			return options[n-1].Value, nil
		}
		fmt.Fprintln(os.Stderr, "  "+Err.Red("Pick 1-"+strconv.Itoa(len(options))))
	}
}

// lineSecret reads a line from the terminal without echoing it.
func lineSecret(question string) (string, error) {
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
	asking.Lock()
	defer asking.Unlock()
	if Remote != nil {
		Remote.WaitKey()
		return
	}
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
