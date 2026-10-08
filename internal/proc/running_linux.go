package proc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// processes lists the processes running, each with the path of its program when it can be read
// (/proc/<pid>/exe; else its name from /proc/<pid>/comm, cut to 15 characters by Linux).
func processes() ([]process, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var out []process
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
			out = append(out, process{path: strings.TrimSuffix(exe, " (deleted)")})
		} else if comm, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
			out = append(out, process{name: strings.TrimSpace(string(comm))})
		}
	}
	return out, nil
}
