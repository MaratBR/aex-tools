package proc

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// processes lists the processes running, each with the path of its exe when it can be read.
func processes() ([]process, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []process
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out = append(out, process{path: exePath(e.ProcessID), name: windows.UTF16ToString(e.ExeFile[:])})
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return nil, err
	}
	return out, nil
}

// exePath is the path of the exe process pid runs, empty when it cannot be read.
func exePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
