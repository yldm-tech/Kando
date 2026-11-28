package store

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// errSharingViolation is Windows' ERROR_SHARING_VIOLATION.
const errSharingViolation = syscall.Errno(32)

// replaceFor is how long replace waits for a reader to let go of the file it replaces.
const replaceFor = 2 * time.Second

// replace moves tmp over path. Windows refuses to replace a file another handle holds open without letting it be deleted, as Go's os.ReadFile holds it while it reads, with ERROR_ACCESS_DENIED or a sharing violation; a reader holds a record only while it reads it, so replace tries again for a little while rather than losing the save.
func replace(tmp, path string) error {
	deadline := time.Now().Add(replaceFor)
	wait := time.Millisecond
	for {
		err := os.Rename(tmp, path)
		if err == nil || !(errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, errSharingViolation)) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(wait)
		wait = min(2*wait, 50*time.Millisecond)
	}
}

// removeAll removes path and everything in it, trying again for a little while, as replace does, while Windows refuses to delete a file a handle holds open: a film's folder is removed just after its download wrote and closed its files, which an antivirus may still be scanning, or a reader still holds a moment, and a folder left so keeps its record, which shows the film as still kept.
func removeAll(path string) error {
	deadline := time.Now().Add(replaceFor)
	wait := time.Millisecond
	for {
		err := os.RemoveAll(path)
		if err == nil || !(errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, errSharingViolation)) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(wait)
		wait = min(2*wait, 50*time.Millisecond)
	}
}
