package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const supported = true

// entry is aex.desktop in $XDG_CONFIG_HOME/autostart (default ~/.config/autostart), which desktop
// environments run when the user logs in.
func entry() (string, error) {
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		config = filepath.Join(home, ".config")
	}
	return filepath.Join(config, "autostart", "aex.desktop"), nil
}

func enabled() bool {
	p, err := entry()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
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

func enable(exe string, args []string) error {
	p, err := entry()
	if err != nil {
		return err
	}
	cmd := []string{execArg(exe)}
	for _, a := range args {
		cmd = append(cmd, execArg(a))
	}
	text := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=aex",
		"Comment=aex work tools",
		"Exec=" + strings.Join(cmd, " "),
		"X-GNOME-Autostart-enabled=true",
		"",
	}, "\n")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(text), 0o644)
}

func disable() error {
	p, err := entry()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
