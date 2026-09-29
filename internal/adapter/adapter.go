// Package adapter is what turns a script into a custom tool (internal/custom): a tool adapter knows
// one kind of script (PowerShell, ...), reads the parameters it accepts without running it, and
// makes the command that runs it with values for them. aex parses a custom tool's args itself
// (Bind), the same way for every adapter, and asks for the parameters left out.
package adapter

import (
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// Adapter runs one kind of script as a tool.
type Adapter interface {
	// Name is the adapter's name, e.g. "powershell", kept with each custom tool.
	Name() string
	// Handles reports whether the adapter runs the file at path (by its extension).
	Handles(path string) bool
	// FileTypes names the files it runs and their patterns, for file dialogs: "PowerShell scripts",
	// ["*.ps1"].
	FileTypes() (name string, patterns []string)
	// Describe reads what the script is and the parameters it accepts, without running it. The
	// caller holds the file open so it cannot change meanwhile (see plugin.Lock).
	Describe(path string) (Description, error)
	// Command is the command that runs the script at path, described as d, with args (in the order
	// of d.Params) and then rest, in session s.
	Command(path string, d Description, args []Arg, rest []string, s Session) (*exec.Cmd, error)
}

// Session is where a script runs.
type Session struct {
	// Console is true when the script has the terminal. Else (the window) its output goes to a pipe
	// and it has no input.
	Console bool
	// Prompts is true for an Interactive script without a console: its environment then has where
	// to send its questions (plugin.ServePrompts, the protocol plugins use), and the adapter makes the
	// script's own way of asking (e.g. Read-Host) use it.
	Prompts bool
}

// Description is a script's help and parameters, as the adapter reads them.
type Description struct {
	Summary string
	Params  []Param
	// Rest is true when the script takes positional args it does not declare (it has no parameters).
	Rest bool
	// Runner is what runs the script, e.g. "powershell.exe".
	Runner string
	// Interactive is true when the script may ask questions while it runs, as the adapter decides
	// (PowerShell: always, Read-Host can be anywhere). Without a console aex answers them (Session).
	Interactive bool
	// Notes are things the adapter could not model, e.g. parameter sets.
	Notes []string
}

// Kind is how aex asks for a parameter and checks its value.
type Kind string

const (
	String Kind = "string"
	Int    Kind = "int"
	Number Kind = "number"
	Bool   Kind = "bool"   // takes a value: true or false
	Switch Kind = "switch" // on when named, no value
	List   Kind = "list"   // several values, comma-separated
)

// Param is one parameter a script accepts.
type Param struct {
	Name     string   `json:"name"`
	Kind     Kind     `json:"kind"`
	Type     string   `json:"type"` // as the script declares it, e.g. "datetime"
	Required bool     `json:"required"`
	Default  string   `json:"default"` // as the script gives it, "" for none
	Choices  []string `json:"choices"`
	Help     string   `json:"help"`
	Aliases  []string `json:"aliases"`
	// Hint is what a String value names, so the window can offer a dialog for it: PathFile,
	// PathFolder or "" for any text.
	Hint string `json:"hint"`
}

// Param.Hint values.
const (
	PathFile   = "file"
	PathFolder = "folder"
)

// Arg is a value given for a parameter.
type Arg struct {
	Param *Param
	Text  string   // String, Int, Number
	List  []string // List
	On    bool     // Bool, Switch
}

// Bind parses args for d's parameters: "-Name value" or "--name value", "-Name:value" or
// "--name=value" (names ignore case; a unique prefix or an alias works too), a switch alone or with
// ":true" / "=false", lists comma-separated; positional values fill the parameters not named yet, in
// order. Returned args are in the order of d.Params. rest is positional values of a script with Rest.
func Bind(d Description, args []string) (bound []Arg, rest []string, err error) {
	byParam := map[*Param]Arg{}
	var positional []string
	for i := 0; i < len(args); i++ {
		name, value, hasValue, isName := splitName(args[i])
		if !isName {
			positional = append(positional, args[i])
			continue
		}
		p, err := find(d.Params, name)
		if err != nil {
			return nil, nil, err
		}
		if _, dup := byParam[p]; dup {
			return nil, nil, fmt.Errorf("-%s given twice", p.Name)
		}
		switch {
		case hasValue:
		case p.Kind == Switch:
			value = "true"
		case i+1 < len(args):
			i++
			value = args[i]
		default:
			return nil, nil, fmt.Errorf("-%s needs a value", p.Name)
		}
		if byParam[p], err = Parse(p, value); err != nil {
			return nil, nil, err
		}
	}
	for _, v := range positional {
		var free *Param
		for i := range d.Params {
			if _, done := byParam[&d.Params[i]]; !done && d.Params[i].Kind != Switch {
				free = &d.Params[i]
				break
			}
		}
		if free == nil {
			if !d.Rest {
				return nil, nil, fmt.Errorf("unexpected argument %q (see --help)", v)
			}
			rest = append(rest, v)
			continue
		}
		if byParam[free], err = Parse(free, v); err != nil {
			return nil, nil, err
		}
	}
	for i := range d.Params {
		if arg, ok := byParam[&d.Params[i]]; ok {
			bound = append(bound, arg)
		}
	}
	return bound, rest, nil
}

// splitName splits "-Name", "--name", "-Name:value" and "--name=value". A lone "-", or a value that
// looks like a negative number, is not a name.
func splitName(a string) (name, value string, hasValue, isName bool) {
	if len(a) < 2 || a[0] != '-' {
		return "", "", false, false
	}
	if _, err := strconv.ParseFloat(a, 64); err == nil {
		return "", "", false, false
	}
	name = strings.TrimPrefix(a[1:], "-")
	if n, v, ok := strings.Cut(name, "="); ok {
		return n, v, true, n != ""
	}
	if n, v, ok := strings.Cut(name, ":"); ok {
		return n, v, true, n != ""
	}
	return name, "", false, name != ""
}

// find is the parameter named name: its name or an alias (ignoring case), else a unique prefix.
func find(params []Param, name string) (*Param, error) {
	var prefixed []*Param
	for i := range params {
		p := &params[i]
		if strings.EqualFold(p.Name, name) || slices.ContainsFunc(p.Aliases, func(a string) bool { return strings.EqualFold(a, name) }) {
			return p, nil
		}
		if len(p.Name) > len(name) && strings.EqualFold(p.Name[:len(name)], name) {
			prefixed = append(prefixed, p)
		}
	}
	switch len(prefixed) {
	case 1:
		return prefixed[0], nil
	case 0:
		return nil, fmt.Errorf("no parameter -%s (see --help)", name)
	}
	names := make([]string, len(prefixed))
	for i, p := range prefixed {
		names[i] = "-" + p.Name
	}
	return nil, fmt.Errorf("-%s could be %s", name, strings.Join(names, ", "))
}

// Parse checks value for p and turns it into an Arg. A choice is given as the script spells it.
func Parse(p *Param, value string) (Arg, error) {
	arg := Arg{Param: p}
	value = strings.TrimSpace(value)
	fail := func(want string) (Arg, error) {
		return Arg{}, fmt.Errorf("-%s: %q is not %s", p.Name, value, want)
	}
	switch p.Kind {
	case Bool, Switch:
		on, ok := ParseBool(value)
		if !ok {
			return fail("true or false")
		}
		arg.On = on
		return arg, nil
	case List:
		for v := range strings.SplitSeq(value, ",") {
			if v = strings.TrimSpace(v); v != "" {
				c, err := choice(p, v)
				if err != nil {
					return Arg{}, err
				}
				arg.List = append(arg.List, c)
			}
		}
		return arg, nil
	case Int:
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fail("a whole number")
		}
	case Number:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fail("a number")
		}
	}
	var err error
	arg.Text, err = choice(p, value)
	return arg, err
}

// choice is v as one of p's choices (ignoring case), or v when p has none.
func choice(p *Param, v string) (string, error) {
	if len(p.Choices) == 0 {
		return v, nil
	}
	if i := slices.IndexFunc(p.Choices, func(c string) bool { return strings.EqualFold(c, v) }); i >= 0 {
		return p.Choices[i], nil
	}
	return "", fmt.Errorf("-%s: %q is not one of %s", p.Name, v, strings.Join(p.Choices, ", "))
}

// ParseBool takes true / false, yes / no, 1 / 0, on / off and $true / $false, ignoring case.
func ParseBool(s string) (on, ok bool) {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "$")) {
	case "true", "yes", "y", "1", "on":
		return true, true
	case "false", "no", "n", "0", "off":
		return false, true
	}
	return false, false
}

// Missing lists the required parameters args has no value for.
func Missing(d Description, args []Arg) []*Param {
	var missing []*Param
	for i := range d.Params {
		p := &d.Params[i]
		if p.Required && !slices.ContainsFunc(args, func(a Arg) bool { return a.Param == p }) {
			missing = append(missing, p)
		}
	}
	return missing
}

// Usage is a custom tool's --help: its parameters.
func Usage(name string, d Description) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: %s", name)
	for _, p := range d.Params {
		arg := "-" + p.Name
		if p.Kind != Switch {
			arg += " <" + p.Type + ">"
		}
		if !p.Required {
			arg = "[" + arg + "]"
		}
		b.WriteString(" " + arg)
	}
	if d.Rest {
		b.WriteString(" [args...]")
	}
	b.WriteString("\n")
	if d.Summary != "" {
		fmt.Fprintf(&b, "\n%s\n", d.Summary)
	}
	if len(d.Params) > 0 {
		b.WriteString("\nParameters:\n")
		w := 0
		for _, p := range d.Params {
			w = max(w, len(p.Name))
		}
		indent := "\n" + strings.Repeat(" ", w+5)
		for _, p := range d.Params {
			fmt.Fprintf(&b, "  -%-*s  %s\n", w, p.Name, DescribeParam(p))
			if p.Help != "" {
				fmt.Fprintf(&b, "%s%s\n", indent[1:], strings.ReplaceAll(p.Help, "\n", indent))
			}
		}
		b.WriteString("\nNames ignore case and may be shortened; --name=value works too. Lists are comma-separated.\n" +
			"Without args, asks for each parameter; required ones left out are asked for.\n")
	}
	for _, n := range d.Notes {
		fmt.Fprintf(&b, "\nNote: %s", n)
	}
	return strings.TrimRight(b.String(), "\n")
}

// DescribeParam is a parameter's type, whether it is required, its default, choices and aliases.
func DescribeParam(p Param) string {
	parts := []string{p.Type}
	if p.Required {
		parts = append(parts, "required")
	}
	if p.Default != "" {
		parts = append(parts, "default "+p.Default)
	}
	if len(p.Choices) > 0 {
		parts = append(parts, "one of "+strings.Join(p.Choices, ", "))
	}
	if len(p.Aliases) > 0 {
		parts = append(parts, "alias "+strings.Join(p.Aliases, ", "))
	}
	return strings.Join(parts, ", ")
}
