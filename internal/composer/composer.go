// Package composer runs composed tools: a tool made of steps run one after another, each step an
// action: running a tool or a program, opening a project in an IDE, a message, putting windows in
// places, switching the window to Home or minimizing it, an IP check, a delay, if / else, trying
// steps until they work, return. Actions are not tools: they run only as steps of a composed tool.
// Some suggest the steps likely wanted on this device (suggest.go).
//
// A step that fails is reported and the next one runs, unless the step is critical: then the
// composed tool stops there. Steps marked parallel next to one another run together. Composed
// tools are kept in composed-tools.json in the data folder, made in the window's Composer page or
// written as JSON.
package composer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"aex/internal/settings"
)

// Group is the name of the group of composed tools in the tool list: "aex composed <name>".
const Group = "composed"

// Composed is a composed tool: its name, what it does and its steps.
type Composed struct {
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Steps   []Step `json:"steps"`
}

// Step is one step of a composed tool: an action, with how its failure counts and whether it
// runs together with the steps around it.
type Step struct {
	// Label names the step in the output and the editor; empty: the action's own description.
	Label string
	// Critical: the composed tool stops when this step fails (else it goes on to the next one).
	Critical bool
	// Parallel: the step runs together with the parallel steps right before and after it.
	Parallel bool
	Action   Action
}

// Action is what a step does. Each kind of action is a struct whose fields are the step's JSON
// fields besides action, label, critical and parallel.
type Action interface {
	// Kind is the action's id, the step's "action" in JSON.
	Kind() string
	// Describe says what it does, in a few words, for the output and --help.
	Describe() string
	// Check reports what is wrong with its fields to c.
	Check(c *Checker)
	// Run does it. It fails with why when it did not work.
	Run(r *Run) error
}

// Kind is a kind of action: its id, name and what it does, and the group the editor lists it in.
type Kind struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Group   string `json:"group"`
	new     func() Action
}

// Kinds are the actions, in the order the editor offers them.
var Kinds = []Kind{
	{"tool", "Run a tool", "Runs an aex tool (built-in, plugin, custom or composed) with arguments", "Run", func() Action { return &ToolAction{} }},
	{"run", "Run a program", "Runs a program, waiting for it to exit unless it is told not to", "Run", func() Action { return &RunAction{} }},
	{"open-ide", "Open in an IDE", "Opens a project or folder in Visual Studio, VS Code or a JetBrains IDE", "Run", func() Action { return &IDEAction{} }},
	{"message", "Show a message", "Shows a message on top of all windows, with a chime", "Windows", func() Action { return &MessageAction{} }},
	{"orient", "Orient windows", "Puts windows in places on the screens, waiting for those not open yet (Windows, AutoHotkey)", "Windows", func() Action { return &OrientAction{} }},
	{"home", "Switch to Home", "Shows the aex window's Home page", "Windows", func() Action { return &HomeAction{} }},
	{"minimize", "Minimize aex", "Minimizes the aex window", "Windows", func() Action { return &MinimizeAction{} }},
	{"ip-check", "IP check", "Checks where this device's IP address is (is the VPN on?)", "Checks", func() Action { return &IPCheckAction{} }},
	{"if", "If / else", "Runs an action as a condition: its then steps when it works, else its else steps", "Flow", func() Action { return &IfAction{} }},
	{"check", "Try until it works", "Runs its steps again and again until they work", "Flow", func() Action { return &CheckAction{} }},
	{"delay", "Delay", "Waits some seconds", "Flow", func() Action { return &DelayAction{} }},
	{"return", "Return", "Ends the steps it is in (a try of a check, or the tool): worked or failed", "Flow", func() Action { return &ReturnAction{} }},
}

func kindOf(id string) *Kind {
	for i := range Kinds {
		if Kinds[i].ID == id {
			return &Kinds[i]
		}
	}
	return nil
}

func kindIDs() string {
	ids := make([]string, len(Kinds))
	for i, k := range Kinds {
		ids[i] = k.ID
	}
	return strings.Join(ids, ", ")
}

// Title is how the step shows: its label, else what its action does.
func (s Step) Title() string {
	if s.Label != "" {
		return s.Label
	}
	if s.Action == nil {
		return "(no action)"
	}
	return s.Action.Describe()
}

// UnmarshalJSON reads a step: {"action": "<kind>", "label", "critical", "parallel", then the
// action's own fields}. A field the action does not have is an error.
func (s *Step) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return errors.New("a step is an object: {\"action\": …}")
	}
	var kind string
	if v, ok := raw["action"]; !ok {
		return fmt.Errorf("a step needs \"action\" (one of %s)", kindIDs())
	} else if err := json.Unmarshal(v, &kind); err != nil {
		return errors.New("\"action\" is a string")
	}
	k := kindOf(kind)
	if k == nil {
		return fmt.Errorf("no action %q (actions: %s)", kind, kindIDs())
	}
	*s = Step{}
	common := []struct {
		key string
		dst any
	}{{"label", &s.Label}, {"critical", &s.Critical}, {"parallel", &s.Parallel}}
	for _, c := range common {
		if v, ok := raw[c.key]; ok {
			if err := json.Unmarshal(v, c.dst); err != nil {
				return fmt.Errorf("%q: %v", c.key, jsonReason(err))
			}
			delete(raw, c.key)
		}
	}
	delete(raw, "action")
	rest, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	a := k.new()
	dec := json.NewDecoder(bytes.NewReader(rest))
	dec.DisallowUnknownFields()
	if err := dec.Decode(a); err != nil {
		return fmt.Errorf("%s: %v", kind, jsonReason(err))
	}
	s.Action = a
	return nil
}

// MarshalJSON writes a step as UnmarshalJSON reads it, action first.
func (s Step) MarshalJSON() ([]byte, error) {
	if s.Action == nil {
		return nil, errors.New("a step without an action")
	}
	var b bytes.Buffer
	b.WriteString(`{"action":`)
	kind, _ := json.Marshal(s.Action.Kind())
	b.Write(kind)
	if s.Label != "" {
		label, _ := json.Marshal(s.Label)
		b.WriteString(`,"label":`)
		b.Write(label)
	}
	if s.Critical {
		b.WriteString(`,"critical":true`)
	}
	if s.Parallel {
		b.WriteString(`,"parallel":true`)
	}
	fields, err := json.Marshal(s.Action)
	if err != nil {
		return nil, err
	}
	if len(fields) > 2 {
		b.WriteByte(',')
		b.Write(fields[1:])
	} else {
		b.WriteByte('}')
	}
	return b.Bytes(), nil
}

// jsonReason is a JSON error without Go's type names where it can say it plainer.
func jsonReason(err error) string {
	if t, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		field := t.Field
		if field == "" {
			return fmt.Sprintf("expected %s, not %s", plainType(t.Type.Kind().String()), t.Value)
		}
		return fmt.Sprintf("%q: expected %s, not %s", field, plainType(t.Type.Kind().String()), t.Value)
	}
	s := err.Error()
	s = strings.TrimPrefix(s, "json: ")
	return strings.Replace(s, "unknown field", "no field", 1)
}

func plainType(kind string) string {
	switch kind {
	case "string":
		return "text"
	case "bool":
		return "true or false"
	case "int", "int64", "float64", "ptr":
		return "a number"
	case "slice":
		return "a list"
	case "struct", "map":
		return "an object"
	}
	return kind
}

// Entry is a composed tool as kept: what it reads as, and why it cannot be read when it cannot.
type Entry struct {
	Composed
	// Raw is its JSON as kept, for the editor to show also when it cannot be read.
	Raw json.RawMessage
	Err error
}

type file struct {
	Tools []json.RawMessage `json:"tools"`
}

// Load lists the composed tools kept, in their order. One that cannot be read is listed with Err
// (and its name, when that much can be read).
func Load() ([]Entry, error) {
	b, err := os.ReadFile(settings.ComposedToolsFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", settings.ComposedToolsFile, err)
	}
	entries := make([]Entry, len(f.Tools))
	for i, raw := range f.Tools {
		entries[i].Raw = raw
		c, err := Parse(raw)
		if err != nil {
			var name struct {
				Name string `json:"name"`
			}
			json.Unmarshal(raw, &name)
			c = Composed{Name: name.Name}
			if c.Name == "" {
				c.Name = fmt.Sprintf("composed-%d", i+1)
			}
		}
		entries[i].Composed, entries[i].Err = c, err
	}
	return entries, nil
}

func save(entries []Entry) error {
	f := file{Tools: make([]json.RawMessage, len(entries))}
	for i, e := range entries {
		f.Tools[i] = e.Raw
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settings.ComposedToolsFile), 0o700); err != nil {
		return err
	}
	return os.WriteFile(settings.ComposedToolsFile, append(b, '\n'), 0o600)
}

// Parse reads a composed tool's JSON. It fails at the first thing it cannot read, naming the step.
func Parse(b []byte) (Composed, error) {
	var raw struct {
		Name    string            `json:"name"`
		Summary string            `json:"summary"`
		Steps   []json.RawMessage `json:"steps"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return Composed{}, errors.New(jsonPosition(b, err))
	}
	c := Composed{Name: raw.Name, Summary: raw.Summary, Steps: make([]Step, len(raw.Steps))}
	for i, s := range raw.Steps {
		if err := json.Unmarshal(s, &c.Steps[i]); err != nil {
			return Composed{}, fmt.Errorf("step %d: %v", i+1, err)
		}
	}
	return c, nil
}

// jsonPosition is a JSON error with the line it is on, when it is a syntax error.
func jsonPosition(b []byte, err error) string {
	if s, ok := errors.AsType[*json.SyntaxError](err); ok {
		line := bytes.Count(b[:min(int(s.Offset), len(b))], []byte("\n")) + 1
		return fmt.Sprintf("line %d: %v", line, s)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return "the JSON ends before it is complete"
	}
	return jsonReason(err)
}

// Marshal is c as kept: indented JSON, with <, > and & as they are.
func Marshal(c Composed) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// Find is the composed tool called name (ignoring case), nil when there is none.
func Find(entries []Entry, name string) *Entry {
	for i := range entries {
		if strings.EqualFold(entries[i].Name, name) {
			return &entries[i]
		}
	}
	return nil
}

// Reserved reports whether name is taken by a tool that is not a composed tool (a built-in tool, a
// plugin, a custom tool or an adapter's group). Set by main.
var Reserved = func(name string) bool { return false }

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// checkName says why name cannot be a composed tool's name (other than the one now called old, ""
// for a new one), nil if it can.
func checkName(name, old string, entries []Entry) error {
	if name == "" {
		return errors.New("give it a name")
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("name %q: use letters, digits, '.', '_' and '-', starting with a letter or digit", name)
	}
	if strings.EqualFold(name, Group) || Reserved(name) {
		return fmt.Errorf("a tool is already called %s", name)
	}
	if e := Find(entries, name); e != nil && !strings.EqualFold(e.Name, old) {
		return fmt.Errorf("a composed tool is already called %s", e.Name)
	}
	return nil
}

// Put saves c, replacing the composed tool called old ("" adds it at the end), after checking
// it: nothing is saved when Check finds errors (warnings do not stop it).
func Put(old string, c Composed) (Problems, error) {
	entries, err := Load()
	if err != nil {
		return nil, err
	}
	p := check(c, old, entries)
	if p.HasErrors() {
		return p, nil
	}
	raw, err := Marshal(c)
	if err != nil {
		return nil, err
	}
	e := Entry{Composed: c, Raw: raw}
	if i := slices.IndexFunc(entries, func(e Entry) bool { return old != "" && strings.EqualFold(e.Name, old) }); i >= 0 {
		entries[i] = e
	} else {
		entries = append(entries, e)
	}
	return p, save(entries)
}

// Remove removes the composed tool called name.
func Remove(name string) error {
	entries, err := Load()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(entries, func(e Entry) bool { return strings.EqualFold(e.Name, name) })
	if i < 0 {
		return fmt.Errorf("no composed tool %s", name)
	}
	return save(slices.Delete(entries, i, i+1))
}
