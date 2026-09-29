package tool

import (
	"os"
	"os/exec"
	"runtime"
)

// OpenFolder creates dir if missing and opens it in the file manager.
func OpenFolder(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	opener := "xdg-open"
	switch runtime.GOOS {
	case "windows":
		opener = "explorer.exe"
	case "darwin":
		opener = "open"
	}
	// Not waited for: the file manager outlives this process (explorer.exe also exits non-zero on success).
	if cmd := exec.Command(opener, dir); cmd.Start() == nil {
		cmd.Process.Release()
	}
	return nil
}

// OpenURL opens an http(s) URL in the default browser, without waiting for it.
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Not "start": cmd.exe would split the URL at its &s.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
