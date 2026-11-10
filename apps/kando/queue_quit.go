package main

import (
	"errors"
	"time"

	"kando/apps/kando/quit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// errQuitting is the cause the running job is stopped with when the app quits. It is never shown: a job stopped with it reports nothing more, and library.json keeps the stage it had reached, which the next launch reads back as queued (shelf.Load).
var errQuitting = errors.New("kando is quitting")

// queueQuitWait is how long a quit waits for the running job to return once asked to stop, so its programs are not left running behind the app. It stays short of the three seconds mygo gives the event loop to stop once the windows have closed.
const queueQuitWait = 2 * time.Second

// queueBeforeQuit holds a quit back while a job runs and asks first (quit.Confirm). Choosing 退出 there quits again, and that quit goes ahead.
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
	job, _ := a.library.Find(a.queue.active)
	a.queue.quitFilm = jobName(job)
	a.queue.quitAsk = true
	if a.queue.quitRelease == nil {
		a.queue.quitRelease = a.holdModal()
	}
}

// queueQuitOverlay draws the confirmation over the window while it is open (state_overlay.go).
func (a *app) queueQuitOverlay(c *ui.Context) {
	if quit.Confirm(c, &a.queue.quitAsk, a.queue.quitFilm) {
		a.queue.quitConfirmed = true
		quitApp()
	}
}

// queueQuitModal gives back the window's modal flag once the confirmation has closed.
func (a *app) queueQuitModal() {
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
