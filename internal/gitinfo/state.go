package gitinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RepoState is a repo's state from what git knows locally (no fetch), for the Git status widget.
type RepoState struct {
	Path     string `json:"path"`
	Branch   string `json:"branch,omitempty"`
	Changed  int    `json:"changed"`  // files with uncommitted changes (untracked included)
	Ahead    int    `json:"ahead"`    // commits not pushed to the upstream
	Behind   int    `json:"behind"`   // upstream commits not pulled, as of the last fetch
	Upstream bool   `json:"upstream"` // the branch has an upstream
	Error    string `json:"error,omitempty"`
}

// Pending reports whether the repo has uncommitted changes, unpushed commits or an error.
func (s RepoState) Pending() bool { return s.Error != "" || s.Changed > 0 || s.Ahead > 0 }

// State reads the state of the repo in dir (its top folder).
func State(dir string) RepoState {
	s := RepoState{Path: dir}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		s.Error = "not a git repo: " + dir
		return s
	}
	out, err := git(dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		s.Error = err.Error()
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
