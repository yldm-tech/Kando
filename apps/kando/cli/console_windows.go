//go:build windows

package cli

import (
	"os"
	"syscall"
)

// attachParentProcess is AttachConsole's ATTACH_PARENT_PROCESS, (DWORD)-1: the console of the process that started this one.
const attachParentProcess = ^uint32(0)

var procAttachConsole = syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")

// attachConsole gives a command run from cmd or PowerShell its terminal back. mygo links the Windows app as a GUI program (-H=windowsgui), which Windows starts without a console, so a command's standard streams are missing unless the shell redirected them, and without this `kando status` would print nothing. When needsConsole says so, it attaches to the console of the process that started it and opens the console for each stream that is missing; a stream the shell redirected to a file or a pipe is left as it is. When there is no such console, as when the app is started from Explorer, it leaves the streams alone.
func attachConsole(args []string) {
	have := stdio{in: usable(os.Stdin), out: usable(os.Stdout), err: usable(os.Stderr)}
	if !needsConsole(args, have) {
		return
	}
	if ok, _, _ := procAttachConsole.Call(uintptr(attachParentProcess)); ok == 0 {
		return
	}
	if !have.in {
		if f, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
			os.Stdin = f
		}
	}
	if !have.out {
		if f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
			os.Stdout = f
		}
	}
	if !have.err {
		if f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
			os.Stderr = f
		}
	}
}

// usable reports whether f is a standard stream the process was started with: a GUI program started from a console gets a null handle, which GetFileType rejects, or none at all.
func usable(f *os.File) bool {
	if f == nil {
		return false
	}
	_, err := syscall.GetFileType(syscall.Handle(f.Fd()))
	return err == nil
}
