package reminder

import (
	"os"
	"testing"
	"time"

	"aex/internal/settings"
)

func TestScheduledDue(t *testing.T) {
	loc := time.FixedZone("x", 3*3600)
	wed := time.Date(2026, 9, 30, 15, 0, 0, 0, loc) // a Wednesday
	s := Scheduled{Message: "m", Time: "15:00", Days: []string{"mon", "wed"}}
	for _, c := range []struct {
		name string
		s    Scheduled
		now  time.Time
		want bool
	}{
		{"at its time", s, wed, true},
		{"a bit late", s, wed.Add(10 * time.Minute), true},
		{"too late", s, wed.Add(late), false},
		{"early", s, wed.Add(-time.Minute), false},
		{"shown today", Scheduled{Message: "m", Time: "15:00", Days: s.Days, Last: "2026-09-30"}, wed, false},
		{"shown yesterday", Scheduled{Message: "m", Time: "15:00", Days: s.Days, Last: "2026-09-29"}, wed, true},
		{"off", Scheduled{Message: "m", Time: "15:00", Days: s.Days, Off: true}, wed, false},
		{"other day", s, wed.AddDate(0, 0, 1), false},
	} {
		if got := c.s.due(c.now); got != c.want {
			t.Errorf("%s: due = %v", c.name, got)
		}
	}
}

func TestScheduledCheck(t *testing.T) {
	s, err := Scheduled{Message: " m ", Time: "09:30", Days: []string{"fri", "mon"}}.Check()
	if err != nil || s.Message != "m" || s.Days[0] != "mon" {
		t.Fatalf("Check = %+v, %v", s, err)
	}
	for _, bad := range []Scheduled{
		{Message: "m", Time: "9:30", Days: []string{"mon"}},
		{Message: "m", Time: "25:00", Days: []string{"mon"}},
		{Message: "m", Time: "09:30"},
		{Message: "m", Time: "09:30", Days: []string{"monday"}},
		{Time: "09:30", Days: []string{"mon"}},
	} {
		if _, err := bad.Check(); err == nil {
			t.Errorf("Check(%+v) took it", bad)
		}
	}
}

func TestSaveScheduledKeepsLast(t *testing.T) {
	old := settings.DataDir
	settings.DataDir = t.TempDir()
	t.Cleanup(func() { settings.DataDir = old })
	list, err := SaveScheduled([]Scheduled{{Message: "m", Time: "00:00", Days: Week}})
	if err != nil || list[0].ID == "" {
		t.Fatalf("SaveScheduled = %+v, %v", list, err)
	}
	// New and its time today passed: not shown today.
	if list[0].Last == "" {
		t.Fatal("a new reminder whose time passed would show at once")
	}
	list[0].Last = ""
	list[0].Message = "changed"
	again, err := SaveScheduled(list)
	if err != nil || again[0].Last == "" || again[0].ID != list[0].ID {
		t.Fatalf("second save = %+v, %v", again, err)
	}
	loaded, _ := LoadScheduled()
	if len(loaded) != 1 || loaded[0].Message != "changed" {
		t.Fatalf("loaded %+v", loaded)
	}
}

func TestLoadScheduledTakesBOM(t *testing.T) {
	old := settings.DataDir
	settings.DataDir = t.TempDir()
	t.Cleanup(func() { settings.DataDir = old })
	data := "\xef\xbb\xbf" + `{"reminders":[{"id":"a","message":"m","time":"09:00","days":["mon"]}]}`
	if err := os.WriteFile(scheduledFile(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if list, err := LoadScheduled(); err != nil || len(list) != 1 {
		t.Fatalf("LoadScheduled = %+v, %v", list, err)
	}
}
