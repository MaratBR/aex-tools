package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"aex/internal/dates"
	"aex/internal/jira"
	"aex/internal/ui"
)

// epicReleases are the releases the epic mentions: its fix versions, plus project releases whose
// name is in its summary.
func epicReleases(j *jira.Client, e *jira.Issue[epicFields]) ([]jira.Version, error) {
	releases := slices.Clone(e.Fields.FixVersions)
	project, _, _ := strings.Cut(e.Key, "-")
	all, err := j.ProjectVersions(project)
	if err != nil {
		return nil, err
	}
	for _, v := range all {
		if mentions(e.Fields.Summary, v.Name) && !slices.ContainsFunc(releases, func(r jira.Version) bool { return r.ID == v.ID }) {
			releases = append(releases, v)
		}
	}
	return releases, nil
}

// mentions reports whether text names the release: "5.12" in "Release 5.12." but not in "5.12.1"
// or "15.12".
func mentions(text, name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	re := regexp.MustCompile(`(?i)(^|[^\w.])` + regexp.QuoteMeta(name) + `($|[^\w.]|\.(\D|$))`)
	return re.MatchString(text)
}

// chooseRelease asks which release the epic belongs to: to approve the one it mentions, or to pick
// one of several (or none). nil means none; a release without a date cannot be checked.
func chooseRelease(releases []jira.Version) (*jira.Version, error) {
	var chosen *jira.Version
	switch len(releases) {
	case 0:
		return nil, nil
	case 1:
		fmt.Printf("Epic mentions release %s\n", ui.Out.Bold(releaseLabel(&releases[0])))
		yes, err := ui.Confirm("Check tickets against release "+releases[0].Name+"?", true)
		if err != nil || !yes {
			return nil, err
		}
		chosen = &releases[0]
	default:
		options := make([]ui.Option, 0, len(releases)+1)
		for i := range releases {
			options = append(options, ui.Option{Label: releaseLabel(&releases[i]), Value: fmt.Sprint(i)})
		}
		options = append(options, ui.Option{Label: "None (do not check against a release)", Value: "none"})
		choice, err := ui.Choose("Epic mentions several releases, check tickets against", options)
		if err != nil || choice == "none" {
			return nil, err
		}
		var i int
		fmt.Sscan(choice, &i)
		chosen = &releases[i]
	}
	if chosen.ReleaseDate == "" {
		ui.Warn("release %s has no release date, tickets are not checked against it", chosen.Name)
		return nil, nil
	}
	return chosen, nil
}

func releaseLabel(v *jira.Version) string {
	state := "not released yet"
	if v.Released {
		state = "released"
	}
	date := "no release date"
	if v.ReleaseDate != "" {
		date = "release date " + v.ReleaseDate
	}
	return fmt.Sprintf("%s (%s, %s)", v.Name, date, state)
}

// checkLateTickets warns about tickets moved to Ready for Production after the release date and
// asks, per ticket, to allow it, allow all such tickets, or decline (skip) it.
func checkLateTickets(j *jira.Client, r *report, release *jira.Version) error {
	out := ui.Out
	allowAll := false
	kept := r.handoffs[:0]
	for _, h := range r.handoffs {
		at, ok, err := j.StatusChangedTo(h.t.Key, readyStatus)
		if err != nil {
			return err
		}
		day := dates.DayOf(at)
		if !ok || day <= release.ReleaseDate {
			kept = append(kept, h)
			continue
		}
		h.late = fmt.Sprintf("moved to %s on %s, after release %s (%s)", readyStatus, day, release.Name, release.ReleaseDate)
		if allowAll {
			kept = append(kept, h)
			continue
		}
		fmt.Printf("\n%s %s %s\n  %s\n  %s\n", out.Bold(out.Yellow("▲")), out.Bold(h.t.Key), h.t.Fields.Summary,
			out.Dim(j.IssueURL(h.t.Key)), out.Yellow(h.late))
		choice, err := ui.Choose(h.t.Key+" became ready after the release date", []ui.Option{
			{Label: "Allow this ticket", Value: "one"},
			{Label: "Allow all tickets like this", Value: "all"},
			{Label: "Decline (skip this ticket)", Value: "decline"},
		})
		if err != nil {
			return err
		}
		switch choice {
		case "decline":
			r.skipped = append(r.skipped, skip{h.t, h.late + ": declined"})
			continue
		case "all":
			allowAll = true
		}
		kept = append(kept, h)
	}
	r.handoffs = kept
	return nil
}
