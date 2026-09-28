package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"aex/internal/ui"
)

var (
	brand     = ui.Brand
	accent    = lipgloss.NewStyle().Foreground(brand).Bold(true)
	faint     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "243"})
	strong    = lipgloss.NewStyle().Bold(true)
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	okStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	selName   = lipgloss.NewStyle().Background(brand).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	selInfo   = lipgloss.NewStyle().Background(brand).Foreground(lipgloss.Color("#E3F5FF"))
	frame     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "250", Dark: "238"}).
			Padding(1, 2)
)

// Frame geometry: border 1 + padding 2 on each side horizontally, border 1 + padding 1 vertically.
const (
	frameX      = 3
	frameY      = 2
	footerLines = 2 // status + key hints below the frame
)

type loginChecked struct{}

type menuModel struct {
	index     int
	status    string
	statusOK  bool
	width     int
	height    int
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
	m := menuModel{index: index, status: status, statusOK: statusOK, width: 80, height: 24, loginDone: loginDone}
	final, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion()).Run()
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
		m.width, m.height = msg.Width, msg.Height
	case tea.MouseMsg:
		switch {
		case msg.Button == tea.MouseButtonWheelUp:
			m.index = max(m.index-1, 0)
		case msg.Button == tea.MouseButtonWheelDown:
			m.index = min(m.index+1, len(tools)-1)
		default:
			i, ok := m.layout().toolAt(msg.X, msg.Y)
			if !ok {
				break
			}
			m.index = i // hover selects
			if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonRight) {
				// Left click runs, right click runs with args.
				m.chosen, m.withArgs = true, msg.Button == tea.MouseButtonRight
				return m, tea.Quit
			}
		}
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

// menuLayout is the rendered screen plus where each tool sits on it, for mouse hits.
type menuLayout struct {
	view  string
	width int
	rows  map[int]int // screen row -> tool index
}

func (l menuLayout) toolAt(x, y int) (int, bool) {
	if x < frameX || x >= l.width-frameX {
		return 0, false
	}
	i, ok := l.rows[y]
	return i, ok
}

func (m menuModel) layout() menuLayout {
	// Content area inside the frame, stretched over the whole window.
	inner := max(m.width-2*frameX, 30)
	bodyHeight := max(m.height-2*frameY-footerLines, 1)
	fit := func(s string) string { return ansi.Truncate(s, inner, "…") }

	lines := []string{accent.Render("aex") + faint.Render(" tools"), ""}
	for _, l := range infoLines() {
		lines = append(lines, fit(renderSegments(l)))
	}
	lines = append(lines, "", faint.Render(strings.Repeat("─", inner)), "")

	// Drop the gaps between tools when the window is too short for them.
	gaps := len(lines)+3*len(tools)-1 <= bodyHeight
	rows := map[int]int{}
	for i, t := range tools {
		num := fmt.Sprintf("%d", i+1)
		rows[frameY+len(lines)], rows[frameY+len(lines)+1] = i, i
		if i == m.index {
			lines = append(lines,
				selName.Width(inner).Render(ansi.Truncate(" ▶ "+num+"  "+t.name, inner, "…")),
				selInfo.Width(inner).Render(ansi.Truncate("      "+t.summary, inner, "…")))
		} else {
			lines = append(lines,
				fit("   "+faint.Render(num)+"  "+t.name),
				fit("      "+faint.Render(t.summary)))
		}
		if gaps && i < len(tools)-1 {
			lines = append(lines, "")
		}
	}

	box := frame.Width(inner + 4).Height(bodyHeight + 2).Render(strings.Join(lines, "\n"))

	status := ""
	if m.status != "" {
		style := okStyle
		if !m.statusOK {
			style = failStyle
		}
		status = "  " + style.Render(ansi.Truncate(m.status, inner+2, "…"))
	}
	hint := func(k, what string) string { return strong.Render(k) + " " + faint.Render(what) }
	footer := "  " + strings.Join([]string{
		hint("↑↓", "move"), hint("enter/click", "run"), hint("1-"+fmt.Sprint(len(tools)), "pick"),
		hint("a/right-click", "run with args"), hint("q", "quit"),
	}, faint.Render("  ·  "))

	return menuLayout{view: box + "\n" + status + "\n" + ansi.Truncate(footer, m.width, "…"), width: m.width, rows: rows}
}

func (m menuModel) View() string {
	return m.layout().view
}

var banner = lipgloss.NewStyle().Background(brand).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1)

// toolBanner heads a tool's output: its name as a badge, then the args.
func toolBanner(name string, args []string) string {
	line := strings.Join(args, " ")
	if !ui.Out.On() {
		return ui.Out.Bold(strings.TrimSpace("▶ " + name + " " + line))
	}
	return banner.Render("▶ "+name) + " " + faint.Render(line)
}

// statusLine styles a runTool outcome.
func statusLine(status string, ok bool) string {
	if !ui.Err.On() {
		return status
	}
	if ok {
		return okStyle.Render(status)
	}
	return failStyle.Render(status)
}
