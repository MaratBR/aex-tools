package autostart

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const supported = true

const label = "com.aexsoft.aex"

// plist is the LaunchAgent in ~/Library/LaunchAgents, which launchd runs when the user logs in.
func plist() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func enabled() bool {
	p, err := plist()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func escape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func enable(exe string, args []string) error {
	p, err := plist()
	if err != nil {
		return err
	}
	var program strings.Builder
	for _, a := range append([]string{exe}, args...) {
		program.WriteString("\t\t<string>" + escape(a) + "</string>\n")
	}
	text := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
` + program.String() + `	</array>
	<key>RunAtLoad</key><true/>
</dict>
</plist>
`
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(text), 0o644)
}

func disable() error {
	p, err := plist()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
