package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/google"
	"aex/internal/jira"
	"aex/internal/plugin"
	"aex/internal/settings"
	"aex/internal/tool"
)

// AEXT, Jira and Google logins shown in the window's header. Checked at start and after each tool (which
// may log in or out, or change settings), without ever prompting for a login.
type loginState struct {
	state   string // checking | in | out | unset | error
	name    string
	email   string
	message string
}

type logins struct{ aext, jira, google loginState }

var checking = logins{loginState{state: "checking"}, loginState{state: "checking"}, loginState{state: "checking"}}

var (
	loginMu sync.Mutex
	login   = checking
)

func currentLogin() logins {
	loginMu.Lock()
	defer loginMu.Unlock()
	return login
}

// startLoginCheck checks the logins in parallel in the background; the channel closes when all are done.
func startLoginCheck() <-chan struct{} {
	loginMu.Lock()
	login = checking
	loginMu.Unlock()
	var wg sync.WaitGroup
	check := func(field *loginState, fn func() loginState) {
		wg.Go(func() {
			s := fn()
			loginMu.Lock()
			*field = s
			loginMu.Unlock()
		})
	}
	check(&login.aext, checkAEXT)
	check(&login.jira, checkJira)
	check(&login.google, checkGoogle)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	return done
}

const loginTimeout = 5 * time.Second

func checkAEXT() loginState {
	c, err := aext.New()
	if err != nil {
		return loginState{state: "error", message: err.Error()}
	}
	me, err := c.WhoAmI(loginTimeout)
	if err != nil {
		return loginState{state: "error", message: err.Error()}
	}
	if me == nil {
		return loginState{state: "out"}
	}
	name := me.DisplayName
	if name == "" {
		name = me.UserName
	}
	return loginState{state: "in", name: name, email: *me.Email}
}

func checkJira() loginState {
	me, err := jira.WhoAmI(loginTimeout)
	switch {
	case errors.Is(err, jira.ErrNotConfigured):
		return loginState{state: "unset"}
	case err != nil:
		return loginState{state: "error", message: err.Error()}
	case me == nil:
		return loginState{state: "out"}
	}
	return loginState{state: "in", name: me.DisplayName, email: me.EmailAddress}
}

func checkGoogle() loginState {
	c, err := google.NewQuiet()
	if errors.Is(err, google.ErrNotConfigured) {
		return loginState{state: "unset"}
	}
	if err != nil {
		return loginState{state: "error", message: err.Error()}
	}
	email, err := c.Check(loginTimeout)
	switch {
	case err != nil:
		return loginState{state: "error", message: err.Error()}
	case email == "":
		return loginState{state: "out"}
	}
	return loginState{state: "in", name: email}
}

// Header segment styles.
type segKind int

const (
	plain segKind = iota
	dim
	bold
	warn
)

type segment struct {
	text string
	kind segKind
}

// loginLine describes one service's login; out and unset are that service's hints.
func loginLine(label string, l loginState, out, unset string) []segment {
	head := segment{fmt.Sprintf("%-10s", label), dim}
	switch l.state {
	case "checking":
		return []segment{head, {"checking login…", dim}}
	case "in":
		who := []segment{head, {"Logged in as ", dim}, {l.name, bold}}
		if l.email != "" {
			who = append(who, segment{" <" + l.email + ">", dim})
		}
		return who
	case "out":
		return []segment{head, {"Not logged in", warn}, {" (" + out + ")", dim}}
	case "unset":
		return []segment{head, {"Login not set", warn}, {" (" + unset + ")", dim}}
	default:
		return []segment{head, {"Login check failed: ", dim}, {l.message, warn}}
	}
}

func infoLines() [][]segment {
	l := currentLogin()
	return [][]segment{
		loginLine("AEXT", l.aext, "tools log in when needed, or run account", ""),
		loginLine("Jira", l.jira, "token rejected, run account", "run account"),
		loginLine("Google", l.google, "run account --login google", "GOOGLE_CLIENT_ID not set"),
		{{"Settings  ", dim}, {fmt.Sprintf("%g h/day · %s", settings.HoursPerDay(), dates.TZLabel()), plain}},
		{{"File      ", dim}, {settings.ConfigEnvFile, plain}},
		credentialsLine(),
		{{"Data      ", dim}, {settings.DataDir, plain}},
	}
}

// credentialsLine says where the AEXT session, JIRA_TOKEN and the Google login are kept, and why when it is the file fallback.
func credentialsLine() []segment {
	line := []segment{{"Secrets   ", dim}, {settings.Credentials.Name(), plain}}
	if settings.CredentialsNote != "" {
		line = append(line, segment{"  (" + settings.CredentialsNote + ")", warn})
	}
	return line
}

// execTool runs a tool, printing its errors, and returns a one-line outcome.
func execTool(name string, t *tool.Tool, args []string) (status string, ok bool) {
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		return t.Exec(args)
	}()
	if err != nil {
		if _, quiet := errors.AsType[*plugin.ExitError](err); !quiet {
			printError(err)
		}
		return fmt.Sprintf("✖ %s failed: %v", name, err), false
	}
	return fmt.Sprintf("✔ %s finished", name), true
}
