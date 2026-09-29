package main

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"aex/internal/tool"
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
	// path is the selected tool: its index in tools, then in each group opened on the way to it.
	path      []int
	status    string
	statusOK  bool
	width     int
	height    int
	loginDone <-chan struct{}

	chosen   bool
	withArgs bool
}

type menuPick struct {
	path     []int
	withArgs bool
	quit     bool
}

// selectTool shows the menu until a tool (not a group, those open) is picked or the menu is quit.
func selectTool(path []int, status string, statusOK bool, loginDone <-chan struct{}) (menuPick, error) {
	m := menuModel{path: slices.Clone(path), status: status, statusOK: statusOK, width: 80, height: 24, loginDone: loginDone}
	final, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion()).Run()
	if err != nil {
		return menuPick{}, err
	}
	m = final.(menuModel)
	return menuPick{path: m.path, withArgs: m.withArgs, quit: !m.chosen}, nil
}

// menuLevel is the tools listed at the depth of path (the last index picks one of them), and the
// groups opened to get there.
func menuLevel(path []int) (list []tool.Tool, groups []*tool.Tool) {
	list = tools
	for _, i := range path[:len(path)-1] {
		groups = append(groups, &list[i])
		list = list[i].Sub
	}
	return list, groups
}

// validPath is path cut back to what still exists after the tools changed.
func validPath(path []int) []int {
	list := tools
	for depth, i := range path {
		if len(list) == 0 {
			return path[:depth]
		}
		path[depth] = min(i, len(list)-1)
		list = list[path[depth]].Sub
	}
	if len(path) == 0 {
		return []int{0}
	}
	return path
}

func (m *menuModel) index() int { return m.path[len(m.path)-1] }
func (m *menuModel) move(i int) { m.path[len(m.path)-1] = i }
func (m *menuModel) back() bool { return len(m.path) > 1 }
func (m *menuModel) list() []tool.Tool {
	list, _ := menuLevel(m.path)
	return list
}

// choose runs the tool at index i, or opens it if it is a group.
func (m menuModel) choose(i int, withArgs bool) (tea.Model, tea.Cmd) {
	m.move(i)
	if m.list()[i].IsGroup() {
		m.path = append(m.path, 0)
		return m, nil
	}
	m.chosen, m.withArgs = true, withArgs
	return m, tea.Quit
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
		n := len(m.list())
		switch {
		case msg.Button == tea.MouseButtonWheelUp:
			m.move(max(m.index()-1, 0))
		case msg.Button == tea.MouseButtonWheelDown:
			m.move(min(m.index()+1, n-1))
		default:
			i, ok := m.layout().toolAt(msg.X, msg.Y)
			if !ok {
				break
			}
			m.move(i) // hover selects
			if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonRight) {
				// Left click runs, right click runs with args.
				return m.choose(i, msg.Button == tea.MouseButtonRight)
			}
		}
	case tea.KeyMsg:
		n := len(m.list())
		switch msg.String() {
		case "up", "k":
			m.move((m.index() - 1 + n) % n)
		case "down", "j":
			m.move((m.index() + 1) % n)
		case "home", "g":
			m.move(0)
		case "end", "G":
			m.move(n - 1)
		case "enter":
			return m.choose(m.index(), false)
		case "right", "l":
			if m.list()[m.index()].IsGroup() {
				return m.choose(m.index(), false)
			}
		case "a":
			return m.choose(m.index(), true)
		case "esc", "backspace", "left", "h":
			if m.back() {
				m.path = m.path[:len(m.path)-1]
				return m, nil
			}
			if msg.String() == "esc" {
				return m, tea.Quit
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		default:
			// A tool's number picks and runs it.
			for i := range n {
				if msg.String() == fmt.Sprint(i+1) {
					return m.choose(i, false)
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

	// In a group: which one, and the way back.
	list, groups := menuLevel(m.path)
	if len(groups) > 0 {
		var names []string
		for _, g := range groups {
			names = append(names, g.Name)
		}
		group := groups[len(groups)-1]
		lines = append(lines, fit(faint.Render("‹ ")+strong.Render(strings.Join(names, " › "))+"  "+faint.Render(group.Summary)), "")
	}

	// Drop the gaps between tools when the window is too short for them.
	gaps := len(lines)+3*len(list)-1 <= bodyHeight
	rows := map[int]int{}
	for i, t := range list {
		num := fmt.Sprintf("%d", i+1)
		name := t.Name
		if t.IsGroup() {
			name += " ›"
		}
		rows[frameY+len(lines)], rows[frameY+len(lines)+1] = i, i
		if i == m.index() {
			lines = append(lines,
				selName.Width(inner).Render(ansi.Truncate(" ▶ "+num+"  "+name, inner, "…")),
				selInfo.Width(inner).Render(ansi.Truncate("      "+t.Summary, inner, "…")))
		} else {
			lines = append(lines,
				fit("   "+faint.Render(num)+"  "+name),
				fit("      "+faint.Render(t.Summary)))
		}
		if gaps && i < len(list)-1 {
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
	hints := []string{
		hint("↑↓", "move"), hint("enter/click", "run"), hint("1-"+fmt.Sprint(len(list)), "pick"),
		hint("a/right-click", "run with args"),
	}
	if len(groups) > 0 {
		hints = append(hints, hint("esc", "back"))
	}
	footer := "  " + strings.Join(append(hints, hint("q", "quit")), faint.Render("  ·  "))

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
