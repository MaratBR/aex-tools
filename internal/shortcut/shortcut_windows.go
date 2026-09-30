package shortcut

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"aex/internal/proc"
)

const (
	supported = true
	where     = "Start menu"
)

// path is aex.lnk in the current user's Start menu Programs folder.
func path() (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "aex.lnk"), nil
}

// createScript makes the .lnk through the WScript.Shell COM object. Values come in through
// environment variables, so paths need no quoting.
const createScript = `$ErrorActionPreference = 'Stop'
$s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:AEX_LNK)
$s.TargetPath = $env:AEX_TARGET
$s.Arguments = $env:AEX_ARGS
$s.WorkingDirectory = $env:USERPROFILE
$s.IconLocation = "$env:AEX_TARGET,0"
$s.Description = 'aex work tools'
$s.Save()`

func create(lnk, exe string, args []string) error {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = windows.EscapeArg(a)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", createScript)
	proc.HideConsole(cmd)
	cmd.Env = append(os.Environ(), "AEX_LNK="+lnk, "AEX_TARGET="+exe, "AEX_ARGS="+strings.Join(quoted, " "))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating %s: %v: %s", lnk, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func remove(lnk string) error {
	if err := os.Remove(lnk); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
