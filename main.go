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

	"aex/internal/plugin"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/tools/account"
	"aex/internal/tools/configure"
	"aex/internal/tools/plugins"
	"aex/internal/tools/quota"
	"aex/internal/tools/worklogsync"
	"aex/internal/ui"
)

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --out rsrc --arch amd64,arm64

// Shared, non-secret config (base URLs), built in so the exe works on its own.
//
//go:embed .env
var embeddedEnv string

// Tool list order: built-in tools, each its own package under internal/tools, then plugins (see
// internal/plugin), added by loadPlugins.
var builtins = []tool.Tool{
	worklogsync.Tool,
	quota.Tool,
	account.Tool,
	configure.Tool,
	plugins.Tool,
}

var tools = builtins

// loadPlugins sets tools to the built-in tools plus the plugins now in the plugins folder.
func loadPlugins() {
	tools = append(slices.Clip(builtins), plugin.Discover(builtins)...)
}

func help() string {
	var list strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&list, "  %-*s%s\n", nameWidth(), t.Name, t.Summary)
		for _, s := range t.Sub {
			fmt.Fprintf(&list, "    %-*s%s\n", nameWidth()-2, s.Name, s.Summary)
		}
	}
	return fmt.Sprintf(`Usage: aex [--data-dir <dir>] [--plain] [<tool> [args...]]

Without a tool, opens the aex window to pick and run tools.
With a tool name, runs that tool once in the terminal with the given args (try "aex <tool> --help");
--plain (or AEX_TUI=0) asks its questions line by line instead of with arrow-key prompts.
A group of tools (its tools indented below it) runs one: "aex <group> <tool> [args...]".

Tools:
%s
Plugins (tools after the built-in ones) are executables in:
  %s

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
	plain := len(args) > 0 && args[0] == "--plain"
	if plain {
		ui.Plain = true
		args = args[1:]
	}
	if len(args) == 0 {
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
		return fmt.Errorf("unknown tool: %s (see aex --help)", args[0])
	}
	return t.Exec(args[1:])
}
