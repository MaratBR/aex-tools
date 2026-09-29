// Package customtools is the custom-tools tool: adds scripts as custom tools (internal/custom),
// lists, describes and removes them.
package customtools

import (
	"flag"
	"fmt"
	"strings"

	"aex/internal/adapter"
	"aex/internal/custom"
	"aex/internal/plugin"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/ui"
)

// Tool is the custom-tools tool.
var Tool = tool.Tool{Name: "custom-tools", Summary: "Add scripts (PowerShell) as tools; list, describe and remove them", Run: run}

func run(args []string) error {
	usage := fmt.Sprintf(`Usage: custom-tools [list | describe <name> | add <path> [--name <name>] | remove <name> [--yes]]

A custom tool is a script run by a tool adapter for its kind of file (adapters: %s). Its
parameters are read without running it; "aex <name> [args]" parses args against them and asks for
the ones left out (every one when there are no args). It runs only after you approve its file: the
SHA-256 of its contents is saved as its safe hash and checked before every run, as for plugins.
When the script is in a git repo, its repo, commit and changes not committed are shown.

  list            List the custom tools with file, SHA-256, state, parameters and git info (default)
  describe <name> A custom tool's parameters (its --help) and info
  add <path>      Add the script at path, named after its file unless --name
  remove <name>   Remove a custom tool and forget its safe hash (the script is kept); asks first
                  unless --yes

Without args on a terminal: lists them, then asks what to do. Kept in:
  %s`, adapterNames(), settings.CustomToolsFile)

	cmd, rest := "", args
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, rest = args[0], args[1:]
	}
	var target string
	if cmd != "" && cmd != "list" && len(rest) > 0 && rest[0] != "" && rest[0][0] != '-' {
		target, rest = rest[0], rest[1:]
	}
	fs := flag.NewFlagSet("custom-tools", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "")
	name := fs.String("name", "", "")
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
	case "":
		if !ui.IsInteractive() {
			return list()
		}
		return manage()
	case "list":
		return list()
	case "describe":
		if err := needs("a custom tool name"); err != nil {
			return err
		}
		return describe(target)
	case "add":
		if err := needs("the script's path"); err != nil {
			return err
		}
		return add(target, *name)
	case "remove":
		if err := needs("a custom tool name"); err != nil {
			return err
		}
		return remove(target, *yes)
	}
	return fmt.Errorf("unknown command %q (see --help)", cmd)
}

func adapterNames() string {
	names := make([]string, len(custom.Adapters))
	for i, a := range custom.Adapters {
		names[i] = a.Name()
	}
	return strings.Join(names, ", ")
}

func list() error {
	infos, err := custom.List(true)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Println(ui.Out.Dim("No custom tools. Add one with: custom-tools add <path>"))
		return nil
	}
	for n, i := range infos {
		if n > 0 {
			fmt.Println()
		}
		printInfo(i)
	}
	return nil
}

func printInfo(i custom.Info) {
	out := ui.Out
	state := out.Yellow(i.State.String())
	if i.State == plugin.Safe {
		state = out.Green(i.State.String())
	}
	if i.Hash == "" {
		state = out.Red("cannot be read")
	}
	fmt.Printf("%s  %s\n", out.Bold(i.Name), state)
	switch {
	case i.Err != nil:
		fmt.Printf("  %s\n", out.Red(i.Err.Error()))
	case i.Desc.Summary != "":
		fmt.Printf("  %s\n", i.Desc.Summary)
	}
	line := func(label, value string) { fmt.Printf("  %s %s\n", out.Dim(fmt.Sprintf("%-8s", label)), value) }
	if i.Hash != "" {
		line("File", i.Path+"  "+out.Dim(i.FileInfo()))
		line("SHA-256", i.Hash)
	} else {
		line("File", i.Path)
	}
	for _, d := range custom.Details(i) {
		value := d[1]
		if d[0] == "Pending" && i.Git != nil && i.Git.Pending() {
			value = out.Yellow(value)
		}
		line(d[0], value)
	}
}

func find(name string) (custom.Info, error) {
	entries, err := custom.Load()
	if err != nil {
		return custom.Info{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			return custom.Inspect(e, true), nil
		}
	}
	return custom.Info{}, fmt.Errorf("no custom tool %s (see custom-tools list)", name)
}

func describe(name string) error {
	i, err := find(name)
	if err != nil {
		return err
	}
	printInfo(i)
	if i.Err == nil {
		fmt.Println()
		fmt.Println(adapter.Usage(i.Name, i.Desc))
	}
	return nil
}

func add(path, name string) error {
	e, err := custom.Add(path, name)
	if err != nil {
		return err
	}
	fmt.Println(ui.Out.Green("Added custom tool "+e.Name+".") + " It asks to be approved on its first run.")
	fmt.Println()
	printInfo(custom.Inspect(e, true))
	return nil
}

func remove(name string, yes bool) error {
	i, err := find(name)
	if err != nil {
		return err
	}
	if !yes {
		if err := ui.AssertInteractive("confirming the removal (or pass --yes)"); err != nil {
			return err
		}
		ok, err := ui.Confirm(fmt.Sprintf("Remove custom tool %s and forget its safe hash? %s is kept.", i.Name, i.Path), false)
		if err != nil || !ok {
			return err
		}
	}
	if _, err := custom.Remove(i.Name); err != nil {
		return err
	}
	fmt.Println("Removed custom tool", i.Name)
	return nil
}

// manage lists the custom tools and asks what to do, until done.
func manage() error {
	for {
		if err := list(); err != nil {
			return err
		}
		entries, err := custom.Load()
		if err != nil {
			return err
		}
		fmt.Println()
		options := []ui.Option{{Label: "Done", Value: "done"}, {Label: "Add a script", Value: "add"}}
		if len(entries) > 0 {
			options = append(options,
				ui.Option{Label: "Describe a custom tool", Value: "describe"},
				ui.Option{Label: "Remove a custom tool", Value: "remove"})
		}
		action, err := ui.Choose("What now?", options)
		if err != nil {
			return err
		}
		switch action {
		case "done":
			return nil
		case "add":
			err = askAdd()
		case "describe", "remove":
			var name string
			if name, err = pick(entries); err == nil && name != "" {
				if action == "describe" {
					err = describe(name)
				} else {
					err = remove(name, false)
				}
			}
		}
		if err != nil {
			fmt.Printf("%s %v\n", ui.Out.Red("✖"), err)
		}
		fmt.Println()
	}
}

func askAdd() error {
	path, err := ui.Input(ui.Field{
		Title:       "Script path",
		Description: "Adapters: " + adapterNames(),
		Validate:    func(s string) error { _, _, err := custom.CheckPath(s); return err },
	})
	if err != nil {
		return err
	}
	abs, _, err := custom.CheckPath(path)
	if err != nil {
		return err
	}
	def := custom.DefaultName(abs)
	name, err := ui.Input(ui.Field{
		Title:       "Tool name",
		Placeholder: def,
		Validate: func(s string) error {
			if s == "" {
				s = def
			}
			return custom.CheckName(s)
		},
	})
	if err != nil {
		return err
	}
	if name == "" {
		name = def
	}
	return add(abs, name)
}

// pick asks for a custom tool; "" is back.
func pick(entries []custom.Entry) (string, error) {
	options := []ui.Option{{Label: "Back", Value: ""}}
	for _, e := range entries {
		options = append(options, ui.Option{Label: e.Name + "  " + ui.Err.Dim(e.Path), Value: e.Name})
	}
	return ui.Choose("Which custom tool?", options)
}
