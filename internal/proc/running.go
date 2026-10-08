package proc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// process is a program running: the file it runs from, or only its file name when its path cannot
// be read (another user's process, an elevated one).
type process struct {
	path string
	name string
}

// Running reports whether a process runs the program at path: that file, or on macOS a program
// inside that .app. A process whose path cannot be read counts when its file name is the program's.
func Running(path string) (bool, error) {
	procs, err := processes()
	if err != nil {
		return false, err
	}
	path = filepath.Clean(path)
	app := runtime.GOOS == "darwin" && strings.HasSuffix(path, ".app")
	for _, p := range procs {
		switch {
		case p.path == "":
			if !app && samePath(p.name, filepath.Base(path)) {
				return true, nil
			}
		case app:
			if within(filepath.Clean(p.path), path) {
				return true, nil
			}
		case samePath(filepath.Clean(p.path), path):
			return true, nil
		}
	}
	return false, nil
}

// samePath compares paths as the file system does: ignoring case on Windows and macOS.
func samePath(a, b string) bool {
	if runtime.GOOS == "linux" {
		return a == b
	}
	return strings.EqualFold(a, b)
}

// within reports whether path is inside dir.
func within(path, dir string) bool {
	return len(path) > len(dir) && samePath(path[:len(dir)], dir) && os.IsPathSeparator(path[len(dir)])
}
