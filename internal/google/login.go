package google

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"aex/internal/tool"
	"aex/internal/ui"
)

// How long a login waits for the browser to come back.
const loginWait = 5 * time.Minute

// openBrowser opens the sign-in page; the tests replace it with a fake browser.
var openBrowser = tool.OpenURL

// browserLogin signs in the way Google has installed apps do it: the sign-in page opens in the
// browser, which Google then sends to http://127.0.0.1:<port>/ with a code, taken by a server here
// that lives only for the login. PKCE ties the code to this login; state ties the answer to it.
func (c *Client) browserLogin() (*saved, string, time.Time, error) {
	var zero time.Time
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", zero, fmt.Errorf("Google login: local address for the browser to come back to: %w", err)
	}
	redirect := fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	verifier, state := randomString(), randomString()
	challenge := sha256.Sum256([]byte(verifier))
	page := authURL + "?" + url.Values{
		"client_id":             {c.clientID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"scope":                 {strings.Join(scopes, " ")},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"access_type":           {"offline"}, // a refresh token
		"prompt":                {"consent"}, // a refresh token even when access was granted before
	}.Encode()

	answers := make(chan url.Values, 1)
	srv := &http.Server{Handler: callback(state, answers), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Fprintln(os.Stderr, "Sign in to Google in your browser. If it did not open, open this address:")
	fmt.Fprintln(os.Stderr, ui.Err.Cyan(page))
	if err := openBrowser(page); err != nil {
		ui.Warn("could not open the browser: %v", err)
	}
	fmt.Fprintln(os.Stderr, ui.Err.Dim(fmt.Sprintf("Waiting for Google (up to %d minutes)…", int(loginWait.Minutes()))))

	var answer url.Values
	select {
	case answer = <-answers:
	case <-time.After(loginWait):
		return nil, "", zero, errors.New("Google login: no answer from the browser in time")
	}
	if e := answer.Get("error"); e != "" {
		if e == "access_denied" {
			return nil, "", zero, errors.New("Google login cancelled (access denied)")
		}
		return nil, "", zero, fmt.Errorf("Google login failed: %s", e)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	t, err := c.exchange(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {answer.Get("code")},
		"code_verifier": {verifier},
		"redirect_uri":  {redirect},
	})
	if err != nil {
		return nil, "", zero, err
	}
	if t.RefreshToken == "" {
		return nil, "", zero, errors.New("Google login: no refresh token in Google's answer")
	}
	s := &saved{ClientID: c.clientID, Refresh: t.RefreshToken, Email: idTokenEmail(t.IDToken), Scope: t.Scope}
	return s, t.AccessToken, time.Now().Add(time.Duration(t.ExpiresIn) * time.Second), nil
}

// callback takes Google's answer (code or error) for state, once, and tells the browser how it went.
// Requests without the right state (anything else reaching the port) are turned away.
func callback(state string, answers chan<- url.Values) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/" || q.Get("state") != state {
			http.NotFound(w, r)
			return
		}
		msg := "Signed in to Google. You can close this tab and go back to aex."
		if e := q.Get("error"); e != "" {
			msg = "Google sign-in did not complete (" + e + "). You can close this tab."
		} else if q.Get("code") == "" {
			http.Error(w, "no code", http.StatusBadRequest)
			return
		}
		select {
		case answers <- q:
		default: // answered already
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>aex</title>`+
			`<body style="font:16px system-ui,sans-serif;margin:3em">%s</body>`, html.EscapeString(msg))
	})
}

// randomString is 32 random bytes, base64url: a PKCE verifier (43 characters) or a state.
func randomString() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// idTokenEmail reads the email claim of an ID token. Not verified: it comes straight from Google's
// token endpoint over TLS, and only names the account for display.
func idTokenEmail(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	json.Unmarshal(payload, &claims)
	return claims.Email
}
