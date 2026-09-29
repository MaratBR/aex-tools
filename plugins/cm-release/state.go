// The cm-repos-state widget's data: the git state of every CM repo, from what git knows locally (no
// fetch), so it is quick enough to load with the home page.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// reposState is the "state" call's result.
type reposState struct {
	ReposDir string      `json:"reposDir"` // empty: not set yet, and Repos is empty
	Repos    []repoState `json:"repos"`
	// Clean is true when no repo has changes, unpushed commits or an error.
	Clean bool `json:"clean"`
	// GitClient is the git client openGitClient starts, empty when none is installed.
	GitClient string `json:"gitClient,omitempty"`
}

type repoState struct {
	Name     string `json:"name"`
	Branch   string `json:"branch,omitempty"`
	Changed  int    `json:"changed"`  // files with uncommitted changes (untracked included)
	Ahead    int    `json:"ahead"`    // commits not pushed to the upstream
	Behind   int    `json:"behind"`   // upstream commits not pulled, as of the last fetch
	Upstream bool   `json:"upstream"` // the branch has an upstream
	Error    string `json:"error,omitempty"`
}

func (s repoState) pending() bool { return s.Error != "" || s.Changed > 0 || s.Ahead > 0 }

func stateCall(json.RawMessage) (any, error) {
	c, err := loadConfig()
	if err != nil {
		return nil, err
	}
	st := reposState{ReposDir: c.ReposDir, Repos: []repoState{}, Clean: true}
	st.GitClient, _, _ = findGitClient()
	if c.ReposDir == "" {
		return st, nil
	}
	st.Repos = make([]repoState, len(c.Repos))
	var wg sync.WaitGroup
	for i, name := range c.Repos {
		wg.Go(func() { st.Repos[i] = readRepoState(repo{name, filepath.Join(c.ReposDir, name)}) })
	}
	wg.Wait()
	for _, r := range st.Repos {
		if r.pending() {
			st.Clean = false
		}
	}
	return st, nil
}

func readRepoState(r repo) repoState {
	s := repoState{Name: r.name}
	if _, err := os.Stat(filepath.Join(r.path, ".git")); err != nil {
		s.Error = "not a git repo: " + r.path
		return s
	}
	out, err := r.git("status", "--porcelain=v2", "--branch")
	if err != nil {
		s.Error = lastLine(out, err)
		return s
	}
	for line := range strings.Lines(out) {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			s.Branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.upstream "):
			s.Upstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			// # branch.ab +<ahead> -<behind>
			if f := strings.Fields(line); len(f) == 4 {
				s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[2], "+"))
				s.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[3], "-"))
			}
		case line != "" && !strings.HasPrefix(line, "#"):
			s.Changed++
		}
	}
	return s
}

// lastLine is the last line git printed, or err when it printed nothing.
func lastLine(out string, err error) string {
	if out = strings.TrimSpace(out); out == "" {
		return fmt.Sprint(err)
	}
	return out[strings.LastIndex(out, "\n")+1:]
}
