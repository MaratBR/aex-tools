package ide

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"aex/internal/proc"
)

// jetbrainsIDE is a JetBrains IDE and where it installs.
type jetbrainsIDE struct {
	id, name string
	// folder starts its install folder's name ("JetBrains Rider 2024.3", Toolbox's "Rider") and
	// its settings folder's ("Rider2024.3").
	folders []string
	exe     string   // bin/<exe>64.exe on Windows, bin/<exe>(.sh) elsewhere; also its Toolbox script
	mac     []string // its apps
	// recent is its list of recent projects in its settings folder's options.
	recent string
}

func jb(id, name, exe string, folders, mac []string) jetbrainsIDE {
	return jetbrainsIDE{id: id, name: name, folders: folders, exe: exe, mac: mac, recent: "recentProjects.xml"}
}

var jetbrainsIDEs = []jetbrainsIDE{
	{id: "rider", name: "Rider", folders: []string{"JetBrains Rider", "Rider"}, exe: "rider", mac: []string{"Rider.app"}, recent: "recentSolutions.xml"},
	jb("idea", "IntelliJ IDEA", "idea", []string{"IntelliJ IDEA", "IntelliJIdea", "IdeaIC"}, []string{"IntelliJ IDEA.app", "IntelliJ IDEA Ultimate.app", "IntelliJ IDEA CE.app"}),
	jb("goland", "GoLand", "goland", []string{"GoLand"}, []string{"GoLand.app"}),
	jb("webstorm", "WebStorm", "webstorm", []string{"WebStorm"}, []string{"WebStorm.app"}),
	jb("pycharm", "PyCharm", "pycharm", []string{"PyCharm"}, []string{"PyCharm.app", "PyCharm Professional Edition.app", "PyCharm CE.app"}),
	jb("clion", "CLion", "clion", []string{"CLion"}, []string{"CLion.app"}),
	jb("phpstorm", "PhpStorm", "phpstorm", []string{"PhpStorm"}, []string{"PhpStorm.app"}),
	jb("rubymine", "RubyMine", "rubymine", []string{"RubyMine"}, []string{"RubyMine.app"}),
	jb("datagrip", "DataGrip", "datagrip", []string{"DataGrip"}, []string{"DataGrip.app"}),
	jb("rustrover", "RustRover", "rustrover", []string{"RustRover"}, []string{"RustRover.app"}),
}

// JetBrains finds the JetBrains IDEs: installed by JetBrains Toolbox or on their own, with the
// projects (for Rider, the solutions) each opened lately, from its settings.
var JetBrains Provider = jetbrainsProvider{}

type jetbrainsProvider struct{}

func (jetbrainsProvider) Name() string { return "JetBrains" }

func (jetbrainsProvider) Known(id string) (string, bool) {
	for _, j := range jetbrainsIDEs {
		if j.id == id {
			return j.name, true
		}
	}
	return "", false
}

func (jetbrainsProvider) Installed() []IDE {
	var list []IDE
	for _, j := range jetbrainsIDEs {
		path := j.find()
		if path == "" {
			continue
		}
		i := IDE{ID: j.id, Name: j.name, Provider: JetBrains.Name(), Path: path}
		info := productInfo(path)
		i.Version = info.Version
		i.data = j.settingsDir(info.DataDirectoryName)
		list = append(list, i)
	}
	return list
}

// find is what starts the IDE: its exe (or app) where it installs, else the one its Toolbox
// script starts, else the script, else its command on PATH; "" when it is not installed.
func (j jetbrainsIDE) find() string {
	home, _ := os.UserHomeDir()
	var globs []string
	switch runtime.GOOS {
	case "windows":
		for _, root := range []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"), filepath.Join(os.Getenv("ProgramFiles"), "JetBrains")} {
			for _, f := range j.folders {
				globs = append(globs, filepath.Join(root, f+"*", "bin", j.exe+"64.exe"))
			}
		}
	case "darwin":
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			for _, app := range j.mac {
				globs = append(globs, filepath.Join(dir, app))
			}
		}
	default:
		for _, root := range []string{filepath.Join(home, ".local", "share", "JetBrains", "Toolbox", "apps"), "/opt", filepath.Join(home, ".local", "share")} {
			for _, f := range j.folders {
				globs = append(globs, filepath.Join(root, strings.ToLower(f)+"*", "bin", j.exe), filepath.Join(root, f+"*", "bin", j.exe))
			}
		}
	}
	var matches []string
	for _, g := range globs {
		m, _ := filepath.Glob(g)
		matches = append(matches, m...)
	}
	if p := newest(matches); p != "" {
		return p
	}
	if script := toolboxScript(j.exe); script != "" {
		if exe := scriptTarget(script); exe != "" {
			return exe
		}
		return script
	}
	if p, err := exec.LookPath(j.exe); err == nil {
		return p
	}
	return ""
}

// toolboxScript is the JetBrains Toolbox launcher script name, "" when there is none.
func toolboxScript(name string) string {
	home, _ := os.UserHomeDir()
	var dir string
	switch runtime.GOOS {
	case "windows":
		dir = filepath.Join(os.Getenv("LOCALAPPDATA"), "JetBrains", "Toolbox", "scripts")
		name += ".cmd"
	case "darwin":
		dir = filepath.Join(home, "Library", "Application Support", "JetBrains", "Toolbox", "scripts")
	default:
		dir = filepath.Join(home, ".local", "share", "JetBrains", "Toolbox", "scripts")
	}
	if p := filepath.Join(dir, name); exists(p) {
		return p
	}
	return ""
}

var quoted = regexp.MustCompile(`"([^"]+)"`)

// scriptTarget is the exe (or app) a Toolbox launcher script starts, "" when it cannot tell.
func scriptTarget(script string) string {
	b, err := os.ReadFile(script)
	if err != nil {
		return ""
	}
	for _, m := range quoted.FindAllStringSubmatch(string(b), -1) {
		p := m[1]
		if i := strings.Index(p, ".app/"); i >= 0 {
			p = p[:i+4]
		}
		if p != script && filepath.IsAbs(p) && exists(p) {
			if info, err := os.Stat(p); err == nil && (!info.IsDir() || strings.HasSuffix(p, ".app")) {
				return p
			}
		}
	}
	return ""
}

// product is what product-info.json says about an install.
type product struct {
	Version           string `json:"version"`
	DataDirectoryName string `json:"dataDirectoryName"`
}

// productInfo reads product-info.json of the install that path (its exe or app) is in.
func productInfo(path string) product {
	var file string
	if strings.HasSuffix(path, ".app") {
		file = filepath.Join(path, "Contents", "Resources", "product-info.json")
	} else {
		file = filepath.Join(filepath.Dir(filepath.Dir(path)), "product-info.json")
	}
	var p product
	if b, err := os.ReadFile(file); err == nil {
		json.Unmarshal(b, &p)
	}
	return p
}

// settingsDir is the IDE's settings folder: name (from product-info.json) in JetBrains' folder,
// else the newest one named like the IDE; "" when there is none.
func (j jetbrainsIDE) settingsDir(name string) string {
	root := filepath.Join(userConfigDir(), "JetBrains")
	if name != "" && exists(filepath.Join(root, name)) {
		return filepath.Join(root, name)
	}
	var dirs []string
	for _, f := range j.folders {
		m, _ := filepath.Glob(filepath.Join(root, strings.ReplaceAll(f, " ", "")+"20*"))
		dirs = append(dirs, m...)
	}
	return newest(dirs)
}

func (jetbrainsProvider) Icon(i IDE) string { return exeIcon(i) }

func (jetbrainsProvider) Projects(i IDE) []Project {
	if i.data == "" {
		return nil
	}
	file := "recentProjects.xml"
	for _, j := range jetbrainsIDEs {
		if j.id == i.ID {
			file = j.recent
		}
	}
	f, err := os.Open(filepath.Join(i.data, "options", file))
	if err != nil {
		return nil
	}
	defer f.Close()
	return recentProjects(f)
}

// recentProjects reads a JetBrains IDE's recent projects (recentProjects.xml, Rider's
// recentSolutions.xml): the entries of its additionalInfo map, keyed by path, each with when it was
// opened. $USER_HOME$ is the home folder.
func recentProjects(r io.Reader) []Project {
	home, _ := os.UserHomeDir()
	dec := xml.NewDecoder(r)
	var list []Project
	var key string
	var opened time.Time
	depth, inInfo := 0, -1
	attr := func(e xml.StartElement, name string) string {
		for _, a := range e.Attr {
			if a.Name.Local == name {
				return a.Value
			}
		}
		return ""
	}
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		switch e := t.(type) {
		case xml.StartElement:
			depth++
			switch {
			case e.Name.Local == "option" && attr(e, "name") == "additionalInfo":
				inInfo = depth
			case inInfo >= 0 && depth == inInfo+2 && e.Name.Local == "entry":
				key, opened = attr(e, "key"), time.Time{}
			case key != "" && e.Name.Local == "option":
				switch attr(e, "name") {
				case "projectOpenTimestamp", "activationTimestamp":
					if ms, err := strconv.ParseInt(attr(e, "value"), 10, 64); err == nil && time.UnixMilli(ms).After(opened) {
						opened = time.UnixMilli(ms)
					}
				}
			}
		case xml.EndElement:
			if e.Name.Local == "entry" && key != "" && depth == inInfo+2 {
				path := filepath.FromSlash(strings.ReplaceAll(key, "$USER_HOME$", filepath.ToSlash(home)))
				if p := project(path, opened); p != nil {
					list = append(list, *p)
				}
				key = ""
			}
			if depth == inInfo {
				inInfo = -1
			}
			depth--
		}
	}
	return list
}

func (jetbrainsProvider) Command(i IDE, project string) *exec.Cmd {
	cmd := command(i.Path, project)
	// A Toolbox script would show a console of its own.
	if ext := strings.ToLower(filepath.Ext(i.Path)); ext == ".cmd" || ext == ".bat" {
		proc.HideConsole(cmd)
	}
	return cmd
}
