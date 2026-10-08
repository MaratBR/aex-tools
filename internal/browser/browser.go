// Package browser opens web links, in a browser the link names or else the default one:
// "aex+brave://google.com" opens https://google.com in Brave when it is installed, else in the
// default browser. Plain http and https links open in the default browser.
package browser

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"aex/internal/tool"
)

// Scheme starts a link that names its browser: "aex+<browser>://<address>". The address is
// opened as https unless it has a scheme of its own ("aex+firefox://http://intranet").
const Scheme = "aex+"

// Browser is one browser aex knows how to find.
type Browser struct {
	Name    string   // how a link names it, e.g. brave
	Title   string   // e.g. Brave
	Aliases []string // other names links may use
	// Windows: prefixes (lowercase) of its key under SOFTWARE\Clients\StartMenuInternet, its exe
	// (for App Paths and PATH) and where it installs, under %LOCALAPPDATA%, %ProgramFiles% and
	// %ProgramFiles(x86)%.
	StartMenu []string
	Exe       string
	Paths     []string
	Mac       []string // macOS app names
	Linux     []string // commands on Linux
}

// Browsers are the browsers links can name.
var Browsers = []Browser{
	{Name: "chrome", Title: "Google Chrome", Aliases: []string{"google-chrome"}, StartMenu: []string{"google chrome"},
		Exe: "chrome.exe", Paths: []string{`Google\Chrome\Application\chrome.exe`},
		Mac: []string{"Google Chrome"}, Linux: []string{"google-chrome", "google-chrome-stable"}},
	{Name: "edge", Title: "Microsoft Edge", Aliases: []string{"msedge"}, StartMenu: []string{"microsoft edge"},
		Exe: "msedge.exe", Paths: []string{`Microsoft\Edge\Application\msedge.exe`},
		Mac: []string{"Microsoft Edge"}, Linux: []string{"microsoft-edge", "microsoft-edge-stable"}},
	{Name: "firefox", Title: "Firefox", Aliases: []string{"mozilla"}, StartMenu: []string{"firefox-", "firefox"},
		Exe: "firefox.exe", Paths: []string{`Mozilla Firefox\firefox.exe`},
		Mac: []string{"Firefox"}, Linux: []string{"firefox"}},
	{Name: "brave", Title: "Brave", StartMenu: []string{"brave"},
		Exe: "brave.exe", Paths: []string{`BraveSoftware\Brave-Browser\Application\brave.exe`},
		Mac: []string{"Brave Browser"}, Linux: []string{"brave-browser", "brave"}},
	{Name: "vivaldi", Title: "Vivaldi", StartMenu: []string{"vivaldi"},
		Exe: "vivaldi.exe", Paths: []string{`Vivaldi\Application\vivaldi.exe`},
		Mac: []string{"Vivaldi"}, Linux: []string{"vivaldi", "vivaldi-stable"}},
	{Name: "yandex", Title: "Yandex Browser", StartMenu: []string{"yandex"},
		Exe: "browser.exe", Paths: []string{`Yandex\YandexBrowser\Application\browser.exe`},
		Mac: []string{"Yandex"}, Linux: []string{"yandex-browser", "yandex-browser-stable"}},
	{Name: "opera", Title: "Opera", StartMenu: []string{"operastable"},
		Exe: "opera.exe", Paths: []string{`Programs\Opera\launcher.exe`, `Opera\launcher.exe`},
		Mac: []string{"Opera"}, Linux: []string{"opera"}},
	{Name: "opera-gx", Title: "Opera GX", Aliases: []string{"operagx"}, StartMenu: []string{"operagxstable", "opera gx"},
		Paths: []string{`Programs\Opera GX\launcher.exe`},
		Mac:   []string{"Opera GX"}},
	{Name: "chromium", Title: "Chromium", StartMenu: []string{"chromium"},
		Paths: []string{`Chromium\Application\chrome.exe`},
		Mac:   []string{"Chromium"}, Linux: []string{"chromium", "chromium-browser"}},
	{Name: "librewolf", Title: "LibreWolf", StartMenu: []string{"librewolf"},
		Exe: "librewolf.exe", Paths: []string{`LibreWolf\librewolf.exe`},
		Mac: []string{"LibreWolf"}, Linux: []string{"librewolf"}},
	{Name: "waterfox", Title: "Waterfox", StartMenu: []string{"waterfox"},
		Exe: "waterfox.exe", Paths: []string{`Waterfox\waterfox.exe`},
		Mac: []string{"Waterfox"}, Linux: []string{"waterfox"}},
	{Name: "arc", Title: "Arc", StartMenu: []string{"arc"}, Mac: []string{"Arc"}},
	{Name: "safari", Title: "Safari", Mac: []string{"Safari"}},
}

// Find is the browser a link names (by name or alias, ignoring case); nil when aex does not know it.
func Find(name string) *Browser {
	name = strings.ToLower(name)
	for i, b := range Browsers {
		if b.Name == name || contains(b.Aliases, name) {
			return &Browsers[i]
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Parse splits a link into the web address it opens (http or https) and the browser it names
// ("" for the default one). Other kinds of links (file:, javascript:, …) are refused.
func Parse(link string) (address, browser string, err error) {
	link = strings.TrimSpace(link)
	if rest, ok := cutPrefixFold(link, Scheme); ok {
		name, addr, ok := strings.Cut(rest, "://")
		if !ok || name == "" || strings.ContainsAny(name, "/?#") {
			return "", "", fmt.Errorf("not a link: %q (use %s<browser>://<address>)", link, Scheme)
		}
		if !hasWebScheme(addr) {
			addr = "https://" + addr
		}
		address, browser = addr, strings.ToLower(name)
	} else {
		address = link
	}
	u, err := url.Parse(address)
	if err != nil || !hasWebScheme(address) || u.Host == "" {
		return "", "", fmt.Errorf("not a web link: %q", link)
	}
	return address, browser, nil
}

func hasWebScheme(s string) bool {
	_, http := cutPrefixFold(s, "http://")
	_, https := cutPrefixFold(s, "https://")
	return http || https
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// Title is how a link's browser is shown: its title, or the name as written for one aex does not
// know, "" for the default one.
func Title(name string) string {
	if b := Find(name); b != nil {
		return b.Title
	}
	return name
}

// ErrNotFound is returned by launch when the browser is not installed.
var ErrNotFound = errors.New("browser not found")

// Open opens a link (see Parse): in the browser it names when that is installed, else in the
// default browser. It does not wait for the browser.
func Open(link string) error {
	address, name, err := Parse(link)
	if err != nil {
		return err
	}
	if name != "" {
		b := Find(name)
		if b == nil {
			// Not one aex knows: maybe a command of that name.
			b = &Browser{Name: name, Exe: name + ".exe", Linux: []string{name}, Mac: []string{name}}
		}
		err := launch(b, address)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			fmt.Fprintf(os.Stderr, "opening %s in %s: %v; opening it in the default browser\n", address, Title(name), err)
		}
	}
	allowForeground()
	return tool.OpenURL(address)
}

// Installed is what starts b on this device: its exe (Windows), its app (macOS) or its command
// (Linux); "" when it is not installed.
func Installed(b *Browser) string { return installed(b) }
