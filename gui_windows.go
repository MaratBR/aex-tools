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
