// Package pty runs a console program in a pseudo-console (Windows ConPTY): the program sees a real
// terminal (prompts, hidden input, colors, cursor moves all work), while its screen comes out as VT
// text to read and what is written in goes to it as typed keys. The window shows it with a
// terminal view.
package pty

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Supported reports whether pseudo-consoles work here (Windows 10 1809+).
var Supported = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find() == nil

// PTY is a program running in a pseudo-console. Read its screen (VT, UTF-8) until io.EOF, which
// comes once it exited and Wait returned; Write sends keys ("\r" is Enter).
type PTY struct {
	console windows.Handle
	in      *os.File // keys to the program
	out     *os.File // its screen
	process windows.Handle

	closeOnce sync.Once
}

// Start starts cmd (its Path, Args, Env and Dir; nothing else of it is used) in a pseudo-console of
// cols × rows.
func Start(cmd *exec.Cmd, cols, rows int) (*PTY, error) {
	if !Supported {
		return nil, errors.New("pseudo-consoles need Windows 10 1809 or later")
	}
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, err
	}
	var console windows.Handle
	err := windows.CreatePseudoConsole(coord(cols, rows), inR, outW, 0, &console)
	// The pseudo-console holds its own ends now.
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, err
	}
	p := &PTY{console: console, in: os.NewFile(uintptr(inW), "pty-in"), out: os.NewFile(uintptr(outR), "pty-out")}
	if p.process, err = create(cmd, console); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func coord(cols, rows int) windows.Coord {
	return windows.Coord{X: int16(max(cols, 1)), Y: int16(max(rows, 1))}
}

// create starts cmd attached to console.
func create(cmd *exec.Cmd, console windows.Handle) (windows.Handle, error) {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return 0, err
	}
	defer attrs.Delete()
	// The attribute's value is the console handle itself, not a pointer to it.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&console)), unsafe.Sizeof(console)); err != nil {
		return 0, err
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	// No std handles of aex's (in the window, pipes): the program's are the pseudo-console's.
	si.Flags = windows.STARTF_USESTDHANDLES

	path, err := exec.LookPath(cmd.Path)
	if err != nil {
		return 0, err
	}
	app, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	args := make([]string, len(cmd.Args))
	for i, a := range cmd.Args {
		args[i] = windows.EscapeArg(a)
	}
	line, err := windows.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return 0, err
	}
	var dir *uint16
	if cmd.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(cmd.Dir); err != nil {
			return 0, err
		}
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	block := envBlock(env)
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(app, line, nil, nil, false, flags, &block[0], dir, &si.StartupInfo, &pi); err != nil {
		return 0, err
	}
	windows.CloseHandle(pi.Thread)
	return pi.Process, nil
}

// envBlock is env as CreateProcess takes it: NUL-separated UTF-16, ending in two NULs.
func envBlock(env []string) []uint16 {
	var b []uint16
	for _, kv := range env {
		if strings.IndexByte(kv, 0) >= 0 {
			continue
		}
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	return append(b, 0)
}

func (p *PTY) Read(b []byte) (int, error) {
	n, err := p.out.Read(b)
	// The pipe breaks once the pseudo-console is closed: that is the end of the screen.
	if errors.Is(err, windows.ERROR_BROKEN_PIPE) {
		return n, io.EOF
	}
	return n, err
}

func (p *PTY) Write(b []byte) (int, error) { return p.in.Write(b) }

// Resize changes the pseudo-console's size.
func (p *PTY) Resize(cols, rows int) error {
	return windows.ResizePseudoConsole(p.console, coord(cols, rows))
}

// Wait waits for the program to exit and gives its exit code, then closes the pseudo-console (the
// rest of the screen is flushed, then Read gives io.EOF).
func (p *PTY) Wait() (int, error) {
	defer p.closeConsole()
	if _, err := windows.WaitForSingleObject(p.process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.process, &code); err != nil {
		return 0, err
	}
	return int(code), nil
}

func (p *PTY) closeConsole() {
	p.closeOnce.Do(func() {
		windows.ClosePseudoConsole(p.console)
		p.in.Close()
	})
}

// Close ends the program if it still runs and lets go of everything. Call it once reading is done.
func (p *PTY) Close() {
	if p.process != 0 {
		var code uint32
		if windows.GetExitCodeProcess(p.process, &code) == nil && code == 259 { // STILL_ACTIVE
			windows.TerminateProcess(p.process, 1)
		}
	}
	p.closeConsole()
	p.out.Close()
	if p.process != 0 {
		windows.CloseHandle(p.process)
		p.process = 0
	}
}
