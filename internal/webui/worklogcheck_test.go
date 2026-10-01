package webui

import (
	"reflect"
	"testing"

	"aex/internal/aext"
	"aex/internal/jira"
)

func TestCompareWorklogs(t *testing.T) {
	entry := func(id int64, date, project, description string, hours float64) aext.TimeEntry {
		return aext.TimeEntry{ID: &id, Date: date, Project: project, Description: description, HoursTotal: &hours}
	}
	logs := []jira.Worklog{
		{IssueKey: "CM-1", ProjectKey: "CM", Day: "2026-09-28", Seconds: 3600},
		{IssueKey: "CM-1", ProjectKey: "CM", Day: "2026-09-28", Seconds: 1800},  // summed: 1.5h
		{IssueKey: "CM-2", ProjectKey: "CM", Day: "2026-09-28", Seconds: 7200},  // AEXT has 1h
		{IssueKey: "OPS-3", ProjectKey: "OPS", Day: "2026-09-29", Seconds: 900}, // not in AEXT
	}
	entries := []aext.TimeEntry{
		entry(1, "2026-09-28", "CMOS", "CM-1", 1.5),
		entry(2, "2026-09-28", "CMOS", " CM-2 ", 1),
		entry(3, "2026-09-29", "CMOS", "CM-9", 2),        // only in AEXT
		entry(4, "2026-09-29", "INT", "Team meeting", 1), // not from Jira: left out
	}
	got := compareWorklogs(logs, entries)
	want := WorklogCheck{Jira: 3.75, AEXT: 4.5, Diffs: []WorklogDiff{
		{Date: "2026-09-28", Issue: "CM-2", Project: "CMOS", Jira: 2, AEXT: 1},
		{Date: "2026-09-29", Issue: "CM-9", Project: "CMOS", Jira: 0, AEXT: 2},
		{Date: "2026-09-29", Issue: "OPS-3", Project: "OPS", Jira: 0.25, AEXT: 0},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compareWorklogs = %+v, want %+v", got, want)
	}
}
