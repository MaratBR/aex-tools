// Package dates does calendar-day math. A "day" is a YYYY-MM-DD string in the TZ_OFFSET_HOURS
// zone (app setting); day strings compare correctly as strings.
package dates

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aex/internal/settings"
)

const layout = "2006-01-02"

func zone() *time.Location {
	return time.FixedZone(TZLabel(), int(math.Round(settings.TZOffsetHours()*3600)))
}

// TZLabel is e.g. UTC+7, UTC-5, UTC+5:30.
func TZLabel() string {
	hours := settings.TZOffsetHours()
	sign := "+"
	if hours < 0 {
		sign = "-"
	}
	whole := math.Trunc(math.Abs(hours))
	minutes := int(math.Round((math.Abs(hours) - whole) * 60))
	label := fmt.Sprintf("UTC%s%d", sign, int(whole))
	if minutes != 0 {
		label += fmt.Sprintf(":%02d", minutes)
	}
	return label
}

func Today() string { return time.Now().In(zone()).Format(layout) }

// FileTimestamp is e.g. 2026-09-28T134142.
func FileTimestamp() string { return time.Now().In(zone()).Format("2006-01-02T150405") }

// DayOf is the day an absolute time falls on.
func DayOf(t time.Time) string { return t.In(zone()).Format(layout) }

// DayStart is 00:00 on the given day.
func DayStart(day string) time.Time {
	t, _ := time.ParseInLocation(layout, day, zone())
	return t
}

func parse(day string) time.Time {
	t, _ := time.Parse(layout, day)
	return t
}

func AddDays(day string, n int) string { return parse(day).AddDate(0, 0, n).Format(layout) }

func EachDay(from, to string) []string {
	var days []string
	for d := from; d <= to; d = AddDays(d, 1) {
		days = append(days, d)
	}
	return days
}

func MonthStart(day string) string     { return day[:8] + "01" }
func PrevMonthStart(day string) string { return MonthStart(AddDays(MonthStart(day), -1)) }
func MonthEnd(day string) string       { return parse(MonthStart(day)).AddDate(0, 1, -1).Format(layout) }

func weekStart(day string) string {
	dow := int(parse(day).Weekday())
	return AddDays(day, -((dow + 6) % 7))
}

func makeDay(y, m, d int) (string, bool) {
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d {
		return "", false
	}
	return t.Format(layout), true
}

var (
	fullDay  = regexp.MustCompile(`^(\d{4})\.(\d{1,2})\.(\d{1,2})$`)
	shortDay = regexp.MustCompile(`^(\d{2})\.(\d{1,2})\.(\d{1,2})$`)
	monthDay = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})$`)
	dash     = regexp.MustCompile(`\s*-\s*`)
	spaces   = regexp.MustCompile(`\s+`)
)

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// 2026.09.20 | 26.09.20 | 09.20 (year defaults to defaultYear)
func parseDay(text string, defaultYear int) (string, bool) {
	if m := fullDay.FindStringSubmatch(text); m != nil {
		return makeDay(atoi(m[1]), atoi(m[2]), atoi(m[3]))
	}
	if m := shortDay.FindStringSubmatch(text); m != nil {
		return makeDay(2000+atoi(m[1]), atoi(m[2]), atoi(m[3]))
	}
	if m := monthDay.FindStringSubmatch(text); m != nil {
		return makeDay(defaultYear, atoi(m[1]), atoi(m[2]))
	}
	return "", false
}

// Range is an inclusive span of days.
type Range struct{ From, To string }

func (r Range) String() string {
	if r.From == r.To {
		return r.From
	}
	return r.From + " .. " + r.To
}

// ParseRange parses a user range expression relative to now (a day). Accepts: today, yesterday,
// this week, last week, this month, last month, a single day (2026.09.20, 26.09.20, 09.20) or two
// days joined by "-" (09.20-09.30).
func ParseRange(input, now string) (Range, error) {
	text := spaces.ReplaceAllString(strings.ToLower(strings.TrimSpace(input)), " ")
	switch text {
	case "today":
		return Range{now, now}, nil
	case "yesterday":
		d := AddDays(now, -1)
		return Range{d, d}, nil
	case "this week":
		return Range{weekStart(now), now}, nil
	case "last week":
		from := AddDays(weekStart(now), -7)
		return Range{from, AddDays(from, 6)}, nil
	case "this month":
		return Range{MonthStart(now), now}, nil
	case "last month":
		return Range{PrevMonthStart(now), AddDays(MonthStart(now), -1)}, nil
	}

	bad := fmt.Errorf("cannot parse range %q", input)
	parts := dash.Split(text, -1)
	if len(parts) > 2 {
		return Range{}, bad
	}
	from, ok := parseDay(parts[0], atoi(now[:4]))
	if !ok {
		return Range{}, bad
	}
	to := from
	if len(parts) == 2 {
		fromYear := atoi(from[:4])
		to, ok = parseDay(parts[1], fromYear)
		// Year-less end before start wraps into next year: 12.30-01.02.
		if ok && to < from && monthDay.MatchString(parts[1]) {
			to, ok = parseDay(parts[1], fromYear+1)
		}
		if !ok {
			return Range{}, bad
		}
	}
	if from > to {
		return Range{}, errors.New("range start " + from + " is after end " + to)
	}
	return Range{from, to}, nil
}

// Now is the time now in the configured TZ.
func Now() time.Time { return time.Now().In(zone()) }
