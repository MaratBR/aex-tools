// Package quota computes the monthly hours quota from AEXT working days, leaves and logged time.
package quota

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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

// Data is AEXT's working days, leaves and hours over a span of days.
type Data struct {
	WorkingDays map[string]bool     // working days per calendar, minus leave days
	LeaveDays   map[string]LeaveDay // calendar working days covered by a leave
	OffDays     map[string]bool     // days the calendar has as not working (weekends, holidays)
	HoursByDay  map[string]float64  // only days with hours
	Warnings    []string
	Raw         Raw // what AEXT answered, for the quota widget's debug view
}

// Raw is what AEXT answered for a Fetch, and how long each request took.
type Raw struct {
	From        string            `json:"from"`
	To          string            `json:"to"`
	WorkingDays []aext.WorkingDay `json:"workingDays"`
	Summary     []aext.DaySummary `json:"summary"`
	Leaves      []aext.Leave      `json:"leaves"` // ignored ones (declined, cancelled) too
	Took        map[string]string `json:"took"`   // request: duration
}

// LeaveDay is the leave covering a day. Approved wins over one still pending on the same day.
type LeaveDay struct {
	Approved bool   `json:"approved,omitempty"`
	Type     string `json:"type,omitempty"`
}

// FetchMonths loads last month and the month of now.
func FetchMonths(c *aext.Client, now string) (*Data, error) {
	return Fetch(c, dates.PrevMonthStart(now), dates.MonthEnd(now))
}

// Fetch loads the days from through to.
func Fetch(c *aext.Client, from, to string) (*Data, error) {
	var (
		days    []aext.WorkingDay
		summary []aext.DaySummary
		leaves  []aext.Leave
		took    [3]time.Duration
	)
	timed := func(i int, fn func() error) func() error {
		return func() error {
			start := time.Now()
			defer func() { took[i] = time.Since(start) }()
			return fn()
		}
	}
	err := parallel(
		timed(0, func() (err error) { days, err = c.WorkingDays(from, to); return }),
		timed(1, func() (err error) { summary, err = c.TimeSummary(from, to); return }),
		timed(2, func() (err error) { leaves, err = fetchLeaves(c, from, to); return }),
	)
	if err != nil {
		return nil, err
	}

	d := &Data{WorkingDays: map[string]bool{}, LeaveDays: map[string]LeaveDay{}, OffDays: map[string]bool{}, HoursByDay: map[string]float64{}}
	for _, s := range summary {
		if *s.HoursTotal > 0 {
			d.HoursByDay[s.Date] += *s.HoursTotal
		}
	}
	calendarWorking := map[string]bool{}
	for _, day := range days {
		if *day.IsWorkingDay {
			calendarWorking[day.Date] = true
		} else {
			d.OffDays[day.Date] = true
		}
	}
	for _, l := range leaves {
		if isIgnored(l) {
			continue
		}
		for _, day := range dates.EachDay(max(l.Start, from), min(l.End, to)) {
			if calendarWorking[day] && !d.LeaveDays[day].Approved {
				d.LeaveDays[day] = LeaveDay{Approved: l.Status == "approved", Type: l.LeaveType}
			}
		}
	}
	for day := range calendarWorking {
		if _, leave := d.LeaveDays[day]; !leave {
			d.WorkingDays[day] = true
		}
	}
	d.Warnings = leaveWarnings(leaves)
	d.Raw = Raw{From: from, To: to, WorkingDays: days, Summary: summary, Leaves: leaves, Took: map[string]string{
		"working-days": took[0].Round(time.Millisecond).String(),
		"time-summary": took[1].Round(time.Millisecond).String(),
		"leaves":       took[2].Round(time.Millisecond).String(),
	}}
	return d, nil
}

// Day is one day of a month as the widget draws it.
type Day struct {
	Date  string    `json:"date"`
	Kind  string    `json:"kind"` // work, leave, holiday (not working on a weekday) or weekend
	Leave *LeaveDay `json:"leave,omitempty"`
	Hours float64   `json:"hours,omitempty"`
}

// Month is the quota of one month as of now.
type Month struct {
	Month          string   `json:"month"` // YYYY-MM
	WorkingDays    int      `json:"workingDays"`
	LeaveDays      int      `json:"leaveDays"`
	Expected       float64  `json:"expected"`
	Logged         float64  `json:"logged"`
	Percent        float64  `json:"percent"`
	DaysDue        int      `json:"daysDue"` // working days through today
	ExpectedToDate float64  `json:"expectedToDate"`
	Behind         float64  `json:"behind"` // negative when ahead
	Remaining      float64  `json:"remaining"`
	DaysLeft       int      `json:"daysLeft"`    // working days from today on
	PerDayLeft     float64  `json:"perDayLeft"`  // only when DaysLeft > 0
	Missing        []string `json:"missing"`     // working days before today without hours, sorted
	Current        bool     `json:"current"`     // the month of now
	Future         bool     `json:"future"`      // after the month of now
	Today          float64  `json:"today"`       // hours logged today, when Current
	LeaveHours     float64  `json:"leaveHours"`  // logged on leave days
	OffDayHours    float64  `json:"offDayHours"` // logged on weekends and holidays
	Days           []Day    `json:"days"`
}

func ComputeMonth(month, now string, d *Data) Month {
	from := dates.MonthStart(month)
	to := dates.MonthEnd(month)
	in := func(day string) bool { return day >= from && day <= to }
	perDay := settings.HoursPerDay()

	q := Month{Month: from[:7], Current: in(now), Future: from > now}
	for day := range d.WorkingDays {
		if !in(day) {
			continue
		}
		q.WorkingDays++
		// Today is due already (its hours count as missing until logged) and still left to work.
		if day <= now {
			q.DaysDue++
		}
		if day < now && d.HoursByDay[day] == 0 {
			q.Missing = append(q.Missing, day)
		}
		if day >= now {
			q.DaysLeft++
		}
	}
	slices.Sort(q.Missing)
	for _, day := range dates.EachDay(from, to) {
		hours := d.HoursByDay[day]
		q.Logged += hours
		dd := Day{Date: day, Kind: "work", Hours: hours}
		if leave, ok := d.LeaveDays[day]; ok {
			dd.Kind, dd.Leave = "leave", &leave
			q.LeaveDays++
			q.LeaveHours += hours
		} else if d.OffDays[day] {
			dd.Kind = "holiday"
			if wd := dates.DayStart(day).Weekday(); wd == time.Saturday || wd == time.Sunday {
				dd.Kind = "weekend"
			}
			q.OffDayHours += hours
		}
		q.Days = append(q.Days, dd)
	}
	if q.Current {
		q.Today = d.HoursByDay[now]
	}
	q.Expected = float64(q.WorkingDays) * perDay
	q.Percent = 100
	if q.Expected > 0 {
		q.Percent = q.Logged / q.Expected * 100
	}
	q.ExpectedToDate = float64(q.DaysDue) * perDay
	q.Behind = q.ExpectedToDate - q.Logged
	q.Remaining = max(0, q.Expected-q.Logged)
	if q.DaysLeft > 0 {
		q.PerDayLeft = q.Remaining / float64(q.DaysLeft)
	}
	return q
}

// Differences below this are rounding, not hours to log.
const epsilon = 0.05

// h formats hours with one decimal, dropping ".0": 8h, 7.5h.
func h(n float64) string {
	s := strconv.FormatFloat(math.Round(n*10)/10, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0") + "h"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// shortDays lists days as "Mon 09.22", at most limit of them.
func shortDays(days []string, limit int) string {
	var parts []string
	for i, day := range days {
		if i == limit {
			parts = append(parts, fmt.Sprintf("+%d more", len(days)-limit))
			break
		}
		t := dates.DayStart(day)
		parts = append(parts, t.Format("Mon 01.02"))
	}
	return strings.Join(parts, ", ")
}

const (
	barWidth  = 36
	lineWidth = 64 // header lines: month name left, verdict right
)

// bar shows logged hours filled, and a marker where the hours due by today end.
func bar(q Month, c ui.Palette) string {
	if q.Expected <= 0 {
		return ""
	}
	cells := func(v float64) int { return min(barWidth, int(math.Round(v/q.Expected*barWidth))) }
	filled, due := cells(q.Logged), -1
	if q.Current && q.DaysLeft > 0 {
		due = cells(q.ExpectedToDate)
	}
	fill := c.Green
	switch {
	case !q.Current && q.Remaining > epsilon && q.Remaining < settings.QuotaWarnHours():
		fill = c.Yellow
	case q.Current && q.Behind > epsilon || !q.Current && q.Remaining > epsilon:
		fill = c.Red
	}
	var b strings.Builder
	for i := range barWidth {
		switch {
		case i == due && i >= filled:
			b.WriteString(c.Yellow("┃"))
		case i < filled:
			b.WriteString(fill("█"))
		default:
			b.WriteString(c.Dim("░"))
		}
	}
	return b.String()
}

// header puts the month name left and the verdict right-aligned to lineWidth.
func header(month, verdict, verdictPlain string, c ui.Palette) string {
	name := dates.DayStart(month + "-01").Format("January 2006")
	gap := max(2, lineWidth-utf8.RuneCountInString(name)-utf8.RuneCountInString(verdictPlain))
	return c.Bold(name) + strings.Repeat(" ", gap) + verdict
}

func row(label, value string, c ui.Palette) string {
	return "  " + c.Dim(fmt.Sprintf("%-14s", label)) + " " + value
}

// OffWork says how many hours were logged on leave days and on weekends and holidays, or "" when none.
func OffWork(q Month) string {
	var parts []string
	if q.LeaveHours > epsilon {
		parts = append(parts, h(q.LeaveHours)+" during your off time")
	}
	if q.OffDayHours > epsilon {
		parts = append(parts, h(q.OffDayHours)+" on non-working days")
	}
	if len(parts) == 0 {
		return ""
	}
	return "You worked " + strings.Join(parts, " and ")
}

func formatMonth(q Month, c ui.Palette) []string {
	perDay := settings.HoursPerDay()
	good := func(s string) (string, string) { return c.Green(c.Bold("✔ " + s)), "✔ " + s }
	bad := func(s string) (string, string) { return c.Red(c.Bold("✖ " + s)), "✖ " + s }
	total := fmt.Sprintf("%s / %s", h(q.Logged), h(q.Expected))

	var lines []string
	if !q.Current {
		// Finished month: one line when complete, else what is short and where.
		if q.Remaining <= epsilon {
			v, p := good("complete · " + total)
			return []string{header(q.Month, v, p, c)}
		}
		v, p := bad(h(q.Remaining) + " short")
		if q.Remaining < settings.QuotaWarnHours() {
			v = c.Yellow(c.Bold(p))
		}
		lines = append(lines, header(q.Month, v, p, c),
			bar(q, c)+"  "+total+c.Dim(fmt.Sprintf(" · %.0f%%", q.Percent)))
		if len(q.Missing) > 0 {
			lines = append(lines, row("Missing hours", c.Yellow(shortDays(q.Missing, 8)), c))
		}
		if w := OffWork(q); w != "" {
			lines = append(lines, row("Off time", c.Yellow("▲ "+w), c))
		}
		return lines
	}

	var v, p string
	switch {
	case q.Remaining <= epsilon:
		v, p = good("quota filled")
	case q.Behind > epsilon:
		v, p = bad(h(q.Behind) + " behind")
	case q.Behind < -epsilon:
		v, p = good("on track, " + h(-q.Behind) + " ahead")
	default:
		v, p = good("on track")
	}
	lines = append(lines, header(q.Month, v, p, c),
		bar(q, c)+"  "+total+c.Dim(fmt.Sprintf(" · %.0f%%", q.Percent)))

	lines = append(lines, row("Due by today", fmt.Sprintf("%s %s · logged %s",
		h(q.ExpectedToDate), c.Dim(fmt.Sprintf("(%d of %d days)", q.DaysDue, q.WorkingDays)), h(q.Logged)), c))
	if len(q.Missing) > 0 {
		lines = append(lines, row("Missing hours", c.Yellow(shortDays(q.Missing, 8))+c.Dim("  → run worklog-sync"), c))
	}
	today := c.Dim("nothing logged yet")
	if q.Today > 0 {
		today = h(q.Today) + " logged"
	}
	lines = append(lines, row("Today", today, c))
	if w := OffWork(q); w != "" {
		lines = append(lines, row("Off time", c.Yellow("▲ "+w), c))
	}
	if q.DaysLeft > 0 && q.Remaining > epsilon {
		need := h(q.PerDayLeft) + "/day"
		if q.PerDayLeft > perDay+epsilon {
			need = c.Yellow(c.Bold(need))
		} else {
			need = c.Green(need)
		}
		lines = append(lines, row("To finish", fmt.Sprintf("%s over %s left → %s %s",
			h(q.Remaining), plural(q.DaysLeft, "day", "days"), need, c.Dim(fmt.Sprintf("(normally %s)", h(perDay)))), c))
	}
	expected := fmt.Sprintf("%s × %s = %s", plural(q.WorkingDays, "working day", "working days"), h(perDay), h(q.Expected))
	if q.LeaveDays > 0 {
		expected += " · " + plural(q.LeaveDays, "day", "days") + " on leave"
	}
	lines = append(lines, row("Expected", c.Dim(expected), c))
	return lines
}

// Format renders this month, then last month, styled for stdout. Leave warnings are counted
// unless verbose, which lists them.
func Format(now string, d *Data, verbose bool) string {
	c := ui.Out
	var lines []string
	lines = append(lines, formatMonth(ComputeMonth(now, now, d), c)...)
	lines = append(lines, "")
	lines = append(lines, formatMonth(ComputeMonth(dates.PrevMonthStart(now), now, d), c)...)

	footer := fmt.Sprintf("%s/working day · %s calendar", h(settings.HoursPerDay()), settings.WorkingDaysCountry)
	if len(d.Warnings) > 0 && !verbose {
		footer += fmt.Sprintf(" · %s (quota --verbose)", plural(len(d.Warnings), "leave warning", "leave warnings"))
	}
	lines = append(lines, "", c.Dim(footer))
	if verbose {
		for _, w := range d.Warnings {
			lines = append(lines, c.Yellow("▲")+" "+c.Dim(w))
		}
	}
	return strings.Join(lines, "\n")
}
