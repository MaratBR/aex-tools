package gitclient

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestIcon reads the icon of an exe that is always there.
func TestIcon(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	notepad := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	if img, err := readIcon(notepad, IconSize); err != nil || img.Bounds().Dx() != IconSize {
		t.Errorf("notepad: %v, %v", img, err)
	}
	if url := Icon(notepad); !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("notepad: got %.40q", url)
	}
	if url := Icon(`C:\no\such.exe`); url != "" {
		t.Errorf("missing file: got %.40q", url)
	}
}
