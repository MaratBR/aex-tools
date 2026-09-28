package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	brand     = lipgloss.Color("#039EEC") // logo blue
	accent    = lipgloss.NewStyle().Foreground(brand).Bold(true)
	faint     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "243"})
	strong    = lipgloss.NewStyle().Bold(true)
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	okStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	frame     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "250", Dark: "238"}).
			Padding(1, 2)
)

type loginChecked struct{}

type menuModel struct {
	index     int
	status    string
	statusOK  bool
	width     int
	loginDone <-chan struct{}

	chosen   bool
	withArgs bool
}

type menuPick struct {
	index    int
	withArgs bool
	quit     bool
}

// selectTool shows the menu until a tool is picked or the menu is quit.
func selectTool(index int, status string, statusOK bool, loginDone <-chan struct{}) (menuPick, error) {
	m := menuModel{index: index, status: status, statusOK: statusOK, width: 80, loginDone: loginDone}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return menuPick{}, err
	}
	m = final.(menuModel)
	return menuPick{index: m.index, withArgs: m.withArgs, quit: !m.chosen}, nil
}

func (m menuModel) Init() tea.Cmd {
	done := m.loginDone
	return func() tea.Msg {
		<-done
		return loginChecked{}
	}
}

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.index = (m.index - 1 + len(tools)) % len(tools)
		case "down", "j":
			m.index = (m.index + 1) % len(tools)
		case "home", "g":
			m.index = 0
		case "end", "G":
			m.index = len(tools) - 1
		case "enter":
			m.chosen = true
			return m, tea.Quit
		case "a":
			m.chosen, m.withArgs = true, true
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		default:
			// A tool's number picks and runs it.
			for i := range tools {
				if msg.String() == fmt.Sprint(i+1) {
					m.index, m.chosen = i, true
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func renderSegments(segments []segment) string {
	var b strings.Builder
	for _, s := range segments {
		switch s.kind {
		case dim:
			b.WriteString(faint.Render(s.text))
		case bold:
			b.WriteString(strong.Render(s.text))
		case warn:
			b.WriteString(warnStyle.Render(s.text))
		default:
			b.WriteString(s.text)
		}
	}
	return b.String()
}

func (m menuModel) View() string {
	// Content width inside the frame (border 2 + padding 4).
	inner := min(max(m.width-6, 36), 72)
	fit := func(s string) string { return ansi.Truncate(s, inner, "…") }

	lines := []string{accent.Render("aex") + faint.Render(" tools"), ""}
	for _, l := range infoLines() {
		lines = append(lines, fit(renderSegments(l)))
	}
	lines = append(lines, "", faint.Render(strings.Repeat("─", inner)), "")

	for i, t := range tools {
		num := fmt.Sprintf("%d", i+1)
		if i == m.index {
			lines = append(lines,
				fit(accent.Render("▌ "+num+"  "+t.name)),
				fit(accent.Render("▌")+"    "+t.summary))
		} else {
			lines = append(lines,
				fit("  "+faint.Render(num)+"  "+t.name),
				fit("     "+faint.Render(t.summary)))
		}
		if i < len(tools)-1 {
			lines = append(lines, "")
		}
	}

	box := frame.Width(inner + 4).Render(strings.Join(lines, "\n"))
	hint := func(k, what string) string { return strong.Render(k) + " " + faint.Render(what) }
	footer := "  " + strings.Join([]string{
		hint("↑↓", "move"), hint("enter", "run"), hint("1-"+fmt.Sprint(len(tools)), "pick"),
		hint("a", "run with args"), hint("q", "quit"),
	}, faint.Render("  ·  "))

	view := "\n" + box + "\n" + footer
	if m.status != "" {
		style := okStyle
		if !m.statusOK {
			style = failStyle
		}
		view += "\n\n  " + style.Render(ansi.Truncate(m.status, inner+2, "…"))
	}
	return view
}
