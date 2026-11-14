//go:build windows && (amd64 || arm64)

package desktop

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Recycle Bin, through IFileOperation, as MyGo did and Electron's shell.trashItem does. Windows deletes an item for good when it cannot recycle it: one too big for the Recycle Bin, or on a drive without one, as network shares and many removable drives are. The operation shows no UI, so nothing would ask first; a progress sink refuses those deletes instead, and trashItem fails, leaving the item where it is.

var (
	clsidFileOperation            = windows.GUID{Data1: 0x3ad05575, Data2: 0x8857, Data3: 0x4850, Data4: [8]byte{0x92, 0x77, 0x11, 0xb8, 0x5b, 0xdb, 0x8e, 0x09}}
	iidIFileOperation             = windows.GUID{Data1: 0x947aab5f, Data2: 0x0a5c, Data3: 0x4c13, Data4: [8]byte{0xb4, 0xd6, 0x4b, 0xf7, 0x83, 0x6f, 0xc9, 0xf8}}
	iidIFileOperationProgressSink = windows.GUID{Data1: 0x04b0f1a7, Data2: 0x9490, Data3: 0x44bc, Data4: [8]byte{0x96, 0xe1, 0x42, 0x96, 0xa3, 0x12, 0x52, 0xe2}}
	iidIShellItem                 = windows.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
	iidIUnknown                   = windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

	procCoCreateInstance            = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
)

const (
	sOK                = 0
	eNoInterface       = 0x80004002
	eAbort             = 0x80004004
	clsctxInprocServer = 0x1

	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoConfirmMkdir = 0x0200
	fofNoErrorUI      = 0x0400
	// fofNoUI is FOF_NO_UI: no progress, no question and no error dialog.
	fofNoUI = fofSilent | fofNoConfirmation | fofNoErrorUI | fofNoConfirmMkdir
	// fofxRecycleOnDelete sends a deleted item to the Recycle Bin where it can.
	fofxRecycleOnDelete = 0x00080000
	fofxAddUndoRecord   = 0x20000000
	// trashFlags are the operation's flags: recycle, undoably, and show nothing.
	trashFlags = fofNoUI | fofAllowUndo | fofxRecycleOnDelete | fofxAddUndoRecord
	// tsfDeleteRecycleIfPossible is the flag PreDeleteItem gets when the item goes to the Recycle Bin; without it the delete is for good.
	tsfDeleteRecycleIfPossible = 0x80

	// IFileOperation's methods, by their place in its vtable.
	foSetOperationFlags       = 5
	foDeleteItem              = 18
	foPerformOperations       = 21
	foGetAnyOperationsAborted = 22
)

// errWouldDelete is trashItem's error for an item the Recycle Bin cannot take, which Windows would have deleted for good.
var errWouldDelete = errors.New("the Recycle Bin cannot take it, so it would be deleted for good")

// A trashSink is an IFileOperationProgressSink: only PreDeleteItem does anything, refusing a delete that would not go to the Recycle Bin; the other methods succeed.
type trashSink struct {
	vtbl    *[19]uintptr
	refs    int32
	refused bool
}

var (
	trashSinkVtbl [19]uintptr
	trashSinkOnce sync.Once
	// trashSinks holds each sink while an operation may call it, and finds it from the pointer COM passes, so no Go pointer is made from a uintptr.
	trashSinksMu sync.Mutex
	trashSinks   = map[uintptr]*trashSink{}
)

func sinkOf(this uintptr) *trashSink {
	trashSinksMu.Lock()
	defer trashSinksMu.Unlock()
	return trashSinks[this]
}

// refuseDelete is PreDeleteItem's answer for flags: E_ABORT, and refused, unless the item goes to the Recycle Bin.
func refuseDelete(flags uint32) (hr uintptr, refused bool) {
	if flags&tsfDeleteRecycleIfPossible == 0 {
		return eAbort, true
	}
	return sOK, false
}

// native is the address addr as a pointer, for memory COM owns, which the garbage collector does not move or free.
func native(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

func initTrashSink() {
	cb := syscall.NewCallback
	ok := cb(func(this uintptr) uintptr { return sOK })
	ok3 := cb(func(this, a, b, c uintptr) uintptr { return sOK })
	ok4 := cb(func(this, a, b, c, d uintptr) uintptr { return sOK })
	ok5 := cb(func(this, a, b, c, d, e uintptr) uintptr { return sOK })
	ok6 := cb(func(this, a, b, c, d, e, f uintptr) uintptr { return sOK })
	trashSinkVtbl = [19]uintptr{
		// QueryInterface
		cb(func(this, riid, out uintptr) uintptr {
			*(*uintptr)(native(out)) = 0
			if iid := *(*windows.GUID)(native(riid)); iid != iidIUnknown && iid != iidIFileOperationProgressSink {
				return eNoInterface
			}
			*(*uintptr)(native(out)) = this
			sinkOf(this).refs++
			return sOK
		}),
		cb(func(this uintptr) uintptr { s := sinkOf(this); s.refs++; return uintptr(s.refs) }), // AddRef
		cb(func(this uintptr) uintptr { s := sinkOf(this); s.refs--; return uintptr(s.refs) }), // Release
		ok, // StartOperations
		cb(func(this, hr uintptr) uintptr { return sOK }), // FinishOperations
		ok3, // PreRenameItem
		ok5, // PostRenameItem
		ok4, // PreMoveItem
		ok6, // PostMoveItem
		ok4, // PreCopyItem
		ok6, // PostCopyItem
		// PreDeleteItem(flags, item)
		cb(func(this, flags, item uintptr) uintptr {
			hr, refused := refuseDelete(uint32(flags))
			if refused {
				sinkOf(this).refused = true
			}
			return hr
		}),
		ok4, // PostDeleteItem
		ok3, // PreNewItem
		cb(func(this, a, b, c, d, e, f, g uintptr) uintptr { return sOK }), // PostNewItem
		cb(func(this, total, done uintptr) uintptr { return sOK }),         // UpdateProgress
		ok, ok, ok, // ResetTimer, PauseTimer, ResumeTimer
	}
}

// comCall calls method i of the COM interface obj. Pointers converted to uintptr in the call stay valid until it returns (go:uintptrescapes).
//
//go:uintptrescapes
func comCall(obj uintptr, i int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(native(obj))
	fn := *(*uintptr)(unsafe.Add(native(vtbl), i*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func release(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func hresultError(what string, hr uintptr) error {
	return fmt.Errorf("%s failed (HRESULT %#08x)", what, uint32(hr))
}

// trashItem moves path to the Recycle Bin, and fails, leaving it where it is, when the Recycle Bin cannot take it. It runs on a thread of its own in a single-threaded COM apartment, which the shell's file operations ask for, and waits for it.
func trashItem(path string) error {
	abs, err := filepath.Abs(path)
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
		done <- recycle(abs)
	}()
	return <-done
}

// recycle deletes path through IFileOperation, recycling it, with a trashSink that refuses a delete for good.
func recycle(path string) error {
	trashSinkOnce.Do(initTrashSink)
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var item uintptr
	if r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item))); failed(r) || item == 0 {
		return hresultError("SHCreateItemFromParsingName", r)
	}
	defer release(item)
	// The operation holds the sink until it is released, which the deferred release below does before the sink leaves the map.
	sink := &trashSink{vtbl: &trashSinkVtbl, refs: 1}
	p := uintptr(unsafe.Pointer(sink))
	trashSinksMu.Lock()
	trashSinks[p] = sink
	trashSinksMu.Unlock()
	defer func() {
		trashSinksMu.Lock()
		delete(trashSinks, p)
		trashSinksMu.Unlock()
	}()
	var op uintptr
	if r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, clsctxInprocServer, uintptr(unsafe.Pointer(&iidIFileOperation)), uintptr(unsafe.Pointer(&op))); failed(r) || op == 0 {
		return hresultError("CoCreateInstance(FileOperation)", r)
	}
	defer release(op)
	if r := comCall(op, foSetOperationFlags, trashFlags); failed(r) {
		return hresultError("IFileOperation.SetOperationFlags", r)
	}
	if r := comCall(op, foDeleteItem, item, p); failed(r) {
		return hresultError("IFileOperation.DeleteItem", r)
	}
	r := comCall(op, foPerformOperations)
	var aborted int32
	comCall(op, foGetAnyOperationsAborted, uintptr(unsafe.Pointer(&aborted)))
	runtime.KeepAlive(sink)
	switch {
	case sink.refused:
		return errWouldDelete
	case failed(r):
		return hresultError("IFileOperation.PerformOperations", r)
	case aborted != 0:
		return errors.New("the operation was aborted")
	}
	return nil
}
