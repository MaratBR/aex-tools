//go:build !windows

// Package pty runs a console program in a pseudo-console; only on Windows (ConPTY) so far.
package pty

import (
	"errors"
	"os/exec"
)

// Supported reports whether pseudo-consoles work here: not yet outside Windows.
const Supported = false

// PTY is a program running in a pseudo-console.
type PTY struct{}

func Start(*exec.Cmd, int, int) (*PTY, error) {
	return nil, errors.New("pseudo-consoles are Windows only")
}

func (*PTY) Read([]byte) (int, error)  { return 0, errors.ErrUnsupported }
func (*PTY) Write([]byte) (int, error) { return 0, errors.ErrUnsupported }
func (*PTY) Resize(int, int) error     { return errors.ErrUnsupported }
func (*PTY) Wait() (int, error)        { return 0, errors.ErrUnsupported }
func (*PTY) Close()                    {}
