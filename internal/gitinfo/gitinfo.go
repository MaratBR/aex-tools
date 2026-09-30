// Package gitinfo tells where a file stands in the git repo it is in: the repo, its branch and
// commit, the last commit that changed the file and whether it has changes not committed; and a
// repo's state (State): its branch, changes and commits to push or pull.
package gitinfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aex/internal/proc"
)

const timeout = 5 * time.Second

// Info is a file's place in its git repo.
type Info struct {
	Root   string // the repo's top folder
	Remote string // URL of origin (else the first remote), credentials removed; "" for none
	Branch string // "" when HEAD is detached
	Head   string // HEAD's short hash; "" before the first commit
	File   string // the file, relative to Root
	Last   *Commit
	// Status is the file's porcelain status (git status): "" when committed with no changes, "??"
	// untracked, "!!" ignored, else two letters for the index and the work tree.
	Status string
	// Added and Deleted are lines changed since HEAD, staged or not.
	Added, Deleted int
}

// Commit is the last commit that changed the file.
type Commit struct {
	Short, Date, Author, Subject string
}

// Of is the git info for the file at path; nil (no error) when it is not in a git repo or git is not
// installed.
func Of(path string) (*Info, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(abs)
	root, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return nil, nil
		}
		return nil, err
	}
	i := &Info{Root: filepath.FromSlash(root)}
	if i.File, err = filepath.Rel(i.Root, abs); err != nil {
		// e.g. a folder reached through a link: git names files from its root.
		i.File = filepath.Base(abs)
	}
	file := filepath.ToSlash(i.File)
	i.Head, _ = git(dir, "rev-parse", "--short", "HEAD")
	if b, err := git(dir, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
		i.Branch = b
	}
	i.Remote = remote(dir)
	status, err := git(i.Root, "status", "--porcelain=v1", "--ignored", "--", file)
	if err != nil {
		return nil, err
	}
	if len(status) >= 2 {
		i.Status = status[:2]
	}
	if i.Head != "" {
		if log, err := git(i.Root, "log", "-1", "--format=%h%x00%cs%x00%an%x00%s", "--", file); err == nil && log != "" {
			if f := strings.SplitN(log, "\x00", 4); len(f) == 4 {
				i.Last = &Commit{f[0], f[1], f[2], f[3]}
			}
		}
		if i.Status != "" && i.Status != "??" && i.Status != "!!" {
			if stat, err := git(i.Root, "diff", "--numstat", "HEAD", "--", file); err == nil {
				f := strings.Fields(stat)
				if len(f) >= 2 {
					i.Added, _ = strconv.Atoi(f[0])
					i.Deleted, _ = strconv.Atoi(f[1])
				}
			}
		}
	}
	return i, nil
}

// remote is origin's URL, else the first remote's, without credentials.
func remote(dir string) string {
	u, err := git(dir, "remote", "get-url", "origin")
	if err != nil {
		names, err := git(dir, "remote")
		if err != nil || names == "" {
			return ""
		}
		if u, err = git(dir, "remote", "get-url", strings.Fields(names)[0]); err != nil {
			return ""
		}
	}
	if parsed, err := url.Parse(u); err == nil && parsed.User != nil {
		parsed.User = nil
		return parsed.String()
	}
	return u
}

// Pending reports whether the file has changes that are not committed (untracked and ignored
// files are nothing but).
func (i *Info) Pending() bool { return i.Status != "" }

// Change describes the file's changes not committed, "" when there are none.
func (i *Info) Change() string {
	lines := ""
	if i.Added+i.Deleted > 0 {
		lines = fmt.Sprintf(" (+%d −%d lines)", i.Added, i.Deleted)
	}
	switch {
	case i.Status == "":
		return ""
	case i.Status == "??":
		return "not committed: untracked"
	case i.Status == "!!":
		return "not committed: ignored by git"
	case i.Status[0] == 'A':
		return "not committed: added, never committed" + lines
	case i.Status[0] != ' ' && i.Status[1] != ' ':
		return "changes not committed, some staged" + lines
	case i.Status[0] != ' ':
		return "changes staged, not committed" + lines
	}
	return "changes not committed" + lines
}

// Lines describe the info as label and value pairs, for listings.
func (i *Info) Lines() [][2]string {
	repo := i.Root
	if i.Remote != "" {
		repo += " (" + i.Remote + ")"
	}
	head := i.Head
	switch {
	case head == "":
		head = "no commits yet"
	case i.Branch != "":
		head = i.Branch + " @ " + head
	default:
		head += " (detached HEAD)"
	}
	lines := [][2]string{{"Repo", repo}, {"Commit", head}}
	if i.Last != nil {
		lines = append(lines, [2]string{"Changed", fmt.Sprintf("%s %s %s: %s", i.Last.Short, i.Last.Date, i.Last.Author, i.Last.Subject)})
	}
	change := i.Change()
	if change == "" {
		change = "none, same as committed"
	}
	return append(lines, [2]string{"Pending", change})
}

// Short is a one-line summary: repo folder, branch and commit, and pending changes.
func (i *Info) Short() string {
	s := filepath.Base(i.Root)
	if i.Branch != "" {
		s += " " + i.Branch
	}
	if i.Head != "" {
		s += " @ " + i.Head
	}
	if c := i.Change(); c != "" {
		s += " · " + c
	}
	return s
}

func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	// Only reading: status must not take the index lock another git may be holding.
	cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	proc.HideConsole(cmd)
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New("git " + args[0] + ": " + msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimRight(out.String(), "\r\n"), nil
}
