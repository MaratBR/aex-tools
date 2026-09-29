package worklogsync

import (
	"slices"
	"testing"

	"aex/internal/aext"
)

func TestNaturalLess(t *testing.T) {
	keys := []string{"CM-10", "AB-2", "CM-9", "CM-100", "CM-9"}
	slices.SortFunc(keys, func(a, b string) int {
		switch {
		case naturalLess(a, b):
			return -1
		case naturalLess(b, a):
			return 1
		}
		return 0
	})
	if want := []string{"AB-2", "CM-9", "CM-9", "CM-10", "CM-100"}; !slices.Equal(keys, want) {
		t.Errorf("sorted = %v", keys)
	}
}

func TestSplitDuplicates(t *testing.T) {
	id, h := int64(1), 3.33
	existing := []aext.TimeEntry{{ID: &id, Date: "2026-09-22", Project: "CMOS", Description: "CM-5783", HoursTotal: &h}}
	entries := []aext.Entry{
		{Date: "2026-09-22", Project: "CMOS", Description: " CM-5783 ", HoursTotal: 3},
		{Date: "2026-09-23", Project: "CMOS", Description: "CM-5783", HoursTotal: 1},
		{Date: "2026-09-23", Project: "CMOS", Description: "CM-5783", HoursTotal: 1},
	}
	keep, skipped := splitDuplicates(entries, existing)
	if len(keep) != 1 || keep[0].Date != "2026-09-23" {
		t.Errorf("keep = %v", keep)
	}
	if len(skipped) != 2 || skipped[0].logged != 3.33 || skipped[0].inCSV || !skipped[1].inCSV {
		t.Errorf("skipped = %v", skipped)
	}
}
