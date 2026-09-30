// Package gitclient finds the git client (a git GUI) installed on the device, opens it and gives
// its icon, for the Git status widget.
package gitclient

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// client is a git GUI and where it installs.
type client struct {
	name    string
	windows []string // paths under %LOCALAPPDATA% ("local/...") or %ProgramFiles% ("programs/...")
	macApp  string   // the app in /Applications
}

// clients in the order they are looked for.
var clients = []client{
	{"Fork", []string{"local/Fork/Fork.exe"}, "Fork.app"},
	{"GitHub Desktop", []string{"local/GitHubDesktop/GitHubDesktop.exe"}, "GitHub Desktop.app"},
	{"GitKraken", []string{"local/gitkraken/gitkraken.exe"}, "GitKraken.app"},
	{"Sourcetree", []string{"local/SourceTree/SourceTree.exe", "programs/Atlassian/Sourcetree/SourceTree.exe"}, "Sourcetree.app"},
	{"Sublime Merge", []string{"programs/Sublime Merge/sublime_merge.exe"}, "Sublime Merge.app"},
	{"SmartGit", []string{"programs/SmartGit/bin/smartgit.exe"}, "SmartGit.app"},
	{"TortoiseGit", []string{"programs/TortoiseGit/bin/TortoiseGitProc.exe"}, ""},
}

// Find gives the first git client installed and the file that starts it (on macOS its app); ok is
// false when there is none.
func Find() (name, path string, ok bool) {
	roots := map[string]string{"local": os.Getenv("LOCALAPPDATA"), "programs": os.Getenv("ProgramFiles")}
	for _, c := range clients {
		var paths []string
		switch runtime.GOOS {
		case "windows":
			for _, p := range c.windows {
				root, rest, _ := strings.Cut(p, "/")
				if roots[root] != "" {
					paths = append(paths, filepath.Join(roots[root], filepath.FromSlash(rest)))
				}
			}
		case "darwin":
			if c.macApp != "" {
				paths = append(paths, filepath.Join("/Applications", c.macApp))
			}
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return c.name, p, true
			}
		}
	}
	return "", "", false
}

// Open starts the git client, without waiting for it.
func Open() error {
	name, path, ok := Find()
	if !ok {
		return errors.New("no git client found")
	}
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", "-a", path)
	case name == "TortoiseGit":
		cmd = exec.Command(path, "/command:repostatus")
	default:
		cmd = exec.Command(path)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// IconSize is the icon's width and height in pixels.
const IconSize = 64

var icons sync.Map // path: data URL, or "" when it has none

// Icon is the icon of the git client at path (from Find) as a PNG data: URL, "" when it cannot be
// read. Kept for the path once read.
func Icon(path string) string {
	if url, ok := icons.Load(path); ok {
		return url.(string)
	}
	url := ""
	if img, err := readIcon(path, IconSize); err == nil {
		var b bytes.Buffer
		if png.Encode(&b, img) == nil {
			url = "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
		}
	}
	icons.Store(path, url)
	return url
}
