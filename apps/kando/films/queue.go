package films

import (
	"fmt"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// queueSection is 处理中: a card for every job that is waiting, running, failed or was cancelled, in QueueOrder, those from source. Past QueueShown cards it shows the first ones and a button for the rest.
type queueSection struct {
	a      App
	source func() string
	queue  func() []Film
}

// Render implements core.Node.
func (q queueSection) Render(cx *ns.Scope) ns.Node {
	a := q.a
	expanded := live.Memo(cx, a, func() bool { return *a.QueueExpanded() })
	count := ns.NewMemo(cx, func() int { return len(q.queue()) }).Get
	shown := func() []Film {
		items := q.queue()
		if !expanded() && len(items) > QueueShown {
			return items[:QueueShown]
		}
		return items
	}
	cards := ns.View{TestID: "films-queue",
		Style: ns.Style{Display: ns.DisplayGrid, GridTemplateColumns: gridTracks(QueueMinWidth), Gap: QueueGap, Padding: ns.Edges{Top: 2, Right: 2, Bottom: 8, Left: 2}, AlignItems: ns.AlignStart},
		Children: []ns.Node{ns.Each(shown, func(item Film) string { return item.Key }, func(cx *ns.Scope, item func() Film, _ func() int) ns.Node {
			return queueCard{a: a, item: item}
		})}}
	more := ns.Show{When: func() bool { return count() > QueueShown }, Then: func(*ns.Scope) ns.Node {
		// The button says 显示全部 with the count, or 收起, so it is built again when either changes, and not when a job's progress does.
		label := func(cx *ns.Scope) string {
			if expanded() {
				return locales.FilmsQueueCollapse(cx)
			}
			return locales.FilmsQueueShowAll(cx, count())
		}
		return ns.View{Style: ns.Style{AlignSelf: ns.AlignStart}, Children: []ns.Node{
			textButton(label, ui.Button{Variant: ui.ButtonSecondary, TestID: "queue-more", OnPress: live.Do(a, func() { *a.QueueExpanded() = !*a.QueueExpanded() })}),
		}}
	}}
	summary := func() string { return QueueSummary(locales.For(cx), q.queue()) }
	// Past one failure, the heading offers to retry or remove them all at once, as after the cause of a batch of failures was fixed.
	failures := live.Memo(cx, a, func() [2]int {
		retryable, failed := a.Failures(q.source())
		return [2]int{retryable, failed}
	})
	bulk := ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, FlexShrink: 0}, Children: []ns.Node{
		ns.Show{When: func() bool { return failures()[1] > 1 }, Then: func(*ns.Scope) ns.Node {
			return textButton(locales.FilmsRemoveFailed, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "queue-remove-failed", OnPress: live.Do(a, func() { a.RemoveFailed(q.source()) })})
		}},
		ns.Show{When: func() bool { return failures()[0] > 1 }, Then: func(*ns.Scope) ns.Node {
			return textButton(locales.FilmsRetryAll, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "queue-retry-all", OnPress: live.Do(a, func() { a.RetryAll(q.source()) })})
		}},
	}}
	head := ns.View{Style: ns.Style{Direction: ns.Row, Gap: 12, AlignItems: ns.AlignCenter}, Children: []ns.Node{
		ns.View{Style: ns.Style{FlexGrow: 1, MinWidth: ns.Pt(0)}, Children: []ns.Node{sectionTitle(cx, func() string { return locales.FilmsQueueTitle(cx) }, summary)}},
		bulk,
	}}
	return ns.View{Style: ns.Style{Margin: ns.Edges{Top: 26}, Gap: 12}, Children: []ns.Node{
		head,
		cards,
		more,
	}}
}

// A processing card's face is the theme's panel, a step off the page, its edge the shade, and a card whose translation waits out a rate limit says so in the theme's retry orange (theme.Of). A thumbnail's face before its picture is there is the theme's thumb, and placeholderInk the glyph on it, which reads on it in both appearances.
var placeholderInk = ns.RGB(0x8E8E95)

// queueCard is one job in 处理中: its thumbnail, its title, what it is doing or why it failed, the stages behind it, and what can be done about it.
type queueCard struct {
	a    App
	item func() Film
}

// Render implements core.Node.
func (q queueCard) Render(cx *ns.Scope) ns.Node {
	a, item := q.a, q.item
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	failed := func() bool { return item().Stage == core.StageFailed }
	library := ns.Show{When: func() bool { return item().Library != "" }, Then: func(*ns.Scope) ns.Node {
		return ns.Text{ContentFunc: func() string { return locales.FilmsQueueSavingTo(cx, item().Library) }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 11}, StyleFunc: ui.Colored(colors.Muted)}
	}}
	// A job a paired phone sent says which phone (core.Job.Device).
	from := ns.Show{When: func() bool { return item().Device != "" }, Then: func(*ns.Scope) ns.Node {
		return ns.Text{TestID: "queue-from", ContentFunc: func() string { return locales.FilmsQueueFrom(cx, item().Device) }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 11}, StyleFunc: ui.Colored(colors.Muted)}
	}}
	working := func(*ns.Scope) ns.Node {
		meta := ns.Text{ContentFunc: func() string { return QueueMeta(locales.For(cx), item()) }, Style: ns.Style{FontSize: 11},
			StyleFunc: func() ns.Style {
				if RetryWaiting(item()) {
					return ns.Style{Color: k.RetryInk()}
				}
				return ns.Style{Color: colors.Muted()}
			}}
		// A job a phone paused waits until it is let go on, here or on the phone (core.Job.PausedBy).
		here := ns.Show{When: func() bool { return item().PausedBy != "" && item().Stage == core.StageQueued }, Then: func(*ns.Scope) ns.Node {
			return ns.View{Style: ns.Style{Direction: ns.Row, Margin: ns.Edges{Top: 2}}, Children: []ns.Node{
				textButton(locales.FilmsQueueContinueHere, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "queue-continue-here",
					OnPress: live.Do(a, func() { a.ContinueHere(item().JobID) })}),
			}}
		}}
		return ns.Group{meta, stepsBar{item: item}, here}
	}
	// A job waiting or running opens from its thumbnail (Openable): the film plays its video or its sound once it has one (Watchable), with its lines once there are some.
	thumb := ns.Show{When: func() bool { return Openable(item()) }, Then: func(cx *ns.Scope) ns.Node {
		return base.Button{TestID: "queue-watch", LabelFunc: func() string { return locales.FilmsQueueWatch(cx, filmName(item())) },
			OnPress:  live.On(a, func(ns.PressEvent) { a.Open(item()) }),
			Style:    ns.Style{FlexShrink: 0, CornerRadius: 9, Cursor: ns.CursorDefault, FocusVisible: ui.FocusRing(cx, 2)},
			Children: []ns.Node{queueThumb{a: a, item: item, watch: true}}}
	}, Else: func(*ns.Scope) ns.Node { return queueThumb{a: a, item: item} }}
	// The end keeps clear of the cancel button over the card's corner.
	// A waiting or running card says beside its title the language it translates into, as one translated again into another language does.
	into := ns.Show{When: func() bool { return !failed() && item().Target != "" }, Then: func(cx *ns.Scope) ns.Node {
		return ns.Text{TestID: "queue-into", ContentFunc: func() string { return locales.FilmsQueueInto(cx, subtitle.TargetLanguageName(cx, item().Target)) }, MaxLines: 1,
			Style: ns.Style{FontSize: 11, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Muted)}
	}}
	// Only the title's row keeps clear of the × at the card's top right; the rows under it, a failed card's actions among them, take the card's whole width.
	text := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 4}, Children: []ns.Node{
		ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignBaseline, MinWidth: ns.Pt(0), Padding: ns.Edges{Right: 18}}, Children: []ns.Node{
			ns.Text{ContentFunc: func() string { return filmName(item()) }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail, Style: ns.Style{FontSize: 13, FontWeight: ui.Semibold, FlexShrink: 1, MinWidth: ns.Pt(0)},
				StyleFunc: ui.Colored(colors.Text)},
			into,
		}},
		library,
		from,
		ns.Show{When: failed, Then: func(*ns.Scope) ns.Node { return failedDetail{a: a, item: item} }, Else: working},
	}}
	// The × at the card's top right: a waiting or running card's withdraws it, a failed or cancelled card's removes it (移除), which leaves the row under its reason to what fixes it, so the row fits the card on one line.
	closeButton := func(cx *ns.Scope, id string, label func(*ns.Scope) string, press func(string)) ns.Node {
		return base.Button{TestID: id, LabelFunc: func() string { return label(cx) },
			OnPress: live.On(a, func(ns.PressEvent) { press(item().JobID) }),
			Style: ns.Style{Position: ns.Absolute, Top: ns.Pt(8), Right: ns.Pt(8), Width: ns.Pt(22), Height: ns.Pt(22), CornerRadius: 11, AlignItems: ns.AlignCenter,
				JustifyContent: ns.JustifyCenter, Cursor: ns.CursorDefault, FocusVisible: ui.FocusRing(cx, -2)},
			StyleFunc: func() ns.Style { return ns.Style{Hover: &ns.Style{Background: k.Shade(0.06)}} },
			Children:  []ns.Node{ui.Icon(svg(closeIcon), 10, colors.Muted)}}
	}
	cancel := ns.Show{When: func() bool { return !failed() }, Then: func(cx *ns.Scope) ns.Node {
		return closeButton(cx, "queue-cancel", locales.FilmsQueueCancel, a.Withdraw)
	}, Else: func(cx *ns.Scope) ns.Node {
		return closeButton(cx, "queue-remove", locales.FilmsRemove, a.Remove)
	}}
	return ns.View{
		Role:      ns.RoleGroup,
		LabelFunc: func() string { return filmName(item()) },
		TestID:    "queue-" + item().Key,
		Style:     ns.Style{Direction: ns.Row, Width: ns.Pct(100), MinWidth: ns.Pt(0), Gap: 12, Padding: ns.All(10), CornerRadius: 16, BorderWidth: 0.5},
		StyleFunc: func() ns.Style {
			s := ns.Style{Background: k.Panel(), BorderColor: k.Shade(0.06), AlignItems: ns.AlignCenter}
			if failed() {
				s.AlignItems = ns.AlignStart
			}
			return s
		},
		Children: []ns.Node{thumb, text, cancel},
	}
}

// failedVeil is the dark veil over a failed job's thumbnail, under its red mark.
var failedVeil = ns.RGBA(0x000000, 0.45)

// queueThumb is a processing card's 16:9 thumbnail: the poster once there is one, the picture the video's site gives it before that, a download glyph until either is there, and a red mark over a failed job's. watch puts a play glyph over it, for a film its card opens.
type queueThumb struct {
	a     App
	item  func() Film
	watch bool
}

// Render implements core.Node.
func (q queueThumb) Render(cx *ns.Scope) ns.Node {
	a, item := q.a, q.item
	picture := live.Func(a, func() ns.ImageSource {
		if img := a.Poster(item().VideoPath); hasImage(img) {
			return img
		}
		return a.Thumbnail(item())
	})
	return ns.View{
		Style:     ns.Style{Width: ns.Pt(112), FlexShrink: 0, AspectRatio: 16.0 / 9, CornerRadius: 9, Overflow: ns.Hidden, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter},
		StyleFunc: ui.Filled(theme.Of(cx).Thumb),
		Children: []ns.Node{
			ns.Show{When: func() bool { return hasImage(picture()) }, Then: func(*ns.Scope) ns.Node { return cover(picture) },
				Else: func(*ns.Scope) ns.Node { return ui.Icon(svg(downloadIcon), 22, ui.Fixed(placeholderInk)) }},
			ns.Show{When: func() bool { return item().Stage == core.StageFailed && !Canceled(item()) }, Then: func(*ns.Scope) ns.Node {
				return ns.View{Style: overlay.Merge(ns.Style{Background: failedVeil, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter}), Children: []ns.Node{
					ns.SVGView{SVG: failedMark, Style: ns.Style{Width: ns.Pt(22), Height: ns.Pt(22)}},
				}}
			}},
			ns.Show{When: func() bool { return q.watch }, Then: func(*ns.Scope) ns.Node {
				return ns.View{PointerEvents: ns.PointerEventsNone, Style: overlay.Merge(ns.Style{AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter}), Children: []ns.Node{
					ns.View{Style: ns.Style{Width: ns.Pt(26), Height: ns.Pt(26), CornerRadius: 13, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter, Padding: ns.Edges{Left: 2}}.Merge(frosted(8, chipShade)),
						Children: []ns.Node{ns.Icon{SVG: playIcon, Style: ns.Style{Width: ns.Pt(10), Height: ns.Pt(11), Color: onPicture}}}},
				}}
			}},
		},
	}
}

// Watchable reports whether a film still in 处理中 can be watched already: its job is waiting or running and has its video, downloaded or found on disk, or a link's sound, which plays until its video comes (PlayPath). Until its first lines are made it plays without subtitles.
func Watchable(item Film) bool {
	return item.JobID != "" && PlayPath(item) != "" && !item.Stage.Done()
}

// Openable reports whether a film still in 处理中 opens from its card: any whose job waits or runs, as opening it is what puts it first in the queue (queuePreferOpen), and a link with nothing downloaded yet plays once its sound is in.
func Openable(item Film) bool { return item.JobID != "" && !item.Stage.Done() }

// PlayPath is the path the player opens item by and keeps its place by: its video, or while a link's sound plays until its video comes (SoundPath) the path the video will have but for its extension (media.SoundVideo); "" for a film with neither.
func PlayPath(item Film) string {
	if item.VideoPath != "" || item.SoundPath == "" {
		return item.VideoPath
	}
	return media.SoundVideo(item.SoundPath)
}

// QueueMeta is the line under a waiting or running job's title, in the language of m: the stage and how far it has gone, as its tool reports it, and about how long it has left once that can be told (Remaining), or how many jobs run before a waiting one, or that it waits for the paused queue.
func QueueMeta(m locales.Messages, item Film) string { return queueMetaAt(m, item, time.Now()) }

// queueMetaAt is QueueMeta at the time now.
func queueMetaAt(m locales.Messages, item Film, now time.Time) string {
	p := item.Progress
	left := func(line string) string {
		d, ok := Remaining(item, now)
		if !ok {
			return line
		}
		if d < time.Minute {
			return line + " · " + m.FilmsQueueUnderAMinute()
		}
		return line + " · " + m.FilmsQueueAboutMinutes(int((d+time.Minute-1)/time.Minute))
	}
	switch item.Stage {
	case core.StageQueued:
		if item.PausedBy != "" {
			return m.FilmsQueuePausedOn(item.PausedBy)
		}
		if item.Paused {
			return m.FilmsQueuePaused()
		}
		if item.Ahead > 0 {
			return m.FilmsQueueWaitingAhead(item.Ahead)
		}
		return m.FilmsQueueWaiting()
	case core.StageDownloading:
		if Local(item) {
			return m.FilmsQueueReadingLocal()
		}
		if !p.Known {
			return m.FilmsQueueDownloadingFrom(SourceLabel(m, item))
		}
		line := m.FilmsStageDownloading() + " · " + Percent(p.Fraction)
		// yt-dlp's own estimate is the download's; without it, the download is timed as the other stages are.
		if p.ETA > 0 {
			return line + " · " + m.FilmsQueueRemaining(Clock(p.ETA))
		}
		return left(line)
	case core.StageTranscribing:
		if p.Known {
			return left(m.FilmsQueueTranscribingPercent(Percent(p.Fraction)))
		}
		return m.FilmsQueueTranscribing()
	case core.StageTranslating:
		if RetryWaiting(item) {
			seconds := int(p.RetryIn.Round(time.Second) / time.Second)
			if p.Unanswered {
				return m.FilmsQueueNoAnswer(seconds, p.Attempt, p.Attempts)
			}
			return m.FilmsQueueRateLimited(seconds, p.Attempt, p.Attempts)
		}
		if p.Of > 0 && p.Reviewing {
			return left(m.FilmsQueueReviewingLines(p.Lines, p.Of))
		}
		if p.Of > 0 && p.Proofreading {
			return left(m.FilmsQueueProofreadingLines(p.Lines, p.Of))
		}
		if p.Of > 0 {
			return left(m.FilmsQueueTranslatingLines(p.Lines, p.Of))
		}
		return m.FilmsQueueTranslating()
	case core.StageExporting:
		return m.FilmsQueueExporting()
	}
	return StageLabel(m, item.Stage)
}

// Remaining is about how long the stage a running job is in still needs at the time now, told from how fast it has gone since it first said how far it had gone (core.Progress.Started, From). It is false until that is worth telling: before the stage has gone 5% further or run 10 seconds since, and once it is done.
func Remaining(item Film, now time.Time) (time.Duration, bool) {
	p := item.Progress
	gone := p.Fraction - p.From
	elapsed := now.Sub(p.Started)
	if !Running(item) || !p.Known || p.Started.IsZero() || p.Fraction >= 1 || gone < 0.05 || elapsed < 10*time.Second {
		return 0, false
	}
	return time.Duration(float64(elapsed) * (1 - p.Fraction) / gone), true
}

// RetryWaiting reports whether the film's translation is waiting to try a request again: one rate-limited, or one that got no usable answer.
func RetryWaiting(item Film) bool {
	return item.Stage == core.StageTranslating && item.Progress.RetryIn > 0
}

// Percent writes a fraction as a whole percentage, such as 42%.
func Percent(f float64) string {
	return fmt.Sprintf("%d%%", int(f*100))
}

// Rate writes a download speed in bytes a second as the Finder writes sizes, such as 3.2 MB/s.
func Rate(bytesPerSecond float64) string {
	const k = 1024.0
	switch {
	case bytesPerSecond >= k*k:
		return fmt.Sprintf("%.1f MB/s", bytesPerSecond/(k*k))
	case bytesPerSecond >= k:
		return fmt.Sprintf("%.0f KB/s", bytesPerSecond/k)
	}
	return fmt.Sprintf("%.0f B/s", bytesPerSecond)
}

// stepsGap is the room between the segments of a card's progress bar.
const stepsGap ns.Points = 3

// stepsBar shows the stages of a job as segments, as the prototype draws them: those behind it filled, the one it is in filled as far as its tool has said, or tinted while it has not said yet, and those ahead empty. A link has four stages and a local video three, so the fourth segment shows only for a link. Assistive technology reads it as a progress bar of the stages behind the job.
type stepsBar struct{ item func() Film }

// Render implements core.Node.
func (s stepsBar) Render(cx *ns.Scope) ns.Node {
	item := s.item
	colors := ui.ColorsOf(cx)
	segment := func(i int) ns.Node {
		fill := ns.View{Style: ns.Style{Height: ns.Pct(100), CornerRadius: 2}, StyleFunc: func() ns.Style {
			f := StepFills(item())
			if i >= len(f) || f[i] <= 0 {
				return ns.Style{Width: ns.Pt(0)}
			}
			// A sliver of progress is at least as wide as the bar is high, so it shows.
			return ns.Style{Width: ns.Pct(float32(f[i]) * 100), MinWidth: ns.Pt(4), Background: colors.Accent()}
		}}
		return ns.View{Style: ns.Style{FlexGrow: 1, FlexBasis: ns.Pt(0), Height: ns.Pct(100), CornerRadius: 2, Overflow: ns.Hidden},
			StyleFunc: func() ns.Style {
				it := item()
				_, done := Steps(it)
				// The stage the job is in, before its tool has said how far it is, is tinted whole.
				if i == done && Running(it) && !it.Progress.Known {
					return ns.Style{Background: colors.Accent().Faded(0.35)}
				}
				return ns.Style{Background: colors.Text().Faded(0.09)}
			},
			Children: []ns.Node{fill}}
	}
	fourth := ns.Show{When: func() bool { total, _ := Steps(item()); return total > 3 }, Then: func(*ns.Scope) ns.Node { return segment(3) }}
	return ns.View{
		TestID:    "queue-steps",
		Role:      ns.RoleProgressBar,
		LabelFunc: func() string { return locales.FilmsQueueProgress(cx) },
		AccessibilityValue: func() ns.AccessibilityValue {
			it := item()
			fills := StepFills(it)
			total, done := Steps(it)
			value := float32(done)
			if done < total {
				value += float32(fills[done])
			}
			return ns.AccessibilityValue{Min: 0, Max: float32(total), Now: value}
		},
		Style:    ns.Style{Direction: ns.Row, Width: ns.Pct(100), Height: ns.Pt(4), Gap: stepsGap, Margin: ns.Edges{Top: 2}},
		Children: []ns.Node{segment(0), segment(1), segment(2), fourth},
	}
}

// Remedy is what a failed card says when the app knows why its download failed, as the prototype words each kind: Headline in red (in the accent's ink while Working, as while yt-dlp updates), Why under it, and the Actions that fix it, the primary one first. 移除 always follows them.
type Remedy struct {
	Headline, Why string
	Working       bool
	Actions       []RemedyAction
}

// equal reports whether r says and offers what o does.
func (r Remedy) equal(o Remedy) bool {
	if r.Headline != o.Headline || r.Why != o.Why || r.Working != o.Working || len(r.Actions) != len(o.Actions) {
		return false
	}
	for i := range r.Actions {
		if r.Actions[i] != o.Actions[i] {
			return false
		}
	}
	return true
}

// RemedyAction is one button of a Remedy; App.RunRemedy hears its ID when it is clicked.
type RemedyAction struct {
	ID, Label         string
	Primary, Disabled bool
}

// A failed card's reason is in the theme's danger ink, and what to do about it in its strong ink (theme.Of).

// failure is what a failed or cancelled card says and offers: its remedy when the app knows why its download failed, else why it failed and the section of the settings that fixes it.
type failure struct {
	remedy          Remedy
	known           bool
	canceled        bool
	reason, section string
}

func (f failure) equal(o failure) bool {
	return f.remedy.equal(o.remedy) && f.known == o.known && f.canceled == o.canceled && f.reason == o.reason && f.section == o.section
}

// failedDetail is what a failed or cancelled card says and offers: for a download whose failure the app knows, its remedy (App.FailureRemedy); otherwise why, in red unless it was cancelled, and 打开设置 when a setting fixes it, 重试, and 报告… when none does. 移除 is the card's × (queueCard).
type failedDetail struct {
	a    App
	item func() Film
}

// Render implements core.Node.
func (f failedDetail) Render(cx *ns.Scope) ns.Node {
	a, item := f.a, f.item
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	look := ns.NewMemoFunc(cx, live.Func(a, func() failure {
		it := item()
		var out failure
		out.canceled = Canceled(it)
		if r, ok := a.FailureRemedy(it); ok && !out.canceled {
			out.remedy, out.known = r, true
			return out
		}
		out.reason, out.section = a.FailureHint(it)
		return out
	}), failure.equal).Get
	plain := func(*ns.Scope) ns.Node {
		reason := ns.Text{ContentFunc: func() string {
			if look().canceled {
				return locales.FilmsQueueCanceled(cx)
			}
			return look().reason
		}, MaxLines: 2, EllipsizeMode: ns.EllipsizeTail, Style: ns.Style{FontSize: 11}, StyleFunc: func() ns.Style {
			if look().canceled {
				return ns.Style{Color: colors.Muted()}
			}
			return ns.Style{Color: k.DangerInk()}
		}}
		settings := ns.Show{When: func() bool { return look().section != "" }, Then: func(*ns.Scope) ns.Node {
			return textButton(locales.FilmsOpenSettings, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "queue-settings",
				OnPress: live.Do(a, func() { a.OpenSettings(look().section) })})
		}}
		retry := textButton(locales.FilmsRetry, ui.Button{Size: ui.SizeSmall, TestID: "queue-retry", OnPress: live.Do(a, func() { a.Retry(item().JobID) })})
		// A failure no setting fixes may be Kando's own: its whole error shows over the reason, which is cut short, and 报告… files it with the job attached.
		detail := live.Func(a, func() string { return a.FailureDetail(item()) })
		why := ns.Show{When: func() bool { return look().canceled || detail() == "" }, Then: func(*ns.Scope) ns.Node { return reason },
			Else: func(*ns.Scope) ns.Node {
				return ui.Tooltip{TestID: "queue-reason-tip", TextFunc: detail, Trigger: reason}
			}}
		report := ns.Show{When: func() bool { return !look().canceled && look().section == "" }, Then: func(*ns.Scope) ns.Node {
			return textButton(locales.FilmsReport, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "queue-report", OnPress: live.Do(a, func() { a.ReportFailure(item().JobID) })})
		}}
		return ns.Group{why, actionRow(settings, retry, report)}
	}
	remedy := func(*ns.Scope) ns.Node {
		headline := ns.Text{ContentFunc: func() string { return look().remedy.Headline }, MaxLines: 2, EllipsizeMode: ns.EllipsizeTail, Style: ns.Style{FontSize: 11},
			StyleFunc: func() ns.Style {
				if look().remedy.Working {
					return ns.Style{Color: k.AccentInk()}
				}
				return ns.Style{Color: k.DangerInk()}
			}}
		why := ns.Show{When: func() bool { return look().remedy.Why != "" }, Then: func(*ns.Scope) ns.Node {
			return ns.Text{ContentFunc: func() string { return look().remedy.Why }, Style: ns.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(k.Strong)}
		}}
		actions := ns.Each(func() []RemedyAction { return look().remedy.Actions }, func(r RemedyAction) string { return r.ID },
			func(cx *ns.Scope, action func() RemedyAction, _ func() int) ns.Node {
				// The remedies' words come from the app in the interface's language and follow the action; ui.Button takes its variant once, so the button is built again only when the action turns primary or back.
				primary := ns.NewMemo(cx, func() bool { return action().Primary })
				id := ns.Untracked(cx, action).ID
				return ns.Dynamic(func(*ns.Scope) ns.Node {
					variant := ui.ButtonSecondary
					if primary.Get() {
						variant = ui.ButtonPrimary
					}
					return ui.Button{TextFunc: func() string { return action().Label }, Variant: variant, Size: ui.SizeSmall, TestID: "queue-remedy-" + id,
						Disabled: func() bool { return action().Disabled },
						OnPress:  live.Do(a, func() { a.RunRemedy(item(), action().ID) })}
				})
			})
		return ns.Group{headline, why, actionRow(actions)}
	}
	return ns.Show{When: func() bool { return look().known }, Then: remedy, Else: plain}
}

// actionRow is the buttons under a failed card's reason, wrapping when the card is narrow.
func actionRow(buttons ...ns.Node) ns.Node {
	return ns.View{Style: ns.Style{Direction: ns.Row, Wrap: ns.Wrap, Gap: 6, Margin: ns.Edges{Top: 2}}, Children: buttons}
}
