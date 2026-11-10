package media

import (
	"context"
	"errors"
	"syscall"
	"time"
)

// busyRetries is how many more times starting a tool is tried when the system says its file is busy.
const busyRetries = 5

// retryBusy runs start, and runs it again a few times, waiting a little longer each time, while it fails because the tool's file is busy (ETXTBSY). Linux refuses to run a file that some process holds open for writing, and a tool Kando has just written, as its own yt-dlp, ffmpeg or whisper-cli right after downloading them, can be held so for a moment: a process another goroutine forks at the same time inherits the file's descriptor until it runs its own program. The tool never started when that happens, so starting it again runs it once. It stops waiting when ctx is done.
func retryBusy(ctx context.Context, start func() error) error {
	wait := 20 * time.Millisecond
	for attempt := 0; ; attempt++ {
		err := start()
		if err == nil || !errors.Is(err, syscall.ETXTBSY) || attempt == busyRetries {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(wait):
		}
		wait *= 2
	}
}
