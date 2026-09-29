package google

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"aex/internal/secrets"
	"aex/internal/settings"
	"aex/internal/tool"
)

// fakeGoogle is Google's token and Calendar endpoints: login code "code1" gives refresh token
// "r1"; refreshing r1 gives access token "a2"; Calendar takes only a2.
func fakeGoogle(t *testing.T, challenge *string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("client_id") != "id" || r.Form.Get("client_secret") != "secret" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"invalid_client","error_description":"bad client"}`)
			return
		}
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "code1" || base64.RawURLEncoding.EncodeToString(sum[:]) != *challenge {
				w.WriteHeader(400)
				fmt.Fprint(w, `{"error":"invalid_grant"}`)
				return
			}
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"me@example.com"}`))
			json.NewEncoder(w).Encode(map[string]any{"access_token": "a1", "expires_in": 3600, "refresh_token": "r1",
				"scope": "openid " + CalendarScope, "id_token": "h." + claims + ".s"})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "r1" {
				w.WriteHeader(400)
				fmt.Fprint(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
				return
			}
			fmt.Fprint(w, `{"access_token":"a2","expires_in":3600}`)
		}
	})
	mux.HandleFunc("GET /calendar/v3/users/me/calendarList", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer a2" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"items":[{"id":"x","summary":"Team"}],"nextPageToken":"p2"}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"id":"me@example.com","summary":"me@example.com","summaryOverride":"Me","primary":true}]}`)
	})
	// Calendar "x" (its id escaped in the path): one event per page, the second cancelled.
	mux.HandleFunc("GET /calendar/v3/calendars/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("Authorization") != "Bearer a2" || r.PathValue("id") != "team@x" || q.Get("singleEvents") != "true" ||
			q.Get("timeMin") != "2026-09-29T10:00:00Z" || q.Get("timeMax") != "2026-09-29T10:00:01Z" {
			w.WriteHeader(400)
			return
		}
		if q.Get("pageToken") == "" {
			fmt.Fprint(w, `{"items":[{"summary":"Standup","status":"confirmed","start":{"dateTime":"2026-09-29T09:45:00Z"},
				"end":{"dateTime":"2026-09-29T10:15:00Z"},"attendees":[{"self":true,"responseStatus":"declined"}]}],"nextPageToken":"p2"}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"summary":"Gone","status":"cancelled"},{"summary":"Off","start":{"date":"2026-09-29"},"end":{"date":"2026-09-30"}}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	tokenURL, calendarURL = srv.URL+"/token", srv.URL+"/calendar/v3"
	settings.Credentials = secrets.Memory("test", nil)
}

func TestLoginAndCalendars(t *testing.T) {
	var challenge string
	fakeGoogle(t, &challenge)
	openBrowser = func(page string) error {
		u, _ := url.Parse(page)
		q := u.Query()
		challenge = q.Get("code_challenge")
		if q.Get("client_id") != "id" || q.Get("code_challenge_method") != "S256" || q.Get("access_type") != "offline" ||
			!strings.Contains(q.Get("scope"), CalendarScope) {
			t.Errorf("sign-in page: %s", page)
		}
		back := q.Get("redirect_uri")
		if !strings.HasPrefix(back, "http://127.0.0.1:") {
			t.Errorf("redirect_uri %q", back)
		}
		// Anything without the login's state is turned away.
		if res, err := http.Get(back + "?code=evil&state=wrong"); err != nil || res.StatusCode != 404 {
			t.Errorf("wrong state: %v %v", res.StatusCode, err)
		}
		go http.Get(back + "?" + url.Values{"code": {"code1"}, "state": {q.Get("state")}}.Encode())
		return nil
	}
	defer func(open func(string) error) { openBrowser = open }(tool.OpenURL)

	c, err := newClient("id", "secret")
	if err != nil {
		t.Fatal(err)
	}
	c.quiet = true // a quiet client never logs in on its own
	if _, err := c.Calendars(); !errors.Is(err, ErrNoLogin) {
		t.Fatalf("quiet client without a login: %v", err)
	}
	c.quiet = false
	c.mu.Lock()
	err = c.loginLocked()
	c.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if c.Email() != "me@example.com" || !c.Granted(CalendarScope) {
		t.Errorf("login: email %q, calendar granted %v", c.Email(), c.Granted(CalendarScope))
	}

	// Calendar rejects a1: the client refreshes once and gets a2.
	cals, err := c.Calendars()
	if err != nil {
		t.Fatal(err)
	}
	if len(cals) != 2 || cals[0].Name() != "Me" || !cals[0].Primary || cals[1].Name() != "Team" {
		t.Errorf("calendars: %+v", cals)
	}

	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	events, err := c.Events("team@x", now, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Summary != "Standup" || !events[0].Declined() || events[1].Start.Date != "2026-09-29" || events[1].Declined() {
		t.Errorf("events: %+v", events)
	}

	// A new client (another run) finds the saved login.
	again, err := newClient("id", "secret")
	if err != nil || again.Email() != "me@example.com" {
		t.Fatalf("saved login: %q %v", again.Email(), err)
	}
	if email, err := again.Check(timeout); email != "me@example.com" || err != nil {
		t.Errorf("Check: %q %v", email, err)
	}
	// Another OAuth client cannot use it.
	if other, _ := newClient("other", "secret"); other.HasLogin() {
		t.Error("login of another client used")
	}
}

const timeout = 5 * time.Second

func TestRejectedLoginIsForgotten(t *testing.T) {
	var challenge string
	fakeGoogle(t, &challenge)
	settings.Credentials.Set(secrets.GoogleLogin, `{"client_id":"id","refresh_token":"revoked"}`)
	c, err := quietClient()
	if err != nil {
		t.Fatal(err)
	}
	if email, err := c.Check(timeout); email != "" || err != nil {
		t.Errorf("Check of a revoked login: %q %v", email, err)
	}
	if v, _ := settings.Credentials.Get(secrets.GoogleLogin); v != "" || c.HasLogin() {
		t.Errorf("revoked login kept: %q", v)
	}
	if _, err := c.Calendars(); !errors.Is(err, ErrNoLogin) {
		t.Errorf("Calendars: %v", err)
	}
}

// quietClient is NewQuiet with the test's OAuth client instead of the settings'.
func quietClient() (*Client, error) {
	c, err := newClient("id", "secret")
	if err == nil {
		c.quiet = true
	}
	return c, err
}

func TestBadClient(t *testing.T) {
	var challenge string
	fakeGoogle(t, &challenge)
	settings.Credentials.Set(secrets.GoogleLogin, `{"client_id":"id","refresh_token":"r1"}`)
	c, _ := newClient("id", "wrong")
	if _, err := c.Check(timeout); err == nil || !strings.Contains(err.Error(), "GOOGLE_CLIENT_SECRET") {
		t.Errorf("Check with a wrong secret: %v", err)
	}
	if !c.HasLogin() {
		t.Error("login dropped for a client problem")
	}
}

func TestIDTokenEmail(t *testing.T) {
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"a@b.c","sub":"1"}`))
	for token, want := range map[string]string{"h." + claims + ".s": "a@b.c", "": "", "a.b": "", "h.!!.s": ""} {
		if got := idTokenEmail(token); got != want {
			t.Errorf("idTokenEmail(%q) = %q, want %q", token, got, want)
		}
	}
}
