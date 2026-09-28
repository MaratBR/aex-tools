// Package quota computes the monthly hours quota from AEXT working days, leaves and logged time.
package quota

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/settings"
	"aex/internal/ui"
)

func isIgnored(l aext.Leave) bool { return slices.Contains(settings.IgnoredLeaveStatuses, l.Status) }

func sameEmail(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// parallel runs fns at once and returns the first error.
func parallel(fns ...func() error) error {
	errs := make([]error, len(fns))
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = fn()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Leave requests overlapping [from, to]. Fetched per calendar year; ids deduped.
func fetchLeaves(c *aext.Client, from, to string) ([]aext.Leave, error) {
	years := []string{from[:4]}
	if to[:4] != from[:4] {
		years = append(years, to[:4])
	}
	lists := make([][]aext.Leave, len(years))
	fns := make([]func() error, len(years))
	for i, y := range years {
		fns[i] = func() (err error) {
			lists[i], err = c.MyLeaves(y+"-01-01", y+"-12-31")
			return err
		}
	}
	if err := parallel(fns...); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var leaves []aext.Leave
	for _, list := range lists {
		for _, l := range list {
			id := fmt.Sprint(l.ID)
			if !seen[id] && l.Start <= to && l.End >= from {
				seen[id] = true
				leaves = append(leaves, l)
			}
		}
	}
	return leaves, nil
}

func leaveWarnings(leaves []aext.Leave) []string {
	var warnings []string
	for _, l := range leaves {
		what := fmt.Sprintf("Leave #%v (%s, %s..%s)", l.ID, l.LeaveType, l.Start, l.End)
		if isIgnored(l) {
			warnings = append(warnings, fmt.Sprintf("%s is %q, ignored.", what, l.Status))
		} else if l.Status != "approved" {
			warnings = append(warnings, fmt.Sprintf("%s is %q, counted as leave anyway.", what, l.Status))
		}
		owner := ""
		if l.User != nil {
			owner = l.User.Email
		}
		if l.Created != nil && l.Created.User != nil && l.Created.User.Email != "" && !sameEmail(l.Created.User.Email, owner) {
			if owner == "" {
				owner = "you"
			}
			warnings = append(warnings, fmt.Sprintf("%s was created by %s, not %s.", what, l.Created.User.Email, owner))
		}
	}
	return warnings
}

// Data is last month + current month from AEXT.
type Data struct {
	WorkingDays map[string]bool    // working days per calendar, minus leave days
	LeaveDays   map[string]bool    // calendar working days covered by a leave
	HoursByDay  map[string]float64 // only days with hours
	Warnings    []string
}

// FetchMonths loads last month and the month of now.
func FetchMonths(c *aext.Client, now string) (*Data, error) {
	from := dates.PrevMonthStart(now)
	to := dates.MonthEnd(now)
	var (
		days    []aext.WorkingDay
		summary []aext.DaySummary
		leaves  []aext.Leave
	)
	err := parallel(
		func() (err error) { days, err = c.WorkingDays(from, to); return },
		func() (err error) { summary, err = c.TimeSummary(from, to); return },
		func() (err error) { leaves, err = fetchLeaves(c, from, to); return },
	)
	if err != nil {
		return nil, err
	}

	d := &Data{WorkingDays: map[string]bool{}, LeaveDays: map[string]bool{}, HoursByDay: map[string]float64{}}
	for _, s := range summary {
		if *s.HoursTotal > 0 {
			d.HoursByDay[s.Date] += *s.HoursTotal
		}
	}
	calendarWorking := map[string]bool{}
	for _, day := range days {
		if *day.IsWorkingDay {
			calendarWorking[day.Date] = true
		}
	}
	for _, l := range leaves {
		if isIgnored(l) {
			continue
		}
		for _, day := range dates.EachDay(max(l.Start, from), min(l.End, to)) {
			if calendarWorking[day] {
				d.LeaveDays[day] = true
			}
		}
	}
	for day := range calendarWorking {
		if !d.LeaveDays[day] {
			d.WorkingDays[day] = true
		}
	}
	d.Warnings = leaveWarnings(leaves)
	return d, nil
}

// Month is the quota of one month as of now.
type Month struct {
	Month          string // YYYY-MM
	WorkingDays    int
	LeaveDays      int
	Expected       float64
	Logged         float64
	Percent        float64
	ExpectedToDate float64
	Behind         float64 // negative when ahead
	Remaining      float64
	DaysLeft       int
	PerDayLeft     float64 // only when DaysLeft > 0
}

func ComputeMonth(month, now string, d *Data) Month {
	from := dates.MonthStart(month)
	to := dates.MonthEnd(month)
	in := func(day string) bool { return day >= from && day <= to }
	perDay := settings.HoursPerDay()

	q := Month{Month: from[:7]}
	daysDue := 0
	for day := range d.WorkingDays {
		if !in(day) {
			continue
		}
		q.WorkingDays++
		// Today counts as remaining, not as already due.
		if day < now {
			daysDue++
		} else {
			q.DaysLeft++
		}
	}
	for day := range d.LeaveDays {
		if in(day) {
			q.LeaveDays++
		}
	}
	for day, h := range d.HoursByDay {
		if in(day) {
			q.Logged += h
		}
	}
	q.Expected = float64(q.WorkingDays) * perDay
	q.Percent = 100
	if q.Expected > 0 {
		q.Percent = q.Logged / q.Expected * 100
	}
	q.ExpectedToDate = float64(daysDue) * perDay
	q.Behind = q.ExpectedToDate - q.Logged
	q.Remaining = max(0, q.Expected-q.Logged)
	if q.DaysLeft > 0 {
		q.PerDayLeft = q.Remaining / float64(q.DaysLeft)
	}
	return q
}

func h(n float64) string { return fmt.Sprintf("%.2fh", n) }

// Format renders last month and the month of now, styled for stdout.
func Format(now string, d *Data) string {
	c := ui.Out
	perDay := settings.HoursPerDay()
	lines := []string{c.Bold("Quota") + c.Dim(fmt.Sprintf(" (%gh/working day, %s calendar)", perDay, settings.WorkingDaysCountry))}
	for _, month := range []string{dates.PrevMonthStart(now), now} {
		q := ComputeMonth(month, now, d)
		finished := q.DaysLeft == 0
		leave := ""
		if q.LeaveDays > 0 {
			leave = c.Dim(fmt.Sprintf(" (+%d on leave)", q.LeaveDays))
		}
		// Only a finished month's % is a verdict; mid-month it is just progress.
		pct := fmt.Sprintf("%.1f%%", q.Percent)
		switch {
		case !finished:
			pct = c.Bold(pct)
		case q.Remaining > 0:
			pct = c.Red(pct)
		default:
			pct = c.Green(pct)
		}
		lines = append(lines, fmt.Sprintf("%s: %d working days%s, %s expected, %s logged, %s",
			c.Bold(q.Month), q.WorkingDays, leave, h(q.Expected), h(q.Logged), pct))
		if finished {
			if q.Remaining > 0 {
				lines = append(lines, "  "+c.Red(h(q.Remaining)+" short"))
			}
			continue
		}
		status := c.Green(h(-q.Behind) + " ahead")
		if q.Behind > 0 {
			status = c.Red(h(q.Behind) + " behind")
		}
		lines = append(lines, fmt.Sprintf("  By today: %s due, %s", h(q.ExpectedToDate), status))
		need := h(q.PerDayLeft) + "/day"
		if q.PerDayLeft > perDay {
			need = c.Yellow(need)
		} else {
			need = c.Green(need)
		}
		lines = append(lines, fmt.Sprintf("  Remaining: %s over %d working day(s) incl. today -> %s", h(q.Remaining), q.DaysLeft, need))
	}
	for _, w := range d.Warnings {
		lines = append(lines, c.Yellow("Warning:")+" "+w)
	}
	return strings.Join(lines, "\n")
}
