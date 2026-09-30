package main

import (
	"debug/pe"
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"aex/internal/settings"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procAttachConsole         = kernel32.NewProc("AttachConsole")
)

// isWindowExe reports whether this exe was built with -H windowsgui (settings.WindowExeName):
// Windows starts it without a console, so it only opens the window.
func isWindowExe() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	f, err := pe.Open(exe)
	if err != nil {
		return false
	}
	defer f.Close()
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		return h.Subsystem == pe.IMAGE_SUBSYSTEM_WINDOWS_GUI
	case *pe.OptionalHeader32:
		return h.Subsystem == pe.IMAGE_SUBSYSTEM_WINDOWS_GUI
	}
	return false
}

// pointToCLI says, for a tool (or --help) given to the window exe, to run it with
// settings.CLIExeName instead: in the terminal it was started from, else in a message box. Exits 2.
func pointToCLI(args []string) {
	msg := fmt.Sprintf("%s opens the aex window. To run a tool in a terminal, use %s:\n  %s %s",
		settings.WindowExeName, settings.CLIExeName, strings.TrimSuffix(settings.CLIExeName, ".exe"),
		windows.ComposeCommandLine(args))
	const attachParentProcess = ^uint32(0)
	if ok, _, _ := procAttachConsole.Call(uintptr(attachParentProcess)); ok != 0 {
		if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			fmt.Fprintf(out, "\n%s\n", msg)
			out.Close()
		}
	} else {
		text, _ := windows.UTF16PtrFromString(msg)
		title, _ := windows.UTF16PtrFromString("aex")
		windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONINFORMATION)
	}
	os.Exit(2)
}

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
