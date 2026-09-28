// aex: personal work tools (Jira worklogs to AEXT, hours quota) behind one menu.
package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"aex/internal/settings"
	"aex/internal/ui"
)

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --out rsrc --arch amd64,arm64

// Shared, non-secret config (base URLs), built in so the exe works on its own.
//
//go:embed .env
var embeddedEnv string

type tool struct {
	name    string
	summary string
	run     func(args []string) error
}

var tools = []tool{
	{"worklog-sync", "Export Jira worklogs to CSV, then send them to AEXT", worklogSync},
	{"quota", "Show AEXT hours quota for last and this month", quotaTool},
	{"configure", "Set AEXT / Jira login, hours per day and timezone (app settings)", configure},
	{"data-folder", "Open the data folder: settings, AEXT session, CSV exports", dataFolder},
}

func help() string {
	var list strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&list, "  %-14s%s\n", t.name, t.summary)
	}
	return fmt.Sprintf(`Usage: aex [--data-dir <dir>] [--plain] [<tool> [args...]]

Without a tool, opens a menu to pick and run tools until you quit: an arrow-key
terminal UI, or a numbered list with --plain (or AEX_TUI=0, or when not on a terminal).
With a tool name, runs that tool once with the given args (try "aex <tool> --help").

Tools:
%s
Data folder (app settings .env.config, AEXT session, output), set with --data-dir <dir> or AEX_DATA_DIR:
  %s
.env and .env.private (fallbacks) are read from:
  %s`, list.String(), settings.DataDir, settings.AppRoot)
}

// findTool takes a tool name or its number in the menu.
func findTool(input string) *tool {
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(tools) {
		return &tools[n-1]
	}
	for i := range tools {
		if tools[i].name == input {
			return &tools[i]
		}
	}
	return nil
}

func printError(err error) {
	fmt.Fprintf(os.Stderr, "%s %v\n", ui.Err.Red("error:"), err)
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
	if err != nil {
		printError(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	plain := len(args) > 0 && args[0] == "--plain"
	if plain {
		args = args[1:]
	}
	if len(args) == 0 {
		if plain || !useTUI() {
			return plainMenu()
		}
		return tuiMenu()
	}
	if args[0] == "--help" || args[0] == "-h" {
		fmt.Println(help())
		return nil
	}
	t := findTool(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool: %s (see aex --help)", args[0])
	}
	return t.run(args[1:])
}

// parseFlags parses a tool's args, printing usage on --help / -h (then done is true).
// Positional args are not accepted.
func parseFlags(fs *flag.FlagSet, args []string, usage string) (done bool, err error) {
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Println(usage)
			return true, nil
		}
		return false, fmt.Errorf("%v (see --help)", err)
	}
	if fs.NArg() > 0 {
		return false, fmt.Errorf("unexpected argument %q (see --help)", fs.Arg(0))
	}
	return false, nil
}
