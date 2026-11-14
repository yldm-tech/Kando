//go:build windows && (amd64 || arm64)

package desktop

import (
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestTrashRecyclesWithoutAsking checks the operation's flags: the item goes to the Recycle Bin, undoably, and no dialog shows, not even the one that warns before deleting for good, which the sink takes the place of.
func TestTrashRecyclesWithoutAsking(t *testing.T) {
	for name, flag := range map[string]uint32{"FOFX_RECYCLEONDELETE": fofxRecycleOnDelete, "FOF_ALLOWUNDO": fofAllowUndo, "FOF_NO_UI": fofNoUI} {
		if trashFlags&flag != flag {
			t.Errorf("the operation lacks %s", name)
		}
	}
}

// TestTrashSinkRefusesADeleteForGood calls the sink's methods as IFileOperation does: it answers for itself and IUnknown only, lets a delete that recycles through, and refuses, remembering it, one that would delete the item for good.
func TestTrashSinkRefusesADeleteForGood(t *testing.T) {
	trashSinkOnce.Do(initTrashSink)
	sink := &trashSink{vtbl: &trashSinkVtbl, refs: 1}
	p := uintptr(unsafe.Pointer(sink))
	trashSinksMu.Lock()
	trashSinks[p] = sink
	trashSinksMu.Unlock()
	t.Cleanup(func() {
		trashSinksMu.Lock()
		delete(trashSinks, p)
		trashSinksMu.Unlock()
	})
	call := func(i int, args ...uintptr) uintptr {
		r, _, _ := syscall.SyscallN(trashSinkVtbl[i], append([]uintptr{p}, args...)...)
		return r
	}

	var out uintptr
	if r := call(0, uintptr(unsafe.Pointer(&iidIFileOperationProgressSink)), uintptr(unsafe.Pointer(&out))); r != sOK || out != p || sink.refs != 2 {
		t.Fatalf("QueryInterface(sink) = %#x, %#x, refs %d", r, out, sink.refs)
	}
	other := windows.GUID{Data1: 1}
	if r := call(0, uintptr(unsafe.Pointer(&other)), uintptr(unsafe.Pointer(&out))); r != eNoInterface || out != 0 {
		t.Fatalf("QueryInterface(other) = %#x, %#x", r, out)
	}
	if r := call(11, tsfDeleteRecycleIfPossible, 0); r != sOK || sink.refused {
		t.Fatalf("a recycling delete: %#x, refused %v", r, sink.refused)
	}
	if r := call(11, 0, 0); r != eAbort || !sink.refused {
		t.Fatalf("a delete for good: %#x, refused %v", r, sink.refused)
	}
}
