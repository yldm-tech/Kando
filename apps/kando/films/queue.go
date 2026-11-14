package films

import (
	"fmt"
	"strings"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// queueSection is 处理中: a card for every job that is waiting, running, failed or was cancelled, in QueueOrder. Past QueueShown cards it shows the first ones and a button for the rest.
type queueSection struct {
	a     App
	queue func() []Film
}

// Render implements core.Node.
func (q queueSection) Render(cx *ns.Scope) ns.Node {
	a := q.a
	expanded := live.Func(a, func() bool { return *a.QueueExpanded() })
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
	more := ns.Show{When: func() bool { return len(q.queue()) > QueueShown }, Then: func(*ns.Scope) ns.Node {
		return ns.View{Style: ns.Style{AlignSelf: ns.AlignStart}, Children: []ns.Node{
			// The button says 显示全部 with the count, or 收起, so it is built again when either changes.
			ns.Dynamic(func(cx *ns.Scope) ns.Node {
				label := locales.FilmsQueueShowAll(cx, len(q.queue()))
				if expanded() {
					label = locales.FilmsQueueCollapse(cx)
				}
				return ui.Button{Text: label, Variant: ui.ButtonSecondary, TestID: "queue-more", OnPress: live.Do(a, func() { *a.QueueExpanded() = !*a.QueueExpanded() })}
			}),
		}}
	}}
	summary := func() string { return QueueSummary(locales.For(cx), q.queue()) }
	return ns.View{Style: ns.Style{Margin: ns.Edges{Top: 26}, Gap: 12}, Children: []ns.Node{
		sectionTitle(cx, func() string { return locales.FilmsQueueTitle(cx) }, summary),
		cards,
		more,
	}}
}

// retryInk is the orange of a card whose translation waits out a rate limit.
var retryInk = ns.RGB(0xA35200)

// cardFill is the face of a processing card, a step darker than the page, and cardHairline its edge.
var cardFill, cardHairline = ns.RGB(0xF5F5F7), ns.RGBA(0x1D1D1F, 0.06)

// thumbFill is a thumbnail's face before its picture is there, and placeholderInk the glyph on it.
var thumbFill, placeholderInk = ns.RGB(0xDCDDE2), ns.RGB(0x8E8E95)

// queueCard is one job in 处理中: its thumbnail, its title, what it is doing or why it failed, the stages behind it, and what can be done about it.
type queueCard struct {
	a    App
	item func() Film
}

// Render implements core.Node.
func (q queueCard) Render(cx *ns.Scope) ns.Node {
	a, item := q.a, q.item
	colors := ui.ColorsOf(cx)
	failed := func() bool { return item().Stage == core.StageFailed }
	library := ns.Show{When: func() bool { return item().Library != "" }, Then: func(*ns.Scope) ns.Node {
		return ns.Text{ContentFunc: func() string { return locales.FilmsQueueSavingTo(cx, item().Library) }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 11}, StyleFunc: ui.Colored(colors.Muted)}
	}}
	working := func(*ns.Scope) ns.Node {
		meta := ns.Text{ContentFunc: func() string { return QueueMeta(locales.For(cx), item()) }, Style: ns.Style{FontSize: 11},
			StyleFunc: func() ns.Style {
				if RetryWaiting(item()) {
					return ns.Style{Color: retryInk}
				}
				return ns.Style{Color: colors.Muted()}
			}}
		return ns.Group{meta, stepsBar{item: item}}
	}
	// The end keeps clear of the cancel button over the card's corner.
	text := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 4, Padding: ns.Edges{Right: 18}}, Children: []ns.Node{
		ns.Text{ContentFunc: func() string { return filmName(item()) }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail, Style: ns.Style{FontSize: 13, FontWeight: ui.Semibold},
			StyleFunc: ui.Colored(colors.Text)},
		library,
		ns.Show{When: failed, Then: func(*ns.Scope) ns.Node { return failedDetail{a: a, item: item} }, Else: working},
	}}
	cancel := ns.Show{When: func() bool { return !failed() }, Then: func(cx *ns.Scope) ns.Node {
		return base.Button{TestID: "queue-cancel", LabelFunc: func() string { return locales.FilmsQueueCancel(cx) },
			OnPress: live.On(a, func(ns.PressEvent) { a.Cancel(item().JobID) }),
			Style: ns.Style{Position: ns.Absolute, Top: ns.Pt(8), Right: ns.Pt(8), Width: ns.Pt(22), Height: ns.Pt(22), CornerRadius: 11, AlignItems: ns.AlignCenter,
				JustifyContent: ns.JustifyCenter, Cursor: ns.CursorDefault, Hover: &ns.Style{Background: ns.RGBA(0x000000, 0.06)}},
			Children: []ns.Node{ui.Icon(svg(closeIcon), 10, colors.Muted)}}
	}}
	return ns.View{
		Role:      ns.RoleGroup,
		LabelFunc: func() string { return filmName(item()) },
		TestID:    "queue-" + item().Key,
		Style: ns.Style{Direction: ns.Row, Width: ns.Pct(100), MinWidth: ns.Pt(0), Gap: 12, Padding: ns.All(10), CornerRadius: 16, Background: cardFill,
			BorderWidth: 0.5, BorderColor: cardHairline},
		StyleFunc: func() ns.Style {
			if failed() {
				return ns.Style{AlignItems: ns.AlignStart}
			}
			return ns.Style{AlignItems: ns.AlignCenter}
		},
		Children: []ns.Node{queueThumb{a: a, item: item}, text, cancel},
	}
}

// failedVeil is the dark veil over a failed job's thumbnail, under its red mark.
var failedVeil = ns.RGBA(0x000000, 0.45)

// queueThumb is a processing card's 16:9 thumbnail: the poster once there is one, the picture the video's site gives it before that, a download glyph until either is there, and a red mark over a failed job's.
type queueThumb struct {
	a    App
	item func() Film
}

// Render implements core.Node.
func (q queueThumb) Render(*ns.Scope) ns.Node {
	a, item := q.a, q.item
	picture := live.Func(a, func() ns.ImageSource {
		if img := a.Poster(item().VideoPath); hasImage(img) {
			return img
		}
		return a.Thumbnail(item())
	})
	return ns.View{
		Style: ns.Style{Width: ns.Pt(112), FlexShrink: 0, AspectRatio: 16.0 / 9, CornerRadius: 9, Overflow: ns.Hidden, Background: thumbFill, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter},
		Children: []ns.Node{
			ns.Show{When: func() bool { return hasImage(picture()) }, Then: func(*ns.Scope) ns.Node { return cover(picture) },
				Else: func(*ns.Scope) ns.Node { return ui.Icon(svg(downloadIcon), 22, ui.Fixed(placeholderInk)) }},
			ns.Show{When: func() bool { return item().Stage == core.StageFailed && !Canceled(item()) }, Then: func(*ns.Scope) ns.Node {
				return ns.View{Style: overlay.Merge(ns.Style{Background: failedVeil, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter}), Children: []ns.Node{
					ns.SVGView{SVG: failedMark, Style: ns.Style{Width: ns.Pt(22), Height: ns.Pt(22)}},
				}}
			}},
		},
	}
}

// QueueMeta is the line under a waiting or running job's title, in the language of m: the stage and how far it has gone, as its tool reports it, or what does the work until the tool has said.
func QueueMeta(m locales.Messages, item Film) string {
	p := item.Progress
	switch item.Stage {
	case core.StageQueued:
		return m.FilmsQueueWaiting()
	case core.StageDownloading:
		if Local(item) {
			return m.FilmsQueueReadingLocal()
		}
		if !p.Known {
			return m.FilmsQueueDownloadingFrom(SourceLabel(m, item))
		}
		parts := []string{m.FilmsStageDownloading(), Percent(p.Fraction)}
		if p.Speed > 0 {
			parts = append(parts, Rate(p.Speed))
		}
		if p.ETA > 0 {
			parts = append(parts, m.FilmsQueueRemaining(Clock(p.ETA)))
		}
		return strings.Join(parts, " · ")
	case core.StageTranscribing:
		if p.Known {
			return m.FilmsQueueTranscribingPercent(Percent(p.Fraction))
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
		if p.Batches > 0 {
			return m.FilmsQueueTranslatingBatch(p.Batch, p.Batches, p.BatchSize)
		}
		return m.FilmsQueueTranslating()
	case core.StageExporting:
		return m.FilmsQueueExporting()
	}
	return StageLabel(m, item.Stage)
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

// Remedy is what a failed card says when the app knows why its download failed, as the prototype words each kind: Headline in red (in the accent while Working, as while yt-dlp updates), Why under it, and the Actions that fix it, the primary one first. 移除 always follows them.
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

// failInk is the red of a failed card's reason, and whyInk the dark gray of what to do about it.
var failInk, whyInk = ns.RGB(0xC4291C), ns.RGB(0x3A3A3F)

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

// failedDetail is what a failed or cancelled card says and offers: for a download whose failure the app knows, its remedy (App.FailureRemedy); otherwise why, in red unless it was cancelled, and 打开设置 when a setting fixes it, 重试 and 移除.
type failedDetail struct {
	a    App
	item func() Film
}

// Render implements core.Node.
func (f failedDetail) Render(cx *ns.Scope) ns.Node {
	a, item := f.a, f.item
	colors := ui.ColorsOf(cx)
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
	remove := textButton(locales.FilmsRemove, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "queue-remove", OnPress: live.Do(a, func() { a.Remove(item().JobID) })})
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
			return ns.Style{Color: failInk}
		}}
		settings := ns.Show{When: func() bool { return look().section != "" }, Then: func(*ns.Scope) ns.Node {
			return textButton(locales.FilmsOpenSettings, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "queue-settings",
				OnPress: live.Do(a, func() { a.OpenSettings(look().section) })})
		}}
		retry := textButton(locales.FilmsRetry, ui.Button{Size: ui.SizeSmall, TestID: "queue-retry", OnPress: live.Do(a, func() { a.Retry(item().JobID) })})
		return ns.Group{reason, actionRow(settings, retry, remove)}
	}
	remedy := func(*ns.Scope) ns.Node {
		headline := ns.Text{ContentFunc: func() string { return look().remedy.Headline }, MaxLines: 2, EllipsizeMode: ns.EllipsizeTail, Style: ns.Style{FontSize: 11},
			StyleFunc: func() ns.Style {
				if look().remedy.Working {
					return ns.Style{Color: colors.Accent()}
				}
				return ns.Style{Color: failInk}
			}}
		why := ns.Show{When: func() bool { return look().remedy.Why != "" }, Then: func(*ns.Scope) ns.Node {
			return ns.Text{ContentFunc: func() string { return look().remedy.Why }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: whyInk}}
		}}
		actions := ns.Each(func() []RemedyAction { return look().remedy.Actions }, func(r RemedyAction) string { return r.ID },
			func(cx *ns.Scope, action func() RemedyAction, _ func() int) ns.Node {
				// The remedies' words come from the app in the interface's language, so a button is built again when its action says something else.
				return ns.Dynamic(func(*ns.Scope) ns.Node {
					r := action()
					variant := ui.ButtonSecondary
					if r.Primary {
						variant = ui.ButtonPrimary
					}
					return ui.Button{Text: r.Label, Variant: variant, Size: ui.SizeSmall, TestID: "queue-remedy-" + r.ID,
						Disabled: func() bool { return action().Disabled },
						OnPress:  live.Do(a, func() { a.RunRemedy(item(), action().ID) })}
				})
			})
		return ns.Group{headline, why, actionRow(actions, remove)}
	}
	return ns.Show{When: func() bool { return look().known }, Then: remedy, Else: plain}
}

// actionRow is the buttons under a failed card's reason, wrapping when the card is narrow.
func actionRow(buttons ...ns.Node) ns.Node {
	return ns.View{Style: ns.Style{Direction: ns.Row, Wrap: ns.Wrap, Gap: 6, Margin: ns.Edges{Top: 2}}, Children: buttons}
}
