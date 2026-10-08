// aex: personal work tools (Jira worklogs to AEXT, hours quota) in one window.
package main

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"aex/internal/autostart"
	"aex/internal/composer"
	"aex/internal/custom"
	"aex/internal/plugin"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/tools/account"
	"aex/internal/tools/composertool"
	"aex/internal/tools/configure"
	"aex/internal/tools/customtools"
	"aex/internal/tools/onboarding"
	"aex/internal/tools/plugins"
	"aex/internal/tools/quota"
	"aex/internal/tools/remind"
	"aex/internal/tools/worklogsync"
	"aex/internal/ui"
)

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --out rsrc --arch amd64,arm64

// Shared, non-secret config (base URLs), built in so the exe works on its own.
//
//go:embed .env
var embeddedEnv string

// Tool list order: built-in tools, each its own package under internal/tools, then plugins (see
// internal/plugin), then custom tools (see internal/custom), then composed tools (see
// internal/composer), added by loadPlugins.
var builtins = []tool.Tool{
	worklogsync.Tool,
	quota.Tool,
	account.Tool,
	configure.Tool,
	plugins.Tool,
	customtools.Tool,
	composertool.Tool,
	onboarding.Tool,
	onboarding.ResetTool,
	remind.Tool,
}

// debugGroup holds the debug tools (tool.Tool.Debug), only with --debug.
const debugGroup = "debug"

var tools = withDebug(builtins)

func init() {
	builtin := func(name string) bool {
		return strings.EqualFold(name, debugGroup) ||
			slices.ContainsFunc(builtins, func(t tool.Tool) bool { return strings.EqualFold(t.Name, name) }) ||
			slices.ContainsFunc(plugin.Names(), func(n string) bool { return strings.EqualFold(n, name) })
	}
	// A custom tool cannot take a built-in tool's, a plugin's, an adapter's (its group's) or a
	// composed tool's name, nor the composed tools' group's.
	custom.Reserved = func(name string) bool {
		if builtin(name) || custom.IsAdapter(name) || strings.EqualFold(name, composer.Group) {
			return true
		}
		entries, _ := composer.Load()
		return composer.Find(entries, name) != nil
	}
	// Nor can a composed tool take any of those, or a custom tool's.
	composer.Reserved = func(name string) bool {
		if builtin(name) || custom.IsAdapter(name) {
			return true
		}
		entries, _ := custom.Load()
		return slices.ContainsFunc(entries, func(e custom.Entry) bool { return strings.EqualFold(e.Name, name) })
	}
	// A tool step may run a plugin, custom or composed tool: the list has them once loaded.
	composer.Tools = func() []tool.Tool {
		if !loaded {
			loadPlugins()
		}
		return tools
	}
}

// extraGroups are the groups of custom tools (one per adapter) and of composed tools, as
// loadPlugins found them: "aex <name>" runs a tool in them too.
var extraGroups []tool.Tool

// loaded is set once loadPlugins has run.
var loaded bool

// loadPlugins sets tools to the built-in tools plus the plugins now in the plugins folder, the
// custom tools added (grouped by adapter) and the composed tools.
func loadPlugins() {
	base := withDebug(builtins)
	withPlugins := append(slices.Clip(base), plugin.Discover(base)...)
	extraGroups = custom.Discover(withPlugins)
	extraGroups = append(extraGroups, composer.Discover(append(slices.Clip(withPlugins), extraGroups...))...)
	tools = append(withPlugins, extraGroups...)
	loaded = true
}

// withDebug takes the debug tools out of list, and with --debug puts them in the debug group at
// its end.
func withDebug(list []tool.Tool) []tool.Tool {
	var out, debug []tool.Tool
	for _, t := range list {
		if t.Debug {
			debug = append(debug, t)
		} else {
			out = append(out, t)
		}
	}
	if settings.Debug && len(debug) > 0 {
		out = append(out, tool.Tool{Name: debugGroup, Summary: "Tools for developing aex (--debug)", Sub: debug})
	}
	return out
}

func help() string {
	var list strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&list, "  %-*s%s\n", nameWidth(), t.Name, t.Summary)
		for _, s := range t.Sub {
			fmt.Fprintf(&list, "    %-*s%s\n", nameWidth()-2, s.Name, s.Summary)
		}
	}
	return fmt.Sprintf(`Usage: aex [--data-dir <dir>] [--plain] [--debug] [--debug-fake-error] [--autostart] [<tool> [args...]]

Without a tool, opens the aex window to pick and run tools.
With a tool name, runs that tool once in the terminal with the given args (try "aex <tool> --help");
--plain (or AEX_TUI=0) asks its questions line by line instead of with arrow-key prompts.
--debug adds the tools and widgets for developing aex (the debug group); bin\aex.ps1 passes it.
--debug-fake-error has the window print a made-up error soon after it opens, outside any run.
A group of tools (its tools indented below it) runs one: "aex <group> <tool> [args...]".

Tools:
%s
Plugins (tools after the built-in ones) are executables in:
  %s
Custom tools (after the plugins) are scripts added with "aex custom-tools add <path>", grouped by
the adapter that runs them: "aex powershell <name>" (or just "aex <name>").
Composed tools (last) are steps run in order, made in the window's Composer or with "aex composer":
"aex composed <name>" (or just "aex <name>").

Data folder (app settings .env.config, AEXT session, output), set with --data-dir <dir> or AEX_DATA_DIR:
  %s
.env (fallback) is read from:
  %s`, list.String(), pluginDir(), settings.DataDir, settings.AppRoot)
}

// findTool takes a tool name or its number in the tool list (aex --help).
func findTool(input string) *tool.Tool {
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(tools) {
		return &tools[n-1]
	}
	for i := range tools {
		if tools[i].Name == input {
			return &tools[i]
		}
	}
	return nil
}

// nameWidth is the column width for tool names in lists.
func nameWidth() int {
	w := 12
	for _, t := range tools {
		w = max(w, len(t.Name))
	}
	return w + 2
}

func printError(err error) {
	fmt.Fprintf(os.Stderr, "%s %v\n", ui.Err.Bold(ui.Err.Red("✖ error:")), err)
}

func main() {
	ui.Setup()
	dataDir, args, err := settings.TakeGlobalArgs(os.Args[1:])
	if err == nil {
		err = settings.Init(dataDir, embeddedEnv)
	}
	if err == nil {
		err = run(args)
	}
	if exit, ok := errors.AsType[*plugin.ExitError](err); ok {
		os.Exit(exit.Code)
	}
	if err != nil {
		printError(err)
		os.Exit(1)
	}
}

func pluginDir() string {
	dir, err := plugin.Dir()
	if err != nil {
		return err.Error()
	}
	return dir
}

func run(args []string) error {
	autostarted := false
	for len(args) > 0 && slices.Contains([]string{"--plain", "--debug", "--debug-fake-error", autostart.Flag}, args[0]) {
		switch args[0] {
		case "--plain":
			ui.Plain = true
		case "--debug":
			settings.Debug = true
		case "--debug-fake-error":
			settings.FakeError = true
		default:
			autostarted = true
		}
		args = args[1:]
	}
	tools = withDebug(builtins)
	if len(args) > 0 && isWindowExe() {
		pointToCLI(args)
	}
	if len(args) == 0 {
		// Started on login (internal/autostart) on a day not picked for that: nothing to do.
		if autostarted && autostart.Skip() {
			return nil
		}
		return guiMenu()
	}
	if args[0] == "--help" || args[0] == "-h" {
		loadPlugins()
		fmt.Println(help())
		return nil
	}
	t := findTool(args[0])
	if t == nil {
		loadPlugins()
		t = findTool(args[0])
	}
	if t == nil {
		// A custom or composed tool also runs without its group: "aex <name>".
		t = custom.Find(extraGroups, args[0])
	}
	if t == nil {
		return fmt.Errorf("unknown tool: %s (see aex --help)", args[0])
	}
	return t.Exec(args[1:])
}
