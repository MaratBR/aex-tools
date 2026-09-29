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
	return fmt.Sprintf(`Usage: configure

Asks for these settings and saves them to app settings (%s):
%s
App settings have the highest priority: they override .env, .env.private and real environment
variables, which remain fallbacks. Leave an answer empty to keep the current value, or enter "-" to
remove it from app settings (settings with a default are reset to it).`, settings.ConfigEnvFile, list.String())
}

// Tool is the configure tool.
var Tool = tool.Tool{Name: "configure", Summary: "Set AEXT / Jira login, hours per day and timezone (app settings)", Run: run}

func run(args []string) error {
	if done, err := tool.ParseFlags(flag.NewFlagSet("configure", flag.ContinueOnError), args, configureHelp()); done || err != nil {
		return err
	}
	if err := ui.AssertInteractive("configure"); err != nil {
		return err
	}
	out := ui.Out
	fmt.Printf("App settings: %s\n\n", out.Cyan(settings.ConfigEnvFile))

	type change struct{ name, before, after string }
	var updates []settings.Change
	var changed []change

	for _, s := range settings.All {
		layers := settings.Layers()
		fallbacks := layers[:len(layers)-1]
		_, inAppSettings := layers[len(layers)-1].Vars[s.Name]
		current := settings.Get(s.Name)
		shown := "not set"
		if current != "" {
			shown = current
			if s.Secret {
				shown = settings.Mask(current)
			}
		}
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
			Description: fmt.Sprintf("%s\nCurrent: %s · empty keeps it, %s", s.Hint, shown, removeLabel),
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
		if input == "-" && s.Default != "" {
			input = s.Default
		}
		if input == "" || (input == "-" && !inAppSettings) {
			fmt.Println()
			continue
		}

		if input == "-" {
			updates = append(updates, settings.Change{Name: s.Name})
			settings.Unset(s.Name)
			// Fall back to the next source down, as a restart would.
			for i := len(fallbacks) - 1; i >= 0; i-- {
				if v := fallbacks[i].Vars[s.Name]; v != "" {
					settings.Set(s.Name, v)
					break
				}
			}
		} else {
			var overridden []string
			for _, l := range fallbacks {
				if l.Vars[s.Name] != "" {
					overridden = append(overridden, l.Label)
				}
			}
			if len(overridden) > 0 {
				fmt.Fprintf(os.Stderr, "  %s %s is also set in %s; app settings will override it.\n",
					ui.Err.Yellow("Warning:"), s.Name, strings.Join(overridden, ", "))
				save, err := ui.Confirm("  Save anyway?", true)
				if err != nil {
					return err
				}
				if !save {
					fmt.Println()
					continue
				}
			}
			value := input
			updates = append(updates, settings.Change{Name: s.Name, Value: &value})
			settings.Set(s.Name, input)
		}
		changed = append(changed, change{s.Name, current, settings.Get(s.Name)})
		fmt.Println()
	}

	if len(changed) == 0 {
		fmt.Println("Nothing changed.")
		return nil
	}
	if err := settings.SaveAppSettings(updates); err != nil {
		return err
	}
	names := make([]string, len(changed))
	for i, c := range changed {
		names[i] = c.name
	}
	fmt.Println(out.Green(fmt.Sprintf("Saved %s", strings.Join(names, ", "))))

	// The cached AEXT session belongs to the old email.
	for _, c := range changed {
		if c.name != "AEXT_EMAIL" || c.before == "" || c.before == c.after {
			continue
		}
		client, err := aext.New()
		if err != nil {
			return err
		}
		if !client.HasSession() {
			continue
		}
		wipe, err := ui.Confirm("AEXT email changed. Wipe the AEXT session of the old email?", true)
		if err != nil {
			return err
		}
		if wipe {
			if err := client.WipeSession(); err != nil {
				return err
			}
			fmt.Println("Session wiped; the next AEXT tool will ask for a login code.")
		}
	}
	return nil
}
