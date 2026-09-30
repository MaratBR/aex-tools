package browser

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var pAllowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

// allowForeground lets the browser come to the front: aex may have the right to (the click on a
// reminder was its input), a browser it starts would not.
func allowForeground() {
	if pAllowSetForegroundWindow.Find() == nil {
		pAllowSetForegroundWindow.Call(^uintptr(0)) // ASFW_ANY
	}
}

func launch(b *Browser, address string) error {
	exe := find(b)
	if exe == "" {
		return ErrNotFound
	}
	allowForeground()
	cmd := exec.Command(exe, address)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// find is b's exe: from the browsers Windows has registered, App Paths, where it installs, or PATH.
func find(b *Browser) string {
	if exe := startMenu(b.StartMenu); exe != "" {
		return exe
	}
	roots := []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE}
	if b.Exe != "" {
		for _, root := range roots {
			if exe := regString(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+b.Exe, ""); isFile(exe) {
				return exe
			}
		}
	}
	for _, env := range []string{"LOCALAPPDATA", "ProgramFiles", "ProgramFiles(x86)"} {
		dir := os.Getenv(env)
		if dir == "" {
			continue
		}
		for _, p := range b.Paths {
			if exe := filepath.Join(dir, p); isFile(exe) {
				return exe
			}
		}
	}
	if b.Exe != "" {
		if exe, err := exec.LookPath(b.Exe); err == nil {
			return exe
		}
	}
	return ""
}

// startMenu is the exe of the first browser Windows has registered (SOFTWARE\Clients\StartMenuInternet,
// for the user or the device) whose key starts with one of prefixes, in their order; shorter keys
// first, so "Google Chrome" comes before "Google Chrome Canary.…".
func startMenu(prefixes []string) string {
	type entry struct {
		root registry.Key
		path string
		name string
	}
	var all []entry
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, base := range []string{`SOFTWARE\Clients\StartMenuInternet`, `SOFTWARE\WOW6432Node\Clients\StartMenuInternet`} {
			k, err := registry.OpenKey(root, base, registry.ENUMERATE_SUB_KEYS)
			if err != nil {
				continue
			}
			names, _ := k.ReadSubKeyNames(-1)
			k.Close()
			for _, name := range names {
				all = append(all, entry{root, base + `\` + name, strings.ToLower(name)})
			}
		}
	}
	slices.SortStableFunc(all, func(a, b entry) int { return len(a.name) - len(b.name) })
	for _, prefix := range prefixes {
		for _, e := range all {
			if !strings.HasPrefix(e.name, prefix) {
				continue
			}
			if exe := commandExe(regString(e.root, e.path+`\shell\open\command`, "")); isFile(exe) {
				return exe
			}
		}
	}
	return ""
}

// commandExe is the program of a command line: `"C:\…\brave.exe" --flag` or `C:\…\brave.exe`.
func commandExe(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if rest, ok := strings.CutPrefix(cmd, `"`); ok {
		exe, _, _ := strings.Cut(rest, `"`)
		return exe
	}
	if i := strings.Index(strings.ToLower(cmd), ".exe"); i >= 0 {
		return cmd[:i+4]
	}
	return cmd
}

func regString(root registry.Key, path, name string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, kind, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	if kind == registry.EXPAND_SZ {
		if expanded, err := registry.ExpandString(v); err == nil {
			return expanded
		}
	}
	return v
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return path != "" && err == nil && !st.IsDir()
}
