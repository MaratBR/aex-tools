// Package tool is what every tool module (internal/tools/<name>) exports, plus shared arg parsing.
package tool

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"aex/internal/ui"
)

// Tool is one entry of the tool list, also runnable as "aex <name> [args]".
//
// A tool with Sub is a group: it does nothing on its own (Run is unused), it only holds its
// sub-tools, run as "aex <group> <sub-tool> [args]".
type Tool struct {
	Name    string
	Summary string
	Run     func(args []string) error
	Sub     []Tool
	// Hidden keeps the tool out of the window's tool list, for a tool the window has its own UI
	// for; it still runs as "aex <name>".
	Hidden bool
	// Warn, when set, is why the window marks the tool with a warning icon (a plugin not approved).
	Warn string
}

// IsGroup reports whether the tool is a group of sub-tools.
func (t *Tool) IsGroup() bool { return len(t.Sub) > 0 }

// Find takes a sub-tool name or its number in the group.
func (t *Tool) Find(input string) *Tool {
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(t.Sub) {
		return &t.Sub[n-1]
	}
	for i := range t.Sub {
		if t.Sub[i].Name == input {
			return &t.Sub[i]
		}
	}
	return nil
}

// Exec runs the tool with args. A group runs the sub-tool args[0] names (or numbers), asked for
// when left out on an interactive console, and prints its tools on --help / -h.
func (t *Tool) Exec(args []string) error {
	if !t.IsGroup() {
		return t.Run(args)
	}
	if len(args) == 0 {
		if !ui.IsInteractive() {
			return fmt.Errorf("%s is a group of tools, name one: %s (see --help)", t.Name, t.subNames())
		}
		options := make([]ui.Option, len(t.Sub))
		for i, s := range t.Sub {
			options[i] = ui.Option{Label: s.Name, Value: s.Name, Summary: s.Summary, Group: s.IsGroup()}
		}
		name, err := ui.PickTool("Which "+t.Name+" tool?", options)
		if err != nil {
			return err
		}
		args = []string{name}
	}
	if args[0] == "--help" || args[0] == "-h" {
		fmt.Println(t.GroupUsage())
		return nil
	}
	sub := t.Find(args[0])
	if sub == nil {
		return fmt.Errorf("%s has no tool %q, only: %s (see --help)", t.Name, args[0], t.subNames())
	}
	return sub.Exec(args[1:])
}

func (t *Tool) subNames() string {
	names := make([]string, len(t.Sub))
	for i, s := range t.Sub {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}

// GroupUsage is a group's --help: its sub-tools.
func (t *Tool) GroupUsage() string {
	w := 12
	for _, s := range t.Sub {
		w = max(w, len(s.Name))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: %s <tool> [args...]\n\n%s\n\nTools:\n", t.Name, t.Summary)
	for _, s := range t.Sub {
		fmt.Fprintf(&b, "  %-*s%s\n", w+2, s.Name, s.Summary)
	}
	fmt.Fprintf(&b, "\nTry \"%s <tool> --help\".", t.Name)
	return b.String()
}

// ParseFlags parses a tool's args, printing usage on --help / -h (then done is true).
// Positional args are not accepted.
func ParseFlags(fs *flag.FlagSet, args []string, usage string) (done bool, err error) {
	if done, err := ParseFlagsAndArgs(fs, args, usage); done || err != nil {
		return done, err
	}
	if fs.NArg() > 0 {
		return false, fmt.Errorf("unexpected argument %q (see --help)", fs.Arg(0))
	}
	return false, nil
}

// ParseFlagsAndArgs is ParseFlags that leaves positional args in fs.Args().
func ParseFlagsAndArgs(fs *flag.FlagSet, args []string, usage string) (done bool, err error) {
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Println(usage)
			return true, nil
		}
		return false, fmt.Errorf("%v (see --help)", err)
	}
	return false, nil
}
