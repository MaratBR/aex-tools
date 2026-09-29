//go:build !windows && !darwin && !linux

package shortcut

const (
	supported = false
	where     = "app launcher"
)

func path() (string, error)                 { return "", ErrUnsupported }
func create(string, string, []string) error { return ErrUnsupported }
func remove(string) error                   { return ErrUnsupported }
