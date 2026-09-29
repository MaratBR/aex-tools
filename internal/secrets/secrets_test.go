package secrets

import (
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func roundTrip(t *testing.T, s Store) {
	t.Helper()
	if v, err := s.Get(JiraToken); v != "" || err != nil {
		t.Fatalf("Get missing = %q, %v", v, err)
	}
	if err := s.Set(JiraToken, "abc"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(AEXTSession, "auth_token=x"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get(JiraToken); v != "abc" || err != nil {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if err := s.Delete(JiraToken); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(JiraToken); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
	if v, _ := s.Get(JiraToken); v != "" {
		t.Fatalf("Get after Delete = %q", v)
	}
	if v, _ := s.Get(AEXTSession); v != "auth_token=x" {
		t.Fatalf("other key = %q", v)
	}
}

func TestFileStore(t *testing.T) {
	s, reason := Open("file", "test", filepath.Join(t.TempDir(), "credentials.json"))
	if reason == "" {
		t.Error("file store without a reason")
	}
	roundTrip(t, s)
}

func TestKeyringStore(t *testing.T) {
	keyring.MockInit()
	s, reason := Open("", "test", filepath.Join(t.TempDir(), "credentials.json"))
	if reason != "" {
		t.Fatalf("fell back to file: %s", reason)
	}
	roundTrip(t, s)
}
