package composer

import (
	"fmt"
	"strconv"
)

// Problem is something wrong with a composed tool: an error keeps it from being saved, a warning
// does not (a tool or program that is not there now may be later).
type Problem struct {
	// Step is the step it is about, numbered from 1; a step inside another after its number, the
	// part it is in and its own number ("2.then.1", "2.do.3"), or ".cond" for an if's condition;
	// "" for the composed tool itself.
	Step    string `json:"step,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Warning bool   `json:"warning,omitempty"`
}

func (p Problem) String() string {
	s := p.Message
	if p.Field != "" {
		s = p.Field + ": " + s
	}
	if p.Step != "" {
		s = "step " + p.Step + ": " + s
	}
	return s
}

// Problems are what Check found.
type Problems []Problem

// HasErrors reports whether any of them is an error, not a warning.
func (ps Problems) HasErrors() bool {
	for _, p := range ps {
		if !p.Warning {
			return true
		}
	}
	return false
}

// Checker collects the problems of a step's action (Action.Check).
type Checker struct {
	step string
	self string // the composed tool's name, which its steps cannot run
	list *Problems
}

// Error reports an error in field ("" for the step).
func (c *Checker) Error(field, format string, args ...any) {
	*c.list = append(*c.list, Problem{Step: c.step, Field: field, Message: fmt.Sprintf(format, args...)})
}

// Warn reports a warning about field ("" for the step).
func (c *Checker) Warn(field, format string, args ...any) {
	*c.list = append(*c.list, Problem{Step: c.step, Field: field, Message: fmt.Sprintf(format, args...), Warning: true})
}

// checkSteps checks steps, numbered prefix and their number from 1 ("2.then.1").
func (c *Checker) checkSteps(steps []Step, prefix string) {
	parallel := func(j int) bool { return j >= 0 && j < len(steps) && steps[j].Parallel }
	for i, s := range steps {
		at := prefix + strconv.Itoa(i+1)
		c.checkStep(s, at)
		if s.Parallel && !parallel(i-1) && !parallel(i+1) {
			*c.list = append(*c.list, Problem{Step: at, Field: "parallel", Warning: true,
				Message: "runs alone: mark the step before or after it parallel too, to run them together"})
		}
	}
}

// checkStep checks s as the step numbered at.
func (c *Checker) checkStep(s Step, at string) {
	sub := &Checker{step: at, self: c.self, list: c.list}
	if s.Action == nil {
		sub.Error("action", "a step needs an action")
		return
	}
	s.Action.Check(sub)
}

// Check checks c as a new composed tool (or one replacing the one called old) among those kept.
func Check(c Composed, old string) (Problems, error) {
	entries, err := Load()
	if err != nil {
		return nil, err
	}
	return check(c, old, entries), nil
}

func check(c Composed, old string, entries []Entry) Problems {
	var list Problems
	if err := checkName(c.Name, old, entries); err != nil {
		list = append(list, Problem{Field: "name", Message: err.Error()})
	}
	if len(c.Steps) == 0 {
		list = append(list, Problem{Field: "steps", Message: "add a step"})
	}
	root := &Checker{self: c.Name, list: &list}
	root.checkSteps(c.Steps, "")
	return list
}
