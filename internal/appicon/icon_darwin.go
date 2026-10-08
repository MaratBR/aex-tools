package appicon

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// readIcon is the icon of the app at path (its CFBundleIconFile), turned into a PNG by sips.
func readIcon(path string, size int) (image.Image, error) {
	out, err := exec.Command("plutil", "-extract", "CFBundleIconFile", "raw", "-o", "-", filepath.Join(path, "Contents", "Info.plist")).Output()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(string(out))
	if filepath.Ext(name) == "" {
		name += ".icns"
	}
	tmp, err := os.CreateTemp("", "aex-icon-*.png")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	icns := filepath.Join(path, "Contents", "Resources", name)
	if err := exec.Command("sips", "-s", "format", "png", "-Z", strconv.Itoa(size), icns, "--out", tmp.Name()).Run(); err != nil {
		return nil, err
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}
