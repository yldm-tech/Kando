package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
