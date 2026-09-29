// Package worklogsync is the worklog-sync tool: Jira worklogs to CSV, then to AEXT.
package worklogsync

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/jira"
	"aex/internal/quota"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/ui"
)

func worklogSyncHelp() string {
	return fmt.Sprintf(`Usage: worklog-sync [--range <expr>] [--manual]

Exports your Jira worklogs for a date range to <data folder>/output/jira-export/<timestamp>.csv,
then offers to send the CSV to AEXT.

Without --range, a picker offers: a range suggested from AEXT (first working day of this
month with no hours logged, through today), common ranges, or typing one.

  --range   Range expression, skips all range prompts. Examples:
            today, yesterday, this week, last week, this month, last month,
            2026.09.20, 26.09.20, 09.20, 09.20-09.30
  --manual  Leave the AEXT suggestion out of the picker

Dates are in %s (TZ_OFFSET_HOURS app setting, see configure).`, dates.TZLabel())
}

func aextLabel() string { return ui.Out.Dim("AEXT:") }

func askRange(now string) (dates.Range, error) {
	if err := ui.AssertInteractive("Asking for a range"); err != nil {
		return dates.Range{}, err
	}
	input, err := ui.Input(ui.Field{
		Title:       "Range",
		Description: "today, yesterday, this week, last week, this month, last month,\n2026.09.20, 09.20, 09.20-09.30",
		Placeholder: "09.20-09.30",
		Validate: func(s string) error {
			_, err := dates.ParseRange(s, now)
			return err
		},
	})
	if err != nil {
		return dates.Range{}, err
	}
	return dates.ParseRange(input, now)
}

// Ranges offered in the range picker, besides AEXT and typing one.
var rangePresets = []string{"today", "yesterday", "this week", "last week", "this month", "last month"}

// chooseRange asks for a range from a list: AEXT suggestion (unless manual), presets, or typed.
// Returns "aext" or "other" for those, else a range expression.
func chooseRange(now string, manual bool) (string, error) {
	var options []ui.Option
	if !manual {
		options = append(options, ui.Option{Label: "Suggest from AEXT (first day without hours)", Value: "aext"})
	}
	for _, p := range rangePresets {
		r, _ := dates.ParseRange(p, now)
		options = append(options, ui.Option{Label: fmt.Sprintf("%-11s %s", strings.ToUpper(p[:1])+p[1:], r), Value: p})
	}
	options = append(options, ui.Option{Label: "Other…", Value: "other"})
	return ui.Choose("Range", options)
}

func emptyWorkingDays(from, to string, d *quota.Data) []string {
	var days []string
	for _, day := range dates.EachDay(from, to) {
		if d.WorkingDays[day] && d.HoursByDay[day] == 0 {
			days = append(days, day)
		}
	}
	return days
}

// rangeFromAEXT suggests a range from AEXT, or nil if there is nothing to import.
func rangeFromAEXT(c *aext.Client, now string) (*dates.Range, error) {
	out := ui.Out
	curStart := dates.MonthStart(now)
	prevStart := dates.PrevMonthStart(now)
	prevEnd := dates.AddDays(curStart, -1)

	data, err := quota.FetchMonths(c, now)
	if err != nil {
		return nil, err
	}
	for _, w := range data.Warnings {
		ui.Warn("%s", w)
	}
	if len(data.LeaveDays) > 0 {
		fmt.Printf("%s leave days (not counted as gaps): %s\n", aextLabel(), strings.Join(slices.Sorted(maps.Keys(data.LeaveDays)), ", "))
	}

	from := ""
	if empty := emptyWorkingDays(curStart, now, data); len(empty) > 0 {
		from = empty[0]
		fmt.Printf("%s first working day without hours this month: %s\n", aextLabel(), out.Bold(from))
	} else {
		fmt.Printf("%s every working day this month has hours logged.\n", aextLabel())
	}

	if prevEmpty := emptyWorkingDays(prevStart, prevEnd, data); len(prevEmpty) > 0 {
		lastLogged := ""
		for day := range data.HoursByDay {
			if day >= prevStart && day <= prevEnd && day > lastLogged {
				lastLogged = day
			}
		}
		alt := prevStart
		if lastLogged != "" {
			alt = dates.AddDays(lastLogged, 1)
		}
		fmt.Printf("%s last month has %s working day(s) without hours: %s\n",
			aextLabel(), out.Yellow(strconv.Itoa(len(prevEmpty))), strings.Join(prevEmpty, ", "))
		limit := from
		if limit == "" {
			limit = dates.AddDays(now, 1)
		}
		if alt < limit {
			prompt := fmt.Sprintf("Nothing logged last month. Start from %s instead?", alt)
			if lastLogged != "" {
				prompt = fmt.Sprintf("Last logged day last month is %s. Start from %s instead?", lastLogged, alt)
			}
			yes, err := ui.Confirm(prompt, false)
			if err != nil {
				return nil, err
			}
			if yes {
				from = alt
			}
		}
	}

	if from == "" {
		return nil, nil
	}
	var already []string
	for _, day := range dates.EachDay(from, now) {
		if data.HoursByDay[day] > 0 {
			already = append(already, day)
		}
	}
	if len(already) > 0 {
		fmt.Printf("%s these days in range already have AEXT hours: %s\n", out.Yellow("Note:"), strings.Join(already, ", "))
	}
	return &dates.Range{From: from, To: now}, nil
}

func resolveRange(rangeExpr string, manual bool, now string, client func() (*aext.Client, error)) (dates.Range, error) {
	if rangeExpr != "" {
		return dates.ParseRange(rangeExpr, now)
	}
	if err := ui.AssertInteractive("Choosing a range"); err != nil {
		return dates.Range{}, err
	}
	choice, err := chooseRange(now, manual)
	if err != nil {
		return dates.Range{}, err
	}
	switch choice {
	case "other":
	case "aext":
		c, err := client()
		if err != nil {
			return dates.Range{}, err
		}
		r, err := rangeFromAEXT(c, now)
		if err != nil {
			return dates.Range{}, err
		}
		if r != nil {
			return *r, nil
		}
		fmt.Println("Nothing to import per AEXT, enter a range manually.")
	default:
		return dates.ParseRange(choice, now)
	}
	return askRange(now)
}

type row struct {
	day, issueKey, project string
	seconds                int
}

// toRows makes one row per (day, issue), hours summed.
func toRows(worklogs []jira.Worklog) []row {
	byKey := map[string]*row{}
	for _, w := range worklogs {
		key := w.Day + "|" + w.IssueKey
		r := byKey[key]
		if r == nil {
			project := w.ProjectKey
			if mapped, ok := settings.ProjectMap[project]; ok {
				project = mapped
			}
			r = &row{day: w.Day, issueKey: w.IssueKey, project: project}
			byKey[key] = r
		}
		r.seconds += w.Seconds
	}
	rows := make([]row, 0, len(byKey))
	for _, r := range byKey {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].day != rows[j].day {
			return rows[i].day < rows[j].day
		}
		return naturalLess(rows[i].issueKey, rows[j].issueKey)
	})
	return rows
}

var digitRuns = regexp.MustCompile(`\d+|\D+`)

// naturalLess compares with digit runs as numbers: CM-9 < CM-10.
func naturalLess(a, b string) bool {
	pa, pb := digitRuns.FindAllString(a, -1), digitRuns.FindAllString(b, -1)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] == pb[i] {
			continue
		}
		na, errA := strconv.Atoi(pa[i])
		nb, errB := strconv.Atoi(pb[i])
		if errA == nil && errB == nil && na != nb {
			return na < nb
		}
		return pa[i] < pb[i]
	}
	return len(pa) < len(pb)
}

func writeCSV(file string, rows []row) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	w.UseCRLF = true
	w.Write([]string{"date", "project", "hours", "description"})
	for _, r := range rows {
		w.Write([]string{r.day, r.project, fmt.Sprintf("%.2f", float64(r.seconds)/3600), r.issueKey})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Tool is the worklog-sync tool.
var Tool = tool.Tool{Name: "worklog-sync", Summary: "Export Jira worklogs to CSV, then send them to AEXT", Run: run}

func run(args []string) error {
	fs := flag.NewFlagSet("worklog-sync", flag.ContinueOnError)
	rangeExpr := fs.String("range", "", "")
	manual := fs.Bool("manual", false, "")
	if done, err := tool.ParseFlags(fs, args, worklogSyncHelp()); done || err != nil {
		return err
	}
	out := ui.Out

	now := dates.Today()
	var client *aext.Client
	aextClient := func() (*aext.Client, error) {
		if client != nil {
			return client, nil
		}
		var err error
		client, err = aext.New()
		return client, err
	}
	r, err := resolveRange(*rangeExpr, *manual, now, aextClient)
	if err != nil {
		return err
	}
	fmt.Printf("Range: %s %s\n", out.Bold(r.String()), out.Dim("("+dates.TZLabel()+")"))
	if *rangeExpr == "" {
		proceed, err := ui.Confirm("Proceed?", true)
		if err != nil || !proceed {
			return err
		}
	}

	j, err := jira.New()
	if err != nil {
		return err
	}
	worklogs, err := j.FetchMyWorklogs(r)
	if err != nil {
		return err
	}
	rows := toRows(worklogs)
	if len(rows) == 0 {
		fmt.Println("No Jira worklogs in range, nothing written.")
		return nil
	}

	if err := os.MkdirAll(settings.JiraExportDir, 0o755); err != nil {
		return err
	}
	csvFile := filepath.Join(settings.JiraExportDir, dates.FileTimestamp()+".csv")
	if err := writeCSV(csvFile, rows); err != nil {
		return err
	}
	seconds := 0
	for _, r := range rows {
		seconds += r.seconds
	}
	fmt.Printf("%d rows, %s -> %s\n", len(rows), out.Bold(fmt.Sprintf("%.2fh", float64(seconds)/3600)), out.Cyan(csvFile))

	if !ui.IsInteractive() {
		return nil
	}
	send, err := ui.Confirm(fmt.Sprintf("Send CSV to AEXT? (edit %s first if needed)", filepath.Base(csvFile)), false)
	if err != nil || !send {
		return err
	}
	c, err := aextClient()
	if err != nil {
		return err
	}
	if err := importCSV(c, csvFile); err != nil {
		return err
	}
	data, err := quota.FetchMonths(c, now)
	if err != nil {
		return err
	}
	fmt.Println(quota.Format(now, data, false))
	return nil
}

var isoDay = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// entryKey identifies an entry: same day, project and description (the issue key) is a duplicate.
type entryKey struct{ date, project, description string }

func keyOf(date, project, description string) entryKey {
	return entryKey{date, strings.TrimSpace(project), strings.TrimSpace(description)}
}

// skipDuplicates drops entries AEXT already has, and says which (with both hours when they differ).
func skipDuplicates(c *aext.Client, entries []aext.Entry) ([]aext.Entry, error) {
	from, to := entries[0].Date, entries[0].Date
	for _, e := range entries {
		from, to = min(from, e.Date), max(to, e.Date)
	}
	existing, err := c.TimeEntries(from, to)
	if err != nil {
		return nil, err
	}
	keep, skipped := splitDuplicates(entries, existing)
	for _, s := range skipped {
		line := fmt.Sprintf("  %s %s %s", s.entry.Date, s.entry.Project, s.entry.Description)
		if math.Abs(s.entry.HoursTotal-s.logged) > 0.01 {
			line += ui.Out.Yellow(fmt.Sprintf(" (AEXT has %.2fh, CSV %.2fh)", s.logged, s.entry.HoursTotal))
		}
		if s.inCSV {
			line += ui.Out.Yellow(" (repeated in CSV)")
		}
		fmt.Println(line)
	}
	if len(skipped) > 0 {
		fmt.Printf("%s skipped %d entries already logged (above).\n", aextLabel(), len(skipped))
	}
	return keep, nil
}

type skippedEntry struct {
	entry  aext.Entry
	logged float64 // hours AEXT (or an earlier CSV row) has for the key
	inCSV  bool    // duplicate of an earlier CSV row, not of an AEXT entry
}

// splitDuplicates keeps entries whose key is neither in existing nor an earlier entry.
func splitDuplicates(entries []aext.Entry, existing []aext.TimeEntry) (keep []aext.Entry, skipped []skippedEntry) {
	logged := map[entryKey]float64{}
	for _, e := range existing {
		logged[keyOf(e.Date, e.Project, e.Description)] += *e.HoursTotal
	}
	sent := map[entryKey]float64{}
	for _, e := range entries {
		k := keyOf(e.Date, e.Project, e.Description)
		if h, ok := logged[k]; ok {
			skipped = append(skipped, skippedEntry{entry: e, logged: h})
		} else if h, ok := sent[k]; ok {
			skipped = append(skipped, skippedEntry{entry: e, logged: h, inCSV: true})
		} else {
			sent[k] = e.HoursTotal
			keep = append(keep, e)
		}
	}
	return keep, skipped
}

// Byte order mark, written as an escape: Go rejects a literal one in source.
const bom = "\xef\xbb\xbf"

// importCSV re-reads the CSV so manual edits made before confirming are what gets sent.
func importCSV(c *aext.Client, file string) error {
	name := filepath.Base(file)
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(records) < 2 {
		return fmt.Errorf("%s has no rows", name)
	}

	header := records[0]
	for i := range header {
		// Excel adds a byte order mark when saving as UTF-8.
		header[i] = strings.TrimSpace(strings.TrimPrefix(header[i], bom))
	}
	var entries []aext.Entry
	for i, rec := range records[1:] {
		r := map[string]string{}
		for col, h := range header {
			if col < len(rec) {
				r[h] = rec[col]
			} else {
				r[h] = ""
			}
		}
		hours, err := strconv.ParseFloat(strings.TrimSpace(r["hours"]), 64)
		if !isoDay.MatchString(r["date"]) || r["project"] == "" || err != nil || hours <= 0 {
			shown, _ := json.Marshal(r)
			return fmt.Errorf("%s row %d is invalid: %s", name, i+2, shown)
		}
		entries = append(entries, aext.Entry{Date: r["date"], Project: r["project"], Description: r["description"], HoursTotal: hours})
	}

	entries, err = skipDuplicates(c, entries)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println(aextLabel(), "every entry is already logged, nothing sent.")
		return nil
	}

	imported, err := c.ImportEntries(entries)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("imported %d of %d entries.", imported, len(entries))
	if imported == len(entries) {
		fmt.Println(aextLabel(), ui.Out.Green(msg))
	} else {
		fmt.Println(aextLabel(), ui.Out.Yellow(msg))
		ui.Warn("AEXT imported a different number of entries than sent.")
	}
	return nil
}
