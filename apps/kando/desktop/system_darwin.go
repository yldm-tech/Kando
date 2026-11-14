package desktop

import (
	"path/filepath"
)

// revealItem has Finder show path selected (open -R), and fails when path is not there.
func revealItem(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return runCommand(openRevealCommand(abs))
}

// trashItem moves path to the Trash with macOS's trash command, or through Finder on a macOS older than 14 without it; either fails for an item on a volume without a Trash.
func trashItem(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := lookPath(macTrash); err == nil {
		return runCommand(macTrashCommand(abs))
	}
	return runCommand(finderTrashCommand(abs))
}
