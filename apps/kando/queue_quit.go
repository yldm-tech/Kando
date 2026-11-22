package main

import (
	"errors"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/quit"

	"github.com/nsgo-dev/nsgo/core"
)

// errQuitting is the cause the running job is stopped with when the app quits. It is never shown: a job stopped with it reports nothing more, and library.json keeps the stage it had reached, which the next launch reads back as queued (shelf.Load).
var errQuitting = errors.New("kando is quitting")

// queueQuitWait is how long a quit waits for the running job to return once asked to stop, so its programs are not left running behind the app. main waits for it once nsgo.Run has returned (areasQuitting), so it holds up no thread the system watches.
const queueQuitWait = 2 * time.Second

// queueBeforeQuit holds a quit back while a job runs and asks first (quit.Confirm), the Dock's Quit and the updater's relaunch too (quitRequested). Choosing 退出 there quits again, and that quit goes ahead. A quit the app cannot hold back comes here with e.Forced set: a logout, restart or shutdown, or a quit that asked no one (quitHeard). It goes ahead without asking, as a signal's quit does, and the running job is stopped as the app quits (queueQuit).
func (a *app) queueBeforeQuit(e *quitEvent) {
	if e.Forced {
		a.queue.quitConfirmed = true
		return
	}
	if a.queue.active == "" || a.queue.quitConfirmed {
		return
	}
	e.PreventDefault()
	a.queueAskQuit()
}

// queueAskQuit opens the confirmation of a quit while a job runs, in the main window brought to the front, which may have been put away by its close button.
func (a *app) queueAskQuit() {
	a.windowShowMain()
	if a.queue.quitAsk {
		return
	}
	a.queue.quitAsk = true
	if a.queue.quitRelease == nil {
		a.queue.quitRelease = a.holdModal()
	}
}

// queueWatchSignals lets SIGINT and SIGTERM quit without asking, as no one is at the window to answer: kill, a logout or shutdown on Linux, a service manager, or nsgo dev rebuilding. A signal marks the quit confirmed and quits (queueQuitOnSignal) on the engine's goroutine, and the running job is stopped. A signal left ignored, as for a shell's background job, stays ignored, and Windows sends none. It runs once, from ready.
func (a *app) queueWatchSignals() {
	if runtime.GOOS == "windows" {
		return
	}
	ch := make(chan os.Signal, 1)
	watched := false
	for _, s := range []os.Signal{syscall.SIGINT, syscall.SIGTERM} {
		if !signal.Ignored(s) {
			signal.Notify(ch, s)
			watched = true
		}
	}
	if !watched {
		return
	}
	go func() {
		<-ch
		signal.Stop(ch)
		a.update(a.queueQuitOnSignal)
	}()
}

// queueQuitOnSignal quits without asking, as a signal asked for it. It runs on the engine's goroutine.
func (a *app) queueQuitOnSignal() {
	a.queue.quitConfirmed = true
	a.requestQuit()
}

// queueQuitOverlay is the confirmation over the window while it is open (state_overlay.go). It names the job running now, which is the next one when the job it was opened for finished meanwhile.
func (a *app) queueQuitOverlay(*core.Scope) core.Node {
	return quit.Confirm{
		Open: live.Func(a, func() bool { return a.queue.quitAsk }),
		Film: live.Func(a, func() string {
			job, _ := a.library.Find(a.queue.active)
			return jobName(job)
		}),
		OnQuit: live.Do(a, func() {
			a.queue.quitAsk = false
			a.queue.quitConfirmed = true
			a.requestQuit()
		}),
		OnCancel: live.Do(a, func() { a.queue.quitAsk = false }),
	}
}

// queueQuitModal closes the confirmation once no job runs any more, as there is nothing left to ask about, and gives back the window's modal flag once it has closed, and reports whether it did either. It runs after every change of the app's state (areaHooks.afterChange).
func (a *app) queueQuitModal() bool {
	changed := false
	if a.queue.active == "" && a.queue.quitAsk {
		a.queue.quitAsk = false
		changed = true
	}
	if !a.queue.quitAsk && a.queue.quitRelease != nil {
		a.queue.quitRelease()
		a.queue.quitRelease = nil
		changed = true
	}
	return changed
}

// queueQuit stops the running job as the app quits and waits for its pipeline to return, at most queueQuitWait, so yt-dlp, ffmpeg or whisper-cli are not left running. No job starts after it.
func (a *app) queueQuit() {
	a.queue.stopped = true
	done := a.queue.done
	if a.queue.cancel == nil {
		return
	}
	a.queue.stopActive(errQuitting)
	if done == nil {
		return
	}
	timer := time.NewTimer(queueQuitWait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}
