package appicon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestIcon reads the icon of an exe that is always there.
func TestDataURL(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	notepad := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	if img, err := readIcon(notepad, Size); err != nil || img.Bounds().Dx() != Size {
		t.Errorf("notepad: %v, %v", img, err)
	}
	if url := DataURL(notepad); !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("notepad: got %.40q", url)
	}
	if url := DataURL(`C:\no\such.exe`); url != "" {
		t.Errorf("missing file: got %.40q", url)
	}
}
