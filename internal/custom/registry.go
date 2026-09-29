package custom

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"aex/internal/adapter"
	"aex/internal/plugin"
	"aex/internal/settings"
)

// Entry is a custom tool added: a name for a script, and the adapter that runs it.
type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Adapter string `json:"adapter"`
}

type registry struct {
	Tools []Entry `json:"tools"`
}

// Load lists the custom tools added, in the order they were added.
func Load() ([]Entry, error) {
	b, err := os.ReadFile(settings.CustomToolsFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r registry
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", settings.CustomToolsFile, err)
	}
	return r.Tools, nil
}

func save(entries []Entry) error {
	b, err := json.MarshalIndent(registry{entries}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settings.CustomToolsFile), 0o700); err != nil {
		return err
	}
	return os.WriteFile(settings.CustomToolsFile, append(b, '\n'), 0o600)
}

// Reserved reports whether name is taken by a tool that is not a custom tool (a built-in tool or a
// plugin). Set by main.
var Reserved = func(name string) bool { return false }

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// DefaultName is the name a script gets when none is given: its file name without extension.
func DefaultName(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return strings.Join(strings.Fields(name), "-")
}

// CheckName says why name cannot be a new custom tool's name, nil if it can.
func CheckName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("%q: use letters, digits, '.', '_' and '-', starting with a letter or digit", name)
	}
	if Reserved(name) {
		return fmt.Errorf("a tool is already called %s", name)
	}
	entries, err := Load()
	if err != nil {
		return err
	}
	if slices.ContainsFunc(entries, func(e Entry) bool { return strings.EqualFold(e.Name, name) }) {
		return fmt.Errorf("a custom tool is already called %s", name)
	}
	return nil
}

// CheckPath says why the file at path cannot be a custom tool, and else returns its absolute path
// and the adapter that runs it. Quotes around a pasted path are dropped.
func CheckPath(path string) (string, adapter.Adapter, error) {
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if path == "" {
		return "", nil, errors.New("enter the script's path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("%s is not a file", abs)
	}
	a := adapterFor(abs)
	if a == nil {
		return "", nil, fmt.Errorf("no tool adapter runs %s files (adapters: %s)", filepath.Ext(abs), adapterNames())
	}
	entries, err := Load()
	if err != nil {
		return "", nil, err
	}
	if i := slices.IndexFunc(entries, func(e Entry) bool { return samePath(e.Path, abs) }); i >= 0 {
		return "", nil, fmt.Errorf("%s is already the custom tool %s", abs, entries[i].Name)
	}
	return abs, a, nil
}

// Add adds the script at path as the custom tool name ("" for DefaultName).
func Add(path, name string) (Entry, error) {
	abs, a, err := CheckPath(path)
	if err != nil {
		return Entry{}, err
	}
	if name == "" {
		name = DefaultName(abs)
	}
	if err := CheckName(name); err != nil {
		return Entry{}, err
	}
	entries, err := Load()
	if err != nil {
		return Entry{}, err
	}
	e := Entry{Name: name, Path: abs, Adapter: a.Name()}
	return e, save(append(entries, e))
}

// Remove removes the custom tool name and forgets its safe hash. The script is left alone.
func Remove(name string) (Entry, error) {
	entries, err := Load()
	if err != nil {
		return Entry{}, err
	}
	i := slices.IndexFunc(entries, func(e Entry) bool { return e.Name == name })
	if i < 0 {
		return Entry{}, fmt.Errorf("no custom tool %s (see custom-tools list)", name)
	}
	e := entries[i]
	if err := save(slices.Delete(entries, i, i+1)); err != nil {
		return Entry{}, err
	}
	return e, plugin.Forget(e.Path)
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
