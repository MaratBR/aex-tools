package webui

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"aex/internal/settings"
	"aex/internal/tool"
)

// The built-in themes, in the same format as a custom theme's file.
//
//go:embed themes/*.json
var builtinThemes embed.FS

// Theme is a set of the window's colors (and fonts), the variables of tokens.css without their
// "--": for light mode, dark mode or both. A theme with only one follows it whatever the mode
// picked; a variable it leaves out keeps the default theme's. theme.js applies it.
type Theme struct {
	ID     string            `json:"id"` // a built-in's file name, "custom:<file name>" for a custom one
	Name   string            `json:"name"`
	Custom bool              `json:"custom,omitempty"`
	File   string            `json:"file,omitempty"` // a custom theme's file
	Light  map[string]string `json:"light,omitempty"`
	Dark   map[string]string `json:"dark,omitempty"`
}

// ThemeList is the Settings page's themes: the default one first (with its variables from
// tokens.css, to show it; theme.js sets none for it), the built-in ones, then the
// custom ones from the themes folder, with the files there that are not a theme and why.
type ThemeList struct {
	Themes   []Theme  `json:"themes"`
	Dir      string   `json:"dir"`
	Problems []string `json:"problems,omitempty"`
}

// themeVars are the variables of tokens.css a theme may set.
var themeVars = func() map[string]bool {
	m := map[string]bool{}
	for _, v := range strings.Fields("accent accent-2 on-accent ink muted faint bg side surface fill fill-2 hover sep " +
		"ok fail warn card-shadow pop-shadow sans display mono") {
		m[v] = true
	}
	for i := range 16 {
		m[fmt.Sprintf("c%d", i)] = true
	}
	return m
}()

// badValue is what a variable's value cannot have: it goes into the widgets' pages too, and
// widgets have no network.
var badValue = regexp.MustCompile(`[;{}<>\\!@]|/\*|(?i)url\(|(?i)image\(|(?i)image-set\(|(?i)expression\(`)

// maxThemeFile is how big a custom theme's file may be.
const maxThemeFile = 64 << 10

// parseTheme reads a theme's file: {"name": "...", "light": {...}, "dark": {...}}, the variables
// with or without their "--".
func parseTheme(data []byte, file string) (Theme, error) {
	var raw struct {
		Name  string            `json:"name"`
		Light map[string]string `json:"light"`
		Dark  map[string]string `json:"dark"`
	}
	if len(data) > maxThemeFile {
		return Theme{}, fmt.Errorf("over %d KB", maxThemeFile>>10)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Theme{}, err
	}
	t := Theme{Name: strings.TrimSpace(raw.Name)}
	if t.Name == "" {
		t.Name = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	}
	var err error
	if t.Light, err = themeMode(raw.Light, "light"); err != nil {
		return Theme{}, err
	}
	if t.Dark, err = themeMode(raw.Dark, "dark"); err != nil {
		return Theme{}, err
	}
	if t.Light == nil && t.Dark == nil {
		return Theme{}, errors.New(`no colors: give "light", "dark" or both`)
	}
	return t, nil
}

// themeMode checks one mode's variables; nil when there are none.
func themeMode(vars map[string]string, mode string) (map[string]string, error) {
	if len(vars) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for k, v := range vars {
		name := strings.TrimPrefix(k, "--")
		if !themeVars[name] {
			return nil, fmt.Errorf("%s: unknown variable %q", mode, k)
		}
		v = strings.TrimSpace(v)
		if v == "" || len(v) > 300 || badValue.MatchString(v) {
			return nil, fmt.Errorf("%s: %s: not a value a theme can set: %q", mode, k, v)
		}
		out[name] = v
	}
	return out, nil
}

// Themes lists the themes.
func (a *App) Themes() ThemeList {
	def := Theme{ID: "default", Name: "Default"}
	var err error
	if def.Light, def.Dark, err = defaultTheme(); err != nil {
		def.Light, def.Dark = nil, nil
	}
	list := ThemeList{Themes: []Theme{def}, Dir: settings.ThemesDir}
	var builtin []Theme
	files, _ := builtinThemes.ReadDir("themes")
	for _, f := range files {
		data, err := builtinThemes.ReadFile("themes/" + f.Name())
		t, err2 := parseTheme(data, f.Name())
		if err = errors.Join(err, err2); err != nil {
			list.Problems = append(list.Problems, "built-in "+f.Name()+": "+err.Error())
			continue
		}
		t.ID = strings.TrimSuffix(f.Name(), ".json")
		builtin = append(builtin, t)
	}
	custom, problems := customThemes()
	byName := func(l []Theme) {
		sort.SliceStable(l, func(i, j int) bool { return strings.ToLower(l[i].Name) < strings.ToLower(l[j].Name) })
	}
	byName(builtin)
	byName(custom)
	list.Themes = append(append(list.Themes, builtin...), custom...)
	list.Problems = append(list.Problems, problems...)
	return list
}

// customThemes reads the themes folder's JSON files.
func customThemes() ([]Theme, []string) {
	entries, err := os.ReadDir(settings.ThemesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, []string{err.Error()}
	}
	var list []Theme
	var problems []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		path := filepath.Join(settings.ThemesDir, e.Name())
		data, err := os.ReadFile(path)
		var t Theme
		if err == nil {
			t, err = parseTheme(data, e.Name())
		}
		if err != nil {
			problems = append(problems, e.Name()+": "+err.Error())
			continue
		}
		t.ID, t.Custom, t.File = "custom:"+e.Name(), true, path
		list = append(list, t)
	}
	return list, problems
}

// CopyTheme copies a theme to a new file in the themes folder, to start a custom theme from, and
// gives the copy.
func (a *App) CopyTheme(id string) (Theme, error) {
	var src *Theme
	for _, t := range a.Themes().Themes {
		if t.ID == id {
			src = &t
			break
		}
	}
	if src == nil {
		return Theme{}, errors.New("no such theme: " + id)
	}
	light, dark := src.Light, src.Dark
	if err := os.MkdirAll(settings.ThemesDir, 0o755); err != nil {
		return Theme{}, err
	}
	base := strings.ToLower(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(src.Name, "-"))
	base = strings.Trim(base, "-")
	if base == "" {
		base = "theme"
	}
	name := src.Name + " (custom)"
	for n := 1; ; n++ {
		file := base + "-custom.json"
		if n > 1 {
			file = fmt.Sprintf("%s-custom-%d.json", base, n)
			name = fmt.Sprintf("%s (custom %d)", src.Name, n)
		}
		path := filepath.Join(settings.ThemesDir, file)
		out := map[string]any{"name": name}
		if light != nil {
			out["light"] = light
		}
		if dark != nil {
			out["dark"] = dark
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return Theme{}, err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return Theme{}, err
		}
		_, err = f.Write(append(data, '\n'))
		if err = errors.Join(err, f.Close()); err != nil {
			return Theme{}, err
		}
		return Theme{ID: "custom:" + file, Name: name, Custom: true, File: path, Light: light, Dark: dark}, nil
	}
}

// defaultTheme is the default theme's variables a theme may set, from tokens.css: its :root block
// (light) and its dark one.
func defaultTheme() (light, dark map[string]string, err error) {
	css, err := assets.ReadFile("frontend/tokens.css")
	if err != nil {
		return nil, nil, err
	}
	block := func(selector string) map[string]string {
		i := strings.Index(string(css), selector+" {")
		if i < 0 {
			return nil
		}
		body, _, _ := strings.Cut(string(css[i:]), "}")
		vars := map[string]string{}
		for _, m := range cssVar.FindAllStringSubmatch(body, -1) {
			if themeVars[m[1]] {
				vars[m[1]] = strings.TrimSpace(m[2])
			}
		}
		return vars
	}
	light, dark = block(":root"), block(`:root[data-theme="dark"]`)
	if light == nil || dark == nil {
		return nil, nil, errors.New("tokens.css: no light or dark block")
	}
	return light, dark, nil
}

var cssVar = regexp.MustCompile(`--([\w-]+):\s*([^;]+);`)

// OpenThemes opens the themes folder in the file manager (making it first).
func (a *App) OpenThemes() error { return tool.OpenFolder(settings.ThemesDir) }
