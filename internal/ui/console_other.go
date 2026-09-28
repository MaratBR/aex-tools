//go:build !windows

package ui

import "os"

// Unix terminals interpret ANSI escapes already.
func enableVT(*os.File) bool { return true }
