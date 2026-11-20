package player

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// settingsSheet is the playback settings, in a sheet over the player: the speed, the sleep timer, how far the film's lines are moved against its picture; shadowing, audio only, the lines on the picture while upright and whether the next film plays by itself; then picture in picture where the system offers it, Settings › Subtitle Style and the film's subtitles as a file.
func settingsSheet(cx *core.Scope, p *play, l *Layer, open *core.Signal[bool]) core.Node {
	label := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 13, LineHeight: 18, Padding: core.Edges{Left: 4}, Color: faint(0.6)}}
	}
	note := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 13, LineHeight: 17, Color: faint(0.6)}}
	}
	group := func(children ...core.Node) core.Node {
		return core.View{Style: core.Style{CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden, Background: faint(0.08)}, Children: children}
	}
	hairline := core.Style{Borders: core.Edges{Top: 0.5}, BorderColors: core.EdgeColors{Top: faint(0.1)}}
	var speeds []ui.Option[float32]
	for _, r := range Speeds {
		speeds = append(speeds, ui.Option[float32]{Value: r, Label: speedText(r), TestID: "player-settings-speed-" + speedText(r)})
	}
	var sleeps []ui.Option[sleepMode]
	for _, m := range sleepModes {
		sleeps = append(sleeps, ui.Option[sleepMode]{Value: m, TestID: "player-settings-sleep-" + sleepID(m), Content: core.Text{ContentFunc: func() string { return sleepName(cx, m) }, MaxLines: 2, Style: core.Style{TextAlign: core.TextAlignCenter}}})
	}
	toggle := func(id string, first bool, title, detail func() string, on func() bool, set func(bool)) core.Node {
		st := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(58), Padding: core.Symmetric(8, 16)}
		if !first {
			st = st.Merge(hairline)
		}
		return core.View{Style: st, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{ContentFunc: title, Style: core.Style{FontSize: 16, LineHeight: 21, Color: theme.PlayerInk}},
				note(detail),
			}},
			// TODO(nsgo#145): ui.Switch's Label is a string only (ui/switch.go:19), so its spoken name keeps the language it was built in; pass a LabelFunc once Switch has one https://github.com/nsgo-dev/nsgo/issues/145
			ui.Switch{TestID: id, Label: title(), Variant: ui.SwitchSystem, Checked: on, OnChange: set},
		}}
	}
	// A link's hairline over it is its holder's, as the rows before it show or not.
	link := func(id string, title, value func() string, press func()) core.Node {
		st := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(52), Padding: core.Symmetric(8, 16), Active: &core.Style{Background: faint(0.06)}}
		return base.Button{TestID: id, LabelFunc: title, OnPress: func(core.PressEvent) { press() }, Style: st, Children: []core.Node{
			core.Text{ContentFunc: title, MaxLines: 1, Style: core.Style{FontSize: 16, LineHeight: 21, FlexGrow: 1, FlexShrink: 1, Color: theme.PlayerInk}},
			core.Text{ContentFunc: value, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 19, Color: faint(0.5)}},
			ui.IconOf(look.Chevron, 13, func() core.Color { return faint(0.3) }),
		}}
	}
	round := func(id string, icon func() *core.SVG, text func() string, press func()) core.Node {
		return base.Button{TestID: id, LabelFunc: text, OnPress: func(core.PressEvent) { press() },
			Style: core.Style{Width: core.Pt(36), Height: core.Pt(36), CornerRadius: 18, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: faint(0.14),
				Active: &core.Style{Opacity: core.Alpha(0.7)}},
			Children: []core.Node{ui.IconOf(icon, 18, func() core.Color { return theme.PlayerInk })}}
	}
	pip := func() bool { return p.pip.Get().Supported }
	return ui.Sheet{Open: open.Get, OnClose: func() { open.Set(false) }, Children: []core.Node{
		core.Scroll{TestID: "player-settings", Style: core.Style{MaxHeight: core.Pct(85)}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 16, Padding: core.Edges{Top: 4, Bottom: 8}}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.PlayerSettings(cx) }, Role: core.RoleHeader, Style: look.Title.Merge(core.Style{FontWeight: look.Heavy, Padding: core.Edges{Left: 4}, Color: theme.PlayerInk})},
				// TODO(nsgo#145): ui.Tabs' Label is a string only (ui/tabs.go:40), so the speed and sleep tabs' spoken names keep the language they were built in; pass a LabelFunc once Tabs has one https://github.com/nsgo-dev/nsgo/issues/145
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					label(func() string { return locales.PlayerSpeed(cx) }),
					ui.Tabs[float32]{TestID: "player-settings-speed", Label: locales.PlayerSpeed(cx), Equal: true, Options: speeds, Value: p.speed, OnChange: func(r float32) { _ = p.rate.Set(r) },
						Style: core.Style{Background: faint(0.1)}},
				}},
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					label(func() string { return locales.PlayerSleep(cx) }),
					ui.Tabs[sleepMode]{TestID: "player-settings-sleep", Label: locales.PlayerSleep(cx), Equal: true, Options: sleeps, Value: p.sleep.Get, OnChange: p.setSleep,
						Style: core.Style{Background: faint(0.1)}},
				}},
				core.View{Style: core.Style{Gap: 8, Padding: core.Symmetric(12, 14), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, Background: faint(0.08)}, Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return locales.PlayerDelay(cx) }, Style: core.Style{FontSize: 16, LineHeight: 21, FlexGrow: 1, FlexShrink: 1, Color: theme.PlayerInk}},
						round("player-delay-earlier", lucide.Minus, func() string { return locales.PlayerDelayEarlier(cx) }, func() { p.setDelay(p.delay.GetUntracked() - delayStep) }),
						core.Text{TestID: "player-delay", Role: core.RoleButton, ContentFunc: func() string { return locales.PlayerDelaySeconds(cx, delayText(p.delay.Get())) },
							LabelFunc: func() string { return locales.PlayerDelayReset(cx, delayText(p.delay.Get())) },
							OnPress:   func(core.PressEvent) { p.setDelay(0) },
							Style:     mono.Merge(core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, MinWidth: core.Pt(64), TextAlign: core.TextAlignCenter, Color: theme.PlayerInk})},
						round("player-delay-later", lucide.Plus, func() string { return locales.PlayerDelayLater(cx) }, func() { p.setDelay(p.delay.GetUntracked() + delayStep) }),
					}},
					note(func() string { return locales.PlayerDelayNote(cx) }),
				}},
				group(
					toggle("player-settings-follow", true, func() string { return locales.PlayerFollow(cx) }, func() string { return locales.PlayerFollowNote(cx) }, p.follow.Get, func(on bool) {
						_ = p.follow.Set(on)
						if !on {
							p.held.Set(false)
						}
					}),
					toggle("player-settings-audio-only", false, func() string { return locales.PlayerAudioOnly(cx) }, func() string { return locales.PlayerAudioOnlyNote(cx) }, p.audioOnly.Get, func(on bool) { _ = p.audioOnly.Set(on) }),
					toggle("player-settings-on-picture", false, func() string { return locales.PlayerOnPicture(cx) }, func() string { return locales.PlayerOnPictureNote(cx) }, p.onPicture.Get, func(on bool) { _ = p.onPicture.Set(on) }),
					toggle("player-settings-auto-next", false, func() string { return locales.PlayerAutoNext(cx) }, func() string { return locales.PlayerAutoNextNote(cx) }, p.autoNext.Get, func(on bool) {
						_ = p.autoNext.Set(on)
						if !on {
							p.closeNext()
						}
					}),
				),
				group(
					core.Show{When: pip, Then: func(cx *core.Scope) core.Node {
						return link("player-settings-pip", func() string { return locales.PlayerPictureInPicture(cx) }, func() string { return "" }, func() {
							open.Set(false)
							p.ref.StartPictureInPicture()
						})
					}},
					core.Show{When: func() bool { return l.OpenStyle != nil }, Then: func(cx *core.Scope) core.Node {
						return core.View{StyleFunc: func() core.Style {
							if pip() {
								return hairline
							}
							return core.Style{}
						}, Children: []core.Node{link("player-settings-style", func() string { return locales.SettingsSubtitleStyle(cx) }, func() string { return locales.SettingsTitle(cx) }, func() {
							open.Set(false)
							l.OpenStyle()
						})}}
					}},
					core.View{StyleFunc: func() core.Style {
						if pip() || l.OpenStyle != nil {
							return hairline
						}
						return core.Style{}
					}, Children: []core.Node{link("player-settings-export", func() string { return locales.PlayerExportSubtitles(cx) }, func() string { return ".srt" }, func() {
						open.Set(false)
						exportSubtitles(cx, p)
					})}},
				),
			}},
		}},
	}}
}

// sleepID names a sleep mode in test IDs.
func sleepID(m sleepMode) string {
	switch m {
	case sleepLine:
		return "line"
	case sleep15:
		return "15"
	case sleep30:
		return "30"
	case sleepFilm:
		return "film"
	}
	return "off"
}

// sleepName is what the settings call a sleep mode.
func sleepName(cx *core.Scope, m sleepMode) string {
	switch m {
	case sleepLine:
		return locales.PlayerSleepLine(cx)
	case sleep15, sleep30:
		return locales.PlayerSleepMinutes(cx, m.minutes())
	case sleepFilm:
		return locales.PlayerSleepFilm(cx)
	}
	return locales.PlayerSleepOff(cx)
}
