package main

import (
	"context"
	"errors"
	"log"
	"math"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
)

// What the queue tells the system outside the window: a notification when a job it ran is done or failed while the app is in the background, which opens the film, or shows its card, when it is clicked; the number of jobs still to do as the badge on the app's icon (the Dock's on macOS, the taskbar button's on Windows; Linux shows none); and how far the running job has gone in its stage as the app's progress bar, which NSGo cannot show yet.

// queueBar is the progress bar on the app's icon: none while Shown is false, a bar moving to and fro while Indeterminate, and else Value of the way, from 0 to 1.
type queueBar struct {
	Shown, Indeterminate bool
	Value                float64
}

// queueDesktop is how the queue reaches the system. Every field is nil until main's window is ready (queueHooks' ready sets them, queueSystemDesktop), and in tests that do not set them, so the queue then tells the system nothing.
type queueDesktop struct {
	// active reports whether the app is the one in front, whose window already shows what a notification would say.
	active func() bool
	// notify posts a notification with id, title and body, and returns without waiting for the system. clear removes the app's notifications.
	notify func(id, title, body string)
	clear  func()
	// badge shows n on the app's icon, and removes the badge for 0. progress shows the progress bar, or removes it for the zero queueBar.
	badge    func(n int)
	progress func(p queueBar)
}

// queueSystem is the queue's state outside the window: how it reaches the system, whether the user wants notifications (Settings › 通用), and what it last put on the app's icon.
type queueSystem struct {
	desktop queueDesktop
	notify  bool
	// told is false until the queue first put the badge and the progress bar on the icon; badge and progress are what it put there, so it tells the system only what changed.
	told     bool
	badge    int
	progress queueBar
	// shown are the IDs of the notifications posted since the app last came to the front, which clear removes: NSGo removes one at a time.
	shown []string
}

func newQueueSystem() queueSystem { return queueSystem{notify: true} }

// queueSystemDesktop reaches the system through NSGo, under the app's scope cx, which lives as long as the app does.
func (a *app) queueSystemDesktop(cx *ns.Scope) queueDesktop {
	return queueDesktop{
		active: func() bool { return ns.Untracked(cx, cx.Env().Lifecycle) == ns.LifecycleActive },
		notify: func(id, title, body string) { a.queueShowNotification(cx, id, title, body) },
		clear: func() {
			// TODO(nsgo): remove every notification of the app's, those of earlier runs too, as MyGo's ClearNotifications did; NSGo closes one ID at a time (core.Scope.CloseNotification), so only the ones this run posted go
			for _, id := range a.queue.sys.shown {
				cx.CloseNotification(id)
			}
			a.queue.sys.shown = nil
		},
		badge: cx.SetBadge,
		progress: func(queueBar) {
			// TODO(nsgo#116): show the running job's progress on the Dock icon, the taskbar button and the launcher entry, indeterminate while its stage has not said how far it is, as MyGo's Window.SetProgressBar did https://github.com/nsgo-dev/nsgo/issues/116
		},
	}
}

// queueShowNotification posts the notification, asking the user to allow them the first time, as macOS does before the first one shows. One the user did not allow is not shown and is not an error; nothing in the window waits for it, so another failure is only logged.
func (a *app) queueShowNotification(cx *ns.Scope, id, title, body string) {
	a.queue.sys.shown = append(a.queue.sys.shown, id)
	cx.RequestNotificationPermission(func(p ns.NotificationPermission) {
		if p != ns.NotificationsGranted {
			return
		}
		if err := cx.ShowNotification(ns.Notification{ID: id, Title: title, Body: body}, nil); err != nil {
			log.Printf("kando: notification for job %s: %v", id, err)
		}
	})
}

// queueDesktopReady starts telling the system about the queue once main's window exists, and listens for clicks on the queue's notifications, of this run or of an earlier one: a click opens what it is about (queueNotificationClicked). NSGo keeps a click that came before the listener, as the one that launched the app, for it. All of it lives under the app's scope.
func (a *app) queueDesktopReady() {
	if a.cx == nil {
		return
	}
	a.cx.Run(func() {
		a.cx.OnNotificationPress(live.On(a, a.queueNotificationClicked))
	})
	a.queueUseDesktop(a.queueSystemDesktop(a.cx))
}

// queueUseDesktop has the queue reach the system through d from now on, and tells it at once what the icon should show: what the queue worked out before had nowhere to go.
func (a *app) queueUseDesktop(d queueDesktop) {
	a.queue.sys.desktop = d
	a.queue.sys.told = false
	a.queueTellSystem()
}

// queueNotificationClicked is a click on the notification of job id, which brings the app to the front: it shows the job. It runs on the engine's goroutine once the app runs.
func (a *app) queueNotificationClicked(id string) { a.queueShowJob(id) }

// queueShowJob brings the window forward on job id: a done film opens as a click on its poster opens it, and a failed one shows the library, where its card is. A job no longer in the open library, as one removed since or one of another library, only brings the window forward. Engine goroutine only.
func (a *app) queueShowJob(id string) {
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
	m := a.messages()
	body := m.QueueNotifyDone()
	if job.Stage == core.StageFailed {
		body = m.QueueNotifyFailed(a.queueFailReason(job))
	}
	d.notify(job.ID, jobName(job), body)
}

// queueFailReason is what the card of the failed job says went wrong: its remedy's headline when the app knows why its download failed, or else the reason in the user's words.
func (a *app) queueFailReason(job core.Job) string {
	item := films.FromJob(job, jobName(job), a.statusOf(job))
	if r, ok := a.FailureRemedy(item); ok {
		return r.Headline
	}
	reason, _ := failureHint(a.messages(), item)
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
func (a *app) queueProgressBar() queueBar {
	if a.queue.active == "" {
		return queueBar{}
	}
	job, ok := a.library.Find(a.queue.active)
	if !ok || job.Stage.Done() {
		return queueBar{}
	}
	if !job.Progress.Known {
		return queueBar{Shown: true, Indeterminate: true}
	}
	return queueBar{Shown: true, Value: math.Round(job.Progress.Fraction*100) / 100}
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
