package media

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// renameWithin renames from to to, trying again until within has passed while Windows says something holds them. Windows refuses to rename or replace a file another handle holds open without letting it be deleted, as Go's os.ReadFile holds it while it reads, or a folder while a handle is open on anything inside it, with ERROR_ACCESS_DENIED or a sharing violation; such holds last a moment, as the player reading a film's subtitles while they are made, Kando counting a library's films just after a move, or an antivirus scanning files just written, as the plugins folder (apps/kando/plugins) and the phone's store also find.
func renameWithin(from, to string, within time.Duration) error {
	deadline := time.Now().Add(within)
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
