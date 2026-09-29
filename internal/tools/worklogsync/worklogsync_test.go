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
	if skipped[0].id == nil || *skipped[0].id != 1 {
		t.Errorf("skipped[0].id = %v, want 1 (the only AEXT entry)", skipped[0].id)
	}
}

func TestSplitDuplicatesSeveralOrRepeated(t *testing.T) {
	id1, id2, id3, h := int64(1), int64(2), int64(3), 1.0
	existing := []aext.TimeEntry{
		{ID: &id1, Date: "2026-09-22", Project: "CMOS", Description: "CM-1", HoursTotal: &h},
		{ID: &id2, Date: "2026-09-22", Project: "CMOS", Description: "CM-1", HoursTotal: &h},
		{ID: &id3, Date: "2026-09-22", Project: "CMOS", Description: "CM-2", HoursTotal: &h},
	}
	entries := []aext.Entry{
		{Date: "2026-09-22", Project: "CMOS", Description: "CM-1", HoursTotal: 3},
		{Date: "2026-09-22", Project: "CMOS", Description: "CM-2", HoursTotal: 2},
		{Date: "2026-09-22", Project: "CMOS", Description: "CM-2", HoursTotal: 5}, // repeat: not changed again
	}
	keep, skipped := splitDuplicates(entries, existing)
	if len(keep) != 0 || len(skipped) != 3 {
		t.Fatalf("keep = %v, skipped = %v", keep, skipped)
	}
	if skipped[0].id != nil || skipped[0].logged != 2 {
		t.Errorf("several AEXT entries: %+v, want no id and 2h", skipped[0])
	}
	if skipped[1].id == nil || *skipped[1].id != 3 {
		t.Errorf("one AEXT entry: %+v, want id 3", skipped[1])
	}
	if !skipped[2].inCSV || skipped[2].logged != 2 {
		t.Errorf("repeat: %+v, want inCSV with the first row's 2h", skipped[2])
	}
}
