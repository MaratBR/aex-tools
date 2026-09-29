// Package plugins is the plugins tool: lists, describes and deletes plugins, opens the plugins folder.
package plugins

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"aex/internal/plugin"
	"aex/internal/tool"
	"aex/internal/ui"
)

// Tool is the plugins tool.
var Tool = tool.Tool{Name: "plugins", Summary: "List, describe, delete and forget plugins; open the plugins folder", Run: run}

func run(args []string) error {
	dir, err := plugin.Dir()
	if err != nil {
		return err
	}
	usage := fmt.Sprintf(`Usage: plugins [list | describe [<name>] | delete <name> [--yes] | forget-all [--yes] | open]

Plugins are tools in their own executables, in:
  %s
A plugin runs, even just to describe itself, only after you approve its file: the SHA-256 of its
contents is then saved as its safe hash (credential store), and checked again before every run.

  list             List the plugins with file, SHA-256 and state; safe ones describe themselves (default)
  describe [name]  Describe a plugin, or each one; asks to approve any that is not safe
  delete <name>    Delete a plugin's file and forget its safe hash; asks first unless --yes
  forget-all       Forget every plugin's safe hash and granted access (files are kept): each asks
                   to be approved again on its next run; asks first unless --yes
  open             Open the plugins folder in the file manager

Without args on a terminal: lists the plugins, then asks what to do.`, dir)

	cmd, rest := "", args
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, rest = args[0], args[1:]
	}
	var name string
	if (cmd == "describe" || cmd == "delete") && len(rest) > 0 && rest[0] != "" && rest[0][0] != '-' {
		name, rest = rest[0], rest[1:]
	}
	fs := flag.NewFlagSet("plugins", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "")
	if done, err := tool.ParseFlags(fs, rest, usage); done || err != nil {
		return err
	}

	switch cmd {
	case "":
		if !ui.IsInteractive() {
			return list()
		}
		return manage(dir)
	case "list":
		return list()
	case "describe":
		return describe(name)
	case "delete":
		if name == "" {
			return errors.New("delete needs a plugin name (see --help)")
		}
		return remove(name, *yes)
	case "forget-all":
		return forgetAll(*yes)
	case "open":
		fmt.Println(dir)
		return tool.OpenFolder(dir)
	}
	return fmt.Errorf("unknown command %q (see --help)", cmd)
}

func list() error {
	plugins, err := plugin.List()
	if err != nil {
		return err
	}
	printList(plugins)
	return nil
}

func printList(plugins []plugin.Info) {
	out := ui.Out
	if len(plugins) == 0 {
		fmt.Println(out.Dim("No plugins."))
		return
	}
	for i, p := range plugins {
		if i > 0 {
			fmt.Println()
		}
		state := out.Yellow(p.State.String())
		if p.State == plugin.Safe {
			state = out.Green(p.State.String())
		}
		fmt.Printf("%s  %s\n", out.Bold(p.Name), state)
		switch {
		case p.DescribeErr != nil:
			fmt.Printf("  %s\n", out.Red(p.DescribeErr.Error()))
		case p.Summary != "":
			fmt.Printf("  %s\n", p.Summary)
		}
		if len(p.Access) > 0 {
			access := make([]string, len(p.Access))
			for i, a := range p.Access {
				access[i] = string(a)
			}
			fmt.Printf("  %s %s\n", out.Dim("Access  "), strings.Join(access, ", "))
		}
		if len(p.Tools) > 0 {
			names := make([]string, len(p.Tools))
			for i, t := range p.Tools {
				names[i] = t.Name
			}
			fmt.Printf("  %s %s\n", out.Dim("Tools   "), strings.Join(names, ", "))
		}
		fmt.Printf("  %s %s\n", out.Dim("File    "), p.FileInfo())
		fmt.Printf("  %s %s\n", out.Dim("SHA-256 "), p.Hash)
	}
}

// find is the plugin called name.
func find(name string) (plugin.Info, error) {
	plugins, err := plugin.List()
	if err != nil {
		return plugin.Info{}, err
	}
	for _, p := range plugins {
		if p.Name == name {
			return p, nil
		}
	}
	return plugin.Info{}, fmt.Errorf("no plugin %s (see plugins list)", name)
}

// describe describes the plugin called name, or each one when name is "".
func describe(name string) error {
	var plugins []plugin.Info
	if name != "" {
		p, err := find(name)
		if err != nil {
			return err
		}
		plugins = []plugin.Info{p}
	} else {
		var err error
		if plugins, err = plugin.List(); err != nil {
			return err
		}
		if len(plugins) == 0 {
			fmt.Println(ui.Out.Dim("No plugins."))
		}
	}
	var failed error
	for _, p := range plugins {
		summary, err := plugin.Describe(p)
		if err != nil {
			fmt.Printf("%s  %s\n", ui.Out.Bold(p.Name), ui.Out.Red(err.Error()))
			failed = errors.New("some plugins could not be described")
			continue
		}
		fmt.Printf("%s  %s\n", ui.Out.Bold(p.Name), summary)
	}
	return failed
}

func remove(name string, yes bool) error {
	p, err := find(name)
	if err != nil {
		return err
	}
	if !yes {
		if err := ui.AssertInteractive("confirming the delete (or pass --yes)"); err != nil {
			return err
		}
		ok, err := ui.Confirm(fmt.Sprintf("Delete %s and forget its safe hash?", p.Path), false)
		if err != nil || !ok {
			return err
		}
	}
	if err := plugin.Delete(p); err != nil {
		return err
	}
	fmt.Println("Deleted", p.Path)
	return nil
}

// forgetAll forgets the safe hash and granted access of every plugin, asking first unless yes.
func forgetAll(yes bool) error {
	paths, err := plugin.Approved()
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		fmt.Println("No plugin approvals to forget.")
		return nil
	}
	if !yes {
		if err := ui.AssertInteractive("confirming forget-all (or pass --yes)"); err != nil {
			return err
		}
		fmt.Println("Forgets the safe hash and granted access of:")
		for _, path := range paths {
			fmt.Println("  " + path)
		}
		ok, err := ui.Confirm("Forget all? Each plugin asks to be approved again on its next run.", false)
		if err != nil || !ok {
			return err
		}
	}
	if err := plugin.ForgetAll(); err != nil {
		return err
	}
	fmt.Println(ui.Out.Green(fmt.Sprintf("Forgot %d plugin approvals.", len(paths))))
	return nil
}

// manage lists the plugins and asks what to do, until done.
func manage(dir string) error {
	for {
		plugins, err := plugin.List()
		if err != nil {
			return err
		}
		printList(plugins)
		fmt.Println()
		options := []ui.Option{{Label: "Done", Value: "done"}}
		if len(plugins) > 0 {
			options = append(options,
				ui.Option{Label: "Describe a plugin", Value: "describe"},
				ui.Option{Label: "Describe each plugin", Value: "describe-all"},
				ui.Option{Label: "Delete a plugin", Value: "delete"})
		}
		options = append(options, ui.Option{Label: "Forget all approvals", Value: "forget-all"})
		options = append(options, ui.Option{Label: "Open the plugins folder", Value: "open"})
		action, err := ui.Choose("What now?", options)
		if err != nil {
			return err
		}
		switch action {
		case "done":
			return nil
		case "open":
			fmt.Println(dir)
			err = tool.OpenFolder(dir)
		case "describe-all":
			err = describe("")
		case "forget-all":
			err = forgetAll(false)
		case "describe", "delete":
			var name string
			if name, err = pick(plugins); err == nil && name != "" {
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

// pick asks for a plugin; "" is back.
func pick(plugins []plugin.Info) (string, error) {
	options := []ui.Option{{Label: "Back", Value: ""}}
	for _, p := range plugins {
		options = append(options, ui.Option{Label: fmt.Sprintf("%s (%v)", p.Name, p.State), Value: p.Name})
	}
	return ui.Choose("Which plugin?", options)
}
