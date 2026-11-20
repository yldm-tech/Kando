package films

import (
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// jobsShown reports whether the card of the jobs under way shows: a computer is paired and the Jobs page holds something not done.
func (v *view) jobsShown() bool {
	if len(v.s.Desktops()) == 0 {
		return false
	}
	active, failed := v.s.Unfinished()
	return active+failed > 0
}

// work is what the computers are doing, as the jobs card says it: the computer whose jobs it names, "" where more than one has jobs; the jobs running or waiting and those that failed; the job running now, if any; and whether that computer is out of reach.
type work struct {
	computer       string
	active, failed int
	current        sync.Job
	running, away  bool
	progress       float32
	progressedJobs int
}

// work reads what the computers are doing from their queues.
func (v *view) work() work {
	var w work
	computers := 0
	for _, d := range v.s.Desktops() {
		q := v.s.Queue(d.ID)
		mine := false
		for _, j := range q.Jobs {
			switch j.State {
			case sync.JobRunning, sync.JobWaiting, sync.JobPaused:
				w.active++
				mine = true
			case sync.JobFailed:
				w.failed++
				mine = true
			}
			if j.State == sync.JobRunning {
				p := jobProgress(j)
				w.progress += p
				w.progressedJobs++
				if !w.running {
					w.current, w.running = j, true
				}
			}
		}
		if mine {
			computers++
			if w.computer == "" {
				w.computer = d.Name
				w.away = q.Reach != session.Online
			}
		}
	}
	if computers > 1 {
		w.computer = ""
	}
	// The links waiting on the phone are the Jobs page's too.
	active, failed := v.s.Unfinished()
	w.active, w.failed = active, failed
	return w
}

// jobProgress is how far a running job has gone, from 0 to 1, its steps counted alike.
func jobProgress(j sync.Job) float32 {
	if len(j.Steps) == 0 {
		return 0
	}
	var done float64
	for _, st := range j.Steps {
		switch st.State {
		case sync.StepDone, sync.StepSkipped:
			done++
		case sync.StepRunning:
			if st.Progress > 0 {
				done += st.Progress
			}
		}
	}
	return float32(done / float64(len(j.Steps)))
}

// jobsTitle is the jobs card's title: the computer processing how many videos, or that it is out of reach with how many waiting, or how many need the user once none is under way.
func jobsTitle(cx *core.Scope, w work) string {
	switch {
	case w.active == 0:
		return locales.FilmsJobsCardFailed(cx, w.failed)
	case w.computer != "" && w.away:
		return locales.FilmsJobsCardOffline(cx, w.computer, w.active)
	case w.computer != "":
		return locales.FilmsJobsCardTitle(cx, w.computer, w.active)
	}
	return locales.FilmsJobsCardTitleAll(cx, w.active)
}

// jobsLine is the line under the jobs card's title: the job running now and its step, how far it has gone.
func jobsLine(cx *core.Scope, w work) string {
	if !w.running {
		return ""
	}
	name := w.current.Title
	if name == "" {
		name = w.current.Link
	}
	parts := []string{name}
	for _, st := range w.current.Steps {
		if st.State != sync.StepRunning {
			continue
		}
		step := stepName(cx, st.Name)
		if st.Progress >= 0 {
			step = locales.JobsStepPercent(cx, step, int(st.Progress*100))
		}
		parts = append(parts, step)
	}
	return strings.Join(parts, " · ")
}

// stepName is what a job's step is called, as the Jobs page names it.
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

// jobsRing is the ring of how far the running jobs have gone, around a disc with the jobs' arrow.
func jobsRing(cx *core.Scope, w func() work, size core.Points) core.Node {
	colors := ui.ColorsOf(cx)
	disc := size - 8
	return core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
		Children: []core.Node{
			// TODO(nsgo): ui.ProgressRing's line is a tenth of its size, at most 3pt (ui/progressring.go:59), so the ring is thinner than the design's 4pt one around the disc.
			ui.ProgressRing{Size: size, Tint: colors.Accent, Value: func() float32 {
				got := w()
				if got.progressedJobs == 0 {
					return 0
				}
				return got.progress / float32(got.progressedJobs) * 100
			}, Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0)}},
			core.View{Style: core.Style{Width: core.Pt(disc), Height: core.Pt(disc), CornerRadius: disc / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				StyleFunc: ui.Filled(colors.Surface),
				Children:  []core.Node{ui.Icon(lucide.ArrowDown, disc*10/22, colors.Accent)}},
		}}
}

// jobsCard is the card under the search while the computers have jobs under way: the ring of how far they have gone, which computer processes how many videos, the job running now and its step, and how many need the user. Pressed, it opens Jobs.
func (v *view) jobsCard(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	w := core.NewMemoFunc(cx, v.work, nil)
	title := func() string { return jobsTitle(cx, w.Get()) }
	line := func() string { return jobsLine(cx, w.Get()) }
	needs := func() bool { return w.Get().failed > 0 && w.Get().active > 0 }
	return core.View{TestID: "films-jobs-card", Role: core.RoleButton,
		LabelFunc: func() string {
			parts := []string{title()}
			if l := line(); l != "" {
				parts = append(parts, l)
			}
			if needs() {
				parts = append(parts, locales.FilmsJobsCardNeedsYou(cx, w.Get().failed))
			}
			return strings.Join(parts, ", ")
		},
		OnPress: func(core.PressEvent) {
			if v.act.OpenJobs != nil {
				v.act.OpenJobs()
			}
		},
		Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(10, 12), MinHeight: core.Pt(60), Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return look.Card(cx) },
		Children: []core.Node{
			jobsRing(cx, w.Get, 30),
			core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{TestID: "films-jobs-card-title", ContentFunc: title, MaxLines: 2, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: func() bool { return line() != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "films-jobs-card-line", ContentFunc: line, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)}
				}},
			}},
			core.Show{When: needs, Then: func(cx *core.Scope) core.Node {
				return core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Height: core.Pt(20), Padding: core.Symmetric(0, 7), CornerRadius: 10, JustifyContent: core.JustifyCenter, FlexShrink: 0,
					Background: theme.BadgeOrange.Faded(0.16)},
					Children: []core.Node{core.Text{TestID: "films-jobs-card-needs", ContentFunc: func() string { return locales.FilmsJobsCardNeedsYou(cx, w.Get().failed) }, MaxLines: 1,
						Style: core.Style{FontSize: 11, LineHeight: 20, FontWeight: core.FontWeightBold, Color: theme.BadgeWarnText}}}}
			}},
			core.Icon{SVG: look.Chevron(), Style: core.Style{Width: core.Pt(8), Height: core.Pt(13), FlexShrink: 0, Color: theme.Chevron}},
		}}
}

// jobsTile is a tablet's fourth stat tile, the jobs card's place there: the ring, how many are processing with how many need the user, and on which computer.
func (v *view) jobsTile(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	w := core.NewMemoFunc(cx, v.work, nil)
	title := func() string {
		got := w.Get()
		if got.failed > 0 && got.active > 0 {
			return locales.FilmsJobsTileNeeds(cx, got.active, got.failed)
		}
		if got.active == 0 {
			return locales.FilmsJobsCardFailed(cx, got.failed)
		}
		return locales.FilmsJobsTile(cx, got.active)
	}
	where := func() string {
		if c := w.Get().computer; c != "" {
			return locales.FilmsJobsTileOn(cx, c)
		}
		return ""
	}
	return core.View{TestID: "films-jobs-card", Role: core.RoleButton, LabelFunc: func() string { return title() + " " + where() },
		OnPress: func(core.PressEvent) {
			if v.act.OpenJobs != nil {
				v.act.OpenJobs()
			}
		},
		Style:     core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(10, 14), Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return look.Card(cx) },
		Children: []core.Node{
			jobsRing(cx, w.Get, 34),
			core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{ContentFunc: title, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: where, MaxLines: 2, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
			}},
		}}
}
