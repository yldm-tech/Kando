//go:build windows && (amd64 || arm64)

package desktop

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shFileOpStruct is SHFILEOPSTRUCTW as the 64-bit Windows lays it out; 32-bit Windows packs it (trash_windows32.go).
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

// SHFileOperationW's operation and flags.
const (
	foDelete = 0x0003
	// fofAllowUndo sends a deleted item to the Recycle Bin.
	fofAllowUndo = 0x0040
	// fofWantNukeWarning asks the user first when an item would be deleted for good rather than recycled, as one on a drive without a Recycle Bin or too big for it; the other flags keep every other question and the progress quiet.
	fofWantNukeWarning = 0x4000
	fofSilent          = 0x0004
	fofNoConfirmation  = 0x0010
	fofNoErrorUI       = 0x0400
	trashFlags         = fofAllowUndo | fofWantNukeWarning | fofSilent | fofNoConfirmation | fofNoErrorUI
)

var procSHFileOperationW = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// shFileOperation runs op through SHFileOperationW and returns its result, 0 when it succeeded. Tests replace it to see op without touching the Recycle Bin.
var shFileOperation = func(op *shFileOpStruct) uintptr {
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(op)))
	return r
}

// trashFrom is SHFileOperationW's pFrom for path: the path followed by two NULs, as the list of one path it takes.
func trashFrom(path string) ([]uint16, error) {
	from, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, err
	}
	return append(from, 0), nil
}

// trashItem moves path to the Recycle Bin with SHFileOperationW. When the Recycle Bin cannot take it, Windows asks before deleting it for good, and trashItem fails when the user declines. It runs on a thread of its own in a single-threaded COM apartment, which the shell's file operations ask for, and waits for it.
func trashItem(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	from, err := trashFrom(abs)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		// The thread ends with the goroutine, as it stays locked, so the apartment goes with it.
		runtime.LockOSThread()
		// S_FALSE says the thread was in the apartment already, which still takes a CoUninitialize.
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil || err == syscall.Errno(windows.S_FALSE) {
			defer windows.CoUninitialize()
		}
		op := &shFileOpStruct{wFunc: foDelete, pFrom: &from[0], fFlags: trashFlags}
		r := shFileOperation(op)
		runtime.KeepAlive(from)
		switch {
		case r != 0:
			done <- fmt.Errorf("SHFileOperationW failed with 0x%X", r)
		case op.fAnyOperationsAborted != 0:
			done <- errors.New("the item was not moved to the Recycle Bin")
		default:
			done <- nil
		}
	}()
	return <-done
}
