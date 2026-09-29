//go:build !windows

package plugin

import "os"

// openLocked opens path for reading. Unlike on Windows, the file is not locked against changes.
func openLocked(path string) (*os.File, error) { return os.Open(path) }
