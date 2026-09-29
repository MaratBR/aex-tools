package main

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"aex/internal/settings"
	"aex/internal/shortcut"
	"aex/internal/tool"
	"aex/internal/ui"
	"aex/internal/webui"
)

// guiMenu opens the tools in the aex window (see internal/webui) until it is closed.
func guiMenu() error {
	// Started from a shell, the terminal is the shell's again once the window closes.
	defer saveConsole()()
	// Started from the Start menu or Explorer, the exe got a console of its own: not needed.
	hideOwnConsole()
	// Questions are answered in the window, never in the terminal: tools and plugins (which inherit
	// stdin) must not read it, or put it in raw mode.
	if null, err := os.Open(os.DevNull); err == nil {
		os.Stdin = null
	}
	// Output goes to a pipe the window renders, which takes ANSI styles.
	ui.ForceColor()
	lipgloss.SetColorProfile(termenv.TrueColor)
	loadPlugins()
	loginDone := startLoginCheck()
	return webui.Run("aex tools", webui.Host{
		Tools: func() []tool.Tool { return tools },
		Run: func(name string, t *tool.Tool, args []string) (string, bool) {
			status, ok := execTool(name, t, args)
			loginDone = startLoginCheck()
			loadPlugins()
			return status, ok
		},
		Header: func(wait bool) []webui.InfoLine {
			if wait {
				<-loginDone
			}
			l := currentLogin()
			var lines []webui.InfoLine
			for i, segs := range infoLines() {
				line := webui.InfoLine{Label: strings.TrimSpace(segs[0].text)}
				switch i {
				case 0:
					line.State = l.aext.state
				case 1:
					line.State = l.jira.state
				}
				switch line.Label {
				case "Settings":
					line.Action = "settings"
				case "Data":
					line.Open = settings.DataDir
				}
				for _, s := range segs[1:] {
					kind := [...]string{plain: "plain", dim: "dim", bold: "bold", warn: "warn"}[s.kind]
					line.Segments = append(line.Segments, webui.Segment{Text: s.text, Kind: kind})
				}
				lines = append(lines, line)
			}
			return lines
		},
		Changed: func() { loginDone = startLoginCheck() },
		Ready: func() {
			// Dev builds are rebuilt in place and run from the repo, so only release builds offer this.
			if !settings.IsDev {
				shortcut.OfferOnce()
			}
		},
	})
}
