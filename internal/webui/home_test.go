package webui

import (
	"encoding/json"
	"fmt"
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
		if w.W < 1 || w.W > gridColumns || w.H < 1 || w.H > maxWidgetH {
			t.Errorf("widget %q: size %dx%d is outside the grid", w.ID, w.W, w.H)
		}
	}
}

func TestCleanWidgets(t *testing.T) {
	got := cleanWidgets([]HomeWidget{
		{ID: "a", Widget: "quota", W: 99, H: 0},
		{ID: "a", Widget: "quota", W: 1, H: 1},    // repeated id
		{ID: "b", Widget: "nope", W: 1, H: 1},     // unknown widget: kept, it shows as unknown
		{ID: "b2", Widget: "No Pe", W: 1, H: 1},   // not a widget id
		{ID: "../x", Widget: "quota", W: 1, H: 1}, // bad id
		{ID: "c", Widget: "quota", W: 2, H: 3},
		{ID: "d", Widget: "cm-release/cm-repos-state", W: 2, H: 1}, // a plugin's: kept, whether or not it is there
		{ID: "e", Widget: "cm-release/Bad", W: 1, H: 1},            // not a plugin widget id
		{ID: "f", Widget: "../x/y", W: 1, H: 1},
		{ID: "g", Widget: "quota", W: 1, H: 1, Settings: json.RawMessage(`{"calendar":"x"}`)},
		{ID: "h", Widget: "quota", W: 1, H: 1, Settings: json.RawMessage(`[1]`)}, // not an object: dropped
	}, gridColumns)
	want := []HomeWidget{{ID: "a", Widget: "quota", W: gridColumns, H: 1, X: at(0), Y: at(0)},
		{ID: "b", Widget: "nope", W: 1, H: 1, X: at(0), Y: at(1)},
		{ID: "c", Widget: "quota", W: 2, H: 3, X: at(1), Y: at(1)},
		{ID: "d", Widget: "cm-release/cm-repos-state", W: 2, H: 1, X: at(3), Y: at(1)},
		{ID: "g", Widget: "quota", W: 1, H: 1, X: at(5), Y: at(1), Settings: json.RawMessage(`{"calendar":"x"}`)},
		{ID: "h", Widget: "quota", W: 1, H: 1, X: at(6), Y: at(1)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func at(n int) *int { return &n }

// places gives each widget's place as "id@x,y", in the order of list.
func places(list []HomeWidget) []string {
	out := []string{}
	for _, w := range list {
		out = append(out, fmt.Sprintf("%s@%d,%d", w.ID, *w.X, *w.Y))
	}
	return out
}

func TestPlaceWidgets(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []HomeWidget
		want []string
	}{
		{"none placed: packed from the top left, as the grid did", []HomeWidget{
			{ID: "a", W: 6, H: 2}, {ID: "b", W: 8, H: 1}, {ID: "c", W: 6, H: 1}, {ID: "d", W: 3, H: 1}},
			[]string{"a@0,0", "c@6,0", "d@6,1", "b@0,2"}},
		{"gaps kept, sorted by place", []HomeWidget{
			{ID: "a", W: 3, H: 1, X: at(9), Y: at(4)}, {ID: "b", W: 3, H: 1, X: at(2), Y: at(1)}},
			[]string{"b@2,1", "a@9,4"}},
		{"clamped into the grid", []HomeWidget{
			{ID: "a", W: 6, H: 1, X: at(10), Y: at(-3)}, {ID: "b", W: 1, H: 1, X: at(-1), Y: at(maxRow + 5)}},
			[]string{"a@6,0", "b@0," + fmt.Sprint(maxRow)}},
		{"an overlapping one goes down", []HomeWidget{
			{ID: "a", W: 6, H: 2, X: at(0), Y: at(0)}, {ID: "b", W: 6, H: 1, X: at(3), Y: at(1)}, {ID: "c", W: 3, H: 1, X: at(4), Y: at(3)}},
			[]string{"a@0,0", "b@3,2", "c@4,3"}},
		{"ones without a place fill the gaps", []HomeWidget{
			{ID: "a", W: 3, H: 1}, {ID: "b", W: 12, H: 1, X: at(0), Y: at(1)}, {ID: "c", W: 9, H: 1, X: at(3), Y: at(0)}},
			[]string{"a@0,0", "c@3,0", "b@0,1"}},
	} {
		if got := places(placeWidgets(c.in, gridColumns)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
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
	if err := a.SaveHome(HomeLayout{Widgets: []HomeWidget{{ID: "x", Widget: "../nope", W: 1, H: 1}}}); err == nil {
		t.Fatal("bad widget id saved")
	}
	if err := a.SaveHome(HomeLayout{Widgets: []HomeWidget{{ID: "x", Widget: "quota", W: 1, H: 1, Settings: json.RawMessage(`"s"`)}}}); err == nil {
		t.Fatal("settings that are not an object saved")
	}
	saved := HomeLayout{Widgets: []HomeWidget{{ID: "x1", Widget: "quota", W: 2, H: 2, X: at(3), Y: at(5), Settings: json.RawMessage(`{"a":1}`)}},
		AutoRefreshOff: true, Width: 1200, Columns: 24}
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
	// Columns out of range (an edited file): the widths and places go to twelfths.
	if err := os.WriteFile(settings.HomeFile, []byte(`{"columns":120,"widgets":[{"id":"a","widget":"quota","w":60,"h":1,"x":60,"y":0}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err = a.Home()
	if err != nil || l.Columns != gridColumns || len(l.Widgets) != 1 || l.Widgets[0].W != 6 || *l.Widgets[0].X != 6 {
		t.Fatalf("120-column file: got %+v, %v; want 12 columns, width 6 at 6", l, err)
	}
	if err := a.SaveHome(HomeLayout{Widgets: []HomeWidget{}, Columns: 65}); err == nil {
		t.Fatal("65 columns saved")
	}
	if err := a.SaveHome(HomeLayout{Widgets: []HomeWidget{}, Columns: 7}); err != nil {
		t.Fatalf("7 columns: %v", err)
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

func TestUnknownWidget(t *testing.T) {
	a := &App{}
	p, err := a.WidgetPage("nope")
	if err != nil || !p.Unknown || p.Problem == "" || p.HTML != "" {
		t.Fatalf("got %+v, %v; want an unknown widget problem", p, err)
	}
	if _, err := a.WidgetPage("No Pe"); err == nil {
		t.Fatal("bad widget id: no error")
	}
	if _, err := a.WidgetCall("nope", "quota", nil); err == nil {
		t.Fatal("unknown widget: call ran")
	}
}

func TestUnknownDebugWidget(t *testing.T) {
	w := unknownDebugWidget()
	if !w.Debug || knownWidget(w.ID) || !validWidgetID(w.ID) || !validSettings(w.Settings) {
		t.Fatalf("got %+v; want a valid id aex does not know, with settings", w)
	}
	if unknownDebugWidget().ID == w.ID {
		t.Fatal("same id twice")
	}
}
