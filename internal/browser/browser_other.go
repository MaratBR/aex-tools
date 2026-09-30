//go:build !windows

package browser

import (
	"os/exec"
	"runtime"
)

func allowForeground() {}

func launch(b *Browser, address string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		for _, app := range b.Mac {
			// open -a fails at once when there is no such app.
			if exec.Command("open", "-a", app, address).Run() == nil {
				return nil
			}
		}
		return ErrNotFound
	}
	for _, name := range b.Linux {
		if path, err := exec.LookPath(name); err == nil {
			cmd = exec.Command(path, address)
			break
		}
	}
	if cmd == nil {
		return ErrNotFound
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
