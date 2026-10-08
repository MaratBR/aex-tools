package ide

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"aex/internal/proc"
)

// VisualStudio finds the Visual Studio installs (Windows only), each with the solutions, projects
// and folders it opened lately. Each install is "vs<year>" ("vs2022"); "vs" is the newest.
var VisualStudio Provider = visualStudioProvider{}

type visualStudioProvider struct{}

func (visualStudioProvider) Name() string { return "Visual Studio" }

var vsID = regexp.MustCompile(`^vs(\d{4})?(-\d+)?$`)

func (visualStudioProvider) Known(id string) (string, bool) {
	m := vsID.FindStringSubmatch(id)
	if m == nil {
		return "", false
	}
	if m[1] == "" {
		return "Visual Studio", true
	}
	return "Visual Studio " + m[1], true
}

// vsInstance is what vswhere says about an install.
type vsInstance struct {
	InstanceID          string `json:"instanceId"`
	InstallationVersion string `json:"installationVersion"`
	ProductPath         string `json:"productPath"`
	DisplayName         string `json:"displayName"`
	Catalog             struct {
		ProductLineVersion string `json:"productLineVersion"`
	} `json:"catalog"`
}

func (visualStudioProvider) Installed() []IDE {
	if runtime.GOOS != "windows" {
		return nil
	}
	var list []IDE
	for _, in := range vswhere() {
		if !exists(in.ProductPath) {
			continue
		}
		id := "vs" + in.Catalog.ProductLineVersion
		for n := 2; slices.ContainsFunc(list, func(i IDE) bool { return i.ID == id }); n++ {
			id = fmt.Sprintf("vs%s-%d", in.Catalog.ProductLineVersion, n)
		}
		name := in.DisplayName
		if name == "" {
			name = "Visual Studio " + in.Catalog.ProductLineVersion
		}
		major, _, _ := strings.Cut(in.InstallationVersion, ".")
		list = append(list, IDE{ID: id, Name: name, Version: in.InstallationVersion, Provider: VisualStudio.Name(),
			Path: in.ProductPath, data: filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "VisualStudio", major+".0_"+in.InstanceID)})
	}
	return list
}

// vswhere asks the Visual Studio installer for the installs, newest first; none without it.
func vswhere() []vsInstance {
	exe := filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft Visual Studio", "Installer", "vswhere.exe")
	if !exists(exe) {
		return nil
	}
	cmd := exec.Command(exe, "-all", "-prerelease", "-products", "*", "-format", "json", "-utf8")
	proc.HideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var list []vsInstance
	if json.Unmarshal(out, &list) != nil {
		return nil
	}
	slices.SortStableFunc(list, func(a, b vsInstance) int { return compareVersions(b.InstallationVersion, a.InstallationVersion) })
	return list
}

func (visualStudioProvider) Icon(i IDE) string { return exeIcon(i) }

func (visualStudioProvider) Projects(i IDE) []Project {
	f, err := os.Open(filepath.Join(i.data, "ApplicationPrivateSettings.xml"))
	if err != nil {
		return nil
	}
	defer f.Close()
	return vsRecent(f)
}

// vsRecent reads the recent list of Visual Studio's start window from its
// ApplicationPrivateSettings.xml: the JSON in the value of its CodeContainers.Offline collection.
func vsRecent(r io.Reader) []Project {
	dec := xml.NewDecoder(r)
	in := false
	for {
		t, err := dec.Token()
		if err != nil {
			return nil
		}
		e, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		name := ""
		for _, a := range e.Attr {
			if a.Name.Local == "name" {
				name = a.Value
			}
		}
		switch {
		case e.Name.Local == "collection":
			in = name == "CodeContainers.Offline"
		case in && e.Name.Local == "value":
			var text string
			if dec.DecodeElement(&text, &e) != nil {
				return nil
			}
			return vsContainers(text)
		}
	}
}

func vsContainers(text string) []Project {
	var items []struct {
		Key   string
		Value struct {
			LocalProperties struct{ FullPath string }
			LastAccessed    time.Time
		}
	}
	if json.Unmarshal([]byte(text), &items) != nil {
		return nil
	}
	var list []Project
	for _, it := range items {
		path := it.Value.LocalProperties.FullPath
		if path == "" {
			path = it.Key
		}
		if p := project(path, it.Value.LastAccessed); p != nil {
			list = append(list, *p)
		}
	}
	return list
}

func (visualStudioProvider) Command(i IDE, project string) *exec.Cmd { return command(i.Path, project) }
