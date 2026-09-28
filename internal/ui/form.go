package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Brand is the logo blue, shared by the menu and the prompts.
var Brand = lipgloss.Color("#039EEC")

var (
	faintStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "243"})
	doneStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	boldStyle  = lipgloss.NewStyle().Bold(true)
)

// Plain turns off the interactive prompts, as --plain and AEX_TUI=0 do: questions are then asked
// line by line, as they always are when stdin or stderr is not a terminal.
var Plain = os.Getenv("AEX_TUI") == "0"

func fancy() bool {
	return !Plain && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// Field describes a text prompt.
type Field struct {
	Title       string
	Description string // shown under the title
	Placeholder string // shown faint while the answer is empty
	Secret      bool   // input is not echoed
	Validate    func(string) error
}

// Option is one choice of Choose.
type Option struct {
	Label string
	Value string
}

// Ask prints question and returns the answer, trimmed.
func Ask(question string) (string, error) { return Input(Field{Title: question}) }

// PromptSecret asks for a value without echoing it.
func PromptSecret(question string) (string, error) {
	return Input(Field{Title: question, Secret: true})
}

// Input asks for a line of text, trimmed. Validate errors are shown and the question asked again.
func Input(f Field) (string, error) {
	f.Title = cleanTitle(f.Title)
	check := func(s string) error {
		if f.Validate == nil {
			return nil
		}
		return f.Validate(strings.TrimSpace(s))
	}
	if !fancy() {
		return lineInput(f, check)
	}
	var value string
	in := huh.NewInput().Title(f.Title).Description(f.Description).Placeholder(f.Placeholder).
		Validate(check).Value(&value)
	if f.Secret {
		in.EchoMode(huh.EchoModePassword)
	}
	if err := run(in); err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	shown := value
	switch {
	case f.Secret && value != "":
		shown = "••••••••"
	case value == "":
		shown = faintStyle.Render("(empty)")
	}
	answered(f.Title, shown)
	return value, nil
}

// Confirm asks a yes/no question; defaultYes picks the answer Enter gives.
func Confirm(question string, defaultYes bool) (bool, error) {
	question = cleanTitle(question)
	if !fancy() {
		return lineConfirm(question, defaultYes)
	}
	yes := defaultYes
	if err := run(huh.NewConfirm().Title(question).Affirmative("Yes").Negative("No").Value(&yes)); err != nil {
		return false, err
	}
	answered(question, map[bool]string{true: "Yes", false: "No"}[yes])
	return yes, nil
}

// Choose asks to pick one of options and returns its Value; the first option is preselected.
func Choose(title string, options []Option) (string, error) {
	title = cleanTitle(title)
	if !fancy() {
		return lineChoose(title, options)
	}
	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o.Label, o.Value)
	}
	var value string
	if err := run(huh.NewSelect[string]().Title(title).Options(opts...).Value(&value)); err != nil {
		return "", err
	}
	for _, o := range options {
		if o.Value == value {
			answered(title, strings.TrimSpace(o.Label))
		}
	}
	return value, nil
}

func run(field huh.Field) error {
	keys := huh.NewDefaultKeyMap()
	keys.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"), key.WithHelp("esc", "cancel"))
	err := huh.NewForm(huh.NewGroup(field)).
		WithTheme(theme()).WithKeyMap(keys).WithOutput(os.Stderr).WithShowHelp(true).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return errors.New("cancelled")
	}
	return err
}

// answered leaves a one-line record of the question and answer; the form itself clears on submit.
func answered(title, value string) {
	fmt.Fprintf(os.Stderr, "%s %s %s\n", doneStyle.Render("✔"), faintStyle.Render(title), boldStyle.Render(value))
}

// cleanTitle drops the indent and trailing ": " / "> " written for line prompts.
func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimSpace(strings.TrimRight(s, ":>"))
}

func theme() *huh.Theme {
	t := huh.ThemeBase()
	var (
		text   = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
		subtle = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
		white  = lipgloss.Color("#FFFFFF")
		red    = lipgloss.Color("1")
	)
	f := &t.Focused
	f.Base = f.Base.BorderForeground(Brand)
	f.Card = f.Base
	f.Title = f.Title.Foreground(Brand).Bold(true)
	f.Description = f.Description.Foreground(subtle)
	f.ErrorIndicator = f.ErrorIndicator.Foreground(red)
	f.ErrorMessage = f.ErrorMessage.Foreground(red)
	f.SelectSelector = f.SelectSelector.Foreground(Brand).SetString("▶ ")
	f.Option = f.Option.Foreground(text)
	f.SelectedOption = f.SelectedOption.Foreground(Brand).Bold(true)
	f.NextIndicator = f.NextIndicator.Foreground(Brand)
	f.PrevIndicator = f.PrevIndicator.Foreground(Brand)
	f.FocusedButton = f.FocusedButton.Foreground(white).Background(Brand).Bold(true)
	f.BlurredButton = f.BlurredButton.Foreground(text).Background(lipgloss.AdaptiveColor{Light: "252", Dark: "237"})
	f.TextInput.Cursor = f.TextInput.Cursor.Foreground(Brand)
	f.TextInput.Prompt = f.TextInput.Prompt.Foreground(Brand)
	f.TextInput.Placeholder = f.TextInput.Placeholder.Foreground(subtle)
	f.TextInput.Text = f.TextInput.Text.Foreground(text)
	t.Blurred = *f
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Help.ShortKey = t.Help.ShortKey.Foreground(subtle).Bold(true)
	t.Help.ShortDesc = t.Help.ShortDesc.Foreground(subtle)
	t.Help.ShortSeparator = t.Help.ShortSeparator.Foreground(subtle)
	return t
}

// Required is a Field.Validate that rejects an empty answer.
func Required(what string) func(string) error {
	return func(s string) error {
		if s == "" {
			return fmt.Errorf("enter the %s", what)
		}
		return nil
	}
}
