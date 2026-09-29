package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// Pasted text reaches a terminal program either as one bracketed paste (tea.KeyMsg.Paste) or, in
// the Windows console, as one key event per character. Those arrive far faster than anyone types:
// characters closer together than pasteGap count as pasted, and once no more arrive for
// pasteSettle the paste is over.
const (
	pasteGap    = 8 * time.Millisecond
	pasteSettle = 40 * time.Millisecond
)

// liveInput is an Input that can rewrite its value right after something is pasted into it
// (rewrite) and show a line under the box describing the value as it is typed (describe, run off
// the UI goroutine, once per value).
type liveInput struct {
	*huh.Input
	value    *string
	rewrite  func(string) string
	describe func(string) string

	lastRune time.Time
	pasted   bool
	seq      int

	described map[string]string // describe results so far
	asked     map[string]bool   // values describe is running or ran for
}

type pasteSettledMsg struct {
	field *liveInput
	seq   int
}

type describedMsg struct {
	field       *liveInput
	value, text string
}

func newLiveInput(in *huh.Input, value *string, f Field) *liveInput {
	return &liveInput{Input: in, value: value, rewrite: f.Paste, describe: f.Describe,
		described: map[string]string{}, asked: map[string]bool{}}
}

func (l *liveInput) Init() tea.Cmd { return tea.Batch(l.Input.Init(), l.describeCmd()) }

func (l *liveInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var settle tea.Cmd
	switch m := msg.(type) {
	case tea.KeyMsg:
		if l.rewrite != nil && m.Type == tea.KeyRunes {
			now := time.Now()
			if m.Paste || len(m.Runes) > 1 || now.Sub(l.lastRune) < pasteGap {
				l.pasted = true
			}
			l.lastRune = now
			l.seq++
			seq := l.seq
			settle = tea.Tick(pasteSettle, func(time.Time) tea.Msg { return pasteSettledMsg{l, seq} })
		}
	case pasteSettledMsg:
		if m.field != l || m.seq != l.seq || !l.pasted {
			return l, nil
		}
		l.pasted = false
		if v := l.rewrite(*l.value); v != *l.value {
			*l.value = v
			l.Input.Value(l.value) // also puts it in the text box
		}
		return l, l.describeCmd()
	case describedMsg:
		if m.field == l {
			l.described[m.value] = m.text
		}
		return l, nil
	}
	_, cmd := l.Input.Update(msg)
	return l, tea.Batch(cmd, settle, l.describeCmd())
}

// describeCmd describes the current value unless that already ran or is running.
func (l *liveInput) describeCmd() tea.Cmd {
	v := *l.value
	if l.describe == nil || l.asked[v] {
		return nil
	}
	l.asked[v] = true
	return func() tea.Msg { return describedMsg{l, v, l.describe(v)} }
}

func (l *liveInput) View() string {
	view := l.Input.View()
	if l.describe == nil {
		return view
	}
	// The line is always there, blank until there is something to show: the form sizes itself
	// once, from the first view, and cuts off anything taller.
	text := l.described[*l.value]
	if text == "" {
		text = " "
	}
	t := theme()
	return view + "\n" + t.Focused.Base.Render(t.Focused.Description.Render(text))
}
