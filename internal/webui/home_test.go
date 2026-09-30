package webui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"aex/internal/settings"
)

func TestEveryWidgetHasItsPage(t *testing.T) {
	for _, w := range widgetCatalog {
		if _, err := assets.Open("frontend/widgets/" + w.ID + ".html"); err != nil {
			t.Errorf("widget %q: %v", w.ID, err)
		}
		if w.W < 1 || w.W > maxWidgetW || w.H < 1 || w.H > maxWidgetH {
			t.Errorf("widget %q: size %dx%d is outside the grid", w.ID, w.W, w.H)
		}
	}
}

func TestCleanWidgets(t *testing.T) {
	got := cleanWidgets([]HomeWidget{
		{ID: "a", Widget: "quota", W: 99, H: 0},
		{ID: "a", Widget: "quota", W: 1, H: 1},    // repeated id
		{ID: "b", Widget: "nope", W: 1, H: 1},     // unknown widget
		{ID: "../x", Widget: "quota", W: 1, H: 1}, // bad id
		{ID: "c", Widget: "quota", W: 2, H: 3},
		{ID: "d", Widget: "cm-release/cm-repos-state", W: 2, H: 1}, // a plugin's: kept, whether or not it is there
		{ID: "e", Widget: "cm-release/Bad", W: 1, H: 1},            // not a plugin widget id
		{ID: "f", Widget: "../x/y", W: 1, H: 1},
		{ID: "g", Widget: "quota", W: 1, H: 1, Settings: json.RawMessage(`{"calendar":"x"}`)},
		{ID: "h", Widget: "quota", W: 1, H: 1, Settings: json.RawMessage(`[1]`)}, // not an object: dropped
	})
	want := []HomeWidget{{ID: "a", Widget: "quota", W: maxWidgetW, H: 1}, {ID: "c", Widget: "quota", W: 2, H: 3},
		{ID: "d", Widget: "cm-release/cm-repos-state", W: 2, H: 1},
		{ID: "g", Widget: "quota", W: 1, H: 1, Settings: json.RawMessage(`{"calendar":"x"}`)},
		{ID: "h", Widget: "quota", W: 1, H: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestHomeSaveLoad(t *testing.T) {
	old := settings.HomeFile
	settings.HomeFile = filepath.Join(t.TempDir(), "sub", "home.json")
	defer func() { settings.HomeFile = old }()
	a := &App{}

	l, err := a.Home()
	if err != nil || len(l.Widgets) != 0 {
		t.Fatalf("no file: got %+v, %v; want empty", l, err)
	}
	if err := a.SaveHome(HomeLayout{Widgets: []HomeWidget{{ID: "x", Widget: "nope", W: 1, H: 1}}}); err == nil {
		t.Fatal("unknown widget saved")
	}
	if err := a.SaveHome(HomeLayout{Widgets: []HomeWidget{{ID: "x", Widget: "quota", W: 1, H: 1, Settings: json.RawMessage(`"s"`)}}}); err == nil {
		t.Fatal("settings that are not an object saved")
	}
	saved := HomeLayout{Widgets: []HomeWidget{{ID: "x1", Widget: "quota", W: 2, H: 2, Settings: json.RawMessage(`{"a":1}`)}},
		AutoRefreshOff: true}
	if err := a.SaveHome(saved); err != nil {
		t.Fatal(err)
	}
	l, err = a.Home()
	if err != nil || !reflect.DeepEqual(l, saved) {
		t.Fatalf("got %+v, %v; want %+v", l, err, saved)
	}
	// A file from the 4-column grid: its widths become twelfths.
	if err := os.WriteFile(settings.HomeFile, []byte(`{"widgets":[{"id":"a","widget":"quota","w":1,"h":1},{"id":"b","widget":"quota","w":4,"h":2}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err = a.Home()
	if err != nil || len(l.Widgets) != 2 || l.Widgets[0].W != 3 || l.Widgets[1].W != 12 {
		t.Fatalf("4-column file: got %+v, %v; want widths 3 and 12", l, err)
	}
	// The cm-release plugin's repos state widget became the built-in Git status one.
	if err := os.WriteFile(settings.HomeFile, []byte(`{"columns":12,"widgets":[{"id":"a","widget":"cm-release/cm-repos-state","w":6,"h":2}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err = a.Home()
	if err != nil || len(l.Widgets) != 1 || l.Widgets[0].Widget != "git-status" || l.Widgets[0].W != 6 {
		t.Fatalf("renamed widget: got %+v, %v; want git-status", l, err)
	}
	if err := os.WriteFile(settings.HomeFile, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Home(); err == nil {
		t.Fatal("broken file loaded without an error")
	}
}

func TestNthWorkingDay(t *testing.T) {
	// 2026-10-02 is a Friday.
	if got, complete := nthWorkingDay("2026-10-01", 3, nil); got != "2026-10-06" || complete {
		t.Fatalf("Mon-Fri: got %s, %v; want 2026-10-06, false", got, complete)
	}
	working := map[string]bool{"2026-10-02": false, "2026-10-03": true, "2026-10-04": false, "2026-10-05": true}
	if got, complete := nthWorkingDay("2026-10-01", 2, working); got != "2026-10-05" || !complete {
		t.Fatalf("calendar: got %s, %v; want 2026-10-05, true", got, complete)
	}
}
