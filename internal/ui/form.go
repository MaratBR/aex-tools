package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Brand is the logo blue of the prompts.
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
	// Paste, when set, rewrites the whole answer right after text is pasted into it (not when it is
	// typed; line prompts never rewrite). Validate and callers must still accept what it rewrites.
	Paste func(string) string
	// Describe, when set, gives a line shown under the box as the answer is typed ("" shows
	// nothing). Called off the UI goroutine, once per value, so it may be slow; line prompts skip it.
	Describe func(string) string
	// Hint says what kind of text the answer is, for a UI that can help enter it: the window offers
	// a file or folder dialog and takes a file dropped on it. Terminals ignore it.
	Hint Hint
}

// Hint is what kind of text an Input answer is.
type Hint struct {
	Kind HintKind `json:"kind,omitempty"`
	// Filters are the files a HintFile dialog offers (all files when empty).
	Filters []FileFilter `json:"filters,omitempty"`
}

// HintKind is a kind of answer.
type HintKind string

const (
	HintText   HintKind = ""       // any text
	HintFile   HintKind = "file"   // a file's path
	HintFolder HintKind = "folder" // a folder's path
)

// FileFilter is a kind of file a file dialog offers, e.g. {"PowerShell scripts", ["*.ps1"]}.
type FileFilter struct {
	Name     string   `json:"name"`
	Patterns []string `json:"patterns"`
}

// Option is one choice of Choose (tagged for the window's frontend, which reads label and value).
type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
	// Summary and Group are for PickTool: what the tool does, and whether it is a group of tools.
	Summary string `json:"summary,omitempty"`
	Group   bool   `json:"group,omitempty"`
}

// asking lets one question be asked at a time: tools run together (a composed tool's parallel
// steps) may ask at once, and each waits for the other's answer.
var asking sync.Mutex

// Ask prints question and returns the answer, trimmed.
func Ask(question string) (string, error) { return Input(Field{Title: question}) }

// PromptSecret asks for a value without echoing it.
func PromptSecret(question string) (string, error) {
	return Input(Field{Title: question, Secret: true})
}

// Input asks for a line of text, trimmed. Validate errors are shown and the question asked again.
func Input(f Field) (string, error) {
	asking.Lock()
	defer asking.Unlock()
	f.Title = cleanTitle(f.Title)
	check := func(s string) error {
		if f.Validate == nil {
			return nil
		}
		return f.Validate(strings.TrimSpace(s))
	}
	if Remote != nil {
		return Remote.Input(f)
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
	var field huh.Field = in
	if f.Paste != nil || f.Describe != nil {
		field = newLiveInput(in, &value, f)
	}
	if err := run(field); err != nil {
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
	asking.Lock()
	defer asking.Unlock()
	question = cleanTitle(question)
	if Remote != nil {
		return Remote.Confirm(question, defaultYes)
	}
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
	asking.Lock()
	defer asking.Unlock()
	title = cleanTitle(title)
	if Remote != nil {
		return Remote.Choose(title, options)
	}
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

// PickTool asks which tool of a group to run and returns its Value. Each option is a tool: Label
// its name, Summary what it does. The window shows it as a tool list, a console as a Choose.
func PickTool(title string, tools []Option) (string, error) {
	title = cleanTitle(title)
	if Remote != nil {
		asking.Lock()
		defer asking.Unlock()
		return Remote.PickTool(title, tools)
	}
	options := make([]Option, len(tools))
	for i, t := range tools {
		options[i] = Option{Label: t.Label + "  " + Err.Dim(t.Summary), Value: t.Value}
	}
	return Choose(title, options)
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
