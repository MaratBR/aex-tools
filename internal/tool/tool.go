// Package tool is what every tool module (internal/tools/<name>) exports, plus shared arg parsing.
package tool

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// Tool is one menu entry, also runnable as "aex <name> [args]".
type Tool struct {
	Name    string
	Summary string
	Run     func(args []string) error
}

// ParseFlags parses a tool's args, printing usage on --help / -h (then done is true).
// Positional args are not accepted.
func ParseFlags(fs *flag.FlagSet, args []string, usage string) (done bool, err error) {
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Println(usage)
			return true, nil
		}
		return false, fmt.Errorf("%v (see --help)", err)
	}
	if fs.NArg() > 0 {
		return false, fmt.Errorf("unexpected argument %q (see --help)", fs.Arg(0))
	}
	return false, nil
}
