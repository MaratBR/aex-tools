package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
)

// hideOwnConsole detaches from the console when it was made for this process alone (started from
// a shortcut or Explorer), which closes its window. A console shared with a shell is kept.
func hideOwnConsole() {
	var ids [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	if n == 1 {
		procFreeConsole.Call()
	}
}

// saveConsole records the modes of the console the window was started from (echo, line input, VT
// processing) and returns a function that sets them back, so the shell gets its terminal back as
// it was, whatever a tool or plugin did to it meanwhile.
func saveConsole() (restore func()) {
	type saved struct {
		h    windows.Handle
		mode uint32
	}
	var modes []saved
	for _, std := range []uint32{windows.STD_INPUT_HANDLE, windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
		h, err := windows.GetStdHandle(std)
		if err != nil {
			continue
		}
		var mode uint32
		if windows.GetConsoleMode(h, &mode) == nil {
			modes = append(modes, saved{h, mode})
		}
	}
	return func() {
		for _, s := range modes {
			windows.SetConsoleMode(s.h, s.mode)
		}
		// Keys typed into the terminal while the window was open would otherwise reach the shell.
		if len(modes) > 0 {
			if h, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE); err == nil {
				windows.FlushConsoleInputBuffer(h)
			}
		}
	}
}
