// jira-handoff, a cm-release tool: hands the "Ready for Production" tickets of an epic over to QA:
// comments asking to test in PROD, assigns the QA owner, moves the ticket to In Production.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"aex/internal/dates"
	"aex/internal/jira"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/ui"
)

const handoffUsage = `Usage: cm-release jira-handoff [--epic <key>] [--dry-run] [--manual]
       cm-release jira-handoff --settings

Finds the epic's child tickets in "Ready for Production" and hands each one off to QA:
comments "Please test in PROD" (To: QA owner, Cc: the always-Cc people), assigns the QA owner,
then moves it to In Production. Every question is asked before anything changes; the plan is
shown, then you pick hand off, dry run or cancel.

Skipped: [Mobile] tickets ([MobileAPI] counts as Web) and tickets of excluded assignees set to
skip. Asked about: tickets with both [Mobile] and [Web], tickets of excluded assignees set to
ask, tickets with more than one QA owner. No QA owner means the default QA.

Release check: releases the epic mentions (fix versions, or named in its summary) are offered to
check against; tickets that became Ready for Production after that release's date are warned
about, to allow (one or all) or decline.

Default project prefix, default QA, always-Cc people and excluded assignees are settings, kept in
<data folder>/plugin-settings/cm-release.json (defaults written on first run). In the window they
are on the Settings page, under Plugins.
A Markdown report is written to <data folder>/output/cm-release/jira-handoff/.

  --epic      Epic key, link, or number in the default project (6996 means CM-6996 unless the
              default prefix is changed; confirmed first). Asked for when left out.
  --dry-run   Change nothing: only show and report what would be done
  --manual    Confirm each ticket (asked for when left out)
  --settings  Change the default QA, always-Cc people and excluded assignees (and the repos
              folder and repos of the git tools)`

const (
	qaField                = "customfield_11215"
	readyStatus            = "Ready for Production"
	transitionInProduction = "111"
)

type user struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

type fields struct {
	Summary  string `json:"summary"`
	Assignee *user  `json:"assignee"`
	QA       []user `json:"customfield_11215"` // qaField
}

type ticket = jira.Issue[fields]

var (
	mobileTag = regexp.MustCompile(`(?i)\[mobile\]`)
	// [MobileAPI] is backend work, so it counts as Web.
	webTag = regexp.MustCompile(`(?i)\[(web|mobileapi)\]`)
)

// Why a ticket was left out.
const (
	reasonMobile       = "[Mobile]"
	reasonMixed        = "[Mobile] and [Web]: you chose to skip"
	reasonExcluded     = "assigned to %s: excluded"
	reasonNotIncluded  = "assigned to %s: not included"
	reasonNotApproved  = "not approved"
	reasonNoTransition = "no In Production transition available"
	reasonNotReached   = "not processed: stopped after a failure"
)

type skip struct {
	t      ticket
	reason string
}

// handoff is a ticket to hand off and how far it got.
type handoff struct {
	t                          ticket
	qa                         person
	commented, assigned, moved bool
	err                        error
	late                       string // why it became ready too late for the release, when allowed anyway
}

type report struct {
	epic        string
	epicSummary string
	release     *jira.Version // checked against, nil when none
	dryRun      bool
	found       int
	handoffs    []*handoff
	skipped     []skip
}

func runHandoff(args []string) error {
	fs := flag.NewFlagSet("jira-handoff", flag.ContinueOnError)
	epicFlag := fs.String("epic", "", "")
	dryRun := fs.Bool("dry-run", false, "")
	manual := fs.Bool("manual", false, "")
	settingsFlag := fs.Bool("settings", false, "")
	if done, err := tool.ParseFlags(fs, args, handoffUsage); done || err != nil {
		return err
	}
	out := ui.Out

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	j, err := jira.New()
	if err != nil {
		return err
	}
	if *settingsFlag {
		return editConfig(j, cfg)
	}

	e, err := resolveEpic(j, cfg.DefaultPrefix, *epicFlag)
	if err != nil {
		return err
	}
	epic := e.Key
	fmt.Printf("Epic: %s %s\n", out.Bold(epic+" "+e.Fields.Summary), out.Dim(j.IssueURL(epic)))
	if *dryRun {
		fmt.Println(out.Yellow("Dry run: nothing will be changed."))
	}
	releases, err := epicReleases(j, e)
	if err != nil {
		return err
	}
	release, err := chooseRelease(releases)
	if err != nil {
		return err
	}

	tickets, err := jira.Search[fields](j, fmt.Sprintf(`parent = %s AND status = "%s" ORDER BY key ASC`, epic, readyStatus),
		[]string{"summary", "assignee", qaField})
	if err != nil {
		return err
	}
	if len(tickets) == 0 {
		fmt.Println("No Ready for Production tickets found.")
		return nil
	}
	fmt.Printf("Found %s Ready for Production ticket(s).\n", out.Bold(fmt.Sprint(len(tickets))))

	r := &report{epic: epic, epicSummary: e.Fields.Summary, release: release, dryRun: *dryRun, found: len(tickets)}
	selected, err := selectTickets(j, cfg, tickets, r)
	if err != nil {
		return err
	}
	if len(selected) > 0 && !*manual {
		if *manual, err = ui.Confirm("Confirm each ticket?", false); err != nil {
			return err
		}
	}
	for _, t := range selected {
		h, err := planTicket(j, t, *manual, cfg.DefaultQA)
		if err != nil {
			return err
		}
		if h == nil {
			r.skipped = append(r.skipped, skip{t, reasonNotApproved})
			continue
		}
		r.handoffs = append(r.handoffs, h)
	}
	if release != nil {
		if err := checkLateTickets(j, r, release); err != nil {
			return err
		}
	}
	if err := checkTransitions(j, r); err != nil {
		return err
	}

	printPlan(r)
	if len(r.handoffs) > 0 && !r.dryRun {
		choice, err := ui.Choose(fmt.Sprintf("Hand off %d ticket(s)?", len(r.handoffs)), []ui.Option{
			{Label: "Hand off", Value: "go"},
			{Label: "Dry run only (change nothing, write the report)", Value: "dry"},
			{Label: "Cancel", Value: "cancel"},
		})
		if err != nil {
			return err
		}
		switch choice {
		case "cancel":
			fmt.Println("Cancelled, nothing changed.")
			return nil
		case "dry":
			r.dryRun = true
		}
	}

	var failed error
	if !r.dryRun {
		failed = execute(j, r, cfg.Cc)
	}
	printSummary(j, r)
	file, err := writeReport(j, r)
	if err != nil {
		ui.Warn("could not write the report: %v", err)
	} else {
		fmt.Printf("\nReport: %s\n", out.Cyan(file))
	}
	return failed
}

type epicFields struct {
	Summary     string         `json:"summary"`
	FixVersions []jira.Version `json:"fixVersions"`
	IssueType   struct {
		Name           string `json:"name"`
		HierarchyLevel int    `json:"hierarchyLevel"`
	} `json:"issuetype"`
}

func isEpic(e *jira.Issue[epicFields]) bool {
	return e.Fields.IssueType.Name == "Epic" || e.Fields.IssueType.HierarchyLevel == 1
}

var ticketNumber = regexp.MustCompile(`^[0-9]+$`)

// epicInput reads a key, a link or a bare number (in project prefix); number says it was a number.
func epicInput(s, prefix string) (key string, number, ok bool) {
	s = strings.TrimSpace(s)
	if ticketNumber.MatchString(s) {
		return prefix + "-" + s, true, true
	}
	key, ok = jira.ParseKey(s)
	return key, false, ok
}

// epicLookup looks tickets up once each, for the prompt: as it is typed (another goroutine) and
// on Enter.
type epicLookup struct {
	j     *jira.Client
	mu    sync.Mutex
	found map[string]*jira.Issue[epicFields]
}

// find returns the ticket, nil when there is none; errors are not kept, so a retry asks again.
func (l *epicLookup) find(key string) (*jira.Issue[epicFields], error) {
	l.mu.Lock()
	e, ok := l.found[key]
	l.mu.Unlock()
	if ok {
		return e, nil
	}
	e, err := jira.FindIssue[epicFields](l.j, key, []string{"summary", "issuetype", "fixVersions"})
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.found[key] = e
	l.mu.Unlock()
	return e, nil
}

// epicProblem says why key cannot be used as the epic, "" when it can.
func epicProblem(key string, e *jira.Issue[epicFields]) string {
	switch {
	case e == nil:
		return key + " not found (or you cannot see it)"
	case !isEpic(e):
		return fmt.Sprintf("%s is a %s, not an epic: %s", key, e.Fields.IssueType.Name, e.Fields.Summary)
	}
	return ""
}

// resolveEpic takes the epic from --epic or asks for it: a key, its link (a pasted link turns into
// the key as soon as it is pasted) or a bare number in project prefix, confirmed before use. While
// typing, the epic's summary shows under the box once the answer is an epic. It must exist and be
// an epic: a bad --epic fails, a bad answer is refused on Enter.
func resolveEpic(j *jira.Client, prefix, flagValue string) (*jira.Issue[epicFields], error) {
	out := ui.Out
	lookup := &epicLookup{j: j, found: map[string]*jira.Issue[epicFields]{}}
	hint := fmt.Sprintf("%s-1234, its link, or just 1234", prefix)
	check := func(s string) error {
		key, _, ok := epicInput(s, prefix)
		if !ok {
			return errors.New("enter " + hint)
		}
		e, err := lookup.find(key)
		if err != nil {
			return err
		}
		if problem := epicProblem(key, e); problem != "" {
			return errors.New(problem)
		}
		return nil
	}
	describe := func(s string) string {
		key, _, ok := epicInput(s, prefix)
		if !ok {
			return ""
		}
		if e, err := lookup.find(key); err == nil && epicProblem(key, e) == "" {
			return e.Key + ": " + e.Fields.Summary
		}
		return ""
	}
	for {
		input := flagValue
		if input == "" {
			var err error
			input, err = ui.Input(ui.Field{
				Title:       "Epic",
				Description: hint,
				Placeholder: prefix + "-1234",
				Validate:    check,
				Paste:       jira.KeyFromPaste,
				Describe:    describe,
			})
			if err != nil {
				return nil, err
			}
		} else if err := check(input); err != nil {
			return nil, fmt.Errorf("--epic %q: %v", input, err)
		}
		key, number, _ := epicInput(input, prefix)
		e, err := lookup.find(key)
		if err != nil {
			return nil, err
		}
		if number {
			fmt.Printf("%s %s\n  %s\n", out.Bold(e.Key), e.Fields.Summary, out.Dim(j.IssueURL(e.Key)))
			yes, err := ui.Confirm("Use epic "+e.Key+"?", true)
			if err != nil {
				return nil, err
			}
			if !yes {
				if flagValue != "" {
					return nil, errors.New("cancelled")
				}
				continue
			}
		}
		return e, nil
	}
}

// selectTickets applies the [Mobile] / [Web] and excluded assignee rules, asking where they need a
// decision. Skipped tickets go into r; the rest are returned in their original order.
func selectTickets(j *jira.Client, cfg *config, tickets []ticket, r *report) ([]ticket, error) {
	out := ui.Out
	keep := make([]bool, len(tickets))
	var mobile []string
	for i, t := range tickets {
		isMobile, isWeb := tags(t.Fields.Summary)
		switch {
		case isMobile && isWeb:
			fmt.Printf("\n%s %s\n  %s\n", out.Bold(t.Key), t.Fields.Summary, out.Dim(j.IssueURL(t.Key)))
			choice, err := ui.Choose(t.Key+" is tagged both [Mobile] and [Web]", []ui.Option{
				{Label: "Hand off (treat as Web)", Value: "web"},
				{Label: "Skip (treat as Mobile)", Value: "skip"},
			})
			if err != nil {
				return nil, err
			}
			if choice == "skip" {
				r.skipped = append(r.skipped, skip{t, reasonMixed})
				continue
			}
		case isMobile:
			r.skipped = append(r.skipped, skip{t, reasonMobile})
			mobile = append(mobile, t.Key)
			continue
		}
		keep[i] = true
	}
	if len(mobile) > 0 {
		fmt.Printf("Skipping [Mobile]: %s\n", strings.Join(mobile, ", "))
	}

	// [Mobile] wins over the excluded assignee rule: only tickets still kept are asked about.
	for _, e := range cfg.Excluded {
		var theirs []int
		for i, t := range tickets {
			if keep[i] && assignedTo(t, e.person) {
				theirs = append(theirs, i)
			}
		}
		if len(theirs) == 0 {
			continue
		}
		keys := make([]string, len(theirs))
		for n, i := range theirs {
			keys[n] = tickets[i].Key
		}
		reason := fmt.Sprintf(reasonExcluded, e.Name)
		if e.Mode == modeSkip {
			fmt.Printf("Skipping %s's tickets (excluded): %s\n", e.Name, strings.Join(keys, ", "))
		} else {
			fmt.Printf("Assigned to %s: %s\n", e.Name, strings.Join(keys, ", "))
			include, err := ui.Confirm(fmt.Sprintf("Include %s's tickets?", e.Name), false)
			if err != nil {
				return nil, err
			}
			if include {
				continue
			}
			reason = fmt.Sprintf(reasonNotIncluded, e.Name)
		}
		for _, i := range theirs {
			keep[i] = false
			r.skipped = append(r.skipped, skip{tickets[i], reason})
		}
	}

	var selected []ticket
	for i, t := range tickets {
		if keep[i] {
			selected = append(selected, t)
		}
	}
	return selected, nil
}

// tags says whether a summary is tagged [Mobile] and whether it is tagged [Web] or [MobileAPI].
func tags(summary string) (mobile, web bool) {
	return mobileTag.MatchString(summary), webTag.MatchString(summary)
}

func assignedTo(t ticket, p person) bool {
	return t.Fields.Assignee != nil && t.Fields.Assignee.AccountID == p.AccountID
}

// planTicket picks the QA owner and, in manual mode, asks to approve the ticket. nil means not
// approved. Choosing between several QA owners counts as approving.
func planTicket(j *jira.Client, t ticket, manual bool, defaultQA person) (*handoff, error) {
	out := ui.Out
	qa := t.Fields.QA
	if len(qa) <= 1 && !manual {
		return &handoff{t: t, qa: qaOwner(qa, defaultQA)}, nil
	}
	assignee := "Unassigned"
	if a := t.Fields.Assignee; a != nil {
		assignee = a.DisplayName
	}
	fmt.Printf("\n%s %s\n  %s\n  %s %s\n", out.Bold(t.Key), t.Fields.Summary, out.Dim(j.IssueURL(t.Key)), out.Dim("Assignee:"), assignee)
	if len(qa) > 1 {
		options := make([]ui.Option, 0, len(qa)+1)
		for i, u := range qa {
			options = append(options, ui.Option{Label: u.DisplayName, Value: fmt.Sprint(i)})
		}
		options = append(options, ui.Option{Label: "Skip this ticket", Value: "skip"})
		choice, err := ui.Choose(t.Key+" has several QA owners, hand off to", options)
		if err != nil || choice == "skip" {
			return nil, err
		}
		var i int
		fmt.Sscan(choice, &i)
		return &handoff{t: t, qa: person{qa[i].DisplayName, qa[i].AccountID}}, nil
	}
	h := &handoff{t: t, qa: qaOwner(qa, defaultQA)}
	fmt.Printf("  %s %s\n", out.Dim("QA:"), h.qa.Name)
	ok, err := ui.Confirm("Hand off "+t.Key+"?", false)
	if err != nil || !ok {
		return nil, err
	}
	return h, nil
}

// qaOwner is the only QA owner, or the default one when none is set.
func qaOwner(qa []user, defaultQA person) person {
	if len(qa) == 0 {
		return defaultQA
	}
	return person{qa[0].DisplayName, qa[0].AccountID}
}

// checkTransitions drops tickets that cannot be moved to In Production now, before anything
// changes, so none is left commented and assigned but not moved.
func checkTransitions(j *jira.Client, r *report) error {
	kept := r.handoffs[:0]
	for _, h := range r.handoffs {
		transitions, err := j.Transitions(h.t.Key)
		if err != nil {
			return err
		}
		available := false
		for _, tr := range transitions {
			available = available || tr.ID == transitionInProduction
		}
		if available {
			kept = append(kept, h)
		} else {
			r.skipped = append(r.skipped, skip{h.t, reasonNoTransition})
		}
	}
	r.handoffs = kept
	return nil
}

// execute hands the tickets off in order: comment, assign, then transition (always last). Stops at
// the first failure without retrying, since the comment may already be posted.
func execute(j *jira.Client, r *report, cc []person) error {
	out := ui.Out
	fmt.Printf("\nHanding off %d ticket(s)...\n", len(r.handoffs))
	for i, h := range r.handoffs {
		fmt.Printf("%s → %s\n", out.Bold(h.t.Key), h.qa.Name)
		step := func(done *bool, label string, do func() error) bool {
			if h.err = do(); h.err != nil {
				fmt.Printf("  %s %s\n", out.Red("✖"), label)
				return false
			}
			*done = true
			fmt.Printf("  %s %s\n", out.Green("✓"), label)
			return true
		}
		_ = step(&h.commented, "commented", func() error { return j.AddComment(h.t.Key, handoffComment(h.qa, cc)) }) &&
			step(&h.assigned, "assigned to "+h.qa.Name, func() error { return j.Assign(h.t.Key, h.qa.AccountID) }) &&
			step(&h.moved, "In Production", func() error { return j.DoTransition(h.t.Key, transitionInProduction) })
		if h.err != nil {
			for _, rest := range r.handoffs[i+1:] {
				r.skipped = append(r.skipped, skip{rest.t, reasonNotReached})
			}
			r.handoffs = r.handoffs[:i+1]
			return fmt.Errorf("%s: %w", h.t.Key, h.err)
		}
	}
	return nil
}

func mention(p person) map[string]any {
	return map[string]any{"type": "mention", "attrs": map[string]any{"id": p.AccountID, "text": "@" + p.Name}}
}

func text(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

// handoffComment is "To: @QA / Cc: @... / Please test in PROD.", leaving the QA owner (and
// repeats) out of Cc, and the Cc line out when that leaves nobody.
func handoffComment(qa person, cc []person) map[string]any {
	content := []any{text("To: "), mention(qa), map[string]any{"type": "hardBreak"}}
	seen := map[string]bool{qa.AccountID: true}
	var ccNodes []any
	for _, p := range cc {
		if seen[p.AccountID] {
			continue
		}
		seen[p.AccountID] = true
		if len(ccNodes) > 0 {
			ccNodes = append(ccNodes, text(" "))
		}
		ccNodes = append(ccNodes, mention(p))
	}
	if len(ccNodes) > 0 {
		content = append(append(append(content, text("Cc: ")), ccNodes...), map[string]any{"type": "hardBreak"})
	}
	content = append(content, text("Please test in PROD."))
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{map[string]any{"type": "paragraph", "content": content}},
	}
}

func printPlan(r *report) {
	out := ui.Out
	fmt.Println()
	if len(r.handoffs) == 0 {
		fmt.Println("Nothing to hand off.")
		return
	}
	fmt.Println(out.Bold(fmt.Sprintf("Plan: hand off %d ticket(s)", len(r.handoffs))))
	for _, h := range r.handoffs {
		fmt.Printf("  %s → QA %s  %s\n", out.Bold(h.t.Key), h.qa.Name, out.Dim(h.t.Fields.Summary))
		if h.late != "" {
			fmt.Printf("    %s\n", out.Yellow("▲ "+h.late+", allowed"))
		}
	}
}

func printSummary(j *jira.Client, r *report) {
	out := ui.Out
	fmt.Println()
	fmt.Println(out.Bold("Production handoff: " + r.epic))
	if r.dryRun {
		fmt.Println(out.Yellow("Dry run: nothing was changed."))
	}
	for _, h := range r.handoffs {
		mark := out.Green("✓")
		if h.err != nil {
			mark = out.Red("✖")
		}
		fmt.Printf("  %s %s → QA: %s  %s\n", mark, out.Bold(h.t.Key), h.qa.Name, out.Dim(result(h, r.dryRun)))
	}
	if len(r.skipped) > 0 {
		fmt.Println(out.Bold(fmt.Sprintf("Skipped (%d):", len(r.skipped))))
		for _, s := range r.skipped {
			fmt.Printf("  %s %s  %s\n", out.Yellow(s.t.Key), out.Dim(j.IssueURL(s.t.Key)), s.reason)
		}
	}
}

// result says what was done to a ticket.
func result(h *handoff, dryRun bool) string {
	late := ""
	if h.late != "" {
		late = "  ▲ " + h.late + ", allowed"
	}
	if dryRun {
		return "would comment, assign, move to In Production" + late
	}
	var done []string
	for _, s := range []struct {
		ok    bool
		label string
	}{{h.commented, "commented"}, {h.assigned, "assigned"}, {h.moved, "In Production"}} {
		if s.ok {
			done = append(done, "✓ "+s.label)
		}
	}
	if h.err != nil {
		done = append(done, "✖ failed: "+firstLine(h.err.Error()))
	}
	return strings.Join(done, "  ") + late
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func writeReport(j *jira.Client, r *report) (string, error) {
	dir := filepath.Join(settings.OutputDir, "cm-release", "jira-handoff")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := r.epic + "-" + dates.FileTimestamp()
	if r.dryRun {
		name += "-dry-run"
	}
	file := filepath.Join(dir, name+".md")
	return file, os.WriteFile(file, []byte(markdown(j, r)), 0o644)
}

func markdown(j *jira.Client, r *report) string {
	link := func(key string) string { return fmt.Sprintf("[%s](%s)", key, j.IssueURL(key)) }
	cell := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ") }

	var b strings.Builder
	fmt.Fprintf(&b, "# Production handoff: %s %s\n\n", r.epic, cell(r.epicSummary))
	fmt.Fprintf(&b, "- Epic: %s\n", link(r.epic))
	if r.release != nil {
		fmt.Fprintf(&b, "- Release: %s\n", cell(releaseLabel(r.release)))
	} else {
		b.WriteString("- Release: none checked\n")
	}
	fmt.Fprintf(&b, "- Run: %s (%s)\n", strings.Replace(dates.FileTimestamp(), "T", " ", 1), dates.TZLabel())
	mode := "hand off"
	if r.dryRun {
		mode = "**dry run**, nothing was changed"
	}
	fmt.Fprintf(&b, "- Mode: %s\n", mode)
	fmt.Fprintf(&b, "- Ready for Production: %d, handed off: %d, skipped: %d\n", r.found, len(r.handoffs), len(r.skipped))

	title := "Handed off"
	if r.dryRun {
		title = "Would hand off"
	}
	fmt.Fprintf(&b, "\n## %s (%d)\n\n", title, len(r.handoffs))
	if len(r.handoffs) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("| Ticket | Summary | QA | Result |\n|---|---|---|---|\n")
		for _, h := range r.handoffs {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", link(h.t.Key), cell(h.t.Fields.Summary), cell(h.qa.Name), cell(result(h, r.dryRun)))
		}
	}

	fmt.Fprintf(&b, "\n## Skipped (%d)\n\n", len(r.skipped))
	if len(r.skipped) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("| Ticket | Summary | Reason |\n|---|---|---|\n")
		for _, s := range r.skipped {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", link(s.t.Key), cell(s.t.Fields.Summary), cell(s.reason))
		}
	}

	for _, h := range r.handoffs {
		if h.err != nil {
			fmt.Fprintf(&b, "\n## Failed: %s\n\n```\n%s\n```\n", h.t.Key, h.err)
		}
	}
	return b.String()
}
