//go:build !windows && !linux

package proc

import (
	"os/exec"
	"strings"
)

// processes lists the processes running, each with the path of its program (ps: on macOS the
// command is the path it was started from).
func processes() ([]process, error) {
	b, err := exec.Command("ps", "-axo", "comm=").Output()
	if err != nil {
		return nil, err
	}
	var out []process
	for line := range strings.Lines(string(b)) {
		if p := strings.TrimSpace(line); p != "" {
			if strings.HasPrefix(p, "/") {
				out = append(out, process{path: p})
			} else {
				out = append(out, process{name: p})
			}
		}
	}
	return out, nil
}
