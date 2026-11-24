package media

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// renameFor is how long renameHeld waits for whoever holds a file of what it moves to let go. Rename runs on the UI thread and falls back to a copy (Copy) when it fails, so this stays short.
const renameFor = 500 * time.Millisecond

// renameHeld renames from to to. Windows refuses to rename a file another handle holds open without letting it be deleted, or a folder while a handle is open on anything inside it, with ERROR_ACCESS_DENIED or a sharing violation: Kando counting a library's films just after a move (films.Videos) or an antivirus scanning the files just moved hold them so for a moment, and the move back that Undo makes right then would otherwise turn into a copy of the whole film. So it tries again for a little while.
func renameHeld(from, to string) error {
	deadline := time.Now().Add(renameFor)
	wait := time.Millisecond
	for {
		err := os.Rename(from, to)
		if err == nil || !(errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, errSharingViolation)) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(wait)
		wait = min(2*wait, 50*time.Millisecond)
	}
}
