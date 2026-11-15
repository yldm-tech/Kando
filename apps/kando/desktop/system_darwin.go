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

// trashItem moves path to the Trash with NSFileManager, through osascript (trashScript), and fails for an item on a volume without a Trash.
func trashItem(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return runCommand(fileManagerTrashCommand(abs))
}
