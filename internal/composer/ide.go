package composer

import (
	"fmt"
	"path/filepath"
	"strings"

	"aex/internal/ide"
)

// IDEAction opens a project (a solution, a project file or a folder) in an IDE found by an IDE
// provider (internal/ide), without waiting for it.
type IDEAction struct {
	// IDE is the IDE's id: "rider", "vscode", "vs2022" ("vs": the newest Visual Studio).
	IDE     string `json:"ide"`
	Project string `json:"project"`
}

func (a *IDEAction) Kind() string { return "open-ide" }

func (a *IDEAction) Describe() string {
	name := a.IDE
	if n, ok := ide.Known(a.IDE); ok {
		name = n
	}
	if i, ok := ide.Find(a.IDE); ok {
		name = i.Name
	}
	return fmt.Sprintf("Open %s in %s", filepath.Base(expand(a.Project)), name)
}

func (a *IDEAction) Check(c *Checker) {
	if a.IDE == "" {
		c.Error("ide", "pick the IDE")
	} else if name, ok := ide.Known(a.IDE); !ok {
		c.Error("ide", "no IDE %q", a.IDE)
	} else if _, ok := ide.Find(a.IDE); !ok {
		c.Warn("ide", "%s is not found on this device", name)
	}
	if strings.TrimSpace(a.Project) == "" {
		c.Error("project", "pick the project or folder to open")
	} else if !exists(expand(a.Project)) {
		c.Warn("project", "there is no %s", expand(a.Project))
	}
}

func (a *IDEAction) Run(r *Run) error {
	project := expand(a.Project)
	if abs, err := filepath.Abs(project); err == nil {
		project = abs
	}
	if !exists(project) {
		return fmt.Errorf("there is no %s", project)
	}
	i, err := ide.Open(a.IDE, project)
	if err != nil {
		return err
	}
	r.Printf("Opened %s in %s", project, i.Name)
	return nil
}
