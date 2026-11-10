package main

import (
	"context"
	"errors"
	"log"
	"math"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/egoist/mygo"
)

// What the queue tells the system outside the window: a notification when a job it ran is done or failed while the app is in the background, which opens the film, or shows its card, when it is clicked; the number of jobs still to do as the badge on the app's Dock icon, on macOS only (MyGo's SetBadgeCount sets the Dock badge, which does nothing on other platforms); and how far the running job has gone in its stage as the app's progress bar (on the Dock icon on macOS, the taskbar button on Windows, where the desktop shows one on Linux). So Windows and Linux show the progress bar and no count.

// queueDesktop is how the queue reaches the system. Every field is nil until main's window is ready (queueHooks' ready sets them, queueSystemDesktop), and in tests that do not set them, so the queue then tells the system nothing.
type queueDesktop struct {
	// active reports whether the app is the one in front, whose window already shows what a notification would say.
	active func() bool
	// notify posts a notification with id, title and body, and returns without waiting for the system. clear removes the app's notifications. Both are nil where the system shows none of the app's, as on macOS for an app that is not packaged.
	notify func(id, title, body string)
	clear  func()
	// badge shows n on the app's Dock icon (macOS only), and removes the badge for 0. progress shows the progress bar, or removes it for the zero ProgressBar.
	badge    func(n int)
	progress func(p mygo.ProgressBar)
}

// queueSystem is the queue's state outside the window: how it reaches the system, whether the user wants notifications (Settings › 通用), and what it last put on the app's icon.
type queueSystem struct {
	desktop queueDesktop
	notify  bool
	// told is false until the queue first put the badge and the progress bar on the icon; badge and progress are what it put there, so it tells the system only what changed.
	told     bool
	badge    int
	progress mygo.ProgressBar
}

func newQueueSystem() queueSystem { return queueSystem{notify: true} }

// queueSystemDesktop reaches the system through MyGo, the progress bar through win. Main's window must be ready.
func queueSystemDesktop(win *mygo.Window) queueDesktop {
	d := queueDesktop{active: mygo.App.IsActive, badge: mygo.App.SetBadgeCount, progress: win.SetProgressBar}
	if mygo.NotificationsSupported() {
		d.notify, d.clear = queueShowNotification, mygo.ClearNotifications
	}
	return d
}

// queueShowNotification posts the notification off the UI thread, as on macOS the first one waits for the user to allow notifications. One the user did not allow is not shown and is not an error; nothing in the window waits for it, so another failure is only logged.
func queueShowNotification(id, title, body string) {
	n := mygo.NewNotification(mygo.NotificationOptions{ID: id, Title: title, Body: body})
	go func() {
		if err := n.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
			log.Printf("kando: notification for job %s: %v", id, err)
		}
	}()
}

// queueDesktopReady starts telling the system about the queue once main's window exists. A click on a notification of this run or of an earlier one opens what it is about (queueNotificationClicked); its listener is registered by restore, before App.Run, so the click that launched the app is heard too.
func (a *app) queueDesktopReady() { a.queueUseDesktop(queueSystemDesktop(a.win)) }

// queueUseDesktop has the queue reach the system through d from now on, and tells it at once what the icon should show: what the queue worked out before had nowhere to go.
func (a *app) queueUseDesktop(d queueDesktop) {
	a.queue.sys.desktop = d
	a.queue.sys.told = false
	a.queueTellSystem()
}

// queueListenForClicks registers the queue's listener for clicks on its notifications. restore calls it, which in main runs once, before App.Run. The listener is added to the process-wide mygo.App and is never removed, so each call adds another: a second call in main would open a clicked job twice, and each test that calls restoreSettings leaves one behind that never fires.
func (a *app) queueListenForClicks() {
	mygo.App.OnNotificationClick(a.queueNotificationClicked)
}

// queueNotificationClicked is a click on the notification of job id, which can come before the window exists, when the click launched the app. It waits until the app is ready, when main's window is made, and then shows the job on the UI thread.
func (a *app) queueNotificationClicked(id string) {
	mygo.App.WhenReady(func() { a.update(func() { a.queueShowJob(id) }) })
}

// queueShowJob brings the window forward on job id: a done film opens as a click on its poster opens it, and a failed one shows the library, where its card is. A job no longer in the open library, as one removed since or one of another library, only brings the window forward. UI thread only.
func (a *app) queueShowJob(id string) {
	if a.win != nil && a.win.IsDestroyed() {
		return
	}
	item, ok := films.Find(a.films(), id)
	switch {
	case !ok:
	case films.InQueue(item) || item.VideoPath == "":
		a.Select(pageFilms)
	case a.page != pageFilms || a.openKey != item.Key:
		a.Select(pageFilms)
		a.Open(item)
	}
	a.remoteRaise()
}

// queueNotifyEnded posts a notification for job, which show just recorded, when it is a job the queue ran that has just ended, done or failed, while the app is in the background and the user wants to be told. A job the user stopped, or that an earlier version stopped when another library was opened, is not news to them.
func (a *app) queueNotifyEnded(was core.Stage, job core.Job) {
	d := a.queue.sys.desktop
	if d.notify == nil || !a.queue.sys.notify || job.ID != a.queue.active || was == job.Stage || !job.Stage.Done() {
		return
	}
	if errors.Is(job.Err, context.Canceled) || errors.Is(job.Err, core.ErrLibrarySwitched) {
		return
	}
	if d.active != nil && d.active() {
		return
	}
	body := locales.QueueNotifyDone()
	if job.Stage == core.StageFailed {
		body = locales.QueueNotifyFailed(a.queueFailReason(job))
	}
	d.notify(job.ID, jobName(job), body)
}

// queueFailReason is what the card of the failed job says went wrong: its remedy's headline when the app knows why its download failed, or else the reason in the user's words.
func (a *app) queueFailReason(job core.Job) string {
	item := films.FromJob(job, jobName(job), statusText(job))
	if r, ok := a.FailureRemedy(item); ok {
		return r.Headline
	}
	reason, _ := failureHint(item)
	return reason
}

// queueTellSystem puts on the app's icon how many jobs of the open library are still to do, the running one and those waiting, and how far the running one has gone. It tells the system only what changed since it last did.
func (a *app) queueTellSystem() {
	s := &a.queue.sys
	badge, progress := a.queueOutstanding(), a.queueProgressBar()
	if s.desktop.badge != nil && (!s.told || badge != s.badge) {
		s.desktop.badge(badge)
	}
	if s.desktop.progress != nil && (!s.told || progress != s.progress) {
		s.desktop.progress(progress)
	}
	s.told, s.badge, s.progress = true, badge, progress
}

// queueOutstanding is how many jobs are still to do: the running one, and those waiting that pump will run, in the open library or another the app knows (queueCanRun). Jobs waiting in a library the app has forgotten start only once it is opened again.
func (a *app) queueOutstanding() int {
	n := 0
	for _, job := range a.library.Jobs {
		if job.ID == a.queue.active || (job.Stage == core.StageQueued && a.queueCanRun(job)) {
			n++
		}
	}
	return n
}

// queueProgressBar is how far the running job has gone in its stage, in whole percents, so a tool reporting many times a second does not redraw the icon each time; it is indeterminate while the stage has not said, and none when no job runs.
func (a *app) queueProgressBar() mygo.ProgressBar {
	if a.queue.active == "" {
		return mygo.ProgressBar{}
	}
	job, ok := a.library.Find(a.queue.active)
	if !ok || job.Stage.Done() {
		return mygo.ProgressBar{}
	}
	if !job.Progress.Known {
		return mygo.ProgressBar{State: mygo.ProgressIndeterminate}
	}
	return mygo.ProgressBar{State: mygo.ProgressNormal, Value: math.Round(job.Progress.Fraction*100) / 100}
}

// queueBecameActive removes the queue's notifications as the app comes to the front, where the window shows what they said.
func (a *app) queueBecameActive() {
	if clear := a.queue.sys.desktop.clear; clear != nil {
		clear()
	}
}

// The queue's part of settings.App, for Settings › 通用.

func (a *app) JobNotifications() bool { return a.queue.sys.notify }

func (a *app) SetJobNotifications(on bool) { a.queue.sys.notify = on }
