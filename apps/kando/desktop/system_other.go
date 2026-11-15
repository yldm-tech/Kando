//go:build !darwin && !linux && !windows

package desktop

import "errors"

// revealItem cannot select an item here, so Reveal opens its folder.
func revealItem(string) error { return errors.ErrUnsupported }

// trashItem has no Trash to move an item to here.
func trashItem(string) error { return errors.ErrUnsupported }
