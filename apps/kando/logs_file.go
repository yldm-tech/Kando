package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"kando/apps/kando/feedback"
)

const (
	// logName is the log file's name in the logs folder (logsDir).
	logName = "kando.log"
	// logLimit is how big the log file grows before a new one starts.
	logLimit = 5 << 20
	// logKeep is how many full log files are kept beside the one being written, as kando.1.log and kando.2.log, the newer first.
	logKeep = 2
)

// logFile is the app's log file. The standard logger writes through it once the window is ready (logsOpen): each line goes to echo, stderr in the app, as before, and to the file, with whatever is shaped like a key or token written as *** (feedback.Scrubber.NoSecrets). Once the file would grow past limit, it is moved aside and a new one starts (rotate). A crash's panic and stacks go to the file as well (debug.SetCrashOutput), as an app opened from Finder or the Dock has no terminal to show its stderr. Its methods are safe to call from any goroutine.
type logFile struct {
	mu    sync.Mutex
	path  string
	limit int64
	echo  io.Writer
	// f is the open file, and size how much it holds. f is nil once the file is closed (closed), or while it cannot be opened again after a rotation or after its folder was removed; Write and Path try again until Close.
	f      *os.File
	size   int64
	closed bool
}

// openLogFile opens the log at path for appending, making its folder, after moving a file already at its limit aside first.
func openLogFile(path string, limit int64, echo io.Writer) (*logFile, error) {
	w := &logFile{path: path, limit: limit, echo: echo}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

// open opens the file at w.path, making its folder again if the user removed it, after rotating a full one, and points the crash output at it.
func (w *logFile) open() error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(w.path); err == nil && info.Size() >= w.limit {
		w.rotate()
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.f, w.size = f, info.Size()
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	return nil
}

// release closes the file and stops the crash output going to it, which holds a handle of its own; Windows does not move a file that is still open.
func (w *logFile) release() {
	if w.f == nil {
		return
	}
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	_ = w.f.Close()
	w.f, w.size = nil, 0
}

// rotate moves each kept file one place older, dropping the oldest, and the log file to kando.1.log. The file must not be open.
func (w *logFile) rotate() {
	for n := logKeep; n > 1; n-- {
		_ = os.Rename(w.older(n-1), w.older(n))
	}
	_ = os.Rename(w.path, w.older(1))
}

// older is the path of the nth file before the one being written: kando.1.log for 1.
func (w *logFile) older(n int) string {
	return strings.TrimSuffix(w.path, ".log") + "." + strconv.Itoa(n) + ".log"
}

// Write writes one entry of the logger, scrubbed, to echo and to the file, starting a new file first when the entry would take this one past its limit. It reports p written whatever happens to the file, as the logger has nowhere to say it failed.
func (w *logFile) Write(p []byte) (int, error) {
	line := []byte(feedback.Scrubber{}.NoSecrets(string(p)))
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.echo != nil {
		_, _ = w.echo.Write(line)
	}
	if w.f == nil && !w.closed {
		_ = w.open()
	}
	if w.f != nil && w.size > 0 && w.size+int64(len(line)) > w.limit {
		w.release()
		w.rotate()
		_ = w.open()
	}
	if w.f != nil {
		n, _ := w.f.Write(line)
		w.size += int64(n)
	}
	return len(p), nil
}

// Path is the log file's path. A file or folder removed from disk since it was opened, as by the user in Finder, is made again first, so there is a file to show.
func (w *logFile) Path() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := os.Stat(w.path); err != nil && !w.closed {
		w.release()
		_ = w.open()
	}
	return w.path
}

// Close closes the file. Entries written after it go to echo alone.
func (w *logFile) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.release()
	w.closed = true
}
