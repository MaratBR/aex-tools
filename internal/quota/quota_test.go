package quota

import (
	"slices"
	"testing"

	"aex/internal/settings"
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
		LeaveDays:   map[string]LeaveDay{},
		HoursByDay:  map[string]float64{"2026-09-15": 8, "2026-09-16": 2.5},
	}
	q := ComputeMonth("2026-09-16", "2026-09-16", d)
	// Today (09.16) is due but not listed as missing, and 09.17 is not due yet.
	if !slices.Equal(q.Missing, []string{"2026-09-14"}) {
		t.Errorf("Missing = %v", q.Missing)
	}
	if !q.Current || q.Today != 2.5 || q.DaysDue != 3 || q.DaysLeft != 2 {
		t.Errorf("Current %v Today %v DaysDue %d DaysLeft %d", q.Current, q.Today, q.DaysDue, q.DaysLeft)
	}
	// Today counts as due: 3 days × hours per day, 10.5h logged.
	if due := 3 * settings.HoursPerDay(); q.ExpectedToDate != due || q.Behind != due-10.5 {
		t.Errorf("ExpectedToDate %v Behind %v", q.ExpectedToDate, q.Behind)
	}
}

func TestComputeMonthOffTime(t *testing.T) {
	d := &Data{
		// Fri 10.02 works, Mon 10.05 is on leave, Tue 10.06 pending leave, Wed 10.07 a holiday.
		WorkingDays: map[string]bool{"2026-10-02": true},
		LeaveDays:   map[string]LeaveDay{"2026-10-05": {Approved: true, Type: "vacation"}, "2026-10-06": {}},
		OffDays:     map[string]bool{"2026-10-03": true, "2026-10-04": true, "2026-10-07": true},
		HoursByDay:  map[string]float64{"2026-10-02": 8, "2026-10-03": 2, "2026-10-05": 4, "2026-10-06": 1, "2026-10-07": 1},
	}
	q := ComputeMonth("2026-10-01", "2026-10-15", d)
	if q.LeaveDays != 2 || q.LeaveHours != 5 || q.OffDayHours != 3 || q.Logged != 16 {
		t.Errorf("LeaveDays %d LeaveHours %v OffDayHours %v Logged %v", q.LeaveDays, q.LeaveHours, q.OffDayHours, q.Logged)
	}
	kinds := map[string]string{}
	for _, day := range q.Days {
		kinds[day.Date] = day.Kind
	}
	want := map[string]string{"2026-10-02": "work", "2026-10-03": "weekend", "2026-10-05": "leave", "2026-10-07": "holiday"}
	for day, kind := range want {
		if kinds[day] != kind {
			t.Errorf("%s is %q, want %q", day, kinds[day], kind)
		}
	}
	if len(q.Days) != 31 {
		t.Errorf("%d days", len(q.Days))
	}
	if got, want := OffWork(q), "You worked 5h during your off time and 3h on non-working days"; got != want {
		t.Errorf("OffWork = %q", got)
	}
}
