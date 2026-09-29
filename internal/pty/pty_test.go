//go:build windows

package pty

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// screen collects what the program shows, stripped of VT sequences enough to search it.
type screen struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *screen) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(b)
}

func (s *screen) waitFor(t *testing.T, text string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got := s.buf.String()
		s.mu.Unlock()
		if strings.Contains(got, text) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Fatalf("no %q on screen:\n%q", text, s.buf.String())
}

func TestReadHost(t *testing.T) {
	if !Supported {
		t.Skip("no ConPTY")
	}
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("no PowerShell")
	}
	script := filepath.Join(t.TempDir(), "ask.ps1")
	src := `$n = Read-Host 'Your name'
$p = Read-Host 'Password' -AsSecureString
$c = $Host.UI.PromptForChoice('Pick', 'Which one?', @('&Apple', '&Banana'), 0)
"got [$n] len $($p.Length) choice $c"
exit 4
`
	// With a BOM, as Windows PowerShell needs to read a UTF-8 script right.
	if err := os.WriteFile(script, append([]byte{0xEF, 0xBB, 0xBF}, src...), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "& '"+script+"'; exit $LASTEXITCODE")
	p, err := Start(cmd, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var s screen
	done := make(chan struct{})
	go func() { io.Copy(&s, p); close(done) }()

	s.waitFor(t, "Your name")
	io.WriteString(p, "Юля\r")
	s.waitFor(t, "Password")
	io.WriteString(p, "abc\r")
	s.waitFor(t, "Which one?")
	io.WriteString(p, "b\r")
	code, err := p.Wait()
	if err != nil || code != 4 {
		t.Errorf("exit %d, %v", code, err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("screen did not end after exit")
	}
	s.waitFor(t, "got [Юля] len 3 choice 1")
	if strings.Contains(s.buf.String(), "abc") {
		t.Error("secure input was echoed")
	}
}
