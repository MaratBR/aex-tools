// Package google is the Google API client: signing in with OAuth (an installed app: the browser
// comes back to a local address, see login.go) and the Calendar API. The OAuth client is the
// GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET settings, built in through .env. The login (refresh
// token, the account's email, the scopes granted) is kept in the credential store
// (settings.Credentials); access tokens only in memory.
package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/httpx"
	"aex/internal/secrets"
	"aex/internal/settings"
	"aex/internal/ui"
)

// Endpoints, variables for the tests.
var (
	authURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenURL    = "https://oauth2.googleapis.com/token"
	revokeURL   = "https://oauth2.googleapis.com/revoke"
	calendarURL = "https://www.googleapis.com/calendar/v3"
)

// CalendarScope is what aex asks for: reading calendars and their events.
const CalendarScope = "https://www.googleapis.com/auth/calendar.readonly"

// scopes asked for on login: the email identifies the account (from the ID token).
var scopes = []string{"openid", "email", CalendarScope}

var (
	// ErrNotConfigured means there is no OAuth client: GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET not set.
	ErrNotConfigured = errors.New("Google OAuth client not set (GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET, see configure)")
	// ErrNoLogin is what a quiet client (NewQuiet) returns where another would log in.
	ErrNoLogin = errors.New("not logged in to Google")
)

// saved is the login kept in the credential store.
type saved struct {
	ClientID string `json:"client_id"` // the client it was granted to: another client cannot use it
	Refresh  string `json:"refresh_token"`
	Email    string `json:"email,omitempty"`
	Scope    string `json:"scope,omitempty"` // granted, space separated
}

type Client struct {
	clientID, secret string
	quiet            bool // never logs in or prints: see NewQuiet

	mu     sync.Mutex // held while refreshing or logging in, so parallel requests share one
	login  *saved
	access string
	expiry time.Time
}

// New is a client that logs in (in the browser) when there is no valid login.
func New() (*Client, error) {
	id, secret := settings.Get("GOOGLE_CLIENT_ID"), settings.Get("GOOGLE_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil, ErrNotConfigured
	}
	return newClient(id, secret)
}

func newClient(id, secret string) (*Client, error) {
	c := &Client{clientID: id, secret: secret}
	raw, err := settings.Credentials.Get(secrets.GoogleLogin)
	if err != nil {
		return nil, err
	}
	if raw != "" {
		var s saved
		// A login of another client (GOOGLE_CLIENT_ID changed) is useless: log in again.
		if json.Unmarshal([]byte(raw), &s) == nil && s.Refresh != "" && s.ClientID == id {
			c.login = &s
		}
	}
	return c, nil
}

// NewQuiet is a client that never logs in or prints, for callers with nobody to ask (the window's
// widgets and header): without a valid login its requests fail with ErrNoLogin.
func NewQuiet() (*Client, error) {
	c, err := New()
	if err != nil {
		return nil, err
	}
	c.quiet = true
	return c, nil
}

// HasLogin reports whether a login is saved (it may have expired or been revoked since).
func (c *Client) HasLogin() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.login != nil
}

// Email is the saved login's account, "" without one.
func (c *Client) Email() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.login == nil {
		return ""
	}
	return c.login.Email
}

// Granted reports whether the saved login grants scope (consent lets the user untick scopes).
func (c *Client) Granted(scope string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.login != nil && slices.Contains(strings.Fields(c.login.Scope), scope)
}

// Login logs in in the browser even when a login is saved, replacing it.
func (c *Client) Login() error {
	if err := ui.AssertInteractive("Google login"); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked()
}

func (c *Client) loginLocked() error {
	s, access, expiry, err := c.browserLogin()
	if err != nil {
		return err
	}
	if err := c.save(s); err != nil {
		return err
	}
	c.access, c.expiry = access, expiry
	fmt.Fprintln(os.Stderr, ui.Err.Green(fmt.Sprintf("Google login ok (%s), saved to %s.", s.Email, settings.Credentials.Name())))
	if !slices.Contains(strings.Fields(s.Scope), CalendarScope) {
		ui.Warn("Calendar access was not granted (it was unticked on Google's page); log in again and allow it.")
	}
	return nil
}

func (c *Client) save(s *saved) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := settings.Credentials.Set(secrets.GoogleLogin, string(b)); err != nil {
		return err
	}
	c.login = s
	return nil
}

// Logout revokes the login at Google, then deletes it locally. expired is true when Google no
// longer knew it (expired or revoked already).
func (c *Client) Logout() (expired bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.login == nil {
		return false, ErrNoLogin
	}
	res, err := httpx.Client.PostForm(revokeURL, url.Values{"token": {c.login.Refresh}})
	if err != nil {
		return false, fmt.Errorf("Google revoke: %w", err)
	}
	if res.StatusCode == 400 {
		httpx.Discard(res)
		expired = true
	} else if _, err := httpx.ReadJSON[any]("Google POST /revoke", res, []int{200}, nil); err != nil {
		return false, err
	}
	return expired, c.forgetLocked()
}

// Forget deletes the saved login without revoking it at Google.
func (c *Client) Forget() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.forgetLocked()
}

func (c *Client) forgetLocked() error {
	c.login, c.access = nil, ""
	return settings.Credentials.Delete(secrets.GoogleLogin)
}

// Check returns the saved login's email when Google still accepts it, "" when there is none or
// Google rejects it. Never logs in or prints (safe for the window header).
func (c *Client) Check(timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.login == nil {
		return "", nil
	}
	if _, err := c.refreshLocked(ctx); err != nil {
		if errors.Is(err, errRejected) {
			return "", nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", errors.New("not reachable (timed out)")
		}
		return "", err
	}
	return c.login.Email, nil
}

// token gives a valid access token: the one in memory, else refreshed, else (not quiet) a new login.
func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.access != "" && time.Until(c.expiry) > time.Minute {
		return c.access, nil
	}
	if c.login != nil {
		access, err := c.refreshLocked(ctx)
		if !errors.Is(err, errRejected) {
			return access, err
		}
	}
	if c.quiet {
		return "", ErrNoLogin
	}
	if !ui.IsInteractive() {
		return "", fmt.Errorf("%w (run account --login google)", ErrNoLogin)
	}
	if err := c.loginLocked(); err != nil {
		return "", err
	}
	return c.access, nil
}

// errRejected means Google no longer accepts the refresh token (revoked, expired: 7 days while
// the app is in Testing, or unused for 6 months).
var errRejected = errors.New("Google login expired or revoked")

// refreshLocked gets a new access token with the refresh token. On errRejected the login is
// deleted, since it can never work again.
func (c *Client) refreshLocked(ctx context.Context) (string, error) {
	t, err := c.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.login.Refresh}})
	if errors.Is(err, errRejected) {
		if !c.quiet {
			fmt.Fprintln(os.Stderr, ui.Err.Yellow("Google login expired or was revoked."))
		}
		c.forgetLocked()
	}
	if err != nil {
		return "", err
	}
	c.access, c.expiry = t.AccessToken, time.Now().Add(time.Duration(t.ExpiresIn)*time.Second)
	return c.access, nil
}

// tokenReply is the token endpoint's answer, success or error.
type tokenReply struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"` // only on login
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// exchange posts to the token endpoint with the client's credentials plus form.
func (c *Client) exchange(ctx context.Context, form url.Values) (tokenReply, error) {
	const label = "Google POST /token"
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.secret)
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenReply{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := httpx.Client.Do(req)
	if err != nil {
		return tokenReply{}, fmt.Errorf("%s: %w", label, err)
	}
	if res.StatusCode == 400 || res.StatusCode == 401 {
		t, err := httpx.ReadJSON[tokenReply](label, res, []int{res.StatusCode}, nil)
		switch {
		case err != nil:
			return t, err
		case t.Error == "invalid_grant":
			return t, errRejected
		case t.Error == "invalid_client" || t.Error == "unauthorized_client":
			return t, fmt.Errorf("Google rejects the OAuth client (GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET): %s", t.Description)
		}
		return t, fmt.Errorf("%s: %s: %s", label, t.Error, t.Description)
	}
	return httpx.ReadJSON(label, res, []int{200}, func(t tokenReply) bool { return t.AccessToken != "" && t.ExpiresIn > 0 })
}

// get calls a Google API with the access token, logging in again once when it is rejected.
func get[T any](c *Client, u string, query url.Values) (T, error) {
	var zero T
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	label := "Google GET " + strings.TrimPrefix(u, "https://")
	for attempt := 0; ; attempt++ {
		access, err := c.token(context.Background())
		if err != nil {
			return zero, err
		}
		req, err := httpx.NewRequest("GET", u, nil)
		if err != nil {
			return zero, err
		}
		req.Header.Set("Authorization", "Bearer "+access)
		res, err := httpx.Client.Do(req)
		if err != nil {
			return zero, fmt.Errorf("%s: %w", label, err)
		}
		if res.StatusCode == 401 && attempt == 0 {
			httpx.Discard(res)
			c.mu.Lock()
			c.access = ""
			c.mu.Unlock()
			continue
		}
		return httpx.ReadJSON[T](label, res, []int{200}, nil)
	}
}

// Calendar is one calendar in the user's calendar list.
type Calendar struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Override    string `json:"summaryOverride"` // the name the user gave it, for a calendar shared with them
	Description string `json:"description"`
	TimeZone    string `json:"timeZone"`
	Color       string `json:"backgroundColor"`
	AccessRole  string `json:"accessRole"` // freeBusyReader, reader, writer, owner
	Primary     bool   `json:"primary"`
	Hidden      bool   `json:"hidden"`
	Selected    bool   `json:"selected"`
}

// Name is the calendar's name as the user sees it.
func (cal Calendar) Name() string {
	if cal.Override != "" {
		return cal.Override
	}
	return cal.Summary
}

// Calendars lists the user's calendars (calendarList), primary first.
func (c *Client) Calendars() ([]Calendar, error) {
	var all []Calendar
	query := url.Values{"maxResults": {"250"}}
	for {
		page, err := get[struct {
			Items []Calendar `json:"items"`
			Next  string     `json:"nextPageToken"`
		}](c, calendarURL+"/users/me/calendarList", query)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.Next == "" {
			break
		}
		query.Set("pageToken", page.Next)
	}
	slices.SortStableFunc(all, func(a, b Calendar) int {
		switch {
		case a.Primary == b.Primary:
			return 0
		case a.Primary:
			return -1
		}
		return 1
	})
	return all, nil
}

// Event is one event of a calendar (events.list with singleEvents: recurring ones as instances).
type Event struct {
	ID           string     `json:"id"`
	Summary      string     `json:"summary"`
	Status       string     `json:"status"`       // confirmed, tentative, cancelled
	Transparency string     `json:"transparency"` // "transparent": shown as free
	Start        EventTime  `json:"start"`
	End          EventTime  `json:"end"` // exclusive: an all-day event ends on the day after
	Attendees    []Attendee `json:"attendees"`
}

// Attendee is someone invited to an event; Self is the user.
type Attendee struct {
	Self           bool   `json:"self"`
	ResponseStatus string `json:"responseStatus"` // needsAction, declined, tentative, accepted
}

// EventTime is when an event starts or ends: DateTime (RFC 3339), or Date (YYYY-MM-DD) for an
// all-day event.
type EventTime struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
}

// Declined reports whether the user declined the event.
func (e Event) Declined() bool {
	return slices.ContainsFunc(e.Attendees, func(a Attendee) bool { return a.Self && a.ResponseStatus == "declined" })
}

// Events lists the events of calendar id that overlap [from, to): ending after from and starting
// before to, in start order. Cancelled ones are left out.
func (c *Client) Events(id string, from, to time.Time) ([]Event, error) {
	var all []Event
	query := url.Values{
		"timeMin":      {from.Format(time.RFC3339)},
		"timeMax":      {to.Format(time.RFC3339)},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
		"maxResults":   {"250"},
	}
	for {
		page, err := get[struct {
			Items []Event `json:"items"`
			Next  string  `json:"nextPageToken"`
		}](c, calendarURL+"/calendars/"+url.PathEscape(id)+"/events", query)
		if err != nil {
			return nil, err
		}
		for _, e := range page.Items {
			if e.Status != "cancelled" {
				all = append(all, e)
			}
		}
		if page.Next == "" {
			return all, nil
		}
		query.Set("pageToken", page.Next)
	}
}
