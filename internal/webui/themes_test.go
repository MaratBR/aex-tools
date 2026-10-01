package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aex/internal/settings"
)

func withThemesDir(t *testing.T) string {
	old := settings.ThemesDir
	settings.ThemesDir = filepath.Join(t.TempDir(), "themes")
	t.Cleanup(func() { settings.ThemesDir = old })
	return settings.ThemesDir
}

func TestBuiltinThemes(t *testing.T) {
	withThemesDir(t)
	list := (&App{}).Themes()
	if len(list.Problems) > 0 {
		t.Fatalf("problems: %v", list.Problems)
	}
	if len(list.Themes) < 2 || list.Themes[0].ID != "default" {
		t.Fatalf("themes: %+v", list.Themes)
	}
	def := list.Themes[0]
	if def.Light["accent"] == "" || def.Dark["accent"] == "" || def.Light["c15"] == "" {
		t.Errorf("default theme from tokens.css: %+v", def)
	}
	if _, ok := def.Light["ring"]; ok {
		t.Error("default theme has --ring, which a theme cannot set")
	}
	for _, th := range list.Themes[1:] {
		if th.Custom || th.Name == "" || (th.Light == nil && th.Dark == nil) {
			t.Errorf("built-in theme %+v", th)
		}
	}
}

func TestParseTheme(t *testing.T) {
	good := `{"name": "Mine", "dark": {"--accent": "#123456", "sans": "\"Inter\", sans-serif", "fill": "rgba(1, 2, 3, .2)"}}`
	th, err := parseTheme([]byte(good), "mine.json")
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "Mine" || th.Dark["accent"] != "#123456" || th.Light != nil {
		t.Errorf("parsed %+v", th)
	}
	if th, err := parseTheme([]byte(`{"light": {"bg": "#fff"}}`), "plain.json"); err != nil || th.Name != "plain" {
		t.Errorf("name from the file: %+v, %v", th, err)
	}
	for _, bad := range []string{
		`{"name": "x"}`,
		`{"light": {"nope": "#fff"}}`,
		`{"light": {"bg": ""}}`,
		`{"light": {"bg": "red; color: blue"}}`,
		`{"light": {"bg": "url(https://example.com/x.png)"}}`,
		`{"light": {"bg": "</style><script>"}}`,
		`{"light": {"bg": "#fff !important"}}`,
		`not json`,
	} {
		if _, err := parseTheme([]byte(bad), "bad.json"); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestCustomThemes(t *testing.T) {
	dir := withThemesDir(t)
	a := &App{}
	c, err := a.CopyTheme("nord")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "custom:nord-custom.json" || c.Name != "Nord (custom)" || c.Light == nil || c.Dark == nil {
		t.Errorf("copy %+v", c)
	}
	c2, err := a.CopyTheme("nord")
	if err != nil || c2.ID != "custom:nord-custom-2.json" {
		t.Errorf("second copy %+v, %v", c2, err)
	}
	if c, err := a.CopyTheme("dracula"); err != nil || c.Light != nil {
		t.Errorf("dark-only copy %+v, %v", c, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"light": {"x": "1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a theme"), 0o644); err != nil {
		t.Fatal(err)
	}
	list := a.Themes()
	var custom []string
	for _, th := range list.Themes {
		if th.Custom {
			custom = append(custom, th.Name)
		}
	}
	if strings.Join(custom, ",") != "Dracula (custom),Nord (custom 2),Nord (custom)" {
		t.Errorf("custom themes %v", custom)
	}
	if len(list.Problems) != 1 || !strings.HasPrefix(list.Problems[0], "broken.json:") {
		t.Errorf("problems %v", list.Problems)
	}
	if _, err := a.CopyTheme("nope"); err == nil {
		t.Error("copied a theme that is not there")
	}
}
