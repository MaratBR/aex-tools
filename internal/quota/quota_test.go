package quota

import (
	"slices"
	"testing"
)

func TestH(t *testing.T) {
	for n, want := range map[float64]string{8: "8h", 7.5: "7.5h", 13.04: "13h", 0.25: "0.3h", 0: "0h"} {
		if got := h(n); got != want {
			t.Errorf("h(%v) = %q, want %q", n, got, want)
		}
	}
}

func TestComputeMonthMissing(t *testing.T) {
	d := &Data{
		WorkingDays: map[string]bool{"2026-09-14": true, "2026-09-15": true, "2026-09-16": true, "2026-09-17": true},
		LeaveDays:   map[string]bool{},
		HoursByDay:  map[string]float64{"2026-09-15": 8, "2026-09-16": 2.5},
	}
	q := ComputeMonth("2026-09-16", "2026-09-16", d)
	// Today (09.16) is not missing even with partial hours, and 09.17 is not due yet.
	if !slices.Equal(q.Missing, []string{"2026-09-14"}) {
		t.Errorf("Missing = %v", q.Missing)
	}
	if !q.Current || q.Today != 2.5 || q.DaysDue != 2 || q.DaysLeft != 2 {
		t.Errorf("Current %v Today %v DaysDue %d DaysLeft %d", q.Current, q.Today, q.DaysDue, q.DaysLeft)
	}
}
