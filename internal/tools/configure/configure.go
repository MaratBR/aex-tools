// Package configure is the configure tool: asks for settings and saves them to app settings.
package configure

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"aex/internal/aext"
	"aex/internal/settings"
	"aex/internal/shortcut"
	"aex/internal/tool"
	"aex/internal/ui"
)

func configureHelp() string {
	var list strings.Builder
	for _, s := range settings.All {
		def := ""
		if s.Default != "" {
			def = fmt.Sprintf(" (default %s)", s.Default)
		}
		fmt.Fprintf(&list, "  %-16s%s%s\n", s.Name, s.Hint, def)
	}
	return fmt.Sprintf(`Usage: configure [NAME | NAME=value]...
       configure --wipe-settings | --wipe-all
       configure --add-shortcut | --remove-shortcut

Without arguments, lists the settings with their current values: pick one to change it (it is
saved right away), or fill in every setting that is not set. Settings (app settings, %s):
%s
  NAME        Ask only for NAME
  NAME=value  Save NAME without asking ("NAME=-" removes it). Not for secret settings (JIRA_TOKEN),
              which would end up in shell history: use NAME to be asked instead.
  --wipe-settings  Delete all settings: app settings, secret settings and plugin settings
  --wipe-all       Delete everything: all settings, the AEXT session, output files, plugin data and
                   anything else in the data folder. Plugins and their approvals are kept.
Both ask you to type CONFIRM. The menu offers them too.
  --add-shortcut     Add aex to the %s (%s), pointing at this exe
  --remove-shortcut  Remove aex from it

App settings have the highest priority: they override .env and real environment variables,
which remain fallbacks. Leave an answer empty to keep the current value, or enter "-" to
remove it from app settings (settings with a default are reset to it).`, settings.ConfigEnvFile, list.String(),
		shortcut.Where(), shortcutPath())
}

// Tool is the configure tool.
// The window has its own settings form instead, so it is hidden there.
var Tool = tool.Tool{Name: "configure", Summary: "Set AEXT / Jira login, hours per day and timezone (app settings)", Run: run, Hidden: true}

func run(args []string) error {
	fs := flag.NewFlagSet("configure", flag.ContinueOnError)
	wipeSettings := fs.Bool("wipe-settings", false, "")
	wipeAll := fs.Bool("wipe-all", false, "")
	addShortcut := fs.Bool("add-shortcut", false, "")
	removeShortcut := fs.Bool("remove-shortcut", false, "")
	if done, err := tool.ParseFlagsAndArgs(fs, args, configureHelp()); done || err != nil {
		return err
	}
	if *wipeSettings || *wipeAll {
		if fs.NArg() > 0 || (*wipeSettings && *wipeAll) {
			return errors.New("--wipe-settings and --wipe-all take no other arguments (see configure --help)")
		}
		if err := ui.AssertInteractive("configure --wipe-*"); err != nil {
			return err
		}
		return wipe(*wipeAll)
	}
	if *addShortcut || *removeShortcut {
		if fs.NArg() > 0 || (*addShortcut && *removeShortcut) {
			return errors.New("--add-shortcut and --remove-shortcut take no other arguments (see configure --help)")
		}
		return setShortcut(*addShortcut)
	}
	if fs.NArg() > 0 {
		return runArgs(fs.Args())
	}
	if err := ui.AssertInteractive("configure"); err != nil {
		return err
	}
	fmt.Printf("App settings: %s\n\n", ui.Out.Cyan(settings.ConfigEnvFile))
	return menu()
}

func find(name string) (settings.Setting, error) {
	for _, s := range settings.All {
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}
	return settings.Setting{}, fmt.Errorf("unknown setting %q (see configure --help)", name)
}

// runArgs handles NAME (ask for it) and NAME=value (save it) arguments.
func runArgs(args []string) error {
	for _, arg := range args {
		name, value, hasValue := strings.Cut(arg, "=")
		s, err := find(strings.TrimSpace(name))
		if err != nil {
			return err
		}
		if !hasValue {
			if err := ui.AssertInteractive("configure " + s.Name); err != nil {
				return err
			}
			if err := edit(s); err != nil {
				return err
			}
			continue
		}
		if s.Secret {
			return fmt.Errorf("%s is secret: run configure %s to be asked for it instead", s.Name, s.Name)
		}
		value = strings.TrimSpace(value)
		if value != "-" {
			if problem := settings.Problem(s.Name, value); problem != "" {
				return errors.New(problem)
			}
		}
		if err := apply(s, value, false); err != nil {
			return err
		}
	}
	return nil
}

// unset lists settings with no value and no default: the ones a first run has to fill in.
func unset() []settings.Setting {
	var list []settings.Setting
	for _, s := range settings.All {
		if s.Default == "" && settings.Get(s.Name) == "" {
			list = append(list, s)
		}
	}
	return list
}

func shown(s settings.Setting) string {
	current := settings.Get(s.Name)
	switch {
	case current == "":
		return "not set"
	case s.Secret:
		return settings.Mask(current)
	}
	return current
}

// menu asks which setting to change until Done, saving each change right away.
func menu() error {
	width := 0
	for _, s := range settings.All {
		width = max(width, len(s.Name))
	}
	for {
		missing := unset()
		var options []ui.Option
		if len(missing) > 0 {
			options = append(options, ui.Option{Label: fmt.Sprintf("Fill in the %d not set", len(missing)), Value: "missing"})
		}
		for _, s := range settings.All {
			options = append(options, ui.Option{Label: fmt.Sprintf("%-*s  %s", width, s.Name, shown(s)), Value: s.Name})
		}
		if shortcut.Supported() {
			if shortcut.Exists() {
				options = append(options, ui.Option{Label: "Remove aex from the " + shortcut.Where(), Value: "remove-shortcut"})
			} else {
				options = append(options, ui.Option{Label: "Add aex to the " + shortcut.Where(), Value: "add-shortcut"})
			}
		}
		options = append(options,
			ui.Option{Label: "Wipe all settings…", Value: "wipe-settings"},
			ui.Option{Label: "Wipe everything (settings, session, output, plugin data)…", Value: "wipe-all"},
			ui.Option{Label: "Done", Value: "done"})

		choice, err := ui.Choose("Setting to change", options)
		if err != nil {
			return err
		}
		switch choice {
		case "done":
			return nil
		case "add-shortcut", "remove-shortcut":
			if err := setShortcut(choice == "add-shortcut"); err != nil {
				return err
			}
			fmt.Println()
		case "wipe-settings", "wipe-all":
			if err := wipe(choice == "wipe-all"); err != nil {
				return err
			}
		case "missing":
			for _, s := range missing {
				if err := edit(s); err != nil {
					return err
				}
			}
		default:
			s, err := find(choice)
			if err != nil {
				return err
			}
			if err := edit(s); err != nil {
				return err
			}
		}
	}
}

// edit asks for one setting and saves the answer.
func edit(s settings.Setting) error {
	current := settings.Get(s.Name)
	removeLabel := `"-" removes it`
	if s.Default != "" {
		removeLabel = `"-" resets to ` + s.Default
	}
	placeholder := "empty keeps current"
	if current != "" && !s.Secret {
		placeholder = current
	}
	input, err := ui.Input(ui.Field{
		Title:       s.Name,
		Description: fmt.Sprintf("%s\nCurrent: %s · empty keeps it, %s", s.Hint, shown(s), removeLabel),
		Placeholder: placeholder,
		Secret:      s.Secret,
		Validate: func(v string) error {
			if v == "" || v == "-" {
				return nil
			}
			if problem := settings.Problem(s.Name, v); problem != "" {
				return errors.New(problem)
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	defer fmt.Println()
	if input == "" {
		return nil
	}
	return apply(s, input, true)
}

// apply saves value ("-" removes it, or resets to the default) for s. ask allows confirming an
// override of a fallback source, and wiping the AEXT session when the email changes.
func apply(s settings.Setting, value string, ask bool) error {
	if value != "-" {
		if overridden := Overrides(s.Name); len(overridden) > 0 {
			fmt.Fprintf(os.Stderr, "  %s %s is also set in %s; app settings will override it.\n",
				ui.Err.Yellow("Warning:"), s.Name, strings.Join(overridden, ", "))
			if ask {
				save, err := ui.Confirm("  Save anyway?", true)
				if err != nil || !save {
					return err
				}
			}
		}
	}
	before := settings.Get(s.Name)
	msg, err := Save(s.Name, value)
	if err != nil {
		return err
	}
	fmt.Println(ui.Out.Green(msg))
	if s.Name == "AEXT_EMAIL" && before != "" && before != settings.Get(s.Name) {
		return emailChanged(ask)
	}
	return nil
}

// Overrides lists the fallback sources (.env, environment variables) that also set name, which
// app settings override.
func Overrides(name string) []string {
	var list []string
	for _, l := range settings.Fallbacks() {
		if l.Vars[name] != "" {
			list = append(list, l.Label)
		}
	}
	return list
}

// Save saves value for the setting name to app settings without asking anything: "-" removes it
// (or resets it to its default). It does not validate value. Returns what it did, for the user.
func Save(name, value string) (string, error) {
	s, err := find(name)
	if err != nil {
		return "", err
	}
	if value == "-" && s.Default != "" {
		value = s.Default
	}
	if value == "-" {
		if !settings.InAppSettings(s.Name) {
			return s.Name + " is not in app settings, nothing to remove", nil
		}
		if _, err := settings.RemoveAppSetting(s.Name); err != nil {
			return "", err
		}
		return "Removed " + s.Name, nil
	}
	if err := settings.SaveAppSettings([]settings.Change{{Name: s.Name, Value: &value}}); err != nil {
		return "", err
	}
	settings.Set(s.Name, value)
	return "Saved " + s.Name, nil
}

// emailChanged offers to wipe the cached AEXT session, which belongs to the old email.
func emailChanged(ask bool) error {
	client, err := aext.New()
	if err != nil || !client.HasSession() {
		return err
	}
	if !ask {
		fmt.Println("AEXT email changed; run account to wipe the old email's session.")
		return nil
	}
	wipe, err := ui.Confirm("AEXT email changed. Wipe the AEXT session of the old email?", true)
	if err != nil || !wipe {
		return err
	}
	if err := client.WipeSession(); err != nil {
		return err
	}
	fmt.Println("Session wiped; the next AEXT tool will ask for a login code.")
	return nil
}

// wipe deletes all settings, or (all) everything in the data folder, once the user types CONFIRM.
func wipe(all bool) error {
	what, run := settings.SettingsToWipe(), settings.WipeSettings
	if all {
		data, err := settings.DataToWipe()
		if err != nil {
			return err
		}
		what, run = append(what, data...), settings.WipeAll
	}
	fmt.Println(ui.Out.Red("This permanently deletes:"))
	for _, w := range what {
		fmt.Println("  " + w)
	}
	if all {
		fmt.Println("Plugins and their approvals are kept.")
	}
	fmt.Println(".env and environment variables are not touched and still apply.")
	input, err := ui.Input(ui.Field{
		Title:       "Type CONFIRM to wipe",
		Description: "This cannot be undone. Anything else cancels.",
	})
	if err != nil {
		return err
	}
	defer fmt.Println()
	if input != "CONFIRM" {
		fmt.Println("Not confirmed, nothing was wiped.")
		return nil
	}
	if err := run(); err != nil {
		return err
	}
	if all {
		fmt.Println(ui.Out.Green("Wiped everything."))
	} else {
		fmt.Println(ui.Out.Green("Wiped all settings."))
	}
	return nil
}

func shortcutPath() string {
	p, err := shortcut.Path()
	if err != nil {
		return err.Error()
	}
	return p
}

// setShortcut adds aex to the OS app launcher (add) or removes it.
func setShortcut(add bool) error {
	if add {
		if err := shortcut.Create(); err != nil {
			return err
		}
		fmt.Println(ui.Out.Green("Added to the " + shortcut.Where() + ": " + shortcutPath()))
		return nil
	}
	if !shortcut.Exists() {
		fmt.Printf("aex is not in the %s, nothing to remove.\n", shortcut.Where())
		return nil
	}
	if err := shortcut.Remove(); err != nil {
		return err
	}
	fmt.Println(ui.Out.Green("Removed from the " + shortcut.Where()))
	return nil
}
