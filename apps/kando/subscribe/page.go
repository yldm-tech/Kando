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

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/ui"
)

// Page is a subscription's page, beside the sidebar, as the design lays it out: the message about the last thing done to it, which × puts away; a card with its site's mark, its site and kind, name and status, with 立即检查, 暂停 or 恢复 and 取消订阅; the banner of a rename or a deletion on its site; for a collection or a playlist subscribed before Kando added a list's videos, the ones it left out, which 列出来 offers; the new videos waiting for the user, to add or skip; its rules; and the videos it added with where each is now. It is built again for another subscription and when the interface's language changes, as the words its choices take are plain strings; within it what changes follows in bindings, so the keyword field keeps the focus as it is typed in.
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
	content := ns.View{Style: ns.Style{Width: ns.Pct(100), MaxWidth: ns.Pt(760), Gap: 14, AlignItems: ns.AlignStretch, Padding: ns.Edges{Bottom: 28}},
		Children: []ns.Node{p.note(cx, head), p.header(cx, head), p.banner(cx, head), p.failed(cx, head), p.earlier(cx, head), p.waiting(cx, head), p.rules(cx, head), p.added(cx, head)}}
	return ns.Scroll{TestID: "subscription-page", Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{content}}
}

// header is the design's card at the top: the mark, the site and kind tag with 已暂停, the name and the status line, and the page's three pills, 立即检查 filled with the accent and 取消订阅 in red.
func (p page) header(cx *ns.Scope, head func() Head) ns.Node {
	a := p.a
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	h := head()
	paused := ns.Show{When: func() bool { return head().Paused }, Then: func(*ns.Scope) ns.Node {
		return ns.Text{Content: locales.SubscribePausedTag(cx), MaxLines: 1, Style: ns.Style{FontSize: 10, LineHeight: 17, FontWeight: ui.Bold, Padding: ns.Symmetric(0, 7), CornerRadius: 7, FlexShrink: 0},
			StyleFunc: func() ns.Style { return ns.Style{Background: k.Shade(0.08), Color: colors.Muted()} }}
	}}
	kicker := ns.View{TestID: "subscription-kicker", Label: kickerOf(cx, h), Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignCenter}, Children: []ns.Node{SiteBadge(kickerOf(cx, h), h.Platform), paused}}
	name := ns.Text{TestID: "subscription-name", Role: ns.RoleHeader, ContentFunc: func() string { return head().Name }, MaxLines: 2, Style: ns.Style{FontSize: 20, LineHeight: 26, FontWeight: ui.Bold, Margin: ns.Edges{Top: 4}}, StyleFunc: ui.Colored(colors.Text)}
	status := ns.Text{TestID: "subscription-status", ContentFunc: func() string { return statusOf(cx, head(), time.Now()) }, Style: ns.Style{FontSize: 13, LineHeight: 18, Margin: ns.Edges{Top: 2}}, StyleFunc: ui.Colored(colors.Muted)}
	titles := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: ns.Pt(220), MinWidth: ns.Pt(0)}, Children: []ns.Node{kicker, name, status}}
	check := pill{TestID: "subscription-check", Look: LookPrimary, Large: true,
		Text: func() string {
			if head().Checking {
				return locales.SubscribeChecking(cx)
			}
			return locales.SubscribeCheckNow(cx)
		},
		// A list its site deleted has nothing to check now; the daily look for it coming back goes on (Sub.Due).
		Disabled: func() bool { h := head(); return h.Checking || h.Paused || h.Stopped || h.Gone },
		OnPress:  live.Do(a, a.CheckSubscription)}
	pause := pill{TestID: "subscription-pause", Look: LookSecondary, Large: true,
		Text: func() string {
			if head().Paused {
				return locales.SubscribeResume(cx)
			}
			return locales.SubscribePause(cx)
		},
		Disabled: func() bool { return head().Stopped },
		OnPress:  live.Do(a, func() { a.PauseSubscription(!head().Paused) })}
	remove := pill{TestID: "subscription-unsubscribe", Look: LookDanger, Large: true, Text: func() string { return locales.SubscribeUnsubscribe(cx) }, OnPress: live.Do(a, a.Unsubscribe)}
	buttons := ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, AlignItems: ns.AlignCenter, FlexShrink: 0, Wrap: ns.Wrap}, Children: []ns.Node{check, pause, remove}}
	return card(cx, ns.Style{Direction: ns.Row, Gap: 16, AlignItems: ns.AlignCenter, Wrap: ns.Wrap, Padding: ns.Symmetric(18, 20)},
		Mark(h.Platform, 60), titles, buttons)
}

// heading is a section's heading, as the design's 新视频 and 已加入: 15-point bold, then a count or a note in the secondary color, and the section's buttons at the end of its line.
func heading(cx *ns.Scope, title string, note func() string, noteID string, buttons ...ns.Node) ns.Node {
	colors := ui.ColorsOf(cx)
	children := []ns.Node{ns.Text{Content: title, Role: ns.RoleHeader, MaxLines: 1, Style: ns.Style{FontSize: 15, LineHeight: 20, FontWeight: ui.Bold, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Text)}}
	if note != nil {
		children = append(children, ns.Text{TestID: noteID, ContentFunc: note, MaxLines: 1, Style: ns.Style{FontSize: 12, LineHeight: 17, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Muted)})
	}
	children = append(children, ns.View{Style: ns.Style{FlexGrow: 1}})
	children = append(children, buttons...)
	return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, AlignItems: ns.AlignCenter, Padding: ns.Edges{Top: 4, Right: 4, Left: 4}}, Children: children}
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

// statusOf is the status line at now: paused, or gone on its site, then how many videos it added; or when it was last checked, how often it is, or that it is kept without checks, and how many it added.
func statusOf(cx *ns.Scope, h Head, now time.Time) string {
	added := locales.SubscribeAddedCount(cx, h.Added)
	switch {
	case h.Paused:
		return locales.SubscribeStatusPair(cx, locales.SubscribeStatusPaused(cx), added)
	case h.Gone && !h.Stopped:
		return locales.SubscribeStatusPair(cx, locales.SubscribeStatusGone(cx, string(h.Kind)), added)
	}
	checked := locales.SubscribeStatusNever(cx)
	if !h.Checked.IsZero() {
		checked = locales.SubscribeStatusChecked(cx, ago(cx, now.Sub(h.Checked)))
	}
	cadence := locales.SubscribeCadenceOpenNote(cx)
	switch {
	case h.Stopped:
		cadence = locales.SubscribeStatusGone(cx, string(h.Kind))
	case h.Cadence == CadenceHourly:
		cadence = locales.SubscribeCadenceHourlyNote(cx)
	case h.Cadence == CadenceDaily:
		cadence = locales.SubscribeCadenceDailyNote(cx)
	}
	return locales.SubscribeStatusLine(cx, checked, cadence, added)
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

// note is the message about the last thing done, a banner over the header card as the design draws it: a round mark, a check on green or a ! on orange, the message, and × that puts it away.
func (p page) note(cx *ns.Scope, head func() Head) ns.Node {
	a := p.a
	return ns.Show{When: func() bool { return noteOf(cx, head().Note) != "" }, Then: func(cx *ns.Scope) ns.Node {
		colors, k := ui.ColorsOf(cx), theme.Of(cx)
		warns := func() bool { return noteWarns(head().Note.Kind) }
		mark := ns.Text{ContentFunc: func() string {
			if warns() {
				return "!"
			}
			return "✓"
		}, Role: ns.RoleNone, MaxLines: 1,
			Style: ns.Style{Width: ns.Pt(22), Height: ns.Pt(22), CornerRadius: 11, FontSize: 13, LineHeight: 22, FontWeight: ui.Bold, TextAlign: ns.TextAlignCenter, FlexShrink: 0},
			StyleFunc: func() ns.Style {
				if warns() {
					return ns.Style{Background: k.WarnMark(), Color: colors.OnAccent()}
				}
				return ns.Style{Background: k.SuccessInk(), Color: colors.OnAccent()}
			}}
		text := ns.Text{TestID: "subscription-note", Role: ns.RoleStatus, ContentFunc: func() string { return noteOf(cx, head().Note) },
			Style: ns.Style{FontSize: 14, LineHeight: 20, FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0)},
			StyleFunc: func() ns.Style {
				if warns() {
					return ns.Style{Color: k.WarnText()}
				}
				return ns.Style{Color: k.OKText()}
			}}
		close := base.Button{TestID: "subscription-note-close", LabelFunc: func() string { return locales.SubscribeNoteClose(cx) }, OnPress: live.On(a, func(ns.PressEvent) { a.DismissSubscriptionNote() }),
			Desktop:   ns.DesktopViewProps{WindowDrag: ns.WindowDragOff},
			Style:     ns.Style{Width: ns.Pt(24), Height: ns.Pt(24), CornerRadius: 12, FlexShrink: 0, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter, Cursor: ns.CursorDefault, FocusVisible: ui.FocusRing(cx, -2)},
			StyleFunc: func() ns.Style { return ns.Style{Hover: &ns.Style{Background: k.Shade(0.06)}} },
			Children:  []ns.Node{ns.Text{Content: "×", Style: ns.Style{FontSize: 16}, StyleFunc: ui.Colored(colors.Muted)}}}
		return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 12, AlignItems: ns.AlignCenter, Padding: ns.Edges{Top: 12, Right: 12, Bottom: 12, Left: 16}, CornerRadius: 14, BorderWidth: 1},
			StyleFunc: func() ns.Style {
				if warns() {
					return ns.Style{Background: k.WarnFill(), BorderColor: k.WarnRing()}
				}
				return ns.Style{Background: k.OKFill(), BorderColor: k.OKRing()}
			},
			Children: []ns.Node{mark, text, close}}
	}}
}

// failed is why the last check failed, in the danger ink, while the list is not gone, which the banner says instead.
func (p page) failed(cx *ns.Scope, head func() Head) ns.Node {
	colors := ui.ColorsOf(cx)
	return ns.Show{When: func() bool { h := head(); return (h.Err != "" || h.TimedOut) && !h.Gone }, Then: func(*ns.Scope) ns.Node {
		return ns.Text{TestID: "subscription-error", Selectable: ns.SelectableOn, ContentFunc: func() string { return locales.SubscribeStatusFailed(cx, failure(cx, head())) },
			Style: ns.Style{FontSize: 12, LineHeight: 17, Padding: ns.Symmetric(0, 2)}, StyleFunc: ui.Colored(colors.Danger)}
	}}
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
		if n.Count > 0 {
			return locales.SubscribeNoteSubscribedAdded(cx, n.Text, n.Count)
		}
		return locales.SubscribeNoteSubscribed(cx, n.Text)
	case NoteAlready:
		return locales.SubscribeNoteAlready(cx)
	case NoteChecked:
		text := locales.SubscribeNoteNone(cx)
		switch {
		case n.Count > 0 && n.Held:
			text = locales.SubscribeNoteHeld(cx, n.Count)
		case n.Over > 0:
			text = locales.SubscribeNoteCapped(cx, n.Count, n.Count-n.Over, n.Over)
		case n.Count > 0:
			text = locales.SubscribeNoteQueued(cx, n.Count)
		}
		if n.Skipped > 0 {
			text += " " + locales.SubscribeNoteSkipped(cx, n.Skipped)
		}
		if n.Earlier > 0 {
			text += " " + locales.SubscribeNoteEarlier(cx, n.Earlier)
		}
		return text
	case NoteNotQueued:
		if n.Queued > 0 {
			return locales.SubscribeNoteSomeQueued(cx, n.Queued, n.Text)
		}
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

// banner is the warning about a rename on the site, with keeping either name, or about the list being deleted there, with keeping it without checks; both offer to unsubscribe, in red.
func (p page) banner(cx *ns.Scope, head func() Head) ns.Node {
	a := p.a
	k := theme.Of(cx)
	colors := ui.ColorsOf(cx)
	warn := live.Memo(cx, a, func() Warn {
		h := head()
		return WarnOf(Sub{Gone: h.Gone, Stopped: h.Stopped, Renamed: h.Renamed})
	})
	box := func(title, body func() string, buttons ...ns.Node) ns.Node {
		buttons = append(buttons, pill{TestID: "subscription-banner-unsubscribe", Look: LookDanger, Text: func() string { return locales.SubscribeUnsubscribe(cx) }, OnPress: live.Do(a, a.Unsubscribe)})
		bang := ns.Text{Content: "!", Role: ns.RoleNone, MaxLines: 1,
			Style:     ns.Style{Width: ns.Pt(24), Height: ns.Pt(24), CornerRadius: 12, FontSize: 14, LineHeight: 24, FontWeight: ui.Bold, TextAlign: ns.TextAlignCenter, FlexShrink: 0},
			StyleFunc: func() ns.Style { return ns.Style{Background: k.WarnMark(), Color: colors.OnAccent()} }}
		texts := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: ns.Pt(200), MinWidth: ns.Pt(0), Gap: 2}, Children: []ns.Node{
			ns.Text{ContentFunc: title, Style: ns.Style{FontSize: 14, LineHeight: 19, FontWeight: ui.Bold}, StyleFunc: ui.Colored(k.WarnText)},
			ns.Text{ContentFunc: body, Style: ns.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(k.WarnText)},
		}}
		return ns.View{TestID: "subscription-banner", Role: ns.RoleStatus,
			Style: ns.Style{Direction: ns.Row, Wrap: ns.Wrap, Gap: 12, Padding: ns.Symmetric(14, 18), CornerRadius: 14, AlignItems: ns.AlignCenter, BorderWidth: 1},
			StyleFunc: func() ns.Style {
				return ns.Style{Background: k.WarnFill(), BorderColor: k.WarnRing()}
			},
			Children: []ns.Node{bang, texts, ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, AlignItems: ns.AlignCenter, FlexShrink: 0}, Children: buttons}}}
	}
	return ns.Dynamic(func(cx *ns.Scope) ns.Node {
		switch warn() {
		case WarnRenamed:
			return box(func() string { return locales.SubscribeRenamedTitle(cx, string(head().Kind)) }, func() string { return locales.SubscribeRenamedBody(cx, head().Renamed) },
				pill{TestID: "subscription-use-new-name", Look: LookPrimary, Text: func() string { return locales.SubscribeUseNewName(cx) }, OnPress: live.Do(a, func() { a.KeepSubscriptionName(true) })},
				pill{TestID: "subscription-keep-old-name", Look: LookSecondary, Text: func() string { return locales.SubscribeKeepOldName(cx) }, OnPress: live.Do(a, func() { a.KeepSubscriptionName(false) })})
		case WarnGone:
			return box(func() string { return locales.SubscribeGoneTitle(cx, string(head().Kind)) }, func() string { return locales.SubscribeGoneBody(cx, head().Added) },
				pill{TestID: "subscription-stop", Look: LookPrimary, Text: func() string { return locales.SubscribeStopChecking(cx) }, OnPress: live.Do(a, a.StopSubscription)})
		}
		return nil
	})
}

// earlier is, on a subscription to a collection or a playlist made before Kando added a list's videos as it was subscribed to, the videos the list held then, which it counted as seen and did not add (Head.Earlier): how many at most, and 列出来, which checks the list so they wait below to be added or skipped (ListEarlier).
func (p page) earlier(cx *ns.Scope, head func() Head) ns.Node {
	a := p.a
	return ns.Show{When: func() bool { return head().Earlier > 0 }, Then: func(cx *ns.Scope) ns.Node {
		colors := ui.ColorsOf(cx)
		list := pill{TestID: "subscription-earlier-list", Look: LookPrimary,
			Text: func() string {
				if head().Checking {
					return locales.SubscribeChecking(cx)
				}
				return locales.SubscribeEarlierList(cx)
			},
			Disabled: func() bool { h := head(); return h.Checking || h.Paused || h.Stopped },
			OnPress:  live.Do(a, a.ListEarlier)}
		label := heading(cx, locales.SubscribeEarlierTitle(cx), func() string { return locales.SubscribeEarlierCount(cx, head().Earlier) }, "subscription-earlier-count", list)
		note := ns.Text{Content: locales.SubscribeEarlierNote(cx), Style: ns.Style{FontSize: 12, LineHeight: 17, Padding: ns.Symmetric(0, 4)}, StyleFunc: ui.Colored(colors.Muted)}
		return ns.View{TestID: "subscription-earlier", Style: ns.Style{Gap: 4, AlignItems: ns.AlignStretch}, Children: []ns.Node{label, note}}
	}}
}

// waiting is the new videos waiting for the user, under a heading with their count and 全部加入 and 全部跳过, in a card: each with the site's mark where the design has the video's picture, which a list does not give (media.ListedVideo), its title, its length, whether it fits the rules as they are now, and its own 加入 and 跳过.
func (p page) waiting(cx *ns.Scope, head func() Head) ns.Node {
	a := p.a
	colors := ui.ColorsOf(cx)
	items := live.Func(a, a.SubscriptionWaiting)
	return ns.Show{When: func() bool { return len(items()) > 0 }, Then: func(cx *ns.Scope) ns.Node {
		label := heading(cx, locales.SubscribePendingTitle(cx), func() string { return locales.SubscribePendingCount(cx, len(items())) }, "subscription-waiting-count",
			pill{TestID: "subscription-add-all", Look: LookPrimary, Text: func() string { return locales.SubscribeAddAll(cx) }, OnPress: live.Do(a, func() { a.AddWaiting(nil) })},
			pill{TestID: "subscription-skip-all", Look: LookSecondary, Text: func() string { return locales.SubscribeSkipAll(cx) }, OnPress: live.Do(a, func() { a.SkipWaiting(nil) })})
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
			titles := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 2}, Children: []ns.Node{
				ns.Text{Content: w.Title, MaxLines: 1, Style: ns.Style{FontSize: 14, LineHeight: 19, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)},
				ns.Show{When: func() bool { return meta() != "" }, Then: func(*ns.Scope) ns.Node {
					return ns.Text{ContentFunc: meta, MaxLines: 1, Style: ns.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)}
				}},
			}}
			add := pill{Text: func() string { return locales.SubscribeAddOne(cx) }, Label: locales.SubscribeAddOneLabel(cx, w.Title), Look: LookTint, TestID: "subscription-add-" + w.URL,
				OnPress: live.Do(a, func() { a.AddWaiting([]string{item().URL}) })}
			skip := pill{Text: func() string { return locales.SubscribeSkipOne(cx) }, Label: locales.SubscribeSkipOneLabel(cx, w.Title), Look: LookQuiet, TestID: "subscription-skip-" + w.URL,
				OnPress: live.Do(a, func() { a.SkipWaiting([]string{item().URL}) })}
			row := ns.View{Style: ns.Style{Direction: ns.Row, MinHeight: ns.Pt(64), Padding: ns.Symmetric(8, 16), Gap: 14, AlignItems: ns.AlignCenter},
				Children: []ns.Node{Thumb(head().Platform, 76, 44), titles, add, skip}}
			return ns.View{Children: []ns.Node{
				ns.Show{When: func() bool { return index() > 0 }, Then: func(*ns.Scope) ns.Node { return form.Separator(false) }},
				row,
			}}
		})
		return ns.View{Style: ns.Style{Gap: 10, AlignItems: ns.AlignStretch}, Children: []ns.Node{label, card(cx, ns.Style{}, rows)}}
	}}
}

// rules is the card of the rules, in the design's order: what happens to new videos, which ones are taken with the keyword field when keywords are chosen, the picture its videos download, whether they take the uploader's subtitles first on a site that has them, whether they are a series, which share a glossary, and how often the subscription is checked. A subscription that names no picture or source of lines shows the settings' (Settings › 通用 › 在线视频), which its jobs take.
func (p page) rules(cx *ns.Scope, head func() Head) ns.Node {
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
		return locales.SubscribeWhenNewAuto(cx, AddPerCheck)
	}
	quality := ui.Tabs[core.Quality]{TestID: "subscription-quality", Label: locales.SubscribeQuality(cx), Size: ui.SizeSmall,
		Options: qualityOptions(cx),
		Value: live.Func(a, func() core.Quality {
			if q := rules().Quality; q.Known() {
				return q
			}
			return a.DownloadQuality()
		}),
		OnChange: live.On(a, func(q core.Quality) {
			r := rules()
			r.Quality = q
			a.SetSubscriptionRules(r)
		})}
	rows := []ns.Node{
		form.Row{Title: locales.SubscribeWhenNew(cx), SubtitleFunc: modeNote, Trailing: []ns.Node{mode}},
		form.Separator(true),
		form.Row{Title: locales.SubscribeOnly(cx), Subtitle: locales.SubscribeOnlyNote(cx), Trailing: []ns.Node{filter}},
		keywords,
		form.Separator(true),
		form.Row{Title: locales.SubscribeQuality(cx), NoGroup: true, TestID: "subscription-quality-row", Trailing: []ns.Node{quality}},
	}
	// Only YouTube's and bilibili's uploaders' subtitles are taken (core.SubtitlesPlatform).
	if p := head().Platform; p == core.PlatformYouTube || p == core.PlatformBilibili {
		rows = append(rows, form.Separator(true),
			form.SwitchRow{TestID: "subscription-platform-subtitles", Title: locales.SubscribePlatformSubtitles(cx), Subtitle: locales.SubscribePlatformSubtitlesNote(cx),
				On: live.Func(a, func() bool {
					if s := rules().Subtitles; s.Known() {
						return s == core.SubtitlesPlatform
					}
					return a.PlatformSubtitles()
				}),
				OnChange: live.On(a, func(on bool) {
					r := rules()
					r.Subtitles = core.SubtitlesLocal
					if on {
						r.Subtitles = core.SubtitlesPlatform
					}
					a.SetSubscriptionRules(r)
				})})
	}
	rows = append(rows, form.Separator(true),
		form.SwitchRow{TestID: "subscription-series", Title: locales.SubscribeSeries(cx), Subtitle: locales.SubscribeSeriesNote(cx),
			On: func() bool { return rules().Series },
			OnChange: live.On(a, func(on bool) {
				r := rules()
				r.Series = on
				a.SetSubscriptionRules(r)
			})},
		form.Separator(true),
		form.Row{Title: locales.SubscribeCadence(cx), Subtitle: locales.SubscribeCadenceNote(cx), Trailing: []ns.Node{cadence}},
	)
	return ns.View{Style: ns.Style{Gap: 10, AlignItems: ns.AlignStretch}, Children: []ns.Node{
		heading(cx, locales.SubscribeRules(cx), nil, ""),
		card(cx, ns.Style{}, rows...),
	}}
}

// qualityOptions are the pictures a download asks for, as the design's 下载 and 保存 offer them: the best, 720p, or the subtitles alone.
func qualityOptions(cx *ns.Scope) []ui.Option[core.Quality] {
	return []ui.Option[core.Quality]{
		{Value: core.QualityBest, Label: locales.SettingsGeneralQualityBest(cx), TestID: "quality-best"},
		{Value: core.Quality720, Label: locales.SettingsGeneralQuality720(cx), TestID: "quality-720"},
		{Value: core.QualitySubtitles, Label: locales.SettingsGeneralQualitySubtitles(cx), TestID: "quality-subtitles"},
	}
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
			ns.View{Style: ns.Style{Direction: ns.Row, MinHeight: ns.Pt(46), Padding: ns.Symmetric(6, 14), Gap: 12, AlignItems: ns.AlignCenter}, Children: []ns.Node{title, stage}},
		}}
	})
	list := ns.Show{When: func() bool { return len(items()) > 0 },
		Then: func(cx *ns.Scope) ns.Node { return card(cx, ns.Style{}, rows) },
		Else: func(cx *ns.Scope) ns.Node {
			return card(cx, ns.Style{}, ns.Text{TestID: "subscription-added-none", Content: locales.SubscribeAddedNone(cx), Style: ns.Style{FontSize: 12, LineHeight: 17, Padding: ns.All(14)}, StyleFunc: ui.Colored(colors.Muted)})
		}}
	count := func() string { return strconv.Itoa(head().Added) }
	return ns.View{Style: ns.Style{Gap: 10, AlignItems: ns.AlignStretch}, Children: []ns.Node{heading(cx, locales.SubscribeAddedTitle(cx), count, "subscription-added-count"), list}}
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
