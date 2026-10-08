package composer

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/appicon"
	"aex/internal/browser"
	"aex/internal/ide"
)

// Suggestion is a step of a kind of action likely to be wanted on this device, ready to add: run
// a browser found here, open a project an IDE opened lately.
type Suggestion struct {
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	// Icon is a PNG data: URL, "" for the action's own.
	Icon string `json:"icon,omitempty"`
	// Step is the step it adds, as JSON.
	Step json.RawMessage `json:"step"`
}

func suggestion(name, summary, icon string, step any) Suggestion {
	b, _ := json.Marshal(step)
	return Suggestion{Name: name, Summary: summary, Icon: icon, Step: b}
}

// suggesters give the suggestions of each kind of action that has some.
var suggesters = map[string]func() []Suggestion{
	"run":      programSuggestions,
	"open-ide": ideSuggestions,
	"delay": func() []Suggestion {
		var list []Suggestion
		for _, s := range []float64{2, 5, 10, 30, 60} {
			list = append(list, suggestion("Wait "+seconds(s), "", "", map[string]any{"action": "delay", "seconds": s}))
		}
		return list
	},
}

var (
	suggestMu sync.Mutex
	suggested map[string][]Suggestion
	suggestAt time.Time
)

// Suggestions gives the suggestions of each kind of action that has some, by kind. What is found
// is reused for 30 seconds.
func Suggestions() map[string][]Suggestion {
	suggestMu.Lock()
	defer suggestMu.Unlock()
	if suggested != nil && time.Since(suggestAt) < 30*time.Second {
		return suggested
	}
	out := map[string][]Suggestion{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for kind, f := range suggesters {
		wg.Go(func() {
			list := f()
			mu.Lock()
			out[kind] = list
			mu.Unlock()
		})
	}
	wg.Wait()
	suggested, suggestAt = out, time.Now()
	return out
}

// ideSuggestions: each IDE found, opening the project it opened last, then the projects opened
// last in any of them.
func ideSuggestions() []Suggestion {
	var list, recent []Suggestion
	type opened struct {
		s Suggestion
		t time.Time
	}
	var all []opened
	for _, i := range ide.Installed() {
		icon := ide.Icon(i)
		projects := ide.Projects(i, 3)
		project := ""
		if len(projects) > 0 {
			project = projects[0].Path
		}
		list = append(list, suggestion("Open in "+i.Name, i.Provider+" · "+i.Path, icon,
			map[string]any{"action": "open-ide", "ide": i.ID, "project": project}))
		for _, p := range projects {
			all = append(all, opened{suggestion("Open "+p.Name+" in "+i.Name, p.Path, icon,
				map[string]any{"action": "open-ide", "ide": i.ID, "project": p.Path}), p.Opened})
		}
	}
	slices.SortStableFunc(all, func(a, b opened) int { return b.t.Compare(a.t) })
	for _, o := range all[:min(len(all), 5)] {
		recent = append(recent, o.s)
	}
	return append(list, recent...)
}

// app is a program people often start, and where it installs.
type app struct {
	name string
	// windows are globs under %LOCALAPPDATA% ("local/…"), %APPDATA% ("roaming/…"),
	// %ProgramFiles% ("programs/…") or %ProgramFiles(x86)% ("programs86/…"); the last match counts.
	windows []string
	mac     string   // its app
	linux   []string // its commands
	args    string   // what it is started with, on Windows
}

// apps are the programs Run a program suggests when found, after the browsers: VPN clients first
// (a composed tool often starts one), then chat and other everyday apps.
var apps = []app{
	{"WireGuard", []string{"programs/WireGuard/wireguard.exe"}, "WireGuard.app", nil, ""},
	{"OpenVPN Connect", []string{"programs/OpenVPN Connect/OpenVPNConnect.exe"}, "OpenVPN Connect.app", nil, ""},
	{"OpenVPN GUI", []string{"programs/OpenVPN/bin/openvpn-gui.exe"}, "", nil, ""},
	{"Cisco Secure Client", []string{"programs86/Cisco/Cisco Secure Client/UI/csc_ui.exe", "programs86/Cisco/Cisco AnyConnect Secure Mobility Client/vpnui.exe"}, "Cisco Secure Client.app", nil, ""},
	{"FortiClient", []string{"programs/Fortinet/FortiClient/FortiClient.exe"}, "FortiClient.app", nil, ""},
	{"GlobalProtect", []string{"programs/Palo Alto Networks/GlobalProtect/PanGPA.exe"}, "GlobalProtect.app", nil, ""},
	{"NordVPN", []string{"programs/NordVPN/NordVPN.exe"}, "NordVPN.app", []string{"nordvpn"}, ""},
	{"Proton VPN", []string{"programs/Proton/VPN/ProtonVPN.Launcher.exe", "programs/Proton Technologies/ProtonVPN/ProtonVPN.exe"}, "Proton VPN.app", []string{"protonvpn-app"}, ""},
	{"AmneziaVPN", []string{"programs/AmneziaVPN/AmneziaVPN.exe"}, "AmneziaVPN.app", nil, ""},
	{"Outline", []string{"local/Programs/outline-client/Outline.exe"}, "Outline.app", nil, ""},
	{"Slack", []string{"local/slack/slack.exe"}, "Slack.app", []string{"slack"}, ""},
	{"Microsoft Teams", []string{"local/Microsoft/WindowsApps/ms-teams.exe"}, "Microsoft Teams.app", []string{"teams-for-linux"}, ""},
	{"Microsoft Outlook", []string{"programs/Microsoft Office/root/Office16/OUTLOOK.EXE", "programs86/Microsoft Office/root/Office16/OUTLOOK.EXE"}, "Microsoft Outlook.app", nil, ""},
	{"Telegram", []string{"roaming/Telegram Desktop/Telegram.exe"}, "Telegram.app", []string{"telegram-desktop"}, ""},
	// Discord's exe is in a folder named by its version: its updater starts the current one.
	{"Discord", []string{"local/Discord/Update.exe"}, "Discord.app", []string{"discord"}, "--processStart Discord.exe"},
	{"Zoom", []string{"roaming/Zoom/bin/Zoom.exe"}, "zoom.us.app", []string{"zoom"}, ""},
	{"Spotify", []string{"roaming/Spotify/Spotify.exe"}, "Spotify.app", []string{"spotify"}, ""},
	{"Postman", []string{"local/Postman/Postman.exe"}, "Postman.app", []string{"postman"}, ""},
	{"Docker Desktop", []string{"programs/Docker/Docker/Docker Desktop.exe"}, "Docker.app", nil, ""},
	{"Windows Terminal", []string{"local/Microsoft/WindowsApps/wt.exe"}, "", nil, ""},
	{"Notepad++", []string{"programs/Notepad++/notepad++.exe"}, "", nil, ""},
	{"Obsidian", []string{"local/Programs/Obsidian/Obsidian.exe"}, "Obsidian.app", []string{"obsidian"}, ""},
	{"Notion", []string{"local/Programs/Notion/Notion.exe"}, "Notion.app", nil, ""},
}

// find is what starts the app on this device, "" when it is not installed.
func (a app) find() string {
	switch runtime.GOOS {
	case "windows":
		roots := map[string]string{"local": os.Getenv("LOCALAPPDATA"), "roaming": os.Getenv("APPDATA"),
			"programs": os.Getenv("ProgramFiles"), "programs86": os.Getenv("ProgramFiles(x86)")}
		for _, w := range a.windows {
			root, rest, _ := strings.Cut(w, "/")
			if roots[root] == "" {
				continue
			}
			if m, _ := filepath.Glob(filepath.Join(roots[root], filepath.FromSlash(rest))); len(m) > 0 {
				slices.Sort(m)
				return m[len(m)-1]
			}
		}
	case "darwin":
		home, _ := os.UserHomeDir()
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			if a.mac != "" && exists(filepath.Join(dir, a.mac)) {
				return filepath.Join(dir, a.mac)
			}
		}
	default:
		for _, c := range a.linux {
			if p, err := exec.LookPath(c); err == nil {
				return p
			}
		}
	}
	return ""
}

// programSuggestions: the browsers, then the apps found here, each started without waiting for it.
func programSuggestions() []Suggestion {
	type candidate struct {
		name, args string
		find       func() string
		path       string
	}
	var list []candidate
	for i := range browser.Browsers {
		b := &browser.Browsers[i]
		list = append(list, candidate{name: b.Title, find: func() string { return browser.Installed(b) }})
	}
	for _, a := range apps {
		list = append(list, candidate{name: a.name, args: a.args, find: a.find})
	}
	var wg sync.WaitGroup
	for i := range list {
		wg.Go(func() { list[i].path = list[i].find() })
	}
	wg.Wait()
	var out []Suggestion
	seen := map[string]bool{}
	for _, c := range list {
		if c.path == "" || seen[c.path] {
			continue
		}
		seen[c.path] = true
		step := map[string]any{"action": "run", "label": "Open " + c.name, "path": portable(c.path), "detach": true, "skipRunning": true}
		if c.args != "" && runtime.GOOS == "windows" {
			step["args"] = c.args
		}
		out = append(out, suggestion("Open "+c.name, c.path, appicon.DataURL(c.path), step))
	}
	return out
}

// portable is path with the user's folders as the variables that name them (%LOCALAPPDATA%,
// %APPDATA%, %ProgramFiles%; the run action expands them), so the step works for another user too.
func portable(path string) string {
	if runtime.GOOS != "windows" {
		return path
	}
	for _, v := range []string{"LOCALAPPDATA", "APPDATA", "ProgramFiles(x86)", "ProgramFiles"} {
		dir := os.Getenv(v)
		if dir != "" && len(path) > len(dir) && strings.EqualFold(path[:len(dir)], dir) && (path[len(dir)] == '\\' || path[len(dir)] == '/') {
			return "%" + v + "%" + path[len(dir):]
		}
	}
	return path
}
