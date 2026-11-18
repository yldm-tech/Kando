package subscribe

import (
	"strconv"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/ui"
)

// Page is a subscription's page, beside the sidebar: its site's mark, its kind, name and status, with 立即检查, 暂停 or 恢复 and 取消订阅; the banner of a rename or a deletion on its site; the message about the last thing done to it; the new videos waiting for the user, to add or skip; its rules; and the videos it added with where each is now. It is built again for another subscription and when the interface's language changes, as the words its choices take are plain strings; within it what changes follows in bindings, so the keyword field keeps the focus as it is typed in.
type Page struct{ App App }

// Render implements ns.Node.
func (p Page) Render(cx *ns.Scope) ns.Node {
	a := p.App
	shown := live.Memo(cx, a, a.SubscriptionShown)
	return ns.Dynamic(func(cx *ns.Scope) ns.Node {
		_, _ = shown(), i18n.Language(cx)
		return ns.Func(func(cx *ns.Scope) ns.Node { return page{a}.build(cx) })
	})
}

type page struct{ a App }

func (p page) build(cx *ns.Scope) ns.Node {
	a := p.a
	head := live.Memo(cx, a, a.SubscriptionHead)
	content := ns.View{Style: ns.Style{Width: ns.Pct(100), MaxWidth: ns.Pt(760), Gap: 6, AlignItems: ns.AlignStretch, Padding: ns.Edges{Bottom: 28}},
		Children: []ns.Node{p.header(cx, head), p.lines(cx, head), p.banner(cx, head), p.waiting(cx), p.rules(cx), p.added(cx, head)}}
	return ns.Scroll{TestID: "subscription-page", Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{content}}
}

// header is the mark, the kicker with 已暂停, the name and the status line, and the page's three buttons.
func (p page) header(cx *ns.Scope, head func() Head) ns.Node {
	a := p.a
	colors := ui.ColorsOf(cx)
	h := head()
	paused := ns.Show{When: func() bool { return head().Paused }, Then: func(*ns.Scope) ns.Node { return form.Badge(locales.SubscribePausedTag(cx)) }}
	kicker := ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignCenter}, Children: []ns.Node{
		ns.Text{TestID: "subscription-kicker", ContentFunc: func() string { return kickerOf(cx, head()) }, MaxLines: 1, Style: ns.Style{FontSize: 11, LineHeight: 16, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Muted)},
		paused,
	}}
	name := ns.Text{TestID: "subscription-name", Role: ns.RoleHeader, ContentFunc: func() string { return head().Name }, MaxLines: 2, Style: ns.Style{FontSize: 20, LineHeight: 26, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)}
	status := ns.Text{TestID: "subscription-status", ContentFunc: func() string { return statusOf(cx, head(), time.Now()) }, Style: ns.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)}
	titles := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 2}, Children: []ns.Node{kicker, name, status}}
	check := form.Button{TestID: "subscription-check",
		TextFunc: func() string {
			if head().Checking {
				return locales.SubscribeChecking(cx)
			}
			return locales.SubscribeCheckNow(cx)
		},
		Disabled: func() bool { h := head(); return h.Checking || h.Paused || h.Stopped },
		OnPress:  live.Do(a, a.CheckSubscription)}
	pause := form.Button{TestID: "subscription-pause",
		TextFunc: func() string {
			if head().Paused {
				return locales.SubscribeResume(cx)
			}
			return locales.SubscribePause(cx)
		},
		Disabled: func() bool { return head().Stopped },
		OnPress:  live.Do(a, func() { a.PauseSubscription(!head().Paused) })}
	remove := form.Button{TestID: "subscription-unsubscribe", Text: locales.SubscribeUnsubscribe(cx), OnPress: live.Do(a, a.Unsubscribe)}
	buttons := ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, AlignItems: ns.AlignCenter, FlexShrink: 0}, Children: []ns.Node{check, pause, remove}}
	return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 14, AlignItems: ns.AlignCenter, Wrap: ns.Wrap, Padding: ns.Edges{Bottom: 10}},
		Children: []ns.Node{Mark(h.Platform, 44), titles, buttons}}
}

// kickerOf is the line above the name: the site and the kind.
func kickerOf(cx *ns.Scope, h Head) string {
	return locales.SubscribeKicker(cx, siteName(cx, h.Platform), kindName(cx, h.Kind))
}

// siteName names the site of platform as the sidebar's sources do.
func siteName(cx *ns.Scope, platform core.Platform) string {
	switch platform {
	case core.PlatformYouTube:
		return "YouTube"
	case core.PlatformBilibili:
		return locales.SidebarSourceBilibili(cx)
	}
	return locales.SidebarSourceWeb(cx)
}

// kindName names a Kind.
func kindName(cx *ns.Scope, kind Kind) string {
	switch kind {
	case KindChannel:
		return locales.SubscribeKindChannel(cx)
	case KindSpace:
		return locales.SubscribeKindSpace(cx)
	case KindCollection:
		return locales.SubscribeKindCollection(cx)
	}
	return locales.SubscribeKindPlaylist(cx)
}

// statusOf is the status line at now: paused, gone or kept without checks, or when it was last checked, how often it is and how many videos it added.
func statusOf(cx *ns.Scope, h Head, now time.Time) string {
	switch {
	case h.Paused:
		return locales.SubscribeStatusPaused(cx)
	case h.Gone && !h.Stopped:
		return locales.SubscribeStatusGone(cx)
	}
	checked := locales.SubscribeStatusNever(cx)
	if !h.Checked.IsZero() {
		checked = locales.SubscribeStatusChecked(cx, ago(cx, now.Sub(h.Checked)))
	}
	cadence := locales.SubscribeCadenceOpenNote(cx)
	switch {
	case h.Stopped:
		cadence = locales.SubscribeStatusGone(cx)
	case h.Cadence == CadenceHourly:
		cadence = locales.SubscribeCadenceHourlyNote(cx)
	case h.Cadence == CadenceDaily:
		cadence = locales.SubscribeCadenceDailyNote(cx)
	}
	return locales.SubscribeStatusLine(cx, checked, cadence, locales.SubscribeAddedCount(cx, h.Added))
}

// ago says how long ago d was: just now, or in minutes, hours or days.
func ago(cx *ns.Scope, d time.Duration) string {
	switch {
	case d < time.Minute:
		return locales.SubscribeAgoNow(cx)
	case d < time.Hour:
		return locales.SubscribeAgoMinutes(cx, int(d/time.Minute))
	case d < 24*time.Hour:
		return locales.SubscribeAgoHours(cx, int(d/time.Hour))
	}
	return locales.SubscribeAgoDays(cx, int(d/(24*time.Hour)))
}

// lines are the message about the last thing done, in the success or the warning ink, and why the last check failed.
func (p page) lines(cx *ns.Scope, head func() Head) ns.Node {
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	note := ns.Show{When: func() bool { return noteOf(cx, head().Note) != "" }, Then: func(*ns.Scope) ns.Node {
		return ns.Text{TestID: "subscription-note", Role: ns.RoleStatus, ContentFunc: func() string { return noteOf(cx, head().Note) },
			Style: ns.Style{FontSize: 12, LineHeight: 17, Padding: ns.Symmetric(8, 12), CornerRadius: 8},
			StyleFunc: func() ns.Style {
				if noteWarns(head().Note.Kind) {
					return ns.Style{Color: k.WarnText(), Background: k.WarnFill()}
				}
				return ns.Style{Color: k.OKText(), Background: k.OKFill()}
			}}
	}}
	failed := ns.Show{When: func() bool { h := head(); return (h.Err != "" || h.TimedOut) && !h.Gone }, Then: func(*ns.Scope) ns.Node {
		return ns.Text{TestID: "subscription-error", Selectable: ns.SelectableOn, ContentFunc: func() string { return locales.SubscribeStatusFailed(cx, failure(cx, head())) },
			Style: ns.Style{FontSize: 12, LineHeight: 17, Padding: ns.Symmetric(0, 2)}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	return ns.View{Style: ns.Style{Gap: 6, AlignItems: ns.AlignStretch}, Children: []ns.Node{note, failed}}
}

// failure says why the last check failed: yt-dlp took too long, or what it said.
func failure(cx *ns.Scope, h Head) string {
	if h.TimedOut {
		return locales.PlaylistTimedOut(cx)
	}
	return h.Err
}

// noteWarns reports whether a note says that something did not go as asked.
func noteWarns(kind NoteKind) bool {
	return kind == NoteNotQueued || kind == NoteFailed || kind == NoteGone
}

// noteOf says n in the language the page shows, "" for no note.
func noteOf(cx *ns.Scope, n Note) string {
	switch n.Kind {
	case NoteSubscribed:
		return locales.SubscribeNoteSubscribed(cx, n.Text)
	case NoteAlready:
		return locales.SubscribeNoteAlready(cx)
	case NoteChecked:
		text := locales.SubscribeNoteNone(cx)
		switch {
		case n.Count > 0 && n.Held:
			text = locales.SubscribeNoteHeld(cx, n.Count)
		case n.Count > 0:
			text = locales.SubscribeNoteQueued(cx, n.Count)
		}
		if n.Skipped > 0 {
			text += " " + locales.SubscribeNoteSkipped(cx, n.Skipped)
		}
		return text
	case NoteNotQueued:
		return locales.SubscribeNoteNotQueued(cx, n.Text)
	case NoteAdded:
		return locales.SubscribeNoteAdded(cx, n.Count)
	case NoteRenamed:
		return locales.SubscribeNoteRenamed(cx, n.Text)
	case NoteKeptName:
		return locales.SubscribeNoteKeptName(cx)
	case NoteStopped:
		return locales.SubscribeNoteStopped(cx, n.Text)
	case NoteGone:
		return locales.SubscribeNoteGone(cx)
	case NoteFailed:
		return locales.SubscribeNoteFailed(cx, n.Text)
	}
	return ""
}

// banner is the warning about a rename on the site, with keeping either name, or about the list being deleted there, with keeping it without checks; both offer to unsubscribe.
func (p page) banner(cx *ns.Scope, head func() Head) ns.Node {
	a := p.a
	k := theme.Of(cx)
	warn := live.Memo(cx, a, func() Warn {
		h := head()
		return WarnOf(Sub{Gone: h.Gone, Stopped: h.Stopped, Renamed: h.Renamed})
	})
	box := func(title, body func() string, buttons ...ns.Node) ns.Node {
		buttons = append(buttons, form.Button{TestID: "subscription-banner-unsubscribe", Text: locales.SubscribeUnsubscribe(cx), OnPress: live.Do(a, a.Unsubscribe)})
		return ns.View{TestID: "subscription-banner", Role: ns.RoleStatus,
			Style: ns.Style{Gap: 8, Padding: ns.Symmetric(10, 12), CornerRadius: 10, AlignItems: ns.AlignStretch},
			StyleFunc: func() ns.Style {
				return ns.Style{Background: k.WarnFill(), Shadows: []ns.BoxShadow{{Spread: 0.5, Color: k.WarnRing()}}}
			},
			Children: []ns.Node{
				ns.Text{ContentFunc: title, Style: ns.Style{FontSize: 13, LineHeight: 18, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(k.WarnText)},
				ns.Text{ContentFunc: body, Style: ns.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(k.WarnText)},
				ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, Wrap: ns.Wrap}, Children: buttons},
			}}
	}
	return ns.Dynamic(func(cx *ns.Scope) ns.Node {
		switch warn() {
		case WarnRenamed:
			return box(func() string { return locales.SubscribeRenamedTitle(cx, head().Name) }, func() string { return locales.SubscribeRenamedBody(cx, head().Renamed) },
				form.Button{TestID: "subscription-use-new-name", Text: locales.SubscribeUseNewName(cx), OnPress: live.Do(a, func() { a.KeepSubscriptionName(true) })},
				form.Button{TestID: "subscription-keep-old-name", Text: locales.SubscribeKeepOldName(cx), OnPress: live.Do(a, func() { a.KeepSubscriptionName(false) })})
		case WarnGone:
			return box(func() string { return locales.SubscribeGoneTitle(cx, head().Name) }, func() string { return locales.SubscribeGoneBody(cx, head().Added) },
				form.Button{TestID: "subscription-stop", Text: locales.SubscribeStopChecking(cx), OnPress: live.Do(a, a.StopSubscription)})
		}
		return nil
	})
}

// waiting is the new videos waiting for the user, under a heading with their count and 全部加入 and 全部跳过: each with its title, its length, whether it fits the rules as they are now, and its own 加入 and 跳过.
func (p page) waiting(cx *ns.Scope) ns.Node {
	a := p.a
	colors := ui.ColorsOf(cx)
	items := live.Func(a, a.SubscriptionWaiting)
	return ns.Show{When: func() bool { return len(items()) > 0 }, Then: func(cx *ns.Scope) ns.Node {
		count := ns.Text{TestID: "subscription-waiting-count", ContentFunc: func() string { return locales.SubscribePendingCount(cx, len(items())) }, Style: ns.Style{FontSize: 11, LineHeight: 16, Margin: ns.Edges{Right: 10}}, StyleFunc: ui.Colored(colors.Muted)}
		label := form.Label(locales.SubscribePendingTitle(cx), 6,
			count,
			form.Button{TestID: "subscription-add-all", Text: locales.SubscribeAddAll(cx), OnPress: live.Do(a, func() { a.AddWaiting(nil) })},
			ns.View{Style: ns.Style{Width: ns.Pt(6)}},
			form.Button{TestID: "subscription-skip-all", Text: locales.SubscribeSkipAll(cx), OnPress: live.Do(a, func() { a.SkipWaiting(nil) })})
		rows := ns.Each(items, func(w Waiting) string { return w.URL }, func(cx *ns.Scope, item func() Waiting, index func() int) ns.Node {
			w := item()
			meta := func() string {
				w := item()
				text := ""
				if w.Duration > 0 {
					text = films.Clock(w.Duration)
				}
				if !w.Fits {
					if text != "" {
						text += " · "
					}
					text += locales.SubscribeMisfit(cx)
				}
				return text
			}
			titles := form.Titles{Title: w.Title, SubtitleFunc: meta}
			add := ui.Button{Text: locales.SubscribeAddOne(cx), Label: locales.SubscribeAddOneLabel(cx, w.Title), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "subscription-add-" + w.URL,
				OnPress: live.Do(a, func() { a.AddWaiting([]string{item().URL}) })}
			skip := ui.Button{Text: locales.SubscribeSkipOne(cx), Label: locales.SubscribeSkipOneLabel(cx, w.Title), Variant: ui.ButtonGhost, Size: ui.SizeSmall, TestID: "subscription-skip-" + w.URL,
				OnPress: live.Do(a, func() { a.SkipWaiting([]string{item().URL}) })}
			row := ns.View{Style: ns.Style{Direction: ns.Row, MinHeight: ns.Pt(46), Padding: ns.Symmetric(6, 14), Gap: 8, AlignItems: ns.AlignCenter},
				Children: []ns.Node{titles, add, skip}}
			return ns.View{Children: []ns.Node{
				ns.Show{When: func() bool { return index() > 0 }, Then: func(*ns.Scope) ns.Node { return form.Separator(false) }},
				row,
			}}
		})
		return ns.View{Style: ns.Style{Gap: 6, AlignItems: ns.AlignStretch, Padding: ns.Edges{Bottom: 6}}, Children: []ns.Node{label, form.Card(rows)}}
	}}
}

// rules is the card of the rules: what happens to new videos, which ones are taken with the keyword field when keywords are chosen, and how often the subscription is checked.
func (p page) rules(cx *ns.Scope) ns.Node {
	a := p.a
	rules := live.Memo(cx, a, a.SubscriptionRules)
	mode := ui.Tabs[Mode]{TestID: "subscription-mode", Label: locales.SubscribeWhenNew(cx), Size: ui.SizeSmall,
		Options: []ui.Option[Mode]{{Value: ModeAuto, Label: locales.SubscribeModeAuto(cx)}, {Value: ModeAsk, Label: locales.SubscribeModeAsk(cx)}},
		Value:   func() Mode { return rules().Mode },
		OnChange: live.On(a, func(m Mode) {
			r := rules()
			r.Mode = m
			a.SetSubscriptionRules(r)
		})}
	filter := ui.Tabs[Filter]{TestID: "subscription-filter", Label: locales.SubscribeOnly(cx), Size: ui.SizeSmall,
		Options: []ui.Option[Filter]{{Value: FilterAll, Label: locales.SubscribeFilterAll(cx)}, {Value: FilterLong, Label: locales.SubscribeFilterLong(cx)}, {Value: FilterKeywords, Label: locales.SubscribeFilterKeywords(cx)}},
		Value:   func() Filter { return rules().Filter },
		OnChange: live.On(a, func(f Filter) {
			r := rules()
			r.Filter = f
			a.SetSubscriptionRules(r)
		})}
	keywords := ns.Show{When: func() bool { return rules().Filter == FilterKeywords }, Then: func(cx *ns.Scope) ns.Node {
		field := ui.Input{TestID: "subscription-keywords", Label: locales.SubscribeKeywords(cx), Placeholder: locales.SubscribeKeywordsHint(cx),
			Value: func() string { return rules().Keywords },
			OnChange: live.On(a, func(text string) {
				r := rules()
				r.Keywords = text
				a.SetSubscriptionRules(r)
			}),
			Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0)}}
		return ns.View{Style: ns.Style{Direction: ns.Row, Padding: ns.Edges{Right: 14, Bottom: 10, Left: 14}}, Children: []ns.Node{field}}
	}}
	cadence := ui.Tabs[Cadence]{TestID: "subscription-cadence", Label: locales.SubscribeCadence(cx), Size: ui.SizeSmall,
		Options: []ui.Option[Cadence]{{Value: CadenceOpen, Label: locales.SubscribeCadenceOpen(cx)}, {Value: CadenceHourly, Label: locales.SubscribeCadenceHourly(cx)}, {Value: CadenceDaily, Label: locales.SubscribeCadenceDaily(cx)}},
		Value:   func() Cadence { return rules().Cadence },
		OnChange: live.On(a, func(c Cadence) {
			r := rules()
			r.Cadence = c
			a.SetSubscriptionRules(r)
		})}
	modeNote := func() string {
		if rules().Mode == ModeAsk {
			return locales.SubscribeWhenNewAsk(cx)
		}
		return locales.SubscribeWhenNewAuto(cx)
	}
	return ns.View{Style: ns.Style{Gap: 6, AlignItems: ns.AlignStretch}, Children: []ns.Node{
		form.Label(locales.SubscribeRules(cx), 6),
		form.Card(
			form.Row{Title: locales.SubscribeWhenNew(cx), SubtitleFunc: modeNote, Trailing: []ns.Node{mode}},
			form.Separator(true),
			form.Row{Title: locales.SubscribeOnly(cx), Subtitle: locales.SubscribeOnlyNote(cx), Trailing: []ns.Node{filter}},
			keywords,
			form.Separator(true),
			form.Row{Title: locales.SubscribeCadence(cx), Subtitle: locales.SubscribeCadenceNote(cx), Trailing: []ns.Node{cadence}},
		),
	}}
}

// added is the videos the subscription added, newest first, each with where it is now, or a line saying there are none yet.
func (p page) added(cx *ns.Scope, head func() Head) ns.Node {
	a := p.a
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	items := live.Func(a, func() []Added {
		all := a.SubscriptionAdded()
		out := make([]Added, len(all))
		for i, v := range all {
			out[len(all)-1-i] = v
		}
		return out
	})
	count := ns.Text{TestID: "subscription-added-count", ContentFunc: func() string { return strconv.Itoa(head().Added) }, Style: ns.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)}
	rows := ns.Each(items, func(v Added) string { return v.URL }, func(cx *ns.Scope, item func() Added, index func() int) ns.Node {
		title := ns.Text{ContentFunc: func() string { return item().Title }, MaxLines: 1, Style: ns.Style{FontSize: 13, LineHeight: 18, FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0)}, StyleFunc: ui.Colored(colors.Text)}
		stage := ns.Text{ContentFunc: func() string { return stageName(cx, item().Stage) }, MaxLines: 1, Style: ns.Style{FontSize: 12, LineHeight: 17, FlexShrink: 0},
			StyleFunc: func() ns.Style {
				switch item().Stage {
				case StageRunning:
					return ns.Style{Color: k.AccentInk()}
				case StageDone:
					return ns.Style{Color: k.SuccessInk()}
				case StageFailed:
					return ns.Style{Color: k.DangerInk()}
				}
				return ns.Style{Color: colors.Muted()}
			}}
		return ns.View{Children: []ns.Node{
			ns.Show{When: func() bool { return index() > 0 }, Then: func(*ns.Scope) ns.Node { return form.Separator(false) }},
			ns.View{Style: ns.Style{Direction: ns.Row, MinHeight: ns.Pt(36), Padding: ns.Symmetric(6, 14), Gap: 10, AlignItems: ns.AlignCenter}, Children: []ns.Node{title, stage}},
		}}
	})
	list := ns.Show{When: func() bool { return len(items()) > 0 },
		Then: func(*ns.Scope) ns.Node { return form.Card(rows) },
		Else: func(cx *ns.Scope) ns.Node {
			return ns.Text{TestID: "subscription-added-none", Content: locales.SubscribeAddedNone(cx), Style: ns.Style{FontSize: 12, LineHeight: 17, Padding: ns.Symmetric(4, 12)}, StyleFunc: ui.Colored(colors.Muted)}
		}}
	return ns.View{Style: ns.Style{Gap: 6, AlignItems: ns.AlignStretch}, Children: []ns.Node{form.Label(locales.SubscribeAddedTitle(cx), 12, count), list}}
}

// stageName names a Stage.
func stageName(cx *ns.Scope, stage Stage) string {
	switch stage {
	case StageRunning:
		return locales.SubscribeStageRunning(cx)
	case StageDone:
		return locales.SubscribeStageDone(cx)
	case StageFailed:
		return locales.SubscribeStageFailed(cx)
	case StageGone:
		return locales.SubscribeStageGone(cx)
	}
	return locales.SubscribeStageQueued(cx)
}
