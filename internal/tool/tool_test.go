package tool

import (
	"slices"
	"strings"
	"testing"
)

func TestExecGroup(t *testing.T) {
	var ran string
	var got []string
	leaf := func(name string) Tool {
		return Tool{Name: name, Run: func(args []string) error { ran, got = name, args; return nil }}
	}
	g := Tool{Name: "g", Sub: []Tool{leaf("a"), {Name: "inner", Sub: []Tool{leaf("b")}}}}

	for _, c := range []struct {
		args     []string
		wantRan  string
		wantArgs []string
	}{
		{[]string{"a", "--x"}, "a", []string{"--x"}},
		{[]string{"1"}, "a", []string{}},
		{[]string{"inner", "b", "y"}, "b", []string{"y"}},
		{[]string{"2", "1"}, "b", []string{}},
	} {
		ran, got = "", nil
		if err := g.Exec(c.args); err != nil {
			t.Fatalf("Exec(%q): %v", c.args, err)
		}
		if ran != c.wantRan || !slices.Equal(got, c.wantArgs) {
			t.Errorf("Exec(%q) ran %s %q, want %s %q", c.args, ran, got, c.wantRan, c.wantArgs)
		}
	}

	if err := g.Exec([]string{"nope"}); err == nil || !strings.Contains(err.Error(), "a, inner") {
		t.Errorf("unknown tool: got %v", err)
	}
}
