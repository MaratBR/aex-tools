package about

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/compare/abc..." + Branch:
			w.Write([]byte(`{"ahead_by": 3, "behind_by": 1, "html_url": "https://github.com/x/compare"}`))
		case "/compare/gone..." + Branch:
			http.NotFound(w, r)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()
	old := githubAPI
	githubAPI = srv.URL
	defer func() { githubAPI = old }()

	b, err := compare(context.Background(), "abc")
	if err != nil || b.Behind != 3 || b.Ahead != 1 || b.CompareURL != "https://github.com/x/compare" {
		t.Errorf("abc: %+v, %v", b, err)
	}
	if _, err := compare(context.Background(), "gone"); !errors.Is(err, ErrNotOnGitHub) {
		t.Errorf("gone: %v", err)
	}
	if _, err := compare(context.Background(), ""); !errors.Is(err, ErrNoCommit) {
		t.Errorf("no commit: %v", err)
	}
	if _, err := compare(context.Background(), "limited"); err == nil {
		t.Error("limited: no error")
	}
}

func TestCachedBehind(t *testing.T) {
	calls := 0
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte(`{"ahead_by": 2, "behind_by": 0, "html_url": "u"}`))
	}))
	defer srv.Close()
	old := githubAPI
	githubAPI = srv.URL
	defer func() { githubAPI = old }()
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Now()

	if b := cachedBehind(ctx, dir, "abc", false, t0); b.Behind != 2 || b.Error != "" || calls != 1 {
		t.Fatalf("first: %+v, calls %d", b, calls)
	}
	if b := cachedBehind(ctx, dir, "abc", false, t0.Add(5*time.Hour)); b.Behind != 2 || calls != 1 {
		t.Errorf("fresh: %+v, calls %d", b, calls)
	}
	if cachedBehind(ctx, dir, "abc", true, t0.Add(30*time.Second)); calls != 1 {
		t.Errorf("forced too soon: calls %d", calls)
	}
	if cachedBehind(ctx, dir, "abc", true, t0.Add(2*time.Minute)); calls != 2 {
		t.Errorf("forced: calls %d", calls)
	}
	if cachedBehind(ctx, dir, "def", false, t0.Add(3*time.Minute)); calls != 3 {
		t.Errorf("other commit: calls %d", calls)
	}
	fail = true
	t1 := t0.Add(10 * time.Hour)
	if b := cachedBehind(ctx, dir, "def", false, t1); b.Error == "" || calls != 4 {
		t.Errorf("stale: %+v, calls %d", b, calls)
	}
	if b := cachedBehind(ctx, dir, "def", false, t1.Add(30*time.Minute)); b.Error == "" || calls != 4 {
		t.Errorf("failed, kept: %+v, calls %d", b, calls)
	}
	if cachedBehind(ctx, dir, "def", false, t1.Add(2*time.Hour)); calls != 5 {
		t.Errorf("failed, retried: calls %d", calls)
	}
	if b := cachedBehind(ctx, dir, "", false, t1); b.Error == "" || calls != 5 {
		t.Errorf("no commit: %+v", b)
	}
}
