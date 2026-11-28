// Package jobs is the Jobs page, the queues of the paired computers as the phone follows them over protocol v2: under each computer how it stands, the links sent from this phone it has not taken yet, and its jobs with each step's progress, the actions on them and, for one that failed, what can be done about it. It is a tab of its own, its root page, and a page pushed on a tab's stack where another page opens it.
package jobs

import (
	"context"
	"math"
	"strconv"
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"
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
	// Back leaves the page, nil where it is a tab's root.
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
	// The design draws the plus in the accent, level with the large title.
	add := ui.IconButton{TestID: "jobs-add", Icon: lucide.Plus, LabelFunc: addLabel, Size: 44, Tint: ui.ColorsOf(cx).Accent, OnPress: act.Add}
	symbols := func() []core.HeaderSymbol { return []core.HeaderSymbol{{Name: "plus", Label: addLabel()}} }
	page := base.NavPage{TestID: "jobs-page", Children: []core.Node{body}}
	if act.Back == nil {
		return look.TabRootTitleRow(cx, page, title, []core.Node{add}, symbols)
	}
	page.TitleFunc = title
	return look.NavPage(cx, page, ui.NavHeader{TitleFunc: title, OnBack: act.Back, Scroll: ref, Trailing: []core.Node{add}, Symbols: symbols})
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
			return empty(cx, desktop)
		}},
		core.Each(links, func(o session.Outgoing) string { return o.ID }, func(cx *core.Scope, o func() session.Outgoing, _ func() int) core.Node {
			return linkCard(cx, s, desktop, o)
		}),
		core.Each(func() []sync.Job { return queue().Jobs }, func(j sync.Job) string { return j.ID }, func(cx *core.Scope, j func() sync.Job, _ func() int) core.Node {
			return jobCard(cx, s, desktop, j, act)
		}),
	}}
}

// empty is what a computer with no jobs and no links waiting for it shows under its card: the design's tray, 64 points under the card, over a line saying how to add one.
func empty(cx *core.Scope, desktop string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 53, Right: 8, Left: 8}}, Children: []core.Node{
		ui.Icon(lucide.ArrowDownToLine, 44, colors.Faint),
		core.Text{TestID: "jobs-empty-" + desktop, ContentFunc: func() string { return locales.JobsEmpty(cx) }, Style: core.Style{FontSize: 14, LineHeight: 18, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// name is desktop's name, "" once it is unpaired.
func name(s *session.Session, desktop string) string {
	d, _ := s.Desktop(desktop)
	return d.Name
}

// computerCard is a computer's name, whether it answers and what its queue is doing.
func computerCard(cx *core.Scope, s *session.Session, desktop string) core.Node {
	colors, own := ui.ColorsOf(cx), theme.Of(cx)
	queue := func() session.Queue { return s.Queue(desktop) }
	online := func() bool { return reached(queue()) }
	return core.View{TestID: "jobs-computer-" + desktop, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 14)},
		StyleFunc: func() core.Style { return look.Card(cx) },
		Children: []core.Node{
			look.Laptop(44),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 1}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return name(s, desktop) }, Role: core.RoleHeader, MaxLines: 1, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Text{TestID: "jobs-summary-" + desktop, ContentFunc: func() string { return summary(cx, s, desktop) }, LiveRegion: core.LiveRegionPolite, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)},
			}},
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(7), Height: core.Pt(7), CornerRadius: 3.5},
					StyleFunc: func() core.Style {
						if online() {
							return core.Style{Background: theme.StatusOK}
						}
						return core.Style{Background: theme.Offline}
					}},
				core.Text{TestID: "jobs-reach-" + desktop, ContentFunc: func() string {
					switch {
					case online() && queue().Relayed:
						return locales.JobsViaStorage(cx)
					case online():
						return locales.JobsOnline(cx)
					}
					return locales.JobsOffline(cx)
				}, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, FontWeight: core.FontWeightBold}, StyleFunc: func() core.Style {
					if online() {
						return core.Style{Color: own.OkText()}
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

// summary is what a computer's queue is doing, as its card says it, and that it is reached through the user's storage when it is, or why that storage failed the phone; when it is offline, how long ago it last answered.
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
		if !q.Seen.IsZero() {
			return locales.JobsOfflineSeen(cx, ago(cx, s.Now().Sub(q.Seen)))
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
	// The computer does the work, so the phone need not stay awake while it does.
	if active, _ := counts(q); active > 0 {
		return locales.JobsLockPhone(cx, queueSummary(cx, q))
	}
	return queueSummary(cx, q)
}

// ago is how long ago d was, in whole minutes under an hour, hours under a day, else days.
func ago(cx *core.Scope, d time.Duration) string {
	switch {
	case d < time.Hour:
		return locales.JobsAgoMinutes(cx, max(1, int(d/time.Minute)))
	case d < 24*time.Hour:
		return locales.JobsAgoHours(cx, int(d/time.Hour))
	}
	return locales.JobsAgoDays(cx, int(d/(24*time.Hour)))
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

// counts is how many of q's jobs run, wait or are paused, and how many failed.
func counts(q session.Queue) (active, failed int) {
	for _, j := range q.Jobs {
		switch j.State {
		case sync.JobRunning, sync.JobWaiting, sync.JobPaused:
			active++
		case sync.JobFailed:
			failed++
		}
	}
	return active, failed
}

// queueSummary is what a computer's queue is doing: how many jobs run or wait, and how many failed.
func queueSummary(cx *core.Scope, q session.Queue) string {
	active, failed := counts(q)
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

// card is the look of a job's or a link's card, ringed in warning's color while warn reports true. The ring is a shadow outside the card, as the design's, so it does not move what the card holds.
func card(cx *core.Scope, testID string, warn func() bool, children ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: testID, Style: core.Style{Padding: core.All(14), Gap: 12},
		StyleFunc: func() core.Style {
			st := look.Card(cx)
			if warn() {
				st.Shadows = []core.BoxShadow{core.Ring(1.5, ui.Alpha(colors.Warn(), 0.6))}
			}
			return st
		}, Children: children}
}

// header is the top row of a job's or a link's card: its picture, its title over a line under it, and its buttons.
func header(cx *core.Scope, picture core.Node, title, meta core.Node, buttons ...core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
		picture,
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{title, meta}},
		// The buttons' targets reach past the card's padding, so the circles in them end at it.
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Margin: core.Edges{Right: -5}}, Children: buttons},
	}}
}

// titleText is a card's title, one bold line.
func titleText(cx *core.Scope, testID string, text func() string) core.Node {
	return core.Text{TestID: testID, ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Text)}
}

// linkCard is a link sent from this phone that its computer has not taken yet: waiting for it, or refused and why, with the way to send it again or drop it.
func linkCard(cx *core.Scope, s *session.Session, desktop string, o func() session.Outgoing) core.Node {
	colors := ui.ColorsOf(cx)
	id := o().ID
	refused := func() bool { return o().Refusal != nil }
	return card(cx, "jobs-link-"+id, refused,
		header(cx, thumb(cx, "", func() core.ImageSource { return core.ImageSource{} }, func() string { return "" }),
			titleText(cx, "", func() string { return linkName(cx, o().URL) }),
			core.Text{TestID: "jobs-link-status-" + id, ContentFunc: func() string {
				if r := o().Refusal; r != nil {
					return refusal(cx, *r, name(s, desktop))
				}
				if o().Relayed {
					return locales.JobsInStorage(cx, name(s, desktop))
				}
				return locales.JobsWaitingFor(cx, name(s, desktop))
			}, MaxLines: 3, Style: look.Footnote, StyleFunc: func() core.Style {
				if refused() {
					return core.Style{Color: theme.Of(cx).WarnText()}
				}
				return core.Style{Color: colors.Muted()}
			}},
			round(cx, "jobs-link-forget-"+id, closeGlyph, func() string { return locales.JobsForget(cx) }, nil, func() { s.Forget(id) }),
		),
		core.Show{When: refused, Then: func(cx *core.Scope) core.Node {
			return pills(pill(cx, "jobs-link-resend-"+id, true, func() string { return locales.CommonRetry(cx) }, func() { s.Resend(id) }))
		}},
	)
}

// linkName is what a link's card calls it before its computer took it: the site it is on, else the link itself.
func linkName(cx *core.Scope, link string) string {
	if src := source(cx, link); src != "" {
		return src
	}
	return link
}

// source is the site a job's link is on, as its card names it: YouTube and Bilibili by the names the Library gives them, another site by its domain; "" for a link Kando cannot read.
func source(cx *core.Scope, link string) string {
	l, ok := kcore.ParseLink(link)
	if !ok {
		return ""
	}
	switch l.Platform {
	case kcore.PlatformYouTube:
		return locales.FilmsSourceYoutube(cx)
	case kcore.PlatformBilibili:
		return locales.FilmsSourceBilibili(cx)
	case kcore.PlatformLocal:
		return locales.FilmsSourceLocal(cx)
	}
	if l.Site != "" {
		return l.Site
	}
	return l.Host
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

// unfinished reports whether a job in state still runs, waits or is paused.
func unfinished(state string) bool {
	return state == sync.JobRunning || state == sync.JobWaiting || state == sync.JobPaused
}

// jobCard is a job: its poster once it made a film, with how far it is over it, its title and where it is from, the buttons that pause, resume, cancel or open it, its steps while it runs or waits, and what can be done about it once it failed or was canceled.
func jobCard(cx *core.Scope, s *session.Session, desktop string, j func() sync.Job, act Actions) core.Node {
	colors := ui.ColorsOf(cx)
	id := j().ID
	q := func() session.Queue { return s.Queue(desktop) }
	reached := func() bool { return q().Reach == session.Online }
	state := func() string { return j().State }
	busy := func() bool { return s.Busy(desktop, id) || !reached() }
	stopped := func() bool { return state() == sync.JobFailed || state() == sync.JobCanceled }
	going := func() bool { return unfinished(state()) }
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
	picture := func() core.ImageSource {
		if data, ok := poster.Get(); ok && len(data) > 0 {
			return core.ImageSource{Data: data}
		}
		return core.ImageSource{}
	}
	// over is what the picture says over it: that the computer is out of reach, that the job is paused, or how far it has gone.
	over := func() string {
		switch {
		case !going():
			return ""
		case !reached():
			return locales.JobsThumbWaiting(cx)
		case state() == sync.JobPaused:
			return locales.JobsPaused(cx)
		case state() == sync.JobRunning:
			if p, ok := overall(j()); ok {
				return locales.JobsPercent(cx, int(math.Round(p*100)))
			}
			return locales.JobsRunning(cx)
		}
		return ""
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
	buttons := []core.Node{
		core.Show{When: func() bool { return going() && state() != sync.JobPaused && q().Can(sync.OpJobPause) }, Then: func(cx *core.Scope) core.Node {
			return round(cx, "jobs-pause-"+id, pauseGlyph, func() string { return locales.JobsPause(cx) }, busy, func() { s.Act(desktop, sync.OpJobPause, id) })
		}},
		core.Show{When: func() bool { return state() == sync.JobPaused && q().Can(sync.OpJobResume) }, Then: func(cx *core.Scope) core.Node {
			return round(cx, "jobs-resume-"+id, playGlyph, func() string { return locales.JobsResume(cx) }, busy, func() { s.Act(desktop, sync.OpJobResume, id) })
		}},
		core.Show{When: func() bool { return going() && q().Can(sync.OpJobCancel) }, Then: func(cx *core.Scope) core.Node {
			return round(cx, "jobs-cancel-"+id, closeGlyph, func() string { return locales.JobsCancel(cx) }, busy, confirmCancel)
		}},
		core.Show{When: func() bool { return state() == sync.JobDone && j().Film != "" }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Margin: core.Edges{Right: 5}}, Children: []core.Node{
				pillBusy(cx, "jobs-open-"+id, true, func() string { return locales.JobsOpen(cx) }, open, func() bool { return s.Busy(desktop, id) }),
			}}
		}},
	}
	return card(cx, "jobs-job-"+id, func() bool { return state() == sync.JobFailed },
		header(cx, thumb(cx, "jobs-thumb-"+id, picture, over),
			titleText(cx, "jobs-title-"+id, func() string { return title(j()) }),
			core.Text{TestID: "jobs-status-" + id, ContentFunc: func() string { return meta(cx, s, desktop, j()) }, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
			buttons...),
		core.Show{When: stopped, Then: func(cx *core.Scope) core.Node { return remedy(cx, s, desktop, j, act) }},
		core.Show{When: going, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				core.Each(func() []sync.Step { return j().Steps }, func(st sync.Step) string { return st.Name }, func(cx *core.Scope, st func() sync.Step, i func() int) core.Node {
					return stepRow(cx, id, st, i, func() bool { return state() == sync.JobRunning && reached() })
				}),
			}}
		}},
	)
}

// overall is how far a job has gone across its steps, from 0 to 1: a step done or skipped whole, the running one by its share; false while the running step's share is not known.
func overall(j sync.Job) (float64, bool) {
	if len(j.Steps) == 0 {
		return 0, false
	}
	var sum float64
	for _, st := range j.Steps {
		switch st.State {
		case sync.StepDone, sync.StepSkipped:
			sum++
		case sync.StepRunning:
			if st.Progress < 0 {
				return 0, false
			}
			sum += st.Progress
		}
	}
	return sum / float64(len(j.Steps)), true
}

// title is what a job is called: its title, else its link.
func title(j sync.Job) string {
	if j.Title != "" {
		return j.Title
	}
	return j.Link
}

// meta is the line under a job's title: the site it is from, then where it stands where its picture and steps do not say it, and the phone that sent it when that is another.
// The design names the uploader after the site, which the protocol does not carry (sync.Job).
func meta(cx *core.Scope, s *session.Session, desktop string, j sync.Job) string {
	var parts []string
	if src := source(cx, j.Link); src != "" {
		parts = append(parts, src)
	}
	q := s.Queue(desktop)
	switch {
	case q.Reach != session.Online && unfinished(j.State) && len(parts) == 0:
		// With no site to follow, the line starts with the sentence rather than its lowercase suffix.
		parts = append(parts, locales.JobsWaitingFor(cx, name(s, desktop)))
	case q.Reach != session.Online && unfinished(j.State):
		parts = append(parts, locales.JobsWaitingSuffix(cx, name(s, desktop)))
	case j.State == sync.JobWaiting:
		parts = append(parts, locales.JobsWaiting(cx))
	case j.State == sync.JobPaused:
		parts = append(parts, locales.JobsPaused(cx))
	case j.State == sync.JobDone:
		parts = append(parts, locales.JobsDone(cx))
	case j.State == sync.JobCanceled:
		parts = append(parts, locales.JobsCanceled(cx))
	case j.State == sync.JobFailed && len(parts) == 0:
		parts = append(parts, locales.JobsFailed(cx))
	case j.State == sync.JobRunning && len(parts) == 0:
		// With no site to name, the line says the job runs rather than start with the phone that sent it.
		parts = append(parts, locales.JobsRunning(cx))
	}
	text := ""
	for i, p := range parts {
		if i > 0 {
			text += " · "
		}
		text += p
	}
	if j.Device != "" && j.Device != s.Device() {
		return locales.JobsFrom(cx, text, j.Device)
	}
	return text
}

// StepText is a running step as a job's line says it: its name, with how far it has gone where that is known. The Library's jobs card says it so too.
func StepText(cx *core.Scope, st sync.Step) string {
	if st.Progress >= 0 {
		return locales.JobsStepPercent(cx, StepName(cx, st.Name), int(st.Progress*100))
	}
	return StepName(cx, st.Name)
}

// StepName is what a step is called.
func StepName(cx *core.Scope, name string) string {
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

// stepRow is a step of a job: its number, or a check once done, its name and where it stands, and its bar, green once done, pulsing while it runs without a known share and moving is true.
func stepRow(cx *core.Scope, job string, st func() sync.Step, i func() int, moving func() bool) core.Node {
	colors, own := ui.ColorsOf(cx), theme.Of(cx)
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
	pulse := func() bool { return running() && st().Progress < 0 && moving() }
	return core.View{TestID: "jobs-step-" + job + "-" + st().Name, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, MinHeight: core.Pt(28)}, Children: []core.Node{
		// The mark repeats the step's name and state, which the texts beside it say.
		core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Width: core.Pt(22), Height: core.Pt(22), CornerRadius: 11, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
			StyleFunc: func() core.Style {
				switch {
				case done():
					return core.Style{Background: theme.StatusOK, Color: colors.OnAccent()}
				case running():
					return core.Style{Background: colors.Accent(), Color: colors.OnAccent()}
				}
				return core.Style{Background: colors.Inset(), Color: colors.Muted()}
			},
			Children: []core.Node{core.Show{When: done,
				Then: func(*core.Scope) core.Node { return ui.Icon(lucide.Check, 13, ui.Inherited) },
				Else: func(cx *core.Scope) core.Node {
					return core.Text{ContentFunc: func() string { return strconv.Itoa(i() + 1) }, Style: core.Style{FontSize: 11, FontWeight: core.FontWeightBold}}
				}}}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 4}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween, Gap: 8}, Children: []core.Node{
				core.Text{ContentFunc: func() string {
					if running() {
						return locales.JobsStepRunning(cx, StepName(cx, st().Name))
					}
					return StepName(cx, st().Name)
				}, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: func() core.Style {
					switch {
					case running():
						return core.Style{Color: colors.Accent(), FontWeight: core.FontWeightBold}
					case done():
						return core.Style{Color: own.OkText(), FontWeight: core.FontWeightMedium}
					}
					return core.Style{Color: colors.Muted(), FontWeight: core.FontWeightMedium}
				}},
				core.Text{TestID: "jobs-step-detail-" + job + "-" + st().Name, ContentFunc: detail, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
			}},
			// The bar is drawn on base.Progress: ui.Progress's is 6 points and always the accent (ui/progress.go:24-27), the design's 4 and green once the step is done.
			base.Progress{LabelFunc: func() string { return StepName(cx, st().Name) },
				Value: func() float32 {
					switch st().State {
					case sync.StepDone:
						return 100
					case sync.StepRunning:
						return float32(max(st().Progress, 0) * 100)
					}
					return 0
				},
				Indeterminate:  pulse,
				TrackStyle:     core.Style{Height: core.Pt(4), CornerRadius: 2, Overflow: core.Hidden},
				TrackStyleFunc: ui.Filled(colors.Inset),
				IndicatorStyleFunc: func() core.Style {
					s := core.Style{Height: core.Pt(4), CornerRadius: 2, Background: colors.Accent()}
					if done() {
						s.Background = theme.StatusOK
					}
					if pulse() {
						s.Width, s.Animation = core.Pct(100), core.KeyframeAnimation{Keyframes: core.KeyframesPulse}
					} else {
						s.Transition = core.Transition{Duration: 200 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionLayout}
					}
					return s
				}},
		}},
	}}
}

// remedy is what a job that failed or was canceled says of it, and what can be done: its computer updating its downloader, trying again, the way to fix it on the computer first, or another link, and taking it off the computer's queue.
func remedy(cx *core.Scope, s *session.Session, desktop string, j func() sync.Job, act Actions) core.Node {
	colors, own := ui.ColorsOf(cx), theme.Of(cx)
	id := j().ID
	q := func() session.Queue { return s.Queue(desktop) }
	failure := func() sync.Failure {
		if f := j().Failure; f != nil {
			return *f
		}
		return sync.Failure{}
	}
	site := func() string { return source(cx, j().Link) }
	canceled := func() bool { return j().State == sync.JobCanceled }
	update := func() bool {
		return !canceled() && failure().Remedy == sync.RemedyUpdateYTDLP && q().Can(sync.OpUpdateYTDLP)
	}
	// another is a failure trying again cannot fix, so the box offers another link in its place.
	another := func() bool {
		return !canceled() && act.Add != nil && hopeless(failure().Code) && q().Can(sync.OpJobCreate)
	}
	retry := func() bool { return q().Can(sync.OpJobRetry) && !another() }
	// remove takes the job off the computer's queue, as the × of its card there does, so one that will not be tried again does not stay on the phone; a computer from before sync.OpJobRemove does not offer it.
	remove := func() bool { return q().Can(sync.OpJobRemove) }
	busy := func() bool { return s.Busy(desktop, id) || s.Updating(desktop) || q().Reach != session.Online }
	body := func() string { return failureBody(cx, failure(), name(s, desktop), site()) }
	return core.View{TestID: "jobs-remedy-" + id, Style: core.Style{Gap: 4, Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style {
			if canceled() {
				return core.Style{Background: colors.Inset()}
			}
			return core.Style{Background: own.WarnBox()}
		},
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignStart}, Children: []core.Node{
				core.Show{When: func() bool { return !canceled() }, Then: func(*core.Scope) core.Node {
					return core.Icon{SVG: warnIcon(), Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0, Margin: core.Edges{Top: 1}}}
				}},
				core.Text{TestID: "jobs-failure-" + id, ContentFunc: func() string {
					if canceled() {
						return locales.JobsCanceledTitle(cx, name(s, desktop))
					}
					return failureTitle(cx, failure(), site())
				}, Style: core.Style{FontSize: 14, LineHeight: 19, FontWeight: core.FontWeightBold, FlexShrink: 1}, StyleFunc: func() core.Style {
					if canceled() {
						return core.Style{Color: colors.Text()}
					}
					return core.Style{Color: own.WarnText()}
				}},
			}},
			core.Show{When: func() bool { return !canceled() && body() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "jobs-failure-body-" + id, ContentFunc: body, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Left: 24}}), StyleFunc: ui.Colored(own.WarnBody)}
			}},
			// The buttons line up with the text under the triangle, or with the title where there is none.
			core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8}, StyleFunc: func() core.Style {
				if canceled() {
					return core.Style{Padding: core.Edges{Top: 6}}
				}
				return core.Style{Padding: core.Edges{Top: 6, Left: 24}}
			}, Children: []core.Node{
				core.Show{When: update, Then: func(cx *core.Scope) core.Node {
					return warnPill(cx, "jobs-update-"+id, func() string { return locales.JobsUpdateRetry(cx) }, func() { s.UpdateYTDLP(desktop) }, busy)
				}},
				core.Show{When: another, Then: func(cx *core.Scope) core.Node {
					return warnPill(cx, "jobs-another-"+id, func() string { return locales.JobsOtherLink(cx) }, act.Add, func() bool { return false })
				}},
				core.Show{When: func() bool { return retry() && !update() }, Then: func(cx *core.Scope) core.Node {
					text := func() string {
						f := failure()
						switch {
						case canceled() || f.Remedy != sync.RemedyDesktop:
							return locales.CommonRetry(cx)
						case f.Code == sync.FailLogin || f.Code == sync.FailLoginExpired:
							return locales.JobsSignedInRetry(cx)
						}
						return locales.JobsFixedRetry(cx)
					}
					return core.Show{When: canceled,
						Then: func(cx *core.Scope) core.Node {
							return pillBusy(cx, "jobs-retry-"+id, true, text, func() { s.Act(desktop, sync.OpJobRetry, id) }, busy)
						},
						Else: func(cx *core.Scope) core.Node {
							return warnPill(cx, "jobs-retry-"+id, text, func() { s.Act(desktop, sync.OpJobRetry, id) }, busy)
						}}
				}},
				core.Show{When: remove, Then: func(cx *core.Scope) core.Node {
					return pillBusy(cx, "jobs-remove-"+id, false, func() string { return locales.JobsRemove(cx) }, func() { s.Act(desktop, sync.OpJobRemove, id) }, busy)
				}},
			}},
		}}
}

// hopeless reports whether a failure of code comes back however often the job is tried again: the video is not there for the computer, so only another link helps.
func hopeless(code string) bool {
	switch code {
	case sync.FailRegion, sync.FailPrivate, sync.FailBadLink, sync.FailDRM, sync.FailUnsupported:
		return true
	}
	return false
}

// failureTitle is what went wrong, by the failure's code, naming site, the site of the job's link, where the design does; the computer's own words for a code the phone does not know.
func failureTitle(cx *core.Scope, f sync.Failure, site string) string {
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
		if site != "" {
			return locales.JobsFailOutdatedSite(cx, site)
		}
		return locales.JobsFailOutdated(cx)
	case sync.FailLogin:
		return locales.JobsFailLogin(cx)
	case sync.FailLoginExpired:
		if site != "" {
			return locales.JobsFailLoginExpiredSite(cx, site)
		}
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
	case sync.FailAIUnavailable:
		return locales.JobsFailAIUnavailable(cx)
	case sync.FailOther, "":
		return locales.JobsFailOther(cx)
	}
	if f.Message != "" {
		return f.Message
	}
	return locales.JobsFailOther(cx)
}

// failureBody is what can be done about a failure, by its remedy, with the computer's own words where they say what to set up, and the site to sign in to again where a sign-in on the computer expired.
func failureBody(cx *core.Scope, f sync.Failure, desktop, site string) string {
	if f.Code == sync.FailRegion {
		return locales.JobsRemedyRegion(cx, desktop)
	}
	switch f.Remedy {
	case sync.RemedyUpdateYTDLP:
		return locales.JobsRemedyUpdate(cx, desktop)
	case sync.RemedyRetry:
		return locales.JobsRemedyRetry(cx)
	case sync.RemedyDesktop:
		switch {
		case f.Code == sync.FailDesktopSetup && f.Message != "":
			return locales.JobsRemedySetup(cx, desktop, f.Message)
		case (f.Code == sync.FailLogin || f.Code == sync.FailLoginExpired) && site != "":
			return locales.JobsRemedySignIn(cx, site, desktop)
		}
		return locales.JobsRemedyDesktop(cx, desktop)
	}
	return ""
}

// thumb is a job's poster, in the 16:9 of a video, a film's mark until it has one, with over, when it says something, in bold white on a dark veil across it; testID names the text over it.
func thumb(cx *core.Scope, testID string, source func() core.ImageSource, over func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Width: core.Pt(80), AspectRatio: 16.0 / 9, CornerRadius: 8, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: ui.Filled(colors.Inset),
		Children: []core.Node{
			// The film's mark gives way to the words over the picture, which it would show through.
			core.Show{When: func() bool { return over() == "" }, Then: func(*core.Scope) core.Node { return ui.Icon(lucide.Film, 20, colors.Faint) }},
			core.Image{SourceFunc: source, ResizeMode: core.Cover, FadeDuration: 150 * time.Millisecond, ElementsHidden: true,
				Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}},
			core.Show{When: func() bool { return over() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Padding: core.Symmetric(0, 4), Background: ui.Black(0.35)}, Children: []core.Node{
					// Bold, not the design's 800: at 800 "Waiting for" is wider than the 72pt the thumbnail leaves inside its padding, so the words would take three lines and the third be cut.
					core.Text{TestID: testID, ContentFunc: over, MaxLines: 2, Style: core.Style{Width: core.Pct(100), FontSize: 13, LineHeight: 16, FontWeight: core.FontWeightBold, TextAlign: core.TextAlignCenter, Color: theme.OnMark}},
				}}
			}},
		}}
}

// round is a card's round button: a glyph in the design's softer ink on the inset fill, 34 points in a 44-point target.
func round(cx *core.Scope, testID string, g glyph, label func() string, disabled func() bool, press func()) core.Node {
	own := theme.Of(cx)
	return core.View{Style: core.Style{Width: core.Pt(44), Height: core.Pt(44), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		Children: []core.Node{ui.IconButton{TestID: testID, Variant: ui.IconButtonFilled, Size: 34, IconSize: max(g.w, g.h), Icon: func() *core.SVG { return g.svg }, Tint: own.Glyph,
			LabelFunc: label, Disabled: disabled, OnPress: press}}}
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
		Style: core.Style{MinHeight: core.Pt(32), Padding: core.Symmetric(0, 14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Active: &core.Style{Opacity: core.Alpha(0.7)}, Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
		StyleFunc: func() core.Style {
			if main {
				return core.Style{Background: colors.Accent(), Color: colors.OnAccent()}
			}
			return core.Style{Background: tints.Accent(), Color: colors.Accent()}
		},
		Children: []core.Node{core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightBold}}}}
}

// warnPill is the failure box's main button: white on the warning's orange, 32 points tall, its text on up to two lines.
func warnPill(cx *core.Scope, testID string, text func() string, press func(), busy func() bool) core.Node {
	return base.Button{TestID: testID, LabelFunc: text, OnPress: func(core.PressEvent) { press() }, Disabled: busy,
		Style: core.Style{MinHeight: core.Pt(32), Padding: core.Symmetric(4, 14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 1,
			Background: theme.BadgeOrange, Color: theme.OnMark, Active: &core.Style{Opacity: core.Alpha(0.7)}, Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
		Children: []core.Node{core.Text{ContentFunc: text, MaxLines: 2, Style: core.Style{FontSize: 13, LineHeight: 16, FontWeight: core.FontWeightBold}}}}
}
