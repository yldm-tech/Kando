package main

import (
	"errors"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/session"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// queueToasts has toaster tell what happens to the computers' queues while the app is open: a job that finished, as a banner whose Open opens its film (openFilm), a link a computer took, and an action that failed.
// TODO(nsgo#59): tell a finished job with a push notification while the app is closed too; NSGo has no remote push (APNs, FCM) yet, so the banner shows only while the app is open https://github.com/nsgo-dev/nsgo/issues/59
func queueToasts(cx *core.Scope, s *session.Session, toaster *ui.Toaster, openFilm func(desktop string, lib sync.Library, f sync.Film)) {
	name := func(desktop string) string { d, _ := s.Desktop(desktop); return d.Name }
	s.OnQueue(session.QueueEvents{
		Done: func(desktop string, job sync.Job) {
			t := ui.ToastContent{ID: "job-done-" + job.ID, Type: ui.ToastSuccess, Title: locales.JobsDoneTitle(cx, name(desktop)), Description: locales.JobsDoneBody(cx, job.Title)}
			if job.Film != "" {
				t.Action = locales.JobsOpen(cx)
				t.OnAction = func() {
					s.JobFilm(desktop, job, func(lib sync.Library, f sync.Film, ok bool) {
						if ok {
							openFilm(desktop, lib, f)
						}
					})
				}
			}
			toaster.Show(t)
		},
		Sent: func(desktop string, created sync.JobCreated) {
			title := locales.JobsSent(cx, name(desktop))
			switch {
			case created.Pending:
				title = locales.JobsSentPending(cx, name(desktop))
			case created.Existing:
				title = locales.JobsSentExisting(cx, name(desktop))
			}
			toaster.Show(ui.ToastContent{Type: ui.ToastInfo, Title: title})
		},
		Failed: func(desktop string, err error) {
			// A computer that answered refused the action, as for a job whose state changed meanwhile; one that did not answer is out of reach.
			title := locales.ComputersUnreachable(cx, name(desktop))
			var e *sync.Error
			switch p := session.ProblemOf(err); {
			case p == session.Unpaired || p == session.Certificate:
				title = locales.ComputersUnpaired(cx, name(desktop))
			case errors.As(err, &e):
				title = locales.JobsActionFailed(cx, name(desktop))
			}
			toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: title})
		},
	})
}
