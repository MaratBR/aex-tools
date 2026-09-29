package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"aex/internal/jira"
	"aex/internal/settings"
	"aex/internal/ui"
)

type person struct {
	Name      string `json:"name"`
	AccountID string `json:"accountId"`
}

// What to do with tickets assigned to an excluded person.
const (
	modeAsk  = "ask"  // ask each run whether to include them
	modeSkip = "skip" // skip them without asking
)

type excluded struct {
	person
	Mode string `json:"mode"`
}

// config is the tool's own settings, kept in the data folder and changed from its settings menu.
type config struct {
	// DefaultPrefix is the project key a bare ticket number is taken to be in: 1234 is CM-1234.
	DefaultPrefix string `json:"defaultPrefix"`
	// DefaultQA gets tickets with no QA owner.
	DefaultQA person `json:"defaultQA"`
	// Cc is added to every handoff comment, except whoever it is addressed to.
	Cc []person `json:"cc"`
	// Excluded are people whose tickets are left alone (Mode says whether to ask first).
	Excluded []excluded `json:"excluded"`
}

// Written on first run.
func defaultConfig() config {
	var (
		alexander = person{"Alexander Martynets", "712020:52b023b0-e2c5-4cc9-9894-effc0b4c309c"}
		heriberto = person{"Heriberto Barajas", "5ff603a7849d6401114c6e23"}
		bulat     = person{"Bulat Khaydurov", "628c196bf2261e00682999df"}
	)
	return config{DefaultPrefix: "CM", DefaultQA: heriberto, Cc: []person{alexander, heriberto}, Excluded: []excluded{{bulat, modeAsk}}}
}

func configFile() string {
	return filepath.Join(settings.DataDir, "plugin-settings", "jira-release-handoff.json")
}

// loadConfig reads the settings, writing the defaults first when there are none yet.
func loadConfig() (*config, error) {
	file := configFile()
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		c := defaultConfig()
		if err := c.save(); err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", ui.Err.Dim("Wrote default settings to"), file)
		return &c, nil
	}
	if err != nil {
		return nil, err
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	// Settings added since the file was written get their defaults.
	if c.DefaultPrefix == "" {
		c.DefaultPrefix = defaultConfig().DefaultPrefix
		if err := c.save(); err != nil {
			return nil, err
		}
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w (fix it in the file or with --settings)", file, err)
	}
	return &c, nil
}

var projectKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func (c *config) validate() error {
	if !projectKey.MatchString(c.DefaultPrefix) {
		return fmt.Errorf("defaultPrefix must be a project key like CM, got %q", c.DefaultPrefix)
	}
	if c.DefaultQA.AccountID == "" {
		return errors.New("defaultQA needs an accountId")
	}
	for _, p := range c.Cc {
		if p.AccountID == "" {
			return fmt.Errorf("cc %q needs an accountId", p.Name)
		}
	}
	for _, e := range c.Excluded {
		if e.AccountID == "" {
			return fmt.Errorf("excluded %q needs an accountId", e.Name)
		}
		if e.Mode != modeAsk && e.Mode != modeSkip {
			return fmt.Errorf("excluded %q: mode must be %q or %q, got %q", e.Name, modeAsk, modeSkip, e.Mode)
		}
	}
	return nil
}

func (c *config) save() error {
	file := configFile()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(data, '\n'), 0o644)
}

func names(people []person) string {
	if len(people) == 0 {
		return "nobody"
	}
	s := make([]string, len(people))
	for i, p := range people {
		s[i] = p.Name
	}
	return strings.Join(s, ", ")
}

func modeLabel(mode string) string {
	if mode == modeSkip {
		return "skip automatically"
	}
	return "ask each run"
}

// editConfig is the settings menu; every change is saved right away.
func editConfig(j *jira.Client, c *config) error {
	fmt.Printf("Settings: %s\n", ui.Out.Cyan(configFile()))
	for {
		var ex []string
		for _, e := range c.Excluded {
			ex = append(ex, fmt.Sprintf("%s (%s)", e.Name, modeLabel(e.Mode)))
		}
		if len(ex) == 0 {
			ex = []string{"nobody"}
		}
		choice, err := ui.Choose("Settings", []ui.Option{
			{Label: "Default project prefix: " + c.DefaultPrefix + " (1234 means " + c.DefaultPrefix + "-1234)", Value: "prefix"},
			{Label: "Default QA: " + c.DefaultQA.Name, Value: "qa"},
			{Label: "Always Cc: " + names(c.Cc), Value: "cc"},
			{Label: "Excluded assignees: " + strings.Join(ex, ", "), Value: "excluded"},
			{Label: "Done", Value: "done"},
		})
		if err != nil {
			return err
		}
		switch choice {
		case "done":
			return nil
		case "prefix":
			prefix, err := ui.Input(ui.Field{
				Title:       "Default project prefix",
				Description: "Project key a bare ticket number belongs to",
				Placeholder: c.DefaultPrefix,
				Validate: func(s string) error {
					if !projectKey.MatchString(strings.ToUpper(s)) {
						return errors.New("enter a project key like CM")
					}
					return nil
				},
			})
			if err != nil {
				return err
			}
			c.DefaultPrefix = strings.ToUpper(prefix)
		case "qa":
			p, err := pickPerson(j, "Default QA")
			if err != nil || p == nil {
				return err
			}
			c.DefaultQA = *p
		case "cc":
			if err := editCc(j, c); err != nil {
				return err
			}
		case "excluded":
			if err := editExcluded(j, c); err != nil {
				return err
			}
		}
		if err := c.save(); err != nil {
			return err
		}
	}
}

func editCc(j *jira.Client, c *config) error {
	for {
		options := []ui.Option{{Label: "Add someone", Value: "add"}}
		for i, p := range c.Cc {
			options = append(options, ui.Option{Label: "Remove " + p.Name, Value: fmt.Sprint(i)})
		}
		options = append(options, ui.Option{Label: "Back", Value: "back"})
		choice, err := ui.Choose("Always Cc: "+names(c.Cc), options)
		if err != nil || choice == "back" {
			return err
		}
		if choice != "add" {
			var i int
			fmt.Sscan(choice, &i)
			c.Cc = slices.Delete(c.Cc, i, i+1)
			continue
		}
		p, err := pickPerson(j, "Add to Cc")
		if err != nil {
			return err
		}
		if p == nil {
			continue
		}
		if slices.ContainsFunc(c.Cc, func(q person) bool { return q.AccountID == p.AccountID }) {
			fmt.Println(p.Name + " is already in Cc.")
			continue
		}
		c.Cc = append(c.Cc, *p)
	}
}

func editExcluded(j *jira.Client, c *config) error {
	for {
		options := []ui.Option{{Label: "Add someone", Value: "add"}}
		for i, e := range c.Excluded {
			options = append(options, ui.Option{Label: fmt.Sprintf("%s (%s)", e.Name, modeLabel(e.Mode)), Value: fmt.Sprint(i)})
		}
		options = append(options, ui.Option{Label: "Back", Value: "back"})
		choice, err := ui.Choose("Excluded assignees: their tickets are left alone", options)
		if err != nil || choice == "back" {
			return err
		}
		if choice == "add" {
			p, err := pickPerson(j, "Exclude tickets assigned to")
			if err != nil {
				return err
			}
			if p == nil {
				continue
			}
			if slices.ContainsFunc(c.Excluded, func(e excluded) bool { return e.AccountID == p.AccountID }) {
				fmt.Println(p.Name + " is already excluded.")
				continue
			}
			mode, err := chooseMode(p.Name)
			if err != nil {
				return err
			}
			c.Excluded = append(c.Excluded, excluded{*p, mode})
			continue
		}
		var i int
		fmt.Sscan(choice, &i)
		action, err := ui.Choose(c.Excluded[i].Name, []ui.Option{
			{Label: "Change what to do with their tickets", Value: "mode"},
			{Label: "Remove (hand their tickets off like any other)", Value: "remove"},
			{Label: "Back", Value: "back"},
		})
		if err != nil {
			return err
		}
		switch action {
		case "mode":
			if c.Excluded[i].Mode, err = chooseMode(c.Excluded[i].Name); err != nil {
				return err
			}
		case "remove":
			c.Excluded = slices.Delete(c.Excluded, i, i+1)
		}
	}
}

func chooseMode(name string) (string, error) {
	return ui.Choose("Tickets assigned to "+name, []ui.Option{
		{Label: "Ask each run whether to include them", Value: modeAsk},
		{Label: "Skip them automatically", Value: modeSkip},
	})
}

// pickPerson finds a Jira user by name or email; nil when cancelled.
func pickPerson(j *jira.Client, title string) (*person, error) {
	for {
		query, err := ui.Input(ui.Field{Title: title, Description: "Name or email to search Jira for (empty: cancel)"})
		if err != nil || query == "" {
			return nil, err
		}
		users, err := j.SearchUsers(query)
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			fmt.Printf("No one found for %q.\n", query)
			continue
		}
		options := make([]ui.Option, 0, len(users)+2)
		for i, u := range users {
			label := u.DisplayName
			if u.EmailAddress != "" {
				label += "  " + u.EmailAddress
			}
			options = append(options, ui.Option{Label: label, Value: fmt.Sprint(i)})
		}
		options = append(options, ui.Option{Label: "Search again", Value: "again"}, ui.Option{Label: "Cancel", Value: "cancel"})
		choice, err := ui.Choose(title, options)
		if err != nil || choice == "cancel" {
			return nil, err
		}
		if choice == "again" {
			continue
		}
		var i int
		fmt.Sscan(choice, &i)
		return &person{users[i].DisplayName, users[i].AccountID}, nil
	}
}
