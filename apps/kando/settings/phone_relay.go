package settings

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// PhoneRelay is the storage relay as the 手机同步 page shows it: phones away from this computer's network reach it through the storage of 账号与上传 (yldm-tech/Kando#219).
type PhoneRelay struct {
	// On is the relay's switch, which settings.json keeps.
	On bool
	// Store names the storage the relay goes through, as "R2 · films", "" while none is set up in 账号与上传.
	Store string
	// Offline is 隐私's 离线模式, under which the relay does not reach the storage.
	Offline bool
	// Working is a turn of the relay that went through since it started, and Err why its last turn failed, in the interface's language, "" for none.
	Working bool
	Err     string
	// SubtitlesOnly is 只同步字幕, which settings.json keeps: the films go to the storage as their lines and posters, without their videos.
	SubtitlesOnly bool
	// Storage is what the films and the messages take in the storage (phone_storage.go).
	Storage PhoneStorage
}

// relayState is what the relay's status row says.
type relayState uint8

const (
	relayOff relayState = iota
	relayOffline
	relayNoStore
	relayFailed
	relayWorking
	relayChecking
)

func (r PhoneRelay) state() relayState {
	switch {
	case !r.On:
		return relayOff
	case r.Offline:
		return relayOffline
	case r.Store == "":
		return relayNoStore
	case r.Err != "":
		return relayFailed
	case r.Working:
		return relayWorking
	}
	return relayChecking
}

// relayCard is the card of 不在同一网络时: the relay's switch, the storage it goes through with 去设置… to 账号与上传, and while it is on, whether it works.
func relayCard(cx *core.Scope, a App) core.Node {
	colors := ui.ColorsOf(cx)
	k := theme.Of(cx)
	relay := func() PhoneRelay { return a.PhoneSync().Relay }
	toggle := form.SwitchRow{Title: locales.SettingsPhoneRelay(cx), Subtitle: locales.SettingsPhoneRelayHint(cx), TestID: "settings-phone-relay",
		On:       live.Func(a, func() bool { return relay().On }),
		OnChange: live.On(a, a.SetPhoneRelay)}
	store := form.Row{Title: locales.SettingsPhoneRelayStore(cx), TestID: "settings-phone-relay-store", Trailing: []core.Node{
		core.Text{TestID: "settings-phone-relay-store-name", ContentFunc: live.Func(a, func() string {
			if s := relay().Store; s != "" {
				return s
			}
			return locales.SettingsPhoneRelayNoStore(cx)
		}), MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: core.Style{FontSize: 12, LineHeight: 17, MaxWidth: core.Pt(220), FlexShrink: 1}, StyleFunc: ui.Colored(colors.Muted)},
		form.Button{Text: locales.SettingsPhoneRelaySetUp(cx), TestID: "settings-phone-relay-set-up", OnPress: live.Do(a, func() { a.SetSettingsSection(SectionAccount) })},
	}}
	state := core.NewMemo(cx, live.Func(a, func() relayState { return relay().state() }))
	dot := func() core.Color {
		switch state.Get() {
		case relayWorking:
			return form.OK
		case relayFailed, relayNoStore:
			return colors.Danger()
		}
		return k.Idle()
	}
	color := func() core.Color {
		switch state.Get() {
		case relayWorking:
			return k.SuccessInk()
		case relayFailed, relayNoStore:
			return colors.Danger()
		}
		return colors.Muted()
	}
	text := live.Func(a, func() string {
		switch state.Get() {
		case relayOffline:
			return locales.SettingsPhoneRelayOffline(cx)
		case relayNoStore:
			return locales.SettingsPhoneRelayNeedsStore(cx)
		case relayFailed:
			return relay().Err
		case relayWorking:
			return locales.SettingsPhoneRelayWorking(cx)
		}
		return locales.SettingsPhoneRelayChecking(cx)
	})
	status := core.Show{When: func() bool { return state.Get() != relayOff }, Then: func(cx *core.Scope) core.Node {
		return core.Group{form.Separator(true), form.Row{Title: locales.SettingsPhoneRelayStatus(cx), TestID: "settings-phone-relay-status", Trailing: []core.Node{
			form.Status(dot, color, text),
		}}}
	}}
	// 只同步字幕 shows while the relay is on, as it is about what goes through it.
	subtitles := core.Show{When: live.Func(a, func() bool { return relay().On }), Then: func(cx *core.Scope) core.Node {
		return core.Group{form.Separator(true), form.SwitchRow{Title: locales.SettingsPhoneRelaySubtitlesOnly(cx), Subtitle: locales.SettingsPhoneRelaySubtitlesOnlyHint(cx), TestID: "settings-phone-relay-subtitles-only",
			On:       live.Func(a, func() bool { return relay().SubtitlesOnly }),
			OnChange: live.On(a, a.SetPhoneRelaySubtitlesOnly)}}
	}}
	return form.Card(toggle, form.Separator(true), store, status, subtitles)
}

// storageSection is the 存储用量 label and card, while the relay is on with a storage set up and not paused by 离线模式.
func storageSection(cx *core.Scope, a App) core.Node {
	shown := live.Func(a, func() bool {
		r := a.PhoneSync().Relay
		return r.state() == relayWorking || r.state() == relayChecking || r.state() == relayFailed
	})
	return core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		where := core.Text{TestID: "settings-phone-storage-where", ContentFunc: live.Func(a, func() string { return a.PhoneSync().Relay.Store }), MaxLines: 1,
			Style: core.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)}
		return core.Group{form.Label(locales.SettingsPhoneStorage(cx), 14, where), storageCard(cx, a)}
	}}
}
