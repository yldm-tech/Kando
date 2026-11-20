// Package jobs is the Jobs page, the queues of the paired computers as the phone follows them over protocol v2: under each computer how it stands, the links sent from this phone it has not taken yet, and its jobs with each step's progress, the actions on them and, for one that failed, what can be done about it. On a phone the page opens from the Library; on a tablet it is a section of its own.
package jobs

import (
	"context"
	"strconv"
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where the page leads.
type Actions struct {
	// Add opens the Add sheet.
	Add func()
	// Pair opens pairing.
	Pair func()
	// Film opens the page of a film a job made.
	Film func(desktop string, lib sync.Library, f sync.Film)
	// Back leaves the page, nil where it is a section's root, as on a tablet.
	Back func()
}

// Page is the Jobs page.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	title := func() string { return locales.JobsTitle(cx) }
	ref := &core.Ref{}
	body := core.Scroll{TestID: "jobs-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{StyleFunc: look.Column(cx), Children: []core.Node{
			core.Show{When: func() bool { return len(s.Desktops()) == 0 }, Then: func(cx *core.Scope) core.Node {
				return look.Empty{TestID: "jobs-unpaired", Icon: lucide.Laptop, Title: func() string { return locales.JobsUnpaired(cx) },
					Action: look.Button{Kind: look.Tinted, Text: func() string { return locales.FilmsPair(cx) }, TestID: "jobs-pair", OnPress: act.Pair}}
			}},
			core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, _ func() int) core.Node {
				return section(cx, s, d().ID, act)
			}),
		}},
	}}
	addLabel := func() string { return locales.AddTitle(cx) }
	add := ui.IconButton{TestID: "jobs-add", Icon: lucide.Plus, LabelFunc: addLabel, Size: 44, OnPress: act.Add}
	symbols := func() []core.HeaderSymbol { return []core.HeaderSymbol{{Name: "plus", Label: addLabel()}} }
	page := base.NavPage{TestID: "jobs-page", Children: []core.Node{body}}
	if act.Back == nil {
		return look.TabRoot(cx, page, title, []core.Node{add}, symbols)
	}
	page.TitleFunc = title
	return ui.NavPage(cx, page, ui.NavHeader{TitleFunc: title, OnBack: act.Back, Scroll: ref, Trailing: []core.Node{add}, Symbols: symbols})
}

// section is a computer's part of the page: its card, the links waiting for it and its jobs.
func section(cx *core.Scope, s *session.Session, desktop string, act Actions) core.Node {
	queue := func() session.Queue { return s.Queue(desktop) }
	links := func() []session.Outgoing {
		var out []session.Outgoing
		for _, o := range s.Outgoing() {
			if o.Desktop == desktop {
				out = append(out, o)
			}
		}
		return out
	}
	colors := ui.ColorsOf(cx)
	online := func() bool { return reached(queue()) }
	return core.View{TestID: "jobs-desktop-" + desktop, Style: core.Style{Gap: 12}, Children: []core.Node{
		computerCard(cx, s, desktop),
		// A computer that made the phone read-only keeps showing its jobs, without the buttons that would change them, and says why.
		core.Show{When: func() bool { return online() && queue().ReadOnly }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "jobs-read-only-" + desktop, ContentFunc: func() string { return locales.JobsReadOnly(cx, name(s, desktop)) }, Style: look.Caption.Merge(core.Style{Padding: core.Symmetric(0, 4)}), StyleFunc: ui.Colored(colors.Muted)}
		}},
		core.Show{When: func() bool { return online() && !queue().ReadOnly && len(queue().Jobs) == 0 && len(links()) == 0 }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "jobs-empty-" + desktop, ContentFunc: func() string { return locales.JobsEmpty(cx) }, Style: look.Caption.Merge(core.Style{Padding: core.Symmetric(0, 4)}), StyleFunc: ui.Colored(colors.Muted)}
		}},
		core.Each(links, func(o session.Outgoing) string { return o.ID }, func(cx *core.Scope, o func() session.Outgoing, _ func() int) core.Node {
			return linkCard(cx, s, desktop, o)
		}),
		core.Each(func() []sync.Job { return queue().Jobs }, func(j sync.Job) string { return j.ID }, func(cx *core.Scope, j func() sync.Job, _ func() int) core.Node {
			return jobCard(cx, s, desktop, j, act)
		}),
	}}
}

// name is desktop's name, "" once it is unpaired.
func name(s *session.Session, desktop string) string {
	d, _ := s.Desktop(desktop)
	return d.Name
}

// computerCard is a computer's name, whether it answers and what its queue is doing.
func computerCard(cx *core.Scope, s *session.Session, desktop string) core.Node {
	colors := ui.ColorsOf(cx)
	queue := func() session.Queue { return s.Queue(desktop) }
	online := func() bool { return reached(queue()) }
	return core.View{TestID: "jobs-computer-" + desktop, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 14), CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} },
		Children: []core.Node{
			ui.Icon(lucide.Laptop, 26, colors.Muted),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return name(s, desktop) }, Role: core.RoleHeader, MaxLines: 1, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
				core.Text{TestID: "jobs-summary-" + desktop, ContentFunc: func() string { return summary(cx, s, desktop) }, LiveRegion: core.LiveRegionPolite, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
			}},
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4},
					StyleFunc: func() core.Style {
						if online() {
							return core.Style{Background: colors.Up()}
						}
						return core.Style{Background: colors.Faint()}
					}},
				core.Text{TestID: "jobs-reach-" + desktop, ContentFunc: func() string {
					switch {
					case online() && queue().Relayed:
						return locales.JobsViaStorage(cx)
					case online():
						return locales.JobsOnline(cx)
					}
					return locales.JobsOffline(cx)
				}, MaxLines: 1, Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: func() core.Style {
					if online() {
						return core.Style{Color: colors.Up()}
					}
					return core.Style{Color: colors.Muted()}
				}},
			}},
		}}
}

// reached reports whether the phone reaches the computer of q: online, and not through a storage that refuses it, which the card's summary says instead (summary), as the follower may not have found the computer out of reach yet.
func reached(q session.Queue) bool {
	return q.Reach == session.Online && !(q.Relayed && q.Storage != sync.RelayNoFailure)
}

// summary is what a computer's queue is doing, as its card says it, and that it is reached through the user's storage when it is, or why that storage failed the phone.
func summary(cx *core.Scope, s *session.Session, desktop string) string {
	q := s.Queue(desktop)
	if q.Relayed && q.Storage != sync.RelayNoFailure && q.Reach != session.Revoked {
		return StorageFailure(cx, q.Storage, name(s, desktop))
	}
	switch q.Reach {
	case session.Reaching:
		return locales.JobsConnecting(cx)
	case session.Offline:
		if q.Relayed {
			return locales.JobsOfflineRelay(cx)
		}
		return locales.JobsOfflineSummary(cx)
	case session.Older:
		return locales.JobsOlder(cx)
	case session.Revoked:
		return locales.ComputersUnpaired(cx, name(s, desktop))
	}
	if q.Relayed {
		return locales.JobsRelayed(cx, queueSummary(cx, q))
	}
	return queueSummary(cx, q)
}

// StorageFailure is why the user's storage failed the phone as it follows desktop, the computer's name, through it (sync.RelayFailure), in the words the computer's Phone Sync page uses for the same failure; "" for none.
func StorageFailure(cx *core.Scope, f sync.RelayFailure, desktop string) string {
	switch f {
	case sync.RelayLapsed:
		return locales.JobsStorageLapsed(cx, desktop)
	case sync.RelayKeys:
		return locales.JobsStorageKeys(cx, desktop)
	case sync.RelayDenied:
		return locales.JobsStorageDenied(cx, desktop)
	case sync.RelayNoBucket:
		return locales.JobsStorageNoBucket(cx, desktop)
	case sync.RelayFull:
		return locales.JobsStorageFull(cx, desktop)
	case sync.RelayUnreachable:
		return locales.JobsStorageUnreachable(cx)
	case sync.RelayFailed:
		return locales.JobsStorageFailed(cx)
	}
	return ""
}

// queueSummary is what a computer's queue is doing: how many jobs run or wait, and how many failed.
func queueSummary(cx *core.Scope, q session.Queue) string {
	active, failed := 0, 0
	for _, j := range q.Jobs {
		switch j.State {
		case sync.JobRunning, sync.JobWaiting, sync.JobPaused:
			active++
		case sync.JobFailed:
			failed++
		}
	}
	switch {
	case active > 0 && failed > 0:
		return locales.JobsActiveFailed(cx, active, failed)
	case active > 0:
		return locales.JobsActive(cx, active)
	case failed > 0:
		return locales.JobsFailedCount(cx, failed)
	}
	return locales.JobsIdle(cx)
}

// card is the look of a job's or a link's card, ringed in warning's color while warn reports true.
func card(cx *core.Scope, testID string, warn func() bool, children ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: testID, Style: core.Style{Padding: core.All(14), Gap: 12, CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1.5},
		StyleFunc: func() core.Style {
			st := core.Style{Background: colors.Surface(), BorderColor: ui.Alpha(colors.Warn(), 0)}
			if warn() {
				st.BorderColor = ui.Alpha(colors.Warn(), 0.6)
			}
			return st
		}, Children: children}
}

// linkCard is a link sent from this phone that its computer has not taken yet: waiting for it, or refused and why, with the way to send it again or drop it.
func linkCard(cx *core.Scope, s *session.Session, desktop string, o func() session.Outgoing) core.Node {
	colors := ui.ColorsOf(cx)
	id := o().ID
	refused := func() bool { return o().Refusal != nil }
	return card(cx, "jobs-link-"+id, refused,
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			thumb(cx, func() core.ImageSource { return core.ImageSource{} }),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return o().URL }, MaxLines: 2, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Text)},
				core.Text{TestID: "jobs-link-status-" + id, ContentFunc: func() string {
					if r := o().Refusal; r != nil {
						return refusal(cx, *r, name(s, desktop))
					}
					if o().Relayed {
						return locales.JobsInStorage(cx, name(s, desktop))
					}
					return locales.JobsWaitingFor(cx, name(s, desktop))
				}, Style: look.Caption, StyleFunc: func() core.Style {
					if refused() {
						return core.Style{Color: theme.Of(cx).WarnText()}
					}
					return core.Style{Color: colors.Muted()}
				}},
			}},
			ui.IconButton{TestID: "jobs-link-forget-" + id, Icon: lucide.X, LabelFunc: func() string { return locales.JobsForget(cx) }, Size: 34, OnPress: func() { s.Forget(id) }},
		}},
		core.Show{When: refused, Then: func(cx *core.Scope) core.Node {
			return pills(pill(cx, "jobs-link-resend-"+id, true, func() string { return locales.CommonRetry(cx) }, func() { s.Resend(id) }))
		}},
	)
}

// refusal is why a computer refused a link.
func refusal(cx *core.Scope, r sync.ErrorBody, desktop string) string {
	switch r.Code {
	case sync.CodeLinksOff:
		return locales.JobsLinksOff(cx, desktop)
	case sync.CodeInvalid:
		return locales.JobsNotAVideo(cx)
	case sync.CodeUnsupported:
		return locales.JobsOlder(cx)
	case sync.CodeNotReady:
		return locales.JobsNotReady(cx, desktop, r.Message)
	case sync.CodeReadOnly:
		return locales.JobsReadOnly(cx, desktop)
	}
	return locales.JobsRefused(cx, desktop, r.Message)
}

// jobCard is a job: its poster once it made a film, its title and where it stands, the buttons that pause, resume, cancel or open it, its steps while it runs or waits, and what can be done about it once it failed or was canceled.
func jobCard(cx *core.Scope, s *session.Session, desktop string, j func() sync.Job, act Actions) core.Node {
	colors := ui.ColorsOf(cx)
	id := j().ID
	q := func() session.Queue { return s.Queue(desktop) }
	reached := func() bool { return q().Reach == session.Online }
	state := func() string { return j().State }
	busy := func() bool { return s.Busy(desktop, id) || !reached() }
	stopped := func() bool { return state() == sync.JobFailed || state() == sync.JobCanceled }
	unfinished := func() bool {
		st := state()
		return st == sync.JobRunning || st == sync.JobWaiting || st == sync.JobPaused
	}
	type posterSource struct {
		c    *sync.Client
		film string
	}
	poster := core.NewResource(cx, func() (posterSource, bool) {
		c := s.Client(desktop)
		return posterSource{c, j().Film}, c != nil && j().Film != ""
	}, func(ctx context.Context, src posterSource, _ core.ResourceInfo) ([]byte, error) {
		return s.Poster(ctx, src.c, desktop, src.film)
	})
	source := func() core.ImageSource {
		if data, ok := poster.Get(); ok && len(data) > 0 {
			return core.ImageSource{Data: data}
		}
		return core.ImageSource{}
	}
	open := func() { s.JobFilm(desktop, j(), func(lib sync.Library, f sync.Film) { act.Film(desktop, lib, f) }) }
	confirmCancel := func() {
		title := j().Title
		cx.ShowAlert(core.AlertConfig{Title: locales.JobsCancelTitle(cx, title, name(s, desktop)), Sheet: !look.Tablet(cx), Cancelable: true, Actions: []core.AlertAction{
			{Label: locales.JobsKeep(cx), Role: core.AlertCancel},
			{Label: locales.JobsCancelConfirm(cx), Role: core.AlertDestructive},
		}}, func(i int) {
			if i == 1 {
				s.Act(desktop, sync.OpJobCancel, id)
			}
		})
	}
	controls := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
		core.Show{When: func() bool { return unfinished() && state() != sync.JobPaused && q().Can(sync.OpJobPause) }, Then: func(cx *core.Scope) core.Node {
			return ui.IconButton{TestID: "jobs-pause-" + id, Icon: lucide.Pause, LabelFunc: func() string { return locales.JobsPause(cx) }, Size: 34, Disabled: busy,
				OnPress: func() { s.Act(desktop, sync.OpJobPause, id) }}
		}},
		core.Show{When: func() bool { return state() == sync.JobPaused && q().Can(sync.OpJobResume) }, Then: func(cx *core.Scope) core.Node {
			return ui.IconButton{TestID: "jobs-resume-" + id, Icon: lucide.Play, LabelFunc: func() string { return locales.JobsResume(cx) }, Size: 34, Disabled: busy,
				OnPress: func() { s.Act(desktop, sync.OpJobResume, id) }}
		}},
		core.Show{When: func() bool { return unfinished() && q().Can(sync.OpJobCancel) }, Then: func(cx *core.Scope) core.Node {
			return ui.IconButton{TestID: "jobs-cancel-" + id, Icon: lucide.X, LabelFunc: func() string { return locales.JobsCancel(cx) }, Size: 34, Disabled: busy, OnPress: confirmCancel}
		}},
		core.Show{When: func() bool { return state() == sync.JobDone && j().Film != "" }, Then: func(cx *core.Scope) core.Node {
			return pillBusy(cx, "jobs-open-"+id, true, func() string { return locales.JobsOpen(cx) }, open, func() bool { return s.Busy(desktop, id) })
		}},
	}}
	return card(cx, "jobs-job-"+id, func() bool { return state() == sync.JobFailed },
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			thumb(cx, source),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
				core.Text{TestID: "jobs-title-" + id, ContentFunc: func() string { return title(j()) }, MaxLines: 2, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Text)},
				core.Text{TestID: "jobs-status-" + id, ContentFunc: func() string { return status(cx, s, desktop, j()) }, MaxLines: 2, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
			}},
			controls,
		}},
		core.Show{When: stopped, Then: func(cx *core.Scope) core.Node { return remedy(cx, s, desktop, j) }},
		core.Show{When: unfinished, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 10}, Children: []core.Node{
				core.Each(func() []sync.Step { return j().Steps }, func(st sync.Step) string { return st.Name }, func(cx *core.Scope, st func() sync.Step, i func() int) core.Node {
					return stepRow(cx, id, st, i, func() bool { return state() == sync.JobRunning && reached() })
				}),
			}}
		}},
	)
}

// title is what a job is called: its title, else its link.
func title(j sync.Job) string {
	if j.Title != "" {
		return j.Title
	}
	return j.Link
}

// status is where a job stands, as the line under its title says it, with the phone that sent it.
func status(cx *core.Scope, s *session.Session, desktop string, j sync.Job) string {
	var text string
	q := s.Queue(desktop)
	switch {
	case q.Reach != session.Online && (j.State == sync.JobRunning || j.State == sync.JobWaiting || j.State == sync.JobPaused):
		text = locales.JobsWaitingFor(cx, name(s, desktop))
	case j.State == sync.JobRunning:
		text = locales.JobsRunning(cx)
		for _, st := range j.Steps {
			if st.State == sync.StepRunning {
				text = stepName(cx, st.Name)
				if st.Progress >= 0 {
					text = locales.JobsStepPercent(cx, text, int(st.Progress*100))
				}
			}
		}
	case j.State == sync.JobWaiting:
		text = locales.JobsWaiting(cx)
	case j.State == sync.JobPaused:
		text = locales.JobsPaused(cx)
	case j.State == sync.JobDone:
		text = locales.JobsDone(cx)
	case j.State == sync.JobFailed:
		text = locales.JobsFailed(cx)
	case j.State == sync.JobCanceled:
		text = locales.JobsCanceled(cx)
	}
	if j.Device != "" {
		return locales.JobsFrom(cx, text, j.Device)
	}
	return text
}

// stepName is what a step is called.
func stepName(cx *core.Scope, name string) string {
	switch name {
	case sync.StepDownload:
		return locales.JobsStepDownload(cx)
	case sync.StepTranscribe:
		return locales.JobsStepTranscribe(cx)
	case sync.StepTranslate:
		return locales.JobsStepTranslate(cx)
	}
	return name
}

// stepRow is a step of a job: its number, or a check once done, its name and where it stands, and its bar, sweeping while it runs without a known share and moving is true.
func stepRow(cx *core.Scope, job string, st func() sync.Step, i func() int, moving func() bool) core.Node {
	colors := ui.ColorsOf(cx)
	is := func(state string) func() bool { return func() bool { return st().State == state } }
	done, running := is(sync.StepDone), is(sync.StepRunning)
	detail := func() string {
		switch st().State {
		case sync.StepDone:
			return locales.JobsStepDone(cx)
		case sync.StepSkipped:
			return locales.JobsStepSkipped(cx)
		case sync.StepFailed:
			return locales.JobsFailed(cx)
		case sync.StepRunning:
			if p := st().Progress; p >= 0 {
				return locales.JobsPercent(cx, int(p*100))
			}
			return ""
		}
		return locales.JobsWaiting(cx)
	}
	return core.View{TestID: "jobs-step-" + job + "-" + st().Name, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
		// The mark repeats the step's name and state, which the texts beside it say.
		core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Width: core.Pt(22), Height: core.Pt(22), CornerRadius: 11, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
			StyleFunc: func() core.Style {
				switch {
				case done():
					return core.Style{Background: colors.Up(), Color: colors.OnAccent()}
				case running():
					return core.Style{Background: colors.Accent(), Color: colors.OnAccent()}
				}
				return core.Style{Background: colors.Track(), Color: colors.Muted()}
			},
			Children: []core.Node{core.Show{When: done,
				Then: func(*core.Scope) core.Node { return ui.Icon(lucide.Check, 13, ui.Inherited) },
				Else: func(cx *core.Scope) core.Node {
					return core.Text{ContentFunc: func() string { return strconv.Itoa(i() + 1) }, Style: core.Style{FontSize: 11, FontWeight: core.FontWeightBold}}
				}}}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 5}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween, Gap: 8}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return stepName(cx, st().Name) }, MaxLines: 1, Style: look.Caption, StyleFunc: func() core.Style {
					switch {
					case running():
						return core.Style{Color: colors.Accent(), FontWeight: core.FontWeightSemibold}
					case done():
						return core.Style{Color: colors.Text()}
					}
					return core.Style{Color: colors.Muted()}
				}},
				core.Text{TestID: "jobs-step-detail-" + job + "-" + st().Name, ContentFunc: detail, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
			}},
			// TODO(nsgo#145): ui.Progress.Label is a string only (ui/progress.go:12-19), so the bar's spoken name keeps the language it was built in; pass a LabelFunc once Progress has one https://github.com/nsgo-dev/nsgo/issues/145
			ui.Progress{Label: stepName(cx, st().Name),
				Value: func() float32 {
					switch st().State {
					case sync.StepDone:
						return 100
					case sync.StepRunning:
						return float32(max(st().Progress, 0) * 100)
					}
					return 0
				},
				Indeterminate: func() bool { return running() && st().Progress < 0 && moving() }},
		}},
	}}
}

// remedy is what a job that failed or was canceled says of it, and what can be done: its computer updating its downloader, trying again, or the way to fix it on the computer first.
func remedy(cx *core.Scope, s *session.Session, desktop string, j func() sync.Job) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	id := j().ID
	q := func() session.Queue { return s.Queue(desktop) }
	failure := func() sync.Failure {
		if f := j().Failure; f != nil {
			return *f
		}
		return sync.Failure{}
	}
	canceled := func() bool { return j().State == sync.JobCanceled }
	update := func() bool {
		return !canceled() && failure().Remedy == sync.RemedyUpdateYTDLP && q().Can(sync.OpUpdateYTDLP)
	}
	retry := func() bool { return q().Can(sync.OpJobRetry) }
	busy := func() bool { return s.Busy(desktop, id) || s.Updating(desktop) || q().Reach != session.Online }
	return core.View{TestID: "jobs-remedy-" + id, Style: core.Style{Gap: 6, Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style {
			if canceled() {
				return core.Style{Background: colors.Inset()}
			}
			return core.Style{Background: tints.Warn()}
		},
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
				core.Show{When: func() bool { return !canceled() }, Then: func(*core.Scope) core.Node { return ui.Icon(lucide.TriangleAlert, 16, colors.Warn) }},
				core.Text{TestID: "jobs-failure-" + id, ContentFunc: func() string {
					if canceled() {
						return locales.JobsCanceledTitle(cx, name(s, desktop))
					}
					return failureTitle(cx, failure())
				}, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
			}},
			core.Show{When: func() bool { return !canceled() && failureBody(cx, failure(), name(s, desktop)) != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "jobs-failure-body-" + id, ContentFunc: func() string { return failureBody(cx, failure(), name(s, desktop)) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}
			}},
			pills(
				core.Show{When: update, Then: func(cx *core.Scope) core.Node {
					return pillBusy(cx, "jobs-update-"+id, true, func() string { return locales.JobsUpdateRetry(cx) }, func() { s.UpdateYTDLP(desktop) }, busy)
				}},
				core.Show{When: func() bool { return retry() && !update() }, Then: func(cx *core.Scope) core.Node {
					return pillBusy(cx, "jobs-retry-"+id, true, func() string {
						if failure().Remedy == sync.RemedyDesktop {
							return locales.JobsFixedRetry(cx)
						}
						return locales.CommonRetry(cx)
					}, func() { s.Act(desktop, sync.OpJobRetry, id) }, busy)
				}},
			),
		}}
}

// failureTitle is what went wrong, by the failure's code; the computer's own words for a code the phone does not know.
func failureTitle(cx *core.Scope, f sync.Failure) string {
	switch f.Code {
	case sync.FailNetwork:
		return locales.JobsFailNetwork(cx)
	case sync.FailRateLimited:
		return locales.JobsFailRateLimited(cx)
	case sync.FailHost:
		return locales.JobsFailHost(cx)
	case sync.FailBadLink:
		return locales.JobsFailBadLink(cx)
	case sync.FailUnsupported:
		return locales.JobsFailUnsupported(cx)
	case sync.FailOutdated:
		return locales.JobsFailOutdated(cx)
	case sync.FailLogin:
		return locales.JobsFailLogin(cx)
	case sync.FailLoginExpired:
		return locales.JobsFailLoginExpired(cx)
	case sync.FailCookies:
		return locales.JobsFailCookies(cx)
	case sync.FailMembers:
		return locales.JobsFailMembers(cx)
	case sync.FailAge:
		return locales.JobsFailAge(cx)
	case sync.FailRegion:
		return locales.JobsFailRegion(cx)
	case sync.FailPrivate:
		return locales.JobsFailPrivate(cx)
	case sync.FailDRM:
		return locales.JobsFailDRM(cx)
	case sync.FailHiddenVideo:
		return locales.JobsFailHidden(cx)
	case sync.FailNoSpeech:
		return locales.JobsFailNoSpeech(cx)
	case sync.FailDesktopSetup:
		return locales.JobsFailSetup(cx)
	case sync.FailTranslation:
		return locales.JobsFailTranslation(cx)
	case sync.FailOther, "":
		return locales.JobsFailOther(cx)
	}
	if f.Message != "" {
		return f.Message
	}
	return locales.JobsFailOther(cx)
}

// failureBody is what can be done about a failure, by its remedy, with the computer's own words where they say what to set up.
func failureBody(cx *core.Scope, f sync.Failure, desktop string) string {
	switch f.Remedy {
	case sync.RemedyUpdateYTDLP:
		return locales.JobsRemedyUpdate(cx, desktop)
	case sync.RemedyRetry:
		return locales.JobsRemedyRetry(cx)
	case sync.RemedyDesktop:
		if f.Code == sync.FailDesktopSetup && f.Message != "" {
			return locales.JobsRemedySetup(cx, desktop, f.Message)
		}
		return locales.JobsRemedyDesktop(cx, desktop)
	}
	return ""
}

// thumb is a job's poster, in the 16:9 of a video, a film's mark until it has one.
func thumb(cx *core.Scope, source func() core.ImageSource) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Width: core.Pt(80), AspectRatio: 16.0 / 9, CornerRadius: 8, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: ui.Filled(colors.Inset),
		Children: []core.Node{
			ui.Icon(lucide.Film, 20, colors.Faint),
			core.Image{SourceFunc: source, ResizeMode: core.Cover, FadeDuration: 150 * time.Millisecond, ElementsHidden: true,
				Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}},
		}}
}

// pills lays small buttons out in a row that wraps.
func pills(children ...core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8}, Children: children}
}

// pill is a small capsule button in the accent, filled where main is set.
func pill(cx *core.Scope, testID string, main bool, text func() string, press func()) core.Node {
	return pillBusy(cx, testID, main, text, press, func() bool { return false })
}

// pillBusy is pill taking no presses while busy reports true.
func pillBusy(cx *core.Scope, testID string, main bool, text func() string, press func(), busy func() bool) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	return base.Button{TestID: testID, LabelFunc: text, OnPress: func(core.PressEvent) { press() }, Disabled: busy,
		Style: core.Style{MinHeight: core.Pt(34), Padding: core.Symmetric(0, 14), CornerRadius: 17, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Active: &core.Style{Opacity: core.Alpha(0.7)}, Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
		StyleFunc: func() core.Style {
			if main {
				return core.Style{Background: colors.Accent(), Color: colors.OnAccent()}
			}
			return core.Style{Background: tints.Accent(), Color: colors.Accent()}
		},
		Children: []core.Node{core.Text{ContentFunc: text, MaxLines: 1, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold})}}}
}
