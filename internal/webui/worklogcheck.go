package webui

import (
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/jira"
	"aex/internal/settings"
)

// The quota widget's check of this week: the hours logged in Jira (as worklog-sync would send them:
// per day and issue, the project mapped) against the AEXT entries for the same days.

// WorklogCheck is the worklogCheck API's result.
type WorklogCheck struct {
	NotConfigured bool          `json:"notConfigured,omitempty"` // no Jira login: nothing else is set
	NeedLogin     bool          `json:"needLogin,omitempty"`     // Jira rejects the token: nothing else is set
	NoSession     bool          `json:"noSession,omitempty"`     // no AEXT session: nothing else is set
	From          string        `json:"from,omitempty"`
	To            string        `json:"to,omitempty"`
	Jira          float64       `json:"jira"` // hours in Jira over the week
	AEXT          float64       `json:"aext"` // hours of the AEXT entries for Jira issues over the week
	Diffs         []WorklogDiff `json:"diffs"`
}

// WorklogDiff is a day and issue whose hours differ between Jira and AEXT (0: none there).
type WorklogDiff struct {
	Date    string  `json:"date"`
	Issue   string  `json:"issue"`
	Project string  `json:"project"`
	Jira    float64 `json:"jira"`
	AEXT    float64 `json:"aext"`
}

// jiraWeekMaxAge is how long the week's Jira worklogs are reused (one request per issue) unless the
// widget asks for fresh ones; AEXT's entries are asked for every time.
const jiraWeekMaxAge = 5 * time.Minute

var jiraWeek struct {
	sync.Mutex
	r    dates.Range
	at   time.Time
	logs []jira.Worklog
}

func worklogCheckAPI(args map[string]any) (any, error) {
	r, err := dates.ParseRange("this week", dates.Today())
	if err != nil {
		return nil, err
	}
	jc, err := jira.NewQuiet()
	if errors.Is(err, jira.ErrNotConfigured) {
		return WorklogCheck{NotConfigured: true}, nil
	}
	if err != nil {
		return nil, err
	}
	ac, err := aext.NewQuiet()
	if err != nil {
		return nil, err
	}
	if !ac.HasSession() {
		return WorklogCheck{NoSession: true}, nil
	}
	fresh, _ := args["fresh"].(bool)

	var logs []jira.Worklog
	var entries []aext.TimeEntry
	var jiraErr, aextErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		logs, jiraErr = weekWorklogs(jc, r, fresh)
	}()
	go func() {
		defer wg.Done()
		entries, aextErr = ac.TimeEntries(r.From, r.To)
	}()
	wg.Wait()
	if errors.Is(aextErr, aext.ErrNoSession) {
		return WorklogCheck{NoSession: true}, nil
	}
	if aextErr != nil {
		return nil, aextErr
	}
	if jiraErr != nil {
		if rejected() {
			return WorklogCheck{NeedLogin: true}, nil
		}
		return nil, jiraErr
	}
	check := compareWorklogs(logs, entries)
	check.From, check.To = r.From, r.To
	return check, nil
}

// weekWorklogs is the user's Jira worklogs in r, reused for jiraWeekMaxAge unless fresh.
func weekWorklogs(c *jira.Client, r dates.Range, fresh bool) ([]jira.Worklog, error) {
	jiraWeek.Lock()
	defer jiraWeek.Unlock()
	if !fresh && jiraWeek.r == r && time.Since(jiraWeek.at) < jiraWeekMaxAge {
		return jiraWeek.logs, nil
	}
	logs, err := c.FetchMyWorklogs(r)
	if err != nil {
		return nil, err
	}
	jiraWeek.r, jiraWeek.at, jiraWeek.logs = r, time.Now(), logs
	return logs, nil
}

var issueDescription = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[0-9]+$`)

// compareWorklogs matches Jira's hours per day and issue (as worklog-sync's CSV rows: the project
// mapped, hours rounded to hundredths) with AEXT entries of the same day, project and description.
// AEXT entries whose description is not an issue key (logged by hand, not from Jira) are left out.
func compareWorklogs(logs []jira.Worklog, entries []aext.TimeEntry) WorklogCheck {
	type key struct{ date, project, issue string }
	seconds := map[key]int{}
	for _, w := range logs {
		project := w.ProjectKey
		if mapped, ok := settings.ProjectMap[project]; ok {
			project = mapped
		}
		seconds[key{w.Day, project, w.IssueKey}] += w.Seconds
	}
	inJira := map[key]float64{}
	check := WorklogCheck{Diffs: []WorklogDiff{}}
	for k, s := range seconds {
		h := math.Round(float64(s)/36) / 100
		inJira[k] = h
		check.Jira += h
	}
	inAEXT := map[key]float64{}
	for _, e := range entries {
		k := key{e.Date, strings.TrimSpace(e.Project), strings.TrimSpace(e.Description)}
		if _, ok := inJira[k]; !ok && !issueDescription.MatchString(k.issue) {
			continue
		}
		inAEXT[k] += *e.HoursTotal
		check.AEXT += *e.HoursTotal
	}
	add := func(k key) {
		if math.Abs(inJira[k]-inAEXT[k]) > 0.01 {
			check.Diffs = append(check.Diffs, WorklogDiff{k.date, k.issue, k.project, inJira[k], inAEXT[k]})
		}
	}
	for k := range inJira {
		add(k)
	}
	for k := range inAEXT {
		if _, ok := inJira[k]; !ok {
			add(k)
		}
	}
	sort.Slice(check.Diffs, func(i, j int) bool {
		a, b := check.Diffs[i], check.Diffs[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		return a.Issue < b.Issue
	})
	check.Jira = math.Round(check.Jira*100) / 100
	check.AEXT = math.Round(check.AEXT*100) / 100
	return check
}
