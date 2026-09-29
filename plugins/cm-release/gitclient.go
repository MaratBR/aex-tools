// The git client the cm-repos-state widget opens: the first one found installed on the device.
package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// gitClient is a git GUI and where it installs.
type gitClient struct {
	name    string
	windows []string // paths under %LOCALAPPDATA% ("local/...") or %ProgramFiles% ("programs/...")
	macApp  string   // the app in /Applications
}

// gitClients in the order they are looked for.
var gitClients = []gitClient{
	{"Fork", []string{"local/Fork/Fork.exe"}, "Fork.app"},
	{"GitHub Desktop", []string{"local/GitHubDesktop/GitHubDesktop.exe"}, "GitHub Desktop.app"},
	{"GitKraken", []string{"local/gitkraken/gitkraken.exe"}, "GitKraken.app"},
	{"Sourcetree", []string{"local/SourceTree/SourceTree.exe", "programs/Atlassian/Sourcetree/SourceTree.exe"}, "Sourcetree.app"},
	{"Sublime Merge", []string{"programs/Sublime Merge/sublime_merge.exe"}, "Sublime Merge.app"},
	{"SmartGit", []string{"programs/SmartGit/bin/smartgit.exe"}, "SmartGit.app"},
	{"TortoiseGit", []string{"programs/TortoiseGit/bin/TortoiseGitProc.exe"}, ""},
}

// findGitClient gives the first git client installed and the file that starts it; ok is false
// when there is none.
func findGitClient() (name, path string, ok bool) {
	roots := map[string]string{"local": os.Getenv("LOCALAPPDATA"), "programs": os.Getenv("ProgramFiles")}
	for _, c := range gitClients {
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

// openGitClientCall starts the git client, without waiting for it.
func openGitClientCall(json.RawMessage) (any, error) {
	name, path, ok := findGitClient()
	if !ok {
		return nil, errors.New("no git client found")
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
		return nil, err
	}
	return nil, cmd.Process.Release()
}
