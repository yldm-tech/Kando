package desktop

import (
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// revealItem has File Explorer show path selected. explorer.exe exits with 1 even when it shows the item, so only a start that failed counts; Reveal then opens the folder instead.
func revealItem(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	err = runCommand(explorerSelectCommand(abs))
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		return nil
	}
	return err
}

// explorerSelectCommand runs explorerSelectLine with the explorer.exe of the Windows folder.
func explorerSelectCommand(path string) *exec.Cmd {
	explorer := "explorer.exe"
	if dir, err := windows.GetWindowsDirectory(); err == nil {
		explorer = filepath.Join(dir, "explorer.exe")
	}
	cmd := exec.Command(explorer)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: explorerSelectLine(path)}
	return cmd
}
