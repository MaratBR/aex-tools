// Package appicon reads the icon of an app: on Windows from an exe's resources, on macOS from an
// .app bundle. For the git client button of the Git status widget and the IDEs of the Composer.
package appicon

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"sync"
)

// Size is the icon's width and height in pixels.
const Size = 64

var icons sync.Map // path: data URL, or "" when it has none

// DataURL is the icon of the app at path as a PNG data: URL, "" when it cannot be read. Kept for
// the path once read.
func DataURL(path string) string {
	if url, ok := icons.Load(path); ok {
		return url.(string)
	}
	url := ""
	if img, err := readIcon(path, Size); err == nil {
		var b bytes.Buffer
		if png.Encode(&b, img) == nil {
			url = "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
		}
	}
	icons.Store(path, url)
	return url
}
