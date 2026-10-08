package composer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"aex/internal/adapter/autohotkey"
	"aex/internal/settings"
	"aex/internal/tool"
)

// fakeTools sets the tool list to tools that record their runs in ran; "fail" fails, "wait-a" and
// "wait-b" each wait for the other to start (so they pass only when run together).
func fakeTools(t *testing.T) *[]string {
	t.Helper()
	var mu sync.Mutex
	ran := &[]string{}
	record := func(name string) {
		mu.Lock()
		*ran = append(*ran, name)
		mu.Unlock()
	}
	a, b := make(chan struct{}), make(chan struct{})
	waitFor := func(mine, other chan struct{}) error {
		close(mine)
		select {
		case <-other:
			return nil
		case <-time.After(2 * time.Second):
			return errors.New("ran alone")
		}
	}
	list := []tool.Tool{
		{Name: "ok", Run: func(args []string) error { record("ok " + strings.Join(args, "|")); return nil }},
		{Name: "fail", Run: func([]string) error { record("fail"); return errors.New("boom") }},
		{Name: "wait-a", Run: func([]string) error { record("wait-a"); return waitFor(a, b) }},
		{Name: "wait-b", Run: func([]string) error { record("wait-b"); return waitFor(b, a) }},
		{Name: "grp", Sub: []tool.Tool{{Name: "inner", Run: func([]string) error { record("inner"); return nil }}}},
	}
	Tools = func() []tool.Tool { return list }
	t.Cleanup(func() { Tools = func() []tool.Tool { return nil } })
	return ran
}

func toolStep(name string) Step { return Step{Action: &ToolAction{Tool: name}} }

func TestStepJSON(t *testing.T) {
	timeout := 0
	c := Composed{Name: "x", Steps: []Step{
		{Label: "build", Critical: true, Action: &RunAction{Path: "go", Args: "build ./..."}},
		{Parallel: true, Action: &CheckAction{Do: Steps{{Action: &IPCheckAction{Want: []string{"US"}}}}, Timeout: &timeout}},
		{Action: &IDEAction{IDE: "rider", Project: "app.sln"}},
	}}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"x","steps":[{"action":"run","label":"build","critical":true,"path":"go","args":"build ./..."},` +
		`{"action":"check","parallel":true,"do":[{"action":"ip-check","want":["US"]}],"timeout":0},` +
		`{"action":"open-ide","ide":"rider","project":"app.sln"}]}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
	back, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if b2, _ := json.Marshal(back); string(b2) != want {
		t.Fatalf("round trip: %s", b2)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"name":"x","steps":[{"tool":"a"}]}`, `step 1: a step needs "action"`},
		{`{"name":"x","steps":[{"action":"nope"}]}`, `step 1: no action "nope"`},
		{`{"name":"x","steps":[{"action":"tool","tool":"a","bogus":1}]}`, `step 1: tool: no field "bogus"`},
		{`{"name":"x","steps":[{"action":"tool","tool":5}]}`, `step 1: tool: "tool": expected text, not number`},
		{`{"name":"x","extra":1}`, `no field "extra"`},
		{"{\n\"name\":", `ends before it is complete`},
		{"{\n\"name\": x}", `line 2:`},
	} {
		_, err := Parse([]byte(tc.in))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.in, err, tc.want)
		}
	}
}

func useFile(t *testing.T) {
	old := settings.ComposedToolsFile
	settings.ComposedToolsFile = filepath.Join(t.TempDir(), "composed-tools.json")
	t.Cleanup(func() { settings.ComposedToolsFile = old })
}

func TestCheck(t *testing.T) {
	useFile(t)
	fakeTools(t)
	c := Composed{Name: "bad name", Steps: []Step{
		toolStep(""),
		toolStep("missing"),
		{Action: &CheckAction{Do: Steps{toolStep("ok"), {Action: &IPCheckAction{}}}}},
		{Action: &ToolAction{Tool: "composed self"}},
	}}
	ps, err := Check(c, "")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range ps {
		got = append(got, p.String())
	}
	for _, want := range []string{
		`name: name "bad name": use letters`,
		"step 1: tool: pick a tool",
		"step 2: tool: there is no tool missing now",
		"step 3.do.2: want: say where the IP should be",
	} {
		if !slices.ContainsFunc(got, func(g string) bool { return strings.HasPrefix(g, want) }) {
			t.Errorf("missing %q in:\n%s", want, strings.Join(got, "\n"))
		}
	}
	if !ps.HasErrors() {
		t.Error("HasErrors is false")
	}
	for _, line := range []string{"composed Self", "self"} {
		ps, _ = Check(Composed{Name: "self", Steps: []Step{toolStep(line)}}, "")
		if len(ps) != 1 || ps[0].String() != "step 1: tool: a composed tool cannot run itself" {
			t.Errorf("%s: %v", line, ps)
		}
	}
}

func TestPutLoadRemove(t *testing.T) {
	useFile(t)
	fakeTools(t)
	c := Composed{Name: "one", Steps: []Step{toolStep("ok")}}
	if p, err := Put("", c); err != nil || p.HasErrors() {
		t.Fatal(p, err)
	}
	if p, _ := Put("", c); !p.HasErrors() {
		t.Error("a second tool with the same name was saved")
	}
	c.Name = "two"
	if p, err := Put("one", c); err != nil || p.HasErrors() {
		t.Fatal(p, err)
	}
	entries, err := Load()
	if err != nil || len(entries) != 1 || entries[0].Name != "two" {
		t.Fatalf("%+v %v", entries, err)
	}
	if err := Remove("TWO"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := Load(); len(entries) != 0 {
		t.Fatalf("%+v", entries)
	}
}

func TestExecute(t *testing.T) {
	ran := fakeTools(t)
	c := Composed{Name: "c", Steps: []Step{
		toolStep("fail"),
		{Action: &ToolAction{Tool: "ok", Args: `a "b c"`}},
		toolStep("inner"),
	}}
	err := Execute(context.Background(), c)
	if err == nil || err.Error() != "1 of 3 steps failed" {
		t.Errorf("err = %v", err)
	}
	if want := []string{"fail", "ok a|b c", "inner"}; !slices.Equal(*ran, want) {
		t.Errorf("ran %q, want %q", *ran, want)
	}
}

func TestCritical(t *testing.T) {
	ran := fakeTools(t)
	c := Composed{Name: "c", Steps: []Step{toolStep("ok"), {Critical: true, Action: &ToolAction{Tool: "fail"}}, toolStep("ok")}}
	err := Execute(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "step 2") {
		t.Errorf("err = %v", err)
	}
	if want := []string{"ok ", "fail"}; !slices.Equal(*ran, want) {
		t.Errorf("ran %q, want %q", *ran, want)
	}
}

func TestParallel(t *testing.T) {
	fakeTools(t)
	c := Composed{Name: "c", Steps: []Step{
		{Parallel: true, Action: &ToolAction{Tool: "wait-a"}},
		{Parallel: true, Action: &ToolAction{Tool: "wait-b"}},
	}}
	if err := Execute(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRetries(t *testing.T) {
	old := second
	second = time.Millisecond
	t.Cleanup(func() { second = old })
	tries := 0
	list := []tool.Tool{{Name: "flaky", Run: func([]string) error {
		tries++
		if tries < 3 {
			return fmt.Errorf("try %d", tries)
		}
		return nil
	}}}
	Tools = func() []tool.Tool { return list }
	t.Cleanup(func() { Tools = func() []tool.Tool { return nil } })
	check := &CheckAction{Do: Steps{{Action: &ToolAction{Tool: "flaky"}}}, Every: 1}
	if err := check.Run(&Run{Ctx: context.Background()}); err != nil || tries != 3 {
		t.Fatalf("err %v after %d tries", err, tries)
	}
	tries = -10
	check.Attempts = 2
	if err := check.Run(&Run{Ctx: context.Background()}); err == nil || !strings.Contains(err.Error(), "after 2 tries") {
		t.Fatalf("err %v", err)
	}
}

func TestRunsItself(t *testing.T) {
	useFile(t)
	ran := fakeTools(t)
	if _, err := Put("", Composed{Name: "a", Steps: []Step{toolStep("ok")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Put("", Composed{Name: "b", Steps: []Step{toolStep("composed a")}}); err != nil {
		t.Fatal(err)
	}
	// a now runs b, which runs a.
	if _, err := Put("a", Composed{Name: "a", Steps: []Step{{Critical: true, Action: &ToolAction{Tool: "composed b"}}}}); err != nil {
		t.Fatal(err)
	}
	groups := Discover(Tools())
	list := append(slices.Clip(Tools()), groups...)
	Tools = func() []tool.Tool { return list }
	entries, _ := Load()
	err := Execute(context.Background(), Find(entries, "a").Composed)
	if err == nil {
		t.Fatal("no error")
	}
	if len(*ran) != 0 {
		t.Errorf("ran %q", *ran)
	}
}

func TestIPCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ip":"1.2.3.4","city":"Frankfurt am Main","region":"Hesse","country":"DE","org":"AS1 Example"}`)
	}))
	defer srv.Close()
	old := geoServices
	geoServices = []geoService{{url: "http://127.0.0.1:1/down", read: old[0].read}, {url: srv.URL, read: old[0].read}}
	t.Cleanup(func() { geoServices = old })
	r := &Run{Ctx: context.Background()}
	for _, tc := range []struct {
		a  IPCheckAction
		ok bool
	}{
		{IPCheckAction{Want: []string{"de"}}, true},
		{IPCheckAction{Want: []string{"US", "hesse"}}, true},
		{IPCheckAction{Want: []string{"US"}}, false},
		{IPCheckAction{Avoid: []string{"RU"}}, true},
		{IPCheckAction{Avoid: []string{"Frankfurt am Main"}}, false},
	} {
		if err := tc.a.Run(r); (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc.a, err)
		}
	}
}

func TestSplitArgs(t *testing.T) {
	got := SplitArgs(` a  "b c" --name="d e" ""`)
	want := []string{"a", "b c", "--name=d e", ""}
	if !slices.Equal(got, want) {
		t.Errorf("%q", got)
	}
}

func TestOneStepDo(t *testing.T) {
	c, err := Parse([]byte(`{"name":"x","steps":[{"action":"check","do":{"action":"tool","tool":"ok"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if do := c.Steps[0].Action.(*CheckAction).Do; len(do) != 1 || do[0].Action.(*ToolAction).Tool != "ok" {
		t.Fatalf("%+v", do)
	}
}

// TestIfReturn: try until it works: if the condition works, run a step and return failed, else
// return worked. The condition fails on the third try.
func TestIfReturn(t *testing.T) {
	old := second
	second = time.Millisecond
	t.Cleanup(func() { second = old })
	ran := fakeTools(t)
	tries := 0
	list := append(slices.Clip(Tools()), tool.Tool{Name: "vpn-off", Run: func([]string) error {
		tries++
		if tries < 3 {
			return nil
		}
		return errors.New("vpn on")
	}})
	Tools = func() []tool.Tool { return list }
	c, err := Parse([]byte(`{"name":"x","steps":[
		{"action":"check","every":1,"do":[
			{"action":"if","cond":{"action":"tool","tool":"vpn-off"},
			 "then":[{"action":"tool","tool":"ok","args":"launch"},{"action":"return","fail":true,"message":"enable the VPN"},{"action":"tool","tool":"fail"}],
			 "else":[{"action":"return"}]}
		]},
		{"action":"tool","tool":"ok","args":"after"},
		{"action":"return","message":"all good"},
		{"action":"tool","tool":"fail"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := Execute(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if want := []string{"ok launch", "ok launch", "ok after"}; !slices.Equal(*ran, want) || tries != 3 {
		t.Errorf("ran %q after %d tries", *ran, tries)
	}
	// A return failed at the top fails the tool.
	c.Steps = []Step{{Action: &ReturnAction{Fail: true, Message: "no"}}, toolStep("ok")}
	if err := Execute(context.Background(), c); err == nil || err.Error() != "returned failed: no" {
		t.Errorf("err = %v", err)
	}
	// Not swaps the branches; a failing branch fails the if.
	ifNot := &IfAction{Cond: &Step{Action: &ToolAction{Tool: "ok"}}, Not: true, Else: Steps{toolStep("fail")}}
	if err := ifNot.Run(&Run{Ctx: context.Background()}); err == nil {
		t.Error("no error from the else branch")
	}
}

func TestOrientScript(t *testing.T) {
	if got := ahkString("a\"b`c\nd"); got != "\"a`\"b``c`nd\"" {
		t.Errorf("ahkString: %s", got)
	}
	x := 10.0
	s := orientScript([]WindowRule{
		{Exe: "chrome.exe", Title: `Say "hi"`, Monitor: 2, Place: "left"},
		{Exe: "code.exe", Place: "custom", X: &x, All: true},
	}, 5*time.Second)
	for _, want := range []string{
		"deadline := A_TickCount + 5000",
		"{exe: \"chrome.exe\", title: \"Say `\"hi`\"\", mon: 2, place: \"left\", x: 0.0000, y: 0.0000, w: 50.0000, h: 100.0000, all: false}",
		"{exe: \"code.exe\", title: \"\", mon: 0, place: \"custom\", x: 10.0000, y: 0.0000, w: 100.0000, h: 100.0000, all: true}",
		"isVisible(hwnd) {",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q:\n%s", want, s)
		}
	}
}

func TestOrientCheck(t *testing.T) {
	w := 50.0
	ps := check(Composed{Name: "x", Steps: []Step{{Action: &OrientAction{Windows: []WindowRule{
		{Exe: "", Place: "left"}, {Exe: "a.exe", Place: "nowhere"}, {Exe: "b.exe", Place: "custom", X: &w, W: &w, Y: &w}, {Exe: "c.exe", Place: "custom", X: &w, W: &w, H: &w},
	}}}}}, "", nil)
	var errs []string
	for _, p := range ps {
		if !p.Warning {
			errs = append(errs, p.Message)
		}
	}
	want := []string{"window 1: pick its program", `window 2: no place "nowhere"`, "window 3: a custom place is within the screen, in percent"}
	if !slices.Equal(errs, want) {
		t.Errorf("got %q", errs)
	}
}

func TestParseWindows(t *testing.T) {
	sc, wins, err := parseWindows("M\t1\t1\t0\t0\t1920\t1040\r\nW\tcode.exe\tC:/c.exe\tChrome_WidgetWin_1\t1\t-8\t-8\t1936\t1056\ta\tb\n")
	if err != nil || len(sc) != 1 || !sc[0].Primary || sc[0].Bottom != 1040 || len(wins) != 1 || wins[0].Title != "a b" || wins[0].State != 1 {
		t.Fatalf("%+v %+v %v", sc, wins, err)
	}
	if _, _, err := parseWindows(""); err == nil {
		t.Error("no error without screens")
	}
}

// TestListWindows lists the windows open now with AutoHotkey, when it is installed (Windows).
func TestListWindows(t *testing.T) {
	if runtime.GOOS != "windows" || autohotkey.Exe() == "" {
		t.Skip("needs AutoHotkey v2 on Windows")
	}
	sc, _, err := ListWindows(context.Background())
	if err != nil || len(sc) == 0 {
		t.Fatalf("%v %v", sc, err)
	}
}

type fakeWindow struct{ home, min int }

func (f *fakeWindow) ShowHome() error { f.home++; return nil }
func (f *fakeWindow) Minimise() error { f.min++; return nil }

func TestWindowActions(t *testing.T) {
	r := &Run{Ctx: context.Background()}
	if err := (&HomeAction{}).Run(r); err == nil {
		t.Error("home ran without a window")
	}
	f := &fakeWindow{}
	Window = f
	t.Cleanup(func() { Window = nil })
	(&HomeAction{}).Run(r)
	(&MinimizeAction{}).Run(r)
	if f.home != 1 || f.min != 1 {
		t.Errorf("%+v", f)
	}
	old := second
	second = time.Millisecond
	t.Cleanup(func() { second = old })
	start := time.Now()
	if err := (&DelayAction{Seconds: 20}).Run(r); err != nil || time.Since(start) < 20*time.Millisecond {
		t.Errorf("delay: %v after %s", err, time.Since(start))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&DelayAction{Seconds: 20}).Run(&Run{Ctx: ctx}); err == nil {
		t.Error("delay not stopped")
	}
	if s := seconds(120); s != "2 min" {
		t.Error(s)
	}
}

func TestSuggestions(t *testing.T) {
	s := Suggestions()
	if len(s["delay"]) == 0 {
		t.Fatal("no delay suggestions")
	}
	for kind, list := range s {
		for _, sug := range list {
			var step Step
			if err := json.Unmarshal(sug.Step, &step); err != nil || step.Action.Kind() != kind {
				t.Errorf("%s: %s: %v", kind, sug.Step, err)
			}
		}
	}
}

func TestInterrupt(t *testing.T) {
	old := second
	second = time.Millisecond
	t.Cleanup(func() { second = old })
	var states []bool
	OnInterruptible = func(on bool) { states = append(states, on) }
	t.Cleanup(func() { OnInterruptible = nil })
	if Interrupt() {
		t.Error("interrupted with nothing running")
	}
	c := Composed{Name: "c", Steps: []Step{
		{Action: &DelayAction{Seconds: 10000}},
		{Action: &DelayAction{Seconds: 10000}},
	}}
	done := make(chan error)
	go func() { done <- interruptible(c) }()
	for !Interrupt() {
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrInterrupted) {
			t.Errorf("err %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("not interrupted")
	}
	if !slices.Equal(states, []bool{true, false}) || Interrupt() {
		t.Errorf("states %v", states)
	}
}
