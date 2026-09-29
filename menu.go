package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/jira"
	"aex/internal/plugin"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/ui"
)

// AEXT and Jira logins shown in the menu header. Checked at start and after each tool (which may
// log in or out, or change settings), without ever prompting for a login.
type loginState struct {
	state   string // checking | in | out | unset | error
	name    string
	email   string
	message string
}

type logins struct{ aext, jira loginState }

var (
	loginMu sync.Mutex
	login   = logins{loginState{state: "checking"}, loginState{state: "checking"}}
)

func currentLogin() logins {
	loginMu.Lock()
	defer loginMu.Unlock()
	return login
}

// startLoginCheck checks both logins in parallel in the background; the channel closes when both are done.
func startLoginCheck() <-chan struct{} {
	loginMu.Lock()
	login = logins{loginState{state: "checking"}, loginState{state: "checking"}}
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

// Header segment styles, rendered by the plain menu and the TUI each in their own way.
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
		{{"Settings  ", dim}, {fmt.Sprintf("%g h/day · %s", settings.HoursPerDay(), dates.TZLabel()), plain}, {"  (configure to change)", dim}},
		{{"File      ", dim}, {settings.ConfigEnvFile, plain}},
		credentialsLine(),
		{{"Data      ", dim}, {settings.DataDir, plain}},
	}
}

// credentialsLine says where the AEXT session and JIRA_TOKEN are kept, and why when it is the file fallback.
func credentialsLine() []segment {
	line := []segment{{"Secrets   ", dim}, {settings.Credentials.Name(), plain}}
	if settings.CredentialsNote != "" {
		line = append(line, segment{"  (" + settings.CredentialsNote + ")", warn})
	}
	return line
}

func plainSegments(segments []segment) string {
	out := ui.Out
	var b strings.Builder
	for _, s := range segments {
		switch s.kind {
		case dim:
			b.WriteString(out.Dim(s.text))
		case bold:
			b.WriteString(out.Bold(s.text))
		case warn:
			b.WriteString(out.Yellow(s.text))
		default:
			b.WriteString(s.text)
		}
	}
	return b.String()
}

var argPattern = regexp.MustCompile(`"[^"]*"|\S+`)

// splitArgs splits a line into args, keeping "quoted parts" together.
func splitArgs(line string) []string {
	args := argPattern.FindAllString(line, -1)
	for i, a := range args {
		if len(a) >= 2 && strings.HasPrefix(a, `"`) && strings.HasSuffix(a, `"`) {
			args[i] = a[1 : len(a)-1]
		}
	}
	return args
}

// runTool runs a tool, printing its errors, and returns a one-line outcome for the menu.
func runTool(t *tool.Tool, args []string) (status string, ok bool) {
	fmt.Println(toolBanner(t.Name, args))
	fmt.Println()
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		return t.Run(args)
	}()
	if err != nil {
		if _, quiet := errors.AsType[*plugin.ExitError](err); !quiet {
			printError(err)
		}
		return fmt.Sprintf("✖ %s failed: %v", t.Name, err), false
	}
	return fmt.Sprintf("✔ %s finished", t.Name), true
}

func printMenu() {
	out := ui.Out
	fmt.Println(out.Bold("aex tools"))
	for _, line := range infoLines() {
		fmt.Println(plainSegments(line))
	}
	for i, t := range tools {
		fmt.Printf("  %s %-*s%s\n", out.Cyan(fmt.Sprintf("%d)", i+1)), nameWidth(), t.Name, out.Dim(t.Summary))
	}
	fmt.Printf("  %s quit\n", out.Cyan("q)"))
	fmt.Println(out.Dim(`  Args may follow the choice, e.g. "1 --range yesterday" or "quota --help".`))
}

func plainMenu() error {
	for {
		<-startLoginCheck()
		printMenu()
		line, err := ui.Ask("> ")
		if err != nil {
			return err
		}
		args := splitArgs(line)
		if len(args) == 0 {
			continue
		}
		switch strings.ToLower(args[0]) {
		case "q", "quit", "exit":
			return nil
		}
		t := findTool(args[0])
		if t == nil {
			fmt.Printf("Unknown choice: %s\n\n", args[0])
			continue
		}
		fmt.Println()
		runTool(t, args[1:])
		fmt.Println()
		loadPlugins()
	}
}

func useTUI() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("AEX_TUI") != "0"
}

// tuiMenu shows the full-screen menu until quit. Tools run in the normal screen, so their output
// stays in the scrollback.
func tuiMenu() error {
	index := 0
	var status string
	var statusOK bool
	loginDone := startLoginCheck()
	for {
		pick, err := selectTool(index, status, statusOK, loginDone)
		if err != nil || pick.quit {
			return err
		}
		index = pick.index
		t := &tools[index]

		var args []string
		if pick.withArgs {
			line, err := ui.Input(ui.Field{
				Title:       t.Name + " arguments",
				Description: "Space-separated; quote values with spaces. --help lists them.",
				Placeholder: "--help",
			})
			if err != nil {
				return err
			}
			args = splitArgs(line)
		}
		status, statusOK = runTool(t, args)
		fmt.Fprintf(os.Stderr, "\n%s  %s", statusLine(status, statusOK), ui.Err.Dim("press any key to return to the menu"))
		loginDone = startLoginCheck()
		loadPlugins()
		index = min(index, len(tools)-1)
		ui.WaitKey()
		fmt.Fprintln(os.Stderr)
	}
}
