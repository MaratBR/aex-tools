// Package aext is the AEXT time-tracking API client. Auth is email + one-time code; the auth_token
// cookie is kept in the credential store (settings.Credentials) and refreshed automatically when
// the server rejects it.
package aext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"aex/internal/httpx"
	"aex/internal/secrets"
	"aex/internal/settings"
	"aex/internal/ui"
)

type Client struct {
	baseURL string

	mu      sync.Mutex
	cookie  string
	pending *loginCall
}

// Requests run in parallel; they share one login instead of each asking for the email and a code.
type loginCall struct {
	done   chan struct{}
	cookie string
	err    error
}

func New() (*Client, error) {
	base, err := settings.Require("AEXT_BASE_URL")
	if err != nil {
		return nil, err
	}
	cookie, err := settings.Credentials.Get(secrets.AEXTSession)
	if err != nil {
		return nil, err
	}
	return &Client{baseURL: strings.TrimRight(base, "/"), cookie: cookie}, nil
}

func (c *Client) currentCookie() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cookie
}

func (c *Client) send(ctx context.Context, method, path string, query url.Values, body any, cookie string) (*http.Response, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := httpx.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	res, err := httpx.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AEXT %s %s: %w", method, path, err)
	}
	return res, nil
}

func (c *Client) login() (string, error) {
	if err := ui.AssertInteractive("AEXT login"); err != nil {
		return "", err
	}
	// Asked for only on login, so a cached session works without it.
	email, err := settings.RequireAuth("AEXT_EMAIL")
	if err != nil {
		return "", err
	}
	res, err := c.send(context.Background(), "POST", "/api/auth/request-code", nil, map[string]any{"email": email}, "")
	if err != nil {
		return "", err
	}
	if _, err := httpx.ReadJSON[any]("AEXT POST /api/auth/request-code", res, []int{200, 204}, nil); err != nil {
		return "", err
	}

	code, err := ui.Input(ui.Field{
		Title:       "AEXT login code",
		Description: "Sent to " + email,
		Placeholder: "code from the email",
		Validate:    ui.Required("code"),
	})
	if err != nil {
		return "", err
	}
	const label = "AEXT POST /api/auth/verify-code"
	res, err = c.send(context.Background(), "POST", "/api/auth/verify-code", nil, map[string]any{"email": email, "code": code, "remember": true}, "")
	if err != nil {
		return "", err
	}
	data, err := httpx.ReadJSON(label, res, nil, func(d struct {
		OK bool `json:"ok"`
	}) bool {
		return d.OK
	})
	if err != nil {
		return "", err
	}

	cookie := ""
	for _, ck := range res.Cookies() {
		if ck.Name == "auth_token" {
			cookie = "auth_token=" + ck.Value
		}
	}
	if cookie == "" {
		body, _ := json.Marshal(data)
		httpx.Dump(label, res, body, "no auth_token cookie")
		return "", fmt.Errorf("%s: no auth_token cookie (details above)", label)
	}

	if err := settings.Credentials.Set(secrets.AEXTSession, cookie); err != nil {
		return "", err
	}
	fmt.Fprintln(os.Stderr, ui.Err.Green("AEXT login ok, session saved to "+settings.Credentials.Name()+"."))
	return cookie, nil
}

// ensureLogin logs in, or waits for the login another request already started.
func (c *Client) ensureLogin() (string, error) {
	c.mu.Lock()
	if c.cookie != "" {
		defer c.mu.Unlock()
		return c.cookie, nil
	}
	if p := c.pending; p != nil {
		c.mu.Unlock()
		<-p.done
		return p.cookie, p.err
	}
	p := &loginCall{done: make(chan struct{})}
	c.pending = p
	c.mu.Unlock()

	p.cookie, p.err = c.login()

	c.mu.Lock()
	c.pending = nil
	if p.err == nil {
		c.cookie = p.cookie
	}
	c.mu.Unlock()
	close(p.done)
	return p.cookie, p.err
}

// dropCookie forgets a rejected cookie. Only the first request rejected with it drops it; the rest
// then wait for that request's login.
func (c *Client) dropCookie(rejected string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cookie != rejected {
		return
	}
	fmt.Fprintln(os.Stderr, ui.Err.Yellow("AEXT session expired, logging in again."))
	c.cookie = ""
	settings.Credentials.Delete(secrets.AEXTSession)
}

func request[T any](c *Client, method, path string, query url.Values, body any, validate func(T) bool) (T, error) {
	var zero T
	cookie := c.currentCookie()
	if cookie == "" {
		var err error
		if cookie, err = c.ensureLogin(); err != nil {
			return zero, err
		}
	}
	res, err := c.send(context.Background(), method, path, query, body, cookie)
	if err != nil {
		return zero, err
	}
	if res.StatusCode == 401 || res.StatusCode == 403 {
		httpx.Discard(res)
		c.dropCookie(cookie)
		if cookie, err = c.ensureLogin(); err != nil {
			return zero, err
		}
		if res, err = c.send(context.Background(), method, path, query, body, cookie); err != nil {
			return zero, err
		}
	}
	return httpx.ReadJSON(fmt.Sprintf("AEXT %s %s", method, path), res, nil, validate)
}

// Me is the logged-in AEXT user. Fields holds every top-level field of the response, for details.
type Me struct {
	Email       *string        `json:"email"`
	DisplayName string         `json:"display_name"`
	UserName    string         `json:"user_name"`
	Fields      map[string]any `json:"-"`
}

// WhoAmI returns the logged-in user, or nil when there is no valid session. Never logs in or
// prints (safe to call for the window header); fails on network/server errors.
func (c *Client) WhoAmI(timeout time.Duration) (*Me, error) {
	cookie := c.currentCookie()
	if cookie == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err := c.send(ctx, "GET", "/api/auth/me", nil, nil, cookie)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("not reachable (timed out)")
		}
		return nil, err
	}
	defer httpx.Discard(res)
	switch res.StatusCode {
	case 401, 403:
		return nil, nil
	case 200:
	default:
		return nil, fmt.Errorf("AEXT GET /api/auth/me: HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	var me Me
	if err != nil || json.Unmarshal(body, &me) != nil || json.Unmarshal(body, &me.Fields) != nil || me.Email == nil {
		return nil, errors.New("AEXT GET /api/auth/me: unexpected response")
	}
	rememberEmail(*me.Email)
	return &me, nil
}

// rememberEmail fills in an unset AEXT_EMAIL with the logged-in user's email, and saves it to app
// settings so the next login does not ask for it. A failed save is ignored (WhoAmI never prints):
// the email is still set for this run, and a later login offers to save it again.
func rememberEmail(email string) {
	if email == "" || settings.Get("AEXT_EMAIL") != "" {
		return
	}
	settings.Set("AEXT_EMAIL", email)
	if !settings.IsPlugin {
		_ = settings.SaveAppSettings([]settings.Change{{Name: "AEXT_EMAIL", Value: &email}})
	}
}

// HasSession reports whether a session cookie is cached (it may still be expired).
func (c *Client) HasSession() bool { return c.currentCookie() != "" }

// Login logs in with a new code even when a session is cached, replacing it.
func (c *Client) Login() error {
	c.mu.Lock()
	c.cookie = ""
	c.mu.Unlock()
	_, err := c.ensureLogin()
	return err
}

// Logout ends the session on the server (POST /api/auth/logout, 204), then wipes it locally.
// expired is true when the server had already dropped the session.
func (c *Client) Logout() (expired bool, err error) {
	cookie := c.currentCookie()
	if cookie == "" {
		return false, errors.New("not logged in to AEXT (no session)")
	}
	res, err := c.send(context.Background(), "POST", "/api/auth/logout", nil, nil, cookie)
	if err != nil {
		return false, err
	}
	if res.StatusCode == 401 || res.StatusCode == 403 {
		httpx.Discard(res)
		expired = true
	} else if _, err := httpx.ReadJSON[any]("AEXT POST /api/auth/logout", res, []int{204}, nil); err != nil {
		return false, err
	}
	return expired, c.WipeSession()
}

// WipeSession forgets the session locally (deletes it from the credential store) without telling
// the server.
func (c *Client) WipeSession() error {
	c.mu.Lock()
	c.cookie = ""
	c.mu.Unlock()
	return settings.Credentials.Delete(secrets.AEXTSession)
}

// DaySummary is the hours logged on a day.
type DaySummary struct {
	Date       string   `json:"date"`
	HoursTotal *float64 `json:"hours_total"`
}

// TimeSummary returns only days with entries.
func (c *Client) TimeSummary(start, end string) ([]DaySummary, error) {
	return list(c, "/api/time-entries/summary", url.Values{"start_date": {start}, "end_date": {end}},
		func(s DaySummary) bool { return s.Date != "" && s.HoursTotal != nil })
}

// TimeEntry is a logged time entry.
type TimeEntry struct {
	ID          *int64   `json:"id"`
	Date        string   `json:"date"`
	Project     string   `json:"project"`
	Description string   `json:"description"`
	HoursTotal  *float64 `json:"hours_total"`
}

// TimeEntries returns every entry in range (dates inclusive).
func (c *Client) TimeEntries(start, end string) ([]TimeEntry, error) {
	return list(c, "/api/time-entries/", url.Values{"start_date": {start}, "end_date": {end}},
		func(e TimeEntry) bool { return e.ID != nil && e.Date != "" && e.HoursTotal != nil })
}

// list GETs a JSON array whose every item passes valid.
func list[T any](c *Client, path string, query url.Values, valid func(T) bool) ([]T, error) {
	items, err := request(c, "GET", path, query, nil, func(d *[]T) bool {
		if d == nil {
			return false
		}
		for _, item := range *d {
			if !valid(item) {
				return false
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return *items, nil
}

// WorkingDay is a day of the AEXT working-days calendar.
type WorkingDay struct {
	Date         string `json:"date"`
	IsWorkingDay *bool  `json:"is_working_day"`
}

// WorkingDays returns every day in range.
func (c *Client) WorkingDays(start, end string) ([]WorkingDay, error) {
	type period struct {
		Days *[]WorkingDay `json:"days"`
	}
	query := url.Values{"start_date": {start}, "end_date": {end}, "country_code": {settings.WorkingDaysCountry}}
	p, err := request(c, "GET", "/api/working-days/period", query, nil, func(p period) bool {
		if p.Days == nil {
			return false
		}
		for _, d := range *p.Days {
			if d.Date == "" || d.IsWorkingDay == nil {
				return false
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return *p.Days, nil
}

type emailRef struct {
	Email string `json:"email"`
}

// Leave is a leave request. Personal fields (names, position) come back redacted; only ids,
// emails, dates, type and status are used.
type Leave struct {
	ID        any       `json:"id"`
	User      *emailRef `json:"user"`
	LeaveType string    `json:"leave_type"`
	Start     string    `json:"start"`
	End       string    `json:"end"`
	Status    string    `json:"status"`
	Created   *struct {
		User *emailRef `json:"user"`
	} `json:"created"`
}

func (c *Client) MyLeaves(start, end string) ([]Leave, error) {
	return list(c, "/api/leaves/my-requests", url.Values{"start_date": {start}, "end_date": {end}},
		func(l Leave) bool { return l.ID != nil && l.Start != "" && l.End != "" })
}

// Entry is a time entry to import.
type Entry struct {
	Date        string  `json:"date"`
	Project     string  `json:"project"`
	Description string  `json:"description"`
	HoursTotal  float64 `json:"hours_total"`
}

// ImportEntries returns how many entries AEXT imported.
func (c *Client) ImportEntries(entries []Entry) (int, error) {
	type result struct {
		ImportedCount *float64 `json:"imported_count"`
	}
	r, err := request(c, "POST", "/api/time-entries/import", nil, map[string]any{"entries": entries},
		func(r result) bool { return r.ImportedCount != nil })
	if err != nil {
		return 0, err
	}
	return int(*r.ImportedCount), nil
}
