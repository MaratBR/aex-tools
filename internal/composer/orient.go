package composer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"aex/internal/adapter/autohotkey"
)

// OrientAction puts windows in places on the screens: each rule finds windows by their program
// (and a part of their title) and moves them to a place on a screen. A window not there yet is
// waited for, up to Wait seconds. Windows only: done by AutoHotkey v2 scripts aex writes.
type OrientAction struct {
	Windows []WindowRule `json:"windows"`
	// Wait is how many seconds to wait for windows not there yet; nil: 30, 0: only those there now.
	Wait *int `json:"wait,omitempty"`
}

// WindowRule is where windows go.
type WindowRule struct {
	// Exe is the window's program: chrome.exe.
	Exe string `json:"exe"`
	// Title, when set, is a part of the window's title (ignoring case).
	Title string `json:"title,omitempty"`
	// Monitor is the screen's number, from 1; 0: the main one.
	Monitor int `json:"monitor,omitempty"`
	// Place is one of Places; custom places it at X, Y, W, H (percent of the screen's work area).
	Place string   `json:"place"`
	X     *float64 `json:"x,omitempty"`
	Y     *float64 `json:"y,omitempty"`
	W     *float64 `json:"w,omitempty"`
	H     *float64 `json:"h,omitempty"`
	// All moves every window that matches, not only the first.
	All bool `json:"all,omitempty"`
}

// Place is a place on a screen, in percent of its work area (left, top, width, height).
type Place struct {
	ID   string     `json:"id"`
	Name string     `json:"name"`
	Rect [4]float64 `json:"rect"`
}

// Places are where a window can go; maximize and minimize are no rectangle.
var Places = []Place{
	{"maximize", "Maximized", [4]float64{0, 0, 100, 100}},
	{"left", "Left half", [4]float64{0, 0, 50, 100}},
	{"right", "Right half", [4]float64{50, 0, 50, 100}},
	{"top", "Top half", [4]float64{0, 0, 100, 50}},
	{"bottom", "Bottom half", [4]float64{0, 50, 100, 50}},
	{"top-left", "Top left quarter", [4]float64{0, 0, 50, 50}},
	{"top-right", "Top right quarter", [4]float64{50, 0, 50, 50}},
	{"bottom-left", "Bottom left quarter", [4]float64{0, 50, 50, 50}},
	{"bottom-right", "Bottom right quarter", [4]float64{50, 50, 50, 50}},
	{"left-third", "Left third", [4]float64{0, 0, 100.0 / 3, 100}},
	{"center-third", "Middle third", [4]float64{100.0 / 3, 0, 100.0 / 3, 100}},
	{"right-third", "Right third", [4]float64{200.0 / 3, 0, 100.0 / 3, 100}},
	{"left-two-thirds", "Left two thirds", [4]float64{0, 0, 200.0 / 3, 100}},
	{"right-two-thirds", "Right two thirds", [4]float64{100.0 / 3, 0, 200.0 / 3, 100}},
	{"center", "Centered", [4]float64{15, 10, 70, 80}},
	{"minimize", "Minimized", [4]float64{}},
	{"custom", "Custom", [4]float64{}},
}

func placeNamed(id string) *Place {
	for i := range Places {
		if Places[i].ID == id {
			return &Places[i]
		}
	}
	return nil
}

// rect is where r puts its windows, in percent of the work area.
func (r WindowRule) rect() [4]float64 {
	if r.Place != "custom" {
		if p := placeNamed(r.Place); p != nil {
			return p.Rect
		}
		return [4]float64{}
	}
	v := func(f *float64, def float64) float64 {
		if f == nil {
			return def
		}
		return *f
	}
	return [4]float64{v(r.X, 0), v(r.Y, 0), v(r.W, 100), v(r.H, 100)}
}

func (r WindowRule) String() string {
	s := r.Exe
	if r.Title != "" {
		s += fmt.Sprintf(" %q", r.Title)
	}
	return s
}

func (a *OrientAction) Kind() string { return "orient" }

func (a *OrientAction) Describe() string {
	switch len(a.Windows) {
	case 0:
		return "Orient windows"
	case 1:
		return "Orient " + a.Windows[0].String()
	}
	return fmt.Sprintf("Orient %s and %d more", a.Windows[0].String(), len(a.Windows)-1)
}

func (a *OrientAction) wait() time.Duration {
	if a.Wait == nil {
		return 30 * second
	}
	return time.Duration(*a.Wait) * second
}

func (a *OrientAction) Check(c *Checker) {
	if runtime.GOOS != "windows" {
		c.Warn("windows", "orienting windows works only on Windows for now")
	} else if autohotkey.Exe() == "" {
		c.Warn("windows", "it needs AutoHotkey v2: winget install AutoHotkey.AutoHotkey")
	}
	if len(a.Windows) == 0 {
		c.Error("windows", "add the windows to orient")
	}
	if a.Wait != nil && *a.Wait < 0 {
		c.Error("wait", "seconds to wait cannot be negative")
	}
	for i, w := range a.Windows {
		n := i + 1
		if strings.TrimSpace(w.Exe) == "" {
			c.Error("windows", "window %d: pick its program", n)
		}
		if placeNamed(w.Place) == nil {
			c.Error("windows", "window %d: no place %q", n, w.Place)
		}
		if w.Monitor < 0 {
			c.Error("windows", "window %d: screens are numbered from 1 (0: the main one)", n)
		}
		if w.Place == "custom" {
			r := w.rect()
			if r[2] <= 0 || r[3] <= 0 || r[0] < 0 || r[1] < 0 || r[0]+r[2] > 100.01 || r[1]+r[3] > 100.01 {
				c.Error("windows", "window %d: a custom place is within the screen, in percent", n)
			}
		}
	}
}

func (a *OrientAction) Run(r *Run) error {
	if runtime.GOOS != "windows" {
		return errors.New("orienting windows works only on Windows for now")
	}
	if len(a.Windows) == 0 {
		return errors.New("no windows to orient")
	}
	out, err := runAHK(r.Ctx, orientScript(a.Windows, a.wait()))
	if err != nil {
		return err
	}
	var missing []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 2 {
			continue
		}
		i, err := strconv.Atoi(f[1])
		if err != nil || i < 1 || i > len(a.Windows) {
			continue
		}
		w := a.Windows[i-1]
		switch f[0] {
		case "ok":
			r.Printf("Placed %s: %s", w, placeNamed(w.Place).Name)
		case "missing":
			missing = append(missing, w.String())
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("not found (waited %s): %s", a.wait(), strings.Join(missing, ", "))
	}
	return nil
}

// runAHK runs an AutoHotkey v2 script and gives what it wrote to stdout.
func runAHK(ctx context.Context, script string) (string, error) {
	exe := autohotkey.Exe()
	if exe == "" {
		return "", errors.New(autohotkey.NotInstalled)
	}
	f, err := os.CreateTemp("", "aex-*.ahk")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(script)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, exe, "/ErrorStdOut=UTF-8", f.Name())
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("AutoHotkey: %s", msg)
		}
		return "", err
	}
	return stdout.String(), nil
}

// ahkString is s as an AutoHotkey v2 string literal.
func ahkString(s string) string {
	r := strings.NewReplacer("`", "``", `"`, "`\"", "\n", "`n", "\r", "`r", "\t", "`t")
	return `"` + r.Replace(s) + `"`
}

func ahkNumber(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }

// isVisible, the helper both scripts share: a window shown on a screen, not a tool window or a
// cloaked one (an app on another virtual desktop, a suspended Store app).
const ahkVisible = `
isVisible(hwnd) {
    try {
        if !(WinGetStyle(hwnd) & 0x10000000) || (WinGetExStyle(hwnd) & 0x80)
            return false
        if WinGetTitle(hwnd) = ""
            return false
        cls := WinGetClass(hwnd)
        if cls = "Progman" || cls = "WorkerW" || cls = "Shell_TrayWnd" || cls = "Shell_SecondaryTrayWnd"
            return false
        cloaked := 0
        DllCall("dwmapi\DwmGetWindowAttribute", "ptr", hwnd, "uint", 14, "int*", &cloaked, "uint", 4)
        return !cloaked
    }
    return false
}
`

var orientTemplate = template.Must(template.New("orient").Parse(`#Requires AutoHotkey v2.0
#NoTrayIcon
#SingleInstance Off
DetectHiddenWindows false
rules := [
{{- range .Rules}}
    {exe: {{.Exe}}, title: {{.Title}}, mon: {{.Mon}}, place: {{.Place}}, x: {{.X}}, y: {{.Y}}, w: {{.W}}, h: {{.H}}, all: {{.All}}},
{{- end}}
]
deadline := A_TickCount + {{.Wait}}
; Windows there from the start are placed at once; one that shows up later is left a moment to
; finish opening (an app may move its window as it does), then placed.
settle := 1000
there := Map()
for hwnd in WinGetList()
    there[hwnd] := true
seen := Map()
done := Map()
Loop {
    for i, r in rules {
        if done.Has(i)
            continue
        n := 0
        for hwnd in WinGetList("ahk_exe " r.exe) {
            if !isVisible(hwnd)
                continue
            if r.title != "" && !InStr(WinGetTitle(hwnd), r.title)
                continue
            if !there.Has(hwnd) {
                if !seen.Has(hwnd)
                    seen[hwnd] := A_TickCount
                if A_TickCount - seen[hwnd] < settle
                    continue
            }
            try place(hwnd, r)
            n++
            if !r.all
                break
        }
        if n {
            done[i] := n
            FileAppend("ok` + "`t" + `" i "` + "`t" + `" n "` + "`n" + `", "*", "UTF-8")
        }
    }
    if done.Count = rules.Length || A_TickCount > deadline
        break
    Sleep 250
}
for i, r in rules
    if !done.Has(i)
        FileAppend("missing` + "`t" + `" i "` + "`n" + `", "*", "UTF-8")
ExitApp

place(hwnd, r) {
    mon := r.mon
    if mon < 1 || mon > MonitorGetCount()
        mon := MonitorGetPrimary()
    MonitorGetWorkArea(mon, &left, &top, &right, &bottom)
    if r.place = "minimize" {
        WinMinimize(hwnd)
        return
    }
    if WinGetMinMax(hwnd) != 0
        WinRestore(hwnd)
    if r.place = "maximize" {
        WinMove(left + 40, top + 40, , , hwnd)
        WinMaximize(hwnd)
        return
    }
    W := right - left, H := bottom - top
    x := Round(left + W * r.x / 100), y := Round(top + H * r.y / 100)
    w := Round(W * r.w / 100), h := Round(H * r.h / 100)
    WinMove(x, y, w, h, hwnd)
    ; Windows 10 and 11 windows have invisible borders: grow the window by them, so what shows
    ; fills the place.
    frame := Buffer(16)
    if !DllCall("dwmapi\DwmGetWindowAttribute", "ptr", hwnd, "uint", 9, "ptr", frame, "uint", 16) {
        WinGetPos(&wx, &wy, &ww, &wh, hwnd)
        fl := NumGet(frame, 0, "int") - wx, ft := NumGet(frame, 4, "int") - wy
        fr := wx + ww - NumGet(frame, 8, "int"), fb := wy + wh - NumGet(frame, 12, "int")
        if fl || ft || fr || fb
            WinMove(x - fl, y - ft, w + fl + fr, h + ft + fb, hwnd)
    }
}
` + ahkVisible))

// orientScript is the AutoHotkey v2 script that places windows by rules, waiting up to wait for
// those not there. It writes "ok\t<rule>\t<windows>" or "missing\t<rule>" for each rule (from 1).
func orientScript(rules []WindowRule, wait time.Duration) string {
	type rule struct{ Exe, Title, Mon, Place, X, Y, W, H, All string }
	data := struct {
		Rules []rule
		Wait  int64
	}{Wait: wait.Milliseconds()}
	for _, r := range rules {
		rc := r.rect()
		data.Rules = append(data.Rules, rule{
			Exe: ahkString(r.Exe), Title: ahkString(r.Title), Mon: strconv.Itoa(r.Monitor), Place: ahkString(r.Place),
			X: ahkNumber(rc[0]), Y: ahkNumber(rc[1]), W: ahkNumber(rc[2]), H: ahkNumber(rc[3]),
			All: strconv.FormatBool(r.All),
		})
	}
	var b strings.Builder
	orientTemplate.Execute(&b, data)
	return b.String()
}

// Screen is a monitor's work area (without the taskbar), in pixels.
type Screen struct {
	N       int  `json:"n"`
	Primary bool `json:"primary"`
	Left    int  `json:"left"`
	Top     int  `json:"top"`
	Right   int  `json:"right"`
	Bottom  int  `json:"bottom"`
}

// OpenWindow is a window shown now.
type OpenWindow struct {
	Exe   string `json:"exe"`
	Path  string `json:"path"`
	Class string `json:"class"`
	Title string `json:"title"`
	// State: -1 minimized, 1 maximized, 0 neither.
	State int `json:"state"`
	X     int `json:"x"`
	Y     int `json:"y"`
	W     int `json:"w"`
	H     int `json:"h"`
}

const listScript = `#Requires AutoHotkey v2.0
#NoTrayIcon
#SingleInstance Off
DetectHiddenWindows false
out := ""
primary := MonitorGetPrimary()
Loop MonitorGetCount() {
    MonitorGetWorkArea(A_Index, &l, &t, &r, &b)
    out .= "M` + "`t" + `" A_Index "` + "`t" + `" (A_Index = primary) "` + "`t" + `" l "` + "`t" + `" t "` + "`t" + `" r "` + "`t" + `" b "` + "`n" + `"
}
for hwnd in WinGetList() {
    if !isVisible(hwnd)
        continue
    try {
        WinGetPos(&x, &y, &w, &h, hwnd)
        title := StrReplace(StrReplace(StrReplace(WinGetTitle(hwnd), "` + "`t" + `", " "), "` + "`n" + `", " "), "` + "`r" + `", " ")
        out .= "W` + "`t" + `" WinGetProcessName(hwnd) "` + "`t" + `" WinGetProcessPath(hwnd) "` + "`t" + `" WinGetClass(hwnd) "` + "`t" + `" WinGetMinMax(hwnd) "` + "`t" + `" x "` + "`t" + `" y "` + "`t" + `" w "` + "`t" + `" h "` + "`t" + `" title "` + "`n" + `"
    }
}
FileAppend(out, "*", "UTF-8")
` + ahkVisible

// ListWindows lists the screens and the windows shown now, in their order on the screen (the one
// in front first). Windows only, with AutoHotkey v2.
func ListWindows(ctx context.Context) ([]Screen, []OpenWindow, error) {
	if runtime.GOOS != "windows" {
		return nil, nil, errors.New("orienting windows works only on Windows for now")
	}
	out, err := runAHK(ctx, listScript)
	if err != nil {
		return nil, nil, err
	}
	return parseWindows(out)
}

func parseWindows(out string) ([]Screen, []OpenWindow, error) {
	var screens []Screen
	var wins []OpenWindow
	num := func(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Split(strings.TrimRight(sc.Text(), "\r"), "\t")
		switch {
		case f[0] == "M" && len(f) == 7:
			screens = append(screens, Screen{N: num(f[1]), Primary: f[2] == "1", Left: num(f[3]), Top: num(f[4]), Right: num(f[5]), Bottom: num(f[6])})
		case f[0] == "W" && len(f) >= 10:
			wins = append(wins, OpenWindow{Exe: f[1], Path: f[2], Class: f[3], State: num(f[4]),
				X: num(f[5]), Y: num(f[6]), W: num(f[7]), H: num(f[8]), Title: strings.Join(f[9:], " ")})
		}
	}
	if len(screens) == 0 {
		return nil, nil, errors.New("AutoHotkey listed no screens")
	}
	return screens, wins, nil
}
