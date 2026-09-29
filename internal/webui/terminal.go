package webui

import (
	"encoding/base64"
	"errors"
	"io"
)

// Terminals: a program run in a pseudo-console (internal/pty) shows in its run as a terminal view
// (xterm.js). Its screen comes as "term-output" events (base64, so a UTF-8 character split between
// two reads stays whole), then "term-end"; keys typed and size changes come back through TermInput
// and TermResize.

// terminal is an open terminal view.
type terminal struct {
	input  io.Writer
	resize func(cols, rows int)
}

// OpenTerminal implements ui.TerminalHost.
func (a *App) OpenTerminal(input io.Writer, resize func(cols, rows int)) io.WriteCloser {
	a.mu.Lock()
	a.nextID++
	id := a.nextID
	a.terminals[id] = &terminal{input, resize}
	a.mu.Unlock()
	// After what the tool printed before it.
	a.send("terminal", map[string]any{"id": id})
	return &screen{a, id}
}

// screen writes a terminal's output to the frontend.
type screen struct {
	a  *App
	id int
}

func (s *screen) Write(b []byte) (int, error) {
	s.a.emit("term-output", map[string]any{"id": s.id, "data": base64.StdEncoding.EncodeToString(b)})
	return len(b), nil
}

func (s *screen) Close() error {
	s.a.mu.Lock()
	delete(s.a.terminals, s.id)
	s.a.mu.Unlock()
	s.a.emit("term-end", map[string]any{"id": s.id})
	return nil
}

func (a *App) terminal(id int) (*terminal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.terminals[id]
	if !ok {
		return nil, errors.New("the program has ended")
	}
	return t, nil
}

// TermInput sends keys typed in terminal id (as xterm.js gives them: text and VT sequences).
func (a *App) TermInput(id int, data string) error {
	t, err := a.terminal(id)
	if err != nil {
		return err
	}
	_, err = io.WriteString(t.input, data)
	return err
}

// TermResize tells the program in terminal id its new size.
func (a *App) TermResize(id, cols, rows int) error {
	t, err := a.terminal(id)
	if err != nil {
		return err
	}
	if cols > 0 && rows > 0 {
		t.resize(cols, rows)
	}
	return nil
}
