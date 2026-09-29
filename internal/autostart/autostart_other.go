//go:build !windows && !darwin && !linux

package autostart

const supported = false

func enabled() bool                 { return false }
func enable(string, []string) error { return ErrUnsupported }
func disable() error                { return ErrUnsupported }
