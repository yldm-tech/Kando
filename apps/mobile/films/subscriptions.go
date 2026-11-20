package films

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// subscriptions are the paired computers' subscriptions of the library the switcher shows, every library's while it shows all of them.
func (v *view) subscriptions() []session.Subscription {
	lib := v.library.Get()
	var out []session.Subscription
	for _, sub := range v.s.Subscriptions() {
		if lib.Desktop == "" || (sub.Desktop == lib.Desktop && sub.Library == lib.ID) {
			out = append(out, sub)
		}
	}
	return out
}

// canSubscribe reports whether a paired computer takes a list link to subscribe to, which the Add sheet sends it.
func canSubscribe(s *session.Session) bool {
	for _, d := range s.Desktops() {
		if s.Queue(d.ID).Can(sync.OpSubscribe) {
			return true
		}
	}
	return false
}

// subscriptionRow is the subscriptions under their heading, in a row that scrolls sideways, each its mark with the count of its new videos waiting, and the way to add one, through the Add sheet, which takes a channel's or a playlist's link. A subscription pressed opens its sheet.
func (v *view) subscriptionRow(cx *core.Scope) core.Node {
	return core.View{TestID: "films-subscriptions", Style: core.Style{Gap: 12}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{look.SectionTitle(cx, func() string { return locales.FilmsSubscriptions(cx) })}},
			core.Show{When: func() bool { return v.act.Add != nil && canSubscribe(v.s) }, Then: func(cx *core.Scope) core.Node {
				colors := ui.ColorsOf(cx)
				return core.View{TestID: "films-subscriptions-add", Role: core.RoleButton, LabelFunc: func() string { return locales.FilmsSubscriptionsAddLabel(cx) },
					OnPress: func(core.PressEvent) { v.act.Add() },
					Style:   core.Style{MinHeight: core.Pt(44), JustifyContent: core.JustifyCenter, Padding: core.Symmetric(0, 4), Active: &core.Style{Opacity: core.Alpha(0.6)}},
					Children: []core.Node{core.Text{ContentFunc: func() string { return locales.FilmsSubscriptionsAdd(cx) }, MaxLines: 1,
						Style: look.Subhead, StyleFunc: ui.Colored(colors.Accent)}}}
			}},
		}},
		core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{FlexGrow: 0, Margin: core.Edges{Right: -16, Left: -16}}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 14, Padding: core.Symmetric(0, 16)}, Children: []core.Node{
				core.Each(v.subscriptions, func(sub session.Subscription) string { return sub.Key() }, func(cx *core.Scope, sub func() session.Subscription, _ func() int) core.Node {
					return v.subscriptionMark(cx, sub)
				}),
			}},
		}},
	}}
}

// markFill is the fill of sub's mark: its site's color, as the films' source marks have it.
func markFill(sub session.Subscription) core.Color {
	switch sub.Platform {
	case sync.PlatformYouTube:
		return theme.SourceYouTube
	case sync.PlatformBilibili:
		return theme.SourceBilibili
	}
	return theme.SourceOther
}

// avatar is sub's mark: a circle size across in its site's color with its initial in white.
func avatar(cx *core.Scope, sub func() session.Subscription, size core.Points) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
		StyleFunc: func() core.Style { return core.Style{Background: markFill(sub())} },
		Children: []core.Node{core.Text{ContentFunc: func() string { return initial(sub().Name) }, MaxLines: 1,
			Style: core.Style{FontSize: 22, FontWeight: look.Heavy}, StyleFunc: ui.Colored(ui.ColorsOf(cx).OnAccent)}}}
}

// initial is the letter sub's mark shows: the first letter or character of its name, in upper case.
func initial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return string(unicode.ToUpper(r))
		}
	}
	if r, _ := utf8.DecodeRuneInString(strings.TrimSpace(name)); r != utf8.RuneError {
		return string(r)
	}
	return "?"
}

// trouble reports whether sub's last check found its list gone or failed.
func trouble(sub session.Subscription) bool { return sub.Gone || sub.Error != "" }

// subscriptionMark is a subscription in the row: its site's mark with its initial, ringed in the accent while new videos wait on it, with how many, or in the warning's color while its list is gone or its last check failed; dimmed while paused; its name under it.
func (v *view) subscriptionMark(cx *core.Scope, sub func() session.Subscription) core.Node {
	colors := ui.ColorsOf(cx)
	waiting := func() int { return len(sub().Waiting) }
	id := sub().Desktop + "-" + sub().ID
	return core.View{TestID: "films-subscription-" + id, Role: core.RoleButton, LabelFunc: func() string { return subscriptionLabel(cx, sub()) },
		OnPress: func(core.PressEvent) { v.act.Subscription(sub().Key()) },
		Style:   core.Style{Width: core.Pt(72), AlignItems: core.AlignCenter, Gap: 6, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style {
			if sub().Paused {
				return core.Style{Opacity: core.Alpha(0.5)}
			}
			return core.Style{Opacity: core.Alpha(1)}
		},
		Children: []core.Node{
			// The ring is 2 points of the accent, or of the warning's orange, 2.5 points clear of the 60-point mark, the page's color between them.
			core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Width: core.Pt(69), Height: core.Pt(69), CornerRadius: 34.5, BorderWidth: 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Margin: core.Edges{Top: -4.5, Bottom: -4.5}},
				StyleFunc: func() core.Style {
					ring := ui.Alpha(colors.Text(), 0)
					switch {
					case waiting() > 0:
						ring = colors.Accent()
					case trouble(sub()):
						ring = theme.BadgeOrange
					}
					return core.Style{BorderColor: ring}
				},
				Children: []core.Node{
					avatar(cx, sub, 60),
					core.Show{When: func() bool { return waiting() > 0 }, Then: func(cx *core.Scope) core.Node {
						return core.View{Style: core.Style{Position: core.Absolute, Right: core.Pt(0.5), Top: core.Pt(0.5), MinWidth: core.Pt(24), Height: core.Pt(24), Padding: core.Symmetric(0, 7), CornerRadius: 12,
							AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, BorderWidth: 2, Background: theme.BadgeRed},
							StyleFunc: func() core.Style { return core.Style{BorderColor: colors.Background()} },
							Children: []core.Node{ui.Tabular(core.Text{TestID: "films-subscription-new-" + id, ContentFunc: func() string { return locales.FilmsSubscriptionNew(cx, waiting()) }, MaxLines: 1,
								Style: core.Style{FontSize: 11, LineHeight: 16, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.OnAccent)})}}
					}},
				}},
			core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{MaxWidth: core.Pt(72)}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return sub().Name }, MaxLines: 1, Style: look.Footnote.Merge(core.Style{FontWeight: core.FontWeightMedium}), StyleFunc: ui.Colored(colors.Text)},
			}},
		}}
}

// subscriptionLabel is what assistive technology reads of sub in the row and the sidebar: its name, with the count of new videos waiting on it, or that its list is gone or it is paused.
func subscriptionLabel(cx *core.Scope, sub session.Subscription) string {
	switch {
	case sub.Gone:
		return locales.FilmsSubscriptionGoneLabel(cx, sub.Name)
	case sub.Paused:
		return locales.FilmsSubscriptionPausedLabel(cx, sub.Name)
	}
	return locales.FilmsSubscriptionLabel(cx, sub.Name, len(sub.Waiting))
}

// subscriptionKind is what sub follows, as its sheet names it.
func subscriptionKind(cx *core.Scope, sub session.Subscription) string {
	switch sub.Kind {
	case sync.SubscriptionChannel:
		return locales.FilmsSubscriptionChannel(cx)
	case sync.SubscriptionSpace:
		return locales.FilmsSubscriptionSpace(cx)
	case sync.SubscriptionCollection:
		return locales.FilmsSubscriptionCollection(cx)
	}
	return locales.FilmsSubscriptionPlaylist(cx)
}

// subscriptionSite is the site sub is on, as the source chips name it.
func subscriptionSite(cx *core.Scope, sub session.Subscription) string {
	switch sub.Platform {
	case sync.PlatformYouTube:
		return locales.FilmsSourceYoutube(cx)
	case sync.PlatformBilibili:
		return locales.FilmsSourceBilibili(cx)
	}
	return locales.FilmsSourceWeb(cx)
}

// SubscriptionSheet is the sheet of the subscription whose key open holds, "" for none, as the Library's row and a tablet's sidebar open it, a dialog in the middle of a tablet's screen: headed by its mark and name over its site, what it follows and its computer, or in their place why its computer does not check it now, the new videos waiting on it with the way to send them all to the queue, its auto-download, and the way to unsubscribe, which the computer keeps the videos it added through. A phone the computer made read-only sees it without the actions. It closes once the subscription is gone. It belongs over the app, so it shows whatever section a tablet shows.
func SubscriptionSheet(cx *core.Scope, s *session.Session, open *core.Signal[string]) core.Node {
	colors := ui.ColorsOf(cx)
	shown := func() (session.Subscription, bool) {
		key := open.Get()
		if key == "" {
			return session.Subscription{}, false
		}
		return s.Subscription(key)
	}
	// A subscription gone from its computer's list, as one unsubscribed on the computer and read again, closes its sheet: the key is cleared after this turn, which the sheet follows.
	// TODO(nsgo#782): headless nsgotest takes no frames after Answer, so a sheet (base.Sheet) closed in the turn an alert over it was answered stays in the tree until the test advances the clock; clearing the key a turn later, once the list changed, closes it there. Set it directly once Answer takes them. https://github.com/nsgo-dev/nsgo/issues/782
	cx.Effect(func() {
		if _, ok := shown(); !ok && open.Get() != "" {
			cx.Post(func() { open.Set("") })
		}
	})
	sub := func() session.Subscription { got, _ := shown(); return got }
	computer := func() string { d, _ := s.Desktop(sub().Desktop); return d.Name }
	lets := func(op string) bool { return s.Queue(sub().Desktop).Can(op) && !s.SubscriptionBusy(sub()) }
	waiting := func() int { return len(sub().Waiting) }
	close := func() { open.Set("") }
	unsubscribe := func() {
		target := sub()
		cx.ShowAlert(core.AlertConfig{Title: locales.FilmsUnsubscribeTitle(cx, target.Name), Message: locales.FilmsUnsubscribeMessage(cx, computer()), Actions: []core.AlertAction{
			{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
			{Label: locales.FilmsUnsubscribe(cx), Role: core.AlertDestructive},
		}}, func(i int) {
			if i == 1 {
				s.Unsubscribe(target)
			}
		})
	}
	// meta is the line under its name: its site, what it follows and its computer, or in their place why its computer does not check it now.
	meta := func() string {
		got := sub()
		site := subscriptionSite(cx, got)
		switch {
		case got.Gone:
			return site + " · " + locales.FilmsSubscriptionGone(cx, site, computer())
		case got.Error != "":
			return site + " · " + locales.FilmsSubscriptionFailed(cx, got.Error)
		case got.Paused:
			return site + " · " + locales.FilmsSubscriptionPaused(cx, computer())
		}
		return locales.FilmsSubscriptionMeta(cx, site, subscriptionKind(cx, got), computer())
	}
	header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 10}}, Children: []core.Node{
		avatar(cx, sub, 54),
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return sub().Name }, Role: core.RoleHeader, MaxLines: 2, Style: core.Style{FontSize: 19, LineHeight: 24, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{TestID: "films-subscription-meta", ContentFunc: meta, LiveRegion: core.LiveRegionPolite, MaxLines: 4, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
		}},
	}}
	// A touch screen's sheet closes by its grabber or a press beside it; a computer's dialog keeps its Done.
	var footer []core.Node
	if !ui.Touch(cx) {
		footer = []core.Node{look.Button{Kind: look.Plain, Text: func() string { return locales.FilmsSubscriptionDone(cx) }, TestID: "films-subscription-done", OnPress: close}}
	}
	return look.Dialog{TestID: "films-subscription-sheet", Open: func() bool { _, ok := shown(); return ok }, Title: func() string { return sub().Name }, Header: header,
		OnClose: close, Footer: footer,
		Children: []core.Node{
			core.Show{When: func() bool { q := s.Queue(sub().Desktop); return q.Reach == session.Online && q.ReadOnly }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "films-subscription-read-only", ContentFunc: func() string { return locales.FilmsSubscriptionReadOnly(cx, computer()) },
					Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 4, Left: 4}}), StyleFunc: ui.Colored(colors.Muted)}
			}},
			core.View{Style: core.Style{Gap: 6, Margin: core.Edges{Top: 2}}, Children: []core.Node{
				func() core.Node {
					h := look.GroupHeader(cx, func() string { return locales.FilmsSubscriptionWaiting(cx, waiting()) })
					h.TestID = "films-subscription-waiting"
					h.Style.Padding = core.Edges{Right: 4, Left: 4}
					return h
				}(),
				core.Show{When: func() bool { return waiting() > 0 },
					Then: func(cx *core.Scope) core.Node {
						return core.Scroll{Style: core.Style{FlexGrow: 0, MaxHeight: core.Pt(260)}, Children: []core.Node{look.Group{Children: []core.Node{
							core.Each(func() []sync.SubscriptionVideo { return sub().Waiting }, func(w sync.SubscriptionVideo) string { return w.URL }, func(cx *core.Scope, w func() sync.SubscriptionVideo, i func() int) core.Node {
								return look.Row{Index: i, TitleLines: 2, TitleStyle: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightSemibold}, Title: func() string {
									if w().Title != "" {
										return w().Title
									}
									return w().URL
								}}
							}),
						}}}}
					},
					Else: func(cx *core.Scope) core.Node {
						return look.Group{Children: []core.Node{core.Text{TestID: "films-subscription-none", ContentFunc: func() string {
							if sub().Auto {
								return locales.FilmsSubscriptionNoneAuto(cx)
							}
							return locales.FilmsSubscriptionNone(cx)
						}, Style: core.Style{FontSize: 14, LineHeight: 19, TextAlign: core.TextAlignCenter, Padding: core.Symmetric(20, 16)}, StyleFunc: ui.Colored(colors.Muted)}}}
					}},
			}},
			core.Show{When: func() bool { return waiting() > 0 && s.Queue(sub().Desktop).Can(sync.OpSubscriptionTake) }, Then: func(cx *core.Scope) core.Node {
				return look.Button{TestID: "films-subscription-take", Kind: look.Primary, Text: func() string { return locales.FilmsSubscriptionTake(cx) },
					Busy: func() bool { return s.SubscriptionBusy(sub()) }, Disabled: func() bool { return !lets(sync.OpSubscriptionTake) },
					OnPress: func() { s.TakeSubscription(sub()) }}
			}},
			core.Show{When: func() bool {
				q := s.Queue(sub().Desktop)
				return q.Can(sync.OpSubscriptionSet) || q.Can(sync.OpUnsubscribe)
			}, Then: func(cx *core.Scope) core.Node {
				return look.Group{Children: []core.Node{
					core.Show{When: func() bool { return s.Queue(sub().Desktop).Can(sync.OpSubscriptionSet) }, Then: func(cx *core.Scope) core.Node {
						// TODO(nsgo#145): ui.Switch's Label is a string only (ui/switch.go:19), so its spoken name keeps the language it was built in; pass a LabelFunc once Switch has one https://github.com/nsgo-dev/nsgo/issues/145
						return look.Row{TestID: "films-subscription-auto", TitleLines: 1,
							Title: func() string { return locales.FilmsSubscriptionAuto(cx) },
							Trailing: ui.Switch{TestID: "films-subscription-auto-switch", Label: locales.FilmsSubscriptionAuto(cx), Variant: ui.SwitchSystem,
								Checked: func() bool { return sub().Auto }, Disabled: func() bool { return !lets(sync.OpSubscriptionSet) },
								OnChange: func(on bool) { s.SetSubscriptionAuto(sub(), on) }}}
					}},
					core.Show{When: func() bool { return s.Queue(sub().Desktop).Can(sync.OpUnsubscribe) }, Then: func(cx *core.Scope) core.Node {
						return look.Row{TestID: "films-subscription-unsubscribe", TitleTone: look.ToneDanger,
							Index: func() int {
								if s.Queue(sub().Desktop).Can(sync.OpSubscriptionSet) {
									return 1
								}
								return 0
							},
							Title: func() string { return locales.FilmsUnsubscribe(cx) },
							OnPress: func() {
								if lets(sync.OpUnsubscribe) {
									unsubscribe()
								}
							}}
					}},
				}}
			}},
		}}
}
