package ide

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"aex/internal/proc"
)

// VSCode finds Visual Studio Code, with the folders and workspaces it opened lately (from the
// workspace storage it keeps for each).
var VSCode Provider = vscodeProvider{}

type vscodeProvider struct{}

func (vscodeProvider) Name() string { return "Visual Studio Code" }

func (vscodeProvider) Known(id string) (string, bool) {
	if id == "vscode" {
		return "Visual Studio Code", true
	}
	return "", false
}

func (vscodeProvider) Installed() []IDE {
	path := findVSCode()
	if path == "" {
		return nil
	}
	return []IDE{{ID: "vscode", Name: "Visual Studio Code", Provider: VSCode.Name(), Path: path,
		data: filepath.Join(userConfigDir(), "Code", "User")}}
}

// findVSCode is what starts VS Code: Code.exe (also the one next to the code command on PATH),
// its app on macOS, else the code command; "" when it is not installed.
func findVSCode() string {
	switch runtime.GOOS {
	case "windows":
		for _, p := range []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "Code.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code", "Code.exe"),
		} {
			if exists(p) {
				return p
			}
		}
	case "darwin":
		home, _ := os.UserHomeDir()
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			if p := filepath.Join(dir, "Visual Studio Code.app"); exists(p) {
				return p
			}
		}
	}
	p, err := exec.LookPath("code")
	if err != nil {
		return ""
	}
	// On Windows code is bin\code.cmd: the exe is next to bin.
	if exe := filepath.Join(filepath.Dir(filepath.Dir(p)), "Code.exe"); runtime.GOOS == "windows" && exists(exe) {
		return exe
	}
	return p
}

func (vscodeProvider) Icon(i IDE) string { return exeIcon(i) }

// Projects are the folders and workspaces of VS Code's workspace storage (a folder for each, with
// workspace.json saying which), each opened when its state was last saved.
func (vscodeProvider) Projects(i IDE) []Project {
	dirs, err := os.ReadDir(filepath.Join(i.data, "workspaceStorage"))
	if err != nil {
		return nil
	}
	var list []Project
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(i.data, "workspaceStorage", d.Name())
		b, err := os.ReadFile(filepath.Join(dir, "workspace.json"))
		if err != nil {
			continue
		}
		var w struct{ Folder, Workspace string }
		if json.Unmarshal(b, &w) != nil {
			continue
		}
		uri := w.Folder
		if uri == "" {
			uri = w.Workspace
		}
		path := fileURIPath(uri)
		if path == "" {
			continue
		}
		var opened time.Time
		for _, f := range []string{filepath.Join(dir, "state.vscdb"), dir} {
			if info, err := os.Stat(f); err == nil {
				opened = info.ModTime()
				break
			}
		}
		if p := project(path, opened); p != nil {
			list = append(list, *p)
		}
	}
	return list
}

// fileURIPath is the path of a file: URI ("file:///c%3A/src/app"), "" for any other (a remote one).
func fileURIPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return ""
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
		// VS Code writes the drive in lower case.
		if len(p) >= 2 && p[1] == ':' {
			p = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return filepath.FromSlash(p)
}

func (vscodeProvider) Command(i IDE, project string) *exec.Cmd {
	cmd := command(i.Path, project)
	if ext := strings.ToLower(filepath.Ext(i.Path)); ext == ".cmd" || ext == ".bat" {
		proc.HideConsole(cmd)
	}
	return cmd
}
