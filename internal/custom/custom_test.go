package custom

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"aex/internal/plugin"
	"aex/internal/pty"
	"aex/internal/secrets"
	"aex/internal/settings"
	"aex/internal/ui"
)

func TestRun(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil && runtime.GOOS == "windows" {
		t.Skip("no PowerShell")
	} else if _, err := exec.LookPath("pwsh"); err != nil && runtime.GOOS != "windows" {
		t.Skip("no PowerShell")
	}
	dir := t.TempDir()
	settings.CustomToolsFile = filepath.Join(dir, "custom-tools.json")
	settings.Credentials = secrets.Memory("test", nil)
	src, err := os.ReadFile(filepath.Join("..", "adapter", "powershell", "testdata", "sample.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "Say Hi.ps1")
	if err := os.WriteFile(script, src, 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Add(script, "")
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "Say-Hi" || e.Adapter != "powershell" {
		t.Errorf("added %+v", e)
	}
	if _, err := Add(script, "other"); err == nil {
		t.Error("added the same script twice")
	}
	if err := CheckName("say-hi"); err == nil {
		t.Error("name taken ignoring case was accepted")
	}

	groups := Discover(nil)
	if len(groups) != 1 || groups[0].Name != "powershell" || Find(groups, "Say-Hi") != &groups[0].Sub[0] {
		t.Fatalf("groups = %+v", groups)
	}
	tools := groups[0].Sub
	if len(tools) != 1 || tools[0].Summary != "Greets someone." || !strings.Contains(tools[0].Warn, "not approved") {
		t.Fatalf("tools = %+v", tools)
	}
	run := tools[0].Run
	if err := run([]string{"-Name", "x"}); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Errorf("unapproved run: %v", err)
	}

	info := Inspect(e, true)
	if info.Git != nil || info.State != plugin.NotApproved || info.Err != nil {
		t.Errorf("info %+v", info)
	}
	key := filepath.Clean(script)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	settings.Credentials.Set("plugin-safe-sha256:"+key, info.Hash)

	out := capture(t, func() {
		if err := run([]string{"-Name", "bob smith", "-Loud", "-Items", "x,y"}); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(out, "Name=bob smith|Word=hi|Count=1|Loud=True|Polite=True|Items=2:x+y") {
		t.Errorf("output %q", out)
	}
	if err := run([]string{"-Word", "hi"}); err == nil || err.Error() != "missing -Name (see --help)" {
		t.Errorf("missing required: %v", err)
	}
	capture(t, func() {
		err := run([]string{"fail"})
		if exit, ok := errors.AsType[*plugin.ExitError](err); !ok || exit.Code != 3 {
			t.Errorf("exit: %v", err)
		}
	})

	if err := os.WriteFile(script, append(src, '#'), 0o644); err != nil {
		t.Fatal(err)
	}
	if i := Inspect(e, false); i.State != plugin.Changed {
		t.Errorf("state after change %v", i.State)
	}
	if _, err := Remove(e.Name); err != nil {
		t.Fatal(err)
	}
	if entries, _ := Load(); len(entries) != 0 {
		t.Errorf("left %+v", entries)
	}
}

// fakeRemote is the window showing a terminal: it types each answer once its question is on screen.
type fakeRemote struct {
	answers [][2]string // question, keys
	mu      sync.Mutex
	screen  strings.Builder
	input   io.Writer
	resized bool
	closed  bool
}

func (r *fakeRemote) OpenTerminal(input io.Writer, resize func(cols, rows int)) io.WriteCloser {
	r.input = input
	resize(80, 20)
	r.resized = true
	return r
}

func (r *fakeRemote) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.screen.Write(b)
	if len(r.answers) > 0 && strings.Contains(r.screen.String(), r.answers[0][0]) {
		io.WriteString(r.input, r.answers[0][1])
		r.answers = r.answers[1:]
	}
	return len(b), nil
}

func (r *fakeRemote) Close() error { r.closed = true; return nil }

func (r *fakeRemote) Input(ui.Field) (string, error)               { return "", nil }
func (r *fakeRemote) Confirm(string, bool) (bool, error)           { return false, nil }
func (r *fakeRemote) Choose(string, []ui.Option) (string, error)   { return "", nil }
func (r *fakeRemote) PickTool(string, []ui.Option) (string, error) { return "", nil }
func (r *fakeRemote) WaitKey()                                     {}
func (r *fakeRemote) ClearScreen()                                 {}

func TestTerminalInWindow(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil || !pty.Supported {
		t.Skip("no Windows PowerShell or ConPTY")
	}
	dir := t.TempDir()
	settings.CustomToolsFile = filepath.Join(dir, "custom-tools.json")
	settings.Credentials = secrets.Memory("test", nil)
	script := filepath.Join(dir, "ask.ps1")
	src := `$name = Read-Host 'Your name'
$pw = Read-Host -Prompt 'Password' -AsSecureString
$plain = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($pw))
"hello $name / $plain"
exit 5
`
	if err := os.WriteFile(script, append([]byte{0xEF, 0xBB, 0xBF}, src...), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Add(script, "ask")
	if err != nil {
		t.Fatal(err)
	}
	info := Inspect(e, false)
	if !info.Desc.Interactive {
		t.Fatal("PowerShell scripts should be interactive")
	}
	settings.Credentials.Set("plugin-safe-sha256:"+strings.ToLower(filepath.Clean(script)), info.Hash)

	remote := &fakeRemote{answers: [][2]string{{"Your name", "Юля O'Brien\r"}, {"Password", "s3cret\r"}}}
	ui.Remote = remote
	defer func() { ui.Remote = nil }()
	capture(t, func() {
		err := runner(e)(nil)
		if exit, ok := errors.AsType[*plugin.ExitError](err); !ok || exit.Code != 5 {
			t.Errorf("exit: %v", err)
		}
	})
	screen := remote.screen.String()
	if !strings.Contains(screen, "hello Юля O'Brien / s3cret") || strings.Count(screen, "s3cret") != 1 {
		t.Errorf("screen %q", screen)
	}
	if !remote.resized || !remote.closed || len(remote.answers) > 0 {
		t.Errorf("resized %v, closed %v, unanswered %v", remote.resized, remote.closed, remote.answers)
	}
}

// capture is what f prints to stdout.
func capture(t *testing.T, f func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = file, file
	f()
	os.Stdout, os.Stderr = stdout, stderr
	file.Close()
	b, _ := os.ReadFile(path)
	return string(b)
}
