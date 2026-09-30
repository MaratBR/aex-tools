package webui

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"aex/internal/gitclient"
	"aex/internal/gitinfo"
	"aex/internal/plugin"
)

// The Git status widget's data: the state of the git repos it watches (its placement's settings),
// from what git knows locally (no fetch), so it is quick enough to load every few seconds; the git
// client found on the device; and the default list of repos, from a plugin that provides one.

const maxGitRepos = 50

// GitStatusReply is the gitStatus API's result.
type GitStatusReply struct {
	Repos []gitinfo.RepoState `json:"repos"`
	// Clean is true when no repo has changes, unpushed commits or an error.
	Clean bool `json:"clean"`
	// GitClient is the git client gitOpenClient starts, empty when none is installed.
	GitClient string `json:"gitClient,omitempty"`
}

// gitStatusAPI gives the state of args.repos (folders).
func gitStatusAPI(args map[string]any) (any, error) {
	dirs, err := gitRepoArgs(args)
	if err != nil {
		return nil, err
	}
	reply := GitStatusReply{Repos: make([]gitinfo.RepoState, len(dirs)), Clean: true}
	reply.GitClient, _, _ = gitclient.Find()
	var wg sync.WaitGroup
	for i, dir := range dirs {
		wg.Go(func() { reply.Repos[i] = gitinfo.State(dir) })
	}
	wg.Wait()
	for _, r := range reply.Repos {
		if r.Pending() {
			reply.Clean = false
		}
	}
	return reply, nil
}

func gitRepoArgs(args map[string]any) ([]string, error) {
	list, _ := args["repos"].([]any)
	if len(list) > maxGitRepos {
		return nil, errors.New("too many repos")
	}
	dirs := []string{}
	for _, v := range list {
		dir, _ := v.(string)
		if dir = strings.TrimSpace(dir); dir == "" || !filepath.IsAbs(dir) {
			return nil, errors.New("repos must be full folder paths")
		}
		dirs = append(dirs, filepath.Clean(dir))
	}
	return dirs, nil
}

// GitClientReply is the gitClient API's result: the git client found and its icon.
type GitClientReply struct {
	Name string `json:"name,omitempty"` // empty when none is installed
	Icon string `json:"icon,omitempty"` // data:image/png URL; empty when it cannot be read
}

func gitClientAPI(map[string]any) (any, error) {
	name, path, ok := gitclient.Find()
	if !ok {
		return GitClientReply{}, nil
	}
	return GitClientReply{Name: name, Icon: gitclient.Icon(path)}, nil
}

func gitOpenClientAPI(map[string]any) (any, error) { return nil, gitclient.Open() }

// GitDefaultReposReply is the gitDefaultRepos API's result.
type GitDefaultReposReply struct {
	Repos  []string `json:"repos"`
	Plugin string   `json:"plugin,omitempty"` // the plugin they come from
}

// gitDefaultReposAPI gives the repos a new Git status widget starts with: those of the first
// approved plugin that provides some (plugin.GitRepos), such as cm-release's; none otherwise.
func gitDefaultReposAPI(map[string]any) (any, error) {
	reply := GitDefaultReposReply{Repos: []string{}}
	plugins, err := plugin.List()
	if err != nil {
		return reply, nil
	}
	for _, p := range plugins {
		if p.State != plugin.Safe {
			continue
		}
		out, err := plugin.Provided(p, plugin.GitRepos)
		if err != nil {
			continue
		}
		var repos []string
		if json.Unmarshal(out, &repos) != nil || len(repos) == 0 {
			continue
		}
		return GitDefaultReposReply{Repos: repos, Plugin: p.Name}, nil
	}
	return reply, nil
}
