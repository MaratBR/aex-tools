// Shared by the git tools of cm-release (pull-all, prepare-release, merge-prod): they work on every
// CM repo at once, all kept in one folder (the reposDir setting).
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"aex/internal/ui"
)

// Branches the git tools use.
const (
	masterBranch  = "master"
	prodBranch    = "PROD"
	stagingBranch = "staging"
)

var releaseBranchRe = regexp.MustCompile(`^release/(.+)$`)

func releaseBranch(version string) string { return "release/" + version }
func releaseTag(version string) string    { return "v" + version }

type repo struct {
	name string
	path string
}

// repos are the configured repos, checked to be git repos. The repos folder is asked for (and
// saved) when not set yet.
func repos(c *config) ([]repo, error) {
	if c.ReposDir == "" {
		if err := askReposDir(c); err != nil {
			return nil, err
		}
		if err := c.save(); err != nil {
			return nil, err
		}
	}
	if len(c.Repos) == 0 {
		return nil, fmt.Errorf("no repos set in %s (see --settings of jira-handoff)", configFile())
	}
	fmt.Println(ui.Out.Cyan("Resolving repo paths..."))
	var rs []repo
	for _, name := range c.Repos {
		path := filepath.Join(c.ReposDir, name)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("repo path not found: %s", path)
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			return nil, fmt.Errorf("not a git repo: %s", path)
		}
		rs = append(rs, repo{name, path})
	}
	return rs, nil
}

// gitRepos gives aex the CM repos' folders (plugin.GitRepos), none while the repos folder is not
// set: the Git status widget's default list.
func gitRepos() (any, error) {
	c, err := loadConfig()
	if err != nil {
		return nil, err
	}
	paths := []string{}
	if c.ReposDir != "" {
		for _, name := range c.Repos {
			paths = append(paths, filepath.Join(c.ReposDir, name))
		}
	}
	return paths, nil
}

func askReposDir(c *config) error {
	dir, err := ui.Input(ui.Field{
		Title:       "Repos folder",
		Description: "Folder holding the CM repos: " + strings.Join(c.Repos, ", "),
		Hint:        ui.Hint{Kind: ui.HintFolder},
		Placeholder: c.ReposDir,
		Validate: func(s string) error {
			if s == "" {
				return errors.New("enter a folder")
			}
			if info, err := os.Stat(s); err != nil || !info.IsDir() {
				return errors.New("no such folder")
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	c.ReposDir, err = filepath.Abs(dir)
	return err
}

// git runs git in the repo and returns its output, trimmed; a failure includes the output.
func (r repo) git(args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", r.path}, args...)...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("'git %s' failed in %s: %v\n%s", strings.Join(args, " "), r.path, err, s)
	}
	return s, nil
}

func (r repo) branch() (string, error) { return r.git("rev-parse", "--abbrev-ref", "HEAD") }

func (r repo) dirty() (bool, error) {
	status, err := r.git("status", "--porcelain")
	return status != "", err
}

func printSection(r repo)  { fmt.Printf("\n%s\n", ui.Out.Bold(ui.Out.Cyan("=== "+r.name+" ==="))) }
func printStep(msg string) { fmt.Println(ui.Out.Cyan(msg)) }
func printDone(msg string) { fmt.Println(ui.Out.Green(msg)) }

// waitForClean returns once no repo has uncommitted changes, asking to fix them and recheck.
func waitForClean(rs []repo) error {
	printStep("Checking working trees are clean...")
	for {
		var dirty []string
		for _, r := range rs {
			d, err := r.dirty()
			if err != nil {
				return err
			}
			if d {
				dirty = append(dirty, r.path)
			}
		}
		if len(dirty) == 0 {
			return nil
		}
		fmt.Println(ui.Out.Yellow("\nUnstaged/uncommitted changes found in:"))
		for _, d := range dirty {
			fmt.Println(ui.Out.Yellow("  - " + d))
		}
		if !ui.IsInteractive() {
			return errors.New("commit, stash, or discard these changes first")
		}
		again, err := ui.Confirm("Commit, stash, or discard these changes, then recheck?", true)
		if err != nil {
			return err
		}
		if !again {
			return errors.New("cancelled")
		}
	}
}

type repoBranch struct {
	repo
	branch string
}

func branches(rs []repo) ([]repoBranch, error) {
	var bs []repoBranch
	for _, r := range rs {
		b, err := r.branch()
		if err != nil {
			return nil, err
		}
		bs = append(bs, repoBranch{r, b})
	}
	return bs, nil
}

func printBranches(title string, bs []repoBranch) {
	fmt.Println(ui.Out.Yellow("\n" + title))
	for _, b := range bs {
		fmt.Println(ui.Out.Yellow(fmt.Sprintf("  - %s: %s", b.path, b.branch)))
	}
}

// Results of releaseVersion.
var (
	errNoneOnRelease = errors.New("no repo is on a release branch")
	errNotAllRelease = errors.New("not all repos are on a 'release/VERSION' branch")
	errMixedRelease  = errors.New("repos are on different release branches")
)

// releaseVersion is the VERSION of the release/VERSION branch every repo is on.
func releaseVersion(branches []string) (string, error) {
	var versions []string
	for _, b := range branches {
		if m := releaseBranchRe.FindStringSubmatch(b); m != nil {
			versions = append(versions, m[1])
		}
	}
	switch {
	case len(versions) == 0:
		return "", errNoneOnRelease
	case len(versions) != len(branches):
		return "", errNotAllRelease
	}
	if versions = slices.Compact(slices.Sorted(slices.Values(versions))); len(versions) != 1 {
		return "", errMixedRelease
	}
	return versions[0], nil
}

func branchNames(bs []repoBranch) []string {
	names := make([]string, len(bs))
	for i, b := range bs {
		names[i] = b.branch
	}
	return names
}

// pressAnyKey waits for a key press on an interactive console.
func pressAnyKey(what string) {
	if !ui.IsInteractive() {
		return
	}
	fmt.Fprint(os.Stderr, ui.Err.Dim("Press any key to "+what+"..."))
	ui.WaitKey()
	fmt.Fprintln(os.Stderr)
}
