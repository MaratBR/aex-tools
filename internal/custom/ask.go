package custom

import (
	"fmt"
	"slices"
	"strings"

	"aex/internal/adapter"
	"aex/internal/ui"
)

// skip is the Choose value that leaves an optional parameter out.
const skip = "\x00skip"

// ask asks for the parameters bound has no value for: every one when all, else the required ones.
// Without a terminal, missing required ones fail instead. The result is in the order of d.Params.
func ask(d adapter.Description, bound []adapter.Arg, all bool) ([]adapter.Arg, error) {
	missing := adapter.Missing(d, bound)
	if !all && len(missing) == 0 {
		return bound, nil
	}
	if !ui.IsInteractive() {
		if len(missing) == 0 {
			return bound, nil
		}
		names := make([]string, len(missing))
		for i, p := range missing {
			names[i] = "-" + p.Name
		}
		return nil, fmt.Errorf("missing %s (see --help)", strings.Join(names, ", "))
	}
	var out []adapter.Arg
	for i := range d.Params {
		p := &d.Params[i]
		if n := slices.IndexFunc(bound, func(a adapter.Arg) bool { return a.Param == p }); n >= 0 {
			out = append(out, bound[n])
			continue
		}
		if !all && !p.Required {
			continue
		}
		arg, ok, err := askParam(p)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, arg)
		}
	}
	return out, nil
}

// askParam asks for p's value; ok is false when an optional one is left out (the script's default).
func askParam(p *adapter.Param) (arg adapter.Arg, ok bool, err error) {
	title := p.Name
	if p.Help != "" {
		title += " — " + firstLine(p.Help)
	}
	switch {
	case p.Kind == adapter.Switch || p.Kind == adapter.Bool:
		def, _ := adapter.ParseBool(p.Default)
		on, err := ui.Confirm(title+"?", def)
		return adapter.Arg{Param: p, On: on}, err == nil, err

	case len(p.Choices) > 0 && p.Kind != adapter.List:
		var options []ui.Option
		if !p.Required {
			label := "(leave out)"
			if p.Default != "" {
				label = p.Default + " (default)"
			}
			options = append(options, ui.Option{Label: label, Value: skip})
		}
		for _, c := range p.Choices {
			options = append(options, ui.Option{Label: c, Value: c})
		}
		v, err := ui.Choose(title, options)
		if err != nil || v == skip {
			return adapter.Arg{}, false, err
		}
		return adapter.Arg{Param: p, Text: v}, true, nil
	}

	placeholder := p.Default
	if placeholder != "" {
		placeholder = "default: " + placeholder
	} else if !p.Required {
		placeholder = "empty leaves it out"
	}
	v, err := ui.Input(ui.Field{
		Title:       title,
		Description: adapter.DescribeParam(*p) + map[bool]string{true: ", comma-separated"}[p.Kind == adapter.List],
		Placeholder: placeholder,
		Validate: func(s string) error {
			if s == "" {
				if p.Required {
					return fmt.Errorf("-%s is required", p.Name)
				}
				return nil
			}
			_, err := adapter.Parse(p, s)
			return err
		},
	})
	if err != nil || v == "" {
		return adapter.Arg{}, false, err
	}
	arg, err = adapter.Parse(p, v)
	return arg, err == nil, err
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
