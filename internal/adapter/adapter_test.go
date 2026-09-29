package adapter

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestBind(t *testing.T) {
	d := Description{Params: []Param{
		{Name: "Name", Kind: String, Type: "string", Required: true, Aliases: []string{"who"}},
		{Name: "Number", Kind: Int, Type: "int"},
		{Name: "Force", Kind: Switch, Type: "switch"},
		{Name: "Tags", Kind: List, Type: "string[]", Choices: []string{"Red", "Blue"}},
		{Name: "On", Kind: Bool, Type: "bool"},
	}}
	show := func(args []Arg) string {
		var s []string
		for _, a := range args {
			switch a.Param.Kind {
			case Switch, Bool:
				s = append(s, fmt.Sprintf("%s=%v", a.Param.Name, a.On))
			case List:
				s = append(s, a.Param.Name+"="+strings.Join(a.List, "+"))
			default:
				s = append(s, a.Param.Name+"="+a.Text)
			}
		}
		return strings.Join(s, " ")
	}
	for _, c := range []struct {
		args []string
		want string // or the error
	}{
		{[]string{"-WHO", "x", "-Numb", "-5"}, "Name=x Number=-5"},
		{[]string{"--name=x", "--force", "--tags", "red, BLUE"}, "Name=x Force=true Tags=Red+Blue"},
		{[]string{"-Force:false", "x", "7", "-on", "yes"}, "Name=x Number=7 Force=false On=true"},
		{[]string{"x", "7", "red", "no", "extra"}, `unexpected argument "extra" (see --help)`},
		{[]string{"-N", "x"}, "-N could be -Name, -Number"},
		{[]string{"-Number", "seven"}, `-Number: "seven" is not a whole number`},
		{[]string{"-Tags", "green"}, `-Tags: "green" is not one of Red, Blue`},
		{[]string{"-Name"}, "-Name needs a value"},
		{[]string{"-Name", "a", "-who", "b"}, "-Name given twice"},
		{[]string{"-Nope"}, "no parameter -Nope (see --help)"},
	} {
		bound, _, err := Bind(d, c.args)
		got := show(bound)
		if err != nil {
			got = err.Error()
		}
		if got != c.want {
			t.Errorf("Bind(%q) = %s, want %s", c.args, got, c.want)
		}
	}
	if missing := Missing(d, nil); len(missing) != 1 || missing[0].Name != "Name" {
		t.Errorf("missing %v", missing)
	}

	_, rest, err := Bind(Description{Rest: true}, []string{"a", "b"})
	if err != nil || !slices.Equal(rest, []string{"a", "b"}) {
		t.Errorf("rest %q, %v", rest, err)
	}
}
