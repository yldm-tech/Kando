package main

import (
	"errors"
	"time"

	"kando/apps/mobile/brand"
	"kando/apps/mobile/jobs"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// queueToasts has toaster tell what happens to the computers' queues while the app is open: a job that finished, as a banner whose Open opens its film (openFilm), a link a computer took, a list it subscribed to, a film it translates again, deleted or downloads the video of, and an action that failed.
// TODO(nsgo#59): tell a finished job with a push notification while the app is closed too; NSGo has no remote push (APNs, FCM) yet, so the banner shows only while the app is open https://github.com/nsgo-dev/nsgo/issues/59
func queueToasts(cx *core.Scope, s *session.Session, toaster *ui.Toaster, openFilm func(desktop string, lib sync.Library, f sync.Film)) {
	name := func(desktop string) string { d, _ := s.Desktop(desktop); return d.Name }
	s.OnQueue(session.QueueEvents{
		Done: func(desktop string, job sync.Job) {
			// The design's banner of a finished job leads with the app's icon, opens its film from anywhere on it and stays a little longer than a toast.
			// TODO(nsgo): ui.Toasts and ui.ToastContent take no style of a toast's surface (ui/toast.go:354-363), so the banner is NSGo's card rather than the design's frosted banner.
			t := ui.ToastContent{ID: "job-done-" + job.ID, Type: ui.ToastSuccess, Title: locales.JobsDoneTitle(cx, name(desktop)), Description: locales.JobsDoneBody(cx, job.Title),
				Leading:  core.Image{Source: brand.Icon(), ResizeMode: core.Cover, ElementsHidden: true, Style: core.Style{Width: core.Pt(38), Height: core.Pt(38), CornerRadius: 9, BorderCurve: core.BorderCurveContinuous}},
				Duration: 5200 * time.Millisecond}
			if job.Film != "" {
				open := func() { s.JobFilm(desktop, job, func(lib sync.Library, f sync.Film) { openFilm(desktop, lib, f) }) }
				t.Action, t.OnAction, t.OnPress = locales.JobsOpen(cx), open, open
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
			toaster.Show(brief(ui.ToastContent{Type: ui.ToastSuccess, Title: title}))
		},
		Subscribed: func(desktop string, got sync.Subscribed) {
			title := locales.AddSubscribed(cx, got.Subscription.Name, name(desktop))
			if got.Existing {
				title = locales.AddSubscribedExisting(cx, got.Subscription.Name, name(desktop))
			}
			toaster.Show(brief(ui.ToastContent{Type: ui.ToastSuccess, Title: title}))
		},
		Acted: func(desktop, op, title string) {
			var text string
			switch op {
			case sync.OpFilmRetranslate:
				text = locales.FilmsRetranslating(cx, title, name(desktop))
			case sync.OpFilmDelete:
				text = locales.FilmsDeletedFromLibrary(cx, title, name(desktop))
			case sync.OpFilmVideo:
				text = locales.FilmVideoAsked(cx, title, name(desktop))
			default:
				return
			}
			toaster.Show(brief(ui.ToastContent{Type: ui.ToastSuccess, Title: text}))
		},
		Failed: func(desktop string, err error) {
			// A done job's film the computer does not serve is told as such; a computer that answered refused the action, as for a job whose state changed meanwhile; one that did not answer is out of reach.
			title := locales.ComputersUnreachable(cx, name(desktop))
			var e *sync.Error
			switch p := session.ProblemOf(err); {
			case errors.Is(err, session.ErrFilmGone):
				title = locales.JobsFilmGone(cx, name(desktop))
			case p == session.Unpaired || p == session.Certificate:
				title = locales.ComputersUnpaired(cx, name(desktop))
			case errors.As(err, &e) && e.Code == sync.CodeReadOnly:
				title = locales.JobsReadOnly(cx, name(desktop))
			case errors.As(err, &e):
				title = locales.JobsActionFailed(cx, name(desktop))
			case s.Queue(desktop).Relayed && s.Queue(desktop).Storage != sync.RelayNoFailure:
				// Away from the computer's network, the storage it goes through failed the phone; a computer that answered there refused the action itself, as above.
				title = jobs.StorageFailure(cx, s.Queue(desktop).Storage, name(desktop))
			}
			t := ui.ToastContent{Type: ui.ToastError, Title: title}
			// A computer not ready for it says why in its own words, as that the library is locked or it has no downloader; the protocol carries them in English.
			if e != nil && (e.Code == sync.CodeNotReady || e.Code == sync.CodeInvalid) && e.Message != "" {
				t.Description = e.Message
			}
			toaster.Show(t)
		},
	})
}

// brief is t as the design's toast of something done: a green check before its line, with no close button, gone after 2.6 seconds.
// TODO(nsgo): ui.Toasts and ui.ToastContent take no style of a toast's surface (ui/toast.go:354-363), so the toast is NSGo's card rather than the design's dark pill.
func brief(t ui.ToastContent) ui.ToastContent {
	t.Leading = ui.Icon(lucide.CircleCheck, 16, func() core.Color { return theme.StatusOK })
	t.NoClose = true
	t.Duration = 2600 * time.Millisecond
	return t
}
