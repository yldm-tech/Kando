package player

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// settingsSheet is the playback settings, in a sheet over the player: the speed, the sleep timer, how far the film's lines are moved against its picture, shadowing, audio only and the lines on the picture while upright.
// TODO(nsgo#54): offer picture in picture here once NSGo plays a video in the system's floating window https://github.com/nsgo-dev/nsgo/issues/54
func settingsSheet(cx *core.Scope, p *play, open *core.Signal[bool]) core.Node {
	colors := ui.ColorsOf(cx)
	heading := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: look.Section, StyleFunc: ui.Colored(colors.Muted)}
	}
	note := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}
	}
	var speeds []ui.Option[float32]
	for _, r := range Speeds {
		speeds = append(speeds, ui.Option[float32]{Value: r, Label: speedText(r), TestID: "player-settings-speed-" + speedText(r)})
	}
	var sleeps []ui.Option[sleepMode]
	for _, m := range sleepModes {
		sleeps = append(sleeps, ui.Option[sleepMode]{Value: m, TestID: "player-settings-sleep-" + sleepID(m), Content: core.Text{ContentFunc: func() string { return sleepName(cx, m) }, MaxLines: 1}})
	}
	toggle := func(id string, title, detail func() string, on func() bool, set func(bool)) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{ContentFunc: title, Style: look.Body, StyleFunc: ui.Colored(colors.Text)},
				note(detail),
			}},
			// TODO(nsgo#145): ui.Switch's Label is a string only (ui/switch.go:19), so its spoken name keeps the language it was built in; pass a LabelFunc once Switch has one https://github.com/nsgo-dev/nsgo/issues/145
			ui.Switch{TestID: id, Label: title(), Checked: on, OnChange: set},
		}}
	}
	return ui.Sheet{Open: open.Get, OnClose: func() { open.Set(false) }, Children: []core.Node{
		core.Scroll{TestID: "player-settings", Style: core.Style{MaxHeight: core.Pct(85)}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 18, Padding: core.Edges{Bottom: 8}}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.PlayerSettings(cx) }, Role: core.RoleHeader, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
				// TODO(nsgo#145): ui.Tabs' Label is a string only (ui/tabs.go:40), so the speed and sleep tabs' spoken names keep the language they were built in; pass a LabelFunc once Tabs has one https://github.com/nsgo-dev/nsgo/issues/145
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					heading(func() string { return locales.PlayerSpeed(cx) }),
					ui.Tabs[float32]{TestID: "player-settings-speed", Label: locales.PlayerSpeed(cx), Equal: true, Options: speeds, Value: p.speed, OnChange: func(r float32) { _ = p.rate.Set(r) }},
				}},
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					heading(func() string { return locales.PlayerSleep(cx) }),
					ui.Tabs[sleepMode]{TestID: "player-settings-sleep", Label: locales.PlayerSleep(cx), Equal: true, Options: sleeps, Value: p.sleep.Get, OnChange: p.setSleep},
				}},
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					heading(func() string { return locales.PlayerDelay(cx) }),
					core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
						ui.IconButton{TestID: "player-delay-earlier", Icon: lucide.Minus, LabelFunc: func() string { return locales.PlayerDelayEarlier(cx) }, Variant: ui.IconButtonRaised, Size: 44,
							OnPress: func() { p.setDelay(p.delay.GetUntracked() - delayStep) }},
						ui.Tabular(core.Text{TestID: "player-delay", Role: core.RoleButton, ContentFunc: func() string { return locales.PlayerDelaySeconds(cx, delayText(p.delay.Get())) },
							LabelFunc: func() string { return locales.PlayerDelayReset(cx, delayText(p.delay.Get())) },
							OnPress:   func(core.PressEvent) { p.setDelay(0) },
							Style:     look.Title.Merge(core.Style{FlexGrow: 1, TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)}),
						ui.IconButton{TestID: "player-delay-later", Icon: lucide.Plus, LabelFunc: func() string { return locales.PlayerDelayLater(cx) }, Variant: ui.IconButtonRaised, Size: 44,
							OnPress: func() { p.setDelay(p.delay.GetUntracked() + delayStep) }},
					}},
					note(func() string { return locales.PlayerDelayNote(cx) }),
				}},
				toggle("player-settings-follow", func() string { return locales.PlayerFollow(cx) }, func() string { return locales.PlayerFollowNote(cx) }, p.follow.Get, func(on bool) {
					_ = p.follow.Set(on)
					if !on {
						p.held.Set(false)
					}
				}),
				toggle("player-settings-audio-only", func() string { return locales.PlayerAudioOnly(cx) }, func() string { return locales.PlayerAudioOnlyNote(cx) }, p.audioOnly.Get, func(on bool) { _ = p.audioOnly.Set(on) }),
				toggle("player-settings-on-picture", func() string { return locales.PlayerOnPicture(cx) }, func() string { return locales.PlayerOnPictureNote(cx) }, p.onPicture.Get, func(on bool) { _ = p.onPicture.Set(on) }),
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
