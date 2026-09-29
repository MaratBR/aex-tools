package plugin

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"aex/internal/ui"
)

// window stands in for the window on aex's side: it answers as a user would, using the Field funcs.
type window struct{ log []string }

func (w *window) Input(f ui.Field) (string, error) {
	w.log = append(w.log, "describe:"+f.Describe("ab"))
	if err := f.Validate("bad"); err != nil {
		w.log = append(w.log, "invalid:"+err.Error())
	}
	v := f.Paste(" pasted ")
	if err := f.Validate(v); err != nil {
		return "", err
	}
	return v, nil
}
func (w *window) Confirm(string, bool) (bool, error) { return false, errors.New("cancelled") }
func (w *window) Choose(_ string, o []ui.Option) (string, error) {
	return o[1].Value, nil
}
func (w *window) PickTool(_ string, o []ui.Option) (string, error) { return o[0].Value, nil }
func (w *window) WaitKey()                                         { w.log = append(w.log, "key") }
func (w *window) ClearScreen()                                     {}

func TestPromptsReachAex(t *testing.T) {
	w := &window{}
	defer func() { ui.Remote = nil }()
	ui.Remote = w
	s, env, err := servePrompts()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		t.Setenv(name, value)
	}
	if err := connectPrompts(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(promptTokenVar) != "" {
		t.Error("token left in the environment")
	}
	c := ui.Remote.(*promptClient)
	ui.Remote = w

	got, err := c.Input(ui.Field{
		Title:    "Epic",
		Validate: func(v string) error { return map[bool]error{true: fmt.Errorf("not %q", v)}[v == "bad"] },
		Paste:    strings.TrimSpace,
		Describe: func(v string) string { return strings.ToUpper(v) },
	})
	if err != nil || got != "pasted" {
		t.Fatalf("Input = %q, %v; want pasted", got, err)
	}
	if want := []string{"describe:AB", `invalid:not "bad"`}; strings.Join(w.log, "|") != strings.Join(want, "|") {
		t.Errorf("window saw %q, want %q", w.log, want)
	}
	if v, err := c.Choose("pick", []ui.Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}); err != nil || v != "b" {
		t.Errorf("Choose = %q, %v", v, err)
	}
	if v, err := c.PickTool("tool", []ui.Option{{Label: "x", Value: "x"}}); err != nil || v != "x" {
		t.Errorf("PickTool = %q, %v", v, err)
	}
	if _, err := c.Confirm("ok?", true); err == nil || err.Error() != "cancelled" {
		t.Errorf("Confirm err = %v, want cancelled", err)
	}
	c.WaitKey()
	if w.log[len(w.log)-1] != "key" {
		t.Error("WaitKey did not reach the window")
	}
}

func TestPromptsNeedToken(t *testing.T) {
	s, env, err := servePrompts()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	defer func() { ui.Remote = nil }()
	t.Setenv(promptsVar, strings.TrimPrefix(env[0], promptsVar+"="))
	t.Setenv(promptTokenVar, "wrong")
	if err := connectPrompts(); err != nil {
		t.Fatal(err)
	}
	if _, err := ui.Remote.Choose("x", []ui.Option{{Label: "a", Value: "a"}}); err == nil {
		t.Error("a connection with the wrong token got an answer")
	}
}
