//go:build !windows && !darwin

package gitclient

import (
	"errors"
	"image"
)

// readIcon: no git client is looked for here (Find), so there is no icon either.
func readIcon(string, int) (image.Image, error) { return nil, errors.New("no icons on this platform") }
