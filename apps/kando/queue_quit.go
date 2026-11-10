package main

import (
	"errors"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"kando/apps/kando/quit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// errQuitting is the cause the running job is stopped with when the app quits. It is never shown: a job stopped with it reports nothing more, and library.json keeps the stage it had reached, which the next launch reads back as queued (shelf.Load).
var errQuitting = errors.New("kando is quitting")

// queueQuitWait is how long a quit waits for the running job to return once asked to stop, so its programs are not left running behind the app. It stays short of the three seconds mygo gives the event loop to stop once the windows have closed.
const queueQuitWait = 2 * time.Second

// queueBeforeQuit holds a quit back while a job runs and asks first (quit.Confirm). Choosing 退出 there quits again, and that quit goes ahead. The updater's relaunch into an installed update comes here as a quit too, and mygo's QuitEvent does not say it was one, so 退出 then quits without starting the app again; the update is still installed and the next launch runs it.
func (a *app) queueBeforeQuit(e *mygo.QuitEvent) {
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
	// The quit comes from the menu, the Dock or a signal, not from the window's own frame, so the window is asked for one to draw the confirmation in.
	if a.win != nil {
		a.win.Invalidate()
	}
}

// queueWatchSignals lets SIGINT and SIGTERM quit without asking. mygo turns them into a quit (its quitOnSignals), but no one is at the window to answer one: kill, a logout or shutdown on Linux, a service manager, or mygo dev rebuilding. A signal marks the quit confirmed and quits again (queueQuitOnSignal), so whichever of mygo's quit and this one runs first, the app quits and the running job is stopped. A signal mygo left ignored, as for a shell's background job, stays ignored, and on Windows mygo quits on no signal. It runs once, from ready.
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
		mygo.RunOnMain(a.queueQuitOnSignal)
	}()
}

// queueQuitOnSignal quits without asking, as a signal asked for it. It runs on the UI thread.
func (a *app) queueQuitOnSignal() {
	a.queue.quitConfirmed = true
	quitApp()
}

// queueQuitOverlay draws the confirmation over the window while it is open (state_overlay.go). It names the job running now, which is the next one when the job it was opened for finished meanwhile.
func (a *app) queueQuitOverlay(c *ui.Context) {
	if !a.queue.quitAsk {
		return
	}
	job, _ := a.library.Find(a.queue.active)
	if quit.Confirm(c, &a.queue.quitAsk, jobName(job)) {
		a.queue.quitConfirmed = true
		quitApp()
	}
}

// queueQuitModal closes the confirmation once no job runs any more, as there is nothing left to ask about, and gives back the window's modal flag once it has closed.
func (a *app) queueQuitModal() {
	if a.queue.active == "" {
		a.queue.quitAsk = false
	}
	if !a.queue.quitAsk && a.queue.quitRelease != nil {
		a.queue.quitRelease()
		a.queue.quitRelease = nil
	}
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
