package desktop

import (
	"errors"
	"fmt"
	"path/filepath"
)

// revealItem has the file manager that answers org.freedesktop.FileManager1 show path selected, through gdbus. It fails when gdbus is missing or no file manager answers, and Reveal then opens the folder instead.
func revealItem(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	gdbus, err := lookPath("gdbus")
	if err != nil {
		return fmt.Errorf("gdbus: %w", errNoTool)
	}
	return runCommand(showItemsCommand(gdbus, abs))
}

// trashItem moves path to the Trash with gio trash, which fails for an item on a volume without a Trash. Without gio it fails as unsupported, leaving the item where it is.
func trashItem(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	gio, err := lookPath("gio")
	if err != nil {
		return fmt.Errorf("gio: %w", errors.Join(errNoTool, errors.ErrUnsupported))
	}
	return runCommand(gioTrashCommand(gio, abs))
}
