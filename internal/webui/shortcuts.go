package webui

// ShortcutTool is a tool the Shortcuts widget can launch: Line is how it is typed on the command
// line (a group's tool after its group's name, e.g. "cm-release pull-all").
type ShortcutTool struct {
	Line    string `json:"line"`
	Summary string `json:"summary"`
}

// toolsAPI lists the tools a Shortcuts button can launch, in the window's order: every tool that
// runs on its own, groups left out (their tools are listed instead).
func toolsAPI(a *App, _ map[string]any) (any, error) {
	out := []ShortcutTool{}
	var walk func(prefix string, ts []ToolInfo)
	walk = func(prefix string, ts []ToolInfo) {
		for _, t := range ts {
			if len(t.Sub) > 0 {
				walk(prefix+t.Name+" ", t.Sub)
				continue
			}
			out = append(out, ShortcutTool{Line: prefix + t.Name, Summary: t.Summary})
		}
	}
	walk("", a.Tools())
	return out, nil
}
