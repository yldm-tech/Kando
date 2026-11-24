package media

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// replaceFor is how long replace waits for a reader to let go of the file it replaces.
const replaceFor = 2 * time.Second

// replace moves tmp over path. Windows refuses to replace a file another handle holds open without letting it be deleted, as Go's os.ReadFile holds it while it reads, with ERROR_ACCESS_DENIED or a sharing violation: a film's subtitles are read by the player that shows the film while it is made, and by Phone Sync, each only while it reads them, so replace tries again for a little while rather than failing the job, as the plugins folder (apps/kando/plugins) and the phone's store already do.
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
