package shortcut

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	supported = true
	where     = "app menu"
)

// path is aex.desktop in $XDG_DATA_HOME/applications (default ~/.local/share/applications), which
// desktop environments list in their app menu.
func path() (string, error) {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "applications", "aex.desktop"), nil
}

// execArg quotes an Exec= argument per the Desktop Entry spec.
func execArg(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	if !strings.ContainsAny(s, " \t\n\"'\\><~|&;$*?#()`") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	// The spec's quoting is itself inside a string value, which escapes backslashes once more.
	return strings.ReplaceAll(`"`+r.Replace(s)+`"`, `\`, `\\`)
}

func create(file, exe string, args []string) error {
	cmd := []string{execArg(exe)}
	for _, a := range args {
		cmd = append(cmd, execArg(a))
	}
	entry := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=aex",
		"Comment=aex work tools",
		"Exec=" + strings.Join(cmd, " "),
		"Icon=utilities-terminal",
		"Terminal=true",
		"Categories=Utility;",
		"",
	}, "\n")
	return os.WriteFile(file, []byte(entry), 0o755)
}

func remove(file string) error {
	if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
