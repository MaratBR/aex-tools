package dates

import "testing"

func TestParseRange(t *testing.T) {
	const now = "2026-09-28" // a Monday
	cases := []struct{ in, from, to string }{
		{"today", now, now},
		{"Yesterday", "2026-09-27", "2026-09-27"},
		{"this  week", "2026-09-28", now},
		{"last week", "2026-09-21", "2026-09-27"},
		{"this month", "2026-09-01", now},
		{"last month", "2026-08-01", "2026-08-31"},
		{"2026.09.20", "2026-09-20", "2026-09-20"},
		{"26.9.2", "2026-09-02", "2026-09-02"},
		{"09.20-09.30", "2026-09-20", "2026-09-30"},
		{"12.30 - 01.02", "2026-12-30", "2027-01-02"},
		{"2025.02.28-03.01", "2025-02-28", "2025-03-01"},
	}
	for _, c := range cases {
		r, err := ParseRange(c.in, now)
		if err != nil || r.From != c.from || r.To != c.to {
			t.Errorf("ParseRange(%q) = %v, %v; want %s..%s", c.in, r, err, c.from, c.to)
		}
	}
	for _, bad := range []string{"", "02.30", "09.20-09.10-09.11", "2026.09.30-2026.09.01", "soon"} {
		if r, err := ParseRange(bad, now); err == nil {
			t.Errorf("ParseRange(%q) = %v; want error", bad, r)
		}
	}
}

func TestMonths(t *testing.T) {
	if got := MonthEnd("2028-02-10"); got != "2028-02-29" {
		t.Errorf("MonthEnd = %s", got)
	}
	if got := PrevMonthStart("2026-01-15"); got != "2025-12-01" {
		t.Errorf("PrevMonthStart = %s", got)
	}
	if got := len(EachDay("2026-09-28", "2026-10-02")); got != 5 {
		t.Errorf("EachDay len = %d", got)
	}
}

func TestOffsetLabel(t *testing.T) {
	for hours, want := range map[float64]string{7: "UTC+7", -5: "UTC-5", 5.5: "UTC+5:30", -9.5: "UTC-9:30", 0: "UTC+0", 12.75: "UTC+12:45"} {
		if got := OffsetLabel(hours); got != want {
			t.Errorf("OffsetLabel(%g) = %q, want %q", hours, got, want)
		}
	}
}
