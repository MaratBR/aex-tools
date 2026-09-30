package about

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aex/internal/httpx"
)

// Branch is the branch of Repo a build is compared with.
const Branch = "master"

// githubAPI is the GitHub API the comparison asks (a test server in tests).
var githubAPI = "https://api.github.com/repos/" + strings.TrimPrefix(Repo, "https://github.com/")

var (
	// ErrNoCommit: the build does not know its commit (e.g. go run).
	ErrNoCommit = errors.New("the build does not know its commit")
	// ErrNotOnGitHub: GitHub does not have the build's commit (not pushed, or rebased away).
	ErrNotOnGitHub = errors.New("GitHub does not have this commit")
)

// Behind is how the build's commit compares with Branch on GitHub.
type Behind struct {
	Commit string `json:"commit"`
	Branch string `json:"branch"`
	// Behind is how many commits Branch has that the build does not; Ahead the other way round.
	Behind     int       `json:"behind"`
	Ahead      int       `json:"ahead"`
	CompareURL string    `json:"compareURL"`
	CheckedAt  time.Time `json:"checkedAt"`
	// Error is why the check failed, when it did.
	Error string `json:"error,omitempty"`
}

// How long a check is kept before GitHub is asked again: an answer, a failed check, and the least
// time between checks asked for on purpose (Check now).
const (
	behindFresh      = 6 * time.Hour
	behindRetry      = time.Hour
	behindForceAfter = time.Minute
)

// BehindFile is the file in the data folder where the last check is kept.
const BehindFile = "update-check.json"

var behindMu sync.Mutex

// CachedBehind gives the last check kept in dir (the data folder) while it is fresh for this build's
// commit, else asks GitHub and keeps the answer, failed or not. force asks GitHub unless the last
// check is under a minute old. Calls at the same time share one check.
func CachedBehind(ctx context.Context, dir string, force bool) Behind {
	return cachedBehind(ctx, dir, Info().Commit, force, time.Now())
}

func cachedBehind(ctx context.Context, dir, commit string, force bool, now time.Time) Behind {
	if commit == "" {
		return Behind{Branch: Branch, Error: ErrNoCommit.Error()}
	}
	behindMu.Lock()
	defer behindMu.Unlock()
	path := filepath.Join(dir, BehindFile)
	var last Behind
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &last) == nil && last.Commit == commit && last.Branch == Branch {
		age := now.Sub(last.CheckedAt)
		fresh := behindFresh
		if last.Error != "" {
			fresh = behindRetry
		}
		if force {
			fresh = behindForceAfter
		}
		if age >= 0 && age < fresh {
			return last
		}
	}
	b, err := compare(ctx, commit)
	b.CheckedAt = now
	if err != nil {
		b.Error = err.Error()
	}
	if data, err := json.MarshalIndent(b, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
	return b
}

// CheckBehind asks GitHub how many commits the build's commit is behind Branch.
func CheckBehind(ctx context.Context) (Behind, error) {
	return compare(ctx, Info().Commit)
}

func compare(ctx context.Context, commit string) (Behind, error) {
	b := Behind{Commit: commit, Branch: Branch, CheckedAt: time.Now()}
	if commit == "" {
		return b, ErrNoCommit
	}
	req, err := http.NewRequestWithContext(ctx, "GET", githubAPI+"/compare/"+commit+"..."+Branch, nil)
	if err != nil {
		return b, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := httpx.Client.Do(req)
	if err != nil {
		return b, errors.New("GitHub could not be reached")
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	switch {
	case res.StatusCode == http.StatusNotFound:
		return b, ErrNotOnGitHub
	case res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusTooManyRequests:
		return b, errors.New("GitHub: too many requests, try again later")
	case res.StatusCode != http.StatusOK:
		return b, fmt.Errorf("GitHub: HTTP %d", res.StatusCode)
	}
	// The compare is commit...Branch: ahead_by counts Branch's commits the build lacks.
	var data struct {
		AheadBy  *int   `json:"ahead_by"`
		BehindBy *int   `json:"behind_by"`
		HTMLURL  string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &data); err != nil || data.AheadBy == nil || data.BehindBy == nil {
		return b, errors.New("GitHub: unexpected response")
	}
	b.Behind, b.Ahead, b.CompareURL = *data.AheadBy, *data.BehindBy, data.HTMLURL
	return b, nil
}
