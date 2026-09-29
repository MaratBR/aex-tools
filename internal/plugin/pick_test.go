package plugin

import (
	"slices"
	"testing"

	"aex/internal/ui"
)

// picker answers each PickTool with the next of picks, recording the titles asked.
type picker struct {
	picks  []string
	titles []string
}

func (p *picker) Input(ui.Field) (string, error)             { return "", nil }
func (p *picker) Confirm(string, bool) (bool, error)         { return false, nil }
func (p *picker) Choose(string, []ui.Option) (string, error) { return "", nil }
func (p *picker) WaitKey()                                   {}
func (p *picker) ClearScreen()                               {}
func (p *picker) PickTool(title string, _ []ui.Option) (string, error) {
	p.titles = append(p.titles, title)
	v := p.picks[0]
	p.picks = p.picks[1:]
	return v, nil
}

func TestPickSub(t *testing.T) {
	tools := []SubTool{{Name: "pull"}, {Name: "jira", Tools: []SubTool{{Name: "handoff"}, {Name: "check"}}}}
	for _, c := range []struct {
		sub, picks, want []string
	}{
		{nil, []string{"pull"}, []string{"pull"}},
		{nil, []string{"jira", "check"}, []string{"jira", "check"}},
		{[]string{"jira"}, []string{"handoff"}, []string{"jira", "handoff"}},
		{[]string{"pull"}, nil, []string{"pull"}},
		{[]string{"nope"}, nil, []string{"nope"}},
	} {
		p := &picker{picks: c.picks}
		ui.Remote = p
		got, err := pickSub("cm", c.sub, tools)
		ui.Remote = nil
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("pickSub(%q) = %q, %v; want %q", c.sub, got, err, c.want)
		}
		if len(p.picks) > 0 {
			t.Errorf("pickSub(%q) left picks %q unasked", c.sub, p.picks)
		}
	}
	if got, _ := pickSub("cm", nil, nil); got != nil {
		t.Errorf("pickSub of a plain plugin = %q, want none", got)
	}
}
