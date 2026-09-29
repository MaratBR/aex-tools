package shortcut

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	supported = true
	where     = "Applications folder"
)

const bundleID = "com.aexsoft.aex"

// path is aex.app in ~/Applications, which Launchpad and Spotlight list.
func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Applications", "aex.app"), nil
}

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>aex</string>
	<key>CFBundleIdentifier</key><string>` + bundleID + `</string>
	<key>CFBundleName</key><string>aex</string>
	<key>CFBundlePackageType</key><string>APPL</string>
</dict>
</plist>
`

// launcher opens the .command next to it in Terminal: aex is a terminal app.
const launcher = `#!/bin/sh
exec open -a Terminal "$(dirname "$0")/../Resources/aex.command"
`

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// create writes a minimal app bundle: its executable opens Resources/aex.command, which runs exe.
func create(app, exe string, args []string) error {
	if err := remove(app); err != nil {
		return err
	}
	command := []string{"exec", shellQuote(exe)}
	for _, a := range args {
		command = append(command, shellQuote(a))
	}
	files := []struct {
		name, text string
		mode       os.FileMode
	}{
		{"Contents/Info.plist", infoPlist, 0o644},
		{"Contents/MacOS/aex", launcher, 0o755},
		{"Contents/Resources/aex.command", "#!/bin/sh\n" + strings.Join(command, " ") + "\n", 0o755},
	}
	for _, f := range files {
		file := filepath.Join(app, f.name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(file, []byte(f.text), f.mode); err != nil {
			return err
		}
	}
	return nil
}

// remove deletes the bundle, but only one this package made (by its bundle id).
func remove(app string) error {
	data, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Stat(app); errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil || !bytes.Contains(data, []byte(bundleID)) {
		return fmt.Errorf("%s was not made by aex, remove it yourself", app)
	}
	return os.RemoveAll(app)
}
