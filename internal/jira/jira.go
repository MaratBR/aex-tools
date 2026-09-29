// Package jira is a minimal Jira Cloud REST v3 client (API token basic auth).
package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"aex/internal/dates"
	"aex/internal/httpx"
	"aex/internal/settings"
)

type Client struct {
	baseURL string
	auth    string
}

func New() (*Client, error) {
	base, err := settings.Require("JIRA_BASE_URL")
	if err != nil {
		return nil, err
	}
	email, err := settings.RequireAuth("JIRA_EMAIL")
	if err != nil {
		return nil, err
	}
	token, err := settings.RequireAuth("JIRA_TOKEN")
	if err != nil {
		return nil, err
	}
	return newClient(base, email, token), nil
}

func newClient(base, email, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(base, "/"),
		auth:    "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
	}
}

// Me is the Jira user the API token belongs to.
type Me struct {
	AccountID    string `json:"accountId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"` // empty when hidden by the profile's privacy settings
	AccountType  string `json:"accountType"`
	Active       bool   `json:"active"`
	TimeZone     string `json:"timeZone"`
	Locale       string `json:"locale"`
}

// ErrNotConfigured means JIRA_EMAIL or JIRA_TOKEN is not set.
var ErrNotConfigured = errors.New("Jira login not configured")

// WhoAmI returns the user the configured token belongs to, or nil when Jira rejects it. Never
// prompts or prints (safe to call from the menu); ErrNotConfigured when the login is not set.
func WhoAmI(timeout time.Duration) (*Me, error) {
	base, err := settings.Require("JIRA_BASE_URL")
	if err != nil {
		return nil, err
	}
	email, token := settings.Get("JIRA_EMAIL"), settings.Get("JIRA_TOKEN")
	if email == "" || token == "" {
		return nil, ErrNotConfigured
	}
	return Check(base, email, token, timeout)
}

// Check returns the user an email + API token belong to, or nil when Jira rejects them. Never
// prompts or prints.
func Check(base, email, token string, timeout time.Duration) (*Me, error) {
	c := newClient(base, email, token)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := httpx.NewRequest("GET", c.baseURL+"/rest/api/3/myself", nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", c.auth)
	res, err := httpx.Client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("not reachable (timed out)")
		}
		return nil, fmt.Errorf("Jira GET /rest/api/3/myself: %w", err)
	}
	defer httpx.Discard(res)
	switch res.StatusCode {
	case 401, 403:
		return nil, nil
	case 200:
	default:
		return nil, fmt.Errorf("Jira GET /rest/api/3/myself: HTTP %d", res.StatusCode)
	}
	var me Me
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil || me.AccountID == "" {
		return nil, errors.New("Jira GET /rest/api/3/myself: unexpected response")
	}
	if me.EmailAddress == "" {
		me.EmailAddress = email
	}
	return &me, nil
}

func request[T any](c *Client, method, path string, query url.Values, body any, validate func(T) bool) (T, error) {
	var zero T
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := httpx.NewRequest(method, u, body)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Authorization", c.auth)
	res, err := httpx.Client.Do(req)
	if err != nil {
		return zero, fmt.Errorf("Jira %s %s: %w", method, path, err)
	}
	return httpx.ReadJSON(fmt.Sprintf("Jira %s %s", method, req.URL.Path), res, nil, validate)
}

type issue struct {
	Key    string `json:"key"`
	Fields struct {
		Project *struct {
			Key string `json:"key"`
		} `json:"project"`
	} `json:"fields"`
}

type searchPage struct {
	Issues        *[]issue `json:"issues"`
	NextPageToken string   `json:"nextPageToken"`
}

type worklog struct {
	Started          *string `json:"started"`
	TimeSpentSeconds *int    `json:"timeSpentSeconds"`
	Author           *struct {
		AccountID string `json:"accountId"`
	} `json:"author"`
}

type worklogPage struct {
	Worklogs *[]worklog `json:"worklogs"`
	Total    *int       `json:"total"`
}

func (c *Client) searchIssues(jql string, fields []string) ([]issue, error) {
	type body struct {
		JQL           string   `json:"jql"`
		Fields        []string `json:"fields"`
		MaxResults    int      `json:"maxResults"`
		NextPageToken string   `json:"nextPageToken,omitempty"`
	}
	var issues []issue
	token := ""
	for {
		page, err := request(c, "POST", "/rest/api/3/search/jql", nil, body{jql, fields, 100, token},
			func(p searchPage) bool { return p.Issues != nil })
		if err != nil {
			return nil, err
		}
		issues = append(issues, *page.Issues...)
		if token = page.NextPageToken; token == "" {
			return issues, nil
		}
	}
}

func (c *Client) issueWorklogs(key string, startedAfter, startedBefore time.Time) ([]worklog, error) {
	var logs []worklog
	for {
		query := url.Values{
			"startAt":       {fmt.Sprint(len(logs))},
			"maxResults":    {"5000"},
			"startedAfter":  {fmt.Sprint(startedAfter.UnixMilli())},
			"startedBefore": {fmt.Sprint(startedBefore.UnixMilli())},
		}
		page, err := request(c, "GET", "/rest/api/3/issue/"+url.PathEscape(key)+"/worklog", query, nil, func(p worklogPage) bool {
			if p.Worklogs == nil || p.Total == nil {
				return false
			}
			for _, w := range *p.Worklogs {
				if w.Started == nil || w.TimeSpentSeconds == nil {
					return false
				}
			}
			return true
		})
		if err != nil {
			return nil, err
		}
		logs = append(logs, *page.Worklogs...)
		if len(*page.Worklogs) == 0 || len(logs) >= *page.Total {
			return logs, nil
		}
	}
}

// Worklog is time the current user logged on an issue on a day.
type Worklog struct {
	IssueKey   string
	ProjectKey string
	Day        string
	Seconds    int
}

// parseStarted reads Jira's "2026-09-28T09:00:00.000+0200".
func parseStarted(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02T15:04:05.000-0700", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// FetchMyWorklogs returns the current user's worklogs whose start falls on a day in r (days in the configured TZ).
func (c *Client) FetchMyWorklogs(r dates.Range) ([]Worklog, error) {
	me, err := request(c, "GET", "/rest/api/3/myself", nil, nil, func(m struct {
		AccountID string `json:"accountId"`
	}) bool {
		return m.AccountID != ""
	})
	if err != nil {
		return nil, err
	}
	startedAfter := dates.DayStart(r.From)
	startedBefore := dates.DayStart(dates.AddDays(r.To, 1))

	// worklogDate uses the Jira profile timezone, so widen by a day and filter exactly below.
	jql := fmt.Sprintf(`worklogAuthor = currentUser() AND worklogDate >= "%s" AND worklogDate <= "%s"`,
		dates.AddDays(r.From, -1), dates.AddDays(r.To, 1))
	issues, err := c.searchIssues(jql, []string{"project"})
	if err != nil {
		return nil, err
	}

	perIssue := make([][]Worklog, len(issues))
	errs := make([]error, len(issues))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i, is := range issues {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			perIssue[i], errs[i] = c.myIssueWorklogs(is, me.AccountID, startedAfter, startedBefore, r)
		}()
	}
	wg.Wait()

	var all []Worklog
	for i := range issues {
		if errs[i] != nil {
			return nil, errs[i]
		}
		all = append(all, perIssue[i]...)
	}
	return all, nil
}

func (c *Client) myIssueWorklogs(is issue, accountID string, after, before time.Time, r dates.Range) ([]Worklog, error) {
	logs, err := c.issueWorklogs(is.Key, after, before)
	if err != nil {
		return nil, err
	}
	project, _, _ := strings.Cut(is.Key, "-")
	if is.Fields.Project != nil && is.Fields.Project.Key != "" {
		project = is.Fields.Project.Key
	}
	var mine []Worklog
	for _, w := range logs {
		if w.Author == nil || w.Author.AccountID != accountID {
			continue
		}
		started, err := parseStarted(*w.Started)
		if err != nil {
			return nil, fmt.Errorf("Jira worklog on %s: bad start time %q", is.Key, *w.Started)
		}
		day := dates.DayOf(started)
		if day >= r.From && day <= r.To {
			mine = append(mine, Worklog{is.Key, project, day, *w.TimeSpentSeconds})
		}
	}
	return mine, nil
}
