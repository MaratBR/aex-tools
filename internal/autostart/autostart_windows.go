package autostart

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const supported = true

// runKey is the current user's Run key: Windows runs each of its values when the user logs in.
const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "aex"
)

func enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

func enable(exe string, args []string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	command := []string{windows.EscapeArg(exe)}
	for _, a := range args {
		command = append(command, windows.EscapeArg(a))
	}
	return k.SetStringValue(runValue, strings.Join(command, " "))
}

func disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
