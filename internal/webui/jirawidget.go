package webui

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"aex/internal/dates"
	"aex/internal/jira"
	"aex/internal/tool"
)

// The jira-tickets widget (frontend/widgets/jira-tickets.html): open tickets assigned to you, or
// where you are in people fields you pick (custom fields of one person or several).

const (
	maxTickets  = 50
	jiraTimeout = 10 * time.Second
)

// JiraFieldsReply is the jiraFields API's result: the custom fields that hold people.
type JiraFieldsReply struct {
	NotConfigured bool        `json:"notConfigured,omitempty"` // no JIRA_EMAIL / JIRA_TOKEN: nothing else is set
	NeedLogin     bool        `json:"needLogin,omitempty"`     // Jira rejects the token: nothing else is set
	Fields        []JiraField `json:"fields,omitempty"`
}

// JiraField is a custom field of people.
type JiraField struct {
	ID       string `json:"id"` // customfield_10050
	Name     string `json:"name"`
	Multiple bool   `json:"multiple,omitempty"` // it can hold several people
}

func jiraFieldsAPI(map[string]any) (any, error) {
	c, err := jira.NewQuiet()
	if errors.Is(err, jira.ErrNotConfigured) {
		return JiraFieldsReply{NotConfigured: true}, nil
	}
	if err != nil {
		return nil, err
	}
	all, err := c.Fields()
	if err != nil {
		if rejected() {
			return JiraFieldsReply{NeedLogin: true}, nil
		}
		return nil, err
	}
	reply := JiraFieldsReply{Fields: []JiraField{}}
	for _, f := range all {
		if f.Custom && f.People() && customField.MatchString(f.ID) {
			reply.Fields = append(reply.Fields, JiraField{ID: f.ID, Name: f.Name, Multiple: f.Schema.Type == "array"})
		}
	}
	slices.SortFunc(reply.Fields, func(a, b JiraField) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return reply, nil
}

var customField = regexp.MustCompile(`^customfield_([0-9]{1,12})$`)

// TicketsReply is the jiraTickets API's result.
type TicketsReply struct {
	NotConfigured bool     `json:"notConfigured,omitempty"` // no JIRA_EMAIL / JIRA_TOKEN: nothing else is set
	NeedLogin     bool     `json:"needLogin,omitempty"`     // Jira rejects the token: nothing else is set
	Tickets       []Ticket `json:"tickets"`
	More          bool     `json:"more,omitempty"` // there are more than maxTickets
	// NewHours is how far back an assignment counts as new: 24 h, or 72 h on a Monday (over the
	// weekend). Only for mode "assigned".
	NewHours int `json:"newHours,omitempty"`
}

// Ticket is an open issue as the widget shows it.
type Ticket struct {
	Key      string `json:"key"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
	Category string `json:"category"` // the status's category: new, indeterminate
	Type     string `json:"type,omitempty"`
	Priority string `json:"priority,omitempty"`
	Updated  string `json:"updated,omitempty"`
	New      bool   `json:"new,omitempty"` // assigned to you within NewHours
}

type ticketFields struct {
	Summary string `json:"summary"`
	Status  *struct {
		Name     string `json:"name"`
		Category *struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"status"`
	IssueType *struct {
		Name string `json:"name"`
	} `json:"issuetype"`
	Priority *struct {
		Name string `json:"name"`
	} `json:"priority"`
	Updated string `json:"updated"`
}

var ticketFieldNames = []string{"summary", "status", "issuetype", "priority", "updated"}

// ticketsJQL is the search for args: mode "assigned", or "field" with fields (custom field ids).
func ticketsJQL(args map[string]any) (string, error) {
	const open = " AND statusCategory != Done ORDER BY updated DESC"
	switch args["mode"] {
	case "assigned":
		return "assignee = currentUser()" + open, nil
	case "field":
		list, _ := args["fields"].([]any)
		var in []string
		for _, f := range list {
			id, _ := f.(string)
			m := customField.FindStringSubmatch(id)
			if m == nil {
				return "", fmt.Errorf("not a custom field: %q", id)
			}
			// "=" matches a field of several people too, when you are one of them.
			in = append(in, fmt.Sprintf("cf[%s] = currentUser()", m[1]))
		}
		if len(in) == 0 {
			return "", errors.New("no fields given")
		}
		return "(" + strings.Join(in, " OR ") + ")" + open, nil
	}
	return "", fmt.Errorf("unknown mode %q", args["mode"])
}

// newHours is how far back an assignment counts as new at now: over the weekend on a Monday.
func newHours(now time.Time) int {
	if now.Weekday() == time.Monday {
		return 72
	}
	return 24
}

// jiraTicketsAPI gives the open tickets for args (ticketsJQL), most recently updated first. For
// mode "assigned" it marks those assigned to you within newHours (or created so).
func jiraTicketsAPI(args map[string]any) (any, error) {
	jql, err := ticketsJQL(args)
	if err != nil {
		return nil, err
	}
	c, err := jira.NewQuiet()
	if errors.Is(err, jira.ErrNotConfigured) {
		return TicketsReply{NotConfigured: true}, nil
	}
	if err != nil {
		return nil, err
	}
	issues, more, err := jira.SearchMax[ticketFields](c, jql, ticketFieldNames, maxTickets)
	if err != nil {
		if rejected() {
			return TicketsReply{NeedLogin: true}, nil
		}
		return nil, err
	}
	reply := TicketsReply{Tickets: []Ticket{}, More: more}
	fresh := map[string]bool{}
	if args["mode"] == "assigned" && len(issues) > 0 {
		reply.NewHours = newHours(dates.Now())
		since := fmt.Sprintf(`"-%dh"`, reply.NewHours)
		newJQL := fmt.Sprintf("assignee = currentUser() AND statusCategory != Done AND (assignee CHANGED TO currentUser() AFTER %s OR created >= %s)", since, since)
		recent, _, err := jira.SearchMax[struct{}](c, newJQL, []string{"summary"}, maxTickets)
		if err != nil {
			return nil, err
		}
		for _, is := range recent {
			fresh[is.Key] = true
		}
	}
	for _, is := range issues {
		f := is.Fields
		t := Ticket{Key: is.Key, Summary: f.Summary, Updated: f.Updated, New: fresh[is.Key]}
		if f.Status != nil {
			t.Status = f.Status.Name
			if f.Status.Category != nil {
				t.Category = f.Status.Category.Key
			}
		}
		if f.IssueType != nil {
			t.Type = f.IssueType.Name
		}
		if f.Priority != nil {
			t.Priority = f.Priority.Name
		}
		reply.Tickets = append(reply.Tickets, t)
	}
	return reply, nil
}

// jiraOpenAPI opens args.key's page in the browser.
func jiraOpenAPI(args map[string]any) (any, error) {
	s, _ := args["key"].(string)
	key, ok := jira.ParseKey(s)
	if !ok {
		return nil, fmt.Errorf("not an issue key: %q", s)
	}
	c, err := jira.NewQuiet()
	if err != nil {
		return nil, err
	}
	return nil, tool.OpenURL(c.IssueURL(key))
}

// rejected reports whether Jira rejects the configured token, after a call failed.
func rejected() bool {
	me, err := jira.WhoAmI(jiraTimeout)
	return err == nil && me == nil
}
