// Package composertool is the composer tool: lists, shows, adds (from JSON) and removes composed
// tools (internal/composer). In the window they are made on the Composer page.
package composertool

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"aex/internal/composer"
	"aex/internal/ide"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/ui"
)

// Tool is the composer tool. The window has its own page for it, so it is not in its tool list.
var Tool = tool.Tool{Name: "composer", Summary: "Composed tools: tools made of steps (tools and actions) run in order", Run: run, Hidden: true}

func run(args []string) error {
	var kinds []string
	for _, k := range composer.Kinds {
		kinds = append(kinds, fmt.Sprintf("  %-10s%s", k.ID, k.Summary))
	}
	usage := fmt.Sprintf(`Usage: composer [list | show <name> | actions | put <file> [--replace <name>] | remove <name> [--yes] | open]

A composed tool runs its steps in order, each step an action. A step that fails is reported and the
next one runs, unless the step is critical: then the composed tool stops. Steps marked parallel
next to one another run together. Run one with "aex %s <name>". The window's Composer page
makes and edits them, with blocks or as JSON.

  list            List the composed tools (default)
  show <name>     A composed tool as JSON
  actions         The actions a step can be, and their JSON fields
  put <file>      Save a composed tool from a JSON file ("-" reads stdin): adds it, or replaces
                  the one with its name (--replace <name>: the one called that, to rename it);
                  checked first, nothing saved on errors
  remove <name>   Remove a composed tool; asks first unless --yes
  open            Open the data folder, where they are kept

Actions:
%s

Kept in:
  %s`, composer.Group, strings.Join(kinds, "\n"), settings.ComposedToolsFile)

	cmd, rest := "", args
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, rest = args[0], args[1:]
	}
	var target string
	if len(rest) > 0 && rest[0] != "" && (rest[0][0] != '-' || rest[0] == "-") {
		target, rest = rest[0], rest[1:]
	}
	fs := flag.NewFlagSet("composer", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "")
	replace := fs.String("replace", "", "")
	if done, err := tool.ParseFlags(fs, rest, usage); done || err != nil {
		return err
	}
	needs := func(what string) error {
		if target == "" {
			return fmt.Errorf("%s needs %s (see --help)", cmd, what)
		}
		return nil
	}
	switch cmd {
	case "", "list":
		return list()
	case "show":
		if err := needs("a composed tool name"); err != nil {
			return err
		}
		return show(target)
	case "actions":
		fmt.Println(actions)
		var ides []string
		for _, i := range ide.Installed() {
			ides = append(ides, fmt.Sprintf("%s (%s)", i.ID, i.Name))
		}
		if len(ides) == 0 {
			ides = append(ides, "none")
		}
		fmt.Println("\nIDEs found here: " + strings.Join(ides, ", "))
		return nil
	case "put":
		if err := needs("a JSON file"); err != nil {
			return err
		}
		return put(target, *replace)
	case "remove":
		if err := needs("a composed tool name"); err != nil {
			return err
		}
		return remove(target, *yes)
	case "open":
		return tool.OpenFolder(settings.DataDir)
	}
	return fmt.Errorf("unknown command %q (see --help)", cmd)
}

const actions = `A step is {"action": "<action>", "label": "…", "critical": true, "parallel": true, …its fields}.
label, critical and parallel are optional; they work the same in the steps of an if or a check.

  tool      Runs an aex tool.
              "tool": "quota" or "cm-release pull-all" (as typed on the command line)
              "args": "--verbose" (optional)
  run       Runs a program.
              "path": "C:\Tools\build.exe" or a name on PATH ("~", $VAR and %VAR% are expanded)
              "args": "--fast \"two words\"" (optional), "dir": "folder to run in" (optional)
              "detach": true starts it without waiting; else the step waits and fails on an exit code not 0
  open-ide  Opens a project or folder in an IDE found here, without waiting.
              "ide": vs2022 (vs: the newest Visual Studio), vscode, rider, idea, goland, webstorm,
                     pycharm, clion, phpstorm, rubymine, datagrip or rustrover
              "project": "C:\src\app\app.sln"
  delay     Waits.
              "seconds": 5 (fractions allowed)
  home      Switches the aex window to Home (only in the window).
  minimize  Minimizes the aex window (only in the window).
  orient    Puts windows in places on the screens (Windows, with AutoHotkey v2), waiting for those not
            open yet.
              "windows": [{"exe": "chrome.exe", "title": "part of its title" (optional),
                           "monitor": 2 (from 1; 0 or left out: the main screen),
                           "place": maximize, minimize, left, right, top, bottom, top-left, top-right,
                                    bottom-left, bottom-right, left-third, center-third, right-third,
                                    left-two-thirds, right-two-thirds, center or custom
                                    ("x", "y", "w", "h": percent of the screen),
                           "all": true (every window that matches, not only the first)}]
              "wait": 30 (seconds to wait for windows not open yet; 0: only those open now)
  message   Shows a message on top of all windows, with a chime.
              "text": "Please enable the VPN", "title": "VPN" (optional)
              "urgent": true (red, chimes until closed), "wait": true (until it is closed)
  ip-check  Checks where this device's IP is (GeoIP: ipinfo.io, else ipapi.co).
              "want": ["US"] (must be in one), "avoid": ["RU"] (must not be in any): country codes, or
              country, region or city names
  if        Runs a condition, an action: its then steps when it works, else its else steps.
              "cond": {"action": …} (without critical or parallel; not an if or a return)
              "then": [steps], "else": [steps] (optional), "not": true swaps them
  check     Runs its steps again and again until they work (a try works when none of its steps
            fails, or a return in it returns worked).
              "do": [steps]
              "every": 5 (seconds between tries), "timeout": 300 (seconds, 0: never), "attempts": 0 (0: no limit)
  return    Ends the steps it is in: one try of the check around it, or else the composed tool.
              "fail": true returns failed (else worked), "message": "why" (optional)`

func list() error {
	entries, err := composer.Load()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println(ui.Out.Dim("No composed tools. Make one on the window's Composer page, or: composer put <file>"))
		return nil
	}
	for _, e := range entries {
		if e.Err != nil {
			fmt.Printf("%s  %s\n", ui.Out.Bold(e.Name), ui.Out.Red("cannot be read: "+e.Err.Error()))
			continue
		}
		summary := e.Summary
		if summary == "" {
			summary = fmt.Sprintf("%d steps", len(e.Steps))
		}
		fmt.Printf("%s  %s\n", ui.Out.Bold(e.Name), ui.Out.Dim(summary))
		for i, s := range e.Steps {
			fmt.Printf("  %2d. %s\n", i+1, s.Title())
		}
	}
	return nil
}

func show(name string) error {
	entries, err := composer.Load()
	if err != nil {
		return err
	}
	e := composer.Find(entries, name)
	if e == nil {
		return fmt.Errorf("no composed tool %s (see composer list)", name)
	}
	fmt.Println(string(e.Raw))
	if e.Err != nil {
		ui.Warn("%s cannot be read: %v", e.Name, e.Err)
	}
	return nil
}

func put(path, replace string) error {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	c, err := composer.Parse(b)
	if err != nil {
		return err
	}
	if replace == "" {
		entries, err := composer.Load()
		if err != nil {
			return err
		}
		if e := composer.Find(entries, c.Name); e != nil {
			replace = e.Name
		}
	}
	problems, err := composer.Put(replace, c)
	if err != nil {
		return err
	}
	for _, p := range problems {
		if p.Warning {
			ui.Warn("%s", p)
		} else {
			fmt.Fprintln(os.Stderr, ui.Err.Red("✖ "+p.String()))
		}
	}
	if problems.HasErrors() {
		return errors.New("not saved")
	}
	fmt.Printf("%s Saved %s: run it with aex %s %s\n", ui.Out.Green("✔"), c.Name, composer.Group, c.Name)
	return nil
}

func remove(name string, yes bool) error {
	if !yes {
		if !ui.IsInteractive() {
			return errors.New("remove asks first: add --yes")
		}
		ok, err := ui.Confirm(fmt.Sprintf("Remove the composed tool %s?", name), false)
		if err != nil || !ok {
			return err
		}
	}
	if err := composer.Remove(name); err != nil {
		return err
	}
	fmt.Printf("%s Removed %s\n", ui.Out.Green("✔"), name)
	return nil
}
