package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unsafe"
)

// sysCommandLine is the command line cmd runs as written whole (explorerSelectCommand).
func sysCommandLine(cmd *exec.Cmd) string {
	if cmd.SysProcAttr == nil {
		return ""
	}
	return cmd.SysProcAttr.CmdLine
}

// TestRevealCountsExplorersExitCode checks that File Explorer's exit code 1, which it gives even when it shows the item, counts as shown, and only a start that failed does not.
func TestRevealCountsExplorersExitCode(t *testing.T) {
	ran := fakeTools(t, &exec.ExitError{ProcessState: &os.ProcessState{}})
	film := filepath.Join(t.TempDir(), "My Film.mp4")
	if err := revealItem(film); err != nil || len(*ran) != 1 || !strings.HasSuffix(sysCommandLine((*ran)[0]), `/select,"`+film+`"`) {
		t.Fatalf("reveal answered %v, ran %v", err, *ran)
	}
	fakeTools(t, errors.New("explorer.exe: file not found"))
	if err := revealItem(film); err == nil {
		t.Fatal("a File Explorer that did not start counted as shown")
	}
}

// TestTrashAsksTheRecycleBin checks SHFileOperationW's request: a delete of the one path, double-NUL terminated, that the Recycle Bin can undo and that warns before deleting for good, and that a declined warning fails the trash.
func TestTrashAsksTheRecycleBin(t *testing.T) {
	film := filepath.Join(t.TempDir(), "My Film.mp4")
	var seen []shFileOpStruct
	var from []uint16
	aborted := int32(0)
	prev := shFileOperation
	shFileOperation = func(op *shFileOpStruct) uintptr {
		seen = append(seen, *op)
		p := op.pFrom
		for n := 0; ; n++ {
			c := *(*uint16)(unsafe.Add(unsafe.Pointer(p), n*2))
			from = append(from, c)
			if c == 0 && len(from) > 1 && from[len(from)-2] == 0 {
				break
			}
		}
		op.fAnyOperationsAborted = aborted
		return 0
	}
	t.Cleanup(func() { shFileOperation = prev })
	if err := trashItem(film); err != nil {
		t.Fatal(err)
	}
	want, _ := trashFrom(film)
	if len(seen) != 1 || seen[0].wFunc != foDelete || seen[0].fFlags&fofAllowUndo == 0 || seen[0].fFlags&fofWantNukeWarning == 0 || !slices.Equal(from, want) {
		t.Fatalf("SHFileOperationW was asked %+v from %v", seen, from)
	}
	aborted = 1
	if err := trashItem(film); err == nil {
		t.Fatal("a delete the user declined counted as trashed")
	}
}
