package composer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aex/internal/reminder"
	"aex/internal/ui"
)

// Steps are the steps of an if's branch or a check: in JSON a list of steps, or one step alone.
type Steps []Step

func (s *Steps) UnmarshalJSON(b []byte) error {
	if b = bytes.TrimSpace(b); len(b) > 0 && b[0] == '{' {
		var one Step
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*s = Steps{one}
		return nil
	}
	var list []Step
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	*s = list
	return nil
}

// IfAction runs its condition, an action: when it works, the steps of Then run, else those of
// Else (Not swaps them). The condition failing is not a failure of the if. The if fails when the
// steps it ran did.
type IfAction struct {
	Cond *Step `json:"cond"`
	// Not: the steps of Then run when the condition fails.
	Not  bool  `json:"not,omitempty"`
	Then Steps `json:"then,omitempty"`
	Else Steps `json:"else,omitempty"`
}

func (a *IfAction) Kind() string { return "if" }

func (a *IfAction) Describe() string {
	if a.Cond == nil {
		return "If"
	}
	if a.Not {
		return "If not: " + a.Cond.Title()
	}
	return "If " + a.Cond.Title()
}

func (a *IfAction) Check(c *Checker) {
	if a.Cond == nil || a.Cond.Action == nil {
		c.Error("cond", "pick the condition: an action, true when it works")
	} else {
		switch a.Cond.Action.(type) {
		case *ReturnAction, *IfAction:
			c.Error("cond", "the condition cannot be %s", a.Cond.Action.Kind())
		}
		if a.Cond.Critical || a.Cond.Parallel {
			c.Error("cond", "the condition has no critical or parallel")
		}
		c.checkStep(*a.Cond, c.step+".cond")
	}
	if len(a.Then) == 0 && len(a.Else) == 0 {
		c.Warn("then", "add steps to then or else")
	}
	c.checkSteps(a.Then, c.step+".then.")
	c.checkSteps(a.Else, c.step+".else.")
}

func (a *IfAction) Run(r *Run) error {
	if a.Cond == nil || a.Cond.Action == nil {
		return errors.New("no condition")
	}
	in := r.nested()
	in.Printf("%s", ui.Out.Bold("? "+a.Cond.Title()))
	err := runAction(in.nested(), a.Cond.Action)
	if ret, ok := errors.AsType[*Returned](err); ok && !ret.Fail {
		err = nil
	}
	yes := err == nil
	if yes {
		in.Printf("%s", ui.Out.Cyan("→ yes"))
	} else {
		if r.Ctx.Err() != nil {
			return err
		}
		in.Printf("%s %v", ui.Out.Cyan("→ no:"), err)
	}
	branch, name := a.Then, "then"
	if yes == a.Not {
		branch, name = a.Else, "else"
	}
	if len(branch) == 0 {
		in.Printf("%s", ui.Out.Dim("(nothing to do in "+name+")"))
		return nil
	}
	return runSteps(in, branch)
}

// ReturnAction ends the steps it is in: one try of the check it is in, or else the composed tool.
// It returns worked, or failed (Fail), with an optional message.
type ReturnAction struct {
	Fail    bool   `json:"fail,omitempty"`
	Message string `json:"message,omitempty"`
}

func (a *ReturnAction) Kind() string { return "return" }

func (a *ReturnAction) Describe() string {
	what := "Return: worked"
	if a.Fail {
		what = "Return: failed"
	}
	return what + colonMessage(a.Message)
}

func (a *ReturnAction) Check(c *Checker) {}

func (a *ReturnAction) Run(r *Run) error { return &Returned{Fail: a.Fail, Message: a.Message} }

// MessageAction shows a message on top of all windows (a reminder, internal/reminder), and with
// Wait waits until it is closed.
type MessageAction struct {
	Title  string `json:"title,omitempty"`
	Text   string `json:"text"`
	Urgent bool   `json:"urgent,omitempty"`
	Wait   bool   `json:"wait,omitempty"`
}

func (a *MessageAction) Kind() string { return "message" }

func (a *MessageAction) Describe() string {
	text := a.Text
	if len([]rune(text)) > 40 {
		text = string([]rune(text)[:39]) + "…"
	}
	return fmt.Sprintf("Message %q", text)
}

func (a *MessageAction) Check(c *Checker) {
	if strings.TrimSpace(a.Text) == "" {
		c.Error("text", "write the message")
	}
}

func (a *MessageAction) Run(r *Run) error {
	done, err := reminder.Open(reminder.Reminder{Title: a.Title, Message: a.Text, Urgent: a.Urgent})
	if err != nil {
		return err
	}
	if !a.Wait {
		return nil
	}
	r.Printf("Waiting for the message to be closed")
	select {
	case <-done:
		return nil
	case <-r.Ctx.Done():
		return r.Ctx.Err()
	}
}
