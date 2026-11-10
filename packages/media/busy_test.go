package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A tool whose file is busy is started again until it starts; any other error, and a busy file that stays busy past the retries, come back as they are.
func TestRetryBusyStartsTheToolOnceItsFileIsFree(t *testing.T) {
	busy := &os.PathError{Op: "fork/exec", Path: "/tmp/yt-dlp", Err: syscall.ETXTBSY}
	tries := 0
	err := retryBusy(context.Background(), func() error {
		tries++
		if tries < 3 {
			return busy
		}
		return nil
	})
	if err != nil || tries != 3 {
		t.Fatalf("err %v after %d tries, want nil after 3", err, tries)
	}

	tries = 0
	other := errors.New("exit status 1")
	if err := retryBusy(context.Background(), func() error { tries++; return other }); err != other || tries != 1 {
		t.Fatalf("err %v after %d tries, want %v after 1", err, tries, other)
	}

	tries = 0
	if err := retryBusy(context.Background(), func() error { tries++; return busy }); !errors.Is(err, syscall.ETXTBSY) || tries != busyRetries+1 {
		t.Fatalf("err %v after %d tries, want ETXTBSY after %d", err, tries, busyRetries+1)
	}
}

// Waiting for a busy file stops when the work is cancelled.
func TestRetryBusyStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tries := 0
	err := retryBusy(ctx, func() error {
		tries++
		cancel()
		return fmt.Errorf("fork/exec: %w", syscall.ETXTBSY)
	})
	if !errors.Is(err, syscall.ETXTBSY) || tries != 1 {
		t.Fatalf("err %v after %d tries, want ETXTBSY after 1", err, tries)
	}
}

// On Linux a file held open for writing cannot be run (ETXTBSY), as Kando's own yt-dlp can be for a moment right after it is written: the tool runs once the file is let go of, rather than failing.
func TestRunToolWaitsForAFileBeingWritten(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only Linux refuses to run a file that is open for writing")
	}
	path := filepath.Join(t.TempDir(), "yt-dlp")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("#!/bin/sh\necho 2026.11.03\n"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(60 * time.Millisecond)
		f.Close()
	}()
	out, err := runTool(context.Background(), false, path, "--version")
	if err != nil || strings.TrimSpace(out) != "2026.11.03" {
		t.Fatalf("out %q, err %v", out, err)
	}
}
