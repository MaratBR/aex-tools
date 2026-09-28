package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/settings"
	"aex/internal/ui"
)

// AEXT login shown in the menu header. Checked at start and after each tool (which may log in or
// out), without ever prompting for a login.
type loginState struct {
	state   string // checking | in | out | error
	name    string
	email   string
	message string
}

var (
	loginMu sync.Mutex
	login   = loginState{state: "checking"}
)

func currentLogin() loginState {
	loginMu.Lock()
	defer loginMu.Unlock()
	return login
}

// startLoginCheck checks the AEXT login in the background; the channel closes when done.
func startLoginCheck() <-chan struct{} {
	loginMu.Lock()
	login = loginState{state: "checking"}
	loginMu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s := checkLogin()
		loginMu.Lock()
		login = s
		loginMu.Unlock()
	}()
	return done
}

func checkLogin() loginState {
	c, err := aext.New()
	if err != nil {
		return loginState{state: "error", message: err.Error()}
	}
	me, err := c.WhoAmI(5 * time.Second)
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

func infoLines() [][]segment {
	l := currentLogin()
	var who []segment
	switch l.state {
	case "checking":
		who = []segment{{"AEXT: checking login…", dim}}
	case "in":
		who = []segment{{"Logged in as ", dim}, {l.name, bold}, {" <" + l.email + ">", dim}}
	case "out":
		who = []segment{{"Not logged in to AEXT", warn}, {" (tools log in when needed)", dim}}
	default:
		who = []segment{{"Login check failed: ", dim}, {l.message, warn}}
	}
	return [][]segment{
		who,
		{{"Settings  ", dim}, {fmt.Sprintf("%g h/day · %s", settings.HoursPerDay(), dates.TZLabel()), plain}, {"  (configure to change)", dim}},
		{{"File      ", dim}, {settings.ConfigEnvFile, plain}},
		{{"Data      ", dim}, {settings.DataDir, plain}},
	}
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
func runTool(t *tool, args []string) (status string, ok bool) {
	fmt.Println(ui.Out.Bold("▶ " + strings.Join(append([]string{t.name}, args...), " ")))
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		return t.run(args)
	}()
	if err != nil {
		printError(err)
		return fmt.Sprintf("● %s failed: %v", t.name, err), false
	}
	return fmt.Sprintf("● %s finished", t.name), true
}

func printMenu() {
	out := ui.Out
	fmt.Println(out.Bold("aex tools"))
	for _, line := range infoLines() {
		fmt.Println(plainSegments(line))
	}
	for i, t := range tools {
		fmt.Printf("  %s %-14s%s\n", out.Cyan(fmt.Sprintf("%d)", i+1)), t.name, out.Dim(t.summary))
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
			line, err := ui.Ask(t.name + " args (try --help): ")
			if err != nil {
				return err
			}
			args = splitArgs(line)
		}
		status, statusOK = runTool(t, args)
		fmt.Fprint(os.Stderr, ui.Err.Dim("\nPress any key to return to the menu..."))
		loginDone = startLoginCheck()
		ui.WaitKey()
		fmt.Fprintln(os.Stderr)
	}
}
